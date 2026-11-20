package pool_test

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// Every rule at its boundary, beyond the port: caps exclusive, the borrow
// window edge to the second, the 5-hour cutoff, a missing 5-hour window,
// stale and unknown resets, ties, force edge cases and empty pools.

// one is a pool of own a and shared b with the defaults (50/70) for both.
func one() pool.Config { return classicNew([]string{"a"}, []string{"b"}) }

// TestBoundaryCapsExclusive: a hair under the cap is usable, the cap
// itself blocks, for the short and the weekly window.
func TestBoundaryCapsExclusive(t *testing.T) {
	const hair = 0.0001
	cases := []struct {
		name        string
		short, week float64
		usable      bool
	}{
		{"short under", 50 - hair, 10, true},
		{"short at", 50, 10, false},
		{"week under", 0, 70 - hair, true},
		{"week at", 0, 70, false},
		{"both at", 50, 70, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := classicNew(nil, []string{"b"})
			r := pickPort(t, map[string]pool.Reading{"b": {Status: pool.StatusOK, Short: &pool.Window{Percent: c.short}, Week: &pool.Window{Percent: c.week, ResetsAt: at(1)}}}, cfg, "")
			if (r.Selected != nil) != c.usable {
				t.Errorf("usable %v, want %v (reasons %v)", r.Selected != nil, c.usable, r.Candidates[0].Reasons)
			}
		})
	}
}

// TestBoundaryBorrowWindowSeconds: exactly 48h is inside, 48h-1s inside,
// 48h+1s outside.
func TestBoundaryBorrowWindowSeconds(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{48*time.Hour - time.Second, "b"}, {48 * time.Hour, "b"}, {48*time.Hour + time.Second, "a"}} {
		reset := portNow.Add(c.d)
		r := pickPort(t, map[string]pool.Reading{
			"a": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(200)}},
			"b": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: &reset}},
		}, one(), "")
		if selected(r) != c.want {
			t.Errorf("%s: selected %s, want %s", c.d, selected(r), c.want)
		}
	}
}

// TestBoundaryShortCutoff: the cutoff blocks at and over it, not under,
// even with caps of 100; a lower cutoff applies as configured.
func TestBoundaryShortCutoff(t *testing.T) {
	for _, c := range []struct {
		cutoff, short float64
		usable        bool
	}{{99, 98.9999, true}, {99, 99, false}, {99, 100, false}, {80, 79.9, true}, {80, 80, false}} {
		cfg := classicNew([]string{"a"}, nil)
		cfg.ShortUnavailableAt = c.cutoff
		cfg.Accounts = map[string]pool.Account{"a": {Short: p(100), Week: p(100)}}
		r := pickPort(t, map[string]pool.Reading{"a": {Status: pool.StatusOK, Short: &pool.Window{Percent: c.short}, Week: &pool.Window{Percent: 1, ResetsAt: at(10)}}}, cfg, "")
		if (r.Selected != nil) != c.usable {
			t.Errorf("cutoff %v short %v: usable %v", c.cutoff, c.short, r.Selected != nil)
		}
	}
}

// TestBoundaryMissingShortWindow: without a 5-hour window only the status
// can block an account that is at its 5-hour limit.
func TestBoundaryMissingShortWindow(t *testing.T) {
	cfg := classicNew([]string{"a"}, nil)
	week := &pool.Window{Percent: 1, ResetsAt: at(10)}
	if r := pickPort(t, map[string]pool.Reading{"a": {Status: pool.StatusOK, Week: week}}, cfg, ""); selected(r) != "a" {
		t.Errorf("ok without short: %s", selected(r))
	}
	r := pickPort(t, map[string]pool.Reading{"a": {Status: "limit-reached", Week: week}}, cfg, "")
	if r.Selected != nil || !slices.Equal(r.Candidates[0].Reasons, []string{"status is limit-reached"}) {
		t.Errorf("limit without short: %v", r.Candidates[0].Reasons)
	}
}

// TestBoundaryResetOrder: known resets by time, then unknown and passed
// resets (stale) in config order; a reset exactly now is not stale.
func TestBoundaryResetOrder(t *testing.T) {
	cfg := classicNew([]string{"a", "b", "c", "d"}, nil)
	cfg.Accounts = map[string]pool.Account{}
	nowReset := portNow
	r := pickPort(t, map[string]pool.Reading{
		"a": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1}},
		"b": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(-0.001)}},
		"c": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(5)}},
		"d": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: &nowReset}},
	}, cfg, "")
	if got := names(r, ""); !slices.Equal(got, []string{"d", "c", "a", "b"}) {
		t.Errorf("order %v", got)
	}
	if len(cand(t, r, "d").Notes) != 0 || len(cand(t, r, "b").Notes) != 1 {
		t.Errorf("notes d %v b %v", cand(t, r, "d").Notes, cand(t, r, "b").Notes)
	}
}

// TestBoundaryTies: equal resets keep config order with own first, even
// when the readings map lists them otherwise.
func TestBoundaryTies(t *testing.T) {
	cfg := classicNew([]string{"z", "y"}, []string{"x", "w"})
	cfg.BorrowWindow = 100 * time.Hour
	same := func() pool.Reading {
		return pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(24)}}
	}
	r := pickPort(t, map[string]pool.Reading{"w": same(), "x": same(), "y": same(), "z": same()}, cfg, "")
	if got := names(r, pool.StateReady); !slices.Equal(got, []string{"z", "y", "x", "w"}) {
		t.Errorf("ties %v", got)
	}
}

// TestBoundaryForceCases: force on a member without a reading is an
// error; on a limited or disabled member it picks with every warning.
func TestBoundaryForceCases(t *testing.T) {
	cfg := one()
	cfg.Accounts = map[string]pool.Account{"b": {Enabled: f2(false)}}
	data := map[string]pool.Reading{
		"a": {Status: "limit-reached", Short: &pool.Window{Percent: 100}, Week: &pool.Window{Percent: 80, ResetsAt: at(5)}},
		"b": {Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(5)}},
	}
	if _, err := pool.Select(cfg, map[string]pool.Reading{}, pool.Options{Now: portNow, Force: "a"}); !errors.Is(err, pool.ErrForcedMissing) {
		t.Errorf("missing: %v", err)
	}
	r := pickPort(t, data, cfg, "a")
	want := []string{"status is limit-reached", "5h is 100% (unavailable at 99%)", "5h is 100% (cap 50%)", "week is 80% (cap 70%)"}
	if r.Selected.Name != "a" || !slices.Equal(r.Selected.Warnings, want) {
		t.Errorf("limited force %+v", r.Selected)
	}
	if r := pickPort(t, data, cfg, "b"); r.Selected.Name != "b" || !slices.Equal(r.Selected.Warnings, []string{"disabled in config"}) {
		t.Errorf("disabled force %+v", r.Selected)
	}
}

// TestBoundaryEmptyAndAll: an empty pool, all disabled, all limited pick
// nothing; a pool of one picks it.
func TestBoundaryEmptyAndAll(t *testing.T) {
	if r := pickPort(t, nil, classicNew(nil, nil), ""); r.Selected != nil || len(r.Candidates) != 0 {
		t.Errorf("empty %+v", r)
	}
	all := classicNew([]string{"a"}, []string{"b"})
	all.Accounts = map[string]pool.Account{"a": {Enabled: f2(false)}, "b": {Enabled: f2(false)}}
	ok := func() pool.Reading {
		return pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: 1, ResetsAt: at(5)}}
	}
	if r := pickPort(t, map[string]pool.Reading{"a": ok(), "b": ok()}, all, ""); r.Selected != nil {
		t.Errorf("all disabled picked %s", selected(r))
	}
	lim := func() pool.Reading { return pool.Reading{Status: "limit-reached"} }
	if r := pickPort(t, map[string]pool.Reading{"a": lim(), "b": lim()}, one(), ""); r.Selected != nil {
		t.Errorf("all limited picked %s", selected(r))
	}
	if r := pickPort(t, map[string]pool.Reading{"b": ok()}, classicNew(nil, []string{"b"}), ""); selected(r) != "b" {
		t.Errorf("pool of one %s", selected(r))
	}
}

// TestPropertyPickIsAlwaysRight: for random readings, configs and clocks
// the pick (unforced) is usable, under every cap and the cutoff, in the
// pool the rules allow, and no allowed usable member resets sooner. With
// nothing picked, no member is usable.
func TestPropertyPickIsAlwaysRight(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 20000 {
		cfg, data := randomPool(rng)
		r, err := pool.Select(cfg, data, pool.Options{Now: portNow})
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		checkPick(t, i, cfg, data, r)
	}
}

// FuzzPickIsAlwaysRight is the property as a fuzz target (go test
// -fuzz=FuzzPickIsAlwaysRight ./pool/).
func FuzzPickIsAlwaysRight(f *testing.F) {
	f.Add(uint64(1), uint64(2))
	f.Add(uint64(42), uint64(7))
	f.Fuzz(func(t *testing.T, a, b uint64) {
		rng := rand.New(rand.NewPCG(a, b))
		cfg, data := randomPool(rng)
		r, err := pool.Select(cfg, data, pool.Options{Now: portNow})
		if err != nil {
			t.Fatal(err)
		}
		checkPick(t, 0, cfg, data, r)
	})
}

// randomPool is a pool of up to six members with random groups, caps,
// cutoff, window, enabled flags and readings (some missing windows,
// resets in the past, unknown resets and bad statuses).
func randomPool(rng *rand.Rand) (pool.Config, map[string]pool.Reading) {
	all := []string{"a", "b", "c", "d", "e", "f"}
	n := rng.IntN(len(all) + 1)
	var own, shared []string
	for _, m := range all[:n] {
		if rng.IntN(2) == 0 {
			own = append(own, m)
		} else {
			shared = append(shared, m)
		}
	}
	cfg := classicNew(own, shared)
	cfg.BorrowWindow = time.Duration(rng.IntN(200)) * time.Hour
	cfg.ShortUnavailableAt = float64(50 + rng.IntN(51))
	cfg.Defaults = pool.Caps{Short: float64(rng.IntN(101)), Week: float64(rng.IntN(101))}
	cfg.OwnDefaults = cfg.Defaults
	cfg.Accounts = map[string]pool.Account{}
	data := map[string]pool.Reading{}
	for _, m := range all[:n] {
		if rng.IntN(3) == 0 {
			cfg.Accounts[m] = pool.Account{Short: p(float64(rng.IntN(101))), Week: p(float64(rng.IntN(101))), Enabled: f2(rng.IntN(5) != 0)}
		}
		if rng.IntN(8) == 0 {
			continue // no reading
		}
		r := pool.Reading{Status: pool.StatusOK}
		if rng.IntN(6) == 0 {
			r.Status = []string{"limit-reached", "rejected", "network"}[rng.IntN(3)]
		}
		if rng.IntN(4) != 0 {
			r.Short = &pool.Window{Percent: rng.Float64() * 100}
		}
		if rng.IntN(5) != 0 {
			w := &pool.Window{Percent: rng.Float64() * 100}
			if rng.IntN(5) != 0 {
				w.ResetsAt = at(rng.Float64()*240 - 24)
			}
			r.Week = w
		}
		data[m] = r
	}
	return cfg, data
}

// checkPick asserts the property on one selection.
func checkPick(t *testing.T, i int, cfg pool.Config, data map[string]pool.Reading, r pool.Result) {
	t.Helper()
	usable := func(m string) bool {
		rd, ok := data[m]
		return len(cfg.Reasons(m, rd, ok)) == 0
	}
	members := slices.Concat(cfg.Own, cfg.Shared)
	ownUsable := slices.ContainsFunc(cfg.Own, usable)
	allowed := func(m string) bool {
		if !usable(m) {
			return false
		}
		if slices.Contains(cfg.Own, m) || !ownUsable {
			return true
		}
		w := data[m].Week
		return w != nil && w.ResetsAt != nil && w.ResetsAt.Sub(portNow) >= 0 && w.ResetsAt.Sub(portNow) <= cfg.BorrowWindow
	}
	deadline := func(m string) time.Duration {
		w := data[m].Week
		if w == nil || w.ResetsAt == nil || w.ResetsAt.Before(portNow) {
			return time.Duration(math.MaxInt64)
		}
		return w.ResetsAt.Sub(portNow)
	}
	if r.Selected == nil {
		if slices.ContainsFunc(members, allowed) {
			t.Fatalf("case %d: nothing picked but a member is allowed (%+v)", i, r.Candidates)
		}
		return
	}
	s := r.Selected.Name
	if !allowed(s) {
		t.Fatalf("case %d: picked %s, which is not usable or not allowed: %v", i, s, cfg.Reasons(s, data[s], true))
	}
	rd := data[s]
	caps := cfg.CapsFor(s)
	if (rd.Short != nil && (rd.Short.Percent >= caps.Short || rd.Short.Percent >= cfg.ShortUnavailableAt)) || (rd.Week != nil && rd.Week.Percent >= caps.Week) {
		t.Fatalf("case %d: picked %s over a cap", i, s)
	}
	for _, m := range members {
		if allowed(m) && deadline(m) < deadline(s) {
			t.Fatalf("case %d: %s resets sooner (%s) than the pick %s (%s)", i, m, deadline(m), s, deadline(s))
		}
	}
}
