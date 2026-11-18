package field_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ubgo/agx/internal/field"
)

const doc = `{"data":[{"name":"work","home":"~/w","args":["a"]},{"name":"personal","home":"~/.claude"}],"error":null,"decisions":[]}`

func TestSelect(t *testing.T) {
	cases := map[string]string{
		"data.0.name":   `"work"`,
		"data.*.home":   `["~/w","~/.claude"]`,
		"data.1":        `{"name":"personal","home":"~/.claude"}`,
		"error":         `null`,
		"data.0.args.0": `"a"`,
	}
	for path, want := range cases {
		got, err := field.Select(json.RawMessage(doc), path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if string(got) != want && compact(t, got) != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func compact(t *testing.T, b json.RawMessage) string {
	var v json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// TestEdge56Missing names the closest existing path.
func TestEdge56Missing(t *testing.T) {
	_, err := field.Select(json.RawMessage(doc), "data.0.homee")
	var me *field.MissingError
	if !errors.As(err, &me) || err.Error() != "--field data.0.homee: no such path (did you mean data.0.home?)" {
		t.Fatalf("%v", err)
	}
	for _, p := range []string{"data.9.name", "data.x", "error.code", "data.*.nope"} {
		if _, err := field.Select(json.RawMessage(doc), p); err == nil {
			t.Errorf("%s: want error", p)
		}
	}
}

func TestBareAndMany(t *testing.T) {
	if field.Bare(json.RawMessage(`"x y"`)) != "x y" || field.Bare(json.RawMessage(`[1, 2]`)) != "[1,2]" {
		t.Fatal("bare")
	}
	m, err := field.Many([]string{"b", "a"}, []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"s"`)})
	if err != nil || string(m) != `{"b":1,"a":"s"}` {
		t.Fatalf("%s %v", m, err)
	}
	if k, ok := field.Column("data.*.home"); !ok || k != "home" {
		t.Fatal("column")
	}
	if _, ok := field.Column("data.0.home"); ok {
		t.Fatal("column on index")
	}
}

// TestSelectDeclaredOnEmpty: a "*" over an empty list or object leaves
// nothing to check the rest of the path against, so a typo would print an
// empty result. SelectDeclared then checks the path against the command's
// declared --field paths: a declared path (or one under or above it)
// passes with the empty result, anything else is the same "no such path"
// error, with the closest declared path as the suggestion. Without
// declarations it behaves as Select.
func TestSelectDeclaredOnEmpty(t *testing.T) {
	empty := json.RawMessage(`{"data":[],"nested":{"folders":[]},"decisions":[]}`)
	declared := []string{"data.*.name", "data.*.home", "nested.folders.*.folder", "decisions"}
	for _, p := range []string{"data.*.name", "data.*", "nested.folders.*.folder", "decisions.*.key", "data.0"} {
		_, err := field.SelectDeclared(empty, p, declared)
		if p == "data.0" {
			if err == nil {
				t.Errorf("%s: an index into an empty list is missing", p)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	_, err := field.SelectDeclared(empty, "data.*.nmae", declared)
	if err == nil || err.Error() != "--field data.*.nmae: no such path (did you mean data.*.name?)" {
		t.Errorf("typo: %v", err)
	}
	for _, p := range []string{"data.*.nosuch", "nested.folders.*.nosuch"} {
		var me *field.MissingError
		if _, err := field.SelectDeclared(empty, p, declared); !errors.As(err, &me) {
			t.Errorf("%s: %v, want a missing path", p, err)
		}
	}
	if v, err := field.SelectDeclared(empty, "data.*.nosuch", nil); err != nil || string(v) != "[]" {
		t.Errorf("no declarations: %s %v", v, err)
	}
	// Non-empty data is still checked against itself.
	if _, err := field.SelectDeclared(json.RawMessage(doc), "data.*.nope", declared); err == nil {
		t.Error("non-empty data.*.nope: want error")
	}
}
