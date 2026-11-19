package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/testkit"
)

// agx login pick through the real binary on the anonymised real snapshot
// (pool/testdata), seeded as a login-usage cache 20 seconds old at the
// pinned clock, so no command probes. Goldens pin the selector's report
// line for line, plain and coloured, forced, and with every time option.

// snapNow is the snapshot's clock (2026-11-18 23:40 IST).
const snapNow = "2026-11-18T18:10:00Z"

// snapshotPath is the anonymised fixture, from this package's folder.
const snapshotPath = "../../pool/testdata/snapshot-2026-11-18-anon.json"

// pickConfig is the snapshot's pool with fake names; own caps 99/100.
const pickConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}", "account": "alice@example.com" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "${DEREK_TOKEN}" },
    "erin": { "agents": ["claude"], "kind": "oauth-token", "token": "${ERIN_TOKEN}" },
    "frank": { "agents": ["claude"], "kind": "oauth-token", "token": "${FRANK_TOKEN}" },
    "grace": { "agents": ["claude"], "kind": "oauth-token", "token": "${GRACE_TOKEN}" },
    "heidi": { "agents": ["claude"], "kind": "oauth-token", "token": "${HEIDI_TOKEN}" },
    "ivan": { "agents": ["claude"], "kind": "oauth-token", "token": "${IVAN_TOKEN}" },
    "judy": { "agents": ["claude"], "kind": "oauth-token", "token": "${JUDY_TOKEN}" },
    "mallory": { "agents": ["claude"], "kind": "oauth-token", "token": "${MALLORY_TOKEN}" },
    "niaj": { "agents": ["claude"], "kind": "oauth-token", "token": "${NIAJ_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob", "carol"],
      "shared": ["derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "niaj"],
      "accounts": { "alice": { "short": 99, "week": 100 }, "bob": { "short": 99, "week": 100 }, "carol": { "short": 99, "week": 100 } } } }
  }
}`

// snapshotCache turns the anonymised agx usage JSON into the login-usage
// cache, probed 20 seconds before snapNow.
func snapshotCache(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Data []struct {
			Name, Status, Account string
			Windows               []map[string]any
		}
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, snapNow)
	logins := map[string]any{}
	for _, d := range doc.Data {
		var ws []map[string]any
		for _, w := range d.Windows {
			ws = append(ws, map[string]any{"class": w["class"], "percent": w["percent"], "resets_at": w["resets_at"]})
		}
		logins[d.Name] = map[string]any{"agent": "claude", "account": d.Account, "probed_at": now.Add(-20 * time.Second).Format(time.RFC3339), "status": d.Status, "windows": ws}
	}
	out, _ := json.MarshalIndent(map[string]any{"version": 1, "logins": logins}, "", "  ")
	return string(out)
}

// pickFixture is the snapshot machine.
func pickFixture(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	f.WriteConfig(pickConfig)
	f.WriteFile(f.ConfigDir+"/cache/login-usage.jsonc", snapshotCache(t), 0o600)
	f.Env[testkit.EnvTestNow] = snapNow
	return f
}

// TestLoginPickGoldens pins the report: plain, coloured, forced (with a
// warning line per ignored rule), --time-format both with a custom
// --date-format and --tz, --duration-format round, and --json.
func TestLoginPickGoldens(t *testing.T) {
	f := pickFixture(t)
	for _, c := range []struct {
		golden string
		args   []string
	}{
		{"login-pick-plain", []string{"--color", "never"}},
		{"login-pick-color", []string{"--color", "always"}},
		{"login-pick-forced", []string{"--color", "never", "--pool-force", "bob"}},
		{"login-pick-both", []string{"--color", "never", "--time-format", "both", "--date-format", "ddd D MMM HH:mm", "--tz", "Asia/Kolkata"}},
		{"login-pick-day", []string{"--color", "never", "--time-format", "day", "--tz", "UTC"}},
		{"login-pick-round", []string{"--color", "never", "--duration-format", "round"}},
	} {
		t.Run(c.golden, func(t *testing.T) {
			r := f.Run(append([]string{"login", "pick", "--id", "auto"}, c.args...)...)
			if r.Code != 0 {
				t.Fatalf("exit %d: %s", r.Code, r.Stderr)
			}
			testkit.GoldenText(t, c.golden, r.Stdout)
		})
	}
	r := f.Run("login", "pick", "--id", "auto", "--json")
	if r.Code != 0 {
		t.Fatalf("json exit %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "login-pick-json", r.Stdout)
}

// TestLoginPickPlainShape: the selector's lines in its order, checked
// here as well as in the golden so the contract reads in the test.
func TestLoginPickPlainShape(t *testing.T) {
	f := pickFixture(t)
	r := f.Run("login", "pick", "--id", "auto")
	lines := strings.Split(r.Stdout, "\n")
	want := []string{
		"Pool: auto (claude)",
		"Use: grace (shared)",
		"shared account resets within 48h, before any usable own account; using its room before it is lost",
		"Source: cache (20s old)",
		"",
		"Candidates (ready accounts in pick order):",
	}
	for i, w := range want {
		if i >= len(lines) || lines[i] != w {
			t.Fatalf("line %d: %q, want %q\n%s", i+1, lines[i], w, r.Stdout)
		}
	}
	if !strings.HasPrefix(lines[6], "- grace [shared] #1 READY (5h 0%, week 26%, resets in ") || !strings.HasSuffix(lines[6], ", room 44%)") {
		t.Errorf("first candidate %q", lines[6])
	}
	if !strings.Contains(r.Stdout, "- bob [own] skip: status is limit-reached; week is 100% (cap 100%) (5h 0%, week 100%, resets in ") {
		t.Errorf("bob line missing:\n%s", r.Stdout)
	}
}

// TestLoginPickFlags: --field reads the envelope; --now reads an old
// cache as of that time (the snapshot is then stale for every reset
// before it); a bad --now, a negative --max-age and an unknown
// --duration-format are usage errors; nothing usable exits 1 with the
// report and an error line; a token login is refused.
func TestLoginPickFlags(t *testing.T) {
	f := pickFixture(t)
	if r := f.Run("login", "pick", "--id", "auto", "--json", "--field", "data.selected.name"); strings.TrimSpace(r.Stdout) != "grace" {
		t.Errorf("field: %q %s", r.Stdout, r.Stderr)
	}
	if r := f.Run("login", "pick", "--id", "auto", "--cached", "--now", "2026-11-20T09:00:00Z"); r.Code != 0 || !strings.Contains(r.Stdout, "reset passed") {
		t.Errorf("--now: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	for _, args := range [][]string{
		{"--now", "yesterday"}, {"--max-age", "-5s"}, {"--duration-format", "hours"}, {"--time-format", "clock"},
	} {
		if r := f.Run(append([]string{"login", "pick", "--id", "auto", "--cached"}, args...)...); r.Code != 2 {
			t.Errorf("%v: exit %d %s", args, r.Code, r.Stderr)
		}
	}
	if r := f.Run("login", "pick", "--id", "alice"); r.Code != 2 || !strings.Contains(r.Stderr, "login pick is for pool logins; login alice is oauth-token. Pools serve claude today") {
		t.Errorf("token login: %d %s", r.Code, r.Stderr)
	}
	if r := f.Run("login", "pick", "--id", "auto", "--cached", "--pool-force", "zoe"); r.Code != 1 || !strings.Contains(r.Stderr, "unknown login zoe") {
		t.Errorf("unknown force: %d %s", r.Code, r.Stderr)
	}
}

// TestLoginPickNoneUsable: with only blocked members, exit 1, the
// selector's "No configured account" line, the error on stderr.
func TestLoginPickNoneUsable(t *testing.T) {
	f := pickFixture(t)
	cfg := strings.Replace(pickConfig, `"own": ["alice", "bob", "carol"],`, `"own": ["bob"],`, 1)
	cfg = strings.Replace(cfg, `"shared": ["derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "niaj"],`, `"shared": ["niaj"],`, 1)
	cfg = strings.Replace(cfg, `"accounts": { "alice": { "short": 99, "week": 100 }, "bob": { "short": 99, "week": 100 }, "carol": { "short": 99, "week": 100 } }`, `"accounts": { "bob": { "short": 99, "week": 100 } }`, 1)
	f.WriteConfig(cfg)
	r := f.Run("login", "pick", "--id", "auto")
	if r.Code != 1 || !strings.HasPrefix(r.Stdout, "Pool: auto (claude)\nNo configured account is currently usable.\n") || !strings.Contains(r.Stderr, "pool auto: no member is usable") {
		t.Errorf("exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestLoginTokenOnPool: agx login token on a pool prints the picked
// member's token alone (stderr empty); --pool-force prints that member's
// with a warning line per broken rule; nothing usable is exit 1 with
// stdout empty (the selector's "token" command, ported).
func TestLoginTokenOnPool(t *testing.T) {
	f := pickFixture(t)
	f.Env["GRACE_TOKEN"], f.Env["BOB_TOKEN"] = "fake-token-GRACE", "fake-token-BOB"
	r := f.Run("login", "token", "--id", "auto")
	if r.Code != 0 || r.Stdout != "fake-token-GRACE\n" || r.Stderr != "" {
		t.Errorf("pick: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	r = f.Run("login", "token", "--id", "auto", "--pool-force", "bob")
	if r.Code != 0 || r.Stdout != "fake-token-BOB\n" || !strings.Contains(r.Stderr, "forced bob: status is limit-reached") {
		t.Errorf("force: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	if r := f.Run("login", "token", "--id", "bob", "--pool-force", "carol"); r.Code != 2 || r.Stdout != "" {
		t.Errorf("force on a token login: %d %q", r.Code, r.Stderr)
	}
	cfg := strings.Replace(pickConfig, `"own": ["alice", "bob", "carol"],`, `"own": ["bob"],`, 1)
	cfg = strings.Replace(cfg, `"shared": ["derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "niaj"],`, `"shared": ["niaj"],`, 1)
	cfg = strings.Replace(cfg, `"accounts": { "alice": { "short": 99, "week": 100 }, "bob": { "short": 99, "week": 100 }, "carol": { "short": 99, "week": 100 } }`, `"accounts": { "bob": { "short": 99, "week": 100 } }`, 1)
	f.WriteConfig(cfg)
	r = f.Run("login", "token", "--id", "auto")
	if r.Code != 1 || r.Stdout != "" || strings.Count(strings.TrimSpace(r.Stderr), "\n") != 0 || !strings.Contains(r.Stderr, "no member is usable") {
		t.Errorf("none usable: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
}

// TestLoginPickCacheFollowsConfigDir: the login-usage cache lives in the
// config dir of the config in use (--config), never the current folder
// or another config dir (the selector's "config.json and .cache.json live
// next to the script, not in cwd").
func TestLoginPickCacheFollowsConfigDir(t *testing.T) {
	f := pickFixture(t)
	other := f.Root + "/other"
	f.WriteFile(other+"/config.jsonc", pickConfig, 0o644)
	f.WriteFile(other+"/cache/login-usage.jsonc", snapshotCache(t), 0o600)
	if err := os.Remove(f.ConfigDir + "/cache/login-usage.jsonc"); err != nil {
		t.Fatal(err)
	}
	r := f.Run("--config", other+"/config.jsonc", "login", "pick", "--id", "auto", "--cached")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\nUse: grace (shared)") {
		t.Fatalf("other config dir: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	if r := f.Run("login", "pick", "--id", "auto", "--cached"); r.Code != 1 || !strings.Contains(r.Stdout, "no usage reading") {
		t.Errorf("default config dir has no cache: %d %s", r.Code, r.Stdout)
	}
	if _, err := os.Stat(f.Work + "/cache"); err == nil {
		t.Error("a cache was written in the current folder")
	}
}
