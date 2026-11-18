package docscheck

import (
	"fmt"
	"regexp"
	"strings"
)

// Region markers name a part of an example file that a doc snippet can
// show. They work in any file type because the marker may sit behind that
// file's own comment token, so the file stays valid where it is used:
//
//	#region first-run         (session.txt)
//	// #region profiles       (JSONC)
//	# #region tokens          (dotenv, shell)
//	<!-- #region intro -->    (markdown, HTML)
//
// "#endregion" (optionally "#endregion <name>") closes the innermost open
// region. Marker lines belong to no region and are dropped from every
// snippet and from the session runner; regions may nest.
var (
	regionStart = regexp.MustCompile(`^\s*(?:(?://|#|--|;|<!--)\s*)?#region\s+([A-Za-z0-9_.-]+)\s*(?:-->)?\s*$`)
	regionEnd   = regexp.MustCompile(`^\s*(?:(?://|#|--|;|<!--)\s*)?#endregion(?:\s+([A-Za-z0-9_.-]+))?\s*(?:-->)?\s*$`)
)

// isRegionMarker reports whether line opens or closes a region.
func isRegionMarker(line string) bool {
	return regionStart.MatchString(line) || regionEnd.MatchString(line)
}

// regionSet is a file split into its regions.
type regionSet struct {
	// body is every line of the file except the marker lines (the
	// whole-file snippet).
	body []string
	// regions maps a region name to its lines, marker lines excluded.
	regions map[string][]string
}

// openRegion is a region whose #endregion has not been seen yet.
type openRegion struct {
	name string
	line int // 1-based line of its #region marker
}

// parseRegions splits the lines of file into regions. A region opened
// twice, an #endregion without an open region or naming another one, and a
// region never closed are errors that read "file:line: reason".
func parseRegions(file string, lines []string) (regionSet, error) {
	rs := regionSet{regions: map[string][]string{}}
	opened := map[string]int{}
	var stack []openRegion
	for i, l := range lines {
		if m := regionStart.FindStringSubmatch(l); m != nil {
			if at, dup := opened[m[1]]; dup {
				return rs, fmt.Errorf("%s:%d: region %q already opened at line %d", file, i+1, m[1], at)
			}
			opened[m[1]] = i + 1
			rs.regions[m[1]] = []string{}
			stack = append(stack, openRegion{name: m[1], line: i + 1})
			continue
		}
		if m := regionEnd.FindStringSubmatch(l); m != nil {
			if len(stack) == 0 {
				return rs, fmt.Errorf("%s:%d: #endregion without an open region", file, i+1)
			}
			top := stack[len(stack)-1]
			if m[1] != "" && m[1] != top.name {
				return rs, fmt.Errorf("%s:%d: #endregion %s closes region %q (opened at line %d)", file, i+1, m[1], top.name, top.line)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		rs.body = append(rs.body, l)
		for _, o := range stack {
			rs.regions[o.name] = append(rs.regions[o.name], l)
		}
	}
	if len(stack) > 0 {
		top := stack[len(stack)-1]
		return rs, fmt.Errorf("%s:%d: region %q is never closed", file, top.line, top.name)
	}
	return rs, nil
}

// region returns the lines of region name, or the whole file without its
// marker lines when name is "". ok is false when the file has no such
// region.
func (rs regionSet) region(name string) ([]string, bool) {
	if name == "" {
		return rs.body, true
	}
	r, ok := rs.regions[name]
	return r, ok
}

// trimForCompare drops trailing whitespace of every line and trailing
// blank lines, so a snippet compares by content, not by editor habits.
func trimForCompare(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}
