package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/internal/apperr"
)

// setConfig is the file agx config set edits in these tests: comments in
// several places, a pool with one member entry, no defaults block.
const setConfig = `// agx config (keep this header)
{
  "version": 1,
  "default_agent": "claude", // the agent of a bare run
  "profiles": {
    // personal account
    "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true }
  },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice"], "shared": ["bob"],
      /* alice is mine */
      "accounts": { "alice": { "short": 95, "week": 98 } }
    } }
  }
}
`

// configPath is config.jsonc of a fixtureApp home.
func configPath(home string) string { return filepath.Join(home, ".agx", "config.jsonc") }

// TestConfigSetTypes is agx config set over every scalar kind the schema
// allows, on keys in the file and keys it does not have yet: the value is
// typed by the schema, never by the Go type or the value already there.
func TestConfigSetTypes(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		// typ and shown are the result; want is the text the file then
		// holds (a substring).
		typ, shown, want string
	}{
		{"integer", "version", "1", "integer", "1", `"version": 1,`},
		{"number on an existing key", "logins.auto.pool.accounts.alice.short", "90", "number", "90", `"short": 90`},
		{"fraction", "logins.auto.pool.accounts.alice.week", "97.5", "number", "97.5", `"week": 97.5`},
		{"exponent written plainly", "logins.auto.pool.accounts.alice.short", "1e2", "number", "100", `"short": 100`},
		{"bounds are inclusive (0)", "logins.auto.pool.accounts.alice.short", "0", "number", "0", `"short": 0`},
		{"number on a new member entry", "logins.auto.pool.accounts.bob.short", "95", "number", "95", `"bob": {`},
		{"number in a new block", "logins.auto.pool.defaults.week", "80", "number", "80", `"defaults": {`},
		{"new cutoff", "logins.auto.pool.short_unavailable_at_percent", "98", "number", "98", `"short_unavailable_at_percent": 98`},
		{"boolean", "profiles.personal.live_login", "true", "boolean", "true", `"live_login": true`},
		{"boolean on a member", "logins.auto.pool.accounts.alice.enabled", "false", "boolean", "false", `"enabled": false`},
		{"string", "default_agent", "codex", "string", "codex", `"default_agent": "codex"`},
		{"digits on a string key stay a string", "profiles.personal.model", "95", "string", "95", `"model": "95"`},
		{"quoted string is unquoted", "profiles.personal.model", `"opus"`, "string", "opus", `"model": "opus"`},
		{"enum value", "profiles.personal.billing", "api", "string", "api", `"billing": "api"`},
		{"null where allowed", "logins.auto.pool.force", "null", "null", "null", `"force": null`},
		{"null on a nullable switch", "profiles.personal.inherit_defaults", "null", "null", "null", `"inherit_defaults": null`},
		{"string where null is allowed", "logins.auto.pool.force", "alice", "string", "alice", `"force": "alice"`},
		{"typed agent block", "profiles.personal.claude.effort", "high", "string", "high", `{"effort":"high"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := fixtureApp(t, setConfig, nil, nil)
			res, err := a.ConfigSet(context.Background(), tc.key, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if res.Key != tc.key || res.Type != tc.typ || res.Value != tc.shown {
				t.Errorf("result %+v, want type %s value %s", res, tc.typ, tc.shown)
			}
			b, err := os.ReadFile(configPath(home))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.want, "// agx config (keep this header)", "// the agent of a bare run", "// personal account", "/* alice is mine */"} {
				if !strings.Contains(string(b), want) {
					t.Errorf("file lacks %q:\n%s", want, b)
				}
			}
			if _, err := a.Config(); err != nil {
				t.Errorf("the result does not load: %v", err)
			}
		})
	}
}

// TestConfigSetRefused: every refusal names the key and what it takes in
// words (never a Go type such as float64), and writes nothing.
func TestConfigSetRefused(t *testing.T) {
	const capKey = "logins.auto.pool.accounts.alice.short"
	const capTakes = "--key " + capKey + " takes a number from 0 to 100, not "
	for _, tc := range []struct {
		name, key, value string
		code             apperr.Code
		msg              string
	}{
		{"over 100", capKey, "101", apperr.CodeUsage, capTakes + `"101"`},
		{"below 0", capKey, "-1", apperr.CodeUsage, capTakes + `"-1"`},
		{"NaN", capKey, "NaN", apperr.CodeUsage, capTakes + `"NaN"`},
		{"infinity", capKey, "Inf", apperr.CodeUsage, capTakes + `"Inf"`},
		{"word for a number", capKey, "high", apperr.CodeUsage, capTakes + `"high"`},
		{"quoted number for a number", capKey, `"95"`, apperr.CodeUsage, capTakes + `"\"95\""`},
		{"null where not allowed", capKey, "null", apperr.CodeUsage, capTakes + `"null"`},
		{"over 100 on a new key", "logins.auto.pool.defaults.short", "150", apperr.CodeUsage, `--key logins.auto.pool.defaults.short takes a number from 0 to 100, not "150"`},
		{"null switch", "logins.auto.pool.accounts.alice.enabled", "null", apperr.CodeUsage, `--key logins.auto.pool.accounts.alice.enabled takes true or false, not "null"`},
		{"word for a switch", "profiles.personal.live_login", "maybe", apperr.CodeUsage, `--key profiles.personal.live_login takes true or false, not "maybe"`},
		{"nullable switch", "profiles.personal.inherit_defaults", "maybe", apperr.CodeUsage, `--key profiles.personal.inherit_defaults takes true or false or null, not "maybe"`},
		{"fraction for an integer", "version", "1.5", apperr.CodeUsage, `--key version takes an integer, not "1.5"`},
		{"word for an integer", "version", "one", apperr.CodeUsage, `--key version takes an integer, not "one"`},
		{"outside an enum", "profiles.personal.billing", "free", apperr.CodeUsage, `--key profiles.personal.billing takes one of: plan, api, not "free"`},
		{"unknown key", "nope", "x", apperr.CodeUsage, "--key nope: unknown key nope"},
		{"unknown nested key", "logins.auto.pool.nope", "1", apperr.CodeUsage, "--key logins.auto.pool.nope: unknown key logins.auto.pool.nope"},
		{"under a scalar", "default_agent.x", "1", apperr.CodeUsage, "--key default_agent.x: default_agent is a string, not an object"},
		{"under a number", capKey + ".x", "1", apperr.CodeUsage, "--key " + capKey + ".x: " + capKey + " is a number, not an object"},
		{"object", "profiles", "x", apperr.CodeUsage, "--key profiles is an object, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit config.jsonc"},
		{"pool object", "logins.auto.pool", "x", apperr.CodeUsage, "--key logins.auto.pool is an object, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit config.jsonc"},
		{"list", "logins.auto.pool.own", "alice", apperr.CodeUsage, "--key logins.auto.pool.own is a list, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit config.jsonc"},
		{"empty key", "", "x", apperr.CodeUsage, "--key is required (a dotted config key such as default_agent or sessions.root)"},
		{"empty part", "profiles..home", "x", apperr.CodeUsage, "--key profiles..home: empty part"},
		{"pool rule after the edit", "logins.auto.pool.accounts.zed.short", "50", apperr.CodeConfig, "refused: the change would make config.jsonc invalid: pool: accounts.zed is not listed in own or shared"},
		{"cross-check after the edit", "default_agent", "nope", apperr.CodeConfig, "refused: the change would make config.jsonc invalid: unknown agent nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := fixtureApp(t, setConfig, nil, nil)
			_, err := a.ConfigSet(context.Background(), tc.key, tc.value)
			e := apperr.As(err)
			if e == nil || e.Code != tc.code || !strings.HasPrefix(e.Message, tc.msg) {
				t.Fatalf("error %v, want %s %q", err, tc.code, tc.msg)
			}
			for _, goType := range []string{"float64", "map[", "[]interface", "int64", "*bool"} {
				if strings.Contains(e.Message+e.Hint, goType) {
					t.Errorf("message names a Go type: %s", e.Message)
				}
			}
			b, err := os.ReadFile(configPath(home))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != setConfig {
				t.Errorf("a refused set wrote the file:\n%s", b)
			}
		})
	}
}

// TestConfigSetUnknownKeyHint lists the keys of the object the key is
// under, in schema order.
func TestConfigSetUnknownKeyHint(t *testing.T) {
	a, _ := fixtureApp(t, setConfig, nil, nil)
	_, err := a.ConfigSet(context.Background(), "logins.auto.pool.accounts.alice.shrot", "1")
	if e := apperr.As(err); e == nil || e.Hint != "keys: short, week, enabled" {
		t.Errorf("hint: %v", err)
	}
}

// TestConfigSetKeepsBytes: a set changes the one value and nothing else,
// byte for byte (comments, spacing, key order).
func TestConfigSetKeepsBytes(t *testing.T) {
	a, home := fixtureApp(t, setConfig, nil, nil)
	if _, err := a.ConfigSet(context.Background(), "logins.auto.pool.accounts.alice.short", "90"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(configPath(home))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(setConfig, `"alice": { "short": 95,`, `"alice": { "short": 90,`, 1)
	if string(b) != want {
		t.Errorf("file:\n%s\nwant:\n%s", b, want)
	}
}

// TestConfigSetRedactsSecrets: a token or env value is written but never
// echoed back.
func TestConfigSetRedactsSecrets(t *testing.T) {
	a, _ := fixtureApp(t, setConfig, nil, nil)
	res, err := a.ConfigSet(context.Background(), "profiles.personal.env.SECRET_X", "fake-secret-1")
	if err != nil || res.Value != redactedText || res.Type != "string" {
		t.Errorf("env: %+v %v", res, err)
	}
}

// TestSetConfigKey is the picker's "remember": the same typing, no result.
func TestSetConfigKey(t *testing.T) {
	a, home := fixtureApp(t, setConfig, nil, nil)
	if err := a.SetConfigKey(context.Background(), "default_agent", "codex"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetConfigKey(context.Background(), "version", "x"); err == nil {
		t.Error("a bad value was accepted")
	}
	b, _ := os.ReadFile(configPath(home))
	if !strings.Contains(string(b), `"default_agent": "codex"`) {
		t.Errorf("file:\n%s", b)
	}
}

// TestScalarTypeWording covers the range wordings no config key uses
// today (a lower or upper bound alone).
func TestScalarTypeWording(t *testing.T) {
	for _, tc := range []struct {
		t    scalarType
		want string
	}{
		{scalarType{types: []string{"number"}, minimum: "1"}, "a number of at least 1"},
		{scalarType{types: []string{"integer"}, maximum: "9"}, "an integer of at most 9"},
		{scalarType{types: []string{"string", "null"}}, "a string or null"},
	} {
		if got := tc.t.expected(); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.t, got, tc.want)
		}
	}
	lo := scalarType{types: []string{"integer"}, minimum: "1"}
	if _, _, ok := lo.parse("0"); ok {
		t.Error("0 accepted below a minimum of 1")
	}
	if got := kindPhrase("boolean"); got != "a boolean" {
		t.Errorf("kindPhrase: %s", got)
	}
}

// TestKindWords covers the kind wordings of the key walk.
func TestKindWords(t *testing.T) {
	if got := kindPhrase("integer"); got != "an integer" {
		t.Errorf("integer: %s", got)
	}
	if got := valueKind(nil, &schema.Schema{}); got != "object" {
		t.Errorf("untyped: %s", got)
	}
	untyped := scalarTypeOf(nil, &schema.Schema{Properties: map[string]*schema.Schema{}})
	if untyped.container != "object" {
		t.Errorf("properties without a type: %+v", untyped)
	}
}
