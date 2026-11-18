package app

import (
	"context"
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// LoginUseTarget is where login use sets the login.
type LoginUseTarget string

// login use targets: exactly one per command (spec 13.5).
const (
	// UseProfile sets profiles.<name>.login (or home_login).
	UseProfile LoginUseTarget = "profile"
	// UseAgent sets agents.<id>.defaults.login.
	UseAgent LoginUseTarget = "agent"
	// UseTerminal prints the AGX_LOGIN line for this terminal; writes nothing.
	UseTerminal LoginUseTarget = "terminal"
)

// LoginUseTargetValues is the canonical list.
func LoginUseTargetValues() []LoginUseTarget {
	return []LoginUseTarget{UseProfile, UseAgent, UseTerminal}
}

// Shell lines --terminal prints (spec 8.3, edge 54). POSIX syntax: the
// shell layer's wrapper evaluates it (it translates for fish and pwsh);
// without the layer the user copies it.
const (
	exportLinePrefix = "export " + resolve.EnvLogin + "="
	unsetLine        = "unset " + resolve.EnvLogin
)

// LoginUseOptions are agx login use's flags.
type LoginUseOptions struct {
	// ID is the login; HomeLogin chooses the home login instead. Exactly one.
	ID        string
	HomeLogin bool
	// Profile, Agent and Terminal pick the target. Exactly one.
	Profile  string
	Agent    agentio.ID
	Terminal bool
}

// LoginUsed is the result of login use.
type LoginUsed struct {
	// Login is the login set; "" for the home login.
	Login string `json:"login"`
	// HomeLogin is true when the home login was chosen.
	HomeLogin bool `json:"home_login"`
	// Target is where it was set.
	Target LoginUseTarget `json:"target"`
	// Name is the profile or agent; "" for the terminal.
	Name string `json:"name,omitempty"`
	// Line is the shell line for --terminal: "export AGX_LOGIN=X" or
	// "unset AGX_LOGIN".
	Line string `json:"line,omitempty"`
	// Warnings are notes (a per-agent AGX_LOGIN_<AGENT> outranks the export).
	Warnings []report.Warning `json:"-"`
}

// LoginUse sets a default login for a profile or an agent (cost W), or
// prints this terminal's AGX_LOGIN line (cost R). A login that does not
// serve the target's agent is refused (edge 11, exit 2).
func (a *App) LoginUse(ctx context.Context, o LoginUseOptions) (LoginUsed, error) {
	var res LoginUsed
	switch {
	case o.ID != "" && o.HomeLogin:
		return res, apperr.Conflict("--id and --home-login both choose the login; pick one")
	case o.ID == "" && !o.HomeLogin:
		return res, apperr.Usage("give --id LOGIN or --home-login")
	}
	target, err := useTarget(o)
	if err != nil {
		return res, err
	}
	res = LoginUsed{Login: o.ID, HomeLogin: o.HomeLogin, Target: target}
	var l config.Login
	if o.ID != "" {
		if _, l, err = a.namedLogin(o.ID); err != nil {
			return res, err
		}
	}
	switch target {
	case UseTerminal:
		return a.useTerminal(res, l)
	case UseAgent:
		res.Name = string(o.Agent)
		return res, a.useAgent(ctx, o, l)
	}
	res.Name = o.Profile
	return res, a.useProfile(ctx, o, l)
}

// useTarget checks that exactly one target flag is given.
func useTarget(o LoginUseOptions) (LoginUseTarget, error) {
	var set []LoginUseTarget
	if o.Profile != "" {
		set = append(set, UseProfile)
	}
	if o.Agent != "" {
		set = append(set, UseAgent)
	}
	if o.Terminal {
		set = append(set, UseTerminal)
	}
	switch len(set) {
	case 0:
		return "", apperr.Usage("give one of --profile NAME, --agent ID or --terminal")
	case 1:
		return set[0], nil
	}
	return "", apperr.Conflict(fmt.Sprintf("--%s and --%s both choose where the login is set; pick one", set[0], set[1]))
}

// useTerminal builds the AGX_LOGIN line after validating the login (it
// exists; the config loads). Nothing is written.
func (a *App) useTerminal(res LoginUsed, l config.Login) (LoginUsed, error) {
	if res.HomeLogin {
		res.Line = unsetLine
		return res, nil
	}
	res.Line = exportLinePrefix + res.Login
	for _, id := range l.Agents {
		env := resolve.LoginEnvFor(id)
		if v, ok := a.d.Lookup(env); ok && v != "" && v != res.Login {
			res.Warnings = append(res.Warnings, report.Warning{Code: WarnTerminalEnv,
				Message: fmt.Sprintf("%s=%s outranks %s for %s; unset it", env, v, resolve.EnvLogin, id)})
		}
	}
	return res, nil
}

// useProfile sets profiles.<name>.login, or home_login true (dropping the
// login) so an inherited agent default no longer applies.
func (a *App) useProfile(ctx context.Context, o LoginUseOptions, l config.Login) error {
	cfg, err := a.Config()
	if err != nil {
		return err
	}
	p, ok := cfg.Config.Profiles[o.Profile]
	if !ok {
		return apperr.Failure(apperr.CodeNotFound, "unknown profile "+o.Profile).WithHint("agx profile list")
	}
	if o.ID != "" && !l.Serves(p.Agent) {
		return loginWrongAgent(o.ID, l, "profile "+o.Profile+" is "+string(p.Agent))
	}
	err = config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
		login, home := config.Pointer("profiles", o.Profile, keyLogin), config.Pointer("profiles", o.Profile, keyHomeLogin)
		if o.HomeLogin {
			if err := deleteIfPresent(d, login); err != nil {
				return err
			}
			return d.Set(home, true)
		}
		if err := deleteIfPresent(d, home); err != nil {
			return err
		}
		return d.Set(login, o.ID)
	})
	a.Reload()
	return err
}

// useAgent sets agents.<id>.defaults.login; --home-login removes it (the
// home login is what an agent uses when no default names one).
func (a *App) useAgent(ctx context.Context, o LoginUseOptions, l config.Login) error {
	if err := a.loginKnownAgent(o.Agent); err != nil {
		return err
	}
	if o.ID != "" && !l.Serves(o.Agent) {
		return loginWrongAgent(o.ID, l, "--agent is "+string(o.Agent))
	}
	err := config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
		login := config.Pointer(keyAgents, string(o.Agent), keyDefaults, keyLogin)
		if o.HomeLogin {
			return deleteIfPresent(d, login)
		}
		if err := deleteIfPresent(d, config.Pointer(keyAgents, string(o.Agent), keyDefaults, keyHomeLogin)); err != nil {
			return err
		}
		return d.Set(login, o.ID)
	})
	a.Reload()
	return err
}

// loginEnvNames are AGX_LOGIN and every AGX_LOGIN_<AGENT> of the
// registry's agents.
func (a *App) loginEnvNames() []string {
	out := []string{resolve.EnvLogin}
	if reg, err := a.Registry(); err == nil {
		for _, id := range reg.IDs() {
			out = append(out, resolve.LoginEnvFor(id))
		}
	}
	return out
}
