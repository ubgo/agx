package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
)

// keySep separates the levels of a config set --key ("profiles.work.home").
const keySep = "."

// redactedText replaces a value that may be a secret in every output.
const redactedText = "[redacted]"

// secretKeys are config keys whose values may be secrets: a login token
// (a reference or a plain value) and literal profile environment values.
// Their values are never echoed (principle 12).
var secretKeys = []string{"token", "env"}

// ConfigSetResult is what agx config set wrote.
type ConfigSetResult struct {
	// Key is the dotted key as given.
	Key string `json:"key"`
	// Value is the value written (redacted for secret keys).
	Value string `json:"value"`
	// Type is the JSON type written: string, number, integer, boolean or
	// null.
	Type string `json:"type"`
}

// ConfigSet is agx config set: one scalar key, validated, comment-
// preserving.
func (a *App) ConfigSet(ctx context.Context, key, value string) (ConfigSetResult, error) {
	keys, typ, v, err := scalarKey(key, value)
	if err != nil {
		return ConfigSetResult{}, err
	}
	if err := a.setKey(ctx, keys, v); err != nil {
		return ConfigSetResult{}, err
	}
	// The value as written: "1e2" was written as 100, "\"x\"" as x.
	shown := string(v)
	if typ == config.SchemaString {
		_ = json.Unmarshal(v, &shown) // v is a string scalarKey just marshaled
	}
	if isSecretPath(keys) {
		shown = redactedText
	}
	return ConfigSetResult{Key: key, Value: shown, Type: typ}, nil
}

// SetConfigKey sets one scalar key of config.jsonc, keeping comments and
// layout (agx config set; the agent picker's "remember"). The key must be
// a scalar of the config shape and the result must still load.
func (a *App) SetConfigKey(ctx context.Context, key, value string) error {
	keys, _, v, err := scalarKey(key, value)
	if err != nil {
		return err
	}
	return a.setKey(ctx, keys, v)
}

// setKey writes v at keys through the validated edit.
func (a *App) setKey(ctx context.Context, keys []string, v json.RawMessage) error {
	return a.editConfig(ctx, func(d *jsonc.Doc) error {
		return d.Set(config.Pointer(keys...), jsonc.Raw(v))
	})
}

// isSecretPath reports a key path that may hold a secret value.
func isSecretPath(keys []string) bool {
	return slices.ContainsFunc(keys, func(k string) bool { return slices.Contains(secretKeys, k) })
}

// scalarKey checks a dotted key against the config schema (the one the
// load validates with: struct fields by their json name, any map key,
// each agent's typed profile block) and converts value to the JSON of the
// scalar type the schema gives that key, whether the key is in the file
// yet or not. A key that names an object or a list is refused: those are
// edited by their own commands (profile edit, project use, alias add,
// login edit) or in the file.
func scalarKey(key, value string) ([]string, string, json.RawMessage, error) {
	if key == "" {
		return nil, "", nil, apperr.Usage("--key is required (a dotted config key such as default_agent or sessions.root)")
	}
	root, err := config.ConfigSchema(agents.BlockSchemas())
	if err != nil {
		return nil, "", nil, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, "config schema: "+err.Error())
	}
	keys := strings.Split(key, keySep)
	n := root
	for i, k := range keys {
		if k == "" {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: empty part", key))
		}
		c, ok := config.SchemaChild(root, n, k)
		if ok {
			n = c
			continue
		}
		at := strings.Join(keys[:i], keySep)
		if kind := valueKind(root, n); kind != config.SchemaObject {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: %s is %s, not an object", key, at, kindPhrase(kind)))
		}
		return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: unknown key %s", key, strings.Join(keys[:i+1], keySep))).
			WithHint("keys: " + strings.Join(propertyNames(root, n), ", "))
	}
	t := scalarTypeOf(root, n)
	if t.container != "" {
		return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s is %s, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit %s", key, kindPhrase(t.container), config.ConfigFileName))
	}
	typ, raw, ok := t.parse(value)
	if !ok {
		return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s takes %s, not %q", key, t.expected(), value))
	}
	return keys, typ, raw, nil
}

// nullText is how --value writes JSON null on a key that takes it.
const nullText = "null"

// scalarType is what the schema allows at one key: its JSON types, the
// allowed strings of an enum and the bounds of a number.
type scalarType struct {
	// types are the scalar JSON types, in schema order.
	types []string
	// container is "object" or "array" when the key holds one (refused).
	container string
	// enum are the allowed strings ("" none).
	enum []string
	// minimum and maximum bound a number or integer ("" = unbounded).
	minimum, maximum json.Number
}

// scalarTypeOf collects the scalar types of every alternative of n.
func scalarTypeOf(root, n *schema.Schema) scalarType {
	var t scalarType
	for _, alt := range config.SchemaAlternatives(root, n) {
		types := alt.Types
		if len(types) == 0 && (alt.Properties != nil || alt.AdditionalProperties != nil) {
			types = []string{config.SchemaObject}
		}
		for _, ty := range types {
			switch ty {
			case config.SchemaObject, config.SchemaArray:
				if t.container == "" {
					t.container = ty
				}
			default:
				if !slices.Contains(t.types, ty) {
					t.types = append(t.types, ty)
				}
			}
		}
		for _, e := range alt.Enum {
			var s string
			if json.Unmarshal(e, &s) == nil {
				t.enum = append(t.enum, s)
			}
		}
		if alt.Minimum != "" {
			t.minimum = alt.Minimum
		}
		if alt.Maximum != "" {
			t.maximum = alt.Maximum
		}
	}
	return t
}

// parse converts value to the JSON of the first type that takes it: null
// (when allowed), a string (an enum value; a JSON-quoted string is
// unquoted, so "\"null\"" writes the word on a key that also takes null),
// a boolean, an integer, then a number. ok is false when no type does.
func (t scalarType) parse(value string) (typ string, raw json.RawMessage, ok bool) {
	has := func(ty string) bool { return slices.Contains(t.types, ty) }
	if value == nullText && has(config.SchemaNull) {
		return config.SchemaNull, json.RawMessage(nullText), true
	}
	if has(config.SchemaString) {
		s := value
		var quoted string
		if strings.HasPrefix(value, `"`) && json.Unmarshal([]byte(value), &quoted) == nil {
			s = quoted
		}
		if len(t.enum) == 0 || slices.Contains(t.enum, s) {
			b, _ := json.Marshal(s) // encoding/json cannot fail on a string
			return config.SchemaString, b, true
		}
	}
	if has(config.SchemaBoolean) {
		if v, err := strconv.ParseBool(value); err == nil {
			return config.SchemaBoolean, json.RawMessage(strconv.FormatBool(v)), true
		}
	}
	if has(config.SchemaInteger) {
		if v, err := strconv.Atoi(value); err == nil && t.inRange(float64(v)) {
			return config.SchemaInteger, json.RawMessage(strconv.Itoa(v)), true
		}
	}
	if has(config.SchemaNumber) {
		// ParseFloat reads "NaN" and "Inf" too; JSON has neither.
		if v, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && t.inRange(v) {
			return config.SchemaNumber, json.RawMessage(strconv.FormatFloat(v, 'f', -1, 64)), true
		}
	}
	return "", nil, false
}

// inRange checks v against the schema bounds.
func (t scalarType) inRange(v float64) bool {
	if lo, err := t.minimum.Float64(); t.minimum != "" && err == nil && v < lo {
		return false
	}
	if hi, err := t.maximum.Float64(); t.maximum != "" && err == nil && v > hi {
		return false
	}
	return true
}

// expected names what the key takes, in words ("a number from 0 to 100",
// "true or false", "one of: plan, api", "a string or null"): never a Go
// type name.
func (t scalarType) expected() string {
	var parts []string
	for _, ty := range t.types {
		switch ty {
		case config.SchemaString:
			if len(t.enum) > 0 {
				parts = append(parts, "one of: "+strings.Join(t.enum, ", "))
			} else {
				parts = append(parts, "a string")
			}
		case config.SchemaBoolean:
			parts = append(parts, "true or false")
		case config.SchemaInteger:
			parts = append(parts, "an integer"+t.bounds())
		case config.SchemaNumber:
			parts = append(parts, "a number"+t.bounds())
		}
	}
	if slices.Contains(t.types, config.SchemaNull) {
		parts = append(parts, nullText)
	}
	return strings.Join(parts, " or ")
}

// bounds words the schema range: " from 0 to 100", " of at least 1",
// " of at most 9", or "" when unbounded.
func (t scalarType) bounds() string {
	switch {
	case t.minimum != "" && t.maximum != "":
		return fmt.Sprintf(" from %s to %s", t.minimum, t.maximum)
	case t.minimum != "":
		return " of at least " + string(t.minimum)
	case t.maximum != "":
		return " of at most " + string(t.maximum)
	}
	return ""
}

// valueKind is the JSON kind a value at n has, for messages: object,
// array, or the first scalar type.
func valueKind(root, n *schema.Schema) string {
	t := scalarTypeOf(root, n)
	switch {
	case t.container != "":
		return t.container
	case len(t.types) > 0:
		return t.types[0]
	}
	return config.SchemaObject
}

// kindPhrase names a JSON kind with its article for messages ("an
// object", "a list", "a number"), so a message never reads "is a object"
// and never names a Go type.
func kindPhrase(kind string) string {
	switch kind {
	case config.SchemaObject:
		return "an object"
	case config.SchemaArray:
		return "a list"
	case config.SchemaInteger:
		return "an integer"
	}
	return "a " + kind
}

// propertyNames lists the keys of an object schema in schema order, for
// hints.
func propertyNames(root, n *schema.Schema) []string {
	var out []string
	for _, alt := range config.SchemaAlternatives(root, n) {
		for _, k := range alt.PropertyOrder {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}
