package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
)

// The pool is optional and every part of it can be overridden: a member
// login bypasses it, --pool-force forces one member for one run, agx live
// use pins and unpins, AGX_LOGIN chooses like for any login, and a picker
// command of the user's replaces the built-in rules. Every member's
// usage here is cached fresh, so no test probes unless it says so.

// freshPool is poolMachine with every member cached one minute old:
// own alice 30% and bob 20%, shared carol 1% and derek 10% (5h 60%:
// blocked), every weekly reset the same (66 h away), so the rules pick
// alice (config order) and carol waits outside the 48 h window.
func freshPool(t *testing.T) (*machine, *fakeVendor, *App) {
	t.Helper()
	m, fv := poolMachine(t, nil)
	m.f.FakeAgent("claude")
	a := m.app(t)
	writeCache(t, a, 0, map[string]login.UsageEntry{
		"alice": okEntry(1, 30, weeklyReset), "bob": okEntry(1, 20, weeklyReset),
		"carol": okEntry(1, 1, weeklyReset), "derek": okEntry(60, 10, weeklyReset),
	})
	return m, fv, m.app(t)
}

// dryRun prepares a dry run with o's login options.
func dryRun(t *testing.T, a *App, edit func(o *LaunchOptions)) (*LaunchPlan, error) {
	t.Helper()
	o := LaunchOptions{Mode: ModeRun, DryRun: true}
	edit(&o)
	return a.Prepare(context.Background(), o)
}

// TestPoolMemberLoginBypassesPool: --login MEMBER is a plain token run:
// not live, no pool lines, the member's token variable.
func TestPoolMemberLoginBypassesPool(t *testing.T) {
	_, _, a := freshPool(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login = "carol" })
	if err != nil {
		t.Fatal(err)
	}
	if p.Live != nil || strings.Contains(decisionValues(p), "pool") {
		t.Errorf("live %+v\n%s", p.Live, decisionValues(p))
	}
}

// TestPoolAGXLogin: AGX_LOGIN chooses the pool or a member like any
// login, below the flags.
func TestPoolAGXLogin(t *testing.T) {
	m, _, _ := freshPool(t)
	m.env["AGX_LOGIN"] = "auto"
	p, err := dryRun(t, m.app(t), func(*LaunchOptions) {})
	if err != nil || p.Live == nil || p.Live.StartLogin != "auto" {
		t.Fatalf("AGX_LOGIN=auto: %+v %v", p, err)
	}
	m.env["AGX_LOGIN_CLAUDE"] = "bob"
	p, err = dryRun(t, m.app(t), func(*LaunchOptions) {})
	if err != nil || p.Live != nil || p.Login != "bob" {
		t.Fatalf("AGX_LOGIN_CLAUDE=bob: %+v %v", p, err)
	}
	p, err = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "auto" })
	if err != nil || p.Live == nil {
		t.Fatalf("--login beats AGX_LOGIN: %v", err)
	}
}

// TestPoolForceFlagOnLaunch: --pool-force is kept for the run and shown
// with its source; it needs a pool login and a member that can serve.
func TestPoolForceFlagOnLaunch(t *testing.T) {
	_, _, a := freshPool(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolForce = "auto", "bob" })
	if err != nil {
		t.Fatal(err)
	}
	notes := decisionValues(p)
	if p.Live.PoolForce != "bob" || !strings.Contains(notes, "pool pick: bob for every key of this run (forced) [--pool-force]") || !strings.Contains(notes, "pick now: bob (force)") {
		t.Errorf("live %+v\n%s", p.Live, notes)
	}
	if _, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolForce = "bob", "carol" }); err == nil || apperr.As(err).Code != apperr.CodeUsage {
		t.Errorf("token login: %v", err)
	}
	if _, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolForce = "auto", "zoe" }); err == nil || apperr.As(err).Code != apperr.CodeNotFound {
		t.Errorf("unknown member: %v", err)
	}
}

// TestPoolRunForcePinUnpin: a run's --pool-force decides every key; agx
// live use --login MEMBER pins another member; --login POOL hands the
// run back to the rules and drops the run force.
func TestPoolRunForcePinUnpin(t *testing.T) {
	m, fv, a := freshPool(t)
	id := liveRun(t, m, a, "auto")
	dir := run.Dir(a.paths.Runs, id)
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error { s.PoolForce = "alice"; return nil }); err != nil {
		t.Fatal(err)
	}
	cred := func() string {
		cr, err := m.app(t).Credential(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return cr.Login
	}
	if got := cred(); got != "alice" {
		t.Errorf("run force: %s", got)
	}
	if _, err := m.app(t).LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "bob"}); err != nil {
		t.Fatal(err)
	}
	if got := cred(); got != "bob" {
		t.Errorf("pinned: %s", got)
	}
	if _, err := m.app(t).LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "auto"}); err != nil {
		t.Fatal(err)
	}
	st, _ := live.Read(dir)
	if got := cred(); got != "alice" || st.PoolForce != "" {
		t.Errorf("unpinned: %s (rules pick alice: same reset, config order) force %q", got, st.PoolForce)
	}
	if fv.count("/v1/messages") != 0 {
		t.Errorf("probed %d times with a fresh cache", fv.count("/v1/messages"))
	}
}

// TestPoolTokenForce: agx login token --pool-force prints that member's
// token with a warning per broken rule; on a token login it is refused.
func TestPoolTokenForce(t *testing.T) {
	_, _, a := freshPool(t)
	tok, warns, err := a.LoginTokenWarn(context.Background(), "auto", "derek", PoolOnly{})
	if err != nil || tok.Reveal() != tokDave || len(warns) != 1 || warns[0].Message != "forced derek: 5h is 60% (cap 50%)" {
		t.Fatalf("tok %v warns %+v err %v", tok.Reveal() == tokDave, warns, err)
	}
	if _, _, err := a.LoginTokenWarn(context.Background(), "bob", "carol", PoolOnly{}); err == nil || apperr.As(err).Code != apperr.CodeUsage {
		t.Errorf("token login: %v", err)
	}
}

// pickerConfig adds a picker command to poolConfig.
func pickerConfig(cmd, timeout string) string {
	pk := `"picker": { "command": "` + cmd + `"`
	if timeout != "" {
		pk += `, "timeout": "` + timeout + `"`
	}
	return strings.Replace(poolConfig, `"key_ttl": "30s"`, pk+` }, "key_ttl": "30s"`, 1)
}

// TestPoolPicker: the picker's member wins (with a warning per broken
// rule); a non-member, no line, extra lines, a non-zero exit and a
// timeout each fall back to the built-in rules with a warning naming the
// failure; a force beats the picker.
func TestPoolPicker(t *testing.T) {
	m, _, _ := freshPool(t)
	script := func(name, body string) {
		m.f.WriteFile(filepath.Join(m.f.Bin, name), "#!/bin/sh\n"+body+"\n", 0o755)
	}
	script("pick-derek", "cat >/dev/null; echo derek")
	script("pick-zoe", "cat >/dev/null; echo zoe")
	script("pick-none", "cat >/dev/null")
	script("pick-two", "cat >/dev/null; echo bob; echo carol")
	script("pick-fail", "cat >/dev/null; echo broken >&2; exit 3")
	script("pick-slow", "sleep 5; echo bob")
	script("pick-spaces", "cat >/dev/null; printf '  bob  \\n\\n'")
	cases := []struct {
		cmd, timeout, want, by, warn string
	}{
		{"pick-derek", "", "derek", PickedByPicker, ""},
		{"pick-spaces", "", "bob", PickedByPicker, ""},
		{"pick-zoe", "", "alice", PickedByRules, `printed "zoe", which is not a member of the pool`},
		{"pick-none", "", "alice", PickedByRules, "printed no login name"},
		{"pick-two", "", "alice", PickedByRules, "printed 2 lines"},
		{"pick-fail", "", "alice", PickedByRules, "exited 3: broken"},
		{"pick-slow", "1s", "alice", PickedByRules, "timed out after 1s"},
	}
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			m.f.WriteConfig(pickerConfig(c.cmd, c.timeout))
			rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Selected.Name != c.want || rep.Selected.PickedBy != c.by {
				t.Errorf("selected %+v", rep.Selected)
			}
			switch {
			case c.warn == "" && len(rep.Warnings) != 0:
				t.Errorf("warnings %v", rep.Warnings)
			case c.warn != "" && (len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], c.warn) || !strings.Contains(rep.Warnings[0], "using the built-in rules")):
				t.Errorf("warnings %v, want %q", rep.Warnings, c.warn)
			}
		})
	}
	m.f.WriteConfig(pickerConfig("pick-derek", ""))
	rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil || !strings.Contains(rep.Selected.Reason, "picked by the picker command (pick-derek)") ||
		len(rep.Selected.Warnings) != 1 || rep.Selected.Warnings[0] != "5h is 60% (cap 50%)" {
		t.Errorf("picker warnings %+v %v", rep.Selected, err)
	}
	rep, err = m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Force: "bob"}, TimeOptions{})
	if err != nil || rep.Selected.Name != "bob" || rep.Selected.PickedBy != PickedByForce {
		t.Errorf("force over picker %+v %v", rep.Selected, err)
	}
}

// TestPoolPickerInput: the picker reads version-1 JSON with every member,
// its usage, the built-in pick and the rules, and AGX_POOL; never a
// token value, swept for each fake token.
func TestPoolPickerInput(t *testing.T) {
	m, _, _ := freshPool(t)
	in := filepath.Join(m.f.Home, "picker-in.json")
	envf := filepath.Join(m.f.Home, "picker-env.txt")
	m.f.WriteFile(filepath.Join(m.f.Bin, "pick-record"), "#!/bin/sh\ncat > "+in+"\necho \"$AGX_POOL\" > "+envf+"\necho alice\n", 0o755)
	m.f.WriteConfig(pickerConfig("pick-record", ""))
	if _, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{tokPersonal, tokWork, tokCarol, tokDave, "fake-token"} {
		if strings.Contains(string(b), tok) {
			t.Errorf("picker input holds a token")
		}
	}
	var got pickerInput
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Pool != "auto" || got.Agent != "claude" || got.Builtin == nil || *got.Builtin != "alice" || len(got.Members) != 4 ||
		got.Rules.SharedBorrowWindowHours != 48 || got.Rules.Accounts["alice"].Week != 98 || !got.Rules.Accounts["derek"].Enabled {
		t.Errorf("input %s", b)
	}
	if e, _ := os.ReadFile(envf); strings.TrimSpace(string(e)) != "auto" {
		t.Errorf("AGX_POOL %q", e)
	}
}

// TestPoolDryRunSources: each pool decision names its source: config
// keys when set, "default" when not, the picker and the force.
func TestPoolDryRunSources(t *testing.T) {
	m, _, a := freshPool(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login = "auto" })
	if err != nil {
		t.Fatal(err)
	}
	notes := decisionValues(p)
	for _, want := range []string{
		"pool pick: the built-in rules at each key request [default]",
		"pool key ttl 30s [config logins.auto.pool.key_ttl]",
		"pool usage max age 1m0s [default]",
		"pool shared borrow window 48h [default]",
		"pool 5h cutoff 99% [default]",
		"pool default caps 5h 50%, week 70% [default]",
		"pool member entries: 2 [config logins.auto.pool.accounts]",
		"pool log ~/pool.log [config logins.auto.pool.log]",
		"pool pick now: alice (rules), from the cache (0s old); agx pool pick --id auto probes [login-usage cache]",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing %q in\n%s", want, notes)
		}
	}
	m.f.WriteConfig(pickerConfig("pick-x", ""))
	p, _ = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "auto" })
	if !strings.Contains(decisionValues(p), "pool pick: the picker command pick-x at each key request; the built-in rules if it fails [config logins.auto.pool.picker]") {
		t.Errorf("picker source:\n%s", decisionValues(p))
	}
	m.f.WriteConfig(strings.Replace(poolConfig, `"key_ttl": "30s"`, `"force": "bob", "key_ttl": "30s"`, 1))
	p, _ = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "auto" })
	if !strings.Contains(decisionValues(p), "pool pick: bob for every key (forced) [config logins.auto.pool.force]") {
		t.Errorf("force source:\n%s", decisionValues(p))
	}
}

// TestPoolProbeWithoutCacheBlocks: a probe that fails with no cached
// reading leaves the member blocked with the failure's status (the
// selector's "failed agx without cache"); the failure is not cached.
func TestPoolProbeWithoutCacheBlocks(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {fail: true}, tokWork: {fail: true}, tokCarol: {fail: true}, tokDave: {fail: true}})
	a := m.app(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err == nil || apperr.As(err).Code != apperr.CodeAutoPick || !strings.Contains(err.Error(), "alice (status is network)") {
		t.Fatalf("err %v", err)
	}
	if rep.Selected != nil {
		t.Errorf("selected %+v", rep.Selected)
	}
	if c, _ := login.ReadUsageCache(a.paths); len(c.Logins) != 0 {
		t.Errorf("failed probes were cached: %v", c.Logins)
	}
}

// TestPoolMaxAge: usage_max_age 0s probes on every call; --max-age
// overrides the config for one call; a reading exactly max-age old is
// stale (younger than, as the selector).
func TestPoolMaxAge(t *testing.T) {
	m, fv := poolMachine(t, map[string]account{tokPersonal: {weekly: 1}, tokWork: {weekly: 1}, tokCarol: {weekly: 1}, tokDave: {weekly: 1}})
	pick := func(o PoolPickOptions) {
		o.ID = "auto"
		if _, err := m.app(t).PoolPick(context.Background(), o, TimeOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	pick(PoolPickOptions{})
	pick(PoolPickOptions{})
	if fv.count("/v1/messages") != 4 {
		t.Errorf("default max age: %d probes", fv.count("/v1/messages"))
	}
	pick(PoolPickOptions{MaxAgeSet: true, MaxAge: 0})
	if fv.count("/v1/messages") != 8 {
		t.Errorf("--max-age 0: %d probes", fv.count("/v1/messages"))
	}
	m2, fv2 := poolMachine(t, map[string]account{tokPersonal: {weekly: 1}, tokWork: {weekly: 1}, tokCarol: {weekly: 1}, tokDave: {weekly: 1}})
	writeCache(t, m2.app(t), 60*time.Second, map[string]login.UsageEntry{"alice": okEntry(1, 1, weeklyReset), "bob": okEntry(1, 1, weeklyReset), "carol": okEntry(1, 1, weeklyReset), "derek": okEntry(1, 1, weeklyReset)})
	if _, err := m2.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{}); err != nil {
		t.Fatal(err)
	}
	if fv2.count("/v1/messages") != 4 {
		t.Errorf("a reading exactly 60s old must be probed again: %d probes", fv2.count("/v1/messages"))
	}
	writeCache(t, m2.app(t), 59*time.Second, map[string]login.UsageEntry{"alice": okEntry(1, 1, weeklyReset), "bob": okEntry(1, 1, weeklyReset), "carol": okEntry(1, 1, weeklyReset), "derek": okEntry(1, 1, weeklyReset)})
	if _, err := m2.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{}); err != nil || fv2.count("/v1/messages") != 4 {
		t.Errorf("59s old is fresh: %d probes (%v)", fv2.count("/v1/messages"), err)
	}
}

// TestPoolProbeTimeoutFallsBack: a hung probe is cut at --timeout and the
// member's cached reading is used, with a warning (the selector's
// "timeout kills a hung agx and falls back").
func TestPoolProbeTimeoutFallsBack(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {delay: 5 * time.Second}, tokWork: {weekly: 99}, tokCarol: {weekly: 90}, tokDave: {weekly: 90}})
	m.timeout = 200 * time.Millisecond
	a := m.app(t)
	writeCache(t, a, 5*time.Minute, map[string]login.UsageEntry{"alice": okEntry(3, 40, weeklyReset)})
	start := time.Now()
	rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil || rep.Selected.Name != "alice" || len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "probe of alice failed (network") {
		t.Fatalf("%+v %v %v", rep.Selected, rep.Warnings, err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("took %s; the timeout did not cut the probe", time.Since(start))
	}
}

// TestPoolPickNotAPool: pool pick on a token login says pick is for
// pool logins, which agents can have one (from capabilities), and why
// the login's own agent cannot.
func TestPoolPickNotAPool(t *testing.T) {
	m, _ := poolMachine(t, nil)
	cfg := strings.Replace(poolConfig, `"logins": {`, `"logins": { "cx": { "agents": ["codex"], "kind": "api-key", "token": "x" },`, 1)
	m.f.WriteConfig(cfg)
	_, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "cx"}, TimeOptions{})
	if err == nil || apperr.As(err).Code != apperr.CodeUsage ||
		!strings.Contains(err.Error(), "pool pick is for pool logins; login cx is api-key. Pools serve claude today (codex: ") {
		t.Errorf("err %v", err)
	}
}
