package pool_test

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// Options.Only pins one member while every rule still applies to it:
// the selection holds that member alone, a block reason leaves nothing
// selected (no other member, no force), and OnlyCaps can only lower its
// caps. team(): alice, bob, carol own (95/98), the rest shared (50/70).

// pinned selects with Only set and fails the test on an error.
func pinned(t *testing.T, cfg pool.Config, readings map[string]pool.Reading, o pool.Options) pool.Result {
	t.Helper()
	o.Now = now
	r, err := pool.Select(cfg, readings, o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestOnlyUsable: the pinned member is selected even when the rules
// would pick another, it is the only candidate, and the selection says
// it was pinned.
func TestOnlyUsable(t *testing.T) {
	readings := map[string]pool.Reading{
		"alice": read(10, 20, 2*day), // the rules' pick: own, soonest reset
		"derek": read(10, 30, 6*day), // shared, outside the window: would wait
	}
	r := pinned(t, team(), readings, pool.Options{Only: "derek"})
	s := r.Selected
	if s == nil || s.Name != "derek" || s.Group != pool.GroupShared || !s.Only || s.Forced || len(s.Warnings) != 0 {
		t.Fatalf("selected %+v", s)
	}
	if !strings.Contains(s.Reason, "caps and the pool's rules still apply") {
		t.Errorf("reason %q", s.Reason)
	}
	if len(r.Candidates) != 1 || r.Candidates[0].Name != "derek" || r.Candidates[0].State != pool.StateReady || r.Candidates[0].Rank != 1 {
		t.Errorf("candidates %+v", r.Candidates)
	}
	if s.Room == nil || *s.Room != 40 {
		t.Errorf("room %v, want the week cap 70 minus 30", s.Room)
	}
	own := pinned(t, team(), readings, pool.Options{Only: "alice"})
	if own.Selected == nil || own.Selected.Name != "alice" || own.Selected.Group != pool.GroupOwn || !own.Selected.Only {
		t.Errorf("own member: %+v", own.Selected)
	}
	if plain := pinned(t, team(), readings, pool.Options{}); plain.Selected.Only {
		t.Error("a selection without Only says it was pinned")
	}
}

// TestOnlyBlocked: each block reason leaves nothing selected, with the
// reason on the one candidate; no other member is picked instead.
func TestOnlyBlocked(t *testing.T) {
	disabled := team()
	disabled.Accounts["derek"] = pool.Account{Enabled: no()}
	nan := read(10, 20, 2*day)
	nan.Week.Percent = math.NaN()
	cases := []struct {
		name    string
		cfg     pool.Config
		reading *pool.Reading
		reason  string
	}{
		{"week at its cap", team(), ptrR(read(10, 70, 2*day)), "week is 70% (cap 70%)"},
		{"5h at its cap", team(), ptrR(read(50, 20, 2*day)), "5h is 50% (cap 50%)"},
		{"limit reached", team(), ptrR(status("limit-reached")), "status is limit-reached"},
		{"no reading", team(), nil, "no usage reading"},
		{"disabled", disabled, ptrR(read(10, 20, 2*day)), "disabled in config"},
		{"unreadable", team(), &nan, "week usage is unreadable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			readings := map[string]pool.Reading{"alice": read(1, 1, 2*day)}
			if c.reading != nil {
				readings["derek"] = *c.reading
			}
			r := pinned(t, c.cfg, readings, pool.Options{Only: "derek"})
			if r.Selected != nil {
				t.Fatalf("selected %+v; a blocked pin selects nothing", r.Selected)
			}
			if len(r.Candidates) != 1 || r.Candidates[0].State != pool.StateBlocked || !slices.Contains(r.Candidates[0].Reasons, c.reason) {
				t.Errorf("candidates %+v, want reason %q", r.Candidates, c.reason)
			}
		})
	}
}

// ptrR returns a pointer to a reading.
func ptrR(r pool.Reading) *pool.Reading { return &r }

// TestOnlyShortCutoff: the pool's 5h cutoff applies to a pinned own
// member whose own 5h cap is above it.
func TestOnlyShortCutoff(t *testing.T) {
	cfg := team()
	cfg.Accounts["alice"] = pool.Account{Short: f(100), Week: f(100)}
	r := pinned(t, cfg, map[string]pool.Reading{"alice": read(99, 10, 2*day)}, pool.Options{Only: "alice"})
	if r.Selected != nil || !slices.Contains(r.Candidates[0].Reasons, "5h is 99% (unavailable at 99%)") {
		t.Errorf("%+v %+v", r.Selected, r.Candidates)
	}
}

// TestOnlyIgnoresConfigForce: a pin outranks the pool's standing force,
// which would ignore the caps it is meant to keep.
func TestOnlyIgnoresConfigForce(t *testing.T) {
	cfg := team()
	cfg.Force = "alice"
	readings := map[string]pool.Reading{"alice": read(1, 1, 2*day), "derek": read(10, 70, 2*day)}
	r := pinned(t, cfg, readings, pool.Options{Only: "derek"})
	if r.Selected != nil {
		t.Errorf("config force picked %+v under a pin", r.Selected)
	}
	if r.Candidates[0].Name != "derek" {
		t.Errorf("candidates %+v", r.Candidates)
	}
}

// TestOnlyRefused: a pin with a force, a non-member, or caps without a
// pin are errors, not picks.
func TestOnlyRefused(t *testing.T) {
	readings := map[string]pool.Reading{"alice": read(1, 1, 2*day)}
	for _, c := range []struct {
		name string
		o    pool.Options
		want error
	}{
		{"only and force", pool.Options{Only: "alice", Force: "bob"}, pool.ErrOnlyAndForce},
		{"not a member", pool.Options{Only: "zoe"}, pool.ErrOnlyNotMember},
		{"caps without only", pool.Options{OnlyCaps: pool.Tighter{Week: f(10)}}, pool.ErrCapsWithoutOnly},
		{"higher cap", pool.Options{Only: "derek", OnlyCaps: pool.Tighter{Week: f(71)}}, pool.ErrCapRaised},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.o.Now = now
			if _, err := pool.Select(team(), readings, c.o); !errors.Is(err, c.want) {
				t.Errorf("err %v, want %v", err, c.want)
			}
		})
	}
	if _, err := pool.Select(team(), readings, pool.Options{Now: now, Only: "zoe"}); err == nil || !strings.Contains(err.Error(), "zoe") {
		t.Errorf("non-member error does not name it: %v", err)
	}
}

// TestOnlyTighterCaps: OnlyCaps lowers the pinned member's caps (a value
// equal to the configured cap is fine), so usage under the configured
// cap but at the lowered one blocks; the caller's config is untouched.
func TestOnlyTighterCaps(t *testing.T) {
	cfg := team()
	readings := map[string]pool.Reading{"derek": read(30, 60, 2*day)}
	if r := pinned(t, cfg, readings, pool.Options{Only: "derek"}); r.Selected == nil {
		t.Fatal("60% is under the configured week cap 70")
	}
	r := pinned(t, cfg, readings, pool.Options{Only: "derek", OnlyCaps: pool.Tighter{Week: f(60)}})
	if r.Selected != nil || !slices.Contains(r.Candidates[0].Reasons, "week is 60% (cap 60%)") {
		t.Errorf("lowered week: %+v %+v", r.Selected, r.Candidates)
	}
	if got := r.Candidates[0].Caps; got != (pool.Caps{Short: 50, Week: 60}) {
		t.Errorf("caps %+v", got)
	}
	r = pinned(t, cfg, readings, pool.Options{Only: "derek", OnlyCaps: pool.Tighter{Short: f(30)}})
	if r.Selected != nil || !slices.Contains(r.Candidates[0].Reasons, "5h is 30% (cap 30%)") {
		t.Errorf("lowered 5h: %+v %+v", r.Selected, r.Candidates)
	}
	r = pinned(t, cfg, readings, pool.Options{Only: "derek", OnlyCaps: pool.Tighter{Short: f(50), Week: f(70)}})
	if r.Selected == nil || r.Candidates[0].Room == nil || *r.Candidates[0].Room != 10 {
		t.Errorf("caps equal to the configured ones: %+v", r.Candidates)
	}
	if _, ok := cfg.Accounts["derek"]; ok || cfg.CapsFor("derek") != (pool.Caps{Short: 50, Week: 70}) {
		t.Errorf("the caller's config changed: %+v", cfg.Accounts)
	}
}

// TestTightened: the effective caps, every refusal, and the own entry's
// caps as the ceiling.
func TestTightened(t *testing.T) {
	cfg := team()
	got, err := cfg.Tightened("alice", pool.Tighter{Short: f(80)})
	if err != nil || got != (pool.Caps{Short: 80, Week: 98}) {
		t.Errorf("alice: %+v %v", got, err)
	}
	if got, err := cfg.Tightened("derek", pool.Tighter{}); err != nil || got != cfg.CapsFor("derek") {
		t.Errorf("no tighter caps: %+v %v", got, err)
	}
	for _, c := range []struct {
		name string
		t    pool.Tighter
		want string
	}{
		{"5h above", pool.Tighter{Short: f(96)}, "5h 96% is above alice's 5h cap 95%"},
		{"week above", pool.Tighter{Week: f(99)}, "week 99% is above alice's week cap 98%"},
		{"over 100", pool.Tighter{Week: f(101)}, "outside 0..100"},
		{"negative", pool.Tighter{Short: f(-1)}, "outside 0..100"},
		{"NaN", pool.Tighter{Week: f(math.NaN())}, "outside 0..100"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := cfg.Tightened("alice", c.t); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err %v, want %q", err, c.want)
			}
		})
	}
	if _, err := cfg.Tightened("alice", pool.Tighter{Week: f(99)}); !errors.Is(err, pool.ErrCapRaised) {
		t.Errorf("not ErrCapRaised: %v", err)
	}
	if (pool.Tighter{}).Given() || !(pool.Tighter{Week: f(1)}).Given() || !(pool.Tighter{Short: f(1)}).Given() {
		t.Error("Given")
	}
}

// TestOnlySharedWithOwnUsable: a pinned shared member is ready even
// while own members are usable and its reset is outside the borrow
// window: the pin, not the window, decides who is a candidate.
func TestOnlySharedWithOwnUsable(t *testing.T) {
	readings := map[string]pool.Reading{"alice": read(1, 1, time.Hour), "erin": read(1, 1, 6*day)}
	r := pinned(t, team(), readings, pool.Options{Only: "erin"})
	if r.Selected == nil || r.Selected.Name != "erin" || r.Candidates[0].State != pool.StateReady {
		t.Errorf("%+v %+v", r.Selected, r.Candidates)
	}
}
