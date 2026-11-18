package datefmt_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/datefmt"
)

// evening and morning are fixed instants: Thursday 19 November 2026 at
// 23:30:07 (two-digit hour, pm) and 09:05:03 (one-digit hour, am).
var (
	evening = time.Date(2026, 11, 19, 23, 30, 7, 0, time.UTC)
	morning = time.Date(2026, 11, 19, 9, 5, 3, 0, time.UTC)
)

// TestTokens: every token alone, on a pm and an am time, so padding and
// the 12-hour clock are pinned.
func TestTokens(t *testing.T) {
	for _, c := range []struct {
		token, evening, morning string
	}{
		{"YYYY", "2026", "2026"},
		{"MM", "11", "11"},
		{"MMM", "Nov", "Nov"},
		{"DD", "19", "19"},
		{"D", "19", "19"},
		{"ddd", "Thu", "Thu"},
		{"dddd", "Thursday", "Thursday"},
		{"HH", "23", "09"},
		{"H", "23", "9"},
		{"hh", "11", "09"},
		{"h", "11", "9"},
		{"mm", "30", "05"},
		{"ss", "07", "03"},
		{"a", "pm", "am"},
		{"A", "PM", "AM"},
	} {
		f, err := datefmt.Parse(c.token)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.token, err)
		}
		if got := f.Date(evening); got != c.evening {
			t.Errorf("%s on 23:30 = %q, want %q", c.token, got, c.evening)
		}
		if got := f.Date(morning); got != c.morning {
			t.Errorf("%s on 09:05 = %q, want %q", c.token, got, c.morning)
		}
	}
	// Every listed token is covered above.
	if n := len(datefmt.Tokens()); n != 15 {
		t.Errorf("Tokens() has %d tokens, the table 15", n)
	}
}

// TestPresets: each preset's whole format and clock part.
func TestPresets(t *testing.T) {
	for _, c := range []struct {
		name, date, clock string
	}{
		{datefmt.PresetISO, "2026-11-19 23:30", "23:30"},
		{datefmt.PresetShort, "Thu 23:30", "23:30"},
		{datefmt.PresetLong, "Thu 19 Nov 23:30", "23:30"},
		{datefmt.PresetUS, "Nov 19 11:30 PM", "11:30 PM"},
	} {
		f, err := datefmt.Parse(c.name)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.name, err)
		}
		if got := f.Date(evening); got != c.date {
			t.Errorf("%s Date = %q, want %q", c.name, got, c.date)
		}
		if got := f.Clock(evening); got != c.clock {
			t.Errorf("%s Clock = %q, want %q", c.name, got, c.clock)
		}
		if p := datefmt.Preset(c.name); p.Date(evening) != c.date || p.String() != c.name {
			t.Errorf("Preset(%s) = %q (%s), want %q", c.name, p.Date(evening), p.String(), c.date)
		}
	}
	if n := len(datefmt.Presets()); n != 4 {
		t.Errorf("Presets() has %d, the table 4", n)
	}
}

// TestCustom: literals, brackets (whose words are never tokens), the
// clock span and its default when a format has no clock token.
func TestCustom(t *testing.T) {
	for _, c := range []struct {
		spec, date, clock string
	}{
		{"YYYY-MM-DD[T]HH:mm:ss", "2026-11-19T23:30:07", "23:30:07"},
		{"dddd, D MMM [at] h:mm a", "Thursday, 19 Nov at 11:30 pm", "11:30 pm"},
		{"[Mon] DD/MM", "Mon 19/11", "23:30"},
		{"H[h]mm", "23h30", "23h30"},
		{"YYYY", "2026", "23:30"},
	} {
		f, err := datefmt.Parse(c.spec)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.spec, err)
		}
		if got := f.Date(evening); got != c.date {
			t.Errorf("%q Date = %q, want %q", c.spec, got, c.date)
		}
		if got := f.Clock(evening); got != c.clock {
			t.Errorf("%q Clock = %q, want %q", c.spec, got, c.clock)
		}
		if f.String() != c.spec {
			t.Errorf("String() = %q, want %q", f.String(), c.spec)
		}
	}
}

// TestErrors: each refusal says what is wrong and lists the tokens and
// presets, so the user can fix the value from the message alone.
func TestErrors(t *testing.T) {
	for _, c := range []struct {
		spec, want string
	}{
		{"", "empty"},
		{"   ", "empty"},
		{"YYYY-Q", `unknown token "Q"`},
		{"DDD", `unknown token "DDD"`},
		{"MMMM", `unknown token "MMMM"`},
		{"dd", `unknown token "dd"`},
		{"HH:mm [at", "unclosed ["},
		{"[only text] -", "no token"},
		{"ISO", `unknown token "I"`},
	} {
		_, err := datefmt.Parse(c.spec)
		if err == nil {
			t.Errorf("Parse(%q) = nil error, want %q", c.spec, c.want)
			continue
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, c.want) {
			t.Errorf("Parse(%q) = %q, want it to start with %q", c.spec, msg, c.want)
		}
		if !strings.Contains(msg, datefmt.Help()) {
			t.Errorf("Parse(%q) = %q, want the token and preset list", c.spec, msg)
		}
	}
}

// TestRoundToNearestMinute: Date and Clock round to the nearest minute
// (:29 down, :30 and :31 up), or to the second in a format with ss.
func TestRoundToNearestMinute(t *testing.T) {
	at := func(sec int) time.Time { return time.Date(2026, 11, 18, 22, 19, sec, 0, time.UTC) }
	long, us := datefmt.Preset(datefmt.PresetLong), datefmt.Preset(datefmt.PresetUS)
	for _, c := range []struct {
		sec      int
		long, us string
	}{{29, "22:19", "10:19 PM"}, {30, "22:20", "10:20 PM"}, {31, "22:20", "10:20 PM"}} {
		if got := long.Clock(at(c.sec)); got != c.long {
			t.Errorf("%ds long clock: %q, want %q", c.sec, got, c.long)
		}
		if got := us.Clock(at(c.sec)); got != c.us {
			t.Errorf("%ds us clock: %q, want %q", c.sec, got, c.us)
		}
		if got := datefmt.RoundMinute(at(c.sec)).Format("15:04"); got != c.long {
			t.Errorf("%ds RoundMinute: %q, want %q", c.sec, got, c.long)
		}
	}
	secs, err := datefmt.Parse("HH:mm:ss")
	if err != nil {
		t.Fatal(err)
	}
	if got := secs.Date(at(31).Add(600 * time.Millisecond)); got != "22:19:32" {
		t.Errorf("seconds format: %q", got)
	}
}
