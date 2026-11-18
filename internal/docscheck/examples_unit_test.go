package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"
)

// TestParseRegions covers nested regions behind each comment token, the
// whole-file body without marker lines, and every malformed-marker error
// in "file:line: reason" form.
func TestParseRegions(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		"top",
		"#region outer",
		"a",
		"  // #region inner",
		"b",
		"# #endregion inner",
		"<!-- #region html -->",
		"c",
		"<!-- #endregion -->",
		"#endregion",
		"#regionless line stays",
	}, "\n"))
	rs, err := parseRegions("r.txt", lines)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"outer": {"a", "b", "c"}, "inner": {"b"}, "html": {"c"}}
	if !reflect.DeepEqual(rs.regions, want) {
		t.Errorf("regions %q, want %q", rs.regions, want)
	}
	if body := []string{"top", "a", "b", "c", "#regionless line stays"}; !reflect.DeepEqual(rs.body, body) {
		t.Errorf("body %q, want %q", rs.body, body)
	}
	bad := map[string]string{
		"#region a\n#region a\n#endregion\n#endregion": "b.txt:2: region \"a\" already opened at line 1",
		"x\n#endregion":                      "b.txt:2: #endregion without an open region",
		"#region a\n#region b\n#endregion a": "b.txt:3: #endregion a closes region \"b\" (opened at line 2)",
		"#region a\nx":                       "b.txt:1: region \"a\" is never closed",
	}
	for in, msg := range bad {
		if _, err := parseRegions("b.txt", splitLines(in)); err == nil || err.Error() != msg {
			t.Errorf("parseRegions(%q) error %v, want %q", in, err, msg)
		}
	}
	if _, ok := rs.region("nope"); ok {
		t.Error("missing region found")
	}
	if body, ok := rs.region(""); !ok || !reflect.DeepEqual(body, rs.body) {
		t.Errorf("whole file %q, want the body", body)
	}
}

// memSource is an in-memory snippetSource.
func memSource(files map[string]string) snippetSource {
	return func(rel string) ([]string, error) {
		s, ok := files[rel]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return splitLines(s), nil
	}
}

// TestCheckSnippets covers every snippet outcome on in-memory files: a
// match despite trailing spaces, a whole-file snippet, a changed line, a
// short block, a missing file or region, a source outside examples/, a
// marker with no fence and a marker inside a fence (ignored).
func TestCheckSnippets(t *testing.T) {
	src := memSource(map[string]string{
		"examples/x/session.txt":  "#: c\n#region a\n$ agx version\nv1   \n\n#endregion\n",
		"examples/x/config.jsonc": "{\n  // #region p\n  \"version\": 1\n  // #endregion\n}\n",
		"examples/x/broken.txt":   "a\n#region open\nb\n",
	})
	doc := splitLines(strings.Join([]string{
		"<!-- snippet: examples/x/session.txt#a -->", // 1: matches (trailing spaces and blank ignored)
		"```sh",
		"$ agx version",
		"v1",
		"```",
		"<!-- snippet: examples/x/config.jsonc -->", // 6: whole file, markers dropped
		"```jsonc",
		"{",
		"  \"version\": 1",
		"}",
		"```",
		"<!-- snippet: examples/x/session.txt#a -->", // 12: differs
		"```sh",
		"$ agx version",
		"v2",
		"```",
		"<!-- snippet: examples/x/session.txt#a -->", // 17: shorter
		"```sh",
		"$ agx version",
		"```",
		"<!-- snippet: examples/x/gone.txt -->", // 21: missing file
		"```",
		"```",
		"<!-- snippet: examples/x/session.txt#b -->", // 24: missing region
		"```",
		"```",
		"<!-- snippet: docs/x.md -->", // 27: not an example
		"```",
		"```",
		"<!-- snippet: examples/x/session.txt -->", // 30: no fence
		"text",
		"```md",
		"<!-- snippet: examples/x/gone.txt -->", // inside a fence: ignored
		"```",
		"<!-- snippet: examples/x/broken.txt -->", // 35: source with an unclosed region
		"```",
		"```",
	}, "\n"))
	got := checkSnippets("d.md", doc, src)
	want := []string{
		`d.md:12: snippet differs from examples/x/session.txt#a (first difference at line 2: doc "v2" vs file "v1")`,
		`d.md:17: snippet differs from examples/x/session.txt#a (first difference at line 2: doc <end> vs file "v1")`,
		"d.md:21: snippet source examples/x/gone.txt: file does not exist",
		`d.md:24: snippet source examples/x/session.txt#b: no region "b"`,
		`d.md:27: snippet source "docs/x.md" must be a clean path under examples/`,
		"d.md:30: snippet marker not followed by a fenced block",
		`d.md:35: snippet source examples/x/broken.txt:2: region "open" is never closed`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("problems\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestUpdateSnippets covers the -update rewrite on in-memory files: a
// differing block gets the source (trailing whitespace and blank lines
// dropped), a block in a list item keeps the fence's indentation on every
// non-blank line, a shorter block grows, a matching block stays as written
// (its trailing spaces too), and a malformed snippet is left alone and
// reported with the line it has in the original doc.
func TestUpdateSnippets(t *testing.T) {
	src := memSource(map[string]string{
		"examples/x/session.txt": "#region a\n$ agx version\nv1   \n\nv2\n\n#endregion\n#region b\n$ agx help\n#endregion\n",
	})
	doc := splitLines(strings.Join([]string{
		"<!-- snippet: examples/x/session.txt#a -->", // 1: differs
		"```sh",
		"$ agx version",
		"v0",
		"```",
		"- item", // 6
		"",
		"  <!-- snippet: examples/x/session.txt#a -->", // 8: indented, shorter
		"  ```sh",
		"  $ agx version",
		"  ```",
		"<!-- snippet: examples/x/session.txt#b -->", // 12: matches
		"```sh",
		"$ agx help   ",
		"```",
		"<!-- snippet: examples/x/session.txt#gone -->", // 16: missing region
		"```sh",
		"old",
		"```",
	}, "\n"))
	got, probs := updateSnippets("d.md", doc, src)
	want := []string{
		"<!-- snippet: examples/x/session.txt#a -->",
		"```sh",
		"$ agx version",
		"v1",
		"",
		"v2",
		"```",
		"- item",
		"",
		"  <!-- snippet: examples/x/session.txt#a -->",
		"  ```sh",
		"  $ agx version",
		"  v1",
		"",
		"  v2",
		"  ```",
		"<!-- snippet: examples/x/session.txt#b -->",
		"```sh",
		"$ agx help   ",
		"```",
		"<!-- snippet: examples/x/session.txt#gone -->",
		"```sh",
		"old",
		"```",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("updated\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	wantProbs := []string{`d.md:16: snippet source examples/x/session.txt#gone: no region "gone"`}
	if !reflect.DeepEqual(probs, wantProbs) {
		t.Errorf("problems %q, want %q", probs, wantProbs)
	}
	// The updated blocks above it grew by 5 lines, so the marker moved.
	wantRest := []string{`d.md:21: snippet source examples/x/session.txt#gone: no region "gone"`}
	if rest := checkSnippets("d.md", got, src); !reflect.DeepEqual(rest, wantRest) {
		t.Errorf("after update the gate reports %q, want only %q", rest, wantRest)
	}
}

// TestSnippetFixture runs the snippet gate on testdata/snippets.md against
// the real seed examples: a whole-file snippet, a region of a session, a
// JSONC region and a snippet inside an indented list fence.
func TestSnippetFixture(t *testing.T) {
	root := repoRoot(t)
	const fixture = "internal/docscheck/testdata/snippets.md"
	// fixtureSnippets is the number of snippet markers in the fixture, so
	// a marker the gate stops recognizing fails instead of passing empty.
	const fixtureSnippets = 4
	lines := readLines(t, root, fixture)
	n := 0
	for _, l := range lines {
		if snippetMarker.MatchString(l) {
			n++
		}
	}
	if n != fixtureSnippets {
		t.Fatalf("%s has %d snippets, want %d", fixture, n, fixtureSnippets)
	}
	for _, p := range checkSnippets(fixture, lines, repoSnippetSource(root)) {
		t.Error(p)
	}
}

// TestParseSession covers options, comments, region markers, exit lines
// and stdin redirects, and every malformed-session problem in
// "file:line: reason" form.
func TestParseSession(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		"#: comment",
		"#options dir=~/projects/client env=A=1 env=B=x=y",
		"",
		"#region one",
		"$ agx version",
		"agx v2",
		"",
		"#endregion",
		"$ agx run --login a --home-login",
		"agx: refused",
		"[exit 125]",
		"",
		"$ agx login add --stdin < ~/token.txt",
		"# a shell-init comment is output",
	}, "\n"))
	s, probs := parseSession("s.txt", lines)
	if probs != nil {
		t.Fatalf("problems %q", probs)
	}
	if s.dir != "~/projects/client" || !reflect.DeepEqual(s.env, map[string]string{"A": "1", "B": "x=y"}) {
		t.Errorf("options %q %v", s.dir, s.env)
	}
	if len(s.commands) != 3 {
		t.Fatalf("commands %+v", s.commands)
	}
	c0, c1, c2 := s.commands[0], s.commands[1], s.commands[2]
	if c0.line != "agx version" || c0.exit != 0 || !reflect.DeepEqual(c0.output, []string{"agx v2", ""}) {
		t.Errorf("first %+v", c0)
	}
	if c1.exit != 125 || !reflect.DeepEqual(c1.output, []string{"agx: refused"}) {
		t.Errorf("second %+v", c1)
	}
	if c2.line != "agx login add --stdin" || c2.stdin != "~/token.txt" || !reflect.DeepEqual(c2.output, []string{"# a shell-init comment is output"}) {
		t.Errorf("third %+v", c2)
	}
	if _, probs := parseSession("q.txt", []string{"$ agx x --v 'a < b'"}); probs != nil {
		t.Errorf("quoted < taken for a redirect: %q", probs)
	}

	bad := splitLines(strings.Join([]string{
		"stray text",
		"[exit 1]",
		"#options bogus",
		"#options color=x",
		"#options env=1A=x",
		"$ agx version",
		"[exit 2]",
		"late output",
		"[exit 3]",
		"#options dir=~",
		"$ agx version",
		"[exit 99999999999999999999]",
	}, "\n"))
	_, probs = parseSession("b.txt", bad)
	want := []string{
		`b.txt:1: text before the first "$ agx …" line (comments start with #:)`,
		"b.txt:2: [exit N] before the first command",
		`b.txt:3: option "bogus" is not key=value`,
		`b.txt:4: unknown option "color" (dir, env, mode)`,
		`b.txt:5: env="1A=x" is not NAME=value`,
		"b.txt:8: output after [exit N] (it closes the command's output)",
		"b.txt:9: a second [exit N] for one command",
		"b.txt:10: #options must come before the first command",
		`b.txt:12: exit code: strconv.Atoi: parsing "99999999999999999999": value out of range`,
	}
	if !reflect.DeepEqual(probs, want) {
		t.Errorf("problems\n%s\nwant\n%s", strings.Join(probs, "\n"), strings.Join(want, "\n"))
	}
	if _, probs := parseSession("e.txt", []string{"#: only a comment"}); !reflect.DeepEqual(probs, []string{`e.txt: no "$ agx …" line`}) {
		t.Errorf("empty session %q", probs)
	}
}

// TestRewriteSession covers -update's rewrite: replaced output and exit
// lines, a kept block, trailing blank lines that stay, and comments and
// region markers inside an output that keep their output line (or move to
// the end of a shorter real output).
func TestRewriteSession(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		"#: head",
		"#region a",
		"$ agx one",
		"old",
		"...",
		"",
		"#endregion",
		"$ agx two",
		"[exit 1]",
		"",
		"",
		"$ agx three",
		"line 1",
		"#region inner",
		"#: inside the output",
		"line 2",
		"#endregion",
		"line 3",
		"$ agx four",
		"same",
		"#: kept as written",
		"...",
		"$ agx five",
		"a",
		"#: past the real output",
		"b",
		"c",
	}, "\n"))
	s, probs := parseSession("r.txt", lines)
	if probs != nil {
		t.Fatal(probs)
	}
	results := []commandRewrite{
		{output: []string{"new 1", "  indented 2"}},
		{output: []string{"ok"}},
		{output: []string{"L1", "L2", "L3", "L4"}, exit: 2},
		{keep: true},
		{output: []string{"x"}},
	}
	got := rewriteSession(lines, s, func(i int) commandRewrite { return results[i] })
	want := []string{
		"#: head", "#region a", "$ agx one", "new 1", "  indented 2", "", "#endregion",
		"$ agx two", "ok", "", "",
		"$ agx three", "L1", "#region inner", "#: inside the output", "L2", "#endregion", "L3", "L4", "[exit 2]",
		"$ agx four", "same", "#: kept as written", "...",
		"$ agx five", "x", "#: past the real output",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rewrite\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestSplitRow covers cell offsets (in runes, past an indent and a
// multi-byte cell) and the separator widths, and an empty line.
func TestSplitRow(t *testing.T) {
	indent, cells := splitRow("         set A=1  —  from launch environment")
	want := []rowCell{{start: 9, text: "set A=1"}, {start: 18, text: "—", sep: 2}, {start: 21, text: "from launch environment", sep: 2}}
	if indent != 9 || !reflect.DeepEqual(cells, want) {
		t.Errorf("splitRow = %d %+v, want 9 %+v", indent, cells, want)
	}
	if _, cells := splitRow(""); cells != nil {
		t.Errorf("empty line cells %+v", cells)
	}
}

// fakeHome is a long fixture path that alignColumns' tests show as "~".
const fakeHome = "/tmp/T/home"

// showFakeHome is the show function alignColumns' tests pass: fakeHome
// reads as "~", the way shownPaths shows a real fixture home.
func showFakeHome(s string) string { return strings.ReplaceAll(s, fakeHome, homeShown) }

// TestAlignColumns realigns a --dry-run table whose columns agx padded to
// a long fixture path, and proves the result differs from the shown input
// only in spacing (so the gate's comparison is unaffected).
func TestAlignColumns(t *testing.T) {
	// --dry-run as agx prints it on the fixture: a 9-wide name column
	// ("settings" gets one space), values padded to the long real path.
	in := []string{
		"agx: a note",
		"agent    claude          from default_agent",
		"home     /tmp/T/home/.c  from profile personal",
		"settings cleanupDays     from profile work",
		"env      set A=1         from launch environment",
		"         set B=2         from launch environment",
		"note     a long last cell that runs past the from column",
		"agx: hint",
		"  agx profile list",
		"plain line",
		"single  row",
	}
	want := []string{
		"agx: a note",
		"agent    claude       from default_agent",
		"home     ~/.c         from profile personal",
		"settings cleanupDays  from profile work",
		"env      set A=1      from launch environment",
		"         set B=2      from launch environment",
		"note     a long last cell that runs past the from column",
		"agx: hint",
		"  agx profile list",
		"plain line",
		"single  row",
	}
	if got := alignColumns(in, showFakeHome); !reflect.DeepEqual(got, want) {
		t.Errorf("align\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i, l := range alignColumns(in, showFakeHome) {
		if normalizeShown(showFakeHome(in[i])) != normalizeShown(l) {
			t.Errorf("line %d changed beyond spacing: %q", i, l)
		}
	}
}

// TestAlignColumnsKeepsText proves alignColumns leaves alone what is not a
// table it must shrink: scripts, hints, tables without fixture paths and a
// name wider than its column.
func TestAlignColumnsKeepsText(t *testing.T) {
	identity := func(s string) string { return s }
	// An indented script (shell-init), a hint under an error and a table
	// without fixture paths come back exactly as agx printed them.
	for _, in := range [][]string{
		{"agx() {", "  case \"$1\" in", "    new|resume)", "      command agx \"$@\"  # plan", "      ;;", "  esac", "}"},
		{"agx: unknown profile nosuch", "  agx profile list"},
		{"NAME      AGENT   HOME            DEFAULT", "personal  claude  ~/.claude       true", "work      claude  ~/.claude-work  false"},
		{"before_run ~/x.sh  from profile work", "agent      claude    from default_agent", "         continued"},
	} {
		if got := alignColumns(in, identity); !reflect.DeepEqual(got, in) {
			t.Errorf("align changed\n%s\nto\n%s", strings.Join(in, "\n"), strings.Join(got, "\n"))
		}
	}
	// The shown paths of a script are replaced, its indentation kept.
	got := alignColumns([]string{"if [ -d /tmp/T/home/.x ]; then", "    cd /tmp/T/home", "fi"}, showFakeHome)
	if want := []string{"if [ -d ~/.x ]; then", "    cd ~", "fi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("script %q, want %q", got, want)
	}
}

// TestExampleReadmeSelection: only examples/README.md and each example
// folder's README.md are public docs; markdown inside an example's home/ is
// fixture data.
func TestExampleReadmeSelection(t *testing.T) {
	cases := map[string]bool{
		"examples/README.md": true, "examples/quickstart/README.md": true,
		"examples/quickstart/home/README.md": false, "examples/quickstart/session.txt": false,
		"examples/quickstart/home/.claude/README.md": false, "other/x/README.md": false,
	}
	for in, want := range cases {
		if got := isPublicMarkdown(in); got != want {
			t.Errorf("isPublicMarkdown(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestNormalizeBackupStamp: agentio's backup folder stamps normalize to
// one placeholder, so a doc may show any real-looking stamp.
func TestNormalizeBackupStamp(t *testing.T) {
	got := normalizeShown("backup: ~/.claude.agentio-bak-20261118T061007Z/projects")
	if want := "backup: ~/.claude.agentio-bak-" + stampPlaceholder + "/projects"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if normalizeShown("backup: ~/.claude.agentio-bak-20251231T235959Z/projects") != got {
		t.Error("two backup stamps do not compare equal")
	}
}

// TestBlockKeyOrder: a block's keys come in schema order, each once, then
// the unlisted ones sorted; the generated tables have no duplicate row.
func TestBlockKeyOrder(t *testing.T) {
	b := &schema.Schema{
		Properties:    map[string]*schema.Schema{"model": {}, "settings": {}, "prompt": {}, "args": {}},
		PropertyOrder: []string{"model", "settings", "gone", "settings"},
	}
	if got, want := blockKeyOrder(b), []string{"model", "settings", "args", "prompt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("blockKeyOrder = %q, want %q", got, want)
	}
	rows, err := blockTable(generatedSchema(t))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if strings.HasPrefix(r, "| `") && seen[r] {
			t.Errorf("duplicate row %s", r)
		}
		seen[r] = true
	}
}

// TestReadExample proves the layout gate of an example folder: README.md
// and session.txt are required, further sessions run after session.txt in
// name order, and a misspelled or unknown entry, a bad folder name and two
// configs each fail, so no session silently stops running.
func TestReadExample(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(root, examplesDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), exampleFileMode); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		"ok/README.md", "ok/session.txt", "ok/b.session.txt", "ok/a.session.txt", "ok/config.jsonc", "ok/home/.claude/settings.json",
		"no-session/README.md",
		"no-readme/session.txt",
		"typo/README.md", "typo/session.txt", "typo/sesion.txt", "typo/x.session.text",
		"Bad_Name/README.md", "Bad_Name/session.txt",
		"two-configs/README.md", "two-configs/session.txt", "two-configs/config.jsonc", "two-configs/home/.agx/config.jsonc",
		"home-file/README.md", "home-file/session.txt", "home-file/home",
		"sessions-dir/README.md", "sessions-dir/session.txt", "sessions-dir/more.session.txt/x",
	} {
		write(f)
	}
	allowed := "(allowed: README.md, session.txt, <x>.session.txt, config.jsonc, home/)"
	cases := []struct {
		name     string
		sessions []string
		probs    []string
	}{
		{"ok", []string{"session.txt", "a.session.txt", "b.session.txt"}, nil},
		{"no-session", nil, []string{"examples/no-session: missing session.txt"}},
		{"no-readme", []string{"session.txt"}, []string{"examples/no-readme: missing README.md"}},
		{"typo", []string{"session.txt"}, []string{"examples/typo/sesion.txt: unknown entry " + allowed, "examples/typo/x.session.text: unknown entry " + allowed}},
		{"Bad_Name", []string{"session.txt"}, []string{"examples/Bad_Name: folder name must be lowercase kebab-case"}},
		{"two-configs", []string{"session.txt"}, []string{"examples/two-configs: both config.jsonc and home/.agx/config.jsonc; keep one"}},
		{"home-file", []string{"session.txt"}, []string{"examples/home-file/home: unknown entry " + allowed}},
		{"sessions-dir", []string{"session.txt"}, []string{"examples/sessions-dir/more.session.txt: unknown entry " + allowed}},
	}
	for _, c := range cases {
		ex, probs := readExample(root, c.name)
		if !reflect.DeepEqual(ex.sessions, c.sessions) {
			t.Errorf("%s: sessions %q, want %q", c.name, ex.sessions, c.sessions)
		}
		if !reflect.DeepEqual(probs, c.probs) {
			t.Errorf("%s: problems\n%s\nwant\n%s", c.name, strings.Join(probs, "\n"), strings.Join(c.probs, "\n"))
		}
	}
	if ex, _ := readExample(root, "ok"); ex.config != "x\n" || !ex.hasHome {
		t.Errorf("ok: config %q, home %v", ex.config, ex.hasHome)
	}
}

// TestExampleNames: every folder under examples/ is an example to run, and
// any other top-level entry but README.md is a problem (a session left
// there would never run); no examples/ at all is no examples.
func TestExampleNames(t *testing.T) {
	root := t.TempDir()
	if names, probs := exampleNames(t, root); names != nil || probs != nil {
		t.Errorf("no examples dir: %q %q", names, probs)
	}
	for _, d := range []string{"b", "a"} {
		if err := os.MkdirAll(filepath.Join(root, examplesDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{exampleReadme, exampleSession} {
		if err := os.WriteFile(filepath.Join(root, examplesDir, f), nil, exampleFileMode); err != nil {
			t.Fatal(err)
		}
	}
	names, probs := exampleNames(t, root)
	if want := []string{"a", "b"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names %q, want %q", names, want)
	}
	if want := []string{"examples/session.txt: not an example folder (only README.md and folders belong in examples/)"}; !reflect.DeepEqual(probs, want) {
		t.Errorf("problems %q, want %q", probs, want)
	}
}

// TestHomeCopyMode: a home/ file's fixture mode never comes from the
// working tree but its executable bit (git tracks nothing else), so an
// example behaves the same on a fresh clone as where it was written.
func TestHomeCopyMode(t *testing.T) {
	override := map[string]fs.FileMode{"tokens.jsonc": 0o644}
	cases := []struct {
		rel      string
		checkout fs.FileMode
		modes    map[string]fs.FileMode
		want     fs.FileMode
	}{
		{".agx/tokens.jsonc", 0o644, nil, 0o600},  // a clone: agx's own mode
		{".agx/tokens.jsonc", 0o600, nil, 0o600},  // where it was written
		{"notes/tokens.jsonc", 0o664, nil, 0o600}, // by name, anywhere
		{"tokens.jsonc", 0o600, override, 0o644},  // mode= wins (doctor's wrong mode)
		{".claude-work/.claude.json", 0o600, nil, 0o644},
		{"projects/app/env", 0o664, nil, 0o644},
		{".agx/scripts/check.sh", 0o775, nil, 0o755},
		{".agx/scripts/check.sh", 0o700, nil, 0o755},
	}
	for _, c := range cases {
		if got := homeCopyMode(c.rel, c.checkout, c.modes); got != c.want {
			t.Errorf("homeCopyMode(%q, %04o) = %04o, want %04o", c.rel, c.checkout, got, c.want)
		}
	}
}

// TestSessionModeOption: mode=~/path=octal sets one home file's mode;
// anything else is a problem naming the expected shape.
func TestSessionModeOption(t *testing.T) {
	s, probs := parseSession("s.txt", []string{"#options mode=~/tokens.jsonc=644 mode=~/.agx/x=0600", "$ agx version"})
	if len(probs) > 0 {
		t.Fatal(probs)
	}
	if s.modes["tokens.jsonc"] != 0o644 || s.modes[".agx/x"] != 0o600 {
		t.Errorf("modes %v", s.modes)
	}
	for _, bad := range []string{"mode=tokens.jsonc=644", "mode=~/x=9", "mode=~/x", "mode=~/=644", "mode=~/x=17777"} {
		_, probs := parseSession("s.txt", []string{"#options " + bad, "$ agx version"})
		if len(probs) != 1 || !strings.Contains(probs[0], "is not ~/path=octal") {
			t.Errorf("%s: problems %q", bad, probs)
		}
	}
}
