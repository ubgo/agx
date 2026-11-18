package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/cli"
	"github.com/ubgo/agx/internal/testkit"
)

// Fake values for agx login token, one per source; none is a real token.
const (
	tokPlain  = "fake-token-123"
	tokEnv    = "fake-token-ENV"
	tokCmd    = "fake-token-CMD"
	tokGopass = "fake-token-GOPASS"
	tokStored = "fake-token-STORED"
	// tokOther is set in an unrelated variable: an error about an unset
	// reference must not quote it, nor anything else from the env.
	tokOther = "fake-token-OTHER"
)

// tokenConfig has one login per source, an api-key-helper and a
// reference to a variable that is never set.
const tokenConfig = `{
  "version": 1,
  "logins": {
    "plain": { "agents": ["claude"], "kind": "oauth-token", "token": "` + tokPlain + `" },
    "env": { "agents": ["claude"], "kind": "api-key", "token": "${FAKE_TOK}" },
    "cmd": { "agents": ["claude"], "kind": "api-key", "token": "$(printf %s ` + tokCmd + `)" },
    "gp": { "agents": ["claude"], "kind": "api-key", "token": "${gopass:x/y}" },
    "stored": { "agents": ["claude"], "kind": "oauth-token", "stored": true },
    "helper": { "agents": ["claude"], "kind": "api-key-helper", "command": "printf %s ` + tokPlain + `" },
    "unset": { "agents": ["claude"], "kind": "api-key", "token": "${FAKE_UNSET}" }
  }
}`

// tokenFixture is a machine with tokenConfig, a fake gopass, a stored
// value and the env variables the references read.
func tokenFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	f.WriteConfig(tokenConfig)
	f.WriteFile(f.Bin+"/gopass", "#!/bin/sh\necho "+tokGopass+"\n", 0o755)
	f.WriteFile(f.ConfigDir+"/tokens.jsonc", `{"tokens":{"stored":"`+tokStored+`"}}`, 0o600)
	f.Env["FAKE_TOK"] = tokEnv
	f.Env["FAKE_OTHER"] = tokOther
	return f
}

// TestLoginTokenPrintsOnlyTheValue: every source prints exactly the
// value and one newline on stdout (a pipe), and nothing on stderr.
func TestLoginTokenPrintsOnlyTheValue(t *testing.T) {
	f := tokenFixture(t)
	for id, want := range map[string]string{"plain": tokPlain, "env": tokEnv, "cmd": tokCmd, "gp": tokGopass, "stored": tokStored} {
		r := f.Run("login", "token", "--id", id)
		if r.Code != 0 || r.Stdout != want+"\n" || r.Stderr != "" {
			t.Errorf("login %s: exit %d, stdout %q (want %q), stderr %q", id, r.Code, r.Stdout, want+"\n", r.Stderr)
		}
	}
}

// TestLoginTokenRefusesJSONAndField: the output is the bare value, so
// --json and --field are usage errors that say why, and print no value.
func TestLoginTokenRefusesJSONAndField(t *testing.T) {
	f := tokenFixture(t)
	for _, mode := range [][]string{{"--json"}, {"--field", "data"}, {"--json", "--field", "data"}} {
		r := f.Run(append([]string{"login", "token", "--id", "plain"}, mode...)...)
		if r.Code != apperr.ExitUsage || !strings.Contains(r.Stdout+r.Stderr, "bare value for scripts and apiKeyHelper") {
			t.Errorf("%v: exit %d (want %d)\nstdout %s\nstderr %s", mode, r.Code, apperr.ExitUsage, r.Stdout, r.Stderr)
		}
		noSecret(t, r, tokPlain)
	}
}

// TestLoginTokenErrors: an unknown login is not-found, an api-key-helper
// is unsupported without running its command, a missing --id is a usage
// error, and an unset reference names the variable but no value.
func TestLoginTokenErrors(t *testing.T) {
	f := tokenFixture(t)
	r := f.Run("login", "token", "--id", "nope")
	if r.Code != apperr.ExitFailure || !strings.Contains(r.Stderr, "unknown login nope") || r.Stdout != "" {
		t.Errorf("unknown: exit %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "token", "--id", "helper")
	if r.Code != apperr.ExitUnsupported || !strings.Contains(r.Stderr, "api-key-helper") || r.Stdout != "" {
		t.Errorf("helper: exit %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	noSecret(t, r, tokPlain)
	r = f.Run("login", "token")
	if r.Code != apperr.ExitUsage || !strings.Contains(r.Stderr, "--id is required") {
		t.Errorf("no --id: exit %d, stderr %q", r.Code, r.Stderr)
	}
	r = f.Run("login", "token", "--id", "unset")
	if r.Code == 0 || r.Stdout != "" || !strings.Contains(r.Stderr, "FAKE_UNSET") {
		t.Errorf("unset: exit %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	noSecret(t, r, tokPlain, tokEnv, tokCmd, tokGopass, tokStored, tokOther)
}

// runTokenInProcess runs agx login token in-process with stdout reported
// as a terminal (the seam SystemEnv fills from term.IsTerminal), which a
// subprocess test cannot fake.
func runTokenInProcess(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".agx")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.jsonc"), []byte(tokenConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"AGX_CONFIG_DIR": cfgDir, "HOME": dir, "FAKE_TOK": tokEnv}
	var out, errb bytes.Buffer
	code := cli.Main(context.Background(), cli.Env{
		Args: args, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
		Host: agentio.Host{UserHome: dir, Env: []string{"HOME=" + dir}},
		Lookup: func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		},
		Cwd: dir, StdoutTTY: true, StdinFD: -1, Self: cli.Name,
	})
	return code, out.String(), errb.String()
}

// TestLoginTokenTerminal: stdout on a terminal is refused (exit 2, with
// the pipe and --reveal hint) before anything resolves; --reveal prints.
func TestLoginTokenTerminal(t *testing.T) {
	code, out, errs := runTokenInProcess(t, "login", "token", "--id", "env")
	if code != apperr.ExitUsage || out != "" || !strings.Contains(errs, "will not print a token to a terminal") || !strings.Contains(errs, "--reveal") {
		t.Errorf("terminal: exit %d, stdout %q, stderr %q", code, out, errs)
	}
	if strings.Contains(out+errs, tokEnv) {
		t.Errorf("terminal refusal printed the value")
	}
	code, out, errs = runTokenInProcess(t, "login", "token", "--id", "env", "--reveal")
	if code != 0 || out != tokEnv+"\n" || errs != "" {
		t.Errorf("--reveal: exit %d, stdout %q, stderr %q", code, out, errs)
	}
}
