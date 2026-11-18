package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// jsonObject is one level of a settings or options object: keys to raw
// values, so agx edits settings it does not model without decoding (and
// losing) them, and without untyped values.
type jsonObject = map[string]json.RawMessage

// pathSep separates the levels of a dotted settings key
// ("statusLine.command").
const pathSep = "."

// errNotObject is a settings value that must be a JSON object and is not.
var errNotObject = errors.New("not a JSON object")

// decodeObject reads raw as an object; empty raw is an empty object.
func decodeObject(raw agentio.Raw) (jsonObject, error) {
	obj := jsonObject{}
	if isEmptyRaw(raw) {
		return obj, nil
	}
	if bytes.TrimSpace(raw)[0] != '{' {
		return nil, errNotObject
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// isEmptyRaw reports no value at all (nil, blank, null or {}).
func isEmptyRaw(raw agentio.Raw) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || agentio.IsEmptyJSON(t)
}

// encodeObject writes obj back (keys sorted by encoding/json, so output
// is deterministic); nil for an empty object.
func encodeObject(obj jsonObject) (agentio.Raw, error) {
	if len(obj) == 0 {
		return nil, nil
	}
	return json.Marshal(obj)
}

// setPath sets the dotted path in obj to val, creating (or replacing a
// non-object value with) objects on the way: "--setting a.b=1" over
// {"a":{"c":2}} gives {"a":{"b":1,"c":2}}.
func setPath(obj agentio.Raw, path []string, val agentio.Raw) (agentio.Raw, error) {
	m, err := decodeObject(obj)
	if err != nil {
		m = jsonObject{}
	}
	if len(path) == 1 {
		m[path[0]] = val
		return json.Marshal(m)
	}
	child, err := setPath(m[path[0]], path[1:], val)
	if err != nil {
		return nil, err
	}
	m[path[0]] = child
	return json.Marshal(m)
}

// deletePath removes the dotted path from obj; a missing path is a no-op.
// An object left empty by the removal is removed too.
func deletePath(obj agentio.Raw, path []string) (agentio.Raw, error) {
	m, err := decodeObject(obj)
	if err != nil {
		return obj, nil
	}
	child, ok := m[path[0]]
	if !ok {
		return obj, nil
	}
	if len(path) == 1 {
		delete(m, path[0])
	} else {
		c, err := deletePath(child, path[1:])
		if err != nil {
			return nil, err
		}
		if isEmptyRaw(c) {
			delete(m, path[0])
		} else {
			m[path[0]] = c
		}
	}
	return encodeObject(m)
}

// leafPaths lists the dotted paths of obj's non-object values, sorted:
// what a decision shows (keys only, never values, which may be secret).
func leafPaths(obj agentio.Raw) []string {
	m, err := decodeObject(obj)
	if err != nil {
		return nil
	}
	var out []string
	for k, v := range m {
		if sub, err := decodeObject(v); err == nil && len(sub) > 0 {
			for _, p := range leafPaths(v) {
				out = append(out, k+pathSep+p)
			}
			continue
		}
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// topKeys lists obj's top-level keys, sorted.
func topKeys(obj agentio.Raw) []string {
	m, err := decodeObject(obj)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// pathsOverlap reports two dotted paths where one contains the other
// ("permissions" and "permissions.defaultMode").
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+pathSep) || strings.HasPrefix(b, a+pathSep)
}

// settingValue is a --setting value as JSON (spec 7.0 recipe 3): text
// that parses as JSON (30, true, ["a"], {"k":1}, "quoted") is that JSON;
// anything else is a string.
func settingValue(s string) agentio.Raw {
	if t := strings.TrimSpace(s); t != "" && json.Valid([]byte(t)) {
		return agentio.Raw(t)
	}
	// Marshalling a Go string cannot fail (invalid UTF-8 is replaced).
	b, _ := json.Marshal(s)
	return b
}
