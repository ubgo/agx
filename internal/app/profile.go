package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// Display values shared by the profile and project commands.
const (
	// HomeLoginText names the home's own stored login. It is a login
	// field's value in results (project use, profile show), so text output
	// compares against it to print "home login" rather than "login home
	// login".
	HomeLoginText = "home login"
	// notModelledText is a plan column for an agent without a plan reader.
	notModelledText = "not modelled"
	// setText stands for a literal env value, which is never printed.
	setText = "(set)"
	// sourceDefault is the source of a value nothing sets.
	sourceDefault = "default"
)

// Warning codes of the profile commands.
const (
	// WarnPlan is a --plan read that failed for one profile.
	WarnPlan = "plan"
	// WarnDiscovery is a discovery source that could not be read.
	WarnDiscovery = "discovery"
)

// ProfileListOptions are agx profile list's flags (without --discovered,
// which is Discover).
type ProfileListOptions struct {
	// Agent keeps only this agent's profiles ("" = all).
	Agent agentio.ID
	// Profiles keeps only these names (repeat --profile; empty = all).
	Profiles []string
	// Plan adds each home's plan (cost N).
	Plan bool
}

// ProfileRow is one row of agx profile list.
type ProfileRow struct {
	Name    string     `json:"name"`
	Agent   agentio.ID `json:"agent"`
	Home    string     `json:"home"`
	Default bool       `json:"default"`
	// Billing is plan or api (plan when unset).
	Billing string `json:"billing"`
	// Login is the login a plain run of the profile uses: its own login,
	// else its agent's defaults login, else "home login".
	Login string `json:"login"`
	// HomeExists is false when the home folder is gone (edge 6).
	HomeExists bool `json:"home_exists"`
	// Plan is the home's plan label with --plan ("" without it).
	Plan string `json:"plan,omitempty"`
	// Account is the email of the home's login on disk, and PlanLabel the
	// plan that login names: read locally, shown as agx v0.3's profiles
	// list did; "" when the home holds none or the agent models none.
	Account   string `json:"account,omitempty"`
	PlanLabel string `json:"plan_label,omitempty"`
	// State is the home login's state: ok, expired, none, or
	// not-modelled when the agent has no identity reader (v0.3 STATE).
	State LoginState `json:"state"`
	// ExpiresAt is when the home login's access credential expires; nil
	// when unknown.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// LoginState is a home login's local state in profile list. Closed set.
type LoginState string

// Login states.
const (
	// LoginStateOK: logged in and not past a known expiry.
	LoginStateOK LoginState = "ok"
	// LoginStateExpired: the stored credential is past its expiry.
	LoginStateExpired LoginState = "expired"
	// LoginStateNone: no login in this home (or the folder is missing).
	LoginStateNone LoginState = "none"
	// LoginStateUnknown: the agent models no identity, or reading it
	// failed; nothing is claimed.
	LoginStateUnknown LoginState = "unknown"
)

// ProfileList is agx profile list.
func (a *App) ProfileList(ctx context.Context, o ProfileListOptions) ([]ProfileRow, []report.Warning, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return nil, nil, err
	}
	if o.Agent != "" {
		if _, ok := reg.Get(o.Agent); !ok {
			return nil, nil, unknownAgent(reg, string(o.Agent))
		}
	}
	for _, n := range o.Profiles {
		if _, ok := cfg.Config.Profiles[n]; !ok {
			return nil, nil, unknownProfile(n)
		}
	}
	rows := []ProfileRow{}
	var warns []report.Warning
	// The file's order, as v0.3's list was: a person sees their own
	// arrangement, not the alphabet.
	for _, name := range cfg.ProfileOrder() {
		p := cfg.Config.Profiles[name]
		if (o.Agent != "" && p.Agent != o.Agent) || (len(o.Profiles) > 0 && !slices.Contains(o.Profiles, name)) {
			continue
		}
		dir := a.expand(p.Home)
		row := ProfileRow{Name: name, Agent: p.Agent, Home: a.tilde(dir), Default: p.Default, Billing: billingOf(p), Login: listLogin(cfg.Config, name, p), HomeExists: statDir(dir)}
		a.fillHomeLogin(reg, &row, p, dir, name)
		if o.Plan {
			if err := ctxErr(ctx); err != nil {
				return nil, nil, err
			}
			plan, err := a.planOf(ctx, p, dir, name)
			if err != nil {
				warns = append(warns, report.Warning{Code: WarnPlan, Message: fmt.Sprintf("profile %s: plan: %s", name, apperr.Text(err))})
				plan = "error"
			}
			row.Plan = plan
		}
		rows = append(rows, row)
	}
	return rows, warns, nil
}

// fillHomeLogin reads the home's login on disk (agentio's identity
// reader, local only) into the row's account, plan label and state.
func (a *App) fillHomeLogin(reg *agents.Registry, row *ProfileRow, p config.Profile, dir, name string) {
	row.State = LoginStateUnknown
	if !row.HomeExists {
		row.State = LoginStateNone
		return
	}
	ag, ok := reg.Get(p.Agent)
	if !ok {
		return
	}
	ir, ok := agentio.IdentityOf(ag)
	if !ok {
		return
	}
	id, err := ir.Identity(resolve.HomeOf(ag, dir, name))
	if err != nil {
		return
	}
	// Reveal is deliberate: v0.3's list showed whose account each home is.
	row.Account, row.PlanLabel, row.ExpiresAt = id.Email.Reveal(), id.PlanLabel, id.ExpiresAt
	switch {
	case !id.LoggedIn:
		row.State = LoginStateNone
	case id.Expired(a.Now()):
		row.State = LoginStateExpired
	default:
		row.State = LoginStateOK
	}
}

// planOf reads a profile home's plan label (network, cost N).
func (a *App) planOf(ctx context.Context, p config.Profile, dir, name string) (string, error) {
	reg, err := a.Registry()
	if err != nil {
		return "", err
	}
	ag, ok := reg.Get(p.Agent)
	if !ok {
		return "", unknownAgent(reg, string(p.Agent))
	}
	sr, ok := agentio.SubscriptionOf(ag)
	if !ok {
		return notModelledText, nil
	}
	sub, err := sr.Subscription(ctx, resolve.HomeOf(ag, dir, name))
	if err != nil {
		return "", err
	}
	return sub.Plan, nil
}

// billingOf is a profile's billing with the default filled in.
func billingOf(p config.Profile) string {
	if p.Billing == "" {
		return config.BillingPlan
	}
	return p.Billing
}

// listLogin is the login a plain run of the profile uses (the launch's
// own chain, resolve.ProfileLogin): an agent default login shows, not
// "home login".
func listLogin(c config.Config, name string, p config.Profile) string {
	pl := resolve.ProfileLogin(c, p.Agent, name, p)
	if pl.Home {
		return HomeLoginText
	}
	return pl.Name
}

// profileLogin is a profile's own login field (profile edit's old value).
func profileLogin(p config.Profile) string {
	if p.Login != "" {
		return p.Login
	}
	return HomeLoginText
}

// unknownProfile is the not-found error for a profile name.
func unknownProfile(name string) error {
	return apperr.Failure(apperr.CodeNotFound, "unknown profile "+name).WithHint("agx profile list")
}

// ValueSource is one value of agx profile show or config show with the
// place it came from (principle 2).
type ValueSource struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// ProfileShowResult is agx profile show.
type ProfileShowResult struct {
	Name   string        `json:"name"`
	Agent  agentio.ID    `json:"agent"`
	Values []ValueSource `json:"values"`
}

// Profile value keys shown by profile show.
const (
	pkAgent      = "agent"
	pkHome       = "home"
	pkHomeExists = "home_exists"
	pkDefault    = "default"
	pkBilling    = "billing"
	pkLogin      = "login"
	pkArgs       = "args"
	pkModel      = "model"
	pkAutonomy   = "autonomy"
	pkEnv        = "env"
	pkEnvRefs    = "env_refs"
	pkEnvFile    = "env_file"
	pkBeforeRun  = "before_run"
	pkDelivery   = "settings_delivery"
	pkLiveLogin  = "live_login"
	pkSharedWith = "shares_home_with"
	pkBoundIn    = "bound_in"
	pkInherit    = "inherit_defaults"
	pkHomeLogin  = "home_login"
)

// ProfileShow is agx profile show --id: every value of one profile with
// the place it comes from (the profile entry, the agent defaults, or the
// built-in default).
func (a *App) ProfileShow(id string) (ProfileShowResult, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return ProfileShowResult{}, err
	}
	if id == "" {
		return ProfileShowResult{}, apperr.Usage("--id is required (a profile name)")
	}
	p, ok := cfg.Config.Profiles[id]
	if !ok {
		return ProfileShowResult{}, unknownProfile(id)
	}
	res := ProfileShowResult{Name: id, Agent: p.Agent}
	from := func(keys ...string) string {
		return "profile " + id + " (" + cfg.Where(append([]string{"profiles", id}, keys...)...) + ")"
	}
	agentDefaults := func(keys ...string) string {
		return "agents." + string(p.Agent) + ".defaults (" + cfg.Where(append([]string{"agents", string(p.Agent), "defaults"}, keys...)...) + ")"
	}
	add := func(k, v, src string) { res.Values = append(res.Values, ValueSource{Key: k, Value: v, Source: src}) }
	dir := a.expand(p.Home)
	add(pkAgent, string(p.Agent), from(pkAgent))
	add(pkHome, a.tilde(dir), from(pkHome))
	add(pkHomeExists, yesNo(statDir(dir)), "disk")
	if p.Default {
		add(pkDefault, "true", from(pkDefault))
	} else {
		add(pkDefault, "false", sourceDefault)
	}
	if p.Billing != "" {
		add(pkBilling, p.Billing, from(pkBilling))
	} else {
		add(pkBilling, config.BillingPlan, sourceDefault)
	}
	ac := cfg.Config.Agents[p.Agent]
	switch {
	case p.HomeLogin:
		add(pkLogin, HomeLoginText, from(pkHomeLogin))
	case p.Login != "":
		add(pkLogin, p.Login, from(pkLogin))
	case ac.Defaults.HomeLogin:
		add(pkLogin, HomeLoginText, agentDefaults(pkHomeLogin))
	case ac.Defaults.Login != "":
		add(pkLogin, ac.Defaults.Login, agentDefaults(pkLogin))
	default:
		add(pkLogin, HomeLoginText, sourceDefault+" (no login named)")
	}
	if p.InheritsDefaults() {
		if len(ac.Defaults.Args) > 0 {
			add(pkArgs, strings.Join(ac.Defaults.Args, " "), agentDefaults(pkArgs))
		}
	} else {
		add(pkInherit, "false", from(pkInherit))
	}
	if len(p.Args) > 0 {
		add(pkArgs, strings.Join(p.Args, " "), from(pkArgs))
	}
	for _, kv := range []struct{ k, v string }{
		{pkModel, p.Model}, {pkAutonomy, p.Autonomy}, {pkEnvFile, p.EnvFile}, {pkBeforeRun, p.BeforeRun}, {pkDelivery, p.SettingsDelivery},
	} {
		if kv.v != "" {
			add(kv.k, kv.v, from(kv.k))
		}
	}
	if p.LiveLogin {
		add(pkLiveLogin, "true", from(pkLiveLogin))
	}
	for _, k := range sortedNames(p.Env) {
		// Literal env values may be secrets (principle 12): names only.
		add(pkEnv, k+"="+setText, from(pkEnv, k))
	}
	for _, k := range sortedNames(p.EnvRefs) {
		add(pkEnvRefs, k+"="+p.EnvRefs[k], from(pkEnvRefs, k))
	}
	var shared []string
	for _, n := range sortedNames(cfg.Config.Profiles) {
		q := cfg.Config.Profiles[n]
		if n != id && q.Agent == p.Agent && filepath.Clean(a.expand(q.Home)) == filepath.Clean(dir) {
			shared = append(shared, n)
		}
	}
	if len(shared) > 0 {
		add(pkSharedWith, strings.Join(shared, ", "), "profiles with the same home")
	}
	for _, d := range sortedNames(cfg.Config.Projects) {
		if b, ok := cfg.Config.Projects[d][p.Agent]; ok && b.Profile == id {
			add(pkBoundIn, a.tilde(d), "projects ("+cfg.Where("projects", d, string(p.Agent), "profile")+")")
		}
	}
	return res, nil
}

// yesNo renders a bool as yes or no.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ProfileUseResult is agx profile use.
type ProfileUseResult struct {
	Name  string     `json:"name"`
	Agent agentio.ID `json:"agent"`
	// Cleared are the profiles of the same agent that were the default.
	Cleared []string `json:"cleared"`
}

// ProfileUse is agx profile use --id: make it its agent's default and
// clear "default" on the agent's other profiles (one default per agent,
// edge 5).
func (a *App) ProfileUse(ctx context.Context, id string) (ProfileUseResult, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return ProfileUseResult{}, err
	}
	if id == "" {
		return ProfileUseResult{}, apperr.Usage("--id is required (a profile name)")
	}
	p, ok := cfg.Config.Profiles[id]
	if !ok {
		return ProfileUseResult{}, unknownProfile(id)
	}
	res := ProfileUseResult{Name: id, Agent: p.Agent, Cleared: []string{}}
	for _, n := range resolve.ProfilesOf(cfg.Config, p.Agent) {
		if n != id && cfg.Config.Profiles[n].Default {
			res.Cleared = append(res.Cleared, n)
		}
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		for _, n := range res.Cleared {
			if err := d.Delete(config.Pointer("profiles", n, pkDefault)); err != nil {
				return err
			}
		}
		return d.Set(config.Pointer("profiles", id, pkDefault), true)
	})
	return res, err
}

// ProfileRemoveResult is agx profile remove.
type ProfileRemoveResult struct {
	Name string `json:"name"`
	// Home is the home folder, which is never deleted.
	Home string `json:"home"`
	// Bindings are the folders whose binding named the profile; the
	// profile is removed from each (the binding goes when nothing is left).
	Bindings []string `json:"bindings"`
	// Removed is false for the plan (no --yes).
	Removed bool `json:"removed"`
}

// ProfileRemove is agx profile remove --id [--yes]: without --yes it
// returns the plan and writes nothing. The home folder is never deleted
// (principle 11). Bindings that name the profile lose it, so the config
// still loads.
func (a *App) ProfileRemove(ctx context.Context, id string, yes bool) (ProfileRemoveResult, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return ProfileRemoveResult{}, err
	}
	if id == "" {
		return ProfileRemoveResult{}, apperr.Usage("--id is required (a profile name)")
	}
	p, ok := cfg.Config.Profiles[id]
	if !ok {
		return ProfileRemoveResult{}, unknownProfile(id)
	}
	res := ProfileRemoveResult{Name: id, Home: a.tilde(a.expand(p.Home)), Bindings: []string{}}
	var dirs []string
	for _, d := range sortedNames(cfg.Config.Projects) {
		if b, ok := cfg.Config.Projects[d][p.Agent]; ok && b.Profile == id {
			dirs = append(dirs, d)
			res.Bindings = append(res.Bindings, a.tilde(d))
		}
	}
	if !yes {
		return res, nil
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		for _, dir := range dirs {
			if err := a.dropBindingField(d, cfg.Config, dir, p.Agent, pkProfile); err != nil {
				return err
			}
		}
		return d.Delete(config.Pointer("profiles", id))
	})
	res.Removed = err == nil
	return res, err
}

// pkProfile is the binding key naming a profile.
const pkProfile = "profile"

// dropBindingField removes one field of the binding dir/agent, and the
// binding (and the folder entry) when nothing is left in it.
func (a *App) dropBindingField(d *jsonc.Doc, c config.Config, dir string, id agentio.ID, field string) error {
	b := c.Projects[dir][id]
	left := 0
	for _, set := range []bool{b.Profile != "" && field != pkProfile, b.Login != "" && field != pkLogin, b.HomeLogin && field != pkHomeLogin} {
		if set {
			left++
		}
	}
	if left > 0 {
		return d.Delete(config.Pointer("projects", dir, string(id), field))
	}
	if len(c.Projects[dir]) == 1 {
		return d.Delete(config.Pointer("projects", dir))
	}
	return d.Delete(config.Pointer("projects", dir, string(id)))
}
