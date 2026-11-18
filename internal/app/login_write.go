package app

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/login"
)

// Decision names login add and edit report (principle 2: the kind can be
// inferred, so its source is shown).
const decLoginKind = "kind"

// Warning codes of login commands.
const (
	// WarnKindShape is a pasted value whose shape looks like another kind.
	WarnKindShape = "kind-shape"
	// WarnTerminalEnv is an AGX_LOGIN* variable in this terminal that
	// names a removed login or outranks the one just exported.
	WarnTerminalEnv = "terminal-env"
)

// maxStdinToken bounds what --stdin reads: a token is a few hundred
// bytes; a runaway pipe must not fill memory.
const maxStdinToken = 64 << 10

// Source flag names, shared by add and edit (spec 13.5) and named in
// messages.
const (
	flagStdin         = "--stdin"
	flagLoginSetup    = "--login-setup"
	flagTokenRef      = "--token-ref"
	flagHelperCommand = "--helper-command"
)

// SetupRunner runs a vendor token setup command attached to the terminal
// and returns its exit code. The CLI supplies it (internal/run starts
// processes; app never does). path is the resolved binary.
type SetupRunner func(ctx context.Context, path string, c agentio.Command) (int, error)

// LoginSource are the value flags add and edit share: at most one (add
// with none asks at a terminal).
type LoginSource struct {
	// Stdin reads the value from stdin (a pipe: `pbpaste | agx login add …`).
	Stdin bool
	// LoginSetup runs the first agent's token setup command (claude
	// setup-token, cost X), then reads the pasted token at the terminal.
	LoginSetup bool
	// TokenRef is a reference (${gopass:…}, $(…), ${VAR}) or a plain value,
	// written to config as is and resolved at use.
	TokenRef string
	// HelperCommand is an api-key-helper command the agent runs itself.
	HelperCommand string
	// RunSetup runs the setup command for LoginSetup.
	RunSetup SetupRunner
}

// given lists the source flags set.
func (s LoginSource) given() []string {
	var out []string
	if s.Stdin {
		out = append(out, flagStdin)
	}
	if s.LoginSetup {
		out = append(out, flagLoginSetup)
	}
	if s.TokenRef != "" {
		out = append(out, flagTokenRef)
	}
	if s.HelperCommand != "" {
		out = append(out, flagHelperCommand)
	}
	return out
}

// check refuses two sources in one command (spec 5.1 rule 9).
func (s LoginSource) check() error {
	if g := s.given(); len(g) > 1 {
		return apperr.Conflict(strings.Join(g[:len(g)-1], ", ") + " and " + g[len(g)-1] + " all give the login's value; pick one")
	}
	return nil
}

// loginValue is what a source produced: exactly one field is set.
type loginValue struct {
	// token is the config "token" (reference or plain value).
	token string
	// pasted is a value for tokens.jsonc (config "stored": true).
	pasted string
	// command is the api-key-helper command.
	command string
}

// apply sets l's value fields from v, clearing the others.
func (v loginValue) apply(l *config.Login) {
	l.Token, l.Stored, l.Command = v.token, v.pasted != "", v.command
}

// LoginChange is the result of add and edit.
type LoginChange struct {
	// Info is the login as now written.
	Info LoginInfo
	// Decisions are inferred values with their source (the kind).
	Decisions []report.Decision
	// Warnings are non-fatal notes (a pasted value of another shape).
	Warnings []report.Warning
}

// LoginAddOptions are agx login add's flags.
type LoginAddOptions struct {
	// Name is the new login name; required.
	Name string
	// Agents are the agents it serves; at least one.
	Agents []agentio.ID
	// Kind is the token kind; "" = api-key-helper with --helper-command,
	// else the first agent's default kind.
	Kind string
	// Account is a display label.
	Account string
	// Provider is the model provider for provider-keyed agents (pi).
	Provider string
	// Source is the value.
	Source LoginSource
}

// LoginAdd adds a login to config.jsonc (and a pasted value to
// tokens.jsonc, 0600). Cost W, plus X with --login-setup.
func (a *App) LoginAdd(ctx context.Context, o LoginAddOptions) (LoginChange, error) {
	var ch LoginChange
	if o.Name == "" {
		return ch, apperr.Usage("--name is required")
	}
	if !login.ValidName(o.Name) {
		return ch, apperr.Usage(fmt.Sprintf("--name %q: use letters, digits, '.', '_' and '-', starting with a letter or digit", o.Name))
	}
	if len(o.Agents) == 0 {
		return ch, apperr.Usage("--agent is required (repeat it for a login that serves several agents)")
	}
	if err := o.Source.check(); err != nil {
		return ch, err
	}
	cfg, err := a.Config()
	if err != nil {
		return ch, err
	}
	if _, ok := cfg.Config.Logins[o.Name]; ok {
		return ch, apperr.Conflict("login " + o.Name + " exists").WithHint("agx login edit --id " + o.Name)
	}
	agents, err := a.tokenAgents(o.Agents)
	if err != nil {
		return ch, err
	}
	kind, src, err := addKind(o, agents)
	if err != nil {
		return ch, err
	}
	ch.Decisions = append(ch.Decisions, report.Decision{Name: decLoginKind, Value: kind, Source: src})
	l := config.Login{Agents: o.Agents, Kind: kind, Account: o.Account, Provider: o.Provider}
	if err := checkKindAndProvider(l, agents); err != nil {
		return ch, err
	}
	v, err := a.obtainValue(ctx, o.Name, kind, o.Source, agents, true)
	if err != nil {
		return ch, err
	}
	v.apply(&l)
	ch.Warnings = shapeWarnings(v, kind, agents)
	if v.pasted != "" {
		if err := login.WriteStored(ctx, a.paths, o.Name, v.pasted); err != nil {
			return ch, err
		}
	}
	err = config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
		return d.Set(config.Pointer("logins", o.Name), l)
	})
	if err != nil {
		if v.pasted != "" {
			// Undo the value write so tokens.jsonc holds no orphan; the
			// config error is the one worth reporting.
			_ = login.DeleteStored(ctx, a.paths, o.Name)
		}
		return ch, err
	}
	return a.changed(ch, o.Name)
}

// changed reloads the config and fills ch.Info for login name.
func (a *App) changed(ch LoginChange, name string) (LoginChange, error) {
	a.Reload()
	cfg, err := a.Config()
	if err != nil {
		return ch, err
	}
	ch.Info = a.loginInfo(cfg.Config, name, cfg.Config.Logins[name])
	return ch, nil
}

// tokenAgents resolves agent ids and refuses one without a token login
// (Codex: agentio's typed reason, exit 4).
func (a *App) tokenAgents(ids []agentio.ID) ([]agentio.Agent, error) {
	out := make([]agentio.Agent, 0, len(ids))
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			return nil, apperr.Usage(fmt.Sprintf("--agent %s is given twice", id))
		}
		ag, err := a.loginAgent(id)
		if err != nil {
			return nil, err
		}
		if _, ok := agentio.TokenLoginOf(ag); !ok {
			reason, _ := agentio.Unsupported(ag, agentio.CapTokenLogin)
			if reason == "" {
				reason = string(id) + " has no token login"
			}
			return nil, apperr.Unsupported(reason)
		}
		out = append(out, ag)
	}
	return out, nil
}

// addKind picks the kind of a new login and says where it came from.
func addKind(o LoginAddOptions, agents []agentio.Agent) (kind, source string, err error) {
	switch {
	case o.Source.HelperCommand != "" && o.Kind != "" && o.Kind != config.KindAPIKeyHelper:
		return "", "", apperr.Conflict(fmt.Sprintf("--kind %s and --helper-command both choose the kind; a helper command is an %s login", o.Kind, config.KindAPIKeyHelper))
	case o.Kind != "":
		return o.Kind, "--kind", nil
	case o.Source.HelperCommand != "":
		return config.KindAPIKeyHelper, flagHelperCommand, nil
	}
	tl, _ := agentio.TokenLoginOf(agents[0]) // tokenAgents checked it
	kinds := tl.TokenKinds()
	if len(kinds) == 0 {
		return "", "", apperr.Usage("--kind is required: " + string(agents[0].ID()) + " lists no default kind")
	}
	return kinds[0].String(), string(agents[0].ID()) + "'s default kind", nil
}

// checkKindAndProvider refuses a kind an agent does not support and a
// provider mismatch (pi needs one of its providers; an agent without
// providers takes none).
func checkKindAndProvider(l config.Login, agents []agentio.Agent) error {
	k, ok := login.KindOf(l.Kind)
	if !ok {
		return apperr.Usage(fmt.Sprintf("--kind %q: use one of %s", l.Kind, strings.Join(config.KindValues(), ", ")))
	}
	keyed := false
	for _, ag := range agents {
		tl, _ := agentio.TokenLoginOf(ag)
		if !slices.Contains(tl.TokenKinds(), k) {
			return apperr.Unsupported(fmt.Sprintf("%s has no %s login; it takes %s", ag.ID(), l.Kind, kindNames(tl.TokenKinds())))
		}
		pl, isKeyed := tl.(agentio.TokenProviderLister)
		if !isKeyed {
			continue
		}
		keyed = true
		providers := pl.TokenProviders(k)
		if l.Provider != "" && !slices.Contains(providers, l.Provider) {
			// A given provider the agent does not know is named as such:
			// "pass --provider" would read as if none was given.
			return apperr.Usage(fmt.Sprintf("--provider %q: %s has no %s provider by that name; use one of %s", l.Provider, ag.ID(), l.Kind, strings.Join(providers, ", ")))
		}
		if l.Provider == "" {
			return apperr.Usage(fmt.Sprintf("%s reads a different variable per provider; pass --provider: one of %s", ag.ID(), strings.Join(providers, ", ")))
		}
	}
	if l.Provider != "" && !keyed {
		return apperr.Usage("--provider " + l.Provider + ": none of these agents reads a per-provider variable")
	}
	return nil
}

// kindNames renders kinds comma-separated.
func kindNames(ks []agentio.TokenKind) string {
	s := make([]string, len(ks))
	for i, k := range ks {
		s[i] = k.String()
	}
	return strings.Join(s, ", ")
}

// obtainValue reads the value from the one given source, or asks at a
// terminal when none is given and required is true. A pasted value is
// cleaned (trimmed; empty or multi-line refused, edge 17).
func (a *App) obtainValue(ctx context.Context, name, kind string, s LoginSource, agents []agentio.Agent, required bool) (loginValue, error) {
	label := "login " + name
	helper := kind == config.KindAPIKeyHelper
	switch {
	case s.HelperCommand != "":
		if !helper {
			return loginValue{}, apperr.Usage(fmt.Sprintf("%s is for %s logins; this one is %s", flagHelperCommand, config.KindAPIKeyHelper, kind))
		}
		return loginValue{command: s.HelperCommand}, nil
	case helper && (required || len(s.given()) > 0):
		return loginValue{}, apperr.Usage(fmt.Sprintf("an %s login takes its command from %s", config.KindAPIKeyHelper, flagHelperCommand))
	case s.TokenRef != "":
		return loginValue{token: s.TokenRef}, nil
	case s.Stdin:
		return a.readStdin(label)
	case s.LoginSetup:
		return a.loginSetup(ctx, label, s, agents)
	case !required:
		return loginValue{}, nil
	}
	if a.d.Picker == nil {
		// --login-setup is not offered: it needs a terminal too.
		return loginValue{}, noTerminal("the token is pasted at a hidden prompt", "pass "+strings.Join([]string{flagStdin, flagTokenRef, flagHelperCommand}, ", "))
	}
	return a.promptValue(ctx, label, "Paste the token for "+label+" (hidden): ")
}

// readStdin reads and cleans a piped value.
func (a *App) readStdin(label string) (loginValue, error) {
	if a.d.Stdin == nil {
		return loginValue{}, apperr.Usage(flagStdin + ": no stdin")
	}
	b, err := io.ReadAll(io.LimitReader(a.d.Stdin, maxStdinToken))
	if err != nil {
		return loginValue{}, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read stdin: "+err.Error())
	}
	tok, err := login.Clean(label, string(b))
	if err != nil {
		return loginValue{}, err
	}
	return loginValue{pasted: tok}, nil
}

// promptValue reads a hidden value at the terminal and cleans it.
func (a *App) promptValue(ctx context.Context, label, prompt string) (loginValue, error) {
	raw, err := a.d.Picker.Secret(ctx, prompt)
	if err != nil {
		return loginValue{}, err
	}
	tok, err := login.Clean(label, raw)
	if err != nil {
		return loginValue{}, err
	}
	return loginValue{pasted: tok}, nil
}

// noTerminal refuses an interactive step that is not a picker (a hidden
// token prompt, a vendor setup command) when agx has no terminal: what
// says what needed one, instead what to do. picker.NoTTY words its text
// for a picker, which these steps never show; the code (no-tty) and the
// exit class are the same.
func noTerminal(what, instead string) error {
	return apperr.New(apperr.CodeNoTTY, apperr.ClassUsage, "no terminal: "+what+"; "+instead)
}

// loginSetup runs the first agent's token setup command attached to the
// terminal (cost X), then reads the token the user pastes. It needs a
// terminal for both steps.
func (a *App) loginSetup(ctx context.Context, label string, s LoginSource, agents []agentio.Agent) (loginValue, error) {
	if a.d.Picker == nil {
		return loginValue{}, noTerminal(flagLoginSetup+" runs the setup command, then asks you to paste the token it prints", "pipe the token with "+flagStdin)
	}
	if s.RunSetup == nil {
		return loginValue{}, apperr.Unsupported(flagLoginSetup + " is not available in this build")
	}
	ag := agents[0]
	tl, _ := agentio.TokenLoginOf(ag) // tokenAgents checked it
	cmd, ok := tl.TokenSetupCommand(ag.DefaultHome())
	if !ok {
		return loginValue{}, apperr.Unsupported(fmt.Sprintf("%s has no token setup command; give the token with %s or %s", ag.ID(), flagStdin, flagTokenRef))
	}
	path, err := ag.LookPath()
	if err != nil {
		return loginValue{}, apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, ag.Spec().Binary+" not found on PATH")
	}
	code, err := s.RunSetup(ctx, path, cmd)
	if err != nil {
		return loginValue{}, err
	}
	if code != 0 {
		return loginValue{}, apperr.Failure(apperr.CodeIO, fmt.Sprintf("%s exited %d; nothing saved", cmd.String(), code))
	}
	return a.promptValue(ctx, label, "Paste the token it printed (hidden): ")
}

// shapeWarnings warns when a pasted value's shape says another kind
// (an API key pasted as an oauth-token login); the value is never shown.
func shapeWarnings(v loginValue, kind string, agents []agentio.Agent) []report.Warning {
	if v.pasted == "" {
		return nil
	}
	tl, _ := agentio.TokenLoginOf(agents[0])
	got, ok := tl.DetectTokenKind(agentio.NewToken(v.pasted))
	if !ok || got.String() == kind {
		return nil
	}
	return []report.Warning{{Code: WarnKindShape, Message: fmt.Sprintf("the value looks like an %s, and the login's kind is %s", got, kind)}}
}

// LoginEditOptions are agx login edit's flags. A nil pointer or empty
// list leaves that field as it is.
type LoginEditOptions struct {
	// ID is the login to change; required.
	ID string
	// Agents replaces the agents list when non-empty.
	Agents []agentio.ID
	// Kind changes the kind when non-empty.
	Kind string
	// Account sets the label; "" clears it.
	Account *string
	// Provider sets the provider; "" clears it.
	Provider *string
	// Source rotates the value (at most one source).
	Source LoginSource
}

// LoginEdit rotates or changes a login, keeping config comments and
// layout. Cost W, plus X with --login-setup.
func (a *App) LoginEdit(ctx context.Context, o LoginEditOptions) (LoginChange, error) {
	var ch LoginChange
	if err := o.Source.check(); err != nil {
		return ch, err
	}
	cfg, cur, err := a.namedLogin(o.ID)
	if err != nil {
		return ch, err
	}
	next := cur
	if len(o.Agents) > 0 {
		next.Agents = o.Agents
	}
	switch {
	case o.Kind != "":
		next.Kind = o.Kind
	case o.Source.HelperCommand != "":
		next.Kind = config.KindAPIKeyHelper
		ch.Decisions = append(ch.Decisions, report.Decision{Name: decLoginKind, Value: next.Kind, Source: flagHelperCommand})
	case cur.Kind == config.KindAPIKeyHelper && (o.Source.TokenRef != "" || o.Source.Stdin || o.Source.LoginSetup):
		return ch, apperr.Usage("login " + o.ID + " is an " + config.KindAPIKeyHelper + " login; add --kind for the new value")
	}
	if o.Account != nil {
		next.Account = *o.Account
	}
	if o.Provider != nil {
		next.Provider = *o.Provider
	}
	agents, err := a.tokenAgents(next.Agents)
	if err != nil {
		return ch, err
	}
	if err := checkKindAndProvider(next, agents); err != nil {
		return ch, err
	}
	v, err := a.obtainValue(ctx, o.ID, next.Kind, o.Source, agents, false)
	if err != nil {
		return ch, err
	}
	if len(o.Source.given()) > 0 {
		v.apply(&next)
	}
	if err := kindValueMatch(o.ID, next); err != nil {
		return ch, err
	}
	if err := usersStillServed(cfg.Config, o.ID, next); err != nil {
		return ch, err
	}
	if loginEqual(cur, next) && v.pasted == "" {
		return ch, apperr.Usage("nothing to change: give a source flag, --agent, --kind, --account or --provider")
	}
	ch.Warnings = shapeWarnings(v, next.Kind, agents)
	if v.pasted != "" {
		if err := login.WriteStored(ctx, a.paths, o.ID, v.pasted); err != nil {
			return ch, err
		}
	}
	if err := config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error { return editLoginDoc(d, o.ID, cur, next) }); err != nil {
		return ch, err
	}
	if cur.Stored && !next.Stored {
		if err := login.DeleteStored(ctx, a.paths, o.ID); err != nil {
			return ch, err
		}
	}
	return a.changed(ch, o.ID)
}

// kindValueMatch refuses a kind change that leaves the value of the other
// kind (a helper command on a token kind, or the reverse).
func kindValueMatch(name string, l config.Login) error {
	helper := l.Kind == config.KindAPIKeyHelper
	switch {
	case helper && l.Command == "":
		return apperr.Usage(fmt.Sprintf("login %s: an %s login needs %s", name, config.KindAPIKeyHelper, flagHelperCommand))
	case !helper && l.Command != "":
		return apperr.Usage(fmt.Sprintf("login %s has a helper command; give %s, %s, %s or a pasted value with --kind %s", name, flagTokenRef, flagStdin, flagLoginSetup, l.Kind))
	}
	return nil
}

// usersStillServed refuses dropping an agent that a profile, default or
// binding uses this login for (config would no longer load).
func usersStillServed(c config.Config, name string, next config.Login) error {
	for _, u := range loginUsers(c, name) {
		if !next.Serves(u.Agent) {
			return apperr.New(apperr.CodeWrongAgent, apperr.ClassUsage,
				fmt.Sprintf("%s uses login %s for %s; keep --agent %s", u, name, u.Agent, u.Agent))
		}
	}
	return nil
}

// loginEqual compares two logins field by field.
func loginEqual(x, y config.Login) bool {
	return slices.Equal(x.Agents, y.Agents) && x.Kind == y.Kind && x.Token == y.Token && x.Stored == y.Stored &&
		x.Account == y.Account && x.Command == y.Command && x.Provider == y.Provider
}

// Login entry keys in config.jsonc (spec 15.3).
const (
	keyAgents    = "agents"
	keyKind      = "kind"
	keyToken     = "token"
	keyStored    = "stored"
	keyAccount   = "account"
	keyCommand   = "command"
	keyProvider  = "provider"
	keyLogin     = "login"
	keyHomeLogin = "home_login"
	keyDefaults  = "defaults"
)

// editLoginDoc writes only the changed keys of login name, so comments on
// untouched keys stay (principle 11).
func editLoginDoc(d *jsonc.Doc, name string, cur, next config.Login) error {
	p := func(k string) string { return config.Pointer("logins", name, k) }
	if !slices.Equal(cur.Agents, next.Agents) {
		if err := d.Set(p(keyAgents), next.Agents); err != nil {
			return err
		}
	}
	for _, f := range []struct {
		key       string
		old, want string
	}{
		{keyKind, cur.Kind, next.Kind},
		{keyToken, cur.Token, next.Token},
		{keyCommand, cur.Command, next.Command},
		{keyAccount, cur.Account, next.Account},
		{keyProvider, cur.Provider, next.Provider},
	} {
		if err := setOrDelete(d, p(f.key), f.old != f.want, f.want); err != nil {
			return err
		}
	}
	if cur.Stored != next.Stored {
		if next.Stored {
			return d.Set(p(keyStored), true)
		}
		return deleteIfPresent(d, p(keyStored))
	}
	return nil
}

// setOrDelete writes value at ptr when changed: "" deletes the key.
func setOrDelete(d *jsonc.Doc, ptr string, changed bool, value string) error {
	if !changed {
		return nil
	}
	if value == "" {
		return deleteIfPresent(d, ptr)
	}
	return d.Set(ptr, value)
}

// deleteIfPresent deletes ptr when it exists.
func deleteIfPresent(d *jsonc.Doc, ptr string) error {
	if !d.Has(ptr) {
		return nil
	}
	return d.Delete(ptr)
}

// LoginRemoveOptions are agx login remove's flags.
type LoginRemoveOptions struct {
	// ID is the login to remove; required.
	ID string
	// Cascade also clears it from profiles, agent defaults and bindings.
	Cascade bool
}

// LoginRemoved is the result of login remove.
type LoginRemoved struct {
	// Name is the removed login.
	Name string `json:"name"`
	// Cleared are the entries --cascade cleared (empty without it).
	Cleared []LoginUser `json:"cleared"`
	// Warnings are notes about this terminal's AGX_LOGIN*.
	Warnings []report.Warning `json:"-"`
}

// LoginRemove removes a login from config.jsonc and tokens.jsonc. While
// a profile, agent default or binding names it, it refuses unless
// Cascade, which clears those entries too (a binding left empty is
// removed). Cost W.
func (a *App) LoginRemove(ctx context.Context, o LoginRemoveOptions) (LoginRemoved, error) {
	res := LoginRemoved{Name: o.ID, Cleared: []LoginUser{}}
	cfg, _, err := a.namedLogin(o.ID)
	if err != nil {
		return res, err
	}
	users := loginUsers(cfg.Config, o.ID)
	if len(users) > 0 && !o.Cascade {
		names := make([]string, len(users))
		for i, u := range users {
			names[i] = u.String()
		}
		return res, apperr.New(apperr.CodeInUse, apperr.ClassUsage,
			fmt.Sprintf("login %s is used by %s", o.ID, strings.Join(names, ", "))).
			WithHint("agx login remove --id " + o.ID + " --cascade   (also clears it there)")
	}
	err = config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
		for _, u := range users {
			if err := clearUser(d, u); err != nil {
				return err
			}
		}
		return d.Delete(config.Pointer("logins", o.ID))
	})
	if err != nil {
		return res, err
	}
	if err := login.DeleteStored(ctx, a.paths, o.ID); err != nil {
		return res, err
	}
	if o.Cascade {
		res.Cleared = users
	}
	res.Warnings = a.terminalNames(o.ID)
	a.Reload()
	return res, nil
}

// clearUser deletes the login key of one user; a binding left with no
// keys is deleted, and a folder left with no bindings too.
func clearUser(d *jsonc.Doc, u LoginUser) error {
	switch u.Type {
	case UserProfile:
		return d.Delete(config.Pointer("profiles", u.Name, keyLogin))
	case UserAgentDefault:
		if err := d.Delete(config.Pointer(keyAgents, u.Name, keyDefaults, keyLogin)); err != nil {
			return err
		}
		if err := deleteIfEmpty(d, config.Pointer(keyAgents, u.Name, keyDefaults)); err != nil {
			return err
		}
		return deleteIfEmpty(d, config.Pointer(keyAgents, u.Name))
	}
	bind := config.Pointer("projects", u.Name, string(u.Agent))
	if err := d.Delete(config.Pointer("projects", u.Name, string(u.Agent), keyLogin)); err != nil {
		return err
	}
	if err := deleteIfEmpty(d, bind); err != nil {
		return err
	}
	return deleteIfEmpty(d, config.Pointer("projects", u.Name))
}

// deleteIfEmpty deletes the object at ptr when it has no keys.
func deleteIfEmpty(d *jsonc.Doc, ptr string) error {
	keys, err := d.Keys(ptr)
	if err != nil || len(keys) > 0 {
		return err
	}
	return d.Delete(ptr)
}

// terminalNames warns about AGX_LOGIN* variables in this terminal that
// name login name (they would make every launch refuse).
func (a *App) terminalNames(name string) []report.Warning {
	var out []report.Warning
	for _, env := range a.loginEnvNames() {
		if v, ok := a.d.Lookup(env); ok && v == name {
			out = append(out, report.Warning{Code: WarnTerminalEnv, Message: fmt.Sprintf("%s in this terminal still names %s; unset it", env, name)})
		}
	}
	return out
}
