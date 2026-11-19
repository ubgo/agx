package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/pool"
)

// credentialVerb is the hidden command the live-login helper runs (spec
// 10, 12): "<agx path> credential".
const credentialVerb = "credential"

// selfFallback is the helper program when the running binary's path is
// unknown (Deps.Self empty): agx on the agent's PATH.
const selfFallback = "agx"

// envSettingPrefix is the dotted --setting prefix that writes the
// settings "env" block (--setting env.NAME=value), which can carry a
// variable that outranks the live-login helper.
const envSettingPrefix = "env."

// livePlan returns the live-login additions of a launch (spec 10): nil
// when the run is not live (no --live-login, profile live_login false).
// The launch-config step merges plan.Settings into the run's settings,
// plan.Env into the env and unsets plan.Scrub. livePlan sets p.Live (what
// Commit writes to live.jsonc) and adds the decision line.
//
// Refused, each before anything is written:
//   - an agent without the live-credential capability (agentio's typed
//     reason, "not switchable");
//   - a run on the home login, or a one-off --token-ref: agx credential
//     resolves a named login each time the agent asks, and there is none;
//   - an api-key-helper login: the agent would run agx, which would have
//     to run the helper, and agx never runs a credential command it only
//     passes on;
//   - a variable set by the profile, --env or --setting env.* that
//     outranks the helper (plan.Scrub), so a kimi-style profile with its
//     own ANTHROPIC_AUTH_TOKEN cannot be live.
func (a *App) livePlan(ctx context.Context, o LaunchOptions, res resolve.Result, p *LaunchPlan) (*agentio.LiveCredentialPlan, error) {
	isPool := res.Login.Kind == resolve.LoginNamed && res.Login.Login.Kind == config.KindPool
	if o.PoolForce != "" && !isPool {
		return nil, apperr.Usage(fmt.Sprintf("--pool-force picks a member of a pool login; this run's login is %s", loginLabel(res.Login))).
			WithHint("agx run --login POOL --pool-force MEMBER")
	}
	if !o.LiveLogin && !res.Profile.LiveLogin && !isPool {
		return nil, nil
	}
	src := "--live-login"
	switch {
	case isPool:
		// A pool is always live: its key changes while the agent runs.
		src = "login " + res.Login.Name + " (pool)"
	case !o.LiveLogin:
		src = "profile " + res.ProfileName + " (live_login)"
	}
	id := res.Agent.ID()
	lc, ok := agentio.LiveCredentialOf(res.Agent)
	if !ok {
		reason, _ := agentio.Unsupported(res.Agent, agentio.CapLiveCredential)
		if reason == "" {
			reason = "no live credential"
		}
		return nil, apperr.Unsupported(fmt.Sprintf("%s is not switchable (%s); %s cannot apply", id, reason, src))
	}
	switch res.Login.Kind {
	case resolve.LoginHome:
		return nil, apperr.Unsupported(fmt.Sprintf("%s needs a stored login; this run starts on the home login (%s)", src, res.Login.Source)).
			WithHint("agx run --login NAME --live-login")
	case resolve.LoginTokenRef:
		return nil, apperr.Unsupported(src + " needs a stored login; a one-off --token-ref cannot be fetched again by agx credential").
			WithHint("agx login add --name NAME --agent " + string(id) + " --kind oauth-token --token-ref REF")
	}
	if res.Login.Login.Kind == config.KindAPIKeyHelper {
		return nil, apperr.Unsupported(fmt.Sprintf("login %s is an api-key-helper; %s needs a token login", res.Login.Name, src))
	}
	helper := a.liveHelper()
	var refresh time.Duration
	if isPool && res.Login.Login.Pool != nil {
		ttl, err := res.Login.Login.Pool.TTL()
		if err != nil {
			return nil, apperr.Usage("login " + res.Login.Name + ": " + err.Error())
		}
		refresh = ttl
	}
	plan, err := lc.LiveCredential(helper, refresh)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnsupported, apperr.ClassUnsupported, string(id)+": "+err.Error())
	}
	if err := outranksHelper(o, res, plan.Scrub, src); err != nil {
		return nil, err
	}
	p.Live = &LiveStart{StartLogin: res.Login.Name, Helper: helper}
	if isPool && o.PoolForce != "" {
		cfg, err := a.Config()
		if err != nil {
			return nil, err
		}
		if err := a.checkForce(cfg.Config, o.PoolForce, id); err != nil {
			return nil, err
		}
		p.Live.PoolForce = o.PoolForce
	}
	if isPool {
		// The settings line names the pool; other live runs keep their
		// documented "--live-login" source.
		p.liveSrc = src
		p.Decisions = append(p.Decisions, a.poolPlanNotes(ctx, o, res, p)...)
	}
	p.Decisions = append(p.Decisions, report.Decision{
		Name:   resolve.DecNote,
		Value:  fmt.Sprintf("live login: switch with agx live use --id %s --login NAME (refresh %s)", p.RunID, plan.Refresh),
		Source: src,
	})
	return &plan, nil
}

// decPool is the decision name of a pool launch's lines.
const decPool = "pool"

// poolPlanNotes are a pool launch's decisions, each with its source, as
// the rest of the chain shows them: the members, who picks (a run force,
// the pool's force, its picker command, the built-in rules), the timings
// and the rules' numbers (config or default), then in a dry run the
// member the pool would hand out now. A dry run probes nothing (cost R):
// that pick comes from the login-usage cache, of any age, and says so.
func (a *App) poolPlanNotes(ctx context.Context, o LaunchOptions, res resolve.Result, p *LaunchPlan) []report.Decision {
	lg, name := res.Login.Login, res.Login.Name
	key := "config logins." + name + ".pool"
	src := func(set bool, k string) string {
		if set {
			return key + "." + k
		}
		return "default"
	}
	pl := lg.Pool
	rules, _ := pl.Rules() // the config loaded, so it parses
	ttl, _ := pl.TTL()
	maxAge, _ := pl.MaxAge()
	out := []report.Decision{{Name: decPool, Value: fmt.Sprintf("%s (%s): own %s; shared %s", name, res.Agent.ID(),
		dashIfEmpty(strings.Join(pl.Own, ", ")), dashIfEmpty(strings.Join(pl.Shared, ", "))), Source: key}}
	switch {
	case p.Live != nil && p.Live.PoolForce != "":
		out = append(out, report.Decision{Name: decPool, Value: "pick: " + p.Live.PoolForce + " for every key of this run (forced)", Source: "--pool-force"})
	case pl.Force != nil && *pl.Force != "":
		out = append(out, report.Decision{Name: decPool, Value: "pick: " + *pl.Force + " for every key (forced)", Source: key + ".force"})
	case pl.Picker != nil:
		out = append(out, report.Decision{Name: decPool, Value: "pick: the picker command " + firstWord(pl.Picker.Command) + " at each key request; the built-in rules if it fails", Source: key + ".picker"})
	default:
		out = append(out, report.Decision{Name: decPool, Value: "pick: the built-in rules at each key request", Source: "default"})
	}
	out = append(out,
		report.Decision{Name: decPool, Value: "key ttl " + ttl.String(), Source: src(pl.KeyTTL != "", "key_ttl")},
		report.Decision{Name: decPool, Value: "usage max age " + maxAge.String(), Source: src(pl.UsageMaxAge != "", "usage_max_age")},
		report.Decision{Name: decPool, Value: "shared borrow window " + pool.Hours(rules.BorrowWindow), Source: src(pl.SharedBorrowWindow != "", "shared_borrow_window")},
		report.Decision{Name: decPool, Value: fmt.Sprintf("5h cutoff %g%%", rules.ShortUnavailableAt), Source: src(pl.ShortUnavailableAtPercent != nil, "short_unavailable_at_percent")},
		report.Decision{Name: decPool, Value: fmt.Sprintf("default caps 5h %g%%, week %g%%", rules.Defaults.Short, rules.Defaults.Week), Source: src(pl.Defaults.Short != nil || pl.Defaults.Week != nil, "defaults")},
	)
	if len(pl.Accounts) > 0 {
		out = append(out, report.Decision{Name: decPool, Value: fmt.Sprintf("member entries: %d", len(pl.Accounts)), Source: key + ".accounts"})
	}
	if pl.Log != "" {
		out = append(out, report.Decision{Name: decPool, Value: "log " + pl.Log, Source: key + ".log"})
	}
	if !o.DryRun {
		return out
	}
	force := ""
	if p.Live != nil {
		force = p.Live.PoolForce
	}
	rep, err := a.poolPick(ctx, name, lg, PoolPickOptions{Cached: true, Force: force})
	if err != nil || rep.Selected == nil {
		return append(out, report.Decision{Name: decPool, Value: "pick now: none from the cache; picks when " + string(res.Agent.ID()) + " first asks for its key", Source: "login-usage cache"})
	}
	return append(out, report.Decision{Name: decPool, Value: fmt.Sprintf("pick now: %s (%s), from the %s; agx login pick --id %s probes", rep.Selected.Name, rep.Selected.PickedBy, rep.Source, name), Source: "login-usage cache"})
}

// loginLabel names a resolved login in a message.
func loginLabel(l resolve.Login) string {
	switch l.Kind {
	case resolve.LoginHome:
		return "the home login"
	case resolve.LoginTokenRef:
		return "a one-off --token-ref"
	}
	return l.Name + " (" + l.Login.Kind + ")"
}

// dashIfEmpty is "-" for an empty list.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// outranksHelper refuses a launch input that sets a variable the live
// plan must scrub: the agent would read it before ever running the helper
// (spec 10 "Refused"). It names the input so the user knows what to drop.
func outranksHelper(o LaunchOptions, res resolve.Result, scrub []string, src string) error {
	outranks := func(name string) bool { return slices.Contains(scrub, name) }
	refuse := func(name, by string) error {
		return apperr.Conflict(fmt.Sprintf("%s sets %s, which outranks the live-login helper; drop it or %s", by, name, src))
	}
	for k := range res.Profile.Env {
		if outranks(k) {
			return refuse(k, "profile "+res.ProfileName+" env")
		}
	}
	for k := range res.Profile.EnvRefs {
		if outranks(k) {
			return refuse(k, "profile "+res.ProfileName+" env_refs")
		}
	}
	for _, kv := range o.Env {
		if k, _, _ := strings.Cut(kv, "="); outranks(k) {
			return refuse(k, "--env")
		}
	}
	for _, kv := range o.Settings {
		k, _, _ := strings.Cut(kv, "=")
		if name, ok := strings.CutPrefix(k, envSettingPrefix); ok && outranks(name) {
			return refuse(name, "--setting "+k)
		}
	}
	return nil
}

// liveHelper is the shell command the agent runs for its credential:
// the running agx binary plus "credential", and --config when this run
// used one (the helper inherits AGX_CONFIG_DIR / AGX_HOME from the
// launch env, but not a flag).
func (a *App) liveHelper() string {
	self := a.d.Self
	if self == "" {
		self = selfFallback
	}
	cmd := shellQuote(self) + " " + credentialVerb
	if a.d.ConfigFlag != "" {
		cmd += " --config " + shellQuote(a.paths.File)
	}
	return cmd
}

// shellQuote quotes s for a POSIX shell when it holds anything beyond a
// plain path: the helper is a settings string the agent runs through its
// shell, and an install path with a space must stay one word.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsFunc(s, unsafeShellRune) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellSafePunct are the punctuation runes a path may hold unquoted.
const shellSafePunct = "/._-+:=@"

// unsafeShellRune reports a rune that needs quoting in a POSIX shell.
func unsafeShellRune(r rune) bool {
	alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
	return !alnum && !strings.ContainsRune(shellSafePunct, r)
}

// writeLiveStart writes live.jsonc of a new live-login run (Commit, after
// run.jsonc): the start login and the helper, no chosen login yet.
func (a *App) writeLiveStart(ctx context.Context, p *LaunchPlan) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	st := live.State{RunID: p.RunID, Agent: string(p.Agent), StartLogin: p.Live.StartLogin, Helper: p.Live.Helper, PoolForce: p.Live.PoolForce}
	if err := live.Create(p.RunDir, st); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "write live.jsonc: "+err.Error())
	}
	return nil
}
