package docscheck

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// rootModule is the module line that marks the repository root.
const rootModule = "module github.com/ubgo/agx"

// rootMarkdown are the public markdown files checked at the repo root.
var rootMarkdown = map[string]bool{
	"README.md": true, "CHANGELOG.md": true, "CONTRIBUTING.md": true, "SECURITY.md": true,
}

// skippedSegments are path segments whose contents are never public docs:
// fixture homes (testdata) and the private design docs (docsi).
var skippedSegments = map[string]bool{"testdata": true, "docsi": true}

// repoRoot walks up from the package directory to the directory whose go.mod
// declares the root module. go.work is not used: it is gitignored and absent
// in a fresh clone.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && firstLine(b) == rootModule {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found: no go.mod declaring " + rootModule)
		}
		dir = parent
	}
}

// firstLine is the first line of b, trimmed (the module line of a go.mod).
func firstLine(b []byte) string {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	return strings.TrimSpace(string(line))
}

// candidateFiles lists every file of the repo (slash separated, relative to
// root) that git would track: tracked plus untracked-but-not-ignored. When git
// is unavailable (an exported tarball) it falls back to a walk that skips
// dot directories.
func candidateFiles(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard").Output()
	var files []string
	if err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			if f := sc.Text(); f != "" {
				files = append(files, f)
			}
		}
	} else {
		werr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") && d.Name() != ".github" {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				rel, _ := filepath.Rel(root, p)
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		if werr != nil {
			t.Fatal(werr)
		}
	}
	var kept []string
	for _, f := range files {
		if hasSkippedSegment(f) {
			continue
		}
		// Symlinks (CLAUDE.md, docsi) point into the private repo; a deleted
		// tracked file is not there to check.
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		kept = append(kept, f)
	}
	sort.Strings(kept)
	return kept
}

// hasSkippedSegment reports whether any segment of rel is in
// skippedSegments (testdata fixtures, the private design docs), which the
// gates never read.
func hasSkippedSegment(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if skippedSegments[seg] {
			return true
		}
	}
	return false
}

// isPublicMarkdown reports whether rel is one of the checked markdown files.
func isPublicMarkdown(rel string) bool {
	if rootMarkdown[rel] {
		return true
	}
	if isExampleReadme(rel) {
		return true
	}
	return (strings.HasPrefix(rel, ".github/") || strings.HasPrefix(rel, docsDir+"/")) && strings.HasSuffix(rel, ".md")
}

// isExampleReadme reports whether rel is examples/README.md or the
// README.md of one example folder. Markdown deeper down (inside an
// example's home/) is fixture data, not a public doc.
func isExampleReadme(rel string) bool {
	if path.Base(rel) != exampleReadme {
		return false
	}
	dir := path.Dir(rel)
	return dir == examplesDir || path.Dir(dir) == examplesDir
}

// publicText is every file whose text readers see: the public markdown
// and every file of the examples (sessions, configs, home files), for the
// content gates (local paths, private docs, attribution).
func publicText(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, f := range candidateFiles(t, root) {
		if isPublicMarkdown(f) || strings.HasPrefix(f, examplesDir+"/") {
			out = append(out, f)
		}
	}
	return out
}

// publicMarkdown is every public markdown file (isPublicMarkdown) of the
// repo, relative to root: the files every markdown gate checks.
func publicMarkdown(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, f := range candidateFiles(t, root) {
		if isPublicMarkdown(f) {
			out = append(out, f)
		}
	}
	return out
}

// readLines returns the file split on "\n" with any trailing "\r" removed.
// A final newline does not produce an empty last line.
func readLines(t *testing.T, root, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return splitLines(string(b))
}

// splitLines splits s on "\n", dropping a trailing "\r" from each line;
// a final newline does not produce an empty last line, and "" is no lines.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// fence describes a fenced code block opener: its character, run length and
// the indentation of the opening line.
type fence struct {
	char   byte
	n      int
	indent int
}

// minFenceRun is the shortest run of ` or ~ that opens a fence.
const minFenceRun = 3

// openFence parses a fence opener (``` or ~~~). Any indentation is accepted,
// unlike CommonMark's 3 spaces, because fences nested in list items are
// indented further and GitHub still renders them as code.
func openFence(line string) (fence, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	rest := line[indent:]
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return fence{}, false
	}
	n := len(rest) - len(strings.TrimLeft(rest, rest[:1]))
	if n < minFenceRun {
		return fence{}, false
	}
	// A backtick fence's info string may not contain a backtick.
	if rest[0] == '`' && strings.Contains(rest[n:], "`") {
		return fence{}, false
	}
	return fence{char: rest[0], n: n, indent: indent}, true
}

// closes reports whether line closes the fenced block f.
func (f fence) closes(line string) bool {
	rest := strings.Trim(line, " \t")
	if len(rest) < f.n {
		return false
	}
	return strings.Trim(rest, string(f.char)) == ""
}

// frontMatterDelim opens and closes YAML front matter on the first line
// (issue templates under .github/ carry it).
const frontMatterDelim = "---"

// fencedLines marks every line that belongs to a fenced code block, including
// the opening and closing fence lines, and any leading YAML front matter. An
// unclosed fence runs to the end.
func fencedLines(lines []string) []bool {
	in := make([]bool, len(lines))
	start := 0
	if len(lines) > 0 && lines[0] == frontMatterDelim {
		for j := 1; j < len(lines); j++ {
			if lines[j] == frontMatterDelim {
				for k := 0; k <= j; k++ {
					in[k] = true
				}
				start = j + 1
				break
			}
		}
	}
	var cur fence
	open := false
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if open {
			in[i] = true
			if cur.closes(l) {
				open = false
			}
			continue
		}
		if f, ok := openFence(l); ok {
			cur, open = f, true
			in[i] = true
		}
	}
	return in
}

// stripInlineCode removes inline code spans (matching backtick runs) so links
// and paths inside `code` are not treated as markdown.
func stripInlineCode(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		j := i
		for j < len(line) && line[j] == '`' {
			j++
		}
		run := line[i:j]
		end := strings.Index(line[j:], run)
		if end < 0 {
			b.WriteString(run)
			i = j
			continue
		}
		i = j + end + len(run)
	}
	return b.String()
}
