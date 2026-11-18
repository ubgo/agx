package docscheck

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	// inlineLink matches the "](target)" tail of inline links and images,
	// with an optional <angle-bracketed> target and "title".
	inlineLink = regexp.MustCompile(`\]\(\s*(<[^>]*>|[^)\s]+)(?:\s+(?:"[^"]*"|'[^']*'))?\s*\)`)
	// refDef matches a link reference definition: [label]: target.
	refDef = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:\s*(<[^>]*>|\S+)`)
	// htmlLink matches src/href attributes of img, a and source tags.
	htmlLink = regexp.MustCompile(`(?i)<(?:img|a|source)\b[^>]*?\s(?:src|href|srcset)\s*=\s*["']([^"']*)["']`)
	// htmlAnchor matches explicit anchors: <a id="x"> or <a name="x">.
	htmlAnchor = regexp.MustCompile(`(?i)<a\b[^>]*?\s(?:id|name)\s*=\s*["']([^"']+)["']`)
	// atxHeading matches "# Title" (closing #s removed later).
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*))?$`)
	// setextUnderline matches the line under a setext heading.
	setextUnderline = regexp.MustCompile(`^ {0,3}(=+|-+)\s*$`)
	// urlScheme matches an absolute URL scheme (http:, mailto:, ...).
	urlScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

	// Inline markup removed from heading text before slugging, the way
	// GitHub slugs the rendered text rather than the source.
	mdImage      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	htmlTag      = regexp.MustCompile(`<[^>]+>`)
	underscoreEm = regexp.MustCompile(`(^|[^\p{L}\p{N}_])_{1,2}([^_]+)_{1,2}($|[^\p{L}\p{N}_])`)
	closingHash  = regexp.MustCompile(`(^|\s+)#+\s*$`)
)

// slugify turns heading text into a GitHub anchor (github-slugger): lowercase,
// remove everything that is not a letter, mark, number, connector punctuation
// ("_"), space or "-", then turn each space into "-".
func slugify(text string) string {
	text = headingText(text)
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-', unicode.IsLetter(r), unicode.IsMark(r), unicode.IsNumber(r), unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// headingText reduces heading source to its rendered text: images become
// their alt text, links their text, HTML tags and code backticks vanish,
// _emphasis_ loses its underscores.
func headingText(s string) string {
	s = mdImage.ReplaceAllString(s, "$1")
	s = mdLink.ReplaceAllString(s, "$1")
	s = htmlTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	for {
		next := underscoreEm.ReplaceAllString(s, "$1$2$3")
		if next == s {
			break
		}
		s = next
	}
	return strings.TrimSpace(s)
}

// slugger assigns unique slugs the way github-slugger does: a repeat of
// "x" becomes "x-1", then "x-2", skipping any slug already taken.
type slugger struct{ seen map[string]int }

// slug returns the next unique slug for a heading's text.
func (s *slugger) slug(text string) string {
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	base := slugify(text)
	result := base
	for {
		if _, taken := s.seen[result]; !taken {
			break
		}
		s.seen[base]++
		result = base + "-" + strconv.Itoa(s.seen[base])
	}
	s.seen[result] = 0
	return result
}

// anchorsOf returns every anchor a markdown document defines: heading slugs
// and explicit <a id>/<a name>. Lines in fenced code are ignored.
func anchorsOf(lines []string) map[string]bool {
	out := map[string]bool{}
	var sl slugger
	fenced := fencedLines(lines)
	for i, l := range lines {
		if fenced[i] {
			continue
		}
		for _, m := range htmlAnchor.FindAllStringSubmatch(stripInlineCode(l), -1) {
			out[m[1]] = true
		}
		if m := atxHeading.FindStringSubmatch(l); m != nil {
			out[sl.slug(closingHash.ReplaceAllString(m[1], ""))] = true
			continue
		}
		// Setext: a one-line paragraph followed by === or ---.
		prevKind := kindBlank
		if i > 0 && !fenced[i-1] {
			prevKind = classify(lines[i-1])
		}
		startsBlock := prevKind == kindBlank || prevKind == kindStructural
		if startsBlock && classify(l) == kindText && i+1 < len(lines) && !fenced[i+1] && setextUnderline.MatchString(lines[i+1]) {
			out[sl.slug(strings.TrimSpace(l))] = true
		}
	}
	return out
}

// linkTargets returns (line index, target) for every link in the document
// outside fenced and inline code.
func linkTargets(lines []string) [][2]string {
	var out [][2]string
	fenced := fencedLines(lines)
	for i, l := range lines {
		if fenced[i] {
			continue
		}
		l = stripInlineCode(l)
		var found []string
		for _, m := range inlineLink.FindAllStringSubmatch(l, -1) {
			found = append(found, m[1])
		}
		if m := refDef.FindStringSubmatch(l); m != nil {
			found = append(found, m[1])
		}
		for _, m := range htmlLink.FindAllStringSubmatch(l, -1) {
			found = append(found, m[1])
		}
		for _, f := range found {
			f = strings.TrimSuffix(strings.TrimPrefix(f, "<"), ">")
			out = append(out, [2]string{strconv.Itoa(i + 1), f})
		}
	}
	return out
}

// TestLinks fails on a relative link or image to a missing file, and on a
// #anchor that no heading or <a id> of the target defines.
func TestLinks(t *testing.T) {
	root := repoRoot(t)
	anchorCache := map[string]map[string]bool{}
	anchors := func(rel string) map[string]bool {
		if a, ok := anchorCache[rel]; ok {
			return a
		}
		a := anchorsOf(readLines(t, root, rel))
		anchorCache[rel] = a
		return a
	}
	for _, md := range publicMarkdown(t, root) {
		for _, lt := range linkTargets(readLines(t, root, md)) {
			where, target := md+":"+lt[0], lt[1]
			if target == "" || urlScheme.MatchString(target) || strings.HasPrefix(target, "//") {
				continue
			}
			p, frag, _ := strings.Cut(target, "#")
			p, _, _ = strings.Cut(p, "?")
			if dp, err := url.PathUnescape(p); err == nil {
				p = dp
			}
			if df, err := url.PathUnescape(frag); err == nil {
				frag = df
			}
			rel := md
			if p != "" {
				if strings.HasPrefix(p, "/") {
					rel = path.Clean(strings.TrimPrefix(p, "/"))
				} else {
					rel = path.Clean(path.Join(path.Dir(md), p))
				}
			}
			if rel == ".." || strings.HasPrefix(rel, "../") {
				t.Errorf("%s: link %q points outside the repository", where, target)
				continue
			}
			fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Errorf("%s: link %q: %s does not exist", where, target, rel)
				continue
			}
			if frag == "" {
				continue
			}
			if fi.IsDir() {
				// GitHub shows the folder's README; anchors refer to it.
				readme := path.Join(rel, "README.md")
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(readme))); err != nil {
					continue
				}
				rel = readme
			}
			if !strings.HasSuffix(rel, ".md") {
				continue // e.g. file.go#L10: line anchors are not checked
			}
			if !anchors(rel)[frag] {
				t.Errorf("%s: link %q: no heading or anchor #%s in %s", where, target, frag, rel)
			}
		}
	}
}
