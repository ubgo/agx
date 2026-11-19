package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/pool"
)

// --pool-only pins one member with its caps enforced: every key of the
// run is that member's while the pool's rules allow, no key otherwise
// (never another member, never over its caps), and the key comes back on
// its own once usage falls under the cap. freshPool's members: own alice
// (30% week) and bob (20%), shared carol (1%) and derek (5h 60%:
// blocked by the default 5h cap 50).

// pct is a pointer to a percent.
func pct(v float64) *float64 { return &v }

// TestPoolOnlyPick: the pin is the only candidate and is picked by
// "only", even where the rules would pick another; a blocked pin is the
// "none usable" failure naming the member and every reason, the report
// still returned; the pool's config force does not apply.
func TestPoolOnlyPick(t *testing.T) {
	m, fv, a := freshPool(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "carol"}}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := rep.Selected
	if s == nil || s.Name != "carol" || s.PickedBy != PickedByOnly || !s.Only || s.Forced || len(rep.Candidates) != 1 {
		t.Fatalf("selected %+v candidates %d", s, len(rep.Candidates))
	}
	rep, err = a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "derek"}}, TimeOptions{})
	if err == nil || apperr.As(err).Code != apperr.CodeAutoPick || apperr.Exit(err, false) != apperr.ExitFailure || err.Error() != "derek: 5h is 60% (cap 50%)" {
		t.Fatalf("blocked pin: %v", err)
	}
	if rep.Selected != nil || len(rep.Candidates) != 1 || rep.Candidates[0].Name != "derek" {
		t.Errorf("report %+v", rep)
	}
	m.f.WriteConfig(strings.Replace(poolConfig, `"key_ttl": "30s"`, `"force": "alice", "key_ttl": "30s"`, 1))
	rep, err = m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "bob"}}, TimeOptions{})
	if err != nil || rep.Selected.Name != "bob" || rep.Selected.Forced {
		t.Errorf("config force under a pin: %+v %v", rep.Selected, err)
	}
	if fv.count("/v1/messages") != 0 {
		t.Errorf("probed %d times with a fresh cache", fv.count("/v1/messages"))
	}
}

// TestPoolOnlyRefusals: --pool-only with --pool-force is a conflict (exit
// 2); a non-member, a tighter cap without --pool-only, above the
// member's cap or outside 0..100 are usage errors (exit 2); each on pool
// pick, launch and login token alike.
func TestPoolOnlyRefusals(t *testing.T) {
	_, _, a := freshPool(t)
	cases := []struct {
		name  string
		force string
		only  PoolOnly
		code  apperr.Code
		msg   string
	}{
		{"with --pool-force", "bob", PoolOnly{Member: "carol"}, apperr.CodeConflict, "--pool-only and --pool-force both choose the member"},
		{"not a member", "", PoolOnly{Member: "zoe"}, apperr.CodeUsage, "--pool-only zoe: zoe is not a member of pool auto (members: alice, bob, carol, derek)"},
		{"caps without --pool-only", "", PoolOnly{Caps: pool.Tighter{Week: pct(10)}}, apperr.CodeUsage, "--pool-only-short and --pool-only-week lower the --pool-only member's caps; add --pool-only NAME"},
		{"week above", "", PoolOnly{Member: "carol", Caps: pool.Tighter{Week: pct(80)}}, apperr.CodeUsage, "--pool-only-week 80 is above carol's week cap 70; it may only lower it"},
		{"5h above own", "", PoolOnly{Member: "alice", Caps: pool.Tighter{Short: pct(96)}}, apperr.CodeUsage, "--pool-only-short 96 is above alice's 5h cap 95; it may only lower it"},
		{"out of range", "", PoolOnly{Member: "carol", Caps: pool.Tighter{Short: pct(-1)}}, apperr.CodeUsage, "--pool-only-short -1 is not a number from 0 to 100"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := func(where string, err error) {
				t.Helper()
				if err == nil || apperr.As(err).Code != c.code || apperr.Exit(err, false) != apperr.ExitUsage || !strings.Contains(err.Error(), c.msg) {
					t.Errorf("%s: %v, want %s %q", where, err, c.code, c.msg)
				}
			}
			_, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Force: c.force, Only: c.only}, TimeOptions{})
			check("pool pick", err)
			_, err = dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolForce, o.PoolOnly = "auto", c.force, c.only })
			check("launch", err)
			_, _, err = a.LoginTokenWarn(context.Background(), "auto", c.force, c.only)
			check("login token", err)
		})
	}
}

// TestPoolOnlyNeedsAPool: --pool-only on a token login is refused at
// launch, on login token and on live use.
func TestPoolOnlyNeedsAPool(t *testing.T) {
	m, _, a := freshPool(t)
	if _, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolOnly = "bob", PoolOnly{Member: "carol"} }); err == nil || !strings.Contains(err.Error(), "--pool-only picks a member of a pool login; this run's login is bob (oauth-token)") {
		t.Errorf("launch: %v", err)
	}
	if _, _, err := a.LoginTokenWarn(context.Background(), "bob", "", PoolOnly{Member: "carol"}); err == nil || apperr.As(err).Code != apperr.CodeUsage {
		t.Errorf("login token: %v", err)
	}
	id := liveRun(t, m, a, "auto")
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "bob", PoolOnly: PoolOnly{Member: "carol"}}); err == nil || !strings.Contains(err.Error(), "login bob is oauth-token") {
		t.Errorf("live use: %v", err)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "zoe", PoolOnly: PoolOnly{Member: "carol"}}); err == nil || apperr.As(err).Code != apperr.CodeNotFound {
		t.Errorf("live use unknown login: %v", err)
	}
}

// TestPoolOnlyDisabledAtLaunch: a disabled member is a clear error at
// launch and on live use (it would never get a key); pool pick reports
// it blocked.
func TestPoolOnlyDisabledAtLaunch(t *testing.T) {
	m, _, _ := freshPool(t)
	m.f.WriteConfig(strings.Replace(poolConfig, `"bob": { "short": 95, "week": 98 }`, `"bob": { "short": 95, "week": 98, "enabled": false }`, 1))
	a := m.app(t)
	_, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.PoolOnly = "auto", PoolOnly{Member: "bob"} })
	if err == nil || apperr.Exit(err, false) != apperr.ExitUsage || !strings.Contains(err.Error(), "--pool-only bob: bob is disabled in pool auto, so it would never get a key") || apperr.As(err).Hint != "agx login edit --id auto --pool-enable bob" {
		t.Errorf("launch: %v (hint %q)", err, apperr.As(err).Hint)
	}
	id := liveRun(t, m, a, "auto")
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Member: "bob"}}); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("live use: %v", err)
	}
	if _, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "bob"}}, TimeOptions{}); err == nil || err.Error() != "bob: disabled in config; no usage reading" {
		t.Errorf("pick: %v", err)
	}
}

// TestPoolOnlyIgnoresPicker: a pin is not the picker's to change: the
// picker command is not run and the pin is picked by "only".
func TestPoolOnlyIgnoresPicker(t *testing.T) {
	m, _, _ := freshPool(t)
	ran := filepath.Join(m.f.Home, "picker-ran")
	m.f.WriteFile(filepath.Join(m.f.Bin, "pick-derek"), "#!/bin/sh\ntouch "+ran+"\necho derek\n", 0o755)
	m.f.WriteConfig(pickerConfig("pick-derek", ""))
	rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "carol"}}, TimeOptions{})
	if err != nil || rep.Selected.Name != "carol" || rep.Selected.PickedBy != PickedByOnly || len(rep.Warnings) != 0 {
		t.Fatalf("%+v %v %v", rep.Selected, rep.Warnings, err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("the picker ran under a pin")
	}
	p, err := dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login, o.PoolOnly = "auto", PoolOnly{Member: "carol"} })
	if err != nil || strings.Contains(decisionValues(p), "picker") {
		t.Errorf("dry run names the picker under a pin: %v\n%s", err, decisionValues(p))
	}
}

// TestPoolOnlyTighterCaps: a lower cap blocks the pin at usage under its
// configured cap, for that call only; the config is not changed.
func TestPoolOnlyTighterCaps(t *testing.T) {
	m, _, a := freshPool(t)
	_, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "alice", Caps: pool.Tighter{Week: pct(30)}}}, TimeOptions{})
	if err == nil || err.Error() != "alice: week is 30% (cap 30%)" {
		t.Errorf("lowered week: %v", err)
	}
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "alice", Caps: pool.Tighter{Week: pct(31), Short: pct(95)}}}, TimeOptions{})
	if err != nil || rep.Candidates[0].Caps != (pool.Caps{Short: 95, Week: 31}) {
		t.Errorf("caps %+v %v", rep.Candidates, err)
	}
	before, _ := os.ReadFile(m.f.ConfigDir + "/config.jsonc")
	if rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Only: PoolOnly{Member: "alice"}}, TimeOptions{}); err != nil || rep.Candidates[0].Caps.Week != 98 {
		t.Errorf("next call keeps the configured cap: %+v %v", rep.Candidates, err)
	}
	if after, _ := os.ReadFile(m.f.ConfigDir + "/config.jsonc"); string(after) != string(before) {
		t.Error("config changed")
	}
}

// TestPoolOnlyDryRun: the pin and the member's effective caps each name
// their source (the run's flag, the member's own entry, the pool's
// defaults, agx's default); the pick now is from the cache, or "none"
// with the member's reasons when it is blocked; the run keeps the pin.
func TestPoolOnlyDryRun(t *testing.T) {
	m, _, a := freshPool(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) {
		o.Login, o.PoolOnly = "auto", PoolOnly{Member: "alice", Caps: pool.Tighter{Week: pct(90)}}
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := decisionValues(p)
	for _, want := range []string{
		"pool pick: only alice (caps enforced) for every key of this run [--pool-only]",
		"pool alice 5h cap 95% [config logins.auto.pool.accounts.alice.short]",
		"pool alice week cap 90% [--pool-only-week]",
		"pool pick now: alice (only), from the cache (0s old); agx pool pick --id auto probes [login-usage cache]",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing %q in\n%s", want, notes)
		}
	}
	if p.Live.PoolOnly != "alice" || p.Live.PoolOnlyWeek == nil || *p.Live.PoolOnlyWeek != 90 || p.Live.PoolOnlyShort != nil {
		t.Errorf("live %+v", p.Live)
	}
	p, _ = dryRun(t, a, func(o *LaunchOptions) {
		o.Login, o.PoolOnly = "auto", PoolOnly{Member: "derek", Caps: pool.Tighter{Short: pct(40)}}
	})
	notes = decisionValues(p)
	for _, want := range []string{
		"pool derek 5h cap 40% [--pool-only-short]",
		"pool derek week cap 70% [default]",
		"pool pick now: none, derek over cap: 5h is 60% (cap 40%), from the cache (0s old); no key until it is under its caps [login-usage cache]",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing %q in\n%s", want, notes)
		}
	}
	m.f.WriteConfig(strings.Replace(poolConfig, `"key_ttl": "30s"`, `"defaults": { "short": 40, "week": 60 }, "key_ttl": "30s"`, 1))
	p, _ = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login, o.PoolOnly = "auto", PoolOnly{Member: "carol"} })
	notes = decisionValues(p)
	for _, want := range []string{"pool carol 5h cap 40% [config logins.auto.pool.defaults.short]", "pool carol week cap 60% [config logins.auto.pool.defaults.week]"} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing %q in\n%s", want, notes)
		}
	}
}

// TestPoolOnlyLaunchWritesLiveState: a real (non-dry) launch keeps the
// pin and its caps in live.jsonc next to the other fields.
func TestPoolOnlyLaunchWritesLiveState(t *testing.T) {
	_, _, a := freshPool(t)
	o := LaunchOptions{Mode: ModeRun, PoolOnly: PoolOnly{Member: "carol", Caps: pool.Tighter{Short: pct(20)}}}
	o.Login = "auto"
	p, err := a.Prepare(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.RunDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.writeLiveStart(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	st, err := live.Read(p.RunDir)
	if err != nil || st.PoolOnly != "carol" || st.PoolOnlyShort == nil || *st.PoolOnlyShort != 20 || st.PoolOnlyWeek != nil || st.StartLogin != "auto" {
		t.Errorf("state %+v %v", st, err)
	}
}

// TestPoolOnlyLiveUse: live use --pool-only pins a running pool session
// (with tighter caps) and clears a run force; --login POOL clears the
// pin back to the pool rules; --login MEMBER is the cap-ignoring pin and
// clears it too; --start-login --pool-only pins on the start pool;
// --login POOL --pool-only switches pool and pins in one step.
func TestPoolOnlyLiveUse(t *testing.T) {
	m, _, a := freshPool(t)
	id := liveRun(t, m, a, "auto")
	dir := run.Dir(a.paths.Runs, id)
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error { s.PoolForce = "bob"; return nil }); err != nil {
		t.Fatal(err)
	}
	r, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Member: "carol", Caps: pool.Tighter{Week: pct(50)}}})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := live.Read(dir)
	if st.PoolOnly != "carol" || *st.PoolOnlyWeek != 50 || st.PoolForce != "" || st.ChosenLogin != "" || r.Only != "carol" || r.Next != "auto" {
		t.Errorf("set: state %+v run %+v", st, r)
	}
	cr, err := m.app(t).Credential(context.Background())
	if err != nil || cr.Login != "carol" || cr.Pool != "auto" {
		t.Fatalf("pinned credential %+v %v", cr, err)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "auto"}); err != nil {
		t.Fatal(err)
	}
	st, _ = live.Read(dir)
	if st.PoolOnly != "" || st.PoolOnlyWeek != nil || st.ChosenLogin != "auto" {
		t.Errorf("cleared by --login POOL: %+v", st)
	}
	if cr, _ := m.app(t).Credential(context.Background()); cr.Login != "alice" {
		t.Errorf("back on the rules: %s", cr.Login)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Member: "carol"}}); err != nil {
		t.Fatal(err)
	}
	if st, _ = live.Read(dir); st.ChosenLogin != "auto" || st.PoolOnly != "carol" {
		t.Errorf("pin on the chosen pool: %+v", st)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "derek"}); err != nil {
		t.Fatal(err)
	}
	if st, _ = live.Read(dir); st.PoolOnly != "" || st.ChosenLogin != "derek" {
		t.Errorf("cleared by --login MEMBER: %+v", st)
	}
	if cr, _ := m.app(t).Credential(context.Background()); cr.Login != "derek" || cr.Pool != "" {
		t.Errorf("member pin ignores caps: %+v", cr)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Member: "carol"}}); err == nil || !strings.Contains(err.Error(), "login derek is oauth-token") {
		t.Errorf("pin while on a member login: %v", err)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, StartLogin: true, PoolOnly: PoolOnly{Member: "carol"}}); err != nil {
		t.Fatal(err)
	}
	if st, _ = live.Read(dir); st.ChosenLogin != "" || st.PoolOnly != "carol" {
		t.Errorf("--start-login --pool-only: %+v", st)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "auto", PoolOnly: PoolOnly{Member: "bob"}}); err != nil {
		t.Fatal(err)
	}
	if st, _ = live.Read(dir); st.ChosenLogin != "auto" || st.PoolOnly != "bob" {
		t.Errorf("--login POOL --pool-only: %+v", st)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Caps: pool.Tighter{Week: pct(10)}}}); err == nil || apperr.As(err).Code != apperr.CodeUsage {
		t.Errorf("caps without --pool-only: %v", err)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, PoolOnly: PoolOnly{Member: "zoe"}}); err == nil || !strings.Contains(err.Error(), "not a member") {
		t.Errorf("non-member: %v", err)
	}
}

// TestPoolOnlyWhoami: whoami names the pin, next is the pin while the
// cache says it is usable and empty while it is blocked, and "in use"
// reads "none (…)" after a key request got nothing.
func TestPoolOnlyWhoami(t *testing.T) {
	m, _, a := freshPool(t)
	id := liveRun(t, m, a, "auto")
	m.env[run.EnvRunLogin] = "auto"
	dir := run.Dir(a.paths.Runs, id)
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error { s.PoolOnly, s.InUse = "carol", "alice"; return nil }); err != nil {
		t.Fatal(err)
	}
	w, err := m.app(t).Whoami()
	if err != nil || w.Only != "carol" || w.Next != "carol" || w.InUse != "alice" {
		t.Fatalf("%+v %v", w, err)
	}
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error {
		s.PoolOnly, s.InUse, s.NoKey = "derek", "", "derek over cap: 5h is 60% (cap 50%)"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	w, _ = m.app(t).Whoami()
	if w.Next != "" || w.InUse != "" || InUseText(w.InUse, w.Live.NoKey, "-") != "none (derek over cap: 5h is 60% (cap 50%))" {
		t.Errorf("blocked: %+v", w)
	}
	if OnlyText("carol") != "carol (caps enforced)" || InUseText("", "", "-") != "-" || InUseText("bob", "x", "-") != "bob" {
		t.Error("texts")
	}
}

// TestPoolOnlyNoKeyText: a cap reason reads "over cap", any other "not
// usable".
func TestPoolOnlyNoKeyText(t *testing.T) {
	if got := onlyNoKey("bob", []string{"status is limit-reached", "week is 99% (cap 98%)"}); got != "bob over cap: status is limit-reached; week is 99% (cap 98%)" {
		t.Errorf("%q", got)
	}
	if got := onlyNoKey("bob", []string{"disabled in config", "no usage reading"}); got != "bob not usable: disabled in config; no usage reading" {
		t.Errorf("%q", got)
	}
	if got := onlyReasons(pool.Result{}); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

// TestPoolOnlyCredentialFlow is a pinned run end to end: Claude's key
// helper (agx credential, a new App each call) against an httptest
// vendor and a movable clock. While the pin is usable it gets its key;
// once its usage reaches the cap it gets none (exit 1, one stderr line
// naming the reason, never another member's key) and live.jsonc says
// why; when usage falls back under the cap the key returns on its own.
// Only the pinned member is probed.
func TestPoolOnlyCredentialFlow(t *testing.T) {
	fl := newFlow(t)
	fl.startUsage()
	if _, err := fl.m.app(t).LiveUse(context.Background(), LiveUseOptions{ID: fl.run, PoolOnly: PoolOnly{Member: "ivan"}}); err != nil {
		t.Fatal(err)
	}
	dir := run.Dir(fl.m.app(t).paths.Runs, fl.run)
	state := func() live.State {
		st, err := live.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	t.Run("usable: the pin's key, though the rules would pick grace", func(t *testing.T) {
		r := fl.helper()
		if r.code != 0 || r.token != tokIvan || len(r.warns) != 0 {
			t.Fatalf("%+v", r)
		}
		if fl.probes() != 1 {
			t.Errorf("probes %d, want only the pinned member", fl.probes())
		}
		if st := state(); st.InUse != "ivan" || st.NoKey != "" {
			t.Errorf("state %+v", st)
		}
		if !strings.Contains(fl.lastLog(), `picked="ivan" group="shared" forced=false by="only"`) {
			t.Errorf("log %s", fl.lastLog())
		}
	})
	t.Run("usage crosses the cap: no key, exit 1, one stderr line", func(t *testing.T) {
		fl.setUsage(member{tok: tokIvan, short: 4, week: 70, resetH: 42})
		fl.advance(61 * time.Second)
		r := fl.helper()
		if r.code != apperr.ExitFailure || r.token != "" {
			t.Fatalf("%+v", r)
		}
		if len(r.warns) != 1 || r.warns[0] != "ivan: week is 70% (cap 70%)" {
			t.Errorf("stderr %q, want one line", r.warns)
		}
		if st := state(); st.InUse != "" || st.NoKey != "ivan over cap: week is 70% (cap 70%)" {
			t.Errorf("state %+v", st)
		}
		if !strings.Contains(fl.lastLog(), `picked=null`) {
			t.Errorf("log %s", fl.lastLog())
		}
		show, err := fl.m.app(t).LiveShow(context.Background(), fl.run)
		if err != nil || show.Only != "ivan" || show.NoKey != "ivan over cap: week is 70% (cap 70%)" || show.InUse != "" {
			t.Errorf("live show %+v %v", show, err)
		}
	})
	t.Run("still over the cap within max age: still no key, no probe", func(t *testing.T) {
		fl.advance(10 * time.Second)
		if r := fl.helper(); r.code != apperr.ExitFailure || fl.probes() != 2 {
			t.Errorf("%+v probes %d", r, fl.probes())
		}
	})
	t.Run("usage drops under the cap: the key returns on its own", func(t *testing.T) {
		fl.setUsage(member{tok: tokIvan, short: 4, week: 40, resetH: 42})
		fl.advance(61 * time.Second)
		r := fl.helper()
		if r.code != 0 || r.token != tokIvan || len(r.warns) != 0 {
			t.Fatalf("%+v", r)
		}
		if st := state(); st.InUse != "ivan" || st.NoKey != "" {
			t.Errorf("state %+v", st)
		}
	})
	t.Run("a tighter cap set live blocks below the configured cap", func(t *testing.T) {
		if _, err := fl.m.app(t).LiveUse(context.Background(), LiveUseOptions{ID: fl.run, PoolOnly: PoolOnly{Member: "ivan", Caps: pool.Tighter{Week: pct(40)}}}); err != nil {
			t.Fatal(err)
		}
		r := fl.helper()
		if r.code != apperr.ExitFailure || len(r.warns) != 1 || r.warns[0] != "ivan: week is 40% (cap 40%)" {
			t.Errorf("%+v", r)
		}
	})
}
