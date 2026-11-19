package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
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
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work" } },
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
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
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

// wantOneLineError asserts an agx login token failure: the exit code,
// nothing on stdout, and exactly one stderr line (no hint lines) holding
// msg.
func wantOneLineError(t *testing.T, name string, code int, stdout, stderr string, exit int, msg string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if code != exit || stdout != "" || len(lines) != 1 || !strings.HasSuffix(stderr, "\n") || !strings.Contains(stderr, msg) {
		t.Errorf("%s: exit %d (want %d), stdout %q, stderr %q (want one line with %q)", name, code, exit, stdout, stderr, msg)
	}
}

// TestLoginTokenRefusesJSONAndField: the output is the bare value, so
// --json and --field are usage errors that say why in one stderr line,
// with stdout empty and no value printed.
func TestLoginTokenRefusesJSONAndField(t *testing.T) {
	f := tokenFixture(t)
	for _, mode := range [][]string{{"--json"}, {"--field", "data"}, {"--json", "--field", "data"}} {
		r := f.Run(append([]string{"login", "token", "--id", "plain"}, mode...)...)
		wantOneLineError(t, strings.Join(mode, " "), r.Code, r.Stdout, r.Stderr, apperr.ExitUsage, "bare value for scripts and apiKeyHelper")
		noSecret(t, r, tokPlain)
	}
}

// TestLoginTokenErrors: every failure is one stderr line with no hint and
// an empty stdout: an unknown login (not-found, exit 1), an api-key-helper
// (unsupported, exit 4, its command not run), a missing --id (exit 2), and
// an unset reference, which names the variable but no value (exit 1).
func TestLoginTokenErrors(t *testing.T) {
	f := tokenFixture(t)
	cases := []struct {
		name string
		args []string
		exit int
		msg  string
	}{
		{"unknown", []string{"--id", "nope"}, apperr.ExitFailure, "unknown login nope"},
		{"helper", []string{"--id", "helper"}, apperr.ExitUnsupported, "api-key-helper"},
		{"no --id", nil, apperr.ExitUsage, "--id is required"},
		{"unset", []string{"--id", "unset"}, apperr.ExitFailure, "FAKE_UNSET"},
	}
	for _, c := range cases {
		r := f.Run(append([]string{"login", "token"}, c.args...)...)
		wantOneLineError(t, c.name, r.Code, r.Stdout, r.Stderr, c.exit, c.msg)
		noSecret(t, r, tokPlain, tokEnv, tokCmd, tokGopass, tokStored, tokOther)
	}
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

// TestLoginTokenTerminal: stdout on a terminal is refused (exit 2, one
// line naming the pipe and --reveal) before anything resolves; --reveal
// prints.
func TestLoginTokenTerminal(t *testing.T) {
	code, out, errs := runTokenInProcess(t, "login", "token", "--id", "env")
	wantOneLineError(t, "terminal", code, out, errs, apperr.ExitUsage, "stdout is a terminal; pipe it (| pbcopy) or add --reveal")
	if strings.Contains(out+errs, tokEnv) {
		t.Errorf("terminal refusal printed the value")
	}
	code, out, errs = runTokenInProcess(t, "login", "token", "--id", "env", "--reveal")
	if code != 0 || out != tokEnv+"\n" || errs != "" {
		t.Errorf("--reveal: exit %d, stdout %q, stderr %q", code, out, errs)
	}
}

// TestHelperLoginDryRunNamesTheSetting: a dry run on an api-key-helper
// login lists the settings key the helper adds and the file it goes to,
// as a --live-login run does, with the login as its source and never the
// helper command; a run with settings of its own lists both and one file.
func TestHelperLoginDryRunNamesTheSetting(t *testing.T) {
	f := tokenFixture(t)
	r := f.Run("run", "--profile", "work", "--login", "helper", "--dry-run", "--quiet", "--json", "--field", "data.decisions")
	var decs []struct{ Name, Value, Source string }
	if err := json.Unmarshal([]byte(r.Stdout), &decs); err != nil || r.Code != 0 {
		t.Fatalf("exit %d, %v\n%s\n%s", r.Code, err, r.Stdout, r.Stderr)
	}
	var keys, files int
	for _, d := range decs {
		if d.Name == "settings" && d.Value == "apiKeyHelper" && d.Source == "login helper" {
			keys++
		}
		if d.Name == "settings" && strings.HasPrefix(d.Value, "delivered as file ") {
			files++
		}
	}
	if keys != 1 || files != 1 {
		t.Errorf("want one apiKeyHelper row from login helper and one file row, got %d and %d: %+v", keys, files, decs)
	}
	if strings.Contains(r.Stdout+r.Stderr, "printf %s") {
		t.Errorf("the helper command was printed:\n%s", r.Stdout)
	}
	r = f.Run("run", "--profile", "work", "--login", "helper", "--setting", "model=opus", "--dry-run", "--quiet")
	if r.Code != 0 || !strings.Contains(r.Stdout, "apiKeyHelper") || !strings.Contains(r.Stdout, "from login helper") || strings.Count(r.Stdout, "delivered as file") != 1 {
		t.Errorf("with --setting: exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
}
