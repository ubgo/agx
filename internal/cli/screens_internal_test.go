package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/testkit"
	"github.com/ubgo/agx/sessions"
)

// Goldens of the screens restored to agx v0.3's look besides usage. Each
// renders fixed data at a fixed clock, plain and coloured, so a change to
// what a person reads shows up as a golden diff to check against v0.3.

// screenNow is the clock of these goldens.
var screenNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// bothStyles runs fn plain and coloured into goldens name and name-color.
func bothStyles(t *testing.T, name string, fn func(b *strings.Builder, st render.Style) error) {
	t.Helper()
	for _, c := range []struct {
		suffix string
		on     bool
	}{{"", false}, {"-color", true}} {
		var b strings.Builder
		if err := fn(&b, render.NewStyle(c.on)); err != nil {
			t.Fatal(err)
		}
		testkit.GoldenText(t, name+c.suffix, b.String())
	}
}

// TestProfileListGolden pins v0.3's `agx profiles` look: the file's order,
// "*" on each agent's default, ACCOUNT as email · plan, STATE with the
// time to expiry, a missing home in red.
func TestProfileListGolden(t *testing.T) {
	exp := screenNow.Add(5*24*time.Hour + 3*time.Hour)
	rows := []app.ProfileRow{
		{Name: "personal", Agent: "claude", Home: "~/.claude", Default: true, Billing: "plan", Login: "home login", HomeExists: true, Account: "me@example.com", PlanLabel: "Max (20x)", State: app.LoginStateOK, ExpiresAt: &exp},
		{Name: "work", Agent: "claude", Home: "~/.claude-work", Billing: "plan", Login: "carol", HomeExists: true, Account: "work@example.com", State: app.LoginStateExpired},
		{Name: "old", Agent: "claude", Home: "~/.claude-old", Billing: "plan", Login: "home login", State: app.LoginStateNone},
		{Name: "codex", Agent: "codex", Home: "~/.codex", Default: true, Billing: "plan", Login: "home login", HomeExists: true, State: app.LoginStateNone},
		{Name: "pi", Agent: "pi", Home: "~/.pi/agent", Default: true, Billing: "api", Login: "anthropic-key", HomeExists: true, State: app.LoginStateUnknown},
	}
	bothStyles(t, "profile-list", func(b *strings.Builder, st render.Style) error { return profileListText(b, rows, false, screenNow, st) })
}

// TestDoctorGolden pins v0.3's doctor look: a mark per check, the subject
// padded, a long finding wrapped under itself, the fix on its own line.
func TestDoctorGolden(t *testing.T) {
	checks := []app.DoctorCheck{
		{Area: "config", ID: "config-valid", Status: agentio.CheckOK, Message: "~/.agx/config.jsonc is valid"},
		{Area: "reference", ID: "reference-variable", Status: agentio.CheckWarn, Message: "logins.nora.token: NORA_TOKEN is not set in this environment"},
		{Area: "shell", ID: "shell-layer", Status: agentio.CheckInfo, Message: "shell layer not loaded: new and resume leave this shell where it is", Fix: `eval "$(agx shell-init --shell zsh)"`},
		{Area: "home", Agent: "claude", Home: "~/.claude", ID: "plugins", Status: agentio.CheckFail, Message: "enabled but not installed: " + strings.Repeat("plugin-name@claude-plugins-official ", 4)},
		{Area: "home", Agent: "codex", Home: "~/.codex", ID: "login", Status: agentio.CheckSkip, Message: "not checked"},
	}
	bothStyles(t, "doctor", func(b *strings.Builder, st render.Style) error { return doctorText(b, checks, 100, st) })
}

// TestSessionListGolden pins v0.3's `sessions ls`: LAST ACTIVITY as "ago",
// HISTORY dim when none and amber when an agent could not answer.
func TestSessionListGolden(t *testing.T) {
	list := []sessions.Folder{
		{Name: "20261118-1130-api", Modified: screenNow.Add(-3 * time.Minute), Files: 4, History: []string{"claude", "codex"}},
		{Name: "20261116-0900-demo", Modified: screenNow.Add(-50 * time.Hour), Files: 0},
		{Name: "scratch", Modified: screenNow.Add(-2 * time.Hour), Files: 1, HistoryUnknown: true},
	}
	bothStyles(t, "session-list", func(b *strings.Builder, st render.Style) error { return sessionListText(b, list, screenNow, st) })
}

// TestConversationLinesGolden pins v0.3's `resume --list` rows: fixed
// columns, a long title and folder cut with an ellipsis, the id prefix.
func TestConversationLinesGolden(t *testing.T) {
	rows := []app.ConversationRow{
		{Agent: "claude", Profiles: []string{"work"}, ID: "7b1d4e20-0000-4000-8000-000000000002", Title: "Review the API", Folder: "/home/you/projects/client", Updated: screenNow.Add(-5 * 24 * time.Hour)},
		{Agent: "claude", Profiles: []string{"personal", "kimi"}, ID: "3f2a9c1e-0000-4000-8000-000000000001", Title: "A very long conversation title that does not fit", Folder: "/home/you/projects/some/deeply/nested/folder/name", Updated: screenNow.Add(-90 * time.Second)},
		{Agent: "codex", Profiles: []string{"codex"}, ID: "abc", Folder: "/home/you", Updated: screenNow.Add(-3 * time.Hour)},
	}
	var b strings.Builder
	if err := conversationLines(&b, rows, screenNow, "/home/you"); err != nil {
		t.Fatal(err)
	}
	testkit.GoldenText(t, "conversation-list", b.String())
	folders := []app.FolderRow{{Folder: "/home/you/projects/client", Conversations: 2, Updated: screenNow.Add(-5 * 24 * time.Hour), Profiles: []string{"work", "personal"}}}
	b.Reset()
	if err := folderLines(&b, folders, screenNow, "/home/you"); err != nil {
		t.Fatal(err)
	}
	testkit.GoldenText(t, "conversation-folders", b.String())
}

// TestWhoamiGolden pins v0.3's "label:  value" lines, inside and outside
// a session.
func TestWhoamiGolden(t *testing.T) {
	var b strings.Builder
	in := app.Whoami{InSession: true, RunID: "20261118-120000-abcd", Agent: "claude", Profile: "work", Login: "carol", Home: "~/.claude-work"}
	if err := whoamiText(&b, in); err != nil {
		t.Fatal(err)
	}
	b.WriteString("---\n")
	out := app.Whoami{TerminalLogins: []app.TerminalLogin{{Variable: "AGX_LOGIN_CLAUDE", Login: "alice"}}}
	if err := whoamiText(&b, out); err != nil {
		t.Fatal(err)
	}
	testkit.GoldenText(t, "whoami", b.String())
}

// TestPlanTableGolden pins --plan's v0.3 plan table and its errors.
func TestPlanTableGolden(t *testing.T) {
	since := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	rows := []app.UsageRow{
		{Kind: app.RowHome, Name: "personal", Agent: "claude", Account: "me@example.com", Plan: &app.UsagePlan{Plan: "Max (20x)", Status: "active", StartedAt: &since}},
		{Kind: app.RowHome, Name: "codex", Agent: "codex", Plan: &app.UsagePlan{Error: "not logged in"}},
	}
	bothStyles(t, "usage-plan", func(b *strings.Builder, st render.Style) error { return writePlanTable(b, rows, st) })
}
