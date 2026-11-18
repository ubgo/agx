package app

import (
	"path/filepath"
	"testing"
)

// TestExpandTilde: only "~" alone or "~/" at the start is the home;
// "~user", "$HOME", a "~" later in the value and inline JSON stay as
// written (no magic).
func TestExpandTilde(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"h", "me")
	for in, want := range map[string]string{
		"~":           home,
		"~/a/b.md":    filepath.Join(home, "a", "b.md"),
		"~user/a":     "~user/a",
		"$HOME/a":     "$HOME/a",
		"a/~/b":       "a/~/b",
		`{"a":"~/x"}`: `{"a":"~/x"}`,
		"":            "",
	} {
		if got := expandTilde(in, home); got != want {
			t.Errorf("expandTilde(%q) = %q, want %q", in, got, want)
		}
	}
}
