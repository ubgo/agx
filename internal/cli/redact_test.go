package cli_test

import (
	"strings"
	"testing"
)

// redactShown is the text a redacted value is replaced by in output.
const redactShown = "[redacted]"

// redactConfig adds to leakConfig a profile whose literal env holds a
// non-secret variable with a secret-looking name and a copy of the plain
// login token under an ordinary name, and whose raw args repeat the
// token; and a profile with a literal value on the agent's own
// credential variable (no login, so nothing scrubs it).
var redactConfig = strings.Replace(leakConfig, `"codex": {`,
	`"names": { "agent": "claude", "home": "~/.claude-work", "login": "plain",
      "env": { "MAX_TOKENS_HINT": "4096", "COPY_OF_LOGIN": "`+leakPlain+`" },
      "args": ["--api-token=`+leakPlain+`"] },
    "credvar": { "agent": "claude", "home": "~/.claude-work",
      "env": { "ANTHROPIC_API_KEY": "`+leakOneOff+`" } },
    "codex": {`, 1)

// TestRedactBySourceNotName: a value is redacted because of where it
// came from (a token, a reference, an env file, tokens.jsonc, the agent's
// own credential variable) or because it equals such a value, never
// because its variable's name looks secret.
func TestRedactBySourceNotName(t *testing.T) {
	f := leakFixture(t)
	f.WriteConfig(redactConfig)

	r := f.Run("run", "--profile", "names", "--dry-run", "--resolved-secrets")
	if r.Code != 0 {
		t.Fatalf("code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
	for _, want := range []string{
		"set MAX_TOKENS_HINT=4096",
		"set COPY_OF_LOGIN=" + redactShown,
		"set CLAUDE_CODE_OAUTH_TOKEN=" + redactShown,
		"--api-token=" + redactShown,
	} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("dry run lacks %q:\n%s", want, r.Stdout)
		}
	}
	for _, s := range leaks {
		if strings.Contains(r.Stdout+r.Stderr, s) {
			t.Errorf("dry run printed a secret (%s…)", s[:14])
		}
	}

	r = f.Run("run", "--profile", "credvar", "--dry-run")
	if r.Code != 0 || !strings.Contains(r.Stdout, "set ANTHROPIC_API_KEY="+redactShown) || strings.Contains(r.Stdout, leakOneOff) {
		t.Errorf("credential variable not redacted: code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestRedactLiveLoginTTL: the live-login helper's refresh interval is a
// number, not a secret, and shows; the helper's settings stay keys only.
func TestRedactLiveLoginTTL(t *testing.T) {
	f := leakFixture(t)
	r := f.Run("run", "--dry-run", "--resolved-secrets", "--login", "ref", "--live-login")
	if r.Code != 0 {
		t.Fatalf("code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
	if strings.Contains(r.Stdout, "CLAUDE_CODE_API_KEY_HELPER_TTL_MS="+redactShown) || !strings.Contains(r.Stdout, "set CLAUDE_CODE_API_KEY_HELPER_TTL_MS=") {
		t.Errorf("TTL not shown:\n%s", r.Stdout)
	}
}

// TestRedactEverySecretSource: with --resolved-secrets, every value from
// a token (plain, reference, tokens.jsonc, gopass, --token-ref), env_refs
// and an env file shows as [redacted] in the human and JSON dry run.
func TestRedactEverySecretSource(t *testing.T) {
	f := leakFixture(t)
	cases := []struct {
		args []string
		vars []string
	}{
		{[]string{"--login", "plain"}, []string{"CLAUDE_CODE_OAUTH_TOKEN", "API_KEY", "FILE_SECRET"}},
		{[]string{"--login", "ref"}, []string{"CLAUDE_CODE_OAUTH_TOKEN"}},
		{[]string{"--login", "stored"}, []string{"CLAUDE_CODE_OAUTH_TOKEN"}},
		{[]string{"--login", "gp"}, []string{"ANTHROPIC_API_KEY"}},
		{[]string{"--token-ref", leakOneOff}, []string{"CLAUDE_CODE_OAUTH_TOKEN"}},
		{[]string{"--env", "X=${LEAK_REF}"}, []string{"X"}},
	}
	for _, c := range cases {
		for _, mode := range [][]string{nil, {"--json"}} {
			args := append(append([]string{"run", "--dry-run", "--resolved-secrets"}, c.args...), mode...)
			r := f.Run(args...)
			if r.Code != 0 {
				t.Fatalf("%v: code %d\n%s%s", args, r.Code, r.Stdout, r.Stderr)
			}
			for _, v := range c.vars {
				if !strings.Contains(r.Stdout, "set "+v+"="+redactShown) {
					t.Errorf("%v: %s not redacted:\n%s", args, v, r.Stdout)
				}
			}
			for _, s := range leaks {
				if strings.Contains(r.Stdout+r.Stderr, s) {
					t.Errorf("%v printed a secret (%s…)", args, s[:14])
				}
			}
		}
	}
}

// TestDoctorLiteralSecretHint: agx doctor no longer claims a literal
// profile env value is redacted because its name looks secret; it says
// the value is shown and how to keep a secret redacted.
func TestDoctorLiteralSecretHint(t *testing.T) {
	f := leakFixture(t)
	f.WriteConfig(strings.Replace(leakConfig, `"env_file": "work.env" },`,
		`"env_file": "work.env", "env": { "MY_SERVICE_TOKEN": "literal-value", "ANTHROPIC_API_KEY": "literal-key" } },`, 1))
	r := f.Run("doctor", "--json")
	for _, want := range []string{
		"profiles.work.env.MY_SERVICE_TOKEN looks like a secret written as a literal: shown as written in output; put it in env_refs to keep it redacted",
		"profiles.work.env.ANTHROPIC_API_KEY holds a plain token value (allowed; redacted in all output)",
	} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("doctor lacks %q:\n%s", want, r.Stdout)
		}
	}
}
