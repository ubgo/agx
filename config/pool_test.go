package config_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
)

// poolKnown is known plus the pool rule of the real registry's shape:
// only claude can run a pool.
var poolKnown = config.Known{Agents: known.Agents, PoolAgent: func(id agentio.ID) string {
	if id == "claude" {
		return ""
	}
	return "no live credential"
}}

// poolDoc is a config with members alice and bob (own), carol and derek
// (shared), a helper and a codex login, and the pool block given.
func poolDoc(pool string) string {
	return `{
  "version": 1,
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${A}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${B}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "stored": true },
    "derek": { "agents": ["claude"], "kind": "api-key", "token": "${D}" },
    "helper": { "agents": ["claude"], "kind": "api-key-helper", "command": "gopass show -o x" },
    "cx": { "agents": ["codex"], "kind": "api-key", "token": "${C}" },
    ` + pool + `
  }
}`
}

// parsePool validates a pool config with poolKnown.
func parsePool(t *testing.T, pool string) (*config.Loaded, error) {
	t.Helper()
	return config.Parse([]byte(poolDoc(pool)), config.Paths{File: filepath.Join(t.TempDir(), "config.jsonc")}, poolKnown)
}

// TestPoolLoads: the documented block loads, null force included, and
// Rules fills every default.
func TestPoolLoads(t *testing.T) {
	l, err := parsePool(t, `"auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob"], "shared": ["carol", "derek"], "force": null,
      "shared_borrow_window": "24h", "defaults": { "week": 60 },
      "accounts": { "alice": { "short": 95, "week": 98 }, "derek": { "enabled": false } },
      "key_ttl": "30s", "log": "~/pool.log" } }`)
	if err != nil {
		t.Fatal(err)
	}
	p := l.Config.Logins["auto"].Pool
	r, err := p.Rules()
	if err != nil {
		t.Fatal(err)
	}
	if r.BorrowWindow != 24*time.Hour || r.Defaults.Short != 50 || r.Defaults.Week != 60 || r.ShortUnavailableAt != 99 || r.Force != "" {
		t.Errorf("rules %+v", r)
	}
	if r.Enabled("derek") || r.CapsFor("alice").Week != 98 {
		t.Errorf("accounts %+v", r.Accounts)
	}
	if ttl, _ := p.TTL(); ttl != 30*time.Second {
		t.Errorf("ttl %s", ttl)
	}
	if age, _ := p.MaxAge(); age != config.DefaultUsageMaxAge {
		t.Errorf("max age %s", age)
	}
}

// TestPoolProblems: every rule of a pool block is checked at load, with
// the position, and all problems are reported.
func TestPoolProblems(t *testing.T) {
	cases := []struct {
		name, pool, want string
	}{
		{"unknown member", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["zoe"] } }`, "unknown login zoe"},
		{"twice", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "shared": ["alice"] } }`, "member alice is listed more than once"},
		{"helper member", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["helper"] } }`, "member helper is an api-key-helper"},
		{"pool member", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["p2"] } }, "p2": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"] } }`, "member p2 is a pool"},
		{"other agent's member", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["cx"] } }`, "member cx serves codex, not claude"},
		{"codex pool", `"auto": { "agents": ["codex"], "kind": "pool", "pool": { "own": ["cx"] } }`, "codex cannot use a login pool (no live credential)"},
		{"two agents", `"auto": { "agents": ["claude", "codex"], "kind": "pool", "pool": { "own": ["alice"] } }`, "a pool login serves exactly one agent"},
		{"no members", `"auto": { "agents": ["claude"], "kind": "pool", "pool": {} }`, "the pool has no members"},
		{"no block", `"auto": { "agents": ["claude"], "kind": "pool" }`, "a pool login needs pool"},
		{"value", `"auto": { "agents": ["claude"], "kind": "pool", "token": "x", "pool": { "own": ["alice"] } }`, "a pool login holds no token"},
		{"block on token login", `"auto": { "agents": ["claude"], "kind": "oauth-token", "token": "x", "pool": { "own": ["alice"] } }`, "pool is only for kind pool logins"},
		{"bad window", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "shared_borrow_window": "2 days" } }`, `shared_borrow_window "2 days" is not a duration`},
		{"short ttl", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "key_ttl": "10ms" } }`, "key_ttl 10ms is below 1s"},
		{"cap", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "defaults": { "short": 150 } } }`, "defaults.short 150 is outside 0..100"},
		{"unlisted account", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "accounts": { "bob": {} } } }`, "accounts.bob is not listed in own or shared"},
		{"unknown force", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "force": "zoe" } }`, "force: unknown login zoe"},
		{"old selector key", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "burnToFullest": ["alice"] } }`, "burnToFullest"},
		{"unknown key", `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "accounts": { "alice": { "daily": 5 } } } }`, "daily"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePool(t, tc.pool)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v\nwant %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "config.jsonc:") {
				t.Errorf("no position in %v", err)
			}
		})
	}
}

// TestPoolMembersOrder: own then shared, in config order.
func TestPoolMembersOrder(t *testing.T) {
	p := config.Pool{Own: []string{"bob", "alice"}, Shared: []string{"derek"}}
	if got := strings.Join(p.Members(), ","); got != "bob,alice,derek" {
		t.Errorf("members %s", got)
	}
}
