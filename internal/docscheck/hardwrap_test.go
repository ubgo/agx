package docscheck

import (
	"regexp"
	"strings"
	"testing"
)

var (
	// listItem matches a bullet or ordered list marker followed by a space or
	// the end of the line.
	listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d{1,9}[.)])(?:\s|$)`)
	// thematicBreak matches ---, ***, ___ (with optional spaces between).
	thematicBreak = regexp.MustCompile(`^ {0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
)

// indentedCodeWidth is the indentation (in spaces, a tab counting as four)
// that makes a line outside a paragraph indented code.
const indentedCodeWidth = 4

// lineKind classifies a markdown line for the hard-wrap check.
type lineKind int

const (
	kindBlank      lineKind = iota
	kindStructural          // heading, table, html, blockquote, rule, definition: never part of a paragraph
	kindListItem            // starts a list item; its text may be hard-wrapped
	kindText                // plain paragraph text
)

// isStructural reports whether a non-blank line is a block construct other
// than paragraph text or a list item.
func isStructural(line string) bool {
	t := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(t, "#"):
		return atxHeading.MatchString(line)
	case strings.HasPrefix(t, "|"), strings.HasSuffix(t, "|"):
		return true
	case strings.HasPrefix(t, "<"), strings.HasPrefix(t, ">"):
		return true
	case setextUnderline.MatchString(line), thematicBreak.MatchString(line):
		return true
	case refDef.MatchString(line):
		return true
	}
	return false
}

// classify tells what one markdown line is, for the hard-wrap scan.
func classify(line string) lineKind {
	switch {
	case strings.TrimSpace(line) == "":
		return kindBlank
	case isStructural(line):
		return kindStructural
	case listItem.MatchString(line):
		return kindListItem
	}
	return kindText
}

// indentWidth is the column a line's text starts at, a tab counting as
// indentedCodeWidth columns, so indents compare by width.
func indentWidth(line string) int {
	w := 0
	for _, r := range line {
		switch r {
		case ' ':
			w++
		case '\t':
			w += indentedCodeWidth
		default:
			return w
		}
	}
	return w
}

// hardWraps returns the 0-based indexes of lines that are paragraph or list
// text immediately continued by a line of plain text: the source of a
// hard-wrapped paragraph. Fenced code, indented code and HTML blocks (an HTML
// line up to the next blank line) are exempt.
func hardWraps(lines []string) []int {
	var out []int
	fenced := fencedLines(lines)
	prev := kindBlank // kind of the previous line, as it counts for wrapping
	inHTML, inCode := false, false
	for i, l := range lines {
		if fenced[i] {
			prev, inHTML, inCode = kindStructural, false, false
			continue
		}
		k := classify(l)
		if k == kindBlank {
			prev, inHTML = kindBlank, false
			continue
		}
		if inHTML {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "<") {
			inHTML, inCode, prev = true, false, kindStructural
			continue
		}
		// Indented code cannot interrupt a paragraph, so a deeply indented
		// line after text is a lazy continuation, not code.
		if indentWidth(l) >= indentedCodeWidth && (prev == kindBlank || prev == kindStructural || inCode) {
			inCode = inCode || prev == kindBlank
			prev = kindStructural
			continue
		}
		inCode = false
		if k == kindText && (prev == kindText || prev == kindListItem) {
			out = append(out, i-1)
		}
		prev = k
	}
	return out
}

// TestNoHardWrap fails on a paragraph or bullet that continues on the
// next physical line in any public markdown file.
func TestNoHardWrap(t *testing.T) {
	root := repoRoot(t)
	for _, md := range publicMarkdown(t, root) {
		for _, i := range hardWraps(readLines(t, root, md)) {
			t.Errorf("%s:%d: hard-wrapped line (the paragraph or bullet continues on the next line; keep it on one physical line)", md, i+1)
		}
	}
}
