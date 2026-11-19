package config_test

import (
	"strings"
	"testing"

	"github.com/ubgo/agx/config"
)

// TestPortConfigValidation is the stand-alone selector's describe("config
// validation"), on agx's pool block: each case refused at load with its
// position. Two deliberate differences (pool/testdata/SELECTOR_PORT.md):
// an accounts entry for an unlisted name is an error, not a warning, and
// own/shared may each be omitted as long as the pool has a member.
func TestPortConfigValidation(t *testing.T) {
	pool := func(body string) string {
		return `"auto": { "agents": ["claude"], "kind": "pool", "pool": { ` + body + ` } }`
	}
	cases := []struct {
		name, pool string
		want       []string
	}{
		{"missing own/shared arrays", pool(`"force": null`), []string{"the pool has no members"}},
		{"duplicate names across own and shared", pool(`"own": ["alice"], "shared": ["alice"]`), []string{"listed more than once"}},
		{"old setting names are rejected so nothing is silently ignored", pool(`"own": ["alice"], "burnToFullest": [], "priority": [], "accountLimits": {}, "limits": {}, "sharedBorrowWindowHours": 48`),
			[]string{"burnToFullest", "priority", "accountLimits", "limits", "sharedBorrowWindowHours"}},
		{"force must be a string or null", pool(`"own": ["alice"], "force": true`), []string{"force"}},
		{"bad borrow window", pool(`"own": ["alice"], "shared_borrow_window": "-1h"`), []string{`shared_borrow_window "-1h" is negative`}},
		{"bad borrow window (a number)", pool(`"own": ["alice"], "shared_borrow_window": 48`), []string{"shared_borrow_window"}},
		{"accounts entry for an unlisted name is an error", pool(`"own": ["alice"], "accounts": { "zed": {} }`), []string{"accounts.zed is not listed in own or shared"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePool(t, tc.pool)
			if err == nil {
				t.Fatal("accepted")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) && !strings.Contains(strings.Join(problemMessages(err), "\n"), w) {
					t.Errorf("err %v\nmissing %q", err, w)
				}
			}
		})
	}
}

// problemMessages lists every problem of a config error.
func problemMessages(err error) []string {
	var out []string
	for _, p := range config.ProblemsOf(err) {
		out = append(out, p.String())
	}
	return out
}

// TestPortStrictConfig is the selector's describe("no silent surprises:
// bad config is an error, never ignored"), on the pool block: each case
// is refused at load (schema or cross-check), never ignored.
func TestPortStrictConfig(t *testing.T) {
	pool := func(body string) string {
		return `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "shared": ["carol"]` + body + ` } }`
	}
	refused := func(t *testing.T, body string, want ...string) {
		t.Helper()
		_, err := parsePool(t, pool(body))
		if err == nil {
			t.Fatalf("accepted: %s", body)
		}
		all := err.Error() + "\n" + strings.Join(problemMessages(err), "\n")
		for _, w := range want {
			if !strings.Contains(all, w) {
				t.Errorf("%s: %v\nmissing %q", body, all, w)
			}
		}
	}
	t.Run("unknown top-level setting (typo) is an error", func(t *testing.T) {
		refused(t, `, "shared_borrow_window_hour": "24h"`, "shared_borrow_window_hour")
		refused(t, `, "Force": "alice"`, "Force")
	})
	t.Run("unknown key inside accounts or defaults is an error", func(t *testing.T) {
		refused(t, `, "accounts": { "alice": { "weekly": 70 } }`, "weekly")
		refused(t, `, "accounts": { "alice": { "day": 10 } }`, "day")
		refused(t, `, "defaults": { "enabled": false }`, "enabled")
	})
	t.Run("enabled must be a real boolean", func(t *testing.T) {
		refused(t, `, "accounts": { "alice": { "enabled": "false" } }`, "enabled")
		refused(t, `, "accounts": { "alice": { "enabled": 0 } }`, "enabled")
		refused(t, `, "accounts": { "alice": { "enabled": null } }`, "accounts.alice.enabled: must be boolean, got null")
	})
	t.Run("caps must be numbers from 0 to 100", func(t *testing.T) {
		for _, v := range []string{`"50"`, `"abc"`, `null`, `-1`, `101`, `1e400`} {
			refused(t, `, "accounts": { "alice": { "week": `+v+` } }`, "week")
			refused(t, `, "defaults": { "short": `+v+` }`, "short")
		}
	})
	t.Run("other settings are type-checked", func(t *testing.T) {
		refused(t, `, "short_unavailable_at_percent": 120`, "short_unavailable_at_percent")
		refused(t, `, "short_unavailable_at_percent": "99"`, "short_unavailable_at_percent")
		refused(t, `, "short_unavailable_at_percent": null`, "short_unavailable_at_percent: must be number, got null")
		refused(t, `, "shared_borrow_window": 48`, "shared_borrow_window")
		refused(t, `, "force": ""`, "force is empty")
		refused(t, `, "accounts": []`, "accounts")
		refused(t, `, "accounts": { "alice": 70 }`, "alice")
		refused(t, `, "defaults": []`, "defaults")
		if _, err := parsePool(t, `"auto": { "agents": [""], "kind": "pool", "pool": { "own": ["alice"] } }`); err == nil {
			t.Error(`agents [""] accepted`)
		}
	})
	t.Run("a valid full config passes", func(t *testing.T) {
		if _, err := parsePool(t, pool(`, "force": null, "accounts": { "alice": { "short": 0, "week": 100, "enabled": true } }, "defaults": { "short": 50, "week": 70 }, "shared_borrow_window": "48h", "short_unavailable_at_percent": 99, "usage_max_age": "60s", "key_ttl": "60s", "picker": { "command": "true", "timeout": "5s" }, "log": "~/p.log"`)); err != nil {
			t.Fatal(err)
		}
	})
}
