package app

import (
	"slices"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/report"
)

// minSecretInText is the shortest secret value searched for inside a
// longer text (an argument, a decision). A real token is far longer;
// a shorter value would match ordinary words and blank out unrelated
// text. Whole env values are compared at any length.
const minSecretInText = 8

// secretRedactor hides secret values in what a launch prints. A value is
// secret by its source or by its value, never by its variable's name:
//   - by source: an env variable set from a token, a reference, an env
//     file or tokens.jsonc (agentio marks these: Command.SecretEnvNames),
//     or the launched agent's own credential variables (its Spec), which
//     only ever hold a credential;
//   - by value: any other variable, argument or decision text that holds
//     one of those values.
//
// agentio's Command.Redacted also hides values whose name merely looks
// secret (TOKEN, KEY, …), which hid plain numbers such as
// CLAUDE_CODE_API_KEY_HELPER_TTL_MS; agx prints through this instead.
type secretRedactor struct {
	names  []string
	values []string
}

// newSecretRedactor collects the secret names and values of cmd. creds
// are the agent's credential variables; dry-run placeholders are not
// secrets and are left alone.
func newSecretRedactor(cmd agentio.Command, creds []string) secretRedactor {
	r := secretRedactor{names: append(cmd.SecretEnvNames(), creds...)}
	for _, kv := range cmd.Env {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || val == "" || isDryPlaceholder(val) || !slices.Contains(r.names, name) {
			continue
		}
		if !slices.Contains(r.values, val) {
			r.values = append(r.values, val)
		}
	}
	// Longest first, so a value that contains another is replaced whole.
	slices.SortFunc(r.values, func(a, b string) int { return len(b) - len(a) })
	return r
}

// isDryPlaceholder reports the stand-ins a dry run puts where a token or
// a reference would be: they are not secrets, and the decisions name them
// on purpose ("carol (not resolved: --dry-run)").
func isDryPlaceholder(val string) bool {
	return val == dryValue || val == notResolvedDry
}

// env redacts one NAME=value entry.
func (r secretRedactor) env(kv string) string {
	name, val, ok := strings.Cut(kv, "=")
	if !ok || val == "" {
		return kv
	}
	if slices.Contains(r.names, name) || slices.Contains(r.values, val) {
		return name + "=" + redactedText
	}
	return name + "=" + r.text(val)
}

// text replaces every secret value inside s.
func (r secretRedactor) text(s string) string {
	for _, v := range r.values {
		if len(v) >= minSecretInText || v == s {
			s = strings.ReplaceAll(s, v, redactedText)
		}
	}
	return s
}

// command is cmd with its env and args redacted, safe to print.
func (r secretRedactor) command(cmd agentio.Command) agentio.Command {
	out := cmd
	out.Args = make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		out.Args[i] = r.text(a)
	}
	out.Env = make([]string, len(cmd.Env))
	for i, kv := range cmd.Env {
		out.Env[i] = r.env(kv)
	}
	return out
}

// decisions redacts secret values in decision values and sources (a raw
// arg holding a token is shown in the args line too).
func (r secretRedactor) decisions(ds []report.Decision) {
	for i := range ds {
		ds[i].Value = r.text(ds[i].Value)
		ds[i].Source = r.text(ds[i].Source)
	}
}

// agentCredentialVars are the variables any agent reads a credential
// from (credentialNames: CredentialEnv, APIBillingEnv, each TokenEnv
// variable): a value there is a credential whatever its source, and
// agx doctor lists a plain one as redacted in all output.
func (a *App) agentCredentialVars() []string {
	reg, err := a.Registry()
	if err != nil {
		return nil
	}
	return credentialNames(reg)
}
