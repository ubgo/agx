package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/testkit"
)

// Fixture run ids.
const (
	runAlive   = "20261114-101500-aaaa"
	runEnded   = "20261114-101501-bbbb"
	runUnknown = "20261114-101502-cccc"
	runStale   = "20261114-101503-dddd"
)

// goneBeyondPIDMax is a pid no Linux or macOS process table holds
// (pid_max is at most 2^22 on Linux, 99998 on macOS).
const goneBeyondPIDMax = 1 << 30

// standInEnv makes the test binary act as a stand-in agent: it sleeps
// instead of running the tests (see TestMain).
const standInEnv = "AGX_TEST_STAND_IN_AGENT"

// standInLife is how long a stand-in agent lives; the test kills it first.
const standInLife = time.Minute

// TestMain turns the test binary into a stand-in agent when standInEnv is
// set, so startAgent can start a process that is ours. A system binary
// such as /bin/sleep will not do: macOS hides a protected process's
// environment from ps -E, so its AGX_RUN_ID could never be seen.
func TestMain(m *testing.M) {
	if os.Getenv(standInEnv) != "" {
		time.Sleep(standInLife)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startAgent starts a stand-in agent process whose start environment
// holds AGX_RUN_ID=id, as an exec'd agent's would, and stops it when the
// test ends. Unix only: Windows reads no process environment.
func startAgent(t *testing.T, id string) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process environment is not readable on Windows")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self)
	cmd.Env = []string{"AGX_RUN_ID=" + id, standInEnv + "=1", "PATH=/usr/bin:/bin"}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // the test is over; a kill error means it already exited
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

// writeRun writes a run folder with run.jsonc (pid 0 = none) and, when
// liveJSON is not empty, live.jsonc.
func writeRun(t *testing.T, f *testkit.Fixture, id string, pid int, liveJSON string) string {
	t.Helper()
	dir := filepath.Join(f.ConfigDir, "runs", id)
	if pid != 0 {
		rec := struct {
			ID      string `json:"id"`
			PID     int    `json:"pid"`
			Agent   string `json:"agent"`
			Profile string `json:"profile"`
			Home    string `json:"home"`
			Login   string `json:"login"`
			Folder  string `json:"folder"`
		}{id, pid, "claude", "work", filepath.Join(f.Home, ".claude-work"), "carol", f.Work}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteFile(filepath.Join(dir, "run.jsonc"), string(b), 0o600)
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if liveJSON != "" {
		f.WriteFile(filepath.Join(dir, "live.jsonc"), liveJSON, 0o600)
	}
	return dir
}

// liveState is a live.jsonc started on carol.
func liveState(id string) string {
	return `{"run_id":"` + id + `","agent":"claude","start_login":"carol","helper":"agx credential"}`
}

// TestCleanUnknownReasons: a run agx cannot verify is kept, and its reason
// says why: no run.jsonc yet, a run.jsonc without a process id (read
// fine, so never "unreadable run.jsonc"), or a run.jsonc that does not
// parse.
func TestCleanUnknownReasons(t *testing.T) {
	f, _ := fixtureResume(t)
	runs := filepath.Join(f.ConfigDir, "runs")
	if err := os.MkdirAll(filepath.Join(runs, "20261114-101500-none"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.WriteFile(filepath.Join(runs, "20261114-101501-npid", "run.jsonc"), `{"id": "20261114-101501-npid", "agent": "claude"}`, 0o600)
	f.WriteFile(filepath.Join(runs, "20261114-101502-bad0", "run.jsonc"), `{"id": `, 0o600)
	r := f.Run("clean", "--json", "--field", "data.runs")
	var rows []struct {
		ID, Reason, Action string
		Status             string `json:"liveness"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &rows); err != nil || r.Code != 0 {
		t.Fatalf("code %d %v: %s %s", r.Code, err, r.Stdout, r.Stderr)
	}
	want := map[string]string{
		"20261114-101500-none": "process info unreadable: no run.jsonc yet",
		"20261114-101501-npid": "process info unreadable: run.jsonc records no process id",
		"20261114-101502-bad0": "process info unreadable: run.jsonc: unexpected end of JSON input",
	}
	for _, row := range rows {
		if row.Reason != want[row.ID] || row.Status != "unknown" || row.Action != "keep" {
			t.Errorf("%s: %+v, want reason %q", row.ID, row, want[row.ID])
		}
	}
	if len(rows) != len(want) {
		t.Errorf("rows %+v", rows)
	}
}

// TestEdge47to49Clean: ended runs (pid gone, edge 47; pid reused without
// the run's AGX_RUN_ID, edge 48) are planned for deletion and deleted
// only with --yes; a running agent is kept; a run agx cannot verify is
// kept and listed (edge 49).
func TestEdge47to49Clean(t *testing.T) {
	f, _ := fixtureResume(t)
	writeRun(t, f, runAlive, startAgent(t, runAlive), "")
	writeRun(t, f, runEnded, goneBeyondPIDMax, "")
	reused := writeRun(t, f, runStale, startAgent(t, "20200101-000000-zzzz"), "")
	unknown := writeRun(t, f, runUnknown, 0, "")

	r := f.Run("clean")
	if r.Code != 0 || !strings.Contains(r.Stdout, "would delete 2, keep 2; run agx clean --yes to delete") ||
		!strings.Contains(r.Stderr, "kept "+runUnknown+": process info unreadable: no run.jsonc yet\n") {
		t.Fatalf("plan: code %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
	if _, err := os.Stat(reused); err != nil {
		t.Fatal("plan deleted a folder")
	}

	r = f.Run("clean", "--yes", "--dry-run", "--json")
	if e := parseEnv(t, r); r.Code != 2 || e.Error.Code != "conflict" {
		t.Fatalf("--yes --dry-run: code %d %+v", r.Code, e.Error)
	}

	r = f.Run("clean", "--yes", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d %s", r.Code, r.Stdout)
	}
	// pids are the stand-in processes' own; the golden pins the shape.
	f.Golden(t, "clean-yes", regexp.MustCompile(`"pid": [0-9]+`).ReplaceAllString(r.Stdout, `"pid": 0`))
	for id, want := range map[string]bool{runAlive: true, runEnded: false, runStale: false, runUnknown: true} {
		_, err := os.Stat(filepath.Join(f.ConfigDir, "runs", id))
		if (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", id, err == nil, want)
		}
	}
	// A folder without run.jsonc older than a minute never started.
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(unknown, old, old); err != nil {
		t.Fatal(err)
	}
	r = f.Run("clean", "--yes")
	if _, err := os.Stat(unknown); r.Code != 0 || err == nil {
		t.Fatalf("never-started folder kept: code %d %s", r.Code, r.Stdout)
	}
}

// TestLiveUseAndCredential: live use chooses the login, agx credential
// (run by the agent, found by AGX_RUN_ID) prints only that login's token
// and records it as in use; --start-login goes back.
func TestLiveUseAndCredential(t *testing.T) {
	f, _ := fixtureResume(t)
	dir := writeRun(t, f, runAlive, startAgent(t, runAlive), liveState(runAlive))

	r := f.Run("live", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("list: %d %s", r.Code, r.Stdout)
	}
	f.Golden(t, "live-list", r.Stdout)

	r = f.Run("live", "use", "--id", runAlive, "--login", "alice", "--json")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"next": "alice"`) {
		t.Fatalf("use: %d %s", r.Code, r.Stdout)
	}

	f.Env["AGX_RUN_ID"] = runAlive
	r = f.Run("credential")
	if r.Code != 0 || r.Stdout != "fake-token-ALICE\n" {
		t.Fatalf("credential: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("credential", "--json")
	if r.Code != 0 || strings.Contains(r.Stdout, "fake-token") || !strings.Contains(r.Stdout, `"login": "alice"`) {
		t.Fatalf("credential --json: %d %s", r.Code, r.Stdout)
	}
	st := f.ReadFile(filepath.Join(dir, "live.jsonc"))
	if !strings.Contains(st, `"in_use": "alice"`) || strings.Contains(st, "fake-token") {
		t.Fatalf("live.jsonc %s", st)
	}
	delete(f.Env, "AGX_RUN_ID")

	r = f.Run("live", "use", "--id", runAlive, "--start-login")
	if r.Code != 0 || !strings.Contains(r.Stdout, "next        carol") {
		t.Fatalf("start-login: %d %q", r.Code, r.Stdout)
	}
	r = f.Run("whoami", "--json")
	if e := parseEnv(t, r); r.Code != 0 || !strings.Contains(string(e.Data), `"in_session": false`) {
		t.Fatalf("whoami outside: %d %s", r.Code, r.Stdout)
	}
}

// TestLiveRefusals: the refusals of live use and credential.
func TestLiveRefusals(t *testing.T) {
	f, _ := fixtureResume(t)
	writeRun(t, f, runAlive, startAgent(t, runAlive), liveState(runAlive))
	writeRun(t, f, runEnded, goneBeyondPIDMax, liveState(runEnded))
	writeRun(t, f, runStale, startAgent(t, runStale), "")
	cases := []struct {
		args       []string
		code       int
		ecode, msg string
	}{
		{[]string{"live", "use", "--id", runAlive, "--login", "alice", "--start-login"}, 2, "conflict", "--login and --start-login both choose the login; pick one"},
		{[]string{"live", "use", "--id", runAlive}, 2, "usage", "choose the login: --login NAME or --start-login"},
		{[]string{"live", "use", "--id", runAlive, "--login", "cx"}, 2, "wrong-agent", "login cx serves codex; this run is claude"},
		{[]string{"live", "use", "--id", runAlive, "--login", "helper"}, 4, "unsupported", "login helper is an api-key-helper; a live run needs a token login"},
		{[]string{"live", "use", "--id", runEnded, "--login", "alice"}, 1, "not-found", "run " + runEnded + " has ended; its login can no longer change"},
		{[]string{"live", "show", "--id", runStale}, 1, "not-found", "run " + runStale + " was not started with --live-login"},
		{[]string{"live", "show", "--id", "../x"}, 2, "usage", `"../x" is not a run id (YYYYMMDD-HHMMSS-xxxx)`},
		{[]string{"credential"}, 2, "usage", "agx credential runs inside a live-login session: AGX_RUN_ID is not set"},
	}
	for _, c := range cases {
		r := f.Run(append(c.args, "--json")...)
		e := parseEnv(t, r)
		if r.Code != c.code || e.Error.Code != c.ecode || e.Error.Message != c.msg {
			t.Errorf("%v: code %d %+v", c.args, r.Code, e.Error)
		}
	}
	// live list leaves out the ended run.
	r := f.Run("live", "list", "--field", "data.*.id")
	if r.Code != 0 || strings.Contains(r.Stdout, runEnded) || !strings.Contains(r.Stdout, runAlive) {
		t.Errorf("list: %d %q", r.Code, r.Stdout)
	}
}

// TestWhoamiInSession: whoami reads AGX_RUN_* and the run's live state,
// and lists AGX_LOGIN* set in the terminal.
func TestWhoamiInSession(t *testing.T) {
	f, _ := fixtureResume(t)
	writeRun(t, f, runAlive, 1, liveState(runAlive))
	f.Env["AGX_RUN_ID"] = runAlive
	f.Env["AGX_RUN_AGENT"] = "claude"
	f.Env["AGX_RUN_PROFILE"] = "work"
	f.Env["AGX_RUN_LOGIN"] = "carol"
	f.Env["AGX_RUN_HOME"] = filepath.Join(f.Home, ".claude-work")
	f.Env["AGX_LOGIN"] = "carol"
	f.Env["AGX_LOGIN_CLAUDE"] = "alice"
	r := f.Run("whoami", "--json")
	if r.Code != 0 {
		t.Fatalf("%d %s", r.Code, r.Stdout)
	}
	f.Golden(t, "whoami", r.Stdout)
	r = f.Run("whoami")
	for _, want := range []string{runAlive, "in use:           — (the agent has not asked for a key yet)", "next:             carol", "AGX_LOGIN_CLAUDE: alice", "profile:          work"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("text lacks %q:\n%s", want, r.Stdout)
		}
	}
}

// TestLiveLoginRefusals: --live-login is refused for an agent without
// the capability (typed reason), a home-login run, a one-off token and a
// profile whose env outranks the helper.
func TestLiveLoginRefusals(t *testing.T) {
	f, _ := fixtureResume(t)
	f.WriteConfig(strings.Replace(resumeConfig, `"work": { "agent": "claude", "home": "~/.claude-work" },`,
		`"work": { "agent": "claude", "home": "~/.claude-work" },
    "kimi": { "agent": "claude", "home": "~/.claude-work", "env": { "ANTHROPIC_AUTH_TOKEN": "fake-token-KIMI" } },`, 1))
	cases := []struct {
		args       []string
		ecode, has string
	}{
		{[]string{"--profile", "codex"}, "unsupported", "codex is not switchable ("},
		{[]string{"--profile", "work"}, "unsupported", "--live-login needs a stored login; this run starts on the home login"},
		{[]string{"--profile", "work", "--token-ref", "fake-token-ONEOFF"}, "unsupported", "a one-off --token-ref cannot be fetched again"},
		{[]string{"--profile", "work", "--login", "helper"}, "unsupported", "login helper is an api-key-helper"},
		{[]string{"--profile", "kimi", "--login", "carol"}, "conflict", "profile kimi env sets ANTHROPIC_AUTH_TOKEN, which outranks the live-login helper"},
		{[]string{"--profile", "work", "--login", "carol", "--env", "ANTHROPIC_API_KEY=x"}, "conflict", "--env ANTHROPIC_API_KEY and --login carol both set the credential"},
	}
	for _, c := range cases {
		r := f.Run(append([]string{"run", "--live-login", "--dry-run", "--json"}, c.args...)...)
		e := parseEnv(t, r)
		if r.Code != 125 || e.Error.Code != c.ecode || !strings.Contains(e.Error.Message, c.has) {
			t.Errorf("%v: code %d %+v", c.args, r.Code, e.Error)
		}
	}
}

// credentialLogins are logins whose token never resolves to one usable
// line, for TestCredentialNeverEmpty. The gopass entries are answered by
// credentialGopass.
const credentialLogins = `"logins": {
    "gpempty": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:team/claude/empty}" },
    "blank": { "agents": ["claude"], "kind": "oauth-token", "token": "${BLANKTOK}" },
    "multi": { "agents": ["claude"], "kind": "oauth-token", "token": "${MULTITOK}" },
    "unset": { "agents": ["claude"], "kind": "oauth-token", "token": "${NOT_SET_ANYWHERE}" },
    "gpgone": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:team/claude/gone}" },
    "gpbad": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:team/claude/bob}" },
    "cmdbad": { "agents": ["claude"], "kind": "oauth-token", "token": "$(sh -c 'echo cannot open /secret/store/bob >&2; exit 3')" },
    "stored": { "agents": ["claude"], "kind": "oauth-token", "stored": true },`

// storePath is the made-up secret-store path credentialGopass and the
// cmdbad login print on stderr; agx credential must never repeat it.
const storePath = "/secret/store"

// credentialGopass stands in for gopass: the "empty" entry prints nothing
// (exit 0), "gone" is gopass's not-found (exit 11) and anything else is a
// store failure whose stderr names the entry's file.
const credentialGopass = `#!/bin/sh
case "$3" in
  team/claude/empty) exit 0 ;;
  team/claude/gone) echo "Error: entry is not in the password store ` + storePath + `/team/claude/gone.gpg" >&2; exit 11 ;;
  *) echo "gpg: decryption failed: ` + storePath + `/$3.gpg" >&2; exit 2 ;;
esac
`

// TestCredentialNeverEmpty: agx credential is the agent's apiKeyHelper,
// and an empty or failing helper is a hard failure for Claude Code (it
// retries three times, then exits 1, with no fallback; LIVEFIX V1). So
// every token that is not one usable line (empty, whitespace only,
// multi-line, unresolvable) exits non-zero with nothing on stdout and
// exactly one stderr line (Claude shows it in /status) that names the
// login and the reference, never a value and never the store's own
// paths.
func TestCredentialNeverEmpty(t *testing.T) {
	f, _ := fixtureResume(t)
	f.WriteConfig(strings.Replace(resumeConfig, `"logins": {`, credentialLogins, 1))
	f.WriteFile(filepath.Join(f.Bin, "gopass"), credentialGopass, 0o755)
	f.Env["BLANKTOK"] = " \t "
	f.Env["MULTITOK"] = "fake-token-LINE1\nfake-token-LINE2"
	check := func(id string) string {
		return " (try: agx login check --id " + id + " --no-probe)"
	}
	cases := []struct {
		login string
		code  int
		line  string
	}{
		{"gpempty", 1, "agx: login gpempty: token is empty" + check("gpempty")},
		{"blank", 1, "agx: login blank: token is empty" + check("blank")},
		{"multi", 1, "agx: login multi: token has 2 lines" + check("multi")},
		{"unset", 1, "agx: login unset: NOT_SET_ANYWHERE is not set" + check("unset")},
		{"gpgone", 1, "agx: login gpgone: gopass:team/claude/gone is not in the gopass store" + check("gpgone")},
		{"gpbad", 1, "agx: login gpbad: gopass:team/claude/bob could not be read by gopass" + check("gpbad")},
		{"cmdbad", 1, `agx: login cmdbad: command "sh -c 'echo cannot open /secret/store/bob >&2; exit 3'" failed: exit 3` + check("cmdbad")},
		{"stored", 1, "agx: login stored: no stored value in tokens.jsonc (try: agx login edit --id stored --stdin)"},
		{"nosuch", 1, "agx: unknown login nosuch (try: agx login list)"},
	}
	for i, c := range cases {
		id := fmt.Sprintf("20261114-101500-c%03d", i)
		writeRun(t, f, id, 1, `{"run_id":"`+id+`","agent":"claude","start_login":"`+c.login+`","helper":"agx credential"}`)
		f.Env["AGX_RUN_ID"] = id
		r := f.Run("credential")
		if r.Code != c.code || r.Stdout != "" || r.Stderr != c.line+"\n" {
			t.Errorf("%s: code %d stdout %q stderr %q\nwant stderr %q", c.login, r.Code, r.Stdout, r.Stderr, c.line)
		}
		if strings.Contains(r.Stderr, "fake-token") || (c.login != "cmdbad" && strings.Contains(r.Stderr, storePath)) {
			t.Errorf("%s: stderr leaks a value or a store path: %q", c.login, r.Stderr)
		}
		st := f.ReadFile(filepath.Join(f.ConfigDir, "runs", id, "live.jsonc"))
		if strings.Contains(st, `"in_use"`) {
			t.Errorf("%s: a failed fetch recorded a login in use: %s", c.login, st)
		}
	}
	// No run: the one line names the run and the command that lists runs.
	f.Env["AGX_RUN_ID"] = "20261114-101500-none"
	if r := f.Run("credential"); r.Code != 1 || r.Stdout != "" || r.Stderr != "agx: no run 20261114-101500-none (try: agx live list)\n" {
		t.Errorf("no run: code %d stdout %q stderr %q", r.Code, r.Stdout, r.Stderr)
	}
}
