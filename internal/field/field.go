// Package field implements --field PATH over the JSON envelope (spec
// 14.1): dots go down a level, a number indexes a list, "*" is every
// item. There is deliberately nothing more (no filters, no functions);
// jq exists for that.
//
// It walks json.RawMessage values (objects as map[string]json.RawMessage,
// arrays as []json.RawMessage) so no untyped interface{} tree is built.
package field

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Separator and wildcard of a path.
const (
	sep      = "."
	wildcard = "*"
)

// MissingError is a path that does not exist in the document (edge 56).
// It carries the closest existing path for a "did you mean".
type MissingError struct {
	// Path is the path as given.
	Path string
	// Suggestion is the closest existing path; "" when none is close.
	Suggestion string
}

// Error renders "--field P: no such path (did you mean Q?)".
func (e *MissingError) Error() string {
	msg := "--field " + e.Path + ": no such path"
	if e.Suggestion != "" {
		msg += " (did you mean " + e.Suggestion + "?)"
	}
	return msg
}

// Select returns the value at path in doc. A "*" segment collects the rest
// of the path from every item of a list (or every value of an object, in
// key order) into a JSON array.
func Select(doc json.RawMessage, path string) (json.RawMessage, error) {
	return SelectDeclared(doc, path, nil)
}

// SelectDeclared is Select for a command that declares its --field paths
// (the fields(...) declarations help --json prints).
//
// Why it exists: a "*" over an empty list (or object) yields [] whatever
// follows it, so on an empty result `data.*.nosuch` passed and a typo
// printed an empty table or []. When the data cannot check the rest of the
// path, the declarations do: the path must be a declared path, lie under
// one (`decisions.*.key` under `decisions`) or above one (`data.*` above
// `data.*.name`), a numeric index matching a declared "*". Anything else
// is a *MissingError suggesting the closest declared path.
//
// Invariant: data that exists is still checked against itself, exactly as
// Select does; declarations are consulted only for a path the data left
// unchecked. With no declarations it is Select.
func SelectDeclared(doc json.RawMessage, path string, declared []string) (json.RawMessage, error) {
	w := walker{}
	v, ok := w.walk(doc, strings.Split(path, sep))
	if !ok {
		return nil, &MissingError{Path: path, Suggestion: suggest(doc, path)}
	}
	if w.unchecked && len(declared) > 0 && !covered(path, declared) {
		return nil, &MissingError{Path: path, Suggestion: closest(declared, path)}
	}
	return v, nil
}

// walker follows a path through a document. unchecked records that a "*"
// met an empty list or object with path segments still to follow, so
// those segments were never compared with any data.
type walker struct {
	unchecked bool
}

// walk follows segs from v.
func (w *walker) walk(v json.RawMessage, segs []string) (json.RawMessage, bool) {
	if len(segs) == 0 {
		return v, true
	}
	seg, rest := segs[0], segs[1:]
	if seg == wildcard {
		items, ok := children(v)
		if !ok {
			return nil, false
		}
		if len(items) == 0 && len(rest) > 0 {
			w.unchecked = true
		}
		out := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			r, ok := w.walk(it, rest)
			if !ok {
				return nil, false
			}
			out = append(out, r)
		}
		b, err := json.Marshal(out)
		return b, err == nil
	}
	if arr, ok := asArray(v); ok {
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(arr) {
			return nil, false
		}
		return w.walk(arr[i], rest)
	}
	obj, ok := asObject(v)
	if !ok {
		return nil, false
	}
	child, ok := obj[seg]
	if !ok {
		return nil, false
	}
	return w.walk(child, rest)
}

// covered reports whether path agrees with a declared path segment by
// segment over their common length: equal segments, or a numeric index in
// path where the declaration has "*". The shorter one may be a prefix of
// the longer (SelectDeclared says why both directions pass).
func covered(path string, declared []string) bool {
	ps := strings.Split(path, sep)
	for _, d := range declared {
		ds := strings.Split(d, sep)
		n := min(len(ps), len(ds))
		match := true
		for i := range n {
			if ps[i] == ds[i] {
				continue
			}
			if _, err := strconv.Atoi(ps[i]); err == nil && ds[i] == wildcard {
				continue
			}
			match = false
			break
		}
		if match {
			return true
		}
	}
	return false
}

// children lists an array's items or an object's values in key order.
func children(v json.RawMessage) ([]json.RawMessage, bool) {
	if arr, ok := asArray(v); ok {
		return arr, true
	}
	obj, ok := asObject(v)
	if !ok {
		return nil, false
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]json.RawMessage, len(keys))
	for i, k := range keys {
		out[i] = obj[k]
	}
	return out, true
}

// asArray decodes v as a JSON array.
func asArray(v json.RawMessage) ([]json.RawMessage, bool) {
	t := bytes.TrimSpace(v)
	if len(t) == 0 || t[0] != '[' {
		return nil, false
	}
	var arr []json.RawMessage
	return arr, json.Unmarshal(t, &arr) == nil
}

// asObject decodes v as a JSON object.
func asObject(v json.RawMessage) (map[string]json.RawMessage, bool) {
	t := bytes.TrimSpace(v)
	if len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	var obj map[string]json.RawMessage
	return obj, json.Unmarshal(t, &obj) == nil
}

// Paths lists every path in doc, with list indexes as "0" (the first item
// only, to keep the list short) and their "*" forms, for suggestions and
// for help --json field listings.
func Paths(doc json.RawMessage) []string {
	var out []string
	collect(doc, "", &out)
	sort.Strings(out)
	return out
}

// collect appends every path under v to out.
func collect(v json.RawMessage, prefix string, out *[]string) {
	join := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + sep + k
	}
	if arr, ok := asArray(v); ok {
		if len(arr) > 0 {
			*out = append(*out, join("0"), join(wildcard))
			collect(arr[0], join("0"), out)
			collect(arr[0], join(wildcard), out)
		}
		return
	}
	if obj, ok := asObject(v); ok {
		for k, c := range obj {
			*out = append(*out, join(k))
			collect(c, join(k), out)
		}
	}
}

// suggestMaxDistance is the largest edit distance a suggestion may have;
// beyond it a "did you mean" is noise.
const suggestMaxDistance = 3

// suggest returns the existing path closest to path.
func suggest(doc json.RawMessage, path string) string {
	return closest(Paths(doc), path)
}

// closest returns the candidate closest to path, or "" when none is
// within suggestMaxDistance.
func closest(cands []string, path string) string {
	best, bestD := "", suggestMaxDistance+1
	for _, p := range cands {
		if d := distance(p, path); d < bestD {
			best, bestD = p, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// Bare renders one selected value the way a single --field prints it: a
// JSON string as its text, anything else as compact JSON.
func Bare(v json.RawMessage) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, v); err != nil {
		return string(v)
	}
	return buf.String()
}

// Many renders several selected values as one object keyed by path, in
// the order the paths were given.
func Many(paths []string, vals []json.RawMessage) (json.RawMessage, error) {
	if len(paths) != len(vals) {
		return nil, fmt.Errorf("field: %d paths, %d values", len(paths), len(vals))
	}
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range paths {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(vals[i])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Column is one table column selected by --field data.*.KEY: KEY is the
// row key. ok is false for a path that is not of that form.
func Column(path string) (string, bool) {
	k, ok := strings.CutPrefix(path, "data.*.")
	if !ok || k == "" || strings.Contains(k, sep) {
		return "", false
	}
	return k, true
}
