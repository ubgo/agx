package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// convoRow is the part of a conversation list row tests assert on.
type convoRow struct {
	Agent    string   `json:"agent"`
	Profiles []string `json:"profiles"`
	ID       string   `json:"id"`
	Folder   string   `json:"folder"`
	Title    string   `json:"title"`
}

// convoRows decodes conversation list data.
func convoRows(t *testing.T, e convoEnvelope) []convoRow {
	t.Helper()
	var out []convoRow
	if err := json.Unmarshal(e.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// convoIds lists the row ids in order.
func convoIDs(rs []convoRow) string {
	s := make([]string, len(rs))
	for i, r := range rs {
		s[i] = r.ID[:8]
	}
	return strings.Join(s, " ")
}

// TestConversationListFolderScope: by default only the current folder's
// conversations, from every configured home, newest first.
func TestConversationListFolderScope(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	r := f.RunIn(f.proj, "", "conversation", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	rs := convoRows(t, convoParse(t, r))
	if got := convoIDs(rs); got != "aaaabbbb dddddddd aaaaaaaa" {
		t.Fatalf("ids %s", got)
	}
	if rs[1].Profiles[0] != "work" || rs[0].Title != "Write the docs" || rs[0].Folder != f.proj {
		t.Fatalf("rows %+v", rs)
	}
	f.golden(t, "conversation-list", r.Stdout)
	text := f.RunIn(f.proj, "", "conversation", "list")
	// v0.3's resume --list rows: when, agent, profile, title, folder, the
	// id prefix; no header.
	if text.Code != 0 || !strings.Contains(text.Stdout, "Fix the parser") || strings.Contains(text.Stdout, "TITLE") ||
		!strings.Contains(text.Stdout, " claude work ") || !strings.HasSuffix(strings.TrimSpace(text.Stdout), "aaaaaaaa") {
		t.Fatalf("text: %d %s", text.Code, text.Stdout)
	}
}

// TestConversationListScopeAllQueryLimit: --scope all widens; --query
// narrows by title, folder or id; --limit caps; --profile and --agent
// filter.
func TestConversationListScopeAllQueryLimit(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	f.codexConv(t, f.proj, idCodex, "codex fake task", convoBase.Add(5*time.Hour))
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--scope", "all"}, "0199e1a0 cccccccc aaaabbbb dddddddd aaaaaaaa"},
		{[]string{"--scope", "all", "--query", "OTHER"}, "cccccccc"},
		{[]string{"--scope", "all", "--query", "dddd"}, "dddddddd"},
		{[]string{"--scope", "all", "--limit", "2"}, "0199e1a0 cccccccc"},
		{[]string{"--scope", "all", "--profile", "work"}, "dddddddd"},
		{[]string{"--agent", "codex"}, "0199e1a0"},
	}
	for _, c := range cases {
		r := f.RunIn(f.proj, "", append([]string{"conversation", "list", "--json"}, c.args...)...)
		if r.Code != 0 {
			t.Fatalf("%v: code %d %s", c.args, r.Code, r.Stdout)
		}
		if got := convoIDs(convoRows(t, convoParse(t, r))); got != c.want {
			t.Errorf("%v: ids %s, want %s", c.args, got, c.want)
		}
	}
}

// TestConversationListByFolder pins the --by-folder shape.
func TestConversationListByFolder(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	r := f.RunIn(f.proj, "", "conversation", "list", "--scope", "all", "--by-folder", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	f.golden(t, "conversation-list-by-folder", r.Stdout)
}

// TestConversationListRefusals: bad values and unknown names.
func TestConversationListRefusals(t *testing.T) {
	f := newConvoFixture(t)
	convoWantErr(t, f.Run("conversation", "list", "--scope", "everywhere", "--json"), 2, "usage")
	convoWantErr(t, f.Run("conversation", "list", "--limit", "-1", "--json"), 2, "usage")
	convoWantErr(t, f.Run("conversation", "list", "--agent", "nope", "--json"), 2, "usage")
	convoWantErr(t, f.Run("conversation", "list", "--profile", "ghost", "--json"), 1, "not-found")
	convoWantErr(t, f.Run("conversation", "list", "--profile", "codex", "--agent", "claude", "--json"), 2, "conflict")
	convoWantErr(t, f.Run("conversation", "list", "stray", "--json"), 2, "usage")
}

// TestConversationListFooterNamesUnconfiguredHomes is spec 6.2: homes on
// disk that no profile uses are named in a footer, never listed.
func TestConversationListFooterNamesUnconfiguredHomes(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	old := f.HomeDir(".claude-old")
	f.WriteFile(filepath.Join(old, "settings.json"), "{}", 0o600)
	f.claudeConv(t, ".claude-old", f.proj, "eeeeeeee-0000-4000-8000-000000000005", "hidden", convoBase)
	r := f.RunIn(f.proj, "", "conversation", "list", "--json")
	e := convoParse(t, r)
	if strings.Contains(convoIDs(convoRows(t, e)), "eeeeeeee") {
		t.Fatalf("an unconfigured home was listed")
	}
	if len(e.Warnings) != 1 || e.Warnings[0].Code != "unconfigured-home" || !strings.Contains(e.Warnings[0].Message, "claude home ~/.claude-old is not in config") {
		t.Fatalf("warnings %+v", e.Warnings)
	}
	text := f.RunIn(f.proj, "", "conversation", "list")
	if !strings.Contains(text.Stderr, "agx profile import --discovered") {
		t.Fatalf("footer missing on stderr: %s", text.Stderr)
	}
	quiet := f.RunIn(f.proj, "", "conversation", "list", "--quiet")
	if strings.Contains(quiet.Stderr, "not in config") {
		t.Fatalf("--quiet printed the footer")
	}
	// --profile narrows the homes listed, not the homes in config: the
	// other profiles' homes (~/.claude of personal) are never in the footer.
	narrow := convoParse(t, f.RunIn(f.proj, "", "conversation", "list", "--profile", "work", "--json"))
	if len(narrow.Warnings) != 1 || !strings.Contains(narrow.Warnings[0].Message, "claude home ~/.claude-old is not in config") {
		t.Fatalf("--profile work warnings %+v", narrow.Warnings)
	}
}

// TestConversationShow: exact id and unique prefix; turns, size, live.
func TestConversationShow(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	r := f.Run("conversation", "show", "--id", "aaaabb", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	var d struct {
		ID    string `json:"id"`
		Turns *int   `json:"turns"`
		Size  int64  `json:"size_bytes"`
		Live  *bool  `json:"live"`
	}
	if err := json.Unmarshal(convoParse(t, r).Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.ID != idBravo || d.Turns == nil || *d.Turns != 2 || d.Size == 0 || d.Live == nil || *d.Live {
		t.Fatalf("detail %+v turns %v live %v", d, d.Turns, d.Live)
	}
	f.golden(t, "conversation-show", r.Stdout)
	text := f.Run("conversation", "show", "--id", idAlpha)
	if text.Code != 0 || !strings.Contains(text.Stdout, "Fix the parser") || !strings.Contains(text.Stdout, "turns") {
		t.Fatalf("text %d %s", text.Code, text.Stdout)
	}
	// Times read in the iso preset by default and in --date-format when
	// given (JSON keeps RFC 3339 either way).
	if !strings.Contains(text.Stdout, "updated   2026-11-10 10:00") {
		t.Errorf("default times:\n%s", text.Stdout)
	}
	us := f.Run("conversation", "show", "--id", idAlpha, "--date-format", "us")
	if us.Code != 0 || !strings.Contains(us.Stdout, "updated   Nov 10 10:00 AM") {
		t.Errorf("--date-format us: %d\n%s", us.Code, us.Stdout)
	}
}

// TestDateFormatFlagRefused: a --date-format that does not parse is a
// usage error (exit 2) on any command, naming the tokens and presets.
func TestDateFormatFlagRefused(t *testing.T) {
	f := newConvoFixture(t)
	for _, args := range [][]string{
		{"conversation", "list", "--date-format", "YYYY-Q"},
		{"usage", "--cached", "--date-format", "YYYY-Q"},
		{"profile", "list", "--date-format", ""},
	} {
		r := f.Run(args...)
		if r.Code != 2 || !strings.Contains(r.Stderr, "--date-format") || !strings.Contains(r.Stderr, "presets: iso, short, long, us") {
			t.Errorf("%v: %d %s", args, r.Code, r.Stderr)
		}
	}
}

// TestConversationShowAmbiguousAndMissing: a prefix matching two
// conversations is exit 2 listing them; an unknown id is exit 1.
func TestConversationShowAmbiguousAndMissing(t *testing.T) {
	f := newConvoFixture(t)
	f.seedConversations(t)
	e := convoWantErr(t, f.Run("conversation", "show", "--id", "aaaa", "--json"), 2, "ambiguous")
	if !strings.Contains(e.Error.Message, "matches 2 conversations") || !strings.Contains(e.Error.Hint, idAlpha) {
		t.Fatalf("error %+v", e.Error)
	}
	convoWantErr(t, f.Run("conversation", "show", "--id", "ffff", "--json"), 1, "not-found")
	convoWantErr(t, f.Run("conversation", "show", "--json"), 2, "usage")
}

// convoMoveData is the part of conversation move data tests assert on.
type convoMoveData struct {
	DryRun  bool `json:"dry_run"`
	Folders []struct {
		Folder        string   `json:"folder"`
		Conversations []string `json:"conversations"`
	} `json:"folders"`
	Backups []string `json:"backups"`
}

// TestConversationMoveDryRunThenApply: the source home is the one other
// home with history here; --dry-run changes nothing; the move renames the
// transcripts into the target and keeps agentio's backup.
func TestConversationMoveDryRunThenApply(t *testing.T) {
	f := newConvoFixture(t)
	f.claudeConv(t, ".claude", f.proj, idAlpha, "Fix the parser", convoBase)
	src := f.claudeConv(t, ".claude", f.proj, idBravo, "Write the docs", convoBase.Add(time.Hour))
	r := f.RunIn(f.proj, "", "conversation", "move", "--to-profile", "work", "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	f.golden(t, "conversation-move-dry-run", r.Stdout)
	if !convoExists(src) {
		t.Fatal("--dry-run moved a file")
	}
	r = f.RunIn(f.proj, "", "conversation", "move", "--to-profile", "work", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	var m convoMoveData
	if err := json.Unmarshal(convoParse(t, r).Data, &m); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(f.Home, ".claude-work", "projects", convoClaudeKey(f.proj), idBravo+".jsonl")
	if convoExists(src) || !convoExists(dst) || len(m.Folders) != 1 || len(m.Folders[0].Conversations) != 2 || len(m.Backups) == 0 {
		t.Fatalf("after move: src %v dst %v data %+v", convoExists(src), convoExists(dst), m)
	}
	list := convoParse(t, f.RunIn(f.proj, "", "conversation", "list", "--json", "--profile", "work"))
	if got := convoIDs(convoRows(t, list)); got != "aaaabbbb aaaaaaaa" {
		t.Fatalf("work home after move: %s", got)
	}
}

// TestConversationMoveOneIDNoBackup: --id moves only that conversation;
// --no-backup leaves no backup folder behind.
func TestConversationMoveOneIDNoBackup(t *testing.T) {
	f := newConvoFixture(t)
	keep := f.claudeConv(t, ".claude", f.proj, idAlpha, "Fix the parser", convoBase)
	moved := f.claudeConv(t, ".claude", f.proj, idBravo, "Write the docs", convoBase.Add(time.Hour))
	r := f.RunIn(f.proj, "", "conversation", "move", "--to-profile", "work", "--from-profile", "personal", "--id", "aaaabb", "--no-backup", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	if !convoExists(keep) || convoExists(moved) {
		t.Fatalf("keep %v moved-still-there %v", convoExists(keep), convoExists(moved))
	}
	ents, err := os.ReadDir(f.Home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.Contains(e.Name(), ".agentio-bak-") {
			t.Fatalf("backup kept with --no-backup: %s", e.Name())
		}
	}
}

// TestConversationMoveScopeAll moves every folder of the source home.
func TestConversationMoveScopeAll(t *testing.T) {
	f := newConvoFixture(t)
	f.claudeConv(t, ".claude", f.proj, idAlpha, "Fix the parser", convoBase)
	f.claudeConv(t, ".claude", f.Mkdir("work/other"), idOther, "Other", convoBase.Add(time.Hour))
	r := f.Run("conversation", "move", "--to-profile", "work", "--from-profile", "personal", "--scope", "all", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stdout)
	}
	var m convoMoveData
	if err := json.Unmarshal(convoParse(t, r).Data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Folders) != 2 {
		t.Fatalf("folders %+v", m.Folders)
	}
}

// TestConversationMoveRefusals: every refusal names its reason and exit.
func TestConversationMoveRefusals(t *testing.T) {
	f := newConvoFixture(t)
	f.claudeConv(t, ".claude", f.proj, idAlpha, "Fix the parser", convoBase)
	run := func(args ...string) convoEnvelope {
		t.Helper()
		return convoParse(t, f.RunIn(f.proj, "", append([]string{"conversation", "move", "--json"}, args...)...))
	}
	cases := []struct {
		args []string
		exit int
		code string
	}{
		{nil, 2, "usage"},
		{[]string{"--to-profile", "ghost"}, 1, "not-found"},
		{[]string{"--to-profile", "work", "--from-profile", "codex"}, 2, "usage"},
		{[]string{"--to-profile", "work", "--from-profile", "work"}, 2, "usage"},
		{[]string{"--to-profile", "work", "--id", "x", "--picker", "chat"}, 2, "conflict"},
		{[]string{"--to-profile", "work", "--dir", ".", "--scope", "all"}, 2, "conflict"},
		{[]string{"--to-profile", "work", "--dir", ".", "--picker", "folder"}, 2, "conflict"},
		{[]string{"--to-profile", "work", "--picker", "list"}, 2, "usage"},
		{[]string{"--to-profile", "work", "--picker", "chat"}, 2, "no-tty"},
		{[]string{"--to-profile", "work", "--id", "zzzz"}, 1, "not-found"},
		{[]string{"--to-profile", "codex"}, 4, "unsupported"},
	}
	for _, c := range cases {
		r := f.RunIn(f.proj, "", append([]string{"conversation", "move", "--json"}, c.args...)...)
		e := convoParse(t, r)
		if r.Code != c.exit || e.Error == nil || e.Error.Code != c.code {
			t.Errorf("%v: exit %d error %+v, want %d %s", c.args, r.Code, e.Error, c.exit, c.code)
		}
	}
	e := run("--to-profile", "codex")
	if !strings.Contains(e.Error.Message, "SQLite") {
		t.Errorf("unsupported message lacks the agent's reason: %s", e.Error.Message)
	}
	// Nothing here in another home: the source cannot be found.
	empty := f.Mkdir("work/empty")
	r := f.RunIn(empty, "", "conversation", "move", "--to-profile", "work", "--json")
	convoWantErr(t, r, 1, "not-found")
}

// TestConversationMoveAmbiguousSource: two other homes with history and
// no --from-profile is exit 2 naming --from-profile.
func TestConversationMoveAmbiguousSource(t *testing.T) {
	f := newConvoFixture(t)
	f.HomeDir(".claude-third")
	f.WriteConfig(strings.Replace(convoConfig, `"codex":`, `"third": { "agent": "claude", "home": "~/.claude-third" },
    "codex":`, 1))
	f.claudeConv(t, ".claude", f.proj, idAlpha, "one", convoBase)
	f.claudeConv(t, ".claude-third", f.proj, idOther, "two", convoBase)
	e := convoWantErr(t, f.RunIn(f.proj, "", "conversation", "move", "--to-profile", "work", "--json"), 2, "ambiguous")
	if !strings.Contains(e.Error.Message, "--from-profile") {
		t.Fatalf("message %s", e.Error.Message)
	}
}

// TestConversationListPartial: one home unreadable, the others answer:
// exit 3 with error.code partial, and the rows that answered are kept.
func TestConversationListPartial(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 folder")
	}
	f := newConvoFixture(t)
	f.seedConversations(t)
	locked := filepath.Join(f.Home, ".claude-work", "projects")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // let t.TempDir remove it
	r := f.RunIn(f.proj, "", "conversation", "list", "--scope", "all", "--json")
	e := convoWantErr(t, r, 3, "partial")
	if got := convoIDs(convoRows(t, e)); got != "cccccccc aaaabbbb aaaaaaaa" {
		t.Fatalf("rows kept: %q", got)
	}
}
