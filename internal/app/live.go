package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/live"
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
func (a *App) livePlan(o LaunchOptions, res resolve.Result, p *LaunchPlan) (*agentio.LiveCredentialPlan, error) {
	if !o.LiveLogin && !res.Profile.LiveLogin {
		return nil, nil
	}
	src := "--live-login"
	if !o.LiveLogin {
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
	plan, err := lc.LiveCredential(helper, 0)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnsupported, apperr.ClassUnsupported, string(id)+": "+err.Error())
	}
	if err := outranksHelper(o, res, plan.Scrub, src); err != nil {
		return nil, err
	}
	p.Live = &LiveStart{StartLogin: res.Login.Name, Helper: helper}
	p.Decisions = append(p.Decisions, report.Decision{
		Name:   resolve.DecNote,
		Value:  fmt.Sprintf("live login: switch with agx live use --id %s --login NAME (refresh %s)", p.RunID, plan.Refresh),
		Source: src,
	})
	return &plan, nil
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
	st := live.State{RunID: p.RunID, Agent: string(p.Agent), StartLogin: p.Live.StartLogin, Helper: p.Live.Helper}
	if err := live.Create(p.RunDir, st); err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "write live.jsonc: "+err.Error())
	}
	return nil
}
