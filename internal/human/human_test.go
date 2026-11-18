package human_test

import (
	"testing"
	"time"

	"github.com/ubgo/agx/internal/human"
)

// TestUntil: claude.ai's wording at every unit boundary, truncated to the
// minute, and "now" for a time already passed.
func TestUntil(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "now"}, {0, "now"}, {59 * time.Second, "now"},
		{time.Minute, "1 min"}, {40*time.Minute + 59*time.Second, "40 min"},
		{time.Hour, "1 hr 0 min"}, {2*time.Hour + 50*time.Minute, "2 hr 50 min"},
		{24 * time.Hour, "1 d 0 hr"}, {5*24*time.Hour + 11*time.Hour + 40*time.Minute, "5 d 11 hr"},
	} {
		if got := human.Until(c.d); got != c.want {
			t.Errorf("Until(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestAgo: coarse units, "just now" under a minute and for the future.
func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "just now"}, {30 * time.Second, "just now"}, {3 * time.Minute, "3 min ago"},
		{2*time.Hour + 59*time.Minute, "2 hr ago"}, {50 * time.Hour, "2 d ago"},
	} {
		if got := human.Ago(now, now.Add(-c.d)); got != c.want {
			t.Errorf("Ago(-%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestConversationLine: v0.3's fixed columns; an empty title reads
// (untitled), long values end in an ellipsis, the id is a bare prefix.
func TestConversationLine(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	got := human.ConversationLine(human.Conversation{Updated: now, Agent: "claude", Profile: "work", Dir: "~/p", ID: "7b1d4e20-0000"}, now)
	want := "just now     claude work       (untitled)                       ~/p                           7b1d4e20"
	if got != want {
		t.Errorf("line\n%q\nwant\n%q", got, want)
	}
	if got := human.Truncate("abcdef", 4); got != "abc…" {
		t.Errorf("Truncate %q", got)
	}
	if got := human.ShortID("abc"); got != "abc" {
		t.Errorf("ShortID %q", got)
	}
}
