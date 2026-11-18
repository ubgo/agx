package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// loginsConfig has profiles of two agents, an agent default, a binding
// and logins of every source; every value is fake.
const loginsConfig = `{
  "version": 1,
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "login": "personal" },
    "cx": { "agent": "codex", "home": "~/.codex" }
  },
  "agents": { "claude": { "defaults": { "login": "personal" } } },
  "projects": { "/p/client": { "claude": { "login": "personal" } } },
  "logins": {
    // a reference
    "personal": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:personal/claude-token}", "account": "me@example.com" },
    "plainval": { "agents": ["claude"], "kind": "oauth-token", "token": "fake-token-PLAINVALUE" },
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "${OSKAR_TOK}" }
  }
}`

// loginFixture is a machine with loginsConfig.
func loginFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.HomeDir(".codex")
	f.WriteConfig(loginsConfig)
	return f
}

// envelope is the part of the JSON envelope the tests read.
type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

// parse decodes a --json stdout.
func parse(t *testing.T, r testkit.Result) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(r.Stdout), &e); err != nil {
		t.Fatalf("not JSON: %v\nstdout %s\nstderr %s", err, r.Stdout, r.Stderr)
	}
	return e
}

// wantFail asserts the exit code and error code (and a message part).
func wantFail(t *testing.T, r testkit.Result, exit int, code, msg string) {
	t.Helper()
	e := parse(t, r)
	if r.Code != exit || e.Error == nil || e.Error.Code != code || !strings.Contains(e.Error.Message, msg) {
		t.Fatalf("exit %d (want %d), error %+v (want %s %q)", r.Code, exit, e.Error, code, msg)
	}
}

// noSecret fails when a fake value appears in any output.
func noSecret(t *testing.T, r testkit.Result, values ...string) {
	t.Helper()
	for _, v := range values {
		if strings.Contains(r.Stdout+r.Stderr, v) {
			t.Errorf("value %q printed:\n%s\n%s", v, r.Stdout, r.Stderr)
		}
	}
}

// TestLoginAddSourcesAndList: every source writes the right config shape,
// pasted values go to tokens.jsonc 0600, and list/show never print values.
func TestLoginAddSourcesAndList(t *testing.T) {
	f := loginFixture(t)
	r := f.RunIn(f.Work, "fake-token-PASTED\n", "login", "add", "--name", "pasted", "--agent", "claude", "--stdin", "--json")
	if r.Code != 0 {
		t.Fatalf("stdin add: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	noSecret(t, r, "fake-token-PASTED")
	r = f.Run("login", "add", "--name", "helper", "--agent", "claude", "--helper-command", "gopass show -o work/key", "--account", "ops@example.com")
	if r.Code != 0 || !strings.Contains(r.Stdout, "added login helper (api-key-helper, helper gopass)") {
		t.Fatalf("helper add: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "add", "--name", "key", "--agent", "claude", "--kind", "api-key", "--token-ref", "$(op read op://v/key)")
	if r.Code != 0 {
		t.Fatalf("ref add: %d %s", r.Code, r.Stderr)
	}
	st, err := os.Stat(filepath.Join(f.ConfigDir, "tokens.jsonc"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("tokens.jsonc: %v %v", err, st)
	}
	if !strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "tokens.jsonc")), "fake-token-PASTED") {
		t.Error("pasted value not stored")
	}
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	for _, want := range []string{`"stored": true`, `"command": "gopass show -o work/key"`, `"token": "$(op read op://v/key)"`, "// a reference"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %s:\n%s", want, cfg)
		}
	}
	r = f.Run("login", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("list: %d %s", r.Code, r.Stderr)
	}
	noSecret(t, r, "fake-token-PASTED", "fake-token-PLAINVALUE")
	f.Golden(t, "login-list", r.Stdout)
	r = f.Run("login", "list", "--agent", "codex", "--json", "--field", "data")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != "[]" {
		t.Errorf("list --agent codex: %d %q", r.Code, r.Stdout)
	}
	r = f.Run("login", "show", "--id", "personal", "--json")
	if r.Code != 0 {
		t.Fatalf("show: %d %s", r.Code, r.Stderr)
	}
	f.Golden(t, "login-show", r.Stdout)
	r = f.Run("login", "show", "--id", "personal")
	if r.Code != 0 || !strings.Contains(r.Stdout, "users    profile work; agent-default claude; binding /p/client (claude)") {
		t.Errorf("show text: %s", r.Stdout)
	}
}

// TestLoginListTokensFileModeRefused: a tokens.jsonc wider than 0600 (a
// copied or checked-out file: git keeps no mode) is refused, and login
// list says so instead of calling a token that is there "missing"; login
// check names the mode and fails.
func TestLoginListTokensFileModeRefused(t *testing.T) {
	f := loginFixture(t)
	r := f.RunIn(f.Work, "fake-token-PASTED\n", "login", "add", "--name", "pasted", "--agent", "claude", "--stdin")
	if r.Code != 0 {
		t.Fatalf("stdin add: %d %s", r.Code, r.Stderr)
	}
	tokens := filepath.Join(f.ConfigDir, "tokens.jsonc")
	if err := os.Chmod(tokens, 0o644); err != nil {
		t.Fatal(err)
	}
	r = f.Run("login", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "stored (tokens.jsonc unreadable; agx login check shows why)") || strings.Contains(r.Stdout, "missing from tokens.jsonc") {
		t.Errorf("list with a 0644 tokens.jsonc: %d\n%s", r.Code, r.Stdout)
	}
	r = f.Run("login", "check", "--id", "pasted", "--no-probe")
	if r.Code == 0 || !strings.Contains(r.Stderr, "has mode 0644; it holds tokens and must be 0600") {
		t.Errorf("check: %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
	noSecret(t, r, "fake-token-PASTED")
}

// TestLoginAddRefusals: each refusal has its exit code and error code.
func TestLoginAddRefusals(t *testing.T) {
	f := loginFixture(t)
	wantFail(t, f.Run("login", "add", "--name", "cx", "--agent", "codex", "--token-ref", "x", "--json"), 4, "unsupported", "Codex has no token login")
	wantFail(t, f.Run("login", "add", "--name", "personal", "--agent", "claude", "--token-ref", "x", "--json"), 2, "conflict", "login personal exists")
	wantFail(t, f.Run("login", "add", "--name", "two", "--agent", "claude", "--stdin", "--token-ref", "x", "--json"), 2, "conflict", "--stdin and --token-ref")
	// No terminal: the message names the step that needed one (never a
	// picker, which these steps do not show) and offers only ways that
	// work without one.
	wantFail(t, f.Run("login", "add", "--name", "none", "--agent", "claude", "--json"), 2, "no-tty",
		"no terminal: the token is pasted at a hidden prompt; pass --stdin, --token-ref, --helper-command")
	wantFail(t, f.Run("login", "add", "--name", "setup", "--agent", "claude", "--login-setup", "--json"), 2, "no-tty",
		"no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; pipe the token with --stdin")
	wantFail(t, f.Run("login", "add", "--name", "bad name", "--agent", "claude", "--token-ref", "x", "--json"), 2, "usage", "--name")
	wantFail(t, f.Run("login", "add", "--name", "k", "--agent", "claude", "--kind", "oauth-token", "--helper-command", "x", "--json"), 2, "conflict", "--helper-command")
	wantFail(t, f.Run("login", "add", "--name", "g", "--agent", "gemini", "--kind", "oauth-token", "--token-ref", "x", "--json"), 4, "unsupported", "gemini has no oauth-token login")
	wantFail(t, f.Run("login", "add", "--name", "pi1", "--agent", "pi", "--token-ref", "x", "--json"), 2, "usage",
		"pi reads a different variable per provider; pass --provider: one of anthropic, ")
	// A provider that was given but is unknown is named as unknown, never
	// answered with "pass --provider" as if none was given.
	r := f.Run("login", "add", "--name", "pi2", "--agent", "pi", "--provider", "nope", "--token-ref", "x", "--json")
	wantFail(t, r, 2, "usage", `--provider "nope": pi has no api-key provider by that name; use one of anthropic, `)
	if strings.Contains(r.Stderr, "pass --provider") {
		t.Errorf("unknown provider answered as a missing one: %s", r.Stderr)
	}
	// Edge 17 at paste time: an empty paste is refused and nothing written.
	r = f.RunIn(f.Work, "  \n", "login", "add", "--name", "empty", "--agent", "claude", "--stdin", "--json")
	wantFail(t, r, 1, "secret", "login empty: token is empty")
	r = f.RunIn(f.Work, "fake-a\nfake-b\n", "login", "add", "--name", "multi", "--agent", "claude", "--stdin", "--json")
	wantFail(t, r, 1, "secret", "login multi: token has 2 lines")
	if strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), "empty") {
		t.Error("refused login written")
	}
}

// TestLoginAddDefaultKindDecision: without --kind the first agent's
// default kind is used and shown as a decision with its source.
func TestLoginAddDefaultKindDecision(t *testing.T) {
	f := loginFixture(t)
	r := f.Run("login", "add", "--name", "gem", "--agent", "gemini", "--token-ref", "${GEM_KEY}", "--json", "--field", "decisions")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"api-key"`) || !strings.Contains(r.Stdout, "gemini's default kind") {
		t.Fatalf("%d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "add", "--name", "pikey", "--agent", "pi", "--provider", "anthropic", "--token-ref", "${PI_KEY}")
	if r.Code != 0 {
		t.Fatalf("pi with provider: %d %s", r.Code, r.Stderr)
	}
	// login show's label column fits its widest label ("provider") with
	// two spaces, never one.
	r = f.Run("login", "show", "--id", "pikey")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\nprovider  anthropic\n") || !strings.HasPrefix(r.Stdout, "name      pikey\n") {
		t.Fatalf("login show: %d %q", r.Code, r.Stdout)
	}
}

// TestLoginEdit rotates and changes a login, keeping comments.
func TestLoginEdit(t *testing.T) {
	f := loginFixture(t)
	r := f.RunIn(f.Work, "fake-token-ROTATED\n", "login", "edit", "--id", "personal", "--stdin")
	if r.Code != 0 {
		t.Fatalf("rotate: %d %s", r.Code, r.Stderr)
	}
	noSecret(t, r, "fake-token-ROTATED")
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if !strings.Contains(cfg, `"stored": true`) || strings.Contains(cfg, "gopass:personal") || !strings.Contains(cfg, "// a reference") {
		t.Fatalf("config after rotate:\n%s", cfg)
	}
	// Back to a reference: the stored value leaves tokens.jsonc.
	r = f.Run("login", "edit", "--id", "personal", "--token-ref", "${gopass:new/path}", "--account", "")
	if r.Code != 0 {
		t.Fatalf("to ref: %d %s", r.Code, r.Stderr)
	}
	if strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "tokens.jsonc")), "ROTATED") {
		t.Error("old stored value kept")
	}
	cfg = f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if strings.Contains(cfg, "me@example.com") || strings.Contains(cfg, `"stored"`) {
		t.Errorf("config after edit:\n%s", cfg)
	}
	wantFail(t, f.Run("login", "edit", "--id", "personal", "--json"), 2, "usage", "nothing to change")
	wantFail(t, f.Run("login", "edit", "--id", "nope", "--account", "x", "--json"), 1, "not-found", "unknown login nope")
	// Dropping claude would orphan its users (edge 11 shape).
	wantFail(t, f.Run("login", "edit", "--id", "personal", "--agent", "gemini", "--kind", "api-key", "--json"), 2, "wrong-agent", "profile work uses login personal")
	wantFail(t, f.Run("login", "edit", "--id", "personal", "--kind", "api-key-helper", "--json"), 2, "usage", "needs --helper-command")
	r = f.Run("login", "edit", "--id", "plainval", "--helper-command", "gopass show -o k")
	if r.Code != 0 || !strings.Contains(r.Stdout, "api-key-helper") {
		t.Errorf("to helper: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestLoginRemove refuses while used, and --cascade clears every user.
func TestLoginRemove(t *testing.T) {
	f := loginFixture(t)
	r := f.Run("login", "remove", "--id", "personal", "--json")
	wantFail(t, r, 2, "in-use", "login personal is used by profile work, agent-default claude, binding /p/client (claude)")
	if e := parse(t, r); !strings.Contains(e.Error.Hint, "--cascade") {
		t.Errorf("hint %q", e.Error.Hint)
	}
	f.RunIn(f.Work, "fake-token-X\n", "login", "add", "--name", "gone", "--agent", "claude", "--stdin")
	f.Env["AGX_LOGIN"] = "gone"
	r = f.Run("login", "remove", "--id", "gone")
	if r.Code != 0 || !strings.Contains(r.Stderr, "AGX_LOGIN in this terminal still names gone") {
		t.Errorf("remove gone: %d %s", r.Code, r.Stderr)
	}
	if strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "tokens.jsonc")), `"gone"`) {
		t.Error("stored value kept")
	}
	delete(f.Env, "AGX_LOGIN")
	r = f.Run("login", "remove", "--id", "personal", "--cascade", "--json")
	if r.Code != 0 {
		t.Fatalf("cascade: %d %s", r.Code, r.Stdout)
	}
	f.Golden(t, "login-remove-cascade", r.Stdout)
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	for _, gone := range []string{`"personal"`, `"/p/client"`, `"defaults"`, `"login": "personal"`} {
		if strings.Contains(cfg, gone) {
			t.Errorf("config still has %s:\n%s", gone, cfg)
		}
	}
	if r := f.Run("profile", "list"); strings.Contains(r.Stderr, "config.jsonc:") {
		t.Errorf("config invalid after cascade: %s", r.Stderr)
	}
}

// TestLoginUse sets profile and agent defaults and the home login.
func TestLoginUse(t *testing.T) {
	f := loginFixture(t)
	r := f.Run("login", "use", "--id", "derek", "--profile", "work")
	if r.Code != 0 || !strings.Contains(r.Stdout, "profile work now uses login derek") {
		t.Fatalf("%d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "use", "--home-login", "--profile", "work", "--json")
	if r.Code != 0 {
		t.Fatalf("%d %s", r.Code, r.Stdout)
	}
	f.Golden(t, "login-use-home", r.Stdout)
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if !strings.Contains(cfg, `"home_login": true`) || strings.Contains(cfg, `"login": "derek"`) {
		t.Errorf("profile after --home-login:\n%s", cfg)
	}
	r = f.Run("login", "use", "--id", "derek", "--agent", "claude")
	if r.Code != 0 || !strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), `"login": "derek"`) {
		t.Fatalf("agent default: %d %s", r.Code, r.Stderr)
	}
	wantFail(t, f.Run("login", "use", "--id", "derek", "--home-login", "--terminal", "--json"), 2, "conflict", "pick one")
	wantFail(t, f.Run("login", "use", "--id", "derek", "--profile", "work", "--terminal", "--json"), 2, "conflict", "--profile and --terminal")
	wantFail(t, f.Run("login", "use", "--id", "derek", "--json"), 2, "usage", "--terminal")
	wantFail(t, f.Run("login", "use", "--id", "derek", "--profile", "nope", "--json"), 1, "not-found", "unknown profile nope")
}

// TestEdge11LoginWrongAgent: a login that does not serve the agent is
// refused (exit 2 outside launches), never skipped.
func TestEdge11LoginWrongAgent(t *testing.T) {
	f := loginFixture(t)
	wantFail(t, f.Run("login", "use", "--id", "derek", "--profile", "cx", "--json"), 2, "wrong-agent", "login derek serves claude; profile cx is codex")
	wantFail(t, f.Run("login", "use", "--id", "derek", "--agent", "codex", "--json"), 2, "wrong-agent", "login derek serves claude; --agent is codex")
	wantFail(t, f.Run("login", "show", "--id", "derek", "--agent", "codex", "--json"), 2, "wrong-agent", "login derek serves claude")
}

// TestEdge54TerminalLine: without the shell layer --terminal prints the
// line to copy, exit 0, and writes nothing.
func TestEdge54TerminalLine(t *testing.T) {
	f := loginFixture(t)
	before := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	r := f.Run("login", "use", "--id", "derek", "--terminal")
	if r.Code != 0 || r.Stdout != "export AGX_LOGIN=derek\n" {
		t.Fatalf("%d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "use", "--home-login", "--terminal")
	if r.Code != 0 || r.Stdout != "unset AGX_LOGIN\n" {
		t.Fatalf("%d %q", r.Code, r.Stdout)
	}
	f.Env["AGX_LOGIN_CLAUDE"] = "personal"
	r = f.Run("login", "use", "--id", "derek", "--terminal")
	if r.Code != 0 || !strings.Contains(r.Stderr, "AGX_LOGIN_CLAUDE=personal outranks AGX_LOGIN for claude") {
		t.Errorf("outrank warning: %s", r.Stderr)
	}
	wantFail(t, f.Run("login", "use", "--id", "nope", "--terminal", "--json"), 1, "not-found", "unknown login nope")
	if f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")) != before {
		t.Error("--terminal wrote config")
	}
}

// checkFixture adds logins for each resolution failure, with a fake
// gopass on PATH.
func checkFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	f.WriteConfig(`{
  "version": 1,
  "logins": {
    "multi": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:multi}" },
    "good": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:good}" },
    "empty": { "agents": ["claude"], "kind": "oauth-token", "token": "${EMPTY_TOK:-}" },
    "cmdfail": { "agents": ["claude"], "kind": "oauth-token", "token": "$(leaky --vault work)" },
    "scheme": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopas:x}" },
    "unset": { "agents": ["claude"], "kind": "oauth-token", "token": "${AWS_REGION}" },
    "plainval": { "agents": ["claude"], "kind": "oauth-token", "token": "fake-token-PLAINVALUE" }
  }
}`)
	f.WriteFile(filepath.Join(f.Bin, "gopass"), `#!/bin/sh
case "$*" in
  *multi*) printf 'fake-a\nfake-b\nfake-c\n' ;;
  *) printf 'fake-token-GOPASS\n' ;;
esac
`, 0o755)
	// leaky prints a value on stdout and fails: the message may quote the
	// command and stderr, never stdout (edge 18).
	f.WriteFile(filepath.Join(f.Bin, "leaky"), "#!/bin/sh\nprintf fake-token-STDOUT\necho locked >&2\nexit 1\n", 0o755)
	return f
}

// TestLoginCheckEdges covers edges 17, 18, 19, 20 and 23 through
// login check --no-probe (resolution only, no quota).
func TestLoginCheckEdges(t *testing.T) {
	f := checkFixture(t)
	cases := []struct {
		id, msg string
	}{
		{"multi", "login multi: token has 3 lines"},                                       // edge 17
		{"empty", "login empty: token is empty"},                                          // edge 17
		{"cmdfail", `login cmdfail: command "leaky --vault work" failed: exit 1: locked`}, // edge 18
		{"scheme", "login scheme: no plugin for scheme gopas"},                            // edge 19
		{"unset", "login unset: AWS_REGION is not set"},                                   // edge 20
	}
	for _, c := range cases {
		r := f.Run("login", "check", "--id", c.id, "--no-probe", "--json")
		wantFail(t, r, 1, "secret", c.msg)
		noSecret(t, r, "fake-token-STDOUT", "fake-a")
	}
	// Edge 23: a plain value is allowed, and printed nowhere.
	r := f.Run("login", "check", "--id", "plainval", "--id", "good", "--no-probe", "--json")
	if r.Code != 0 {
		t.Fatalf("plain: %d %s", r.Code, r.Stdout)
	}
	noSecret(t, r, "fake-token-PLAINVALUE", "fake-token-GOPASS")
	f.Golden(t, "login-check-no-probe", r.Stdout)
	r = f.Run("login", "list")
	noSecret(t, r, "fake-token-PLAINVALUE")
	if !strings.Contains(r.Stdout, "plain") {
		t.Errorf("list: %s", r.Stdout)
	}
	// All logins: partial (exit 3) with the rows kept.
	r = f.Run("login", "check", "--no-probe", "--json")
	e := parse(t, r)
	if r.Code != 3 || e.Error == nil || e.Error.Code != "partial" || !strings.Contains(string(e.Data), `"name": "good"`) {
		t.Fatalf("all: %d %s", r.Code, r.Stdout)
	}
	r = f.Run("login", "check", "--no-probe")
	if r.Code != 3 || !strings.Contains(r.Stdout, "good") || !strings.Contains(r.Stderr, "5 of 7 logins failed") {
		t.Errorf("human partial: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	wantFail(t, f.Run("login", "check", "--id", "nope", "--json"), 1, "not-found", "unknown login nope")
}
