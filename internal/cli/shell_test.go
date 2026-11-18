package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/testkit"
)

// shellConfig is workConfig plus aliases (one the scripts must skip).
const shellConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true },
    "other": { "agent": "claude", "home": "~/.claude-other" }
  },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" } },
  "aliases": { "cw": "run --profile work" }
}`

// fakeToken is the token TOKVAR resolves to; it must never reach a file.
const fakeToken = "fake-token-ALPHA"

// fixtureShell is a machine with two profiles, a reference login and a
// fake claude.
func fixtureShell(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.HomeDir(".claude-other")
	f.FakeAgent("claude")
	f.WriteConfig(shellConfig)
	f.Env["TOKVAR"] = fakeToken
	return f
}

// planEnvelope is the part of the JSON planEnvelope the tests read.
type planEnvelope struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decode parses a --json planEnvelope.
func decodePlanEnvelope(t *testing.T, out string) planEnvelope {
	t.Helper()
	var e planEnvelope
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return e
}

func TestShellInitNeedsShell(t *testing.T) {
	f := fixtureShell(t)
	for _, args := range [][]string{{"shell-init"}, {"shell-init", "--shell", "csh"}, {"shell-init", "bash"}, {"completion"}, {"completion", "--shell", "powershell"}} {
		if r := f.Run(args...); r.Code != 2 {
			t.Errorf("%v: code %d, want 2 (%s)", args, r.Code, r.Stderr)
		}
	}
}

// TestShellInitAliasesAndBrokenConfig: valid aliases become functions; a
// bad alias now fails the config load (spec 15.3 cross-check, tested in
// alias_test and shellinit's CheckAlias unit test), and an unreadable
// config still lets the layer load with a warning.
func TestShellInitAliasesAndBrokenConfig(t *testing.T) {
	f := fixtureShell(t)
	r := f.Run("shell-init", "--shell", "bash")
	if r.Code != 0 || !strings.Contains(r.Stdout, "cw() { agx run --profile work \"$@\"; }") {
		t.Fatalf("code %d stdout %s", r.Code, r.Stdout)
	}
	f.WriteConfig(`{ "version": 1, `)
	if r := f.Run("shell-init", "--shell", "zsh"); r.Code != 0 || !strings.Contains(r.Stdout, "agx()") || !strings.Contains(r.Stderr, "config not loaded") {
		t.Errorf("a broken config must not break the layer: code %d %s", r.Code, r.Stderr)
	}
}

// TestShellInitHeaderKeepsConfig: with --config the header's load line
// repeats it, so pasting the line loads the same config (aliases and all).
func TestShellInitHeaderKeepsConfig(t *testing.T) {
	f := testkit.New(t)
	cfg := f.WriteFile(filepath.Join(f.Root, "alt", "config.jsonc"), `{"version": 1, "aliases": {"cw": "run --profile work"}}`, 0o644)
	r := f.Run("--config", cfg, "shell-init", "--shell", "zsh")
	want := `# Load it with: eval "$(agx shell-init --shell zsh --config ` + cfg + `)"`
	if r.Code != 0 || !strings.Contains(r.Stdout, want) || !strings.Contains(r.Stdout, "cw()") {
		t.Fatalf("code %d, stderr %s, stdout head %q", r.Code, r.Stderr, strings.SplitN(r.Stdout, "\n", 3)[:2])
	}
}

func TestShellInitJSONGolden(t *testing.T) {
	f := fixtureShell(t)
	r := f.Run("shell-init", "--shell", "bash", "--osc7", "--json", "--field", "data.shell", "--field", "data.osc7", "--field", "data.aliases", "--field", "warnings")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "shell-init", r.Stdout)
}

func TestCompletionEveryShell(t *testing.T) {
	f := fixtureShell(t)
	for _, sh := range []string{"bash", "zsh", "fish", "pwsh"} {
		r := f.Run("completion", "--shell", sh)
		if r.Code != 0 || !strings.Contains(r.Stdout, "agx") || len(r.Stdout) < 200 {
			t.Errorf("%s: code %d, %d bytes, %s", sh, r.Code, len(r.Stdout), r.Stderr)
		}
	}
	r := f.Run("completion", "--shell", "bash")
	if bash, err := exec.LookPath("bash"); err == nil {
		file := f.WriteFile("completion.bash", r.Stdout, 0o600)
		if out, err := exec.Command(bash, "-n", file).CombinedOutput(); err != nil {
			t.Errorf("bash completion syntax: %v %s", err, out)
		}
	}
}

// planFile is a fresh empty plan path, as the wrapper's mktemp makes it.
func planFile(t *testing.T, f *testkit.Fixture) string {
	t.Helper()
	return f.WriteFile(filepath.Join("tmp", "agx-plan.test"), "", 0o600)
}

// TestPlanFileHoldsNoSecretAndLaunches drives the protocol by hand: new
// --plan-file writes a plan (no token, no run folder), plan show prints
// the folder, launch execs the agent there and deletes the plan, and the
// run folder records plan.jsonc without the options.
func TestPlanFileHoldsNoSecretAndLaunches(t *testing.T) {
	f := fixtureShell(t)
	pf := planFile(t, f)
	r := f.Run("new", "--plan-file", pf, "--name", "my task", "--login", "carol", "--model", "opus", "--", "--verbose")
	if r.Code != 0 || r.Stdout != "" {
		t.Fatalf("plan: code %d stdout %q stderr %s", r.Code, r.Stdout, r.Stderr)
	}
	body := f.ReadFile(pf)
	if strings.Contains(body, fakeToken) || !strings.Contains(body, `"schema_version": 1`) {
		t.Fatalf("plan file: %s", body)
	}
	if st, err := os.Stat(pf); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("plan mode: %v %v", st.Mode(), err)
	}
	if runs, _ := os.ReadDir(filepath.Join(f.ConfigDir, "runs")); len(runs) != 0 {
		t.Fatalf("the plan phase created %d run folders", len(runs))
	}
	dir := f.Run("plan", "show", "--plan-file", pf, "--field", "data.dir")
	session := strings.TrimSpace(dir.Stdout)
	if dir.Code != 0 || !strings.HasPrefix(session, filepath.Join(f.ConfigDir, "sessions", "my_task_")) {
		t.Fatalf("plan show: code %d %q %s", dir.Code, dir.Stdout, dir.Stderr)
	}
	if st, err := os.Stat(session); err != nil || !st.IsDir() {
		t.Fatalf("session folder not created: %v", err)
	}

	r = f.RunIn(session, "", "launch", "--plan-file", pf)
	a, ok := r.Agent()
	if r.Code != 0 || !ok {
		t.Fatalf("launch: code %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	if a.Pwd != session || a.Env["CLAUDE_CODE_OAUTH_TOKEN"] != fakeToken || strings.Join(a.Argv, " ") != "--model opus --verbose" {
		t.Errorf("agent: pwd %s argv %q", a.Pwd, a.Argv)
	}
	if _, err := os.Stat(pf); !os.IsNotExist(err) {
		t.Errorf("plan file not removed: %v", err)
	}
	rec := f.ReadFile(filepath.Join(f.ConfigDir, "runs", a.Env["AGX_RUN_ID"], "plan.jsonc"))
	if strings.Contains(rec, fakeToken) || strings.Contains(rec, "options") || !strings.Contains(rec, `"mode": "new"`) {
		t.Errorf("run folder plan.jsonc: %s", rec)
	}
	if r := f.RunIn(session, "", "launch", "--plan-file", pf); r.Code != 125 {
		t.Errorf("second launch from one plan: code %d", r.Code)
	}
}

func TestPlanShowJSONGolden(t *testing.T) {
	f := fixtureShell(t)
	pf := planFile(t, f)
	if r := f.Run("run", "--plan-file", pf, "--profile", "other", "--home-login"); r.Code != 0 {
		t.Fatalf("plan: %d %s", r.Code, r.Stderr)
	}
	r := f.Run("plan", "show", "--plan-file", pf, "--json")
	if r.Code != 0 {
		t.Fatalf("show: %d %s", r.Code, r.Stderr)
	}
	f.Golden(t, "plan-show", r.Stdout)
}

// TestDryRunWithPlanFileWritesNothing: --dry-run wins; the wrapper sees an
// empty plan and stops.
func TestDryRunWithPlanFileWritesNothing(t *testing.T) {
	f := fixtureShell(t)
	pf := planFile(t, f)
	r := f.Run("new", "--plan-file", pf, "--dry-run")
	if r.Code != 0 || f.ReadFile(pf) != "" || !strings.Contains(r.Stdout, "command") {
		t.Fatalf("code %d plan %q stdout %s", r.Code, f.ReadFile(pf), r.Stdout)
	}
}

// ageCreatedAt rewrites the plan's created_at to d ago (mode kept).
func ageCreatedAt(t *testing.T, f *testkit.Fixture, pf string, d time.Duration) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.ReadFile(pf)), &m); err != nil {
		t.Fatal(err)
	}
	ts, err := json.Marshal(time.Now().Add(-d))
	if err != nil {
		t.Fatal(err)
	}
	m["created_at"] = ts
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pf, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestEdge53PlanStaleOrNotYours: a plan older than 60 s, or writable by
// others, is refused by launch (125) and plan show (1), and starts nothing.
func TestEdge53PlanStaleOrNotYours(t *testing.T) {
	const msg = "plan file is older than 60s or not yours"
	f := fixtureShell(t)
	pf := planFile(t, f)
	if r := f.Run("run", "--plan-file", pf); r.Code != 0 {
		t.Fatalf("plan: %d %s", r.Code, r.Stderr)
	}
	ageCreatedAt(t, f, pf, 61*time.Second)
	r := f.Run("launch", "--plan-file", pf, "--json")
	e := decodePlanEnvelope(t, r.Stdout)
	if r.Code != 125 || e.Error == nil || e.Error.Code != "plan-file" || e.Error.Message != msg {
		t.Fatalf("stale launch: code %d %s", r.Code, r.Stdout)
	}
	if _, ok := r.Agent(); ok {
		t.Fatal("an agent ran")
	}
	if r := f.Run("plan", "show", "--plan-file", pf); r.Code != 1 || !strings.Contains(r.Stderr, msg) {
		t.Errorf("stale show: code %d %s", r.Code, r.Stderr)
	}
	if runtime.GOOS == "windows" {
		return
	}
	ageCreatedAt(t, f, pf, 0)
	if err := os.Chmod(pf, 0o666); err != nil {
		t.Fatal(err)
	}
	if r := f.Run("launch", "--plan-file", pf); r.Code != 125 || !strings.Contains(r.Stderr, msg) {
		t.Errorf("foreign-writable launch: code %d %s", r.Code, r.Stderr)
	}
}

func TestLaunchNeedsPlanFile(t *testing.T) {
	f := fixtureShell(t)
	if r := f.Run("launch"); r.Code != 125 || !strings.Contains(r.Stderr, "--plan-file is required") {
		t.Errorf("code %d %s", r.Code, r.Stderr)
	}
	if r := f.Run("launch", "--plan-file", filepath.Join(f.Root, "none")); r.Code != 125 {
		t.Errorf("missing plan: code %d", r.Code)
	}
}

// TestLaunchRefusesDrift: a config edit between plan and launch that
// changes a decision (the profile's home) is refused, never launched.
func TestLaunchRefusesDrift(t *testing.T) {
	f := fixtureShell(t)
	pf := planFile(t, f)
	if r := f.Run("run", "--plan-file", pf); r.Code != 0 {
		t.Fatalf("plan: %d %s", r.Code, r.Stderr)
	}
	f.WriteConfig(strings.Replace(shellConfig, `"home": "~/.claude-work"`, `"home": "~/.claude-other"`, 1))
	r := f.Run("launch", "--plan-file", pf)
	if r.Code != 125 || !strings.Contains(r.Stderr, "no longer matches its plan (home") {
		t.Fatalf("code %d %s", r.Code, r.Stderr)
	}
	if _, ok := r.Agent(); ok {
		t.Fatal("an agent ran")
	}
}

// TestLaunchUsesPlanCwdForRelativePaths: the wrapper cds before launch,
// but relative --dir (and --env-file, --before-run) mean the folder the
// command was typed in.
func TestLaunchUsesPlanCwdForRelativePaths(t *testing.T) {
	f := fixtureShell(t)
	sub := f.Mkdir(filepath.Join("work", "sub"))
	pf := planFile(t, f)
	if r := f.Run("run", "--plan-file", pf, "--dir", "sub"); r.Code != 0 {
		t.Fatalf("plan: %d %s", r.Code, r.Stderr)
	}
	r := f.RunIn(f.Home, "", "launch", "--plan-file", pf)
	a, ok := r.Agent()
	if !ok || a.Pwd != sub {
		t.Fatalf("code %d pwd %q %s", r.Code, a.Pwd, r.Stderr)
	}
}

// shellEnv is the fixture's environment for a real shell, with the built
// agx first on PATH (the wrapper calls `command agx`).
func shellEnv(t *testing.T, f *testkit.Fixture) []string {
	bin := filepath.Dir(testkit.Binary(t))
	return []string{
		"HOME=" + f.Home, "AGX_CONFIG_DIR=" + f.ConfigDir, "TMPDIR=" + f.Mkdir("tmp"), "LANG=C",
		"PATH=" + bin + ":" + f.Bin + ":/usr/bin:/bin", "TOKVAR=" + fakeToken,
	}
}

// TestShellLayerEndToEnd loads the real layer into bash and zsh: agx new
// cds the shell into the new session folder, the fake agent runs there
// with the token, the plan is gone, and an alias goes through the
// wrapper.
func TestShellLayerEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells")
	}
	for _, sh := range []string{"bash", "zsh"} {
		t.Run(sh, func(t *testing.T) {
			shell, err := exec.LookPath(sh)
			if err != nil {
				t.Skipf("%s not installed", sh)
			}
			f := fixtureShell(t)
			script := `eval "$(agx shell-init --shell ` + sh + `)"
agx new --name e2e --login carol
echo "rc=$? shellpwd=$PWD shell=$AGX_SHELL"
ls "$TMPDIR"
cd "$HOME"
cw --dry-run --field data.profile
agx login use --id carol --terminal
echo "login=[$AGX_LOGIN]"
agx login use --home-login --terminal
echo "login=[${AGX_LOGIN-unset}]"
`
			cmd := exec.Command(shell, "-c", script)
			cmd.Dir = f.Work
			cmd.Env = shellEnv(t, f)
			outb, err := cmd.CombinedOutput()
			out := string(outb)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			sessions := filepath.Join(f.ConfigDir, "sessions")
			ents, err := os.ReadDir(sessions)
			if err != nil || len(ents) != 1 {
				t.Fatalf("sessions: %v %v\n%s", ents, err, out)
			}
			session := filepath.Join(sessions, ents[0].Name())
			for _, want := range []string{"pwd " + session, "env CLAUDE_CODE_OAUTH_TOKEN=" + fakeToken, "rc=0 shellpwd=" + session + " shell=" + sh, "\nwork\n", "login=[carol]", "login=[unset]"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "agx-plan") {
				t.Errorf("plan file left behind:\n%s", out)
			}
		})
	}
}
