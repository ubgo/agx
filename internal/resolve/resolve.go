// Package resolve answers the questions of spec section 6 for one launch:
// which agent, which profile, which home, which login. Each answer comes
// from the first source that has one, by a fixed precedence, and the
// source is recorded as a Decision so --dry-run, project show and --json
// can show it (principle 2).
//
// It is pure policy over the loaded config and an injected environment:
// no process runs, no secret resolves, no usage is read here. The two
// choices that need more (an agent picker at a terminal, an --auto-profile
// pick over usage, an --auto-login pick over probes) are callbacks the app
// layer supplies, so this package stays testable with plain values.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
)

// Environment variables that pick a login per terminal (spec 8.3).
const (
	// EnvLogin is the login for every agent in this terminal.
	EnvLogin = "AGX_LOGIN"
	// envLoginPrefix + upper-cased agent id beats EnvLogin for that agent.
	envLoginPrefix = "AGX_LOGIN_"
)

// LoginEnvFor is the per-agent variable for id: AGX_LOGIN_CLAUDE; every
// byte that is not a letter or digit becomes "_".
func LoginEnvFor(id agentio.ID) string {
	var b strings.Builder
	b.WriteString(envLoginPrefix)
	for _, r := range strings.ToUpper(string(id)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Decision names (report.Decision.Name), one per concern, shared by every
// command that shows decisions.
const (
	DecAgent   = "agent"
	DecProfile = "profile"
	DecHome    = "home"
	DecLogin   = "login"
	DecBinary  = "binary"
	DecFolder  = "folder"
	DecArgs    = "args"
	DecOptions = "options"
	DecRun     = "run"
	DecCommand = "command"
	DecEnv     = "env"
	DecSetting = "settings"
	DecNote    = "note"
	DecBefore  = "before_run"
)

// LoginKind says what a launch authenticates with.
type LoginKind uint8

// Login kinds.
const (
	// LoginHome is the home's own stored login (nothing is added).
	LoginHome LoginKind = iota
	// LoginNamed is a logins.<name> entry.
	LoginNamed
	// LoginTokenRef is a one-off --token-ref.
	LoginTokenRef
)

// Login is the resolved login choice; no secret is resolved yet.
type Login struct {
	Kind LoginKind
	// Name is the login name for LoginNamed.
	Name string
	// Login is the config entry for LoginNamed.
	Login config.Login
	// Ref is the --token-ref value (a reference or a plain value).
	Ref string
	// RefKind is the --token-kind for LoginTokenRef.
	RefKind string
	// Source is where the choice came from.
	Source string
}

// Inputs are the launch flags that drive resolution.
type Inputs struct {
	Profile     string
	AutoProfile bool
	Agent       agentio.ID
	Login       string
	HomeLogin   bool
	AutoLogin   bool
	TokenRef    string
	TokenKind   string
	// Dir is the launch folder (absolute); bindings are looked up from it.
	Dir string
	// AgentFrom, ProfileFrom and HomeFrom name who chose the agent, the
	// profile and the home when it was not the user (resume: the
	// conversation decides agent and home, the shared-home rule the
	// profile; spec 6.5). "" = the usual sources.
	AgentFrom   string
	ProfileFrom string
	HomeFrom    string
}

// Choosers are the decisions that need a terminal or the network.
type Choosers struct {
	// Agent asks at a terminal which agent to use; nil = no terminal.
	// remember is true when the user ticked "remember as default_agent".
	Agent func(candidates []AgentChoice) (id agentio.ID, remember bool, err error)
	// AutoProfile picks one of the named profiles of an agent by usage
	// headroom; reason is shown as the decision source.
	AutoProfile func(agent agentio.Agent, profiles []string) (name, reason string, err error)
	// AutoLogin picks one of the named logins by probing; reason as above.
	AutoLogin func(agent agentio.Agent, logins []string) (name, reason string, err error)
}

// AgentChoice is one row of the agent picker.
type AgentChoice struct {
	ID             agentio.ID
	DefaultProfile string
}

// Result is a resolved launch target.
type Result struct {
	Agent       agentio.Agent
	ProfileName string
	Profile     config.Profile
	Home        agentio.Home
	Login       Login
	// Binding is the folder binding used (nearest bound folder), if any.
	BindingDir string
	Binding    config.Binding
	// RememberAgent is set when the picker's "remember" box was ticked.
	RememberAgent bool
	Decisions     []report.Decision
	// Notes are stderr notes for the non-flag sources that decided
	// (binding, shared home, AGX_LOGIN*, default_agent; principle 2).
	Notes []string
}

// Resolver holds what resolution reads.
type Resolver struct {
	Cfg      *config.Loaded
	Agents   *agents.Registry
	Lookup   config.Lookup
	UserHome string
	Choose   Choosers
	// FoldOS is the OS whose folder-case rule binding lookups apply; ""
	// is runtime.GOOS. Tests set "darwin" to check the macOS rule on any
	// machine.
	FoldOS string
}

// foldOS is the OS whose folder-case rule applies (FoldOS or the host's).
func (rs *Resolver) foldOS() string {
	if rs.FoldOS != "" {
		return rs.FoldOS
	}
	return runtime.GOOS
}

// decide appends a decision.
func (r *Result) decide(name, value, source string) {
	r.Decisions = append(r.Decisions, report.Decision{Name: name, Value: value, Source: source})
}

// tilde renders a path for display.
func (rs *Resolver) tilde(p string) string { return config.TildePath(p, rs.UserHome) }

// Resolve runs steps 1 to 4 of the chain (6.1).
func (rs *Resolver) Resolve(in Inputs) (Result, error) {
	var res Result
	cfg := rs.Cfg.Config
	if len(cfg.Profiles) == 0 {
		return res, apperr.Failure(apperr.CodeNoProfiles, "no profiles configured").
			WithHint("agx config init\nagx profile import --discovered --yes")
	}
	if in.Profile != "" && in.AutoProfile {
		return res, apperr.Conflict("--profile and --auto-profile both choose the profile; pick one")
	}
	if err := rs.agent(in, &res); err != nil {
		return res, err
	}
	if err := rs.profile(in, &res); err != nil {
		return res, err
	}
	if err := rs.home(in, &res); err != nil {
		return res, err
	}
	if err := rs.login(in, &res); err != nil {
		return res, err
	}
	return res, nil
}

// agent is step 1: --profile's agent → --agent → the folder binding when
// it binds one agent → default_agent → ask.
func (rs *Resolver) agent(in Inputs, res *Result) error {
	cfg := rs.Cfg.Config
	set := func(id agentio.ID, source string) error {
		a, ok := rs.Agents.Get(id)
		if !ok {
			return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("unknown agent %s", id)).WithHint("agx agent list")
		}
		res.Agent = a
		res.decide(DecAgent, string(id), source)
		return nil
	}
	if in.Profile != "" {
		p, ok := cfg.Profiles[in.Profile]
		if !ok {
			return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("unknown profile %s", in.Profile)).WithHint("agx profile list")
		}
		if in.Agent != "" && in.Agent != p.Agent {
			return apperr.Conflict(fmt.Sprintf("--agent %s and --profile %s (a %s profile) disagree; pick one", in.Agent, in.Profile, p.Agent))
		}
		src := "profile " + in.Profile
		if in.AgentFrom != "" {
			src = in.AgentFrom
		}
		return set(p.Agent, src)
	}
	if in.Agent != "" {
		return set(in.Agent, "--agent")
	}
	if dir, binds, ok := rs.nearestBinding(in.Dir, ""); ok && len(binds) == 1 {
		for id := range binds {
			res.Notes = append(res.Notes, fmt.Sprintf("agent %s (folder binding %s)", id, rs.tilde(dir)))
			return set(id, "folder binding "+rs.tilde(dir))
		}
	}
	if cfg.DefaultAgent != "" {
		res.Notes = append(res.Notes, fmt.Sprintf("agent %s (default_agent)", cfg.DefaultAgent))
		return set(cfg.DefaultAgent, "default_agent")
	}
	choices := rs.agentChoices()
	if rs.Choose.Agent != nil {
		id, remember, err := rs.Choose.Agent(choices)
		if err != nil {
			return err
		}
		res.RememberAgent = remember
		return set(id, "picker")
	}
	var hint []string
	for _, c := range choices {
		line := "agx run --agent " + string(c.ID)
		if c.DefaultProfile != "" {
			line += "      (default profile: " + c.DefaultProfile + ")"
		}
		hint = append(hint, line)
	}
	if len(choices) > 0 {
		hint = append(hint, "agx config set --key default_agent --value "+string(choices[0].ID))
	}
	return apperr.New(apperr.CodeNoAgent, apperr.ClassUsage, `no profile, binding or agent given, and "default_agent" is not set`).
		WithHint(strings.Join(hint, "\n"))
}

// agentChoices lists the agents that have at least one profile, in
// registry order, with their default profile.
func (rs *Resolver) agentChoices() []AgentChoice {
	var out []AgentChoice
	for _, id := range rs.Agents.IDs() {
		names := ProfilesOf(rs.Cfg.Config, id)
		if len(names) == 0 {
			continue
		}
		out = append(out, AgentChoice{ID: id, DefaultProfile: DefaultProfile(rs.Cfg.Config, id)})
	}
	return out
}

// ProfilesOf lists the profile names of agent id, sorted.
func ProfilesOf(c config.Config, id agentio.ID) []string {
	var out []string
	for name, p := range c.Profiles {
		if p.Agent == id {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// DefaultProfile is the profile marked default for agent id, or "".
func DefaultProfile(c config.Config, id agentio.ID) string {
	for _, name := range ProfilesOf(c, id) {
		if c.Profiles[name].Default {
			return name
		}
	}
	return ""
}

// profile is step 2: --profile → --auto-profile → binding → default.
func (rs *Resolver) profile(in Inputs, res *Result) error {
	cfg := rs.Cfg.Config
	id := res.Agent.ID()
	set := func(name, source string) {
		res.ProfileName = name
		res.Profile = cfg.Profiles[name]
		res.decide(DecProfile, name, source)
	}
	if in.Profile != "" {
		src := "--profile"
		if in.ProfileFrom != "" {
			src = in.ProfileFrom
			// "from", not parentheses: ProfileFrom may carry its own
			// ("home ~/.claude (plan profile)"), and nested ones misread.
			res.Notes = append(res.Notes, fmt.Sprintf("profile %s from %s", in.Profile, in.ProfileFrom))
		}
		set(in.Profile, src)
		return nil
	}
	if in.AutoProfile {
		names := ProfilesOf(cfg, id)
		if len(names) == 0 {
			return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("auto profile: %s has no profiles", id))
		}
		if rs.Choose.AutoProfile == nil {
			return apperr.Failure(apperr.CodeAutoPick, "auto profile: not available here")
		}
		name, reason, err := rs.Choose.AutoProfile(res.Agent, names)
		if err != nil {
			return err
		}
		set(name, "--auto-profile ("+reason+")")
		return nil
	}
	if dir, binds, ok := rs.nearestBindingWhere(in.Dir, id, func(b config.Binding) bool { return b.Profile != "" }); ok {
		set(binds[id].Profile, "folder binding "+rs.tilde(dir))
		res.Notes = append(res.Notes, fmt.Sprintf("profile %s (folder binding %s)", binds[id].Profile, rs.tilde(dir)))
		return nil
	}
	if name := DefaultProfile(cfg, id); name != "" {
		set(name, "default profile of "+string(id))
		return nil
	}
	names := ProfilesOf(cfg, id)
	hint := "agx profile add --name NAME --agent " + string(id) + " --home DIR"
	if len(names) > 0 {
		hint = "agx run --profile " + names[0] + "\nagx profile use --id " + names[0]
	}
	return apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage, fmt.Sprintf("no profile chosen for %s and none is its default", id)).WithHint(hint)
}

// home is step 3: the profile's home must exist.
func (rs *Resolver) home(in Inputs, res *Result) error {
	dir := config.ExpandPath(res.Profile.Home, rs.Lookup, rs.UserHome)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return apperr.Failure(apperr.CodeHomeMissing, fmt.Sprintf("profile %s: home %s does not exist", res.ProfileName, rs.tilde(dir))).
			WithHint("mkdir -p " + dir + "   (then log in), or agx profile edit --id " + res.ProfileName + " --home DIR")
	}
	res.Home = HomeOf(res.Agent, dir, res.ProfileName)
	src := "profile " + res.ProfileName
	if in.HomeFrom != "" {
		src = in.HomeFrom
	}
	res.decide(DecHome, rs.tilde(dir), src)
	return nil
}

// HomeOf builds the agentio Home for dir: the default home when dir is the
// agent's default home folder (so the home variable is removed, never set
// to the default path), else a named non-default home.
func HomeOf(a agentio.Agent, dir, name string) agentio.Home {
	def := a.DefaultHome()
	if filepath.Clean(dir) == filepath.Clean(def.Dir()) {
		return def
	}
	return agentio.NewHome(a.ID(), dir, false, name)
}

// login is step 4: --token-ref → --login / --home-login / --auto-login →
// AGX_LOGIN_<AGENT> → AGX_LOGIN → binding → profile → agent default →
// home login. Every stored login must serve the run's agent.
func (rs *Resolver) login(in Inputs, res *Result) error {
	cfg := rs.Cfg.Config
	id := res.Agent.ID()
	choices := 0
	for _, set := range []bool{in.TokenRef != "", in.Login != "", in.HomeLogin, in.AutoLogin} {
		if set {
			choices++
		}
	}
	if choices > 1 {
		return apperr.Conflict(conflictMessage(in))
	}
	named := func(name, source string) error {
		lg, ok := cfg.Logins[name]
		if !ok {
			return apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("unknown login %s (from %s)", name, source)).WithHint("agx login list")
		}
		if !lg.Serves(id) {
			return apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage, fmt.Sprintf("login %s serves %s; this run is %s", name, joinIDs(lg.Agents), id))
		}
		res.Login = Login{Kind: LoginNamed, Name: name, Login: lg, Source: source}
		res.decide(DecLogin, name, source)
		return nil
	}
	home := func(source string) error {
		res.Login = Login{Kind: LoginHome, Source: source}
		res.decide(DecLogin, "home login", source)
		return nil
	}
	switch {
	case in.TokenRef != "":
		res.Login = Login{Kind: LoginTokenRef, Ref: in.TokenRef, RefKind: in.TokenKind, Source: "--token-ref"}
		res.decide(DecLogin, "one-off token", "--token-ref")
		return nil
	case in.Login != "":
		return named(in.Login, "--login")
	case in.HomeLogin:
		return home("--home-login")
	case in.AutoLogin:
		names := LoginsServing(cfg, id)
		if len(names) == 0 {
			return apperr.Failure(apperr.CodeAutoPick, fmt.Sprintf("auto login: no stored login serves %s", id))
		}
		if rs.Choose.AutoLogin == nil {
			return apperr.Failure(apperr.CodeAutoPick, "auto login: not available here")
		}
		name, reason, err := rs.Choose.AutoLogin(res.Agent, names)
		if err != nil {
			return err
		}
		return named(name, "--auto-login ("+reason+")")
	}
	for _, env := range []string{LoginEnvFor(id), EnvLogin} {
		if v, ok := rs.Lookup(env); ok && v != "" {
			res.Notes = append(res.Notes, fmt.Sprintf("login %s (%s)", v, env))
			return named(v, env)
		}
	}
	if dir, binds, ok := rs.nearestBindingWhere(in.Dir, id, func(b config.Binding) bool { return b.Login != "" || b.HomeLogin }); ok {
		b := binds[id]
		res.BindingDir, res.Binding = dir, b
		src := "folder binding " + rs.tilde(dir)
		if b.HomeLogin {
			res.Notes = append(res.Notes, "home login ("+src+")")
			return home(src)
		}
		res.Notes = append(res.Notes, fmt.Sprintf("login %s (%s)", b.Login, src))
		return named(b.Login, src)
	}
	pl := ProfileLogin(cfg, id, res.ProfileName, res.Profile)
	if pl.Home {
		return home(pl.Source)
	}
	return named(pl.Name, pl.Source)
}

// ProfileLoginChoice is the login a profile runs on by itself.
type ProfileLoginChoice struct {
	// Name is the named login; "" when Home.
	Name string
	// Home is true for the home's own stored login.
	Home bool
	// Source names where the choice is written ("profile work",
	// "agents.claude.defaults", "default (no login named)").
	Source string
}

// ProfileLogin is the tail of the login chain: the login a profile runs on
// when no flag, login variable or folder binding chooses one. The profile's
// own home_login or login, then its agent's defaults, then the home login.
// The launch and agx profile list both read it, so the LOGIN a listing
// shows is the one a plain run of the profile uses. id is the run's agent
// (a run may have no profile, so it is not read from p). Pure.
func ProfileLogin(cfg config.Config, id agentio.ID, name string, p config.Profile) ProfileLoginChoice {
	switch {
	case p.HomeLogin:
		return ProfileLoginChoice{Home: true, Source: "profile " + name}
	case p.Login != "":
		return ProfileLoginChoice{Name: p.Login, Source: "profile " + name}
	}
	if ac, ok := cfg.Agents[id]; ok {
		src := "agents." + string(id) + ".defaults"
		if ac.Defaults.HomeLogin {
			return ProfileLoginChoice{Home: true, Source: src}
		}
		if ac.Defaults.Login != "" {
			return ProfileLoginChoice{Name: ac.Defaults.Login, Source: src}
		}
	}
	return ProfileLoginChoice{Home: true, Source: "default (no login named)"}
}

// conflictMessage names the two login flags given together (edge 12).
func conflictMessage(in Inputs) string {
	var names []string
	if in.TokenRef != "" {
		names = append(names, "--token-ref")
	}
	if in.Login != "" {
		names = append(names, "--login")
	}
	if in.HomeLogin {
		names = append(names, "--home-login")
	}
	if in.AutoLogin {
		names = append(names, "--auto-login")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " both choose the login; pick one"
}

// LoginsServing lists the token login names that serve id, sorted: the
// --auto-login candidates. A pool is left out: it has no token to probe,
// and it picks among its own members by its own rules.
func LoginsServing(c config.Config, id agentio.ID) []string {
	var out []string
	for name, l := range c.Logins {
		if l.Serves(id) && l.Kind != config.KindPool {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// joinIDs renders ids comma-separated.
func joinIDs(ids []agentio.ID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = string(id)
	}
	return strings.Join(s, ", ")
}

// nearestBinding walks from dir up to the root and returns the first
// folder with a binding: for agent id (id != ""), the nearest folder that
// binds that agent; for id "", the nearest folder with any binding. Both
// sides are normalized (13.6), so a folder reached through a symlink
// matches its real path (edge 30).
func (rs *Resolver) nearestBinding(dir string, id agentio.ID) (string, map[agentio.ID]config.Binding, bool) {
	return rs.nearestBindingWhere(dir, id, nil)
}

// nearestBindingWhere is nearestBinding restricted to bindings of id for
// which want holds (nil = any): each field of a binding is inherited from
// the nearest folder that sets it, so a sub-folder that binds only a login
// keeps the profile its parent binds (edge 29).
func (rs *Resolver) nearestBindingWhere(dir string, id agentio.ID, want func(config.Binding) bool) (string, map[agentio.ID]config.Binding, bool) {
	if dir == "" || len(rs.Cfg.Config.Projects) == 0 {
		return "", nil, false
	}
	norm := map[string]string{}
	for k := range rs.Cfg.Config.Projects {
		norm[config.DirKeyFor(rs.foldOS(), config.NormalizeDir(k, rs.Lookup, rs.UserHome))] = k
	}
	cur := config.NormalizeDir(dir, rs.Lookup, rs.UserHome)
	for {
		if key, ok := norm[config.DirKeyFor(rs.foldOS(), cur)]; ok {
			binds := rs.Cfg.Config.Projects[key]
			b, has := binds[id]
			if id == "" || (has && (want == nil || want(b))) {
				return key, binds, true
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", nil, false
		}
		cur = parent
	}
}

// NearestBinding is nearestBinding for other packages (project show).
func (rs *Resolver) NearestBinding(dir string, id agentio.ID) (string, map[agentio.ID]config.Binding, bool) {
	return rs.nearestBinding(dir, id)
}

// ProfileForHome is the shared-home rule (6.2): among the profiles whose
// home is dir (same agent), the one marked default, else the only plan
// profile, else ask (ask != nil) or refuse listing them.
func (rs *Resolver) ProfileForHome(id agentio.ID, dir string, ask func(names []string) (string, error)) (name, source string, err error) {
	cfg := rs.Cfg.Config
	var names, plan []string
	for _, n := range ProfilesOf(cfg, id) {
		p := cfg.Profiles[n]
		if filepath.Clean(config.ExpandPath(p.Home, rs.Lookup, rs.UserHome)) != filepath.Clean(dir) {
			continue
		}
		names = append(names, n)
		if p.Default {
			return n, "home " + rs.tilde(dir) + " (default)", nil
		}
		if p.Billing != config.BillingAPI {
			plan = append(plan, n)
		}
	}
	switch {
	case len(names) == 0:
		return "", "", apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("%s is not the home of any %s profile", rs.tilde(dir), id)).
			WithHint("agx profile import --id " + dir)
	case len(names) == 1:
		return names[0], "home " + rs.tilde(dir), nil
	case len(plan) == 1:
		return plan[0], "home " + rs.tilde(dir) + " (plan profile)", nil
	case ask != nil:
		n, err := ask(names)
		if err != nil {
			return "", "", err
		}
		return n, "home " + rs.tilde(dir) + " (asked)", nil
	}
	return "", "", apperr.New(apperr.CodeAmbiguous, apperr.ClassUsage,
		fmt.Sprintf("%s has %d profiles (%s) and no default; add --profile", rs.tilde(dir), len(names), strings.Join(names, ", ")))
}
