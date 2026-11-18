package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
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
	// Type is the JSON type written: string, boolean or integer.
	Type string `json:"type"`
}

// JSON scalar type names a config key can hold.
const (
	typeString  = "string"
	typeBoolean = "boolean"
	typeInteger = "integer"
)

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
	shown := value
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

// scalarKey checks a dotted key against the typed config shape (struct
// fields by their json name, any map key) and converts value to the JSON
// of the scalar type found there. A key that names an object or a list is
// refused: those are edited by their own commands (profile edit, project
// use, alias add) or in the file.
func scalarKey(key, value string) ([]string, string, json.RawMessage, error) {
	if key == "" {
		return nil, "", nil, apperr.Usage("--key is required (a dotted config key such as default_agent or sessions.root)")
	}
	keys := strings.Split(key, keySep)
	t := reflect.TypeFor[config.Config]()
	for i, k := range keys {
		if k == "" {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: empty part", key))
		}
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			f, ok := fieldByJSON(t, k)
			if !ok {
				return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: unknown key %s", key, strings.Join(keys[:i+1], keySep))).
					WithHint("keys: " + strings.Join(jsonNames(t), ", "))
			}
			t = f
		case reflect.Map:
			t = t.Elem()
		default:
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s: %s is %s, not an object", key, strings.Join(keys[:i], keySep), kindPhrase(t)))
		}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		b, err := json.Marshal(value)
		return keys, typeString, b, err
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s takes true or false, not %q", key, value))
		}
		return keys, typeBoolean, json.RawMessage(strconv.FormatBool(v)), nil
	case reflect.Int, reflect.Int64:
		v, err := strconv.Atoi(value)
		if err != nil {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s takes an integer, not %q", key, value))
		}
		return keys, typeInteger, json.RawMessage(strconv.Itoa(v)), nil
	}
	return nil, "", nil, apperr.Usage(fmt.Sprintf("--key %s is %s, not a scalar; use its own command (profile edit, project use, alias add) or edit %s", key, kindPhrase(t), config.ConfigFileName))
}

// kindPhrase is kindName with its article ("an object", "a list", "an
// int"), so a message never reads "is a object".
func kindPhrase(t reflect.Type) string {
	n := kindName(t)
	if strings.ContainsRune(vowels, rune(n[0])) {
		return "an " + n
	}
	return "a " + n
}

// vowels are the first letters that take "an" (every kind name is a
// lower-case English word, so the letter decides).
const vowels = "aeiou"

// kindName names a Go kind in JSON terms for messages.
func kindName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Slice, reflect.Array:
		return "list"
	}
	return t.Kind().String()
}

// fieldByJSON finds a struct field by its json name.
func fieldByJSON(t reflect.Type, name string) (reflect.Type, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if n, _, _ := strings.Cut(f.Tag.Get("json"), ","); n == name && n != "-" {
			return f.Type, true
		}
	}
	return nil, false
}

// jsonNames lists a struct's json field names, for hints.
func jsonNames(t reflect.Type) []string {
	var out []string
	for i := range t.NumField() {
		if n, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); n != "" && n != "-" {
			out = append(out, n)
		}
	}
	return out
}
