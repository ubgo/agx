package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// optEnvConfig is a Claude profile with every env source of spec 7.5 and a
// stored-reference login; env_file is relative to the config dir.
const optEnvConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": {
      "agent": "claude", "home": "~/.claude-work", "default": true,
      "env": { "LAYER": "env", "PLAIN": "$NOT_EXPANDED" },
      "env_refs": { "REF_VAR": "${SRCVAR}" },
      "env_file": "envs/work.env"
    },
    "cmds": { "agent": "claude", "home": "~/.claude-work", "env_file": "envs/cmd.env", "env_file_commands": true },
    "nocmds": { "agent": "claude", "home": "~/.claude-work", "env_file": "envs/cmd.env" },
    "personal": { "agent": "claude", "home": "~/.claude-work", "login": "carol" }
  },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" } }
}`

// optEnvFixture is a machine with optEnvConfig, its env files and a fake claude.
func optEnvFixture(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
	f.WriteConfig(optEnvConfig)
	f.WriteFile(filepath.Join(f.ConfigDir, "envs", "work.env"), "LAYER=file\nFILE_ONLY=from-file\nFILE_REF=${SRCVAR}-x\n", 0o644)
	f.WriteFile(filepath.Join(f.ConfigDir, "envs", "cmd.env"), "# commands\nA=1\nCMD_VAR=$(printf from-cmd)\n", 0o644)
	f.Env["SRCVAR"] = "src-value"
	f.Env["TOKVAR"] = "fake-token-ALPHA"
	return f
}

// optAgent runs agx and returns the fake agent's view.
func optAgent(t *testing.T, r testkit.Result) testkit.Agent {
	t.Helper()
	if r.Code != 0 {
		t.Fatalf("code %d\nstdout %s\nstderr %s", r.Code, r.Stdout, r.Stderr)
	}
	a, ok := r.Agent()
	if !ok {
		t.Fatalf("no agent ran: %s", r.Stdout)
	}
	return a
}

// optErrEnvelope is the --json error part.
type optErrEnvelope struct {
	OK    bool `json:"ok"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// optFail asserts exit 125 with error code and a message fragment.
func optFail(t *testing.T, r testkit.Result, code, msg string) {
	t.Helper()
	if r.Code != 125 {
		t.Fatalf("code %d, want 125\nstdout %s\nstderr %s", r.Code, r.Stdout, r.Stderr)
	}
	var env optErrEnvelope
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.Stdout)
	}
	if env.OK || env.Error.Code != code || !strings.Contains(env.Error.Message, msg) {
		t.Fatalf("error %+v, want code %s containing %q", env.Error, code, msg)
	}
}

// TestEnvPrecedence pins spec 7.8: env_file → env → env_refs →
// --env-file → --env → --no-env, profile env literal, references strict.
func TestEnvPrecedence(t *testing.T) {
	f := optEnvFixture(t)
	f.WriteFile(filepath.Join(f.Work, "local.env"), "LAYER=cli-file\nCLI_FILE=yes\n", 0o644)
	f.Env["DROPME"] = "x"
	a := optAgent(t, f.Run("run", "--env-file", "local.env", "--env", "FLAG_REF=${SRCVAR}", "--env", "PLAIN_FLAG=a=b", "--no-env", "DROPME"))
	want := map[string]string{
		"LAYER": "cli-file", "FILE_ONLY": "from-file", "FILE_REF": "src-value-x", "PLAIN": "$NOT_EXPANDED",
		"REF_VAR": "src-value", "CLI_FILE": "yes", "FLAG_REF": "src-value", "PLAIN_FLAG": "a=b",
	}
	for k, v := range want {
		if a.Env[k] != v {
			t.Errorf("%s = %q, want %q", k, a.Env[k], v)
		}
	}
	if _, ok := a.Env["DROPME"]; ok {
		t.Errorf("--no-env DROPME kept")
	}
	a = optAgent(t, f.Run("run", "--env", "LAYER=flag"))
	if a.Env["LAYER"] != "flag" {
		t.Errorf("--env does not beat the profile: %q", a.Env["LAYER"])
	}
}

// TestEnvDryRunResolvesNothing: a dry run checks references but shows a
// placeholder, runs no command, and names each source.
func TestEnvDryRunResolvesNothing(t *testing.T) {
	f := optEnvFixture(t)
	marker := filepath.Join(f.Root, "ran")
	r := f.Run("run", "--profile", "cmds", "--env", "X=$(touch "+marker+")", "--dry-run")
	if r.Code != 0 {
		t.Fatalf("code %d: %s %s", r.Code, r.Stdout, r.Stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a dry run ran a command")
	}
	if !strings.Contains(r.Stdout, "X=[redacted]") || strings.Contains(r.Stdout, "from-cmd") {
		t.Errorf("dry run env: %s", r.Stdout)
	}
	if !strings.Contains(r.Stdout, "profile cmds env_file envs/cmd.env") || !strings.Contains(r.Stdout, "A, CMD_VAR") {
		t.Errorf("env source decision missing: %s", r.Stdout)
	}
	// Resolved values are secrets (they came from a reference): the
	// resolved run succeeds, still shows them redacted, never the value.
	r = f.Run("run", "--profile", "cmds", "--dry-run", "--resolved-secrets")
	if r.Code != 0 || !strings.Contains(r.Stdout, "CMD_VAR=[redacted]") || strings.Contains(r.Stdout, "from-cmd") {
		t.Errorf("--resolved-secrets: code %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestEnvFileCommands: env_file_commands true runs $(…) in the env file.
func TestEnvFileCommands(t *testing.T) {
	f := optEnvFixture(t)
	a := optAgent(t, f.Run("run", "--profile", "cmds"))
	if a.Env["CMD_VAR"] != "from-cmd" || a.Env["A"] != "1" {
		t.Errorf("env %v", a.Env)
	}
}

// TestEdge32bEnvFileCommandOff: $(…) in an env file with commands off is
// refused with the file's line:col, in a dry run too.
func TestEdge32bEnvFileCommandOff(t *testing.T) {
	f := optEnvFixture(t)
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		r := f.Run(append([]string{"run", "--profile", "nocmds", "--json"}, extra...)...)
		optFail(t, r, "secret", "envs/cmd.env:3:9: $(printf from-cmd) is a command, and env_file_commands is off")
	}
}

// TestEdge19EnvUnknownScheme: an unknown scheme in --env is refused.
func TestEdge19EnvUnknownScheme(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env", "API_KEY=${gopas:x}", "--json"), "secret", "--env API_KEY: no plugin for scheme gopas")
	optFail(t, f.Run("run", "--env", "API_KEY=${gopas:x}", "--json", "--dry-run"), "secret", "no plugin for scheme gopas")
}

// TestEdge20EnvUnsetVar: an unset $VAR in --env is refused (strict).
func TestEdge20EnvUnsetVar(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env", "REGION=${AWS_REGION}", "--json"), "secret", "--env REGION: AWS_REGION is not set")
	optFail(t, f.Run("run", "--profile", "work", "--env", "REGION=${AWS_REGION}", "--json", "--dry-run"), "secret", "AWS_REGION is not set")
}

// TestEdge18EnvCommandFails: a failing $(…) names the command, never stdout.
func TestEdge18EnvCommandFails(t *testing.T) {
	f := optEnvFixture(t)
	r := f.Run("run", "--env", "K=$(echo SECRETOUT; exit 3)", "--json")
	optFail(t, r, "secret", "exit 3")
	if strings.Contains(r.Stdout+r.Stderr, "SECRETOUT\n") || strings.Contains(r.Stdout, `"SECRETOUT`) {
		t.Errorf("stdout leaked: %s", r.Stdout)
	}
}

// TestEdge21EnvHomeVar: --env of the home variable is refused.
func TestEdge21EnvHomeVar(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env", "CLAUDE_CONFIG_DIR=/x", "--json"), "usage", "CLAUDE_CONFIG_DIR is set by the profile's home; use --profile")
}

// TestEdge22EnvTokenWithLogin: --env of the token variable together with a
// token login is a conflict naming both; with the home login it is kept.
func TestEdge22EnvTokenWithLogin(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env", "CLAUDE_CODE_OAUTH_TOKEN=fake-token-BETA", "--login", "carol", "--json"),
		"conflict", "--env CLAUDE_CODE_OAUTH_TOKEN and --login carol both set the credential; pick one")
	optFail(t, f.Run("run", "--profile", "personal", "--env", "ANTHROPIC_API_KEY=fake-token-BETA", "--json"),
		"conflict", "login carol (from profile personal)")
	a := optAgent(t, f.Run("run", "--env", "CLAUDE_CODE_OAUTH_TOKEN=fake-token-BETA"))
	if a.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "fake-token-BETA" {
		t.Errorf("home login dropped a deliberate credential")
	}
}

// TestEnvFileMissing: a missing --env-file is refused.
func TestEnvFileMissing(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env-file", "nope.env", "--json"), "not-found", "env file nope.env not found")
}

// TestEnvBadFlag: --env without "=" is a usage error.
func TestEnvBadFlag(t *testing.T) {
	f := optEnvFixture(t)
	optFail(t, f.Run("run", "--env", "NOEQUALS", "--json"), "usage", "use NAME=value")
}
