package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/claude"

	"github.com/ubgo/agx/internal/apperr"
)

// convoTestConfig has two Claude homes, so a move has a source and a
// target.
const convoTestConfig = `{
  "version": 1,
  "sessions": { "root": "~/sessions" },
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work":     { "agent": "claude", "home": "~/.claude-work" }
  }
}`

// Fake values for the live-session fixture.
const (
	livePID   = 4242
	liveConv  = "aaaaaaaa-0000-4000-8000-000000000001"
	liveStart = 1794597025793 // startedAt in the session file (Unix ms)
)

// writeFile writes data under dir, creating parents.
func writeFile(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// claudeKeyOf is Claude Code's history folder name for dir, from agentio's
// claude.EncodePath: a local copy missed the cut at 200 characters plus
// hash suffix, which a long TMPDIR reaches (fixture building only).
func claudeKeyOf(dir string) string {
	return claude.EncodePath(dir)
}

// liveApp builds an App over a fixture user home with one conversation in
// proj (personal home) whose Claude session runs as pid 4242 in a fake
// process table, so the session reads as alive.
func liveApp(t *testing.T) (*App, string, string) {
	t.Helper()
	home := t.TempDir()
	proj := filepath.Join(home, "proj")
	for _, d := range []string{proj, filepath.Join(home, ".claude-work"), filepath.Join(home, "sessions")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(home, ".agx", "config.jsonc"), convoTestConfig)
	writeFile(t, filepath.Join(home, ".claude", "projects", claudeKeyOf(proj), liveConv+".jsonl"),
		`{"type":"user","sessionId":"`+liveConv+`","timestamp":"2026-11-10T09:00:00Z","cwd":"`+proj+`","message":{"role":"user","content":"fake"}}`+"\n")
	writeFile(t, filepath.Join(home, ".claude", "sessions", strconv.Itoa(livePID)+".json"),
		`{"pid":`+strconv.Itoa(livePID)+`,"sessionId":"`+liveConv+`","cwd":"`+proj+`","startedAt":`+strconv.Itoa(liveStart)+`,"kind":"interactive","status":"busy"}`)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: livePID, Start: time.UnixMilli(liveStart), Argv: []string{"claude"}}}}
	h := agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithEnv("HOME="+home, "PATH=/nonexistent-bin"), agenttest.WithProcesses(procs))
	lookup := func(k string) (string, bool) { return h.Getenv(k) }
	return New(Deps{Host: h, Lookup: lookup, Cwd: proj}), home, proj
}

// TestConversationMoveRefusesLiveConversation: a running conversation
// blocks the move before any file changes (LiveSessionsOf), even on a
// dry run, which shows the plan's blockers as the refusal.
func TestConversationMoveRefusesLiveConversation(t *testing.T) {
	a, home, proj := liveApp(t)
	for _, dry := range []bool{true, false} {
		_, err := a.ConversationMove(context.Background(), ConversationMoveOptions{ToProfile: "work", DryRun: dry})
		e := apperr.As(err)
		if err == nil || e.Code != apperr.CodeMoveBlocked || !strings.Contains(e.Message, "pid 4242") {
			t.Fatalf("dry=%v: err %v", dry, err)
		}
		if strings.Count(e.Message, "pid 4242") != 1 {
			t.Errorf("one line per running process: %s", e.Message)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "projects", claudeKeyOf(proj), liveConv+".jsonl")); err != nil {
		t.Fatal("a blocked move touched the transcript")
	}
}

// TestConversationShowLive: show reports the conversation as live.
func TestConversationShowLive(t *testing.T) {
	a, _, _ := liveApp(t)
	d, err := a.ConversationShow(context.Background(), "aaaa")
	if err != nil {
		t.Fatal(err)
	}
	if d.Live == nil || !*d.Live || d.Turns == nil || *d.Turns != 1 {
		t.Fatalf("live %v turns %v", d.Live, d.Turns)
	}
}

// TestSessionPromoteRefusesLiveAgent: a running agent in the session
// folder refuses the promote; nothing moves.
func TestSessionPromoteRefusesLiveAgent(t *testing.T) {
	a, home, _ := liveApp(t)
	sess := filepath.Join(home, "sessions", "busy_20200101_101500")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".claude", "projects", claudeKeyOf(sess), "bbbbbbbb-0000-4000-8000-000000000002.jsonl"),
		`{"type":"user","sessionId":"b","timestamp":"2026-11-10T09:00:00Z","cwd":"`+sess+`","message":{"role":"user","content":"fake"}}`+"\n")
	writeFile(t, filepath.Join(home, ".claude", "sessions", strconv.Itoa(livePID)+".json"),
		`{"pid":`+strconv.Itoa(livePID)+`,"sessionId":"b","cwd":"`+sess+`","startedAt":`+strconv.Itoa(liveStart)+`,"kind":"interactive","status":"busy"}`)
	_, _, err := a.SessionPromote(context.Background(), SessionPromoteOptions{ID: filepath.Base(sess), To: filepath.Join(home, "dest")})
	if e := apperr.As(err); err == nil || e.Code != apperr.CodeMoveBlocked {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(sess); err != nil {
		t.Fatal("a refused promote moved the folder")
	}
}
