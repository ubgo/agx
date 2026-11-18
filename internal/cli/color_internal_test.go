package cli

import "testing"

// TestColorDecision is the --color table (spec 13 global flags,
// no-color.org): auto colours only when stderr is a terminal and NO_COLOR
// is absent or empty; an explicit always wins over NO_COLOR; never never
// colours. The terminal half of auto cannot be reached from a subprocess
// test, so the decision is checked here directly.
func TestColorDecision(t *testing.T) {
	cases := []struct {
		mode    string
		noColor string
		hasNo   bool
		tty     bool
		want    bool
	}{
		{ColorAuto, "", false, true, true},
		{ColorAuto, "", false, false, false},
		{ColorAuto, "1", true, true, false},
		{ColorAuto, "", true, true, true}, // NO_COLOR present but empty is unset (no-color.org)
		{ColorAlways, "1", true, false, true},
		{ColorAlways, "", false, false, true},
		{ColorNever, "", false, true, false},
	}
	for _, c := range cases {
		lookup := func(k string) (string, bool) {
			if k == envNoColor && c.hasNo {
				return c.noColor, true
			}
			return "", false
		}
		got := colorOn(c.mode, lookup, c.tty)
		if got != c.want {
			t.Errorf("mode %s, NO_COLOR %q (set %v), tty %v: got %v, want %v", c.mode, c.noColor, c.hasNo, c.tty, got, c.want)
		}
	}
}
