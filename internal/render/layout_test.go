package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/render"
)

// maxLine is the longest line of s.
func maxLine(s string) int {
	n := 0
	for l := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		n = max(n, len(l))
	}
	return n
}

// TestWriteTableWrapsAndMovesBelow: a long message wraps inside the
// width, the other columns keep their own widths, a "below" column is an
// indented line, and nothing exceeds the width.
func TestWriteTableWrapsAndMovesBelow(t *testing.T) {
	long := "enabled but not installed: " + strings.Repeat("plugin-name@claude-plugins-official, ", 12)
	rows := [][]string{
		{"ok", "config", "config-valid", "config is valid", "—"},
		{"warn", "home", "plugins-missing", long, "agx doctor --fix"},
	}
	var b bytes.Buffer
	spec := render.TableSpec{Head: []string{"STATUS", "AREA", "CHECK", "MESSAGE", "FIX"}, Wrap: 3, Below: []int{4}, Width: 100}
	if err := render.WriteTable(&b, spec, rows); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if maxLine(out) > 100 {
		t.Fatalf("line wider than 100 (%d):\n%s", maxLine(out), out)
	}
	if !strings.Contains(out, "    fix: agx doctor --fix") || strings.Contains(out, "fix: —") {
		t.Errorf("below column:\n%s", out)
	}
	if first := strings.SplitN(out, "\n", 3)[1]; !strings.HasPrefix(first, "ok      config  config-valid     config is valid") {
		t.Errorf("fixed columns padded to the message width:\n%q", first)
	}
}

func TestWrap(t *testing.T) {
	got := render.Wrap("aaa bbb ccc", 7)
	if strings.Join(got, "|") != "aaa bbb|ccc" {
		t.Fatalf("%q", got)
	}
	got = render.Wrap("installer,usage,identity,conversations", 20)
	for _, l := range got {
		if len(l) > 20 {
			t.Fatalf("%q", got)
		}
	}
	if got[0] != "installer,usage," {
		t.Fatalf("comma break: %q", got)
	}
}

// TestWriteDecisionsLongValue: a long value keeps its text and its "from"
// moves to the next line; the other lines stay aligned and short.
func TestWriteDecisionsLongValue(t *testing.T) {
	var b bytes.Buffer
	lines := []render.DecisionLine{
		{Name: "agent", Value: "claude", Source: "default_agent"},
		{Name: "folder", Value: "/very/long/" + strings.Repeat("deep/", 25) + "project", Source: "current folder"},
		{Name: "profile", Value: "work", Source: "--profile"},
	}
	if err := render.WriteDecisions(&b, 100, lines, render.Style{}); err != nil {
		t.Fatal(err)
	}
	out := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(out) != 4 || strings.TrimSpace(out[2]) != "from current folder" {
		t.Fatalf("%q", out)
	}
	if out[0] != "agent    claude  from default_agent" || out[3] != "profile  work    from --profile" {
		t.Fatalf("aligned lines: %q", out)
	}
}

// TestWriteDecisionsLabelColumn: the name column is the widest name plus
// two spaces, so an 8- or 10-letter name ("settings", "before_run") is
// never one space from its value.
func TestWriteDecisionsLabelColumn(t *testing.T) {
	var b bytes.Buffer
	lines := []render.DecisionLine{
		{Name: "agent", Value: "claude", Source: "default_agent"},
		{Name: "settings", Value: "apiKeyHelper", Source: "--live-login"},
		{Name: "before_run", Value: "~/bin/prep", Source: "profile work"},
	}
	if err := render.WriteDecisions(&b, 100, lines, render.Style{}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agent       claude        from default_agent",
		"settings    apiKeyHelper  from --live-login",
		"before_run  ~/bin/prep    from profile work",
	}
	if got := strings.Split(strings.TrimRight(b.String(), "\n"), "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
