package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// --pool-only through the real binary on the snapshot machine (cached
// usage, no probe): pool pick restricted to the pin, the dry run's pin
// and caps with their sources, the refusals, and whoami / live show of a
// pinned run. In the snapshot grace is usable (shared, week 26%), bob is
// at its limit, and ivan's week is 43% (cap 70%).

// TestPoolOnlyPickGoldens: the normal report restricted to the pin,
// plain and --json; a blocked pin prints the "none usable" line with its
// skip reasons and exits 1 with one stderr line.
func TestPoolOnlyPickGoldens(t *testing.T) {
	f := pickFixture(t)
	r := f.Run("pool", "pick", "--id", "auto", "--pool-only", "ivan", "--color", "never")
	if r.Code != 0 {
		t.Fatalf("exit %d: %s", r.Code, r.Stderr)
	}
	testkit.GoldenText(t, "pool-pick-only", r.Stdout)
	for _, want := range []string{"Use: ivan (shared)", "the only member of this run (--pool-only); its caps and the pool's rules still apply"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.Stdout)
		}
	}
	r = f.Run("pool", "pick", "--id", "auto", "--pool-only", "ivan", "--json")
	if r.Code != 0 {
		t.Fatalf("json exit %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "pool-pick-only-json", r.Stdout)
	if r := f.Run("pool", "pick", "--id", "auto", "--pool-only", "ivan", "--json", "--field", "data.selected.picked_by"); strings.TrimSpace(r.Stdout) != "only" {
		t.Errorf("picked_by %q", r.Stdout)
	}

	r = f.Run("pool", "pick", "--id", "auto", "--pool-only", "ivan", "--pool-only-week", "43", "--color", "never")
	if r.Code != 1 {
		t.Fatalf("lowered cap: exit %d", r.Code)
	}
	testkit.GoldenText(t, "pool-pick-only-blocked", r.Stdout)
	if r.Stderr != "agx: ivan: week is 43% (cap 43%)\n" {
		t.Errorf("stderr %q, want one line", r.Stderr)
	}
	r = f.Run("pool", "pick", "--id", "auto", "--pool-only", "ivan", "--pool-only-week", "43", "--json")
	if r.Code != 1 {
		t.Fatalf("blocked json: exit %d", r.Code)
	}
	f.Golden(t, "pool-pick-only-blocked-json", r.Stdout)
}

// TestPoolOnlyRefusedFlags: --pool-only with --pool-force is a usage
// error on pool pick and login token (exit 2) and on a launch (exit 125,
// a launching command's code for agx's own errors), with nothing on
// stdout; so are a cap above the member's, a cap without --pool-only, a
// non-number and a non-member.
func TestPoolOnlyRefusedFlags(t *testing.T) {
	f := pickFixture(t)
	f.FakeAgent("claude")
	f.HomeDir(".claude")
	both := []string{"--pool-only", "ivan", "--pool-force", "bob"}
	for _, args := range [][]string{
		append([]string{"pool", "pick", "--id", "auto", "--cached"}, both...),
		append([]string{"run", "--login", "auto", "--dry-run"}, both...),
		append([]string{"new", "--login", "auto", "--dry-run"}, both...),
		append([]string{"login", "token", "--id", "auto"}, both...),
	} {
		r := f.Run(args...)
		want := 2
		if args[0] == "run" || args[0] == "new" {
			want = 125
		}
		if r.Code != want || r.Stdout != "" || !strings.Contains(r.Stderr, "--pool-only and --pool-force both choose the member") {
			t.Errorf("agx %s: exit %d %q %q", strings.Join(args, " "), r.Code, r.Stdout, r.Stderr)
		}
	}
	for _, c := range []struct {
		args []string
		msg  string
	}{
		{[]string{"--pool-only", "ivan", "--pool-only-week", "80"}, "--pool-only-week 80 is above ivan's week cap 70; it may only lower it"},
		{[]string{"--pool-only-short", "10"}, "add --pool-only NAME"},
		{[]string{"--pool-only", "ivan", "--pool-only-short", "high"}, `"high" is not a number`},
		{[]string{"--pool-only", "zoe"}, "zoe is not a member of pool auto"},
	} {
		r := f.Run(append([]string{"run", "--login", "auto", "--dry-run"}, c.args...)...)
		if r.Code != 125 || !strings.Contains(r.Stderr, c.msg) {
			t.Errorf("%v: exit %d %q", c.args, r.Code, r.Stderr)
		}
	}
}

// TestPoolOnlyDryRunGolden: the dry run shows the pin with its source,
// the member's effective caps with theirs, and the pick from the cache.
// The golden holds the pool decisions of --json (the text table wraps
// by the length of the temporary paths); the text is checked by line.
func TestPoolOnlyDryRunGolden(t *testing.T) {
	f := pickFixture(t)
	f.FakeAgent("claude")
	f.HomeDir(".claude")
	args := []string{"run", "--login", "auto", "--pool-only", "ivan", "--pool-only-week", "60", "--dry-run"}
	r := f.Run(append(args, "--json")...)
	if r.Code != 0 {
		t.Fatalf("exit %d: %s", r.Code, r.Stderr)
	}
	var pools []map[string]string
	for _, d := range parseEnv(t, r).Decisions {
		if d.Name == "pool" {
			pools = append(pools, map[string]string{"value": d.Value, "source": d.Source})
		}
	}
	b, err := json.Marshal(pools)
	if err != nil {
		t.Fatal(err)
	}
	f.Golden(t, "run-pool-only-dry-run", string(b))
	r = f.Run(args...)
	for _, want := range []string{"pick: only ivan (caps enforced) for every key of this run", "from --pool-only", "ivan 5h cap 50%", "ivan week cap 60%", "from --pool-only-week", "pick now: ivan (only), from the cache (20s old)"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.Stdout)
		}
	}
	r = f.Run("resume", "--login", "auto", "--pool-only", "ivan", "--dry-run", "--last")
	if strings.Contains(r.Stderr, "unknown flag") {
		t.Errorf("resume does not take --pool-only: %s", r.Stderr)
	}
}

// TestPoolOnlyWhoamiAndLiveShow: a pinned run shows "only: NAME (caps
// enforced)" in whoami and live show; "in use" reads "none (…)" while
// the member is over its cap; live use sets and clears the pin.
func TestPoolOnlyWhoamiAndLiveShow(t *testing.T) {
	f := pickFixture(t)
	const id = "20261118-180000-only"
	dir := writeRun(t, f, id, startAgent(t, id), `{"run_id":"`+id+`","agent":"claude","start_login":"auto","in_use":"ivan","helper":"agx credential","pool_only":"ivan"}`)
	f.Env["AGX_RUN_ID"], f.Env["AGX_RUN_AGENT"], f.Env["AGX_RUN_PROFILE"], f.Env["AGX_RUN_LOGIN"], f.Env["AGX_RUN_HOME"] = id, "claude", "personal", "auto", f.Home+"/.claude"
	r := f.Run("whoami", "--color", "never")
	if r.Code != 0 {
		t.Fatalf("%d %s", r.Code, r.Stderr)
	}
	testkit.GoldenText(t, "whoami-pool-only", f.Normalize(r.Stdout))
	for _, want := range []string{"only:     ivan (caps enforced)", "in use:   ivan"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.Stdout)
		}
	}
	if strings.Contains(r.Stdout, "next:") {
		t.Errorf("next while the pin is in use:\n%s", r.Stdout)
	}
	if r := f.Run("whoami", "--json", "--field", "data.only"); strings.TrimSpace(r.Stdout) != "ivan" {
		t.Errorf("--field data.only %q", r.Stdout)
	}

	f.WriteFile(filepath.Join(dir, "live.jsonc"), `{"run_id":"`+id+`","agent":"claude","start_login":"auto","helper":"agx credential","pool_only":"ivan","pool_only_week":40,"no_key":"ivan over cap: week is 43% (cap 40%)"}`, 0o600)
	r = f.Run("whoami", "--color", "never")
	testkit.GoldenText(t, "whoami-pool-only-blocked", f.Normalize(r.Stdout))
	if !strings.Contains(r.Stdout, "in use:   none (ivan over cap: week is 43% (cap 40%))") || strings.Contains(r.Stdout, "next:") {
		t.Errorf("blocked:\n%s", r.Stdout)
	}
	delete(f.Env, "AGX_RUN_ID")
	r = f.Run("live", "show", "--id", id, "--color", "never")
	if r.Code != 0 {
		t.Fatalf("live show %d %s", r.Code, r.Stderr)
	}
	testkit.GoldenText(t, "live-show-pool-only", f.Normalize(r.Stdout))
	for _, want := range []string{"only        ivan (caps enforced)", "in use      none (ivan over cap: week is 43% (cap 40%))"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.Stdout)
		}
	}
	if r := f.Run("live", "show", "--id", id, "--json", "--field", "data.no_key"); strings.TrimSpace(r.Stdout) != "ivan over cap: week is 43% (cap 40%)" {
		t.Errorf("--field data.no_key %q", r.Stdout)
	}

	r = f.Run("live", "use", "--id", id, "--login", "auto")
	if r.Code != 0 || strings.Contains(r.Stdout, "caps enforced") {
		t.Fatalf("clear: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	if st := f.ReadFile(filepath.Join(dir, "live.jsonc")); strings.Contains(st, "pool_only") || strings.Contains(st, "no_key") {
		t.Errorf("not cleared: %s", st)
	}
	r = f.Run("live", "use", "--id", id, "--pool-only", "grace", "--pool-only-short", "10")
	if r.Code != 0 || !strings.Contains(r.Stdout, "only        grace (caps enforced)") {
		t.Fatalf("set: %d %q %q", r.Code, r.Stdout, r.Stderr)
	}
	if st := f.ReadFile(filepath.Join(dir, "live.jsonc")); !strings.Contains(st, `"pool_only": "grace"`) || !strings.Contains(st, `"pool_only_short": 10`) {
		t.Errorf("live.jsonc %s", st)
	}
	if r := f.Run("live", "use", "--id", id, "--pool-only", "grace", "--pool-only-week", "90"); r.Code != 2 || !strings.Contains(r.Stderr, "may only lower it") {
		t.Errorf("higher cap on live use: %d %q", r.Code, r.Stderr)
	}
}
