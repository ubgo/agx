package docscheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// runnableMarker is the line above a runnable example:
// <!-- runnable --> or <!-- runnable: fixture=empty dir=~/projects/client exit=125 -->.
var runnableMarker = regexp.MustCompile(`^\s*<!--\s*runnable(?::\s*(.*?))?\s*-->\s*$`)

// Runnable example options (the key=value words of the marker).
const (
	optFixture = "fixture"
	optDir     = "dir"
	optExit    = "exit"
)

// runnableLang is the only fence info string a runnable example may use.
const runnableLang = "sh"

// promptPrefix starts a command line inside a runnable example.
const promptPrefix = "$ "

// ellipsisLine in shown output matches any number of output lines.
const ellipsisLine = "..."

// runnable is one marked example found in a markdown file.
type runnable struct {
	where    string // "docs/x.md:12" (the marker line)
	fixture  string
	dir      string // "~/projects/client" form; "" = defaultDir
	exit     int    // expected exit code of the last command
	commands []shownCommand
}

// shownCommand is one "$ agx …" line and the output shown under it.
type shownCommand struct {
	line   string
	output []string
}

// parseMarkerOptions parses "fixture=x dir=y exit=2" into a runnable.
func parseMarkerOptions(where, opts string) (runnable, error) {
	r := runnable{where: where, fixture: defaultFixture}
	for word := range strings.FieldsSeq(opts) {
		k, v, ok := strings.Cut(word, "=")
		if !ok {
			return r, fmt.Errorf("runnable option %q is not key=value", word)
		}
		switch k {
		case optFixture:
			r.fixture = v
		case optDir:
			r.dir = v
		case optExit:
			n, err := strconv.Atoi(v)
			if err != nil {
				return r, fmt.Errorf("runnable exit=%q is not a number", v)
			}
			r.exit = n
		default:
			return r, fmt.Errorf("unknown runnable option %q (fixture, dir, exit)", k)
		}
	}
	return r, nil
}

// findRunnables returns every runnable example of one markdown file, with
// problems as "file:line: reason".
func findRunnables(md string, lines []string) ([]runnable, []string) {
	var out []runnable
	var probs []string
	fenced := fencedLines(lines)
	for i, l := range lines {
		if fenced[i] {
			continue
		}
		m := runnableMarker.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		where := fmt.Sprintf("%s:%d", md, i+1)
		r, err := parseMarkerOptions(where, m[1])
		if err != nil {
			probs = append(probs, where+": "+err.Error())
			continue
		}
		if i+1 >= len(lines) {
			probs = append(probs, where+": runnable marker not followed by a fenced sh block")
			continue
		}
		f, ok := openFence(lines[i+1])
		if !ok || strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i+1]), string(f.char))) != runnableLang {
			probs = append(probs, where+": runnable marker not followed by a fenced sh block")
			continue
		}
		end := -1
		for j := i + 2; j < len(lines); j++ {
			if f.closes(lines[j]) {
				end = j
				break
			}
		}
		if end < 0 {
			probs = append(probs, where+": runnable block is never closed")
			continue
		}
		for _, b := range lines[i+2 : end] {
			b = strings.TrimPrefix(b, strings.Repeat(" ", f.indent))
			if strings.HasPrefix(b, promptPrefix) {
				r.commands = append(r.commands, shownCommand{line: strings.TrimPrefix(b, promptPrefix)})
				continue
			}
			if len(r.commands) == 0 {
				probs = append(probs, where+": runnable block must start with a \"$ agx …\" line")
				break
			}
			last := &r.commands[len(r.commands)-1]
			last.output = append(last.output, b)
		}
		if len(r.commands) == 0 {
			probs = append(probs, where+": runnable block has no \"$ agx …\" line")
			continue
		}
		out = append(out, r)
	}
	return out, probs
}

// shellWords splits a command line the way sh does for the simple forms
// docs use: whitespace-separated words, '…' kept literally, "…" with \"
// and \\ escapes, and an unquoted leading ~ expanded to home. Pipes,
// substitutions and globs are not supported (such examples are not
// marked runnable).
func shellWords(line, home string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
		}
		cur.Reset()
		inWord = false
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			flush()
		case c == '\'':
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unclosed ' in %q", line)
			}
			cur.WriteString(line[i+1 : i+1+j])
			i += j + 1
			inWord = true
		case c == '"':
			i++
			for ; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && (line[i+1] == '"' || line[i+1] == '\\') {
					i++
				}
				cur.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, fmt.Errorf("unclosed \" in %q", line)
			}
			inWord = true
		case c == '|' || c == ';' || c == '&' || c == '`' || c == '<' || c == '>':
			return nil, fmt.Errorf("shell operator %q is not supported in a runnable example", string(c))
		case c == '~' && !inWord && (i+1 == len(line) || line[i+1] == '/' || line[i+1] == ' '):
			cur.WriteString(home)
			inWord = true
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	flush()
	return words, nil
}

// envAssign matches a leading NAME=value word of a command line.
var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// splitCommand separates leading NAME=value assignments from the agx argv.
func splitCommand(words []string) (env map[string]string, argv []string, err error) {
	env = map[string]string{}
	i := 0
	for ; i < len(words) && envAssign.MatchString(words[i]); i++ {
		k, v, _ := strings.Cut(words[i], "=")
		env[k] = v
	}
	if i >= len(words) || words[i] != "agx" {
		return nil, nil, fmt.Errorf("a runnable command must run agx")
	}
	return env, words[i+1:], nil
}

// Volatile values replaced on both sides before comparing, so docs can
// show realistic run ids, times and session stamps.
var (
	runIDRe = regexp.MustCompile(`\b[0-9]{8}-[0-9]{6}-[a-z0-9]{4}\b`)
	timeRe  = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[+-][0-9:]+)\b`)
	stampRe = regexp.MustCompile(`[0-9]{8}_[0-9]{6}`) // no \b: the slug before it ends in "_", a word character
	// backupStampRe is agentio's backup folder stamp, as in
	// ~/.claude.agentio-bak-20261118T061007Z/ (conversation move).
	backupStampRe = regexp.MustCompile(`\b[0-9]{8}T[0-9]{6}Z\b`)
	spaceRe       = regexp.MustCompile(`[ \t]+`)
)

// Placeholders the volatile values become.
const (
	runIDPlaceholder = "<run-id>"
	timePlaceholder  = "<time>"
	stampPlaceholder = "<stamp>"
)

// normalizeShown makes one output line comparable: volatile values become
// placeholders and every run of spaces becomes one space, so docs may
// align columns differently from the real table widths.
func normalizeShown(s string) string {
	s = runIDRe.ReplaceAllString(s, runIDPlaceholder)
	s = timeRe.ReplaceAllString(s, timePlaceholder)
	s = stampRe.ReplaceAllString(s, stampPlaceholder)
	s = backupStampRe.ReplaceAllString(s, stampPlaceholder)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// outputMatches compares shown lines with real lines (both normalized);
// an ellipsis line in shown matches any number of real lines. It returns
// the 1-based shown line of the first mismatch, or 0.
func outputMatches(shown, real []string) int {
	var match func(si, ri int) bool
	match = func(si, ri int) bool {
		if si == len(shown) {
			return ri == len(real)
		}
		if shown[si] == ellipsisLine {
			for k := ri; k <= len(real); k++ {
				if match(si+1, k) {
					return true
				}
			}
			return false
		}
		return ri < len(real) && shown[si] == real[ri] && match(si+1, ri+1)
	}
	if match(0, 0) {
		return 0
	}
	n := 0
	for n < len(shown) && n < len(real) && (shown[n] == real[n] || shown[n] == ellipsisLine) {
		if shown[n] == ellipsisLine {
			break
		}
		n++
	}
	return n + 1
}

// realLines is what a command printed, the way a terminal shows it:
// stderr notes first (agx writes them before the result), then stdout,
// with fixture paths shown as a user would see them.
func realLines(fx *testkit.Fixture, res testkit.Result) []string {
	raw := rawLines(fx, res)
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		out = append(out, normalizeShown(l))
	}
	return out
}

// rawLines is realLines before normalization: the terminal's lines with
// fixture paths shown as a user would see them and trailing spaces
// dropped.
func rawLines(fx *testkit.Fixture, res testkit.Result) []string {
	show := shownPaths(fx)
	lines := terminalLines(res)
	for i, l := range lines {
		lines[i] = show(l)
	}
	return lines
}

// terminalLines is what a command printed, stderr first, as agx wrote it
// (fixture paths still real), with trailing spaces dropped.
func terminalLines(res testkit.Result) []string {
	var out []string
	for _, s := range []string{res.Stderr, res.Stdout} {
		for _, l := range splitLines(s) {
			out = append(out, strings.TrimRight(l, " \t"))
		}
	}
	return out
}

// shownPaths rewrites fx's real paths in a piece of output the way docs
// show them: the fake binaries' folder as /usr/local/bin, the home as ~
// (also inside Claude's history folder names), the root as a neutral
// path. No replacement spans a line, so it works on lines and on cells.
func shownPaths(fx *testkit.Fixture) func(string) string {
	return func(s string) string {
		s = strings.ReplaceAll(s, fx.Bin, fixtureBinShown)
		s = strings.ReplaceAll(s, claudeHistoryKey(fx.Home), fixtureHomeKeyShown)
		s = strings.ReplaceAll(s, fx.Home, homeShown)
		return strings.ReplaceAll(s, fx.Root, fixtureRootShown)
	}
}

// normalizeAll normalizes every shown line (normalizeShown) and drops
// trailing blank lines, so a block's spacing never fails the gate.
func normalizeAll(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, normalizeShown(l))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// TestRunnableExamples runs every marked example against a fixture
// machine and compares the shown output with the real one.
func TestRunnableExamples(t *testing.T) {
	root := repoRoot(t)
	mds := publicMarkdown(t, root)
	fixtures, probs := findFixtureConfigs(t, root, mds)
	for _, p := range probs {
		t.Error(p)
	}
	for _, md := range mds {
		rs, probs := findRunnables(md, readLines(t, root, md))
		for _, p := range probs {
			t.Error(p)
		}
		for _, r := range rs {
			t.Run(r.where, func(t *testing.T) {
				t.Parallel()
				runExample(t, fixtures, r)
			})
		}
	}
}

// runExample runs one example's commands in order on a fresh fixture.
func runExample(t *testing.T, fixtures map[string]string, r runnable) {
	fx, err := newDocFixture(t, fixtures, r.fixture)
	if err != nil {
		t.Fatalf("%s: %v", r.where, err)
	}
	dir := commandDir(fx, r.dir)
	for i, c := range r.commands {
		res, err := runShown(fx, dir, c.line, "")
		if err != nil {
			t.Fatalf("%s: %v", r.where, err)
		}
		want := 0
		if i == len(r.commands)-1 {
			want = r.exit
		}
		if res.Code != want {
			t.Errorf("%s: $ %s: exit %d, want %d\nstderr: %s\nstdout: %s", r.where, c.line, res.Code, want, res.Stderr, res.Stdout)
			continue
		}
		shown, got := normalizeAll(c.output), realLines(fx, res)
		if n := outputMatches(shown, got); n != 0 {
			t.Errorf("%s: $ %s: shown output differs at its line %d\n--- shown\n%s\n--- real (normalized)\n%s", r.where, c.line, n, strings.Join(shown, "\n"), strings.Join(got, "\n"))
		}
	}
}

// runShown runs one shown command line on fx in dir with stdin: its
// leading NAME=value words set the environment of this command only.
func runShown(fx *testkit.Fixture, dir, line, stdin string) (testkit.Result, error) {
	words, err := shellWords(line, fx.Home)
	if err != nil {
		return testkit.Result{}, err
	}
	env, argv, err := splitCommand(words)
	if err != nil {
		return testkit.Result{}, fmt.Errorf("%q: %w", line, err)
	}
	saved := map[string]string{}
	for k, v := range env {
		saved[k] = fx.Env[k]
		fx.Env[k] = v
	}
	res := fx.RunIn(dir, stdin, argv...)
	for k, v := range saved {
		fx.Env[k] = v
	}
	return res, nil
}
