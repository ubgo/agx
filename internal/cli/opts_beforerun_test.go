package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// beforeRunConfig has a before_run from the config dir (default), one
// from the launch folder, and one with the full environment, all on a
// profile with a token login.
const beforeRunConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true, "login": "carol", "before_run": "scripts/pre.sh" },
    "folder": { "agent": "claude", "home": "~/.claude-work", "before_run": "scripts/setup.sh", "before_run_from": "folder" },
    "full": { "agent": "claude", "home": "~/.claude-work", "login": "carol", "before_run": "scripts/pre.sh", "before_run_env": "full" }
  },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" } }
}`

// preScript records where it ran and whether it saw the token, then
// prints to stdout (never shown) and exits with $PRE_EXIT.
const preScript = `#!/bin/sh
pwd -P > "$MARK"
if [ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]; then echo token >> "$MARK"; else echo notoken >> "$MARK"; fi
echo STDOUT-SECRET-LINE
echo "setup broke" >&2
exit ${PRE_EXIT:-0}
`

// beforeRunFixture is a machine with beforeRunConfig and its script.
func beforeRunFixture(t *testing.T) *testkit.Fixture {
	if runtime.GOOS == "windows" {
		t.Skip("sh scripts")
	}
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.FakeAgent("claude")
	f.WriteConfig(beforeRunConfig)
	f.WriteFile(filepath.Join(f.ConfigDir, "scripts", "pre.sh"), preScript, 0o755)
	f.Env["TOKVAR"] = "fake-token-ALPHA"
	f.Env["MARK"] = filepath.Join(f.Root, "mark")
	return f
}

// TestBeforeRunRunsInFolderWithoutCredentials: the script runs in the
// launch folder before the agent, without the token (safe env).
func TestBeforeRunRunsInFolderWithoutCredentials(t *testing.T) {
	f := beforeRunFixture(t)
	a := optAgent(t, f.Run("run"))
	if a.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "fake-token-ALPHA" {
		t.Errorf("agent lost its token")
	}
	mark := f.ReadFile(f.Env["MARK"])
	if mark != f.Work+"\nnotoken\n" {
		t.Errorf("mark %q", mark)
	}
}

// TestBeforeRunFullEnv: before_run_env full keeps credential variables.
func TestBeforeRunFullEnv(t *testing.T) {
	f := beforeRunFixture(t)
	optAgent(t, f.Run("run", "--profile", "full"))
	if mark := f.ReadFile(f.Env["MARK"]); !strings.HasSuffix(mark, "\ntoken\n") {
		t.Errorf("mark %q", mark)
	}
}

// TestBeforeRunDryRunNeverRuns: --dry-run shows the path and source only.
func TestBeforeRunDryRunNeverRuns(t *testing.T) {
	f := beforeRunFixture(t)
	r := f.Run("run", "--dry-run")
	if r.Code != 0 || !strings.Contains(r.Stdout, "~/.agx/scripts/pre.sh") || !strings.Contains(r.Stdout, "profile work (before_run_from config)") {
		t.Fatalf("code %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	if _, err := os.Stat(f.Env["MARK"]); err == nil {
		t.Error("dry run ran before_run")
	}
}

// TestBeforeRunFlag: --before-run is relative to the current folder and
// beats the profile's.
func TestBeforeRunFlag(t *testing.T) {
	f := beforeRunFixture(t)
	f.WriteFile(filepath.Join(f.Work, "mine.sh"), "#!/bin/sh\necho flag > \"$MARK\"\n", 0o755)
	optAgent(t, f.Run("run", "--before-run", "mine.sh"))
	if mark := f.ReadFile(f.Env["MARK"]); mark != "flag\n" {
		t.Errorf("mark %q", mark)
	}
}

// TestEdge42BeforeRunFolderMissing: before_run_from folder with no script.
func TestEdge42BeforeRunFolderMissing(t *testing.T) {
	f := beforeRunFixture(t)
	r := f.Run("run", "--profile", "folder", "--json")
	optFail(t, r, "before-run-failed", "before_run scripts/setup.sh not found in "+f.Work)
	f.WriteFile(filepath.Join(f.Work, "scripts", "setup.sh"), "#!/bin/sh\necho folder > \"$MARK\"\n", 0o755)
	optAgent(t, f.Run("run", "--profile", "folder"))
	if mark := f.ReadFile(f.Env["MARK"]); mark != "folder\n" {
		t.Errorf("mark %q", mark)
	}
}

// TestEdge43BeforeRunFails: a failing script stops the launch (125) with
// its exit code and stderr, never its stdout; the agent never starts.
func TestEdge43BeforeRunFails(t *testing.T) {
	f := beforeRunFixture(t)
	f.Env["PRE_EXIT"] = "3"
	r := f.Run("run", "--json")
	optFail(t, r, "before-run-failed", "before_run failed (exit 3): setup broke")
	if strings.Contains(r.Stdout+r.Stderr, "STDOUT-SECRET-LINE") {
		t.Error("script stdout leaked")
	}
	if _, ran := r.Agent(); ran {
		t.Error("agent started after a failed before_run")
	}
}
