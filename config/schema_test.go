package config

import (
	"slices"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"
)

// TestSchemaWalk covers the schema walk agx config set types keys by: a
// $ref into $defs, anyOf alternatives, a map's value schema, a struct
// that forbids other keys, and a value that is not an object.
func TestSchemaWalk(t *testing.T) {
	str := &schema.Schema{Types: []string{SchemaString}}
	root := &schema.Schema{
		Defs: map[string]*schema.Schema{"Item": {Types: []string{SchemaObject}, Properties: map[string]*schema.Schema{"name": str}, AdditionalProperties: schema.False()}},
		Properties: map[string]*schema.Schema{
			"item":  {Ref: "#/$defs/Item"},
			"maybe": {AnyOf: []*schema.Schema{{Ref: "#/$defs/Item"}, {Types: []string{SchemaNull}}}},
			"tags":  {Types: []string{SchemaObject}, AdditionalProperties: str},
			"title": str,
		},
	}
	for _, tc := range []struct {
		keys []string
		ok   bool
	}{
		{[]string{"item", "name"}, true},
		{[]string{"maybe", "name"}, true},
		{[]string{"tags", "anything"}, true},
		{[]string{"item", "other"}, false},
		{[]string{"title", "x"}, false},
		{[]string{"nope"}, false},
	} {
		_, err := schemaAt(root, tc.keys...)
		if (err == nil) != tc.ok {
			t.Errorf("%v: %v", tc.keys, err)
		}
	}
	if alts := SchemaAlternatives(root, root.Properties["maybe"]); len(alts) != 2 || alts[0].Properties == nil || !slices.Equal(alts[1].Types, []string{SchemaNull}) {
		t.Errorf("alternatives: %+v", alts)
	}
	if SchemaAlternatives(root, nil) != nil {
		t.Error("nil schema has alternatives")
	}
}

// TestPoolSchemaTightened: every percent is a number from 0 to 100 and
// never null; enabled is a boolean, never null; force still takes null.
func TestPoolSchemaTightened(t *testing.T) {
	s, err := ConfigSchema(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range poolValueKeys {
		if !k.percent {
			continue
		}
		n, err := schemaAt(s, append([]string{"logins", "x", "pool"}, k.keys...)...)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(n.Types, []string{SchemaNumber}) || n.Minimum != "0" || n.Maximum != "100" {
			t.Errorf("%v: %v %s..%s", k.keys, n.Types, n.Minimum, n.Maximum)
		}
	}
	n, err := schemaAt(s, "logins", "x", "pool", "accounts", "m", "enabled")
	if err != nil || !slices.Equal(n.Types, []string{SchemaBoolean}) {
		t.Errorf("enabled: %v %v", n, err)
	}
	n, err = schemaAt(s, "logins", "x", "pool", "force")
	if err != nil || !slices.Contains(n.Types, SchemaNull) {
		t.Errorf("force: %v %v", n, err)
	}
	if err := tightenPoolSchema(&schema.Schema{}); err == nil {
		t.Error("a schema without logins was tightened")
	}
}
