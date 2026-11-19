package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// agx pool list and agx pool show through the real binary: config only,
// no probe and no cache. The snapshot's pool (auto) sets only its
// members and three own caps, so most values are defaults; the second
// pool (team) sets every key, so every source shows.

// teamPool is a pool login that sets every pool key.
const teamPool = `"team": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice"], "shared": ["bob", "carol"], "force": "bob",
      "shared_borrow_window": "24h", "short_unavailable_at_percent": 95,
      "defaults": { "short": 40, "week": 60 },
      "accounts": { "alice": { "short": 98 }, "carol": { "enabled": false } },
      "usage_max_age": "2m", "key_ttl": "30s", "log": "~/pool.log",
      "picker": { "command": "my-picker", "timeout": "5s" } } }`

// poolFixture is the snapshot machine with the team pool added.
func poolFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := pickFixture(t)
	f.WriteConfig(strings.Replace(pickConfig, `"auto": {`, teamPool+`,
    "auto": {`, 1))
	return f
}

// TestPoolListShowGoldens pins pool list (table and --json) and pool
// show (plain, coloured, --json) for a pool of defaults and one that
// sets every key.
func TestPoolListShowGoldens(t *testing.T) {
	f := poolFixture(t)
	for _, c := range []struct {
		golden string
		args   []string
	}{
		{"pool-list", []string{"pool", "list", "--color", "never"}},
		{"pool-show-plain", []string{"pool", "show", "--id", "auto", "--color", "never"}},
		{"pool-show-team", []string{"pool", "show", "--id", "team", "--color", "never"}},
		{"pool-show-color", []string{"pool", "show", "--id", "team", "--color", "always"}},
	} {
		t.Run(c.golden, func(t *testing.T) {
			r := f.Run(c.args...)
			if r.Code != 0 || r.Stderr != "" {
				t.Fatalf("exit %d: %s", r.Code, r.Stderr)
			}
			testkit.GoldenText(t, c.golden, r.Stdout)
		})
	}
	for golden, args := range map[string][]string{
		"pool-list-json":      {"pool", "list", "--json"},
		"pool-show-team-json": {"pool", "show", "--id", "team", "--json"},
	} {
		r := f.Run(args...)
		if r.Code != 0 {
			t.Fatalf("%s exit %d: %s", golden, r.Code, r.Stderr)
		}
		f.Golden(t, golden, r.Stdout)
	}
}

// TestPoolShowSources: each value names where it comes from: config for
// a key the pool sets, defaults for a member cap from the pool's
// defaults key, default for agx's value of an omitted key.
func TestPoolShowSources(t *testing.T) {
	f := poolFixture(t)
	var env struct {
		Data struct {
			Force   string
			Picker  string
			Members []struct {
				Name          string
				ShortSource   string `json:"short_source"`
				WeekSource    string `json:"week_source"`
				EnabledSource string `json:"enabled_source"`
			}
			Settings []struct{ Key, Value, Source string }
		}
	}
	read := func(id string) {
		t.Helper()
		r := f.Run("pool", "show", "--id", id, "--json")
		if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
			t.Fatalf("%s: %v\n%s", id, err, r.Stdout)
		}
	}
	setting := func(key string) string {
		for _, s := range env.Data.Settings {
			if s.Key == key {
				return s.Value + " (" + s.Source + ")"
			}
		}
		return "missing"
	}
	read("auto")
	for key, want := range map[string]string{
		"force": "off (default)", "defaults.short": "50 (default)", "defaults.week": "70 (default)",
		"shared_borrow_window": "48h (default)", "short_unavailable_at_percent": "99 (default)",
		"usage_max_age": "60s (default)", "key_ttl": "60s (default)", "picker.command": "built-in rules (default)",
		"picker.timeout": "10s (default)", "log": "none (default)",
	} {
		if got := setting(key); got != want {
			t.Errorf("auto %s: %q, want %q", key, got, want)
		}
	}
	read("team")
	for key, want := range map[string]string{
		"force": "bob (config)", "defaults.short": "40 (config)", "shared_borrow_window": "24h (config)",
		"short_unavailable_at_percent": "95 (config)", "usage_max_age": "2m (config)", "key_ttl": "30s (config)",
		"picker.command": "my-picker (config)", "picker.timeout": "5s (config)", "log": "~/pool.log (config)",
	} {
		if got := setting(key); got != want {
			t.Errorf("team %s: %q, want %q", key, got, want)
		}
	}
	m := env.Data.Members
	if len(m) != 3 || m[0].Name != "alice" || m[0].ShortSource != "config" || m[0].WeekSource != "defaults" ||
		m[2].Name != "carol" || m[2].EnabledSource != "config" || m[1].EnabledSource != "default" {
		t.Errorf("members: %+v", m)
	}
}

// TestPoolCommandsRefuse: a non-pool login is refused with the pool
// commands' wording, an unknown id is not-found, a missing --id is a
// usage error, and no pools configured is an empty list with a hint.
func TestPoolCommandsRefuse(t *testing.T) {
	f := poolFixture(t)
	for _, sub := range []string{"show", "pick"} {
		r := f.Run("pool", sub, "--id", "alice", "--cached")
		if sub == "show" {
			r = f.Run("pool", sub, "--id", "alice")
		}
		if r.Code != 2 || !strings.Contains(r.Stderr, "pool "+sub+" is for pool logins; login alice is oauth-token. Pools serve claude today") ||
			!strings.Contains(r.Stderr, "agx pool list") {
			t.Errorf("%s on a token login: %d %s", sub, r.Code, r.Stderr)
		}
		if r := f.Run("pool", sub, "--id", "zoe"); r.Code != 1 || !strings.Contains(r.Stderr, "unknown login zoe") {
			t.Errorf("%s unknown id: %d %s", sub, r.Code, r.Stderr)
		}
		if r := f.Run("pool", sub); r.Code != 2 || !strings.Contains(r.Stderr, "--id is required") {
			t.Errorf("%s without --id: %d %s", sub, r.Code, r.Stderr)
		}
	}
	f.WriteConfig(`{"version": 1, "logins": {"alice": {"agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}"}}}`)
	r := f.Run("pool", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "no pool logins") || !strings.Contains(r.Stdout, "agx login add") {
		t.Errorf("no pools: %d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	if r := f.Run("pool", "list", "--json", "--field", "data"); strings.TrimSpace(r.Stdout) != "[]" {
		t.Errorf("no pools --json: %q", r.Stdout)
	}
	if r := f.Run("pool", "show", "--id", "auto"); r.Code != 1 || !strings.Contains(r.Stderr, "unknown login auto") {
		t.Errorf("no pools show: %d %s", r.Code, r.Stderr)
	}
}

// TestLoginPickRemoved: agx login pick is gone (no alias): an unknown
// command, exit 2, whose hint names agx pool pick, with flags too.
func TestLoginPickRemoved(t *testing.T) {
	f := pickFixture(t)
	for _, args := range [][]string{
		{"login", "pick"},
		{"login", "pick", "--id", "auto"},
		{"--color", "never", "login", "--json", "pick", "--id", "auto"},
	} {
		r := f.Run(args...)
		if r.Code != 2 || r.Stdout != "" && !strings.Contains(r.Stdout, `"hint": "agx pool pick"`) {
			t.Errorf("%v: exit %d %q", args, r.Code, r.Stdout)
		}
		if out := r.Stdout + r.Stderr; !strings.Contains(out, "agx pool pick") || strings.Contains(out, "Pool: auto") {
			t.Errorf("%v: no hint or a pick ran:\n%s", args, out)
		}
	}
	if r := f.Run("login", "bogus"); r.Code != 2 || strings.Contains(r.Stderr, "pool") {
		t.Errorf("other unknown command: %d %s", r.Code, r.Stderr)
	}
	if r := f.Run("login", "--help"); strings.Contains(r.Stdout, "  pick ") {
		t.Errorf("login help still names pick:\n%s", r.Stdout)
	}
}
