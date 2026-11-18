package app

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/expand"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/login"
)

// referenceSchemes are the ${scheme:ref} schemes a launch can resolve:
// the plugins login.Expander installs (gopass today). Kept in step with
// login.NewExpander; requested as login.Expander.Schemes (doctor R4).
var referenceSchemes = []string{gopassplugin.Scheme}

// refMarker starts every reference (${…}, $(…), $VAR); a value without
// it is a plain value.
const refMarker = "$"

// configRef is one config value that holds (or may hold) a reference.
type configRef struct {
	// key is the dotted config path ("logins.derek.token"); it names the
	// value in every message, the value itself is never shown.
	key   string
	value string
}

// collectRefs lists every reference-bearing value of the config in a
// stable order: login tokens, then profile env_refs.
func collectRefs(c config.Config) []configRef {
	var out []configRef
	for _, name := range sortedNames(c.Logins) {
		lg := c.Logins[name]
		if lg.Token != "" && strings.Contains(lg.Token, refMarker) {
			out = append(out, configRef{key: "logins." + name + ".token", value: lg.Token})
		}
	}
	for _, name := range sortedNames(c.Profiles) {
		p := c.Profiles[name]
		for _, k := range sortedNames(p.EnvRefs) {
			out = append(out, configRef{key: "profiles." + name + ".env_refs." + k, value: p.EnvRefs[k]})
		}
	}
	return out
}

// refScan is what parsing one reference found, without resolving it.
type refScan struct {
	commands []string
	err      error
}

// scanRef parses s with textdata's engine in strict mode, resolving
// nothing: variables answer from the environment (set or not), a known
// scheme answers empty, an unknown scheme fails (UnknownSchemeError), and
// $(…) is recorded instead of run. Pure apart from reading the env.
func scanRef(s string, lookup config.Lookup) refScan {
	var sc refScan
	eng := expand.Engine{
		Strict: true,
		Lookup: func(name string, _ *expand.Stack) (expand.Var, error) {
			_, ok := lookup(name)
			return expand.Var{Set: ok, Literal: true, At: expand.AtLookup}, nil
		},
		Resolve: func(scheme, _ string) (string, bool, error) {
			return "", slices.Contains(referenceSchemes, scheme), nil
		},
		Run: func(cmd string) (string, error) {
			sc.commands = append(sc.commands, cmd)
			return "", nil
		},
	}
	out, err := eng.Expand(s, expand.NewStack())
	switch {
	case err != nil:
		sc.err = err
	case strings.Contains(out, "${") || strings.Contains(out, "$("):
		sc.err = errUnbalanced
	}
	return sc
}

// errUnbalanced is a ${ or $( without its closing bracket: the engine
// keeps such text as written, which a launch would pass on literally.
var errUnbalanced = errors.New("unbalanced ${ or $(")

// checkReferences reports every reference without resolving it (spec 9):
// it parses, its scheme has a plugin, its variables are set here, and
// each $(…) program is on PATH. api-key-helper commands, which the agent
// runs itself, get the program check too.
func (a *App) checkReferences(r *DoctorReport, c config.Config) {
	for _, ref := range collectRefs(c) {
		a.checkRef(r, ref)
	}
	for _, name := range sortedNames(c.Logins) {
		lg := c.Logins[name]
		if lg.Command != "" {
			a.checkProgram(r, "logins."+name+".command", lg.Command)
		}
	}
	for _, name := range sortedNames(c.Profiles) {
		if f := c.Profiles[name].EnvFile; f != "" {
			a.checkEnvFile(r, "profiles."+name+".env_file", f)
		}
	}
}

// checkRef reports one reference.
func (a *App) checkRef(r *DoctorReport, ref configRef) {
	sc := scanRef(ref.value, a.d.Lookup)
	var (
		unset  *expand.UnsetError
		scheme *expand.UnknownSchemeError
	)
	switch {
	case errors.As(sc.err, &scheme):
		r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceScheme, Status: agentio.CheckFail,
			Message: ref.key + ": no plugin for scheme " + scheme.Scheme + " (known: " + strings.Join(referenceSchemes, ", ") + ")"})
		return
	case errors.As(sc.err, &unset):
		// The variable may be set in the terminal that launches; a launch
		// refuses then (edge 20), so warn rather than fail.
		r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceVar, Status: agentio.CheckWarn,
			Message: ref.key + ": " + unset.Name + " is not set in this environment"})
		return
	case sc.err != nil:
		r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceParse, Status: agentio.CheckFail, Message: ref.key + ": does not parse: " + apperr.Text(sc.err)})
		return
	}
	for _, cmd := range sc.commands {
		a.checkProgram(r, ref.key, cmd)
	}
	r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceParse, Status: agentio.CheckOK, Message: ref.key + ": parses"})
}

// checkProgram reports whether the program of a command line (its first
// word) is on PATH, or exists when given as a path.
func (a *App) checkProgram(r *DoctorReport, key, cmd string) {
	prog := firstWord(cmd)
	if prog == "" {
		r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceProgram, Status: agentio.CheckFail, Message: key + ": empty command"})
		return
	}
	if strings.ContainsRune(prog, '/') || strings.ContainsRune(prog, filepath.Separator) {
		if st, err := os.Stat(config.ExpandPath(prog, a.d.Lookup, a.d.Host.UserHome)); err != nil || st.IsDir() {
			r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceProgram, Status: agentio.CheckFail, Message: key + ": program " + prog + " not found"})
			return
		}
	} else if _, err := a.d.Host.LookPath(prog); err != nil {
		r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceProgram, Status: agentio.CheckFail, Message: key + ": program " + prog + " not on PATH"})
		return
	}
	r.add(DoctorCheck{Area: AreaReference, ID: CheckReferenceProgram, Status: agentio.CheckOK, Message: key + ": program " + prog + " found"})
}

// firstWord is a command line's program: its first word, without quotes.
func firstWord(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	return strings.Trim(f[0], `"'`)
}

// checkEnvFile reports a profile env_file that does not exist (it is
// relative to the config dir).
func (a *App) checkEnvFile(r *DoctorReport, key, f string) {
	p := f
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "~") {
		p = filepath.Join(a.paths.Dir, p)
	}
	p = config.ExpandPath(p, a.d.Lookup, a.d.Host.UserHome)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		r.add(DoctorCheck{Area: AreaReference, ID: CheckEnvFile, Status: agentio.CheckFail, Message: key + ": " + a.tilde(p) + " not found"})
		return
	}
	r.add(DoctorCheck{Area: AreaReference, ID: CheckEnvFile, Status: agentio.CheckOK, Message: key + ": " + a.tilde(p) + " found"})
}

// checkPlainTokens lists, as information, every plain token value in
// config.jsonc by its key, never its value (spec 9, edge 23): login
// tokens without a reference, and profile env entries whose name is a
// credential variable of any agent; a literal whose name only looks
// secret (agentio's rule) gets a hint that it is shown as written.
func (a *App) checkPlainTokens(r *DoctorReport, c config.Config, reg *agents.Registry) {
	for _, name := range sortedNames(c.Logins) {
		lg := c.Logins[name]
		if lg.Token != "" && !strings.Contains(lg.Token, refMarker) {
			r.add(plainCheck("logins." + name + ".token"))
		}
	}
	creds := credentialNames(reg)
	for _, name := range sortedNames(c.Profiles) {
		env := c.Profiles[name].Env
		for _, k := range sortedNames(env) {
			key := "profiles." + name + ".env." + k
			switch {
			case env[k] == "":
			case slices.Contains(creds, k):
				r.add(plainCheck(key))
			case looksSecret(k):
				r.add(literalCheck(key))
			}
		}
	}
}

// literalCheck is the info line for a literal profile env value whose
// name looks secret. Output redacts by source and value, never by name,
// so such a literal is shown as written; the hint says how to keep a
// real secret redacted.
func literalCheck(key string) DoctorCheck {
	return DoctorCheck{Area: AreaPlainToken, ID: CheckPlainToken, Status: agentio.CheckInfo,
		Message: key + " looks like a secret written as a literal: shown as written in output; put it in env_refs to keep it redacted"}
}

// plainCheck is the info line for one plain token key.
func plainCheck(key string) DoctorCheck {
	return DoctorCheck{Area: AreaPlainToken, ID: CheckPlainToken, Status: agentio.CheckInfo,
		Message: key + " holds a plain token value (allowed; redacted in all output)"}
}

// credentialNames are every agent's credential variables: CredentialEnv,
// APIBillingEnv and each TokenEnv variable.
func credentialNames(reg *agents.Registry) []string {
	var out []string
	for _, ag := range reg.All() {
		s := ag.Spec()
		out = append(out, s.CredentialEnv...)
		out = append(out, s.APIBillingEnv...)
		for _, te := range s.TokenEnv {
			out = append(out, te.Env)
		}
	}
	return out
}

// looksSecret applies agentio's secret-name rule (the one Command.Redacted
// uses) to one variable name, so agx keeps no second list.
func looksSecret(name string) bool {
	c := agentio.Command{Env: []string{name + "=x"}}
	return c.Redacted().Env[0] != c.Env[0]
}

// checkResolved resolves every reference and stored value (cost X,
// --resolved-secrets) and reports only whether it resolved; a value is
// never kept or shown. A Ctrl-C stops it.
func (a *App) checkResolved(ctx context.Context, r *DoctorReport, c config.Config) error {
	for _, name := range sortedNames(c.Logins) {
		lg := c.Logins[name]
		if lg.Kind == config.KindAPIKeyHelper || (!lg.Stored && !strings.Contains(lg.Token, refMarker)) {
			continue
		}
		_, err := login.Credential(ctx, a.exp, a.paths, name, lg)
		if cerr := ctxErr(ctx); cerr != nil {
			return cerr
		}
		r.add(resolvedCheck("login "+name, err))
	}
	for _, ref := range collectRefs(c) {
		if strings.HasPrefix(ref.key, "logins.") {
			continue
		}
		_, err := a.exp.Expand(ctx, ref.key, ref.value, true)
		if cerr := ctxErr(ctx); cerr != nil {
			return cerr
		}
		r.add(resolvedCheck(ref.key, err))
	}
	return nil
}

// resolvedCheck is the line for one resolution: ok, or the error, which
// names the reference and the reason and never the value (login package).
func resolvedCheck(label string, err error) DoctorCheck {
	if err != nil {
		return DoctorCheck{Area: AreaSecret, ID: CheckSecretResolves, Status: agentio.CheckFail, Message: errMessage(err)}
	}
	return DoctorCheck{Area: AreaSecret, ID: CheckSecretResolves, Status: agentio.CheckOK, Message: label + ": resolves"}
}

// errMessage is an error's message as agx prints it.
func errMessage(err error) string { return apperr.As(err).Message }

// sortedNames returns a map's keys sorted, so the report is stable.
func sortedNames[K cmp.Ordered, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
