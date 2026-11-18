package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/plugin"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"
	"github.com/ubgo/textdata/pos"
	"github.com/ubgo/textdata/shell"
	"github.com/ubgo/textdata/value"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/login"
)

// dryValue stands in for every value that holds a reference during a
// --dry-run without --resolved-secrets: the reference is checked (unset
// names, unknown schemes, commands while off) but nothing runs, so the
// dry run stays cost R and never shows a half-resolved value.
const dryValue = "[not resolved: --dry-run]"

// envSource is one input of the launch environment, in precedence order
// (spec 7.8). label names it in decisions and errors.
type envSource struct {
	label   string
	changes []agentio.EnvChange
	// cmdLine marks command-line sources (--env-file, --env): only these
	// can conflict with another command-line choice (spec 7.1 rule 3).
	cmdLine bool
}

// launchEnv builds the env changes of a launch in precedence order (spec
// 7.5, 7.8): profile env_file → env → env_refs → --env-file → --env →
// --no-env. agentio applies them after the credential scrub and before
// the login token and the home rule. References expand strictly (7.7);
// with --dry-run they are checked but not resolved unless
// --resolved-secrets. Each source adds one decision naming its variables
// (never values). Refused: any source setting the home variable (edge
// 21), and a command-line credential variable while a token login is
// used (edge 22).
func (a *App) launchEnv(ctx context.Context, o LaunchOptions, res resolve.Result, p *LaunchPlan) ([]agentio.EnvChange, error) {
	prof := res.Profile
	ex := envExpander{app: a, ctx: ctx, dry: o.DryRun && !o.ResolvedSecrets}
	profLabel := "profile " + res.ProfileName
	var sources []envSource

	if prof.EnvFile != "" {
		ch, err := ex.file(prof.EnvFile, a.paths.Dir, prof.EnvFileCommands)
		if err != nil {
			return nil, err
		}
		sources = append(sources, envSource{label: profLabel + " env_file " + prof.EnvFile, changes: ch})
	}
	if len(prof.Env) > 0 {
		var ch []agentio.EnvChange
		for _, k := range sortedNames(prof.Env) {
			ch = append(ch, agentio.EnvSet(k, prof.Env[k]))
		}
		sources = append(sources, envSource{label: profLabel + " env", changes: ch})
	}
	if len(prof.EnvRefs) > 0 {
		var ch []agentio.EnvChange
		for _, k := range sortedNames(prof.EnvRefs) {
			v, err := ex.value(profLabel+" env_refs "+k, prof.EnvRefs[k])
			if err != nil {
				return nil, err
			}
			ch = append(ch, envChange(k, prof.EnvRefs[k], v))
		}
		sources = append(sources, envSource{label: profLabel + " env_refs", changes: ch})
	}
	for _, f := range o.EnvFiles {
		ch, err := ex.file(f, a.d.Cwd, prof.EnvFileCommands)
		if err != nil {
			return nil, err
		}
		sources = append(sources, envSource{label: "--env-file " + f, changes: ch, cmdLine: true})
	}
	if len(o.Env) > 0 {
		var ch []agentio.EnvChange
		for _, kv := range o.Env {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				return nil, apperr.Usage("--env " + kv + ": use NAME=value")
			}
			ev, err := ex.value("--env "+k, v)
			if err != nil {
				return nil, err
			}
			ch = append(ch, envChange(k, v, ev))
		}
		sources = append(sources, envSource{label: "--env", changes: ch, cmdLine: true})
	}
	if len(o.NoEnv) > 0 {
		var ch []agentio.EnvChange
		for _, k := range o.NoEnv {
			if k == "" || strings.Contains(k, "=") {
				return nil, apperr.Usage("--no-env " + k + ": use a variable NAME")
			}
			ch = append(ch, agentio.EnvUnset(k))
		}
		sources = append(sources, envSource{label: "--no-env", changes: ch, cmdLine: true})
	}

	if err := checkEnvSources(res, sources); err != nil {
		return nil, err
	}
	var out []agentio.EnvChange
	for _, s := range sources {
		out = append(out, s.changes...)
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecEnv, Value: envNames(s.changes), Source: s.label})
	}
	return out, nil
}

// checkEnvSources refuses the two env choices agentio would silently
// override: the home variable (the home rule is applied last, edge 21)
// and a credential variable from the command line while the run uses a
// token login (the login token is applied after the env, edge 22).
func checkEnvSources(res resolve.Result, sources []envSource) error {
	homeEnv := res.Agent.Spec().HomeEnv
	cred := tokenLoginVars(res)
	for _, s := range sources {
		for _, ch := range s.changes {
			name := ch.Name()
			if homeEnv != "" && name == homeEnv {
				msg := name + " is set by the profile's home; use --profile"
				if s.label != "--env" {
					msg += " (" + s.label + ")"
				}
				return apperr.Usage(msg)
			}
			if s.cmdLine && ch.Op() != agentio.EnvUnsetOp && slices.Contains(cred, name) {
				return apperr.Conflict(fmt.Sprintf("%s %s and %s both set the credential; pick one", flagOf(s.label), name, loginChoice(res.Login)))
			}
		}
	}
	return nil
}

// flagOf is the flag part of a source label ("--env-file ./x" → "--env-file").
func flagOf(label string) string {
	f, _, _ := strings.Cut(label, " ")
	return f
}

// loginChoice names the run's token login and what chose it: "--login
// personal", "--token-ref", "login personal (from profile work)".
func loginChoice(l resolve.Login) string {
	if l.Kind == resolve.LoginTokenRef {
		return "--token-ref"
	}
	if strings.HasPrefix(l.Source, "--") {
		return flagOf(l.Source) + " " + l.Name
	}
	return fmt.Sprintf("login %s (from %s)", l.Name, l.Source)
}

// tokenLoginVars are the variables that carry or outrank the run's token
// login: what agentio's TokenEnv sets for its kind and provider, plus the
// agent's credential variables. Empty for the home login, where a
// credential set on purpose is kept (spec 7.5).
func tokenLoginVars(res resolve.Result) []string {
	l := res.Login
	if l.Kind == resolve.LoginHome {
		return nil
	}
	kind := l.Login.Kind
	if l.Kind == resolve.LoginTokenRef {
		kind = l.RefKind
	}
	out := slices.Clone(res.Agent.Spec().CredentialEnv)
	tl, ok := agentio.TokenLoginOf(res.Agent)
	k, kok := login.KindOf(kind)
	if !ok || !kok {
		return out
	}
	// The token value is a placeholder: only the variable names are read.
	chs, err := tl.TokenEnv(agentio.TokenCredential{Kind: k, Token: agentio.NewToken(dryValue), Provider: l.Login.Provider})
	if err != nil {
		return out
	}
	for _, ch := range chs {
		if ch.Op() != agentio.EnvUnsetOp {
			out = append(out, ch.Name())
		}
	}
	return out
}

// envNames lists the variables a source changes, "-NAME" for removals.
func envNames(chs []agentio.EnvChange) string {
	names := make([]string, len(chs))
	for i, ch := range chs {
		names[i] = ch.Name()
		if ch.Op() == agentio.EnvUnsetOp {
			names[i] = "-" + names[i]
		}
	}
	return strings.Join(names, ", ")
}

// envExpander expands env values for one launch, strictly (spec 7.7).
// With dry set it runs the same strict engine over stub resolvers: every
// reference is checked, nothing runs, and a value holding a reference
// becomes dryValue.
type envExpander struct {
	app *App
	ctx context.Context
	dry bool
}

// value expands one --env or env_refs value; commands are on (bounded by
// login.CommandTimeout), as for a login token.
func (e envExpander) value(label, s string) (string, error) {
	if !strings.Contains(s, refMarker) {
		return s, nil
	}
	if !e.dry {
		return e.app.exp.Expand(e.ctx, label, s, true)
	}
	_, err := value.Expand(e.ctx, s, value.WithStrict(), value.WithLookup(e.app.d.Lookup),
		value.WithPlugins(dryResolver{}), value.WithRunner(dryRunner{}))
	if err != nil {
		return "", login.ExpandError(label, err, login.CommandTimeout)
	}
	return dryValue, nil
}

// file reads a dotenv file (raw is as written, relative to base) with
// strict expansion; $(…) is an error unless commands (env_file_commands).
// A file reference resolves against the file's own keys first, then the
// process environment and the gopass plugin.
func (e envExpander) file(raw, base string, commands bool) ([]agentio.EnvChange, error) {
	path := resolvePath(raw, base, e.app)
	opts := []dotenv.Option{dotenv.WithStrictExpand(), dotenv.WithLookup(e.app.d.Lookup)}
	switch {
	case e.dry:
		opts = append(opts, dotenv.WithPlugin(dryResolver{}))
		if commands {
			opts = append(opts, dotenv.WithCommandRunner(dryRunner{}.RunCommand))
		}
	default:
		gp := []gopassplugin.Option{gopassplugin.WithTimeout(login.CommandTimeout), gopassplugin.WithContext(e.ctx)}
		if e.app.d.GopassBinary != "" {
			gp = append(gp, gopassplugin.WithBinary(e.app.d.GopassBinary))
		}
		opts = append(opts, dotenv.WithPlugin(gopassplugin.New(gp...)))
		if commands {
			opts = append(opts, dotenv.WithCommandRunner(shell.Runner{Timeout: login.CommandTimeout}.RunCommand), dotenv.WithCommandContext(e.ctx))
		}
	}
	f, err := dotenv.Open(path, opts...)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("env file %s: %s", raw, errText(err)))
	}
	if !f.Existed() {
		return nil, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("env file %s not found (%s)", raw, e.app.tilde(path)))
	}
	vals, err := f.ExpandedMap()
	if err != nil {
		return nil, envFileError(raw, err)
	}
	raws := f.Map()
	var out []agentio.EnvChange
	for _, k := range f.Keys() {
		v, ok := vals[k]
		if !ok {
			continue
		}
		if e.dry && strings.Contains(raws[k], refMarker) {
			v = dryValue
		}
		// An env file is where people keep secrets, literal or not: every
		// value it sets is redacted in output.
		out = append(out, agentio.EnvSetSecret(k, agentio.NewToken(v)))
	}
	return out, nil
}

// envChange sets name to the expanded value: a value that came from a
// reference (the raw text holds "$") is a secret and set through a Token,
// so --dry-run, --json and errors show it redacted; a literal the user
// typed is shown as written.
func envChange(name, raw, value string) agentio.EnvChange {
	if strings.Contains(raw, refMarker) {
		return agentio.EnvSetSecret(name, agentio.NewToken(value))
	}
	return agentio.EnvSet(name, value)
}

// envFileError maps a strict expansion failure in an env file to
// "<file>:<line>:<col>: …" naming the reference, never a value (edge 32b).
func envFileError(raw string, err error) error {
	where := raw
	if pe := pos.First(err); pe != nil && pe.Pos.IsValid() {
		where = fmt.Sprintf("%s:%d:%d", raw, pe.Pos.Line, pe.Pos.Col)
	}
	var off *expand.CommandOffError
	if errors.As(err, &off) {
		return apperr.Wrap(err, apperr.CodeSecret, apperr.ClassFailure, fmt.Sprintf("%s: $(%s) is a command, and env_file_commands is off", where, off.Cmd))
	}
	return login.ExpandError(where, err, login.CommandTimeout)
}

// errText is an error's text without the file position prefix dotenv
// adds (the caller names the file as written).
func errText(err error) string {
	if pe := pos.First(err); pe != nil && pe.Msg != "" {
		return pe.Msg
	}
	return apperr.Text(err)
}

// resolvePath makes raw absolute: "~" and absolute paths as written
// (with $VAR expanded), anything else relative to base.
func resolvePath(raw, base string, a *App) string {
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "~") || strings.HasPrefix(raw, refMarker) {
		return config.ExpandPath(raw, a.d.Lookup, a.d.Host.UserHome)
	}
	return filepath.Join(base, raw)
}

// dryResolver claims the schemes agx installs (gopass) during a dry run,
// so a known scheme passes the strict check without running anything and
// an unknown one still fails (edge 19).
type dryResolver struct{}

// Name names the stub in plugin errors.
func (dryResolver) Name() string { return "dry-run" }

// Schemes are the schemes the real expansion installs.
func (dryResolver) Schemes() []string { return []string{gopassplugin.Scheme} }

// Resolve never reads the store.
func (dryResolver) Resolve(string, string) (string, error) { return dryValue, nil }

// dryRunner stands in for the shell during a dry run: commands are on, so
// strict mode does not refuse them, but none runs.
type dryRunner struct{}

// RunCommand never runs cmd.
func (dryRunner) RunCommand(string) (string, error) { return dryValue, nil }

// compile-time checks that the stubs satisfy textdata's plugin seams.
var (
	_ plugin.Resolver      = dryResolver{}
	_ plugin.CommandRunner = dryRunner{}
)
