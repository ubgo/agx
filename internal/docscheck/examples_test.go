package docscheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/testkit"
)

// examplesDir holds the example folders, relative to the repo root. Each
// folder is one or more scripted sessions run with the real binary on a
// fresh fixture by TestExamples; docs show parts of them as snippets.
const examplesDir = "examples"

// The files of an example folder. Any other top-level entry fails, so a
// misspelled session never silently stops running.
const (
	exampleReadme  = "README.md"
	exampleSession = "session.txt"
	// exampleSessionSuffix names further sessions: "<x>.session.txt", each
	// run on its own fresh fixture.
	exampleSessionSuffix = ".session.txt"
	exampleConfig        = "config.jsonc"
	// exampleHome is copied over the fixture home after the standard
	// fixture is built (extra settings, env files, tokens, conversations).
	exampleHome = "home"
	// exampleAgxDir is agx's config folder inside exampleHome: a config
	// there and a config.jsonc beside the session would both claim the
	// fixture's config.
	exampleAgxDir = ".agx"
)

// exampleFixture is the fixture name an example's config.jsonc is
// registered under for newDocFixture.
const exampleFixture = "example"

// exampleName is the shape of an example folder name.
var exampleName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Session file syntax beyond the runnable-block format. Each form is
// chosen so it cannot be mistaken for agx output: agx prints "# " lines
// (shell-init scripts) but never "#:", "#options" or "[exit N]".
const (
	// sessionComment starts a comment line, skipped by the runner.
	sessionComment = "#:"
	// sessionOptions starts the options line (before the first command):
	// "#options dir=~/projects/client env=NAME=value".
	sessionOptions = "#options"
)

// exitLine is "[exit N]" alone on a line after a command's output: the
// exit code that command must return (default 0 for every command).
// exitLineFormat writes one; the two must stay in step.
var exitLine = regexp.MustCompile(`^\[exit (\d+)\]$`)

// exitLineFormat is the Sprintf format of an exitLine (-update writes it).
const exitLineFormat = "[exit %d]"

// stdinRedirect is a trailing "< ~/file" or "< file" on a command line:
// the file (fixture home or run folder relative) is the command's stdin.
// The path may not hold quotes or spaces, so a quoted "<" inside an
// argument is never taken for a redirect.
var stdinRedirect = regexp.MustCompile(`\s<\s*([~A-Za-z0-9_./-]+)$`)

// Session options (the key=value words of the #options line).
const (
	sessionOptDir = "dir"
	sessionOptEnv = "env"
	// sessionOptMode sets the mode of one home/ file in the fixture:
	// "mode=~/tokens.jsonc=644" (octal). Needed because git keeps only
	// the executable bit, so a mode on disk is never a source of truth.
	sessionOptMode = "mode"
)

// session is one parsed session file.
type session struct {
	file string            // "examples/x/session.txt"
	dir  string            // "~/projects/client" form; "" = defaultDir
	env  map[string]string // set for every command
	// modes are home/ file modes set by mode= options, keyed by the path
	// relative to the fixture home in slash form (".agx/tokens.jsonc").
	modes    map[string]fs.FileMode
	commands []sessionCommand
	// kinds classifies every line of the file (for -update).
	kinds []sessionLineKind
}

// sessionCommand is one "$ agx …" line of a session.
type sessionCommand struct {
	at     int    // 0-based line of the "$ " line
	line   string // the command, without "$ " and the stdin redirect
	stdin  string // the redirect path as written; "" = empty stdin
	exit   int
	output []string
}

// sessionLineKind tells what one line of a session file is.
type sessionLineKind int

const (
	lineKept    sessionLineKind = iota // comment, option, region marker, blank before the first command
	lineCommand                        // "$ agx …"
	lineOutput                         // shown output of the command above
	lineExit                           // "[exit N]"
)

// parseSession parses a session file; problems read "file:line: reason".
func parseSession(file string, lines []string) (session, []string) {
	s := session{file: file, env: map[string]string{}, modes: map[string]fs.FileMode{}, kinds: make([]sessionLineKind, len(lines))}
	var probs []string
	exitSet := false
	for i, l := range lines {
		where := fmt.Sprintf("%s:%d", file, i+1)
		switch {
		case isRegionMarker(l), strings.HasPrefix(l, sessionComment):
			s.kinds[i] = lineKept
		case l == sessionOptions || strings.HasPrefix(l, sessionOptions+" "):
			s.kinds[i] = lineKept
			if len(s.commands) > 0 {
				probs = append(probs, where+": "+sessionOptions+" must come before the first command")
				continue
			}
			probs = append(probs, s.parseOptions(where, strings.TrimPrefix(l, sessionOptions))...)
		case strings.HasPrefix(l, promptPrefix):
			s.kinds[i] = lineCommand
			c := sessionCommand{at: i, line: strings.TrimPrefix(l, promptPrefix)}
			if m := stdinRedirect.FindStringSubmatchIndex(c.line); m != nil {
				c.stdin = c.line[m[2]:m[3]]
				c.line = strings.TrimRight(c.line[:m[0]], " \t")
			}
			s.commands = append(s.commands, c)
			exitSet = false
		case exitLine.MatchString(l):
			s.kinds[i] = lineExit
			if len(s.commands) == 0 {
				probs = append(probs, where+": [exit N] before the first command")
				continue
			}
			if exitSet {
				probs = append(probs, where+": a second [exit N] for one command")
				continue
			}
			n, err := strconv.Atoi(exitLine.FindStringSubmatch(l)[1])
			if err != nil {
				probs = append(probs, fmt.Sprintf("%s: exit code: %v", where, err))
				continue
			}
			s.commands[len(s.commands)-1].exit, exitSet = n, true
		case len(s.commands) == 0:
			s.kinds[i] = lineKept
			if strings.TrimSpace(l) != "" {
				probs = append(probs, where+": text before the first \"$ agx …\" line (comments start with "+sessionComment+")")
			}
		default:
			s.kinds[i] = lineOutput
			if exitSet {
				if strings.TrimSpace(l) != "" {
					probs = append(probs, where+": output after [exit N] (it closes the command's output)")
				}
				continue
			}
			last := &s.commands[len(s.commands)-1]
			last.output = append(last.output, l)
		}
	}
	if len(s.commands) == 0 {
		probs = append(probs, file+": no \"$ agx …\" line")
	}
	return s, probs
}

// parseOptions reads the words of an #options line into s.
func (s *session) parseOptions(where, opts string) []string {
	var probs []string
	for word := range strings.FieldsSeq(opts) {
		k, v, ok := strings.Cut(word, "=")
		if !ok {
			probs = append(probs, fmt.Sprintf("%s: option %q is not key=value", where, word))
			continue
		}
		switch k {
		case sessionOptDir:
			s.dir = v
		case sessionOptEnv:
			name, val, ok := strings.Cut(v, "=")
			if !ok || !envAssign.MatchString(v) {
				probs = append(probs, fmt.Sprintf("%s: env=%q is not NAME=value", where, v))
				continue
			}
			s.env[name] = val
		case sessionOptMode:
			file, octal, ok := strings.Cut(v, "=")
			rel, home := strings.CutPrefix(file, homePrefix)
			n, err := strconv.ParseUint(octal, 8, 32)
			if !ok || !home || rel == "" || err != nil || n > uint64(fs.ModePerm) {
				probs = append(probs, fmt.Sprintf("%s: mode=%q is not ~/path=octal (e.g. mode=~/tokens.jsonc=644)", where, v))
				continue
			}
			s.modes[rel] = fs.FileMode(n)
		default:
			probs = append(probs, fmt.Sprintf("%s: unknown option %q (%s, %s, %s)", where, k, sessionOptDir, sessionOptEnv, sessionOptMode))
		}
	}
	return probs
}

// example is one folder under examplesDir.
type example struct {
	name     string
	dir      string   // absolute folder
	sessions []string // session file names, session.txt first
	config   string   // config.jsonc content; "" = none
	hasHome  bool
}

// readExample checks one example folder's layout and loads it.
func readExample(root, name string) (example, []string) {
	ex := example{name: name, dir: filepath.Join(root, examplesDir, name)}
	rel := examplesDir + "/" + name
	var probs []string
	if !exampleName.MatchString(name) {
		probs = append(probs, fmt.Sprintf("%s: folder name must be lowercase kebab-case", rel))
	}
	entries, err := os.ReadDir(ex.dir)
	if err != nil {
		return ex, append(probs, rel+": "+err.Error())
	}
	have := map[string]bool{}
	var extra []string
	for _, e := range entries {
		n := e.Name()
		have[n] = true
		switch {
		case n == exampleReadme, n == exampleSession:
		case n == exampleConfig:
			b, err := os.ReadFile(filepath.Join(ex.dir, n))
			if err != nil {
				probs = append(probs, rel+"/"+n+": "+err.Error())
			}
			ex.config = string(b)
		case n == exampleHome && e.IsDir():
			ex.hasHome = true
		case strings.HasSuffix(n, exampleSessionSuffix) && !e.IsDir():
			extra = append(extra, n)
		default:
			probs = append(probs, fmt.Sprintf("%s/%s: unknown entry (allowed: %s, %s, <x>%s, %s, %s/)", rel, n, exampleReadme, exampleSession, exampleSessionSuffix, exampleConfig, exampleHome))
		}
	}
	for _, req := range []string{exampleReadme, exampleSession} {
		if !have[req] {
			probs = append(probs, fmt.Sprintf("%s: missing %s", rel, req))
		}
	}
	if ex.config != "" && ex.hasHome {
		if _, err := os.Stat(filepath.Join(ex.dir, exampleHome, exampleAgxDir, exampleConfig)); err == nil {
			probs = append(probs, fmt.Sprintf("%s: both %s and %s/%s/%s; keep one", rel, exampleConfig, exampleHome, exampleAgxDir, exampleConfig))
		}
	}
	sort.Strings(extra)
	if have[exampleSession] {
		ex.sessions = append(ex.sessions, exampleSession)
	}
	ex.sessions = append(ex.sessions, extra...)
	return ex, probs
}

// exampleNames lists the example folders (none when examplesDir is absent)
// and a problem for every other entry of examplesDir but its README.md: a
// session or config left at the top level would never run.
func exampleNames(t *testing.T, root string) ([]string, []string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, examplesDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names, probs []string
	for _, e := range entries {
		switch {
		case e.IsDir():
			names = append(names, e.Name())
		case e.Name() != exampleReadme:
			probs = append(probs, fmt.Sprintf("%s/%s: not an example folder (only %s and folders belong in %s/)", examplesDir, e.Name(), exampleReadme, examplesDir))
		}
	}
	return names, probs
}

// TestExamples runs every session of every example folder on a fresh
// fixture and compares the shown output with the real one. With
// -args -update it rewrites the output of each command whose shown output
// or exit code no longer matches the real run.
func TestExamples(t *testing.T) {
	root := repoRoot(t)
	names, probs := exampleNames(t, root)
	for _, p := range probs {
		t.Error(p)
	}
	// failed collects every failing session so one line at the end names
	// them all: in a long run (task check) the first FAIL is easy to take
	// for the only one.
	var (
		mu     sync.Mutex
		failed []string
	)
	t.Cleanup(func() {
		if len(failed) > 0 {
			sort.Strings(failed)
			t.Logf("%d failing example session(s): %s", len(failed), strings.Join(failed, ", "))
		}
	})
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ex, probs := readExample(root, name)
			for _, p := range probs {
				t.Error(p)
			}
			for _, sf := range ex.sessions {
				t.Run(sf, func(t *testing.T) {
					t.Parallel()
					t.Cleanup(func() {
						if t.Failed() {
							mu.Lock()
							failed = append(failed, name+"/"+sf)
							mu.Unlock()
						}
					})
					runSession(t, root, ex, sf)
				})
			}
		})
	}
}

// runSession runs one session file of ex: every command in order on one
// fresh fixture, each compared with its shown output and exit code (or,
// with -update, the file rewritten from the real run).
func runSession(t *testing.T, root string, ex example, sessionFile string) {
	rel := path.Join(examplesDir, ex.name, sessionFile)
	lines := readLines(t, root, rel)
	if _, err := parseRegions(rel, lines); err != nil {
		t.Error(err)
	}
	s, probs := parseSession(rel, lines)
	if len(probs) > 0 {
		t.Fatal(strings.Join(probs, "\n"))
	}
	fx := exampleMachine(t, ex, s.modes)
	for k, v := range s.env {
		fx.Env[k] = v
	}
	dir := commandDir(fx, s.dir)
	results := make([]testkit.Result, len(s.commands))
	for i, c := range s.commands {
		where := fmt.Sprintf("%s:%d", rel, c.at+1)
		stdin, err := sessionStdin(fx, dir, c.stdin)
		if err != nil {
			t.Fatalf("%s: %v", where, err)
		}
		res, err := runShown(fx, dir, c.line, stdin)
		if err != nil {
			t.Fatalf("%s: %v", where, err)
		}
		results[i] = res
		if updating() {
			continue
		}
		if res.Code != c.exit {
			t.Errorf("%s: $ %s: exit %d, want %d (add or fix an [exit N] line)\nstderr: %s\nstdout: %s", where, c.line, res.Code, c.exit, res.Stderr, res.Stdout)
			continue
		}
		shown, got := normalizeAll(c.output), realLines(fx, res)
		if n := outputMatches(shown, got); n != 0 {
			t.Errorf("%s: $ %s: shown output differs at its line %d\n--- shown\n%s\n--- real (normalized)\n%s", where, c.line, n, strings.Join(shown, "\n"), strings.Join(got, "\n"))
		}
	}
	if updating() {
		updateSession(t, filepath.Join(root, filepath.FromSlash(rel)), lines, s, fx, results)
	}
}

// updateSession rewrites the session file at p (-update) from the real
// results of its commands, in order.
func updateSession(t *testing.T, p string, lines []string, s session, fx *testkit.Fixture, results []testkit.Result) {
	t.Helper()
	out := rewriteSession(lines, s, func(i int) commandRewrite {
		c, res := s.commands[i], results[i]
		// A command whose shown output still matches keeps its block as
		// written: hand alignment, "..." lines, comments and the run ids
		// shown stay, so -update only touches what changed.
		if res.Code == c.exit && outputMatches(normalizeAll(c.output), realLines(fx, res)) == 0 {
			return commandRewrite{keep: true}
		}
		return commandRewrite{output: alignColumns(terminalLines(res), shownPaths(fx)), exit: res.Code}
	})
	if err := os.WriteFile(p, []byte(strings.Join(out, "\n")+"\n"), exampleFileMode); err != nil {
		t.Fatal(err)
	}
}

// exampleFileMode is the mode -update writes a session file with.
const exampleFileMode = 0o644

// Modes of home/ files in the fixture. git keeps only the executable bit,
// so a fresh clone has every other file at the umask's default (0644 on
// macOS); the copy must not depend on the mode the working tree happens
// to have, or an example passes where it was written and fails on a clone.
const (
	homeFileMode = 0o644
	homeExecMode = 0o755
	execBits     = 0o111
)

// tokensFileName is agx's pasted-token file, which agx itself always
// writes 0600 and refuses at any wider mode: a home/ copy gets that mode
// unless a mode= option says otherwise.
const tokensFileName = "tokens.jsonc"

// homeCopyMode is the fixture mode of the home/ file at rel (slash form,
// relative to home/): a mode= option first, then executable files 0755,
// tokens.jsonc 0600, everything else 0644. checkout is the mode in the
// working tree; only its executable bit counts (git tracks nothing else).
func homeCopyMode(rel string, checkout fs.FileMode, modes map[string]fs.FileMode) fs.FileMode {
	if m, ok := modes[rel]; ok {
		return m
	}
	switch {
	case checkout&execBits != 0:
		return homeExecMode
	case path.Base(rel) == tokensFileName:
		return config.TokensFileMode
	}
	return homeFileMode
}

// exampleMachine builds the fixture of ex: the standard fixture with its
// config.jsonc (none when absent), then its home/ copied over the home
// with the modes of homeCopyMode (modes are the session's mode= options).
func exampleMachine(t *testing.T, ex example, modes map[string]fs.FileMode) *testkit.Fixture {
	t.Helper()
	configs, name := map[string]string{}, emptyFixture
	if ex.config != "" {
		configs[exampleFixture], name = ex.config, exampleFixture
	}
	fx, err := newDocFixture(t, configs, name)
	if err != nil {
		t.Fatal(err)
	}
	if !ex.hasHome {
		return fx
	}
	src := filepath.Join(ex.dir, exampleHome)
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		r, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: only regular files can be copied into the fixture home", path.Join(examplesDir, ex.name, exampleHome, filepath.ToSlash(r)))
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fx.WriteFile(filepath.Join(fx.Home, r), string(b), homeCopyMode(filepath.ToSlash(r), info.Mode().Perm(), modes))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

// sessionStdin reads a command's stdin redirect: "~/x" is in the fixture
// home, anything else relative to the folder the command runs in. "" (no
// redirect) is an empty stdin.
func sessionStdin(fx *testkit.Fixture, dir, redirect string) (string, error) {
	if redirect == "" {
		return "", nil
	}
	p := filepath.Join(dir, filepath.FromSlash(redirect))
	if rest, ok := strings.CutPrefix(redirect, homePrefix); ok {
		p = filepath.Join(fx.Home, filepath.FromSlash(rest))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("stdin %s: %w", redirect, err)
	}
	return string(b), nil
}

// commandRewrite is what -update writes for one command of a session.
type commandRewrite struct {
	// keep: the shown block still matches the real run; it stays
	// verbatim (output, exit line, comments and markers inside it).
	keep bool
	// output and exit are the real ones when keep is false.
	output []string
	exit   int
}

// rewriteSession is lines with every command's block rewritten by
// result(i): kept verbatim, or its shown output and exit line replaced by
// the real ones, written right under the command. A comment or region
// marker inside the shown output stays at the same output line (after as
// many real lines as there were shown lines above it; at the end when the
// real output is shorter), so inner regions keep their place. Blank lines
// that end a block (after its last shown line) and what follows them stay.
func rewriteSession(lines []string, s session, result func(i int) commandRewrite) []string {
	var out []string
	if len(s.commands) > 0 {
		out = append(out, lines[:s.commands[0].at]...)
	}
	for k, c := range s.commands {
		end := len(lines)
		if k+1 < len(s.commands) {
			end = s.commands[k+1].at
		}
		r := result(k)
		if r.keep {
			out = append(out, lines[c.at:end]...)
			continue
		}
		lastShown := c.at
		for j := c.at + 1; j < end; j++ {
			if s.kinds[j] != lineKept && strings.TrimSpace(lines[j]) != "" {
				lastShown = j
			}
		}
		// kept[p] are the comment and marker lines that sat after p shown
		// output lines.
		kept := map[int][]string{}
		shown := 0
		for j := c.at + 1; j <= lastShown; j++ {
			switch s.kinds[j] {
			case lineOutput:
				shown++
			case lineKept:
				p := min(shown, len(r.output))
				kept[p] = append(kept[p], lines[j])
			}
		}
		out = append(out, lines[c.at])
		for p := 0; p <= len(r.output); p++ {
			out = append(out, kept[p]...)
			if p < len(r.output) {
				out = append(out, r.output[p])
			}
		}
		if r.exit != 0 {
			out = append(out, fmt.Sprintf(exitLineFormat, r.exit))
		}
		out = append(out, lines[lastShown+1:end]...)
	}
	return out
}

// columnGap is the run of spaces that separates two table cells; a single
// space belongs inside a cell ("LOOKS LIKE A HOME").
var columnGap = regexp.MustCompile(` {2,}`)

// Table shape for alignColumns.
const (
	// minColumnGap is the fewest spaces alignColumns puts between two
	// cells that agx separated by two or more (columnGap's lower bound).
	minColumnGap = 2
	// minRowCells is the fewest cells a line needs to count as a table
	// row: one cell is plain text.
	minRowCells = 2
	// minTableRows is the fewest rows a run of lines needs to be
	// realigned as a table: a single row has no column to agree on.
	minTableRows = 2
)

// rowCell is one cell of a table row in agx's real output.
type rowCell struct {
	start int    // rune offset in the real line
	text  string // as agx printed it (fixture paths still real)
	sep   int    // spaces before it in the real line (0 for the first)
}

// splitRow splits a real output line into its indentation and its cells:
// runs of text separated by two or more spaces.
func splitRow(line string) (int, []rowCell) {
	body := strings.TrimLeft(line, " ")
	indent := len(line) - len(body)
	var cells []rowCell
	prev, sep := 0, 0 // byte offset of the next cell in body, spaces before it
	add := func(end int) {
		cells = append(cells, rowCell{start: indent + utf8.RuneCountInString(body[:prev]), text: body[prev:end], sep: sep})
	}
	for _, gap := range columnGap.FindAllStringIndex(body, -1) {
		add(gap[0])
		prev, sep = gap[1], gap[1]-gap[0]
	}
	if prev < len(body) {
		add(len(body))
	}
	return indent, cells
}

// alignedRow is a line of a table being realigned.
type alignedRow struct {
	line  int // index into the lines
	cells []rowCell
	cols  []int // column index of each cell
}

// alignColumns returns lines with fixture paths shown (show) and agx's
// tables re-padded. agx pads each column to its widest cell, which on the
// fixture is often a long temporary path; once show shortens it to "~"
// the columns no longer line up. The columns are read from the real
// output, before show: every run of consecutive lines with two or more
// cells (on 2+ spaces) holds a table whose columns start where its cells
// start. An indented one-cell line whose cell starts on such a column
// (the continuation rows of --dry-run's env) belongs to the table; other
// lines (an indented script, a hint) keep their text and indentation. A
// cell that runs over a column start where a word begins after a space
// ("settings cleanupPeriodDays", whose name is wider than its column) is
// split there, so it is aligned as agx aligned it. Each column then
// shrinks by as much as its widest cell did. With an identity show the
// output equals the input. Safe for the gate: the comparison treats every
// run of spaces as one space.
func alignColumns(lines []string, show func(string) string) []string {
	out := make([]string, len(lines))
	type row struct {
		indent int
		cells  []rowCell
	}
	rows := make([]row, len(lines))
	for i, l := range lines {
		out[i] = show(l)
		rows[i].indent, rows[i].cells = splitRow(l)
	}
	candidate := func(r row) bool { return len(r.cells) >= minRowCells || r.indent > 0 && len(r.cells) == 1 }
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && candidate(rows[j]) {
			j++
		}
		var table []alignedRow
		starts := map[int]bool{}
		for k := i; k < j; k++ {
			if len(rows[k].cells) >= minRowCells {
				table = append(table, alignedRow{line: k, cells: rows[k].cells})
				for _, c := range rows[k].cells {
					starts[c.start] = true
				}
			}
		}
		if len(table) >= minTableRows {
			for k := i; k < j; k++ {
				if len(rows[k].cells) == 1 && starts[rows[k].cells[0].start] {
					table = append(table, alignedRow{line: k, cells: rows[k].cells})
				}
			}
			alignTable(out, table, starts, show)
		}
		i = max(j, i+1)
	}
	return out
}

// alignTable writes the rows of one table into out, realigned.
func alignTable(out []string, table []alignedRow, starts map[int]bool, show func(string) string) {
	cols := make([]int, 0, len(starts))
	for s := range starts {
		cols = append(cols, s)
	}
	sort.Ints(cols)
	colOf := map[int]int{}
	for k, s := range cols {
		colOf[s] = k
	}
	for n := range table {
		table[n].cells = splitAtColumns(table[n].cells, starts)
		for _, c := range table[n].cells {
			table[n].cols = append(table[n].cols, colOf[c.start])
		}
	}
	// The widest real and shown cell per column, over the cells that fill
	// exactly one column and are not their line's last (a last cell is
	// never padded, so a long note does not widen its column).
	realWidth, shownWidth := make([]int, len(cols)), make([]int, len(cols))
	measured := make([]bool, len(cols))
	for _, r := range table {
		for n := 0; n+1 < len(r.cells); n++ {
			if k := r.cols[n]; r.cols[n+1] == k+1 {
				realWidth[k] = max(realWidth[k], utf8.RuneCountInString(r.cells[n].text))
				shownWidth[k] = max(shownWidth[k], utf8.RuneCountInString(show(r.cells[n].text)))
				measured[k] = true
			}
		}
	}
	at := make([]int, len(cols))
	at[0] = cols[0]
	for k := 1; k < len(cols); k++ {
		gap := cols[k] - cols[k-1]
		if measured[k-1] {
			gap -= realWidth[k-1] - shownWidth[k-1]
		}
		at[k] = at[k-1] + max(gap, 1)
	}
	for _, r := range table {
		var b strings.Builder
		width := 0
		for n, c := range r.cells {
			pad := at[r.cols[n]] - width
			if n > 0 {
				pad = max(pad, min(c.sep, minColumnGap))
			}
			text := show(c.text)
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(text)
			width += pad + utf8.RuneCountInString(text)
		}
		out[r.line] = b.String()
	}
}

// splitAtColumns splits every cell but the last where a column start
// falls inside it on a word that follows a single space: agx printed a
// name wider than its column ("settings", "before_run") and the value
// after one space.
func splitAtColumns(cells []rowCell, starts map[int]bool) []rowCell {
	var out []rowCell
	for n, c := range cells {
		if n == len(cells)-1 {
			out = append(out, c)
			continue
		}
		runes := []rune(c.text)
		from := 0
		for k := 1; k < len(runes); k++ {
			if starts[c.start+k] && runes[k-1] == ' ' && runes[k] != ' ' {
				out = append(out, rowCell{start: c.start + from, text: strings.TrimRight(string(runes[from:k]), " "), sep: sepOf(c, from)})
				from = k
			}
		}
		out = append(out, rowCell{start: c.start + from, text: string(runes[from:]), sep: sepOf(c, from)})
	}
	return out
}

// sepOf is the separator before the part of c that begins at rune from:
// the cell's own separator for its first part, one space inside it.
func sepOf(c rowCell, from int) int {
	if from == 0 {
		return c.sep
	}
	return 1
}
