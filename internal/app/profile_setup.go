package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/login"
)

// checkLoginSetup validates profile add --login-setup before anything is
// written: the agent has a token setup command for the home, a terminal
// exists to paste the token, no other login flag is given, and the login
// name (the profile name) is free. It returns the command line to show.
func (a *App) checkLoginSetup(c config.Config, ag agentio.Agent, home agentio.Home, o ProfileAddOptions) (string, error) {
	if o.Login != "" || o.HomeLogin {
		return "", apperr.Conflict("--login-setup and --login/--home-login both choose the login; pick one")
	}
	tl, ok := agentio.TokenLoginOf(ag)
	if !ok {
		return "", apperr.Unsupported(fmt.Sprintf("%s has no token login, so no token setup command", ag.ID()))
	}
	cmd, ok := tl.TokenSetupCommand(home)
	if !ok {
		return "", apperr.Unsupported(fmt.Sprintf("%s has no token setup command", ag.ID()))
	}
	if a.d.Picker == nil && !o.DryRun {
		return "", noTerminal(flagLoginSetup+" runs the setup command, then asks you to paste the token it prints", "run it at a terminal, or use agx login add --stdin")
	}
	if _, taken := c.Logins[o.Name]; taken {
		return "", apperr.Conflict("login " + o.Name + " already exists; --login-setup stores the token as a login named like the profile").
			WithHint("agx profile add --name " + o.Name + " --agent " + string(ag.ID()) + " --login " + o.Name)
	}
	return commandLine(cmd), nil
}

// commandLine renders a command for display: binary name and args (args
// never hold a token, agentio invariant 8).
func commandLine(c agentio.Command) string {
	return strings.Join(append([]string{filepath.Base(c.Path)}, c.Args...), " ")
}

// LoginSetupCommand returns the token setup command for profile name's
// home and the binary to run it with (the CLI runs it attached to the
// terminal through internal/run). Cost X when run.
func (a *App) LoginSetupCommand(name string) (string, agentio.Command, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return "", agentio.Command{}, err
	}
	p, ok := cfg.Config.Profiles[name]
	if !ok {
		return "", agentio.Command{}, unknownProfile(name)
	}
	ag, ok := reg.Get(p.Agent)
	if !ok {
		return "", agentio.Command{}, unknownAgent(reg, string(p.Agent))
	}
	tl, ok := agentio.TokenLoginOf(ag)
	if !ok {
		return "", agentio.Command{}, apperr.Unsupported(fmt.Sprintf("%s has no token login", ag.ID()))
	}
	cmd, ok := tl.TokenSetupCommand(resolve.HomeOf(ag, a.expand(p.Home), name))
	if !ok {
		return "", agentio.Command{}, apperr.Unsupported(fmt.Sprintf("%s has no token setup command", ag.ID()))
	}
	path, err := ag.LookPath()
	if err != nil {
		msg := fmt.Sprintf("%s not found on PATH", ag.Spec().Binary)
		return "", agentio.Command{}, apperr.Wrap(err, apperr.CodeBinaryMissing, apperr.ClassNotFoundBinary, msg)
	}
	return path, cmd, nil
}

// StoreSetupToken reads the token the setup command printed (pasted at
// the terminal, never echoed), stores it in tokens.jsonc under the
// profile's name, adds the login entry (stored, the agent's first token
// kind) and makes it the profile's login. The value is never printed.
func (a *App) StoreSetupToken(ctx context.Context, name string) (string, error) {
	cfg, reg, err := a.loadedConfig()
	if err != nil {
		return "", err
	}
	p, ok := cfg.Config.Profiles[name]
	if !ok {
		return "", unknownProfile(name)
	}
	ag, ok := reg.Get(p.Agent)
	if !ok {
		return "", unknownAgent(reg, string(p.Agent))
	}
	tl, ok := agentio.TokenLoginOf(ag)
	if !ok || len(tl.TokenKinds()) == 0 {
		return "", apperr.Unsupported(fmt.Sprintf("%s has no token login", ag.ID()))
	}
	kind, err := tl.TokenKinds()[0].MarshalText()
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "token kind: "+err.Error())
	}
	if a.d.Picker == nil {
		return "", noTerminal("the token the setup command printed is pasted at a hidden prompt", "run it at a terminal, or use agx login add --stdin")
	}
	tok, err := a.d.Picker.Secret(ctx, "Paste the token the setup command printed: ")
	if err != nil {
		return "", err
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", apperr.Failure(apperr.CodeSecret, "login "+name+": token is empty")
	}
	if strings.ContainsAny(tok, "\r\n") {
		return "", apperr.Failure(apperr.CodeSecret, "login "+name+": token has several lines")
	}
	if err := login.WriteStored(ctx, a.paths, name, tok); err != nil {
		return "", err
	}
	entry := config.Login{Agents: []agentio.ID{p.Agent}, Kind: string(kind), Stored: true}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		if err := d.Set(config.Pointer("logins", name), entry); err != nil {
			return err
		}
		return d.Set(config.Pointer("profiles", name, pkLogin), name)
	})
	return name, err
}
