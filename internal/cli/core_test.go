package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// workConfig is a one-profile Claude config with a reference login.
const workConfig = `{
  "version": 1,
  "default_agent": "claude",
  // agent defaults
  "agents": { "claude": { "defaults": { "args": ["--chrome"] } } },
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true, "env": { "DEBUG": "1" } }
  },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" } }
}`

// fixtureWork is a machine with the work profile and a fake claude.
func fixtureWork(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
	f.WriteConfig(workConfig)
	f.Env["TOKVAR"] = "fake-oauth-SECRETVALUE"
	return f
}

// TestBareAgxPrintsHelp is edge 1.
func TestEdge01BareAgxPrintsHelp(t *testing.T) {
	r := testkit.New(t).Run()
	if r.Code != 0 || !strings.Contains(r.Stdout, "Usage:") {
		t.Fatalf("code %d, stdout %q", r.Code, r.Stdout)
	}
}

// TestBareWordIsUsageError is spec 5.1 rule 1.
func TestBareWordIsUsageError(t *testing.T) {
	f := fixtureWork(t)
	r := f.Run("profile", "list", "work")
	if r.Code != 2 && r.Code != 1 {
		t.Fatalf("code %d", r.Code)
	}
}

// TestNoConfigRefusesLaunch is edge 2.
func TestEdge02NoConfigRefusesLaunch(t *testing.T) {
	f := testkit.New(t)
	f.FakeAgent("claude")
	r := f.Run("run", "--json")
	if r.Code != 125 {
		t.Fatalf("code %d, want 125", r.Code)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatal(err, r.Stdout)
	}
	if env.OK || env.Error.Code != "no-profiles" || !strings.Contains(env.Error.Hint, "agx config init") {
		t.Fatalf("envelope %+v", env)
	}
}

// TestProfileListLoginFollowsAgentDefaults: LOGIN is what a plain run of
// the profile uses, so a profile without a login of its own shows its
// agent's defaults login (as the dry run does), not "home login".
func TestProfileListLoginFollowsAgentDefaults(t *testing.T) {
	f := fixtureWork(t)
	f.WriteConfig(strings.Replace(workConfig, `"defaults": { "args": ["--chrome"] }`, `"defaults": { "args": ["--chrome"], "login": "carol" }`, 1))
	r := f.Run("profile", "list", "--json", "--field", "data.*.login")
	if r.Code != 0 || strings.TrimSpace(r.Stdout) != `["carol"]` {
		t.Fatalf("code %d, logins %s %s", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("run", "--profile", "work", "--dry-run", "--json", "--field", "data.login")
	if strings.TrimSpace(r.Stdout) != "carol" {
		t.Fatalf("dry run login %q %s", r.Stdout, r.Stderr)
	}
}

// TestFieldUnknownKeyOnTable: an unknown data.*.KEY on the human table is
// the same "no such path" error as with --json (exit 2), never a column
// of dashes; a known key still makes the column.
func TestFieldUnknownKeyOnTable(t *testing.T) {
	f := fixtureWork(t)
	for _, mode := range [][]string{nil, {"--json"}} {
		r := f.Run(append([]string{"profile", "list", "--field", "data.*.nosuch"}, mode...)...)
		if r.Code != 2 || !strings.Contains(r.Stderr, "--field data.*.nosuch: no such path") || r.Stdout != "" {
			t.Errorf("%v: code %d, stdout %q, stderr %q", mode, r.Code, r.Stdout, r.Stderr)
		}
	}
	if r := f.Run("profile", "list", "--field", "data.*.name"); r.Code != 0 || !strings.Contains(r.Stdout, "NAME") || !strings.Contains(r.Stdout, "work") {
		t.Errorf("known key: code %d %q", r.Code, r.Stdout)
	}
}

// TestConfigCheckCountsInNumber: config check's summary puts each noun in
// the number of its count ("1 profile", "0 aliases").
func TestConfigCheckCountsInNumber(t *testing.T) {
	r := fixtureWork(t).Run("config", "check")
	if r.Code != 0 || !strings.Contains(r.Stdout, "1 profile, 1 login, 0 projects, 0 aliases)") {
		t.Fatalf("code %d, summary %q", r.Code, r.Stdout)
	}
}

// TestFieldOnFailureKeepsRealError: a failed command has null data, so a
// --field data path is missing; the command's own error and exit code must
// win over a "no such path" usage error (exit 2), in both output modes.
// A path that exists on failure (error.code) still prints.
func TestFieldOnFailureKeepsRealError(t *testing.T) {
	f := testkit.New(t)
	f.FakeAgent("claude")
	for _, mode := range [][]string{nil, {"--json"}} {
		args := append([]string{"run", "--dry-run", "--field", "data.command.line"}, mode...)
		r := f.Run(args...)
		if r.Code != 125 || strings.Contains(r.Stderr, "no such path") || !strings.Contains(r.Stderr, "agx: ") || r.Stdout != "" {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", mode, r.Code, r.Stdout, r.Stderr)
		}
	}
	r := f.Run("run", "--dry-run", "--json", "--field", "error.code")
	if r.Code != 125 || strings.TrimSpace(r.Stdout) != "no-profiles" {
		t.Fatalf("error.code: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	// A failure whose exit code is not a usage-class one (127: the agent
	// binary is missing) proves the code is the command's, not the path's.
	g := testkit.New(t)
	g.HomeDir(".claude-work")
	g.WriteConfig(workConfig)
	r = g.Run("run", "--dry-run", "--json", "--field", "data.command.line")
	if r.Code != 127 || !strings.Contains(r.Stderr, "not found on PATH") {
		t.Fatalf("missing binary: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
}

// TestRunExecsAgentWithTokenInEnvOnly checks the launch path end to end:
// exec replaces agx, the token is in the env and never in argv, the home
// variable is set, raw args come last, AGX_RUN_* are set.
func TestRunExecsAgentWithTokenInEnvOnly(t *testing.T) {
	f := fixtureWork(t)
	r := f.Run("run", "--login", "carol", "--model", "opus", "--", "--verbose")
	if r.Code != 0 {
		t.Fatalf("code %d stderr %s", r.Code, r.Stderr)
	}
	a, ok := r.Agent()
	if !ok {
		t.Fatalf("no agent ran: %s", r.Stdout)
	}
	want := []string{"--model", "opus", "--chrome", "--verbose"}
	if strings.Join(a.Argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv %q, want %q", a.Argv, want)
	}
	if a.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "fake-oauth-SECRETVALUE" {
		t.Errorf("token not in env")
	}
	if a.Env["CLAUDE_CONFIG_DIR"] != filepath.Join(f.Home, ".claude-work") {
		t.Errorf("CLAUDE_CONFIG_DIR %q", a.Env["CLAUDE_CONFIG_DIR"])
	}
	if a.Env["AGX_RUN_ID"] == "" || a.Env["AGX_RUN_PROFILE"] != "work" || a.Env["DEBUG"] != "1" {
		t.Errorf("run env %v", a.Env)
	}
	if strings.Contains(r.Stderr, "SECRETVALUE") {
		t.Errorf("token printed on stderr")
	}
}

// TestDryRunRedactsAndResolvesNothing: --dry-run never resolves a
// reference and never prints a token.
func TestDryRunRedactsAndResolvesNothing(t *testing.T) {
	f := fixtureWork(t)
	f.Env["TOKVAR"] = ""
	r := f.Run("run", "--login", "carol", "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d stderr %s stdout %s", r.Code, r.Stderr, r.Stdout)
	}
	if !strings.Contains(r.Stdout, "not resolved: --dry-run") || !strings.Contains(r.Stdout, "[redacted]") {
		t.Errorf("dry run output: %s", r.Stdout)
	}
	f.Env["TOKVAR"] = "fake-oauth-SECRETVALUE"
	r = f.Run("run", "--login", "carol", "--dry-run", "--resolved-secrets")
	if r.Code != 0 || !strings.Contains(r.Stdout, "(resolved)") || strings.Contains(r.Stdout+r.Stderr, "SECRETVALUE") {
		t.Errorf("resolved dry run: code %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestDryRunJSONGolden pins the launch plan's --json shape.
func TestDryRunJSONGolden(t *testing.T) {
	f := fixtureWork(t)
	r := f.Run("run", "--login", "carol", "--model", "opus", "--dry-run", "--json", "--", "--verbose")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "run-dry-run", r.Stdout)
}

// TestRunEnvValuesAreMachineValues: every AGX_RUN_* value is one word
// (FIX1 item 2): the home login is "home", a one-off token "token-ref".
func TestRunEnvValuesAreMachineValues(t *testing.T) {
	f := fixtureWork(t)
	for args, want := range map[string]string{"--home-login": "home", "--login carol": "carol", "--token-ref ${TOKVAR}": "token-ref"} {
		r := f.Run(append([]string{"run"}, strings.Fields(args)...)...)
		a, ok := r.Agent()
		if !ok {
			t.Fatalf("%s: no agent: %s", args, r.Stderr)
		}
		if a.Env["AGX_RUN_LOGIN"] != want {
			t.Errorf("%s: AGX_RUN_LOGIN=%q, want %q", args, a.Env["AGX_RUN_LOGIN"], want)
		}
		for k, v := range a.Env {
			if strings.HasPrefix(k, "AGX_RUN_") && strings.ContainsAny(v, " \t") {
				t.Errorf("%s: %s=%q holds whitespace", args, k, v)
			}
		}
	}
}
