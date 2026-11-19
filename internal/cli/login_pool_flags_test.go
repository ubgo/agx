package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// flagsConfig is a pool with one member entry and a login outside it.
const flagsConfig = `{
  "version": 1,
  // pool flags through the binary
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob"], "shared": ["carol"],
      "accounts": { "alice": { "short": 95, "week": 98 } } } }
  }
}`

// everyFlag sets every pool setting flag once.
var everyFlag = []string{
	"--pool-cap-short", "bob=90", "--pool-cap-week", "bob=97.5", "--pool-disable", "carol",
	"--pool-default-short", "60", "--pool-default-week", "80", "--pool-short-cutoff", "98",
	"--pool-borrow-window", "24h", "--pool-usage-max-age", "2m", "--pool-key-ttl", "30s",
	"--pool-picker", "my-picker",
}

// TestLoginPoolFlagGoldens pins the before/after summaries of login edit
// and add on a pool: a change, the same change as --dry-run, clearing
// every key again, flags that match the file, and a new pool.
func TestLoginPoolFlagGoldens(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(flagsConfig)
	cfgPath := filepath.Join(f.ConfigDir, "config.jsonc")
	run := func(golden string, args ...string) {
		t.Helper()
		r := f.Run(args...)
		if r.Code != 0 {
			t.Fatalf("%s: exit %d: %s", golden, r.Code, r.Stderr)
		}
		testkit.GoldenText(t, golden, r.Stdout)
	}
	before := f.ReadFile(cfgPath)
	run("login-pool-edit-dry-run", append([]string{"login", "edit", "--id", "auto", "--dry-run"}, everyFlag...)...)
	if f.ReadFile(cfgPath) != before {
		t.Fatal("--dry-run wrote the file")
	}
	run("login-pool-edit", append([]string{"login", "edit", "--id", "auto"}, everyFlag...)...)
	wantCode(t, f.Run("config", "check"), 0)
	run("login-pool-no-change", "login", "edit", "--id", "auto", "--pool-cap-short", "alice=95", "--pool-cap-week", "alice=98", "--dry-run")
	run("login-pool-clear", "login", "edit", "--id", "auto",
		"--pool-cap-short", "bob=", "--pool-cap-week", "bob=", "--pool-enable", "carol",
		"--pool-default-short", "", "--pool-default-week", "", "--pool-short-cutoff", "",
		"--pool-borrow-window", "", "--pool-usage-max-age", "", "--pool-key-ttl", "", "--pool-picker", "")
	if got := f.ReadFile(cfgPath); got != before {
		t.Errorf("clearing every key did not restore the file:\n%s", got)
	}
	run("login-pool-add-dry-run", "login", "add", "--name", "team", "--agent", "claude", "--kind", "pool",
		"--pool-own", "alice", "--pool-shared", "carol", "--pool-cap-short", "alice=95", "--pool-cap-week", "alice=98", "--dry-run")
	run("login-pool-add", "login", "add", "--name", "team", "--agent", "claude", "--kind", "pool",
		"--pool-own", "alice", "--pool-shared", "carol", "--pool-cap-short", "alice=95", "--pool-cap-week", "alice=98")
	wantCode(t, f.Run("config", "check"), 0)
}

// TestLoginPoolFlagsJSON: --json carries the changes and whether the file
// was written.
func TestLoginPoolFlagsJSON(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(flagsConfig)
	r := f.Run("login", "edit", "--id", "auto", "--pool-cap-short", "bob=50", "--dry-run", "--json")
	wantCode(t, r, 0)
	for _, w := range []string{`"key": "pool.accounts.bob.short"`, `"to": "50"`, `"written": false`} {
		if !strings.Contains(r.Stdout, w) {
			t.Errorf("json lacks %s:\n%s", w, r.Stdout)
		}
	}
	r = f.Run("login", "edit", "--id", "auto", "--pool-cap-short", "bob=50", "--json")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, `"written": true`) {
		t.Errorf("json:\n%s", r.Stdout)
	}
}

// TestLoginPoolFlagsRefusedByTheBinary: a bad value exits 2 with the
// flag's message and leaves the file as it was.
func TestLoginPoolFlagsRefusedByTheBinary(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(flagsConfig)
	cfgPath := filepath.Join(f.ConfigDir, "config.jsonc")
	before := f.ReadFile(cfgPath)
	wantErr(t, f.Run("login", "edit", "--id", "auto", "--pool-cap-short", "alice=101", "--json"), 2, "usage", "--pool-cap-short alice=101: 101 is not a number from 0 to 100")
	wantErr(t, f.Run("login", "edit", "--id", "auto", "--pool-disable", "zed", "--json"), 2, "usage", "zed is not a member of pool auto")
	wantErr(t, f.Run("login", "edit", "--id", "auto", "--pool-key-ttl", "soon", "--json"), 2, "usage", `--pool-key-ttl "soon" is not a duration`)
	wantErr(t, f.Run("login", "edit", "--id", "alice", "--account", "x", "--dry-run", "--json"), 2, "usage", "--dry-run is for pool logins")
	wantErr(t, f.Run("config", "set", "--key", "logins.auto.pool.accounts.bob.short", "--value", "101", "--json"), 2, "usage", `--key logins.auto.pool.accounts.bob.short takes a number from 0 to 100, not "101"`)
	if f.ReadFile(cfgPath) != before {
		t.Error("a refused command wrote the file")
	}
	// The reported bug: a number on a member entry that does not exist yet.
	r := f.Run("config", "set", "--key", "logins.auto.pool.accounts.bob.short", "--value", "95")
	wantCode(t, r, 0)
	if r.Stdout != "logins.auto.pool.accounts.bob.short = 95\n" {
		t.Errorf("config set: %q", r.Stdout)
	}
	wantCode(t, f.Run("config", "check"), 0)
}
