package cli_test

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/cli"
)

// TestResolvedSecretsNeedsDryRun: --resolved-secrets only changes what a
// dry run reports, so on a real launch it is a usage error naming the
// pairing (125 on a launching command, spec 14.2), never silently
// ignored, and nothing starts. With --dry-run it is accepted.
func TestResolvedSecretsNeedsDryRun(t *testing.T) {
	f := fixtureWork(t)
	for _, verb := range []string{"run", "new"} {
		r := f.Run(verb, "--profile", "work", "--resolved-secrets")
		if r.Code != 125 || !strings.Contains(r.Stderr, "--resolved-secrets only works with --dry-run; add --dry-run") {
			t.Errorf("%s: code %d, stderr %q", verb, r.Code, r.Stderr)
		}
		if _, started := r.Agent(); started {
			t.Errorf("%s: the agent started", verb)
		}
	}
	if r := f.Run("run", "--profile", "work", "--resolved-secrets", "--dry-run"); r.Code != 0 {
		t.Errorf("with --dry-run: code %d %s", r.Code, r.Stderr)
	}
}

// repeatValue is a valid value for flag h: its default when it has one
// (an enum's default is one of its values), else a plain word.
func repeatValue(h cli.HelpFlag) string {
	if h.Default != "" {
		return h.Default
	}
	switch h.Type {
	case "bool":
		return "true"
	case "int", "percent":
		return "1"
	}
	return "x"
}

// helpFlagName is cobra's own --help, created lazily at execution and
// left unwrapped (asking for help twice is harmless).
const helpFlagName = "help"

// TestSingleValueFlagRepeatRefused walks the whole command tree (the same
// declarations help --json prints): every single-value flag given twice
// is a "pick one" conflict at parse time (spec 5.1 rule 9), and every
// repeatable flag still takes two values. Root flags are tried on the
// root and on a leaf, where they arrive inherited.
func TestSingleValueFlagRepeatRefused(t *testing.T) {
	newRoot := func() *cobra.Command {
		root, _ := cli.NewRoot(cli.Env{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
		return root
	}
	var single, repeatable int
	var visit func(h cli.HelpCommand, inherited []cli.HelpFlag)
	visit = func(h cli.HelpCommand, inherited []cli.HelpFlag) {
		for _, fl := range append(slices.Clone(h.Flags), inherited...) {
			if fl.Name == helpFlagName {
				continue
			}
			root := newRoot()
			cmd, _, err := root.Find(strings.Fields(h.Path))
			if err != nil {
				t.Fatalf("%q: %v", h.Path, err)
			}
			v := repeatValue(fl)
			perr := cmd.ParseFlags([]string{"--" + fl.Name + "=" + v, "--" + fl.Name + "=" + v})
			if fl.Repeatable {
				repeatable++
				if perr != nil {
					t.Errorf("agx %s --%s twice: %v (repeatable)", h.Path, fl.Name, perr)
				}
				continue
			}
			single++
			if perr == nil {
				t.Errorf("agx %s --%s twice: accepted, want a pick-one conflict", h.Path, fl.Name)
				continue
			}
			if e := apperr.As(perr); e.Code != apperr.CodeConflict || !strings.HasSuffix(e.Message, "; pick one") {
				t.Errorf("agx %s --%s twice: %v, want a pick-one conflict", h.Path, fl.Name, perr)
			}
		}
		for _, s := range h.Commands {
			visit(s, nil)
		}
	}
	tree := cli.HelpTree(newRoot())
	visit(tree, nil)
	visit(cli.HelpCommand{Path: "usage"}, tree.Flags)
	if single < 50 || repeatable < 5 {
		t.Fatalf("walked %d single-value and %d repeatable flags; the tree walk is broken", single, repeatable)
	}
}

// TestRepeatedFlagMessage: the refusal names the flag (its long name, even
// when -p was typed) and both values, exit 2 (125 on a launching command),
// and nothing runs.
func TestRepeatedFlagMessage(t *testing.T) {
	f := fixtureWork(t)
	cases := []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"profile", "list", "--agent", "claude", "--agent", "codex"}, 2, "agx: --agent given twice (claude and codex); pick one\n"},
		{[]string{"profile", "list", "--quiet", "--quiet"}, 2, "agx: --quiet given twice; pick one\n"},
		{[]string{"run", "--profile", "work", "-p", "other"}, 125, "agx: --profile given twice (work and other); pick one\n"},
	}
	for _, c := range cases {
		r := f.Run(c.args...)
		if r.Code != c.code || r.Stderr != c.msg || r.Stdout != "" {
			t.Errorf("%v: code %d, stdout %q, stderr %q", c.args, r.Code, r.Stdout, r.Stderr)
		}
	}
	if r := f.Run("run", "--profile", "work", "--add-dir", "a", "--add-dir", "b", "--dry-run"); r.Code != 0 {
		t.Errorf("a repeatable flag twice: code %d %s", r.Code, r.Stderr)
	}
}

// SGR sequences the colour tests look for (render's palette: red errors,
// yellow warnings, bold headers and decision names).
const (
	sgrRed    = "\x1b[31m"
	sgrYellow = "\x1b[33m"
	sgrBold   = "\x1b[1m"
	sgrEsc    = "\x1b["
)

// TestColorFlag: --color takes auto, always or never (anything else is a
// usage error); "always" colours agx's own error line red and warning
// lines yellow on stderr, and the human output on stdout in the v0.3 look
// (bold headers and decision names), even with NO_COLOR set (an explicit
// flag wins, no-color.org); "never" and auto off a terminal print no
// escape on either stream; the JSON envelope is never coloured.
func TestColorFlag(t *testing.T) {
	f := fixtureWork(t)
	r := f.Run("profile", "list", "--color", "purple")
	if r.Code != 2 || r.Stderr != "agx: --color \"purple\": use one of auto, always, never\n" {
		t.Errorf("bad value: code %d, stderr %q", r.Code, r.Stderr)
	}
	r = f.Run("profile", "show", "--id", "nosuch", "--color", "always")
	if r.Code == 0 || !strings.HasPrefix(r.Stderr, sgrRed+"agx: ") {
		t.Errorf("always, error: code %d, stderr %q", r.Code, r.Stderr)
	}
	r = f.Run("run", "--profile", "work", "--dry-run", "--color", "always")
	if r.Code != 0 || !strings.Contains(r.Stderr, sgrYellow+"agx: warning: ") || !strings.HasPrefix(r.Stdout, sgrBold+"agent") {
		t.Errorf("always, dry-run warning: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	if r := f.Run("profile", "list", "--color", "always"); r.Code != 0 || !strings.HasPrefix(r.Stdout, sgrBold+"PROFILE") {
		t.Errorf("always, table: code %d, stdout %q", r.Code, r.Stdout)
	}
	if r := f.Run("profile", "list", "--color", "always", "--json"); strings.Contains(r.Stdout, sgrEsc) {
		t.Errorf("always, --json list: stdout %q", r.Stdout)
	}
	r = f.Run("run", "--profile", "work", "--color", "always")
	if !strings.Contains(r.Stderr, sgrYellow+"agx: warning: ") {
		t.Errorf("always, launch warning: stderr %q", r.Stderr)
	}
	if r := f.Run("profile", "show", "--id", "nosuch", "--color", "always", "--json"); strings.Contains(r.Stdout+r.Stderr, sgrEsc) {
		t.Errorf("always, --json: stdout %q stderr %q", r.Stdout, r.Stderr)
	}
	f.Env["NO_COLOR"] = "1"
	if r := f.Run("profile", "show", "--id", "nosuch", "--color", "always"); !strings.HasPrefix(r.Stderr, sgrRed) {
		t.Errorf("always beats NO_COLOR: stderr %q", r.Stderr)
	}
	if r := f.Run("profile", "list", "--color", "always"); !strings.HasPrefix(r.Stdout, sgrBold) {
		t.Errorf("always beats NO_COLOR on stdout: %q", r.Stdout)
	}
	for _, mode := range []string{"never", "auto"} {
		if r := f.Run("profile", "show", "--id", "nosuch", "--color", mode); r.Code == 0 || strings.Contains(r.Stderr, sgrEsc) {
			t.Errorf("%s: code %d, stderr %q", mode, r.Code, r.Stderr)
		}
		if r := f.Run("profile", "list", "--color", mode); strings.Contains(r.Stdout, sgrEsc) {
			t.Errorf("%s: stdout %q", mode, r.Stdout)
		}
	}
	delete(f.Env, "NO_COLOR")
	// Off a terminal (a pipe, as here) auto never colours stdout.
	if r := f.Run("profile", "list"); strings.Contains(r.Stdout, sgrEsc) {
		t.Errorf("auto off a terminal: stdout %q", r.Stdout)
	}
}

// TestFieldOnEmptyListUsesDeclaredFields: with no rows there is no data to
// check data.*.KEY against, so the path is checked against the command's
// declared fields (help --json): an unknown key is still "no such path"
// (exit 2) in text and --json alike, a declared key prints the empty
// result.
func TestFieldOnEmptyListUsesDeclaredFields(t *testing.T) {
	f := fixtureWork(t) // no aliases: alias list is empty
	for _, mode := range [][]string{nil, {"--json"}} {
		r := f.Run(append([]string{"alias", "list", "--field", "data.*.nosuch"}, mode...)...)
		if r.Code != 2 || !strings.Contains(r.Stderr+r.Stdout, "--field data.*.nosuch: no such path") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", mode, r.Code, r.Stdout, r.Stderr)
		}
		if r := f.Run(append([]string{"alias", "list", "--field", "data.*.name"}, mode...)...); r.Code != 0 {
			t.Errorf("%v declared key: code %d %s", mode, r.Code, r.Stderr)
		}
	}
}
