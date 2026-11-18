package docscheck

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// snippetMarker is the line above a fenced block that shows part of an
// example file: <!-- snippet: examples/quickstart/session.txt#first-run -->
// (or without "#region" for the whole file). The block must equal that
// region, so a doc can never show a session that no longer runs.
var snippetMarker = regexp.MustCompile(`^\s*<!--\s*snippet:\s*(\S+?)\s*-->\s*$`)

// snippetRegionSep separates the file from the region in a snippet target
// (examples/x/session.txt#first-run), as an anchor does in a link.
const snippetRegionSep = "#"

// snippetEnd stands for the missing side when a snippet and its source
// differ in length.
const snippetEnd = "<end>"

// snippetSource is a loader of an example file's lines by its path
// relative to the repo root; tests swap it for an in-memory one.
type snippetSource func(rel string) ([]string, error)

// repoSnippetSource reads example files from the repository at root.
func repoSnippetSource(root string) snippetSource {
	return func(rel string) ([]string, error) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		return splitLines(string(b)), nil
	}
}

// checkSnippets returns a "file:line: reason" problem for every snippet of
// one markdown file that is malformed or differs from its source.
func checkSnippets(md string, lines []string, load snippetSource) []string {
	var probs []string
	for _, i := range snippetMarkers(lines) {
		target := snippetMarker.FindStringSubmatch(lines[i])[1]
		if p := checkSnippet(target, lines, i+1, load); p != "" {
			probs = append(probs, fmt.Sprintf("%s:%d: %s", md, i+1, p))
		}
	}
	return probs
}

// snippetMarkers returns the 0-based lines of every snippet marker outside
// fenced code, in file order.
func snippetMarkers(lines []string) []int {
	var out []int
	fenced := fencedLines(lines)
	for i, l := range lines {
		if !fenced[i] && snippetMarker.MatchString(l) {
			out = append(out, i)
		}
	}
	return out
}

// snippetBlock is a resolved snippet: the fenced block under a marker and
// the source region it must equal.
type snippetBlock struct {
	// open and end are the 0-based lines of the opening and closing fence.
	open, end int
	// indent is the opening fence's indentation, which every body line
	// carries in the doc (a block inside a list item) but not in the source.
	indent int
	// doc is the block's body with the indentation removed.
	doc []string
	// want is the source region, marker lines excluded.
	want []string
}

// resolveSnippet finds the fenced block opening at line open (0-based) and
// loads target ("examples/x/file#region"); the string is the problem when
// the marker, the block or the source is malformed.
func resolveSnippet(target string, lines []string, open int, load snippetSource) (snippetBlock, string) {
	rel, region, _ := strings.Cut(target, snippetRegionSep)
	if clean := path.Clean(rel); clean != rel || !strings.HasPrefix(rel, examplesDir+"/") {
		return snippetBlock{}, fmt.Sprintf("snippet source %q must be a clean path under %s/", rel, examplesDir)
	}
	if open >= len(lines) {
		return snippetBlock{}, "snippet marker not followed by a fenced block"
	}
	f, ok := openFence(lines[open])
	if !ok {
		return snippetBlock{}, "snippet marker not followed by a fenced block"
	}
	end := -1
	for j := open + 1; j < len(lines); j++ {
		if f.closes(lines[j]) {
			end = j
			break
		}
	}
	if end < 0 {
		return snippetBlock{}, "snippet block is never closed"
	}
	doc := make([]string, 0, end-open-1)
	for _, b := range lines[open+1 : end] {
		doc = append(doc, strings.TrimPrefix(b, strings.Repeat(" ", f.indent)))
	}
	src, err := load(rel)
	if err != nil {
		return snippetBlock{}, fmt.Sprintf("snippet source %s: %v", rel, err)
	}
	rs, err := parseRegions(rel, src)
	if err != nil {
		return snippetBlock{}, fmt.Sprintf("snippet source %v", err)
	}
	want, ok := rs.region(region)
	if !ok {
		return snippetBlock{}, fmt.Sprintf("snippet source %s: no region %q", target, region)
	}
	return snippetBlock{open: open, end: end, indent: f.indent, doc: doc, want: want}, ""
}

// checkSnippet compares the fenced block opening at line open (0-based)
// with target ("examples/x/file#region"); it returns "" when they match.
func checkSnippet(target string, lines []string, open int, load snippetSource) string {
	b, p := resolveSnippet(target, lines, open, load)
	if p != "" {
		return p
	}
	if n, d, w := firstDifference(trimForCompare(b.doc), trimForCompare(b.want)); n != 0 {
		return fmt.Sprintf("snippet differs from %s (first difference at line %d: doc %s vs file %s)", target, n, d, w)
	}
	return ""
}

// updateSnippets returns lines with the body of every snippet block that
// differs from its source replaced by that source (trailing whitespace and
// trailing blank lines dropped, the fence's indentation added); markers,
// fences and matching blocks stay as written. Malformed snippets are left
// alone and returned as "file:line: reason" problems, as checkSnippets
// reports them.
func updateSnippets(md string, lines []string, load snippetSource) ([]string, []string) {
	out := append([]string(nil), lines...)
	var probs []string
	markers := snippetMarkers(lines)
	// Bottom up, so replacing a block never moves a marker not yet handled.
	for k := len(markers) - 1; k >= 0; k-- {
		i := markers[k]
		target := snippetMarker.FindStringSubmatch(lines[i])[1]
		b, p := resolveSnippet(target, lines, i+1, load)
		if p != "" {
			probs = append(probs, fmt.Sprintf("%s:%d: %s", md, i+1, p))
			continue
		}
		want := trimForCompare(b.want)
		if n, _, _ := firstDifference(trimForCompare(b.doc), want); n == 0 {
			continue
		}
		pad := strings.Repeat(" ", b.indent)
		body := make([]string, 0, len(want))
		for _, w := range want {
			if w != "" {
				w = pad + w
			}
			body = append(body, w)
		}
		out = append(append(append([]string(nil), out[:b.open+1]...), body...), out[b.end:]...)
	}
	slices.Reverse(probs)
	return out, probs
}

// firstDifference returns the 1-based line where doc and file first
// differ and both sides of it (quoted, or snippetEnd past the end), or 0
// when they are equal.
func firstDifference(doc, file []string) (int, string, string) {
	side := func(lines []string, i int) string {
		if i < len(lines) {
			return strconv.Quote(lines[i])
		}
		return snippetEnd
	}
	for i := 0; i < len(doc) || i < len(file); i++ {
		if i >= len(doc) || i >= len(file) || doc[i] != file[i] {
			return i + 1, side(doc, i), side(file, i)
		}
	}
	return 0, "", ""
}

// TestSnippets fails when a doc snippet differs from its example file.
// With -args -update it rewrites every differing block from its source
// instead (go test -count=1 ./internal/docscheck/ -run TestSnippets -args
// -update) and fails only on malformed snippets.
func TestSnippets(t *testing.T) {
	root := repoRoot(t)
	load := repoSnippetSource(root)
	for _, md := range publicMarkdown(t, root) {
		lines := readLines(t, root, md)
		if !updating() {
			for _, p := range checkSnippets(md, lines, load) {
				t.Error(p)
			}
			continue
		}
		next, probs := updateSnippets(md, lines, load)
		for _, p := range probs {
			t.Error(p)
		}
		if slices.Equal(next, lines) {
			continue
		}
		p := filepath.Join(root, filepath.FromSlash(md))
		if err := os.WriteFile(p, []byte(strings.Join(next, "\n")+"\n"), generatedFileMode); err != nil {
			t.Fatal(err)
		}
	}
}
