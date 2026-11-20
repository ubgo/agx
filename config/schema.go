package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/pool"
)

// JSON Schema type names a config key can hold.
const (
	SchemaString  = "string"
	SchemaNumber  = "number"
	SchemaInteger = "integer"
	SchemaBoolean = "boolean"
	SchemaNull    = "null"
	SchemaObject  = "object"
	SchemaArray   = "array"
)

// defsPrefix starts a local $ref into the root's $defs.
const defsPrefix = "#/$defs/"

// SchemaAlternatives is n with its $ref followed and its anyOf flattened:
// every schema a value at n may match. A struct pointer field is generated
// as anyOf [object, null], so both halves are returned.
func SchemaAlternatives(root, n *schema.Schema) []*schema.Schema {
	if n == nil {
		return nil
	}
	if n.Ref != "" {
		return SchemaAlternatives(root, root.Defs[strings.TrimPrefix(n.Ref, defsPrefix)])
	}
	if len(n.AnyOf) == 0 {
		return []*schema.Schema{n}
	}
	var out []*schema.Schema
	for _, alt := range n.AnyOf {
		out = append(out, SchemaAlternatives(root, alt)...)
	}
	return out
}

// SchemaChild is the schema of key under n: a property, or the value
// schema of a map (additionalProperties). false = n has no such key (a
// struct without that field, or a value that is not an object).
func SchemaChild(root, n *schema.Schema, key string) (*schema.Schema, bool) {
	for _, alt := range SchemaAlternatives(root, n) {
		if p, ok := alt.Properties[key]; ok {
			return p, true
		}
		if ap := alt.AdditionalProperties; ap != nil && ap.Bool == nil {
			return ap, true
		}
	}
	return nil, false
}

// schemaAt walks keys from root; an error names the first missing key.
func schemaAt(root *schema.Schema, keys ...string) (*schema.Schema, error) {
	n := root
	for i, k := range keys {
		c, ok := SchemaChild(root, n, k)
		if !ok {
			return nil, fmt.Errorf("config schema has no %s", strings.Join(keys[:i+1], "."))
		}
		n = c
	}
	return n, nil
}

// poolValueKeys are the pool keys below logins.<name>.pool ("*" is any
// map key) that need a value of one type: the percents, which are also
// bounded, and the on/off switch.
var poolValueKeys = []struct {
	keys    []string
	typ     string
	percent bool
}{
	{[]string{"short_unavailable_at_percent"}, SchemaNumber, true},
	{[]string{"defaults", "short"}, SchemaNumber, true},
	{[]string{"defaults", "week"}, SchemaNumber, true},
	{[]string{"accounts", "*", "short"}, SchemaNumber, true},
	{[]string{"accounts", "*", "week"}, SchemaNumber, true},
	{[]string{"accounts", "*", "enabled"}, SchemaBoolean, false},
	{[]string{"accounts", "*", "week_relax_max"}, SchemaNumber, true},
	{[]string{"accounts", "*", "capacity"}, SchemaNumber, false},
	{[]string{"own_caps", "short"}, SchemaNumber, true},
	{[]string{"own_caps", "week"}, SchemaNumber, true},
	{[]string{"own_caps", "week_relax_max"}, SchemaNumber, true},
	{[]string{"shared_caps", "short"}, SchemaNumber, true},
	{[]string{"shared_caps", "week"}, SchemaNumber, true},
	{[]string{"shared_caps", "week_relax_max"}, SchemaNumber, true},
	{[]string{"defaults", "week_relax_max"}, SchemaNumber, true},
	{[]string{"max_sessions"}, SchemaInteger, false},
	{[]string{"sticky"}, SchemaBoolean, false},
	{[]string{"sticky_leave_ratio"}, SchemaNumber, false},
}

// tightenPoolSchema states in the schema what the pool rules need, so the
// editor, the load and agx config set all read it from one place: a
// percent is a number from pool.MinPercent to pool.MaxPercent, and a cap,
// the cutoff or enabled is never null. The generated schema allows null
// on every pointer field, and a null there would decode as "use the
// default": a value silently ignored. Only force takes null (off).
func tightenPoolSchema(root *schema.Schema) error {
	for _, k := range poolValueKeys {
		n, err := schemaAt(root, append([]string{"logins", "*", "pool"}, k.keys...)...)
		if err != nil {
			return err
		}
		n.Types = []string{k.typ}
		if k.typ == SchemaInteger || (k.typ == SchemaNumber && !k.percent) {
			// A count (max_sessions) or a ratio: never negative.
			n.Minimum = jsonNumber(0)
		}
		if k.percent {
			n.Minimum = jsonNumber(pool.MinPercent)
			n.Maximum = jsonNumber(pool.MaxPercent)
		}
	}
	return nil
}

// jsonNumber writes v the shortest exact way ("100", not "1e+02").
func jsonNumber(v float64) json.Number {
	return json.Number(strconv.FormatFloat(v, 'f', -1, 64))
}
