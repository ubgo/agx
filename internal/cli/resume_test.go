package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/testkit"
)

// resumeConfig has two Claude homes (personal default on ~/.claude, work
// on ~/.claude-work), a shared home with two plan profiles and no default
// (~/.claude-shared: a, b), and a codex profile.
const resumeConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "a": { "agent": "claude", "home": "~/.claude-shared" },
    "b": { "agent": "claude", "home": "~/.claude-shared" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" },
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKALICE}" },
    "helper": { "agents": ["claude"], "kind": "api-key-helper", "command": "printf x" },
    "cx": { "agents": ["codex"], "kind": "api-key", "token": "${TOKVAR}" }
  }
}`

// Fixture conversation ids (claude transcripts are named <id>.jsonl).
const (
	chatWork   = "3f2a9c1e-0000-4000-8000-000000000001"
	chatWork2  = "3f2a9c1e-0000-4000-8000-000000000002"
	chatPers   = "3f2a77aa-0000-4000-8000-000000000003"
	chatShared = "5b5b5b5b-0000-4000-8000-000000000004"
)

// fixtureResume is a machine with resumeConfig, its homes, a fake claude
// and a project folder.
func fixtureResume(t *testing.T) (*testkit.Fixture, string) {
	t.Helper()
	f := testkit.New(t)
	for _, h := range []string{".claude", ".claude-work", ".claude-shared", ".codex"} {
		f.HomeDir(h)
	}
	f.FakeAgent("claude")
	f.FakeAgent("codex")
	f.WriteConfig(resumeConfig)
	f.Env["TOKVAR"] = "fake-token-CAROL"
	f.Env["TOKALICE"] = "fake-token-ALICE"
	return f, f.Mkdir("proj")
}

// encodeClaudeDir is Claude's project-folder name for an ASCII path:
// every byte that is not a letter or digit becomes "-" (the on-disk shape
// agentio's claude fixtures use).
func encodeClaudeDir(dir string) string {
	return regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(dir, "-")
}

// writeChat writes a Claude transcript for id in home (relative to the
// user home), started in cwd, last active at mtime.
func writeChat(t *testing.T, f *testkit.Fixture, home, cwd, id, title string, mtime time.Time) {
	t.Helper()
	tpl, err := os.ReadFile(filepath.Join("testdata", "resume", "claude-transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer("{{ID}}", id, "{{CWD}}", cwd, "{{TITLE}}", title).Replace(string(tpl))
	p := f.WriteFile(filepath.Join(f.Home, home, "projects", encodeClaudeDir(cwd), id+".jsonl"), body, 0o600)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// resumeEnvelope is the part of the JSON envelope the tests read.
type resumeEnvelope struct {
	OK        bool `json:"ok"`
	Decisions []struct {
		Name, Value, Source string
	} `json:"decisions"`
	Error struct {
		Code, Message, Hint string
	} `json:"error"`
	Data json.RawMessage `json:"data"`
}

// parseEnv decodes r.Stdout as the envelope.
func parseEnv(t *testing.T, r testkit.Result) resumeEnvelope {
	t.Helper()
	var e resumeEnvelope
	if err := json.Unmarshal([]byte(r.Stdout), &e); err != nil {
		t.Fatalf("not an envelope (code %d): %v\nstdout %s\nstderr %s", r.Code, err, r.Stdout, r.Stderr)
	}
	return e
}

// decision returns the source of decision name.
func (e resumeEnvelope) decision(name string) (string, string) {
	for _, d := range e.Decisions {
		if d.Name == name {
			return d.Value, d.Source
		}
	}
	return "", ""
}

var t0 = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

// TestResumeIDPrefixDryRun is spec 6.5 one match: a unique prefix decides
// agent, home and folder; the profile comes from the home; every source
// is shown.
func TestResumeIDPrefixDryRun(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-work", proj, chatWork, "work chat", t0)
	r := f.Run("resume", "--id", "3f2a9c1e-0000-4000-8000-0000000000", "--login", "carol", "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s %s", r.Code, r.Stdout, r.Stderr)
	}
	e := parseEnv(t, r)
	checks := map[string][2]string{
		"agent":   {"claude", "conversation " + chatWork},
		"home":    {"~/.claude-work", "conversation " + chatWork + " (stored there)"},
		"profile": {"work", "home ~/.claude-work"},
		"folder":  {proj, "conversation " + chatWork},
		"login":   {"carol (not resolved: --dry-run)", "--login"},
	}
	for name, want := range checks {
		v, s := e.decision(name)
		if v != want[0] || s != want[1] {
			t.Errorf("%s = %q from %q, want %q from %q", name, v, s, want[0], want[1])
		}
	}
	if v, _ := e.decision("command"); !strings.Contains(v, "--resume "+chatWork) {
		t.Errorf("command %q", v)
	}
	f.Golden(t, "resume-dry-run", r.Stdout)
}

// TestResumeExecsInConversationFolder: the agent runs in the chat's
// folder with the resume id, on the chat's home.
func TestResumeExecsInConversationFolder(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-work", proj, chatWork, "work chat", t0)
	r := f.Run("resume", "--id", chatWork)
	a, ok := r.Agent()
	if r.Code != 0 || !ok {
		t.Fatalf("code %d: %s %s", r.Code, r.Stdout, r.Stderr)
	}
	if a.Pwd != proj || !slices.Contains(a.Argv, chatWork) || a.Env["CLAUDE_CONFIG_DIR"] != filepath.Join(f.Home, ".claude-work") {
		t.Errorf("agent %+v", a)
	}
	if !strings.Contains(r.Stderr, "agx: profile work from home ~/.claude-work\n") {
		t.Errorf("no shared-home note on stderr: %q", r.Stderr)
	}
	// The source carries its own parentheses; the note never nests them.
	writeChat(t, f, ".claude", proj, chatPers, "personal chat", t0)
	r = f.Run("resume", "--id", chatPers)
	if r.Code != 0 || !strings.Contains(r.Stderr, "agx: profile personal from home ~/.claude (default)\n") {
		t.Errorf("code %d, default-home note: %q", r.Code, r.Stderr)
	}
}

// TestResumeLastAndQuery: --last takes the newest chat of this folder;
// --query narrows first.
func TestResumeLastAndQuery(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-work", proj, chatWork, "older alpha", t0)
	writeChat(t, f, ".claude", proj, chatPers, "newer beta", t0.Add(time.Hour))
	r := f.RunIn(proj, "", "resume", "--last", "--dry-run", "--json")
	if v, _ := parseEnv(t, r).decision("profile"); r.Code != 0 || v != "personal" {
		t.Fatalf("--last chose %q (code %d): %s", v, r.Code, r.Stderr)
	}
	r = f.RunIn(proj, "", "resume", "--last", "--query", "ALPHA", "--dry-run", "--json")
	if v, _ := parseEnv(t, r).decision("profile"); r.Code != 0 || v != "work" {
		t.Fatalf("--query chose %q (code %d): %s", v, r.Code, r.Stderr)
	}
	r = f.Run("resume", "--last", "--dry-run", "--json")
	if e := parseEnv(t, r); r.Code != 125 || e.Error.Code != "not-found" || !strings.Contains(e.Error.Hint, "--scope all") {
		t.Fatalf("other folder: code %d %+v", r.Code, e.Error)
	}
	r = f.Run("resume", "--last", "--scope", "all", "--dry-run", "--json")
	if v, _ := parseEnv(t, r).decision("profile"); r.Code != 0 || v != "personal" {
		t.Fatalf("--scope all chose %q (code %d)", v, r.Code)
	}
}

// TestResumeFlagConflicts: two choices of the conversation are refused.
func TestResumeFlagConflicts(t *testing.T) {
	f, _ := fixtureResume(t)
	for _, args := range [][]string{
		{"--id", chatWork, "--last"},
		{"--id", chatWork, "--query", "x"},
		{"--last", "--picker", "folder"},
	} {
		r := f.Run(append([]string{"resume", "--json"}, args...)...)
		if e := parseEnv(t, r); r.Code != 125 || e.Error.Code != "conflict" {
			t.Errorf("%v: code %d %+v", args, r.Code, e.Error)
		}
	}
	r := f.Run("resume", "--json", "--last", "--scope", "everywhere")
	if e := parseEnv(t, r); r.Code != 125 || e.Error.Code != "usage" {
		t.Errorf("bad scope: code %d %+v", r.Code, e.Error)
	}
}

// TestResumeNotFound is 6.5 "none": not found, naming what was searched.
func TestResumeNotFound(t *testing.T) {
	f, _ := fixtureResume(t)
	r := f.Run("resume", "--id", "nope", "--json")
	e := parseEnv(t, r)
	if r.Code != 125 || e.Error.Code != "not-found" || !strings.Contains(e.Error.Message, "conversation nope not found in") {
		t.Fatalf("code %d %+v", r.Code, e.Error)
	}
}

// TestResumeNotFoundHintKnowsEveryProfile: with --profile only that
// profile's home is searched, but the other profiles' homes are still in
// config; the "homes not in config" hint names only a home no profile
// uses (~/.claude-old), never ~/.claude of profile personal.
func TestResumeNotFoundHintKnowsEveryProfile(t *testing.T) {
	f, _ := fixtureResume(t)
	// A marker makes a folder a home agentio finds on disk.
	for _, h := range []string{".claude", ".claude-old"} {
		f.WriteFile(filepath.Join(f.HomeDir(h), "settings.json"), "{}", 0o600)
	}
	r := f.Run("resume", "--id", "nope", "--profile", "work", "--dry-run", "--json")
	e := parseEnv(t, r)
	want := "homes not in config: ~/.claude-old   (agx profile import --discovered)"
	if r.Code != 125 || !slices.Contains(strings.Split(e.Error.Hint, "\n"), want) {
		t.Fatalf("code %d, hint %q", r.Code, e.Error.Hint)
	}
}

// TestEdge24FolderGone: a conversation whose folder was deleted.
func TestEdge24FolderGone(t *testing.T) {
	f, _ := fixtureResume(t)
	gone := filepath.Join(f.Root, "old", "path")
	writeChat(t, f, ".claude-work", gone, chatWork, "gone", t0)
	r := f.Run("resume", "--id", "3f2a9c1e", "--json")
	e := parseEnv(t, r)
	want := "conversation " + chatWork + " was in " + gone + ", which no longer exists"
	if r.Code != 125 || e.Error.Code != "folder-gone" || e.Error.Message != want || e.Error.Hint == "" {
		t.Fatalf("code %d %+v", r.Code, e.Error)
	}
}

// TestEdge25NoTTY: resume without --id or --last and no terminal.
func TestEdge25NoTTY(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-work", proj, chatWork, "work chat", t0)
	r := f.RunIn(proj, "", "resume")
	if r.Code != 125 || !strings.Contains(r.Stderr, "no terminal for the picker; pass --id or --last") {
		t.Fatalf("code %d stderr %q", r.Code, r.Stderr)
	}
	r = f.RunIn(proj, "", "resume", "--json")
	if e := parseEnv(t, r); e.Error.Code != "no-tty" {
		t.Fatalf("%+v", e.Error)
	}
}

// TestEdge26PrefixMatchesTwoHomes: a prefix found in two homes is refused,
// listing each match with its --profile; --profile narrows to one.
func TestEdge26PrefixMatchesTwoHomes(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-work", proj, chatWork, "work chat", t0)
	writeChat(t, f, ".claude", proj, chatPers, "personal chat", t0)
	r := f.Run("resume", "--id", "3f2a", "--json")
	e := parseEnv(t, r)
	if r.Code != 125 || e.Error.Code != "ambiguous" || !strings.HasPrefix(e.Error.Message, "3f2a matches 2 conversations: ") ||
		!strings.HasSuffix(e.Error.Message, "; add --profile") || !strings.Contains(e.Error.Message, "--profile personal") || !strings.Contains(e.Error.Message, "--profile work") {
		t.Fatalf("code %d %+v", r.Code, e.Error)
	}
	r = f.Run("resume", "--id", "3f2a", "--profile", "work", "--dry-run", "--json")
	e = parseEnv(t, r)
	if v, s := e.decision("profile"); r.Code != 0 || v != "work" || s != "--profile" {
		t.Fatalf("with --profile: code %d %q %q %+v", r.Code, v, s, e.Error)
	}
	// Two matches inside one home: --profile cannot help, so say so.
	writeChat(t, f, ".claude-work", proj, chatWork2, "work chat 2", t0)
	r = f.Run("resume", "--id", "3f2a9c1e", "--json")
	if e := parseEnv(t, r); r.Code != 125 || !strings.HasSuffix(e.Error.Message, "; give more of the id") {
		t.Fatalf("same home: code %d %+v", r.Code, e.Error)
	}
}

// TestEdge28SharedHomeNoTTY: a shared home with two plan profiles and no
// default, without a terminal, is refused listing them.
func TestEdge28SharedHomeNoTTY(t *testing.T) {
	f, proj := fixtureResume(t)
	writeChat(t, f, ".claude-shared", proj, chatShared, "shared chat", t0)
	r := f.Run("resume", "--id", chatShared, "--json")
	e := parseEnv(t, r)
	if r.Code != 125 || e.Error.Code != "ambiguous" || e.Error.Message != "~/.claude-shared has 2 profiles (a, b) and no default; add --profile" {
		t.Fatalf("code %d %+v", r.Code, e.Error)
	}
	r = f.Run("resume", "--id", chatShared, "--profile", "b", "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("with --profile: %d %s", r.Code, r.Stdout)
	}
}

// TestEdge50NestedLaunchMakesNewRun: an agx launch inside an agent (its
// AGX_RUN_ID set) creates its own run folder and id.
func TestEdge50NestedLaunchMakesNewRun(t *testing.T) {
	f, proj := fixtureResume(t)
	const outer = "20261114-101500-k3x9"
	f.Env["AGX_RUN_ID"] = outer
	r := f.RunIn(proj, "", "run", "--profile", "work")
	a, ok := r.Agent()
	if r.Code != 0 || !ok {
		t.Fatalf("code %d %s", r.Code, r.Stderr)
	}
	inner := a.Env["AGX_RUN_ID"]
	if inner == "" || inner == outer {
		t.Fatalf("AGX_RUN_ID %q", inner)
	}
	if _, err := os.Stat(filepath.Join(f.ConfigDir, "runs", inner, "run.jsonc")); err != nil {
		t.Fatal(err)
	}
}
