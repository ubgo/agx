package docscheck

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/cli"
)

// commandsDir holds the command reference: one heading "agx <path>" per
// command, whose section documents every flag of that command.
const commandsDir = docsDir + "/commands"

// globalFlagsHeading is the heading whose section documents the flags
// every command takes (the root's persistent flags).
const globalFlagsHeading = "Global flags"

// commandHeadingPrefix starts the heading text of a command section.
const commandHeadingPrefix = "agx "

// cobraHelpFlag is cobra's own --help, documented once with the globals.
const cobraHelpFlag = "help"

var (
	// inlineFlag matches a long flag inside inline code: `--name`.
	inlineFlag = regexp.MustCompile("`(--[a-z][a-z0-9-]*)")
	// tableFlagRow matches a table row whose first cell starts with a flag.
	tableFlagRow = regexp.MustCompile("^\\|\\s*`(--[a-z][a-z0-9-]*)")
)

// docSection is one heading and the lines under it (up to the next
// heading of the same or a higher level).
type docSection struct {
	file  string
	line  int // 1-based heading line
	title string
	body  []string
}

// sectionsOf splits a markdown file into heading sections.
func sectionsOf(file string, lines []string) []docSection {
	type open struct {
		idx   int
		level int
	}
	var out []docSection
	var stack []open
	fenced := fencedLines(lines)
	closeTo := func(level, end int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			s := &out[top.idx]
			s.body = lines[s.line:end]
		}
	}
	for i, l := range lines {
		if fenced[i] {
			continue
		}
		m := atxHeading.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		level := len(strings.TrimLeft(l, " ")) - len(strings.TrimLeft(strings.TrimLeft(l, " "), "#"))
		closeTo(level, i)
		out = append(out, docSection{file: file, line: i + 1, title: headingText(closingHash.ReplaceAllString(m[1], ""))})
		stack = append(stack, open{idx: len(out) - 1, level: level})
	}
	closeTo(0, len(lines))
	return out
}

// commandTree is agx's command tree, built in-process from the same
// declarations as the parser (cli.HelpTree), never from the binary's text.
func commandTree() cli.HelpCommand {
	root, _ := cli.NewRoot(cli.Env{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
		Lookup: func(string) (string, bool) { return "", false }, StdinFD: -1,
	})
	// cobra adds the help command when the tree executes; add it here so
	// the tree matches what agx help --json prints.
	root.InitDefaultHelpCmd()
	return cli.HelpTree(root)
}

// visibleCommands lists every visible command that runs (not a group),
// depth first; hidden commands (the shell layer's) are skipped.
func visibleCommands(h cli.HelpCommand) []cli.HelpCommand {
	var out []cli.HelpCommand
	for _, c := range h.Commands {
		if c.Hidden {
			continue
		}
		if len(c.Commands) == 0 {
			out = append(out, c)
			continue
		}
		out = append(out, visibleCommands(c)...)
	}
	return out
}

// visibleFlags are the documented flag names of one command.
func visibleFlags(h cli.HelpCommand) []string {
	var out []string
	for _, f := range h.Flags {
		if !f.Hidden && f.Name != cobraHelpFlag {
			out = append(out, f.Name)
		}
	}
	return out
}

// referenceSections are the sections of every file under commandsDir.
func referenceSections(t *testing.T, root string) []docSection {
	t.Helper()
	var out []docSection
	for _, md := range publicMarkdown(t, root) {
		if path.Dir(md) == commandsDir {
			out = append(out, sectionsOf(md, readLines(t, root, md))...)
		}
	}
	return out
}

// flagsIn returns the flags named in inline code in body (outside fences).
func flagsIn(body []string) map[string]bool {
	out := map[string]bool{}
	fenced := fencedLines(body)
	for i, l := range body {
		if fenced[i] {
			continue
		}
		for _, m := range inlineFlag.FindAllStringSubmatch(l, -1) {
			out[strings.TrimPrefix(m[1], "--")] = true
		}
	}
	return out
}

// tableFlags returns the flags that start a table row in body, with the
// row's 1-based offset in body.
func tableFlags(body []string) map[string]int {
	out := map[string]int{}
	fenced := fencedLines(body)
	for i, l := range body {
		if m := tableFlagRow.FindStringSubmatch(l); m != nil && !fenced[i] {
			out[strings.TrimPrefix(m[1], "--")] = i + 1
		}
	}
	return out
}

// TestCommandReferenceDrift fails when the command reference misses a
// command or a flag of agx, or documents one agx does not have.
func TestCommandReferenceDrift(t *testing.T) {
	root := repoRoot(t)
	for _, p := range referenceDrift(commandTree(), referenceSections(t, root)) {
		t.Error(p)
	}
}

// referenceDrift compares the command tree with the reference sections
// and returns every problem as "file:line: reason". Pure, so the unit
// tests drive it with a small tree.
func referenceDrift(tree cli.HelpCommand, sections []docSection) []string {
	var probs []string
	byTitle := map[string]docSection{}
	for _, s := range sections {
		if !strings.HasPrefix(s.title, commandHeadingPrefix) && s.title != globalFlagsHeading {
			continue
		}
		if prev, dup := byTitle[s.title]; dup {
			probs = append(probs, fmt.Sprintf("%s:%d: %q is documented twice (also %s:%d)", s.file, s.line, s.title, prev.file, prev.line))
			continue
		}
		byTitle[s.title] = s
	}

	globals := map[string]bool{cobraHelpFlag: true}
	for _, f := range visibleFlags(tree) {
		globals[f] = true
	}
	probs = append(probs, checkFlags(byTitle, globalFlagsHeading, visibleFlags(tree), globals)...)

	known := map[string]bool{}
	for _, c := range visibleCommands(tree) {
		title := commandHeadingPrefix + c.Path
		known[title] = true
		probs = append(probs, checkFlags(byTitle, title, visibleFlags(c), globals)...)
	}
	var stale []string
	for title, s := range byTitle {
		if title != globalFlagsHeading && !known[title] {
			stale = append(stale, fmt.Sprintf("%s:%d: %q is documented but agx has no such command", s.file, s.line, title))
		}
	}
	sort.Strings(stale)
	return append(probs, stale...)
}

// checkFlags checks one command's section: it exists, names every flag,
// and its flag table lists only real flags (or globals, which any section
// may mention).
func checkFlags(byTitle map[string]docSection, title string, flags []string, globals map[string]bool) []string {
	s, ok := byTitle[title]
	if !ok {
		return []string{fmt.Sprintf("%s: no section %q (add a heading for it in a file under %s/)", commandsDir, title, commandsDir)}
	}
	var probs []string
	named := flagsIn(s.body)
	have := map[string]bool{}
	for _, f := range flags {
		have[f] = true
		if !named[f] {
			probs = append(probs, fmt.Sprintf("%s:%d: %q does not document --%s", s.file, s.line, title, f))
		}
	}
	rows := tableFlags(s.body)
	names := make([]string, 0, len(rows))
	for f := range rows {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		if !have[f] && !globals[f] {
			probs = append(probs, fmt.Sprintf("%s:%d: %q documents --%s, which it does not have", s.file, s.line+rows[f], title, f))
		}
	}
	return probs
}
