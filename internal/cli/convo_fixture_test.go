package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/claude"

	"github.com/ubgo/agx/internal/testkit"
)

// convoConfig has two Claude homes, one Codex home and a sessions root
// inside the fixture, so conversation and session commands have something
// to read and move.
const convoConfig = `{
  "version": 1,
  "sessions": { "root": "~/sessions", "promote_root": "~/projects" },
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work":     { "agent": "claude", "home": "~/.claude-work" },
    "codex":    { "agent": "codex",  "home": "~/.codex", "default": true }
  }
}`

// Fake conversation ids: shaped like the vendors' (uuid), plainly fake.
const (
	idAlpha = "aaaaaaaa-0000-4000-8000-000000000001"
	idBravo = "aaaabbbb-0000-4000-8000-000000000002"
	idOther = "cccccccc-0000-4000-8000-000000000003"
	idWork  = "dddddddd-0000-4000-8000-000000000004"
	idCodex = "0199e1a0-0000-7000-8000-00000000c0de"
)

// convoFixture is a machine with convoConfig's homes and a project folder
// (Work/proj) to run commands in.
type convoFixture struct {
	*testkit.Fixture
	proj string
}

// newConvoFixture builds the machine; homes start empty.
func newConvoFixture(t *testing.T) *convoFixture {
	t.Helper()
	f := testkit.New(t)
	for _, h := range []string{".claude", ".claude-work", ".codex", "sessions"} {
		f.HomeDir(h)
	}
	f.WriteConfig(convoConfig)
	return &convoFixture{Fixture: f, proj: f.Mkdir("work/proj")}
}

// convoTemplate reads a transcript template and fills it.
func convoTemplate(t *testing.T, name, id, cwd, title string, at time.Time) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "transcripts", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer("{{ID}}", id, "{{CWD}}", cwd, "{{TITLE}}", title, "{{TIME}}", at.UTC().Format(time.RFC3339)).Replace(string(b))
}

// convoClaudeKey is Claude Code's history folder name for dir, from
// agentio's claude.EncodePath (memory note claude-history-path-encoding).
// A local copy of the encoding missed the cut at 200 characters plus hash
// suffix, so under a long TMPDIR the fixture wrote history where agx (via
// agentio) never looks. Fixture building only.
func convoClaudeKey(dir string) string {
	return claude.EncodePath(dir)
}

// claudeConv writes a Claude transcript for dir in home (".claude"…),
// last changed at the given time.
func (f *convoFixture) claudeConv(t *testing.T, home, dir, id, title string, at time.Time) string {
	t.Helper()
	p := filepath.Join(f.Home, home, "projects", convoClaudeKey(dir), id+".jsonl")
	f.WriteFile(p, convoTemplate(t, "claude.jsonl", id, dir, title, at), 0o600)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	return p
}

// codexConv writes a Codex rollout for dir in ~/.codex.
func (f *convoFixture) codexConv(t *testing.T, dir, id, title string, at time.Time) string {
	t.Helper()
	name := "rollout-" + at.UTC().Format("2006-01-02T15-04-05") + "-" + id + ".jsonl"
	p := filepath.Join(f.Home, ".codex", "sessions", at.UTC().Format("2006/01/02"), name)
	f.WriteFile(p, convoTemplate(t, "codex.jsonl", id, dir, title, at), 0o600)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	return p
}

// convoBase is a fixed time the fixture conversations are spaced from.
var convoBase = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

// seedConversations writes the standard set: two personal chats and one
// work chat in proj, one personal chat elsewhere.
func (f *convoFixture) seedConversations(t *testing.T) {
	t.Helper()
	f.claudeConv(t, ".claude", f.proj, idAlpha, "Fix the parser", convoBase.Add(1*time.Hour))
	f.claudeConv(t, ".claude", f.proj, idBravo, "Write the docs", convoBase.Add(3*time.Hour))
	f.claudeConv(t, ".claude-work", f.proj, idWork, "Work review", convoBase.Add(2*time.Hour))
	f.claudeConv(t, ".claude", f.Mkdir("work/other"), idOther, "Other folder chat", convoBase.Add(4*time.Hour))
}

// convoEnvelope is the part of the JSON convoEnvelope tests assert on.
type convoEnvelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Warnings []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"warnings"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

// convoParse decodes a --json result.
func convoParse(t *testing.T, r testkit.Result) convoEnvelope {
	t.Helper()
	var e convoEnvelope
	if err := json.Unmarshal([]byte(r.Stdout), &e); err != nil {
		t.Fatalf("not JSON (code %d): %v\nstdout %s\nstderr %s", r.Code, err, r.Stdout, r.Stderr)
	}
	return e
}

// convoWantErr asserts a failed --json run: exit code and error code.
func convoWantErr(t *testing.T, r testkit.Result, code int, errCode string) convoEnvelope {
	t.Helper()
	e := convoParse(t, r)
	if r.Code != code || e.Error == nil || e.Error.Code != errCode {
		t.Fatalf("exit %d error %+v, want exit %d code %s\n%s", r.Code, e.Error, code, errCode, r.Stdout)
	}
	return e
}

// convoExists reports a path on disk.
func convoExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// convoSizeRe matches a transcript size, which varies with the temp path
// length written into the fixture transcript.
var convoSizeRe = regexp.MustCompile(`"size_bytes": [0-9]+`)

// golden pins out after replacing the fixture root's Claude history key
// (it holds the temp path, which Normalize cannot see in encoded form) and
// the transcript size.
func (f *convoFixture) golden(t *testing.T, name, out string) {
	t.Helper()
	out = strings.ReplaceAll(out, convoClaudeKey(f.Root), "<root-key>")
	f.Golden(t, name, convoSizeRe.ReplaceAllString(out, `"size_bytes": "<size>"`))
}
