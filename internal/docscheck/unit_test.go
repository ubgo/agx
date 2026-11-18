package docscheck

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/internal/cli"
)

// TestSlugify pins GitHub's heading slug rules on the shapes docs use.
func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Step 1 — Open", "step-1--open"},
		{"Before you start", "before-you-start"},
		{"Try it", "try-it"},
		{"`agentio.UsageOf` and friends", "agentiousageof-and-friends"},
		{"What's new?", "whats-new"},
		{"snake_case_name", "snake_case_name"},
		{"A - B", "a---b"},
		{"Host (the machine)", "host-the-machine"},
		{"[Link text](other.md) here", "link-text-here"},
		{"**Bold** and _emphasis_", "bold-and-emphasis"},
		{"Ünïcode Wörds", "ünïcode-wörds"},
		{"v1.2.3: release", "v123-release"},
		{"C++ / Go", "c--go"},
		{"Homes & targets", "homes--targets"},
		{"<code>Spec</code> fields", "spec-fields"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSluggerDuplicates pins github-slugger's numbering of repeated
// headings, including a heading whose own text looks like a numbered slug.
func TestSluggerDuplicates(t *testing.T) {
	var s slugger
	got := []string{s.slug("Pitfalls"), s.slug("Pitfalls"), s.slug("Pitfalls-1"), s.slug("Pitfalls")}
	// The third heading is literally "Pitfalls-1" but that slug is taken, so
	// github-slugger gives it "pitfalls-1-1"; the fourth "Pitfalls" then skips to -2.
	want := []string{"pitfalls", "pitfalls-1", "pitfalls-1-1", "pitfalls-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}
}

// TestAnchorsOf: headings and <a id> define anchors; front matter and
// fenced code do not.
func TestAnchorsOf(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"---",
		"title: front matter is not a heading",
		"---",
		"# Title #",
		"## Step 1 — Open",
		"```md",
		"# not a heading",
		"```",
		"Setext heading",
		"==============",
		`<a id="custom"></a>`,
		`<a name="legacy">x</a>`,
		"## Title",
	}, "\n"))
	got := anchorsOf(doc)
	for _, want := range []string{"title", "step-1--open", "setext-heading", "custom", "legacy", "title-1"} {
		if !got[want] {
			t.Errorf("anchor %q missing from %v", want, got)
		}
	}
	for _, bad := range []string{"not-a-heading", "title-front-matter-is-not-a-heading"} {
		if got[bad] {
			t.Errorf("anchor %q should not exist", bad)
		}
	}
}

// TestLinkTargets: links and images in text are found; links in inline or
// fenced code are not.
func TestLinkTargets(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"See [a](a.md) and ![img](assets/x.svg \"title\") and [b](b.md#sec).",
		"Inline `[not](code.md)` is ignored, [c](<with space.md>) is not.",
		"```",
		"[fenced](fenced.md)",
		"```",
		`<img src="assets/y.png" width="10"> <a href="#top">top</a>`,
		"[![badge](badge.svg)](https://example.com)",
		"[ref]: docs/ref.md",
	}, "\n"))
	var got []string
	for _, lt := range linkTargets(doc) {
		got = append(got, lt[0]+" "+lt[1])
	}
	want := []string{
		"1 a.md", "1 assets/x.svg", "1 b.md#sec",
		"2 with space.md",
		"6 assets/y.png", "6 #top",
		"7 badge.svg", "7 https://example.com",
		"8 docs/ref.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("targets =\n%v\nwant\n%v", got, want)
	}
}

// TestHardWraps covers which line shapes count as a hard wrap and which
// (code, tables, lists, HTML, headings) are exempt.
func TestHardWraps(t *testing.T) {
	cases := []struct {
		name string
		doc  []string
		want []int // 0-based lines reported
	}{
		{"one line per paragraph", []string{"First paragraph.", "", "Second paragraph."}, nil},
		{"wrapped paragraph", []string{"This sentence was", "wrapped at 80 columns", "twice."}, []int{0, 1}},
		{"wrapped bullet lazy", []string{"- a bullet that", "continues here"}, []int{0}},
		{"wrapped bullet indented", []string{"- a bullet that", "  continues here"}, []int{0}},
		{"bullets", []string{"- one", "- two", "  - nested", "1. ordered", "2) ordered"}, nil},
		{"heading then text", []string{"## Title", "Body text."}, nil},
		{"bold label then text", []string{"**Question?**", "Answer."}, []int{0}},
		{"table", []string{"| a | b |", "|---|---|", "| 1 | 2 |"}, nil},
		{"text then table", []string{"Intro:", "| a | b |"}, nil},
		{"fenced code", []string{"```go", "line one", "line two", "```"}, nil},
		{"tilde fence in list", []string{"- step", "", "  ~~~", "  a", "  b", "  ~~~"}, nil},
		{"indented code", []string{"Para.", "", "    code one", "    code two", "", "    code three"}, nil},
		{"lazy indented continuation", []string{"Para starts", "    and continues"}, []int{0}},
		{"html block", []string{`<p align="center">`, "  some text", "  more text", "</p>"}, nil},
		{"blockquote", []string{"> quoted", "> still quoted"}, nil},
		{"setext heading", []string{"Title", "=====", "Text."}, nil},
		{"thematic break", []string{"Text.", "***", "More."}, nil},
		{"reference definitions", []string{"[a]: a.md", "[b]: b.md"}, nil},
		{"snippet marker then fence", []string{"<!-- snippet: examples/x/main.go#run -->", "```go", "x", "```"}, nil},
		{"front matter", []string{"---", "name: Bug", "about: thing", "---", "", "Body."}, nil},
		{"hard break still a wrap", []string{"Line with break  ", "next"}, []int{0}},
		{"text after fence close", []string{"```", "x", "```", "after", "again"}, []int{3}},
	}
	for _, c := range cases {
		got := hardWraps(c.doc)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: hardWraps = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestStripInlineCode: inline code spans of any backtick count are removed;
// an unclosed backtick is text.
func TestStripInlineCode(t *testing.T) {
	cases := map[string]string{
		"a `b` c":           "a  c",
		"a ``b ` c`` d":     "a  d",
		"unclosed `tick":    "unclosed `tick",
		"[x](y) `[z](w)` v": "[x](y)  v",
	}
	for in, want := range cases {
		if got := stripInlineCode(in); got != want {
			t.Errorf("stripInlineCode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPublicSelection: the root docs, docs/ and .github/ markdown are
// public; markdown elsewhere in the tree is not.
func TestPublicSelection(t *testing.T) {
	cases := map[string]bool{
		"README.md": true, "CHANGELOG.md": true, "docs/commands/profile.md": true,
		".github/pull_request_template.md": true, "internal/cli/NOTES.md": false, "schema/README.md": false,
	}
	for in, want := range cases {
		if got := isPublicMarkdown(in); got != want {
			t.Errorf("isPublicMarkdown(%q) = %v, want %v", in, got, want)
		}
	}
	if !hasSkippedSegment("docs/testdata/x.md") || !hasSkippedSegment("docsi/SPEC.md") || hasSkippedSegment("docs/x.md") {
		t.Error("hasSkippedSegment")
	}
}

// TestAttributionCopilot: the gate is phrase-precise about "copilot". The
// real pi provider id github-copilot (in agx and agentio output, such as
// pi's --provider list) and its variable COPILOT_GITHUB_TOKEN are product
// names and pass; Copilot as a word, credited for the writing in any
// form, is still caught.
func TestAttributionCopilot(t *testing.T) {
	for _, ok := range []string{
		"agx: --provider nope: use one of anthropic, openai, github-copilot, huggingface",
		`| github-copilot | COPILOT_GITHUB_TOKEN |`,
		"$ agx login add --name gh --agent pi --kind api-key --provider github-copilot --token-ref '${GH}'",
		"COPILOT_GITHUB_TOKEN=…",
	} {
		if p := firstAttribution(ok); p != "" {
			t.Errorf("%q flagged as %q", ok, p)
		}
	}
	for _, bad := range []string{
		"Written with GitHub Copilot", "co-written by copilot", "Copilot-generated docs", "Thanks, Copilot!",
		"github-copilot-generated", "my-copilot helper", "copilot_github_token_note",
		"Co-Authored-By: someone",
	} {
		if p := firstAttribution(bad); p == "" {
			t.Errorf("%q not flagged", bad)
		}
	}
}

// TestPhrases pins the attribution and local-path matchers on lines they
// must flag and lines they must leave alone.
func TestPhrases(t *testing.T) {
	if p := firstAttribution("Co-Authored-By: someone"); p != "co-authored-by" {
		t.Errorf("got %q", p)
	}
	for _, ok := range []string{"agx runs Claude Code, Codex and Gemini CLI", "the generated schema", "agx login add"} {
		if p := firstAttribution(ok); p != "" {
			t.Errorf("%q flagged as %q", ok, p)
		}
	}
	for _, ok := range []string{"cd ~/projects/client", "<!-- snippet: examples/acct-x/home/.claude/settings.json -->", "examples/x/home/ copies over ~"} {
		if p := firstLocalPath(ok); p != "" {
			t.Errorf("%q flagged as %q", ok, p)
		}
	}
	for _, bad := range []string{"see /home/someone/x", "/Users/me", "`/space/projects`", "examples/x/home/ and /home/me"} {
		if p := firstLocalPath(bad); p == "" {
			t.Errorf("%q not flagged", bad)
		}
	}
}

// TestShellWords covers the quoting and ~ expansion a runnable command
// line supports.
func TestShellWords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`agx run --profile work`, []string{"agx", "run", "--profile", "work"}},
		{`agx run --token-ref '${gopass:a/b}'`, []string{"agx", "run", "--token-ref", "${gopass:a/b}"}},
		{`agx new --name "pglite golang"`, []string{"agx", "new", "--name", "pglite golang"}},
		{`agx x --v "a \"q\" b"`, []string{"agx", "x", "--v", `a "q" b`}},
		{`agx run --dir ~/projects/client`, []string{"agx", "run", "--dir", "/h/projects/client"}},
		{`agx run --dir '~/x' --v a~b`, []string{"agx", "run", "--dir", "~/x", "--v", "a~b"}},
		{`AGX_LOGIN=carol agx run`, []string{"AGX_LOGIN=carol", "agx", "run"}},
		{`agx  run   `, []string{"agx", "run"}},
		{`agx --v ''`, []string{"agx", "--v", ""}},
	}
	for _, c := range cases {
		got, err := shellWords(c.in, "/h")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("shellWords(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{`agx 'open`, `agx "open`, `pbpaste | agx login add`, `agx run > out`} {
		if _, err := shellWords(bad, "/h"); err == nil {
			t.Errorf("shellWords(%q) accepted", bad)
		}
	}
}

// TestSplitCommand: leading NAME=value words are the environment, the
// command must be agx, and later NAME=value words are arguments.
func TestSplitCommand(t *testing.T) {
	env, argv, err := splitCommand([]string{"AGX_LOGIN=carol", "A=", "agx", "run", "X=1"})
	if err != nil || env["AGX_LOGIN"] != "carol" || env["A"] != "" || !reflect.DeepEqual(argv, []string{"run", "X=1"}) {
		t.Errorf("got %v %v %v", env, argv, err)
	}
	if _, _, err := splitCommand([]string{"claude", "--resume"}); err == nil {
		t.Error("non-agx command accepted")
	}
}

// TestNormalizeShown: run ids, session stamps and times become
// placeholders and runs of spaces one space.
func TestNormalizeShown(t *testing.T) {
	got := normalizeShown("run      ~/.agx/runs/20261114-101500-k3x9   from AGX_RUN_ID=20261117-114545-2xq1  ")
	if got != "run ~/.agx/runs/<run-id> from AGX_RUN_ID=<run-id>" {
		t.Errorf("got %q", got)
	}
	if got := normalizeShown("~/sessions/pglite_golang_20261114_101500 at 2026-11-14T10:15:00Z"); got != "~/sessions/pglite_golang_<stamp> at <time>" {
		t.Errorf("got %q", got)
	}
}

// TestOutputMatches covers exact matches, "..." lines and the 1-based
// line of the first difference.
func TestOutputMatches(t *testing.T) {
	real := []string{"a", "b", "c", "d"}
	cases := []struct {
		shown []string
		want  int
	}{
		{[]string{"a", "b", "c", "d"}, 0},
		{[]string{"a", "...", "d"}, 0},
		{[]string{"...", "c", "d"}, 0},
		{[]string{"a", "b", "c", "d", "..."}, 0},
		{[]string{"..."}, 0},
		{[]string{"a", "x", "c", "d"}, 2},
		{[]string{"a", "b", "c"}, 4},
		{[]string{"a", "...", "x"}, 2},
	}
	for _, c := range cases {
		if got := outputMatches(c.shown, real); (got == 0) != (c.want == 0) || (c.want != 0 && got != c.want) {
			t.Errorf("outputMatches(%q) = %d, want %d", c.shown, got, c.want)
		}
	}
	if outputMatches(nil, nil) != 0 || outputMatches(nil, []string{"x"}) == 0 {
		t.Error("empty cases")
	}
}

// TestFindRunnables covers marker options, commands with their shown
// output, and malformed markers.
func TestFindRunnables(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"<!-- runnable -->",
		"```sh",
		"$ agx run --dry-run",
		"agent claude",
		"$ agx version",
		"```",
		"<!-- runnable: fixture=empty dir=~/projects/client exit=125 -->",
		"```sh",
		"$ agx run",
		"```",
		"<!-- runnable: bogus -->",
		"```sh",
		"$ agx x",
		"```",
		"<!-- runnable -->",
		"```jsonc",
		"{}",
		"```",
		"<!-- runnable -->",
		"```sh",
		"output first",
		"```",
		"```sh",
		"<!-- runnable -->",
		"```",
	}, "\n"))
	rs, probs := findRunnables("x.md", doc)
	if len(rs) != 2 {
		t.Fatalf("runnables %+v", rs)
	}
	if rs[0].fixture != defaultFixture || len(rs[0].commands) != 2 || !reflect.DeepEqual(rs[0].commands[0].output, []string{"agent claude"}) || rs[0].commands[1].output != nil {
		t.Errorf("first %+v", rs[0])
	}
	if rs[1].fixture != emptyFixture || rs[1].dir != "~/projects/client" || rs[1].exit != 125 {
		t.Errorf("second %+v", rs[1])
	}
	want := []string{
		"x.md:11: runnable option \"bogus\" is not key=value",
		"x.md:15: runnable marker not followed by a fenced sh block",
		"x.md:19: runnable block must start with a \"$ agx …\" line",
	}
	if !reflect.DeepEqual(probs[:3], want) || len(probs) != 4 {
		t.Errorf("problems %q", probs)
	}
}

// TestSectionsOf: a command section runs to the next heading of the same
// or a higher level (subsections included), and a heading inside fenced
// code is not a heading.
func TestSectionsOf(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"# Profiles",
		"intro",
		"## agx profile list",
		"| `--agent` | x |",
		"```sh",
		"## not a heading",
		"```",
		"### Example",
		"text `--plan`",
		"## agx profile show",
		"`--id`",
	}, "\n"))
	ss := sectionsOf("p.md", doc)
	if len(ss) != 4 {
		t.Fatalf("sections %+v", ss)
	}
	if ss[1].title != "agx profile list" || len(ss[1].body) != 6 || ss[1].line != 3 {
		t.Errorf("list section %+v", ss[1])
	}
	if got := flagsIn(ss[1].body); !got["agent"] || !got["plan"] || len(got) != 2 {
		t.Errorf("flagsIn %v", got)
	}
	if got := tableFlags(ss[1].body); got["agent"] != 1 || len(got) != 1 {
		t.Errorf("tableFlags %v", got)
	}
	if ss[0].title != "Profiles" || len(ss[0].body) != len(doc)-1 {
		t.Errorf("top section %+v", ss[0])
	}
}

// TestKeyPaths: every key path of a schema, with map keys and array items
// as placeholders.
func TestKeyPaths(t *testing.T) {
	s, err := schema.Parse([]byte(`{
	  "type": "object",
	  "properties": {
	    "profiles": { "type": "object", "additionalProperties": { "type": "object", "properties": { "home": { "type": "string" }, "env": { "type": "object", "additionalProperties": { "type": "string" } } } } },
	    "projects": { "type": "object", "additionalProperties": { "type": "object", "additionalProperties": { "type": "object", "properties": { "login": { "type": "string" } } } } },
	    "list": { "type": "array", "items": { "type": "object", "properties": { "kind": { "type": "string" } } } },
	    "other": { "type": "object", "additionalProperties": { "type": "object", "properties": { "x": { "type": "string" } } } },
	    "spec": { "anyOf": [ { "type": "object", "properties": { "name": { "type": "string" } } }, { "type": "null" } ] }
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	keyPaths(s, "", got)
	want := map[string]bool{
		"profiles": true, "profiles.<profile>.home": true, "profiles.<profile>.env": true,
		"projects": true, "projects.<folder>.<agent>.login": true,
		"list": true, "list[].kind": true, "other": true, "other.<key>.x": true, "spec": true, "spec.name": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keyPaths = %v", got)
	}
}

// TestReferenceDrift runs the command-reference gate on an in-memory tree
// and docs: an unnamed flag, a row for a flag agx lacks, a heading for a
// command agx lacks and a missing section each fail.
func TestReferenceDrift(t *testing.T) {
	tree := cli.HelpCommand{
		Flags: []cli.HelpFlag{{Name: "json"}},
		Commands: []cli.HelpCommand{
			{Path: "profile", Commands: []cli.HelpCommand{
				{Path: "profile list", Flags: []cli.HelpFlag{{Name: "agent"}, {Name: "plan"}, {Name: "secret", Hidden: true}}},
				{Path: "profile show", Flags: []cli.HelpFlag{{Name: "id"}}},
			}},
			{Path: "launch", Hidden: true, Flags: []cli.HelpFlag{{Name: "plan-file"}}},
		},
	}
	doc := splitLines(strings.Join([]string{
		"## Global flags",
		"| `--json` | x |",
		"| `--help` | x |",
		"## agx profile list",
		"| `--agent` | x |",
		"| `--gone` | x |",
		"| `--json` | a global may be listed |",
		"## agx profile remove",
		"## agx profile show",
		"`--id`",
	}, "\n"))
	got := referenceDrift(tree, sectionsOf("p.md", doc))
	want := []string{
		`p.md:4: "agx profile list" does not document --plan`,
		`p.md:6: "agx profile list" documents --gone, which it does not have`,
		`p.md:8: "agx profile remove" is documented but agx has no such command`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("drift =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := referenceDrift(tree, nil); len(got) != 3 || !strings.Contains(got[1], `no section "agx profile list"`) {
		t.Errorf("empty docs: %q", got)
	}
}
