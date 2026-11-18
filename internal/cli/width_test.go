package cli_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// widest returns the longest line of s and its length.
func widest(s string) (string, int) {
	best, n := "", 0
	for l := range strings.SplitSeq(s, "\n") {
		if len([]rune(l)) > n {
			best, n = l, len([]rune(l))
		}
	}
	return best, n
}

// TestHumanTablesFitTheWidth: doctor with a long plugin-list message and
// run --dry-run in a deep folder stay within COLUMNS (FIX1 item 1); the
// fixed columns are not padded to the message, and --json is unchanged.
func TestHumanTablesFitTheWidth(t *testing.T) {
	f := fixtureWork(t)
	var plugins []string
	for i := range 30 {
		plugins = append(plugins, fmt.Sprintf(`"plugin-number-%02d@claude-plugins-official": true`, i))
	}
	f.WriteFile(filepath.Join(f.Home, ".claude-work", "settings.json"), `{"enabledPlugins": {`+strings.Join(plugins, ",")+`}}`, 0o644)
	deep := f.Mkdir("work/" + strings.Repeat("a-rather-long-folder-name/", 6) + "project")
	f.Env["COLUMNS"] = "100"

	r := f.Run("doctor")
	if line, n := widest(r.Stdout); n > 100 {
		t.Errorf("doctor line of %d columns:\n%s", n, line)
	}
	if !strings.Contains(r.Stdout, "plugin-number-29") {
		t.Errorf("long message lost:\n%s", r.Stdout)
	}
	if j := f.Run("doctor", "--json"); !strings.Contains(j.Stdout, `"fix"`) || strings.Contains(j.Stdout, "\n    fix:") {
		t.Errorf("--json changed:\n%s", j.Stdout)
	}

	r = f.RunIn(deep, "", "run", "--dry-run", "--quiet")
	if r.Code != 0 {
		t.Fatalf("dry run: %d %s", r.Code, r.Stderr)
	}
	for l := range strings.SplitSeq(r.Stdout, "\n") {
		// The value column is padded to at most 55% of the width, so the
		// short lines keep "from" within label + 55 + gap + source.
		if strings.HasPrefix(l, "agent ") && len(l) > 9+55+2+len("from default_agent") {
			t.Errorf("the deep folder pushed every from far right: %q", l)
		}
	}
}
