package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/sessions"
)

// convoOldStamp is a session timestamp far older than any --min-age.
const convoOldStamp = "20200101_101500"

// session makes a folder under the fixture's sessions root.
func (f *convoFixture) session(name string) string {
	return f.HomeDir(filepath.Join("sessions", name))
}

// convoCleanData is the part of session clean data tests assert on.
type convoCleanData struct {
	Items []struct {
		Folder struct {
			Name string `json:"name"`
		} `json:"folder"`
		Remove  bool   `json:"remove"`
		Reason  string `json:"reason"`
		Removed bool   `json:"removed"`
	} `json:"items"`
}

// decisions maps folder name to "reason" (+ " removed").
func (c convoCleanData) decisions() map[string]string {
	out := map[string]string{}
	for _, it := range c.Items {
		v := it.Reason
		if it.Removed {
			v += " removed"
		}
		out[it.Folder.Name] = v
	}
	return out
}

// TestSessionListAndClean: list shows files and history; clean is a plan
// unless --yes, and removes only empty, history-free, old folders.
func TestSessionListAndClean(t *testing.T) {
	f := newConvoFixture(t)
	empty := f.session("scratch_" + convoOldStamp)
	dsStore := f.session("finder_" + convoOldStamp)
	f.WriteFile(filepath.Join(dsStore, ".DS_Store"), "x", 0o600)
	files := f.session("notes_" + convoOldStamp)
	f.WriteFile(filepath.Join(files, "a.txt"), "x", 0o600)
	chat := f.session("chat_" + convoOldStamp)
	f.claudeConv(t, ".claude", chat, idAlpha, "a chat", convoBase)
	// The agx binary runs with the fixture's TZ=UTC (testkit), so the
	// stamp is written in UTC whatever this test process's zone is.
	fresh := f.session(sessions.FolderName("fresh", time.Now().UTC()))

	r := f.Run("session", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("list: %d %s", r.Code, r.Stdout)
	}
	var list []struct {
		Name    string   `json:"name"`
		Files   int      `json:"files"`
		History []string `json:"history"`
	}
	if err := json.Unmarshal(convoParse(t, r).Data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 || list[0].Name != filepath.Base(fresh) {
		t.Fatalf("list %+v", list)
	}
	for _, s := range list {
		if s.Name == filepath.Base(chat) && (len(s.History) != 1 || s.History[0] != "claude") {
			t.Fatalf("chat history %+v", s)
		}
		if s.Name == filepath.Base(files) && s.Files != 1 {
			t.Fatalf("files %+v", s)
		}
	}

	r = f.Run("session", "clean", "--json")
	if r.Code != 0 {
		t.Fatalf("clean plan: %d %s", r.Code, r.Stdout)
	}
	var plan convoCleanData
	if err := json.Unmarshal(convoParse(t, r).Data, &plan); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		filepath.Base(empty): "empty, no history", filepath.Base(dsStore): "empty, no history",
		filepath.Base(files): "has files", filepath.Base(chat): "has conversation history",
		filepath.Base(fresh): "newer than --min-age",
	}
	if got := plan.decisions(); !convoMapsEqual(got, want) {
		t.Fatalf("plan %v, want %v", got, want)
	}
	if !convoExists(empty) {
		t.Fatal("the plan removed a folder")
	}
	text := f.Run("session", "clean")
	if !strings.Contains(text.Stdout, "add --yes") {
		t.Fatalf("plan text %s", text.Stdout)
	}

	r = f.Run("session", "clean", "--yes", "--json")
	if r.Code != 0 {
		t.Fatalf("clean --yes: %d %s", r.Code, r.Stdout)
	}
	if convoExists(empty) || convoExists(dsStore) || !convoExists(files) || !convoExists(chat) || !convoExists(fresh) {
		t.Fatalf("after clean: empty %v ds %v files %v chat %v fresh %v", convoExists(empty), convoExists(dsStore), convoExists(files), convoExists(chat), convoExists(fresh))
	}
	r = f.Run("session", "clean", "--min-age", "0s", "--yes", "--json")
	if r.Code != 0 || convoExists(fresh) {
		t.Fatalf("--min-age 0s: %d fresh %v", r.Code, convoExists(fresh))
	}
	convoWantErr(t, f.Run("session", "clean", "--min-age", "-1h", "--json"), 2, "usage")
}

// convoMapsEqual compares two string maps.
func convoMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestSessionCleanEmptyRoot: no sessions root is an empty plan, exit 0.
func TestSessionCleanEmptyRoot(t *testing.T) {
	f := newConvoFixture(t)
	r := f.Run("session", "list", "--json")
	if r.Code != 0 || string(convoParse(t, r).Data) != "[]" {
		t.Fatalf("list %d %s", r.Code, r.Stdout)
	}
}

// TestSessionPromoteMovesClaudeHistory: the folder moves to promote_root
// under its slug; its Claude history is re-keyed, so the conversation is
// listed from the new folder; --dry-run changes nothing.
func TestSessionPromoteMovesClaudeHistory(t *testing.T) {
	f := newConvoFixture(t)
	id := "pglite_golang_" + convoOldStamp
	src := f.session(id)
	f.WriteFile(filepath.Join(src, "main.go"), "package main\n", 0o600)
	f.claudeConv(t, ".claude", src, idAlpha, "pglite chat", convoBase)
	r := f.Run("session", "promote", "--id", id, "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("dry run: %d %s", r.Code, r.Stdout)
	}
	f.golden(t, "session-promote-dry-run", r.Stdout)
	if !convoExists(src) {
		t.Fatal("--dry-run moved the folder")
	}
	r = f.Run("session", "promote", "--id", id, "--json")
	if r.Code != 0 {
		t.Fatalf("promote: %d %s", r.Code, r.Stdout)
	}
	dst := filepath.Join(f.Home, "projects", "pglite_golang")
	if convoExists(src) || !convoExists(filepath.Join(dst, "main.go")) {
		t.Fatalf("folder not moved: src %v", convoExists(src))
	}
	list := convoParse(t, f.RunIn(dst, "", "conversation", "list", "--json"))
	if got := convoIDs(convoRows(t, list)); got != "aaaaaaaa" {
		t.Fatalf("history after promote: %q", got)
	}
	e := convoWantErr(t, f.Run("session", "promote", "--id", id, "--json"), 1, "not-found")
	if !strings.Contains(e.Error.Hint, "agx session list") {
		t.Fatalf("hint %q", e.Error.Hint)
	}
}

// TestEdge55PromoteWithCodexConversations: Codex history cannot move
// with the folder, so promote is refused with exit 4 unless --no-codex,
// which promotes and warns that it stays behind.
func TestEdge55PromoteWithCodexConversations(t *testing.T) {
	f := newConvoFixture(t)
	id := "mixed_" + convoOldStamp
	src := f.session(id)
	f.codexConv(t, src, idCodex, "codex one", convoBase)
	f.codexConv(t, src, "0199e1a0-0000-7000-8000-00000000c0df", "codex two", convoBase.Add(time.Minute))
	f.claudeConv(t, ".claude", src, idAlpha, "claude chat", convoBase)
	r := f.Run("session", "promote", "--id", id, "--to", "~/elsewhere", "--json")
	e := convoWantErr(t, r, 4, "unsupported")
	if e.Error.Message != "this folder has 2 Codex conversations; add --no-codex" {
		t.Fatalf("message %q", e.Error.Message)
	}
	if !convoExists(src) {
		t.Fatal("refused promote moved the folder")
	}
	r = f.Run("session", "promote", "--id", id, "--to", "~/elsewhere", "--no-codex", "--json")
	if r.Code != 0 {
		t.Fatalf("--no-codex: %d %s", r.Code, r.Stdout)
	}
	env := convoParse(t, r)
	if len(env.Warnings) != 1 || env.Warnings[0].Code != "history-left" {
		t.Fatalf("warnings %+v", env.Warnings)
	}
	if !convoExists(filepath.Join(f.Home, "elsewhere", "mixed")) || convoExists(src) {
		t.Fatal("folder not promoted")
	}
}

// convoFakeGit records where and how git was run.
const convoFakeGit = `#!/bin/sh
printf '%s %s\n' "$(pwd -P)" "$*" > "$GIT_LOG"
`

// TestSessionPromoteGitRepo runs git init (a fake git on PATH) in the
// promoted folder, with --name and --to.
func TestSessionPromoteGitRepo(t *testing.T) {
	f := newConvoFixture(t)
	f.WriteFile(filepath.Join(f.Bin, "git"), convoFakeGit, 0o755)
	log := filepath.Join(f.Root, "git.log")
	f.Env["GIT_LOG"] = log
	id := "anything_" + convoOldStamp
	f.session(id)
	r := f.Run("session", "promote", "--id", id, "--name", "proj-x", "--to", filepath.Join(f.Root, "dest"), "--git-repo", "--json")
	if r.Code != 0 {
		t.Fatalf("promote: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	want := filepath.Join(f.Root, "dest", "proj-x") + " init -q\n"
	if got := f.ReadFile(log); got != want {
		t.Fatalf("git ran as %q, want %q", got, want)
	}
}

// TestSessionPromoteRefusals: every refusal and its exit code.
func TestSessionPromoteRefusals(t *testing.T) {
	f := newConvoFixture(t)
	bare := convoOldStamp
	f.session(bare)
	named := "taken_" + convoOldStamp
	f.session(named)
	f.HomeDir(filepath.Join("projects", "taken"))
	cases := []struct {
		args []string
		exit int
		code string
	}{
		{nil, 2, "usage"},
		{[]string{"--id", "../x"}, 2, "usage"},
		{[]string{"--id", "ghost"}, 1, "not-found"},
		{[]string{"--id", bare}, 2, "usage"},
		{[]string{"--id", bare, "--name", "a/b"}, 2, "usage"},
		{[]string{"--id", named}, 1, "exists"},
	}
	for _, c := range cases {
		r := f.Run(append([]string{"session", "promote", "--json"}, c.args...)...)
		e := convoParse(t, r)
		if r.Code != c.exit || e.Error == nil || e.Error.Code != c.code {
			t.Errorf("%v: exit %d error %+v, want %d %s", c.args, r.Code, e.Error, c.exit, c.code)
		}
	}
	f.WriteConfig(strings.Replace(convoConfig, `, "promote_root": "~/projects"`, "", 1))
	convoWantErr(t, f.Run("session", "promote", "--id", named, "--json"), 2, "usage")
}
