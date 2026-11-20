package pool_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// expiring is team() under the expiring strategy at its defaults.
func expiring() pool.Config {
	c := team()
	c.Strategy = pool.StrategyExpiring
	return c
}

// pick runs Select and returns the selection's name ("" for none).
func pick(t *testing.T, cfg pool.Config, readings map[string]pool.Reading, o pool.Options) (string, pool.Result) {
	t.Helper()
	if o.Now.IsZero() {
		o.Now = now
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	r, err := pool.Select(cfg, readings, o)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if r.Selected == nil {
		return "", r
	}
	return r.Selected.Name, r
}

// TestRelaxedWeek: the weekly cap rises in a straight line over the last
// window before the reset, and nowhere else.
func TestRelaxedWeek(t *testing.T) {
	cases := []struct {
		name          string
		cap, max      float64
		window, until time.Duration
		want          float64
	}{
		{"outside the window", 70, 95, day, 30 * time.Hour, 70},
		{"at the window's start", 70, 95, day, day, 70},
		{"halfway", 70, 95, day, 12 * time.Hour, 82.5},
		{"at the reset", 70, 95, day, 0, 95},
		{"a passed reset is stale data", 70, 95, day, -time.Minute, 70},
		{"window off", 70, 95, 0, time.Hour, 70},
		{"max below the cap keeps the cap", 98, 95, day, time.Hour, 98},
		{"max equal to the cap", 70, 70, day, time.Hour, 70},
		{"six hours left", 70, 95, day, 6 * time.Hour, 88.75},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pool.RelaxedWeek(c.cap, c.max, c.window, c.until); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("RelaxedWeek = %v, want %v", got, c.want)
			}
		})
	}
}

// TestExpiringRelaxCaps: a shared member over its weekly cap is usable
// again inside the relax window, up to the relaxed cap; the 5-hour cap
// never relaxes; own members keep their higher cap; per-member and
// per-group relax maxima win.
func TestExpiringRelaxCaps(t *testing.T) {
	cases := []struct {
		name    string
		cfg     func() pool.Config
		reading pool.Reading
		usable  bool
		weekCap float64
	}{
		{"week 72 with 12h left: cap 82.5", expiring, read(10, 72, 12*time.Hour), true, 82.5},
		{"week 72 with 30h left: cap 70", expiring, read(10, 72, 30*time.Hour), false, 70},
		{"week 83 with 12h left: over 82.5", expiring, read(10, 83, 12*time.Hour), false, 82.5},
		{"5h over its cap is still blocked at the reset", expiring, read(52, 36, 27*time.Minute), false, 94.53},
		{"member relax max at its cap: never relaxed", func() pool.Config {
			c := expiring()
			c.Accounts["derek"] = pool.Account{WeekRelaxMax: f(70)}
			return c
		}, read(10, 72, time.Hour), false, 70},
		{"group relax max", func() pool.Config {
			c := expiring()
			c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupShared: {WeekRelaxMax: f(80)}}
			return c
		}, read(10, 72, 12*time.Hour), true, 75},
		{"member beats group", func() pool.Config {
			c := expiring()
			c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupShared: {WeekRelaxMax: f(80)}}
			c.Accounts["derek"] = pool.Account{WeekRelaxMax: f(90)}
			return c
		}, read(10, 72, 12*time.Hour), true, 80},
		{"window off", func() pool.Config {
			c := expiring()
			c.WeekRelaxWindow = 0
			return c
		}, read(10, 72, time.Hour), false, 70},
		{"deadline strategy never relaxes", team, read(10, 72, time.Hour), false, 70},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			readings := map[string]pool.Reading{"derek": c.reading}
			_, r := pick(t, c.cfg(), readings, pool.Options{})
			cand := candidateOf(t, r, "derek")
			if cand.Eligible != c.usable {
				t.Errorf("eligible = %v, want %v (reasons %v)", cand.Eligible, c.usable, cand.Reasons)
			}
			if math.Abs(cand.Caps.Week-c.weekCap) > 0.01 {
				t.Errorf("week cap = %v, want %v", cand.Caps.Week, c.weekCap)
			}
			relaxed := cand.WeekCapConfigured != nil
			if relaxed != (c.weekCap != 70) {
				t.Errorf("week_cap_configured = %v, relaxed cap %v", cand.WeekCapConfigured, c.weekCap)
			}
			if relaxed && !strings.Contains(strings.Join(cand.Notes, "; "), "rises to") {
				t.Errorf("notes %v do not explain the relaxed cap", cand.Notes)
			}
		})
	}
}

// TestExpiringOwnKeepsHigherCap: an own member capped at 98 is never
// lowered to the relax max of 95.
func TestExpiringOwnKeepsHigherCap(t *testing.T) {
	_, r := pick(t, expiring(), map[string]pool.Reading{"alice": read(10, 96, time.Hour)}, pool.Options{})
	if c := candidateOf(t, r, "alice"); !c.Eligible || c.Caps.Week != 98 || c.WeekCapConfigured != nil {
		t.Errorf("alice: eligible %v, cap %v, configured %v", c.Eligible, c.Caps.Week, c.WeekCapConfigured)
	}
}

// TestExpiringUrgencyOrder: the pool is sorted by room ÷ hours to the
// reset, highest first; an unknown reset sorts last; ties keep config
// order.
func TestExpiringUrgencyOrder(t *testing.T) {
	cases := []struct {
		name     string
		readings map[string]pool.Reading
		order    []string
	}{
		{"shared with 30% in 20h beats own with 90% in 6 days", map[string]pool.Reading{
			"alice": read(0, 8, 144*time.Hour), "derek": read(0, 40, 20*time.Hour),
		}, []string{"derek", "alice"}},
		{"own with more room per hour beats a shared reset sooner", map[string]pool.Reading{
			"alice": read(0, 8, 100*time.Hour), "derek": read(0, 60, 40*time.Hour),
		}, []string{"alice", "derek"}},
		{"soonest reset is not first when it has little room", map[string]pool.Reading{
			"alice": read(0, 90, 10*time.Hour), "bob": read(0, 10, 50*time.Hour),
		}, []string{"bob", "alice"}},
		{"unknown reset sorts last", map[string]pool.Reading{
			"alice": {Status: pool.StatusOK, Week: &pool.Window{Percent: 0}}, "bob": read(0, 90, 100*time.Hour),
		}, []string{"bob", "alice"}},
		{"ties keep config order", map[string]pool.Reading{
			"bob": read(0, 48, 50*time.Hour), "alice": read(0, 48, 50*time.Hour),
		}, []string{"alice", "bob"}},
		{"minutes to the reset count as a quarter hour", map[string]pool.Reading{
			"alice": read(0, 97, time.Minute), "bob": read(0, 0, 24*time.Hour),
		}, []string{"bob", "alice"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, r := pick(t, expiring(), c.readings, pool.Options{})
			var got []string
			for _, cand := range r.Candidates {
				if cand.State == pool.StateReady {
					got = append(got, cand.Name)
				}
			}
			if strings.Join(got, ",") != strings.Join(c.order, ",") {
				t.Errorf("order %v, want %v", got, c.order)
			}
		})
	}
}

// TestExpiringUrgencyValue: the candidate shows its urgency in percent
// per hour, from the relaxed cap.
func TestExpiringUrgencyValue(t *testing.T) {
	_, r := pick(t, expiring(), map[string]pool.Reading{"derek": read(0, 40, 12*time.Hour)}, pool.Options{})
	c := candidateOf(t, r, "derek")
	if c.Urgency == nil || *c.Urgency != 3.54 { // (82.5 - 40) / 12
		t.Errorf("urgency = %v, want 3.54", c.Urgency)
	}
}

// sessions is a pool where alice, bob and derek are usable, derek the
// most urgent, then alice, then bob; erin waits (outside the window).
func sessions() map[string]pool.Reading {
	return map[string]pool.Reading{
		"derek": read(10, 40, 20*time.Hour),
		"alice": read(10, 20, 100*time.Hour),
		"bob":   read(10, 20, 150*time.Hour),
		"erin":  read(10, 10, 100*time.Hour),
	}
}

// TestExpiringAssign: a session's own pick: sticky, max sessions, the
// overflow to waiting members, the least loaded when every member is
// full.
func TestExpiringAssign(t *testing.T) {
	cases := []struct {
		name   string
		cfg    func() pool.Config
		o      pool.Options
		want   string
		sticky bool
		reason string
	}{
		{"most urgent first", expiring, pool.Options{}, "derek", false, "most room to lose"},
		{"sticky keeps a ready member", expiring, pool.Options{Current: "bob"}, "bob", true, "sticky"},
		{"sticky off", func() pool.Config { c := expiring(); c.Sticky = false; return c }, pool.Options{Current: "bob"}, "derek", false, ""},
		{"a waiting member is not kept", expiring, pool.Options{Current: "erin"}, "derek", false, ""},
		{"a blocked member is not kept", expiring, pool.Options{Current: "carol"}, "derek", false, ""},
		{"a member outside the pool is not kept", expiring, pool.Options{Current: "zed"}, "derek", false, ""},
		{"kept while fewer than max_sessions older sessions hold it", expiring,
			pool.Options{Current: "bob", CurrentAhead: 2, Load: map[string]int{"bob": 5}}, "bob", true, ""},
		{"left when max_sessions older sessions hold it", expiring,
			pool.Options{Current: "bob", CurrentAhead: 3, Load: map[string]int{"bob": 3}}, "derek", false, ""},
		{"a full member is passed over", expiring,
			pool.Options{Load: map[string]int{"derek": 3}}, "alice", false, "derek has 3 other sessions"},
		{"under max_sessions is not full", expiring,
			pool.Options{Load: map[string]int{"derek": 2}}, "derek", false, ""},
		{"full ready members overflow to a waiting one", expiring,
			pool.Options{Load: map[string]int{"derek": 3, "alice": 3, "bob": 3}}, "erin", false, "borrow window"},
		{"every member full: the least loaded ready one", expiring,
			pool.Options{Load: map[string]int{"derek": 5, "alice": 4, "bob": 6, "erin": 3}}, "alice", false, "least loaded"},
		{"max_sessions 0 is no limit", func() pool.Config { c := expiring(); c.MaxSessions = 0; return c },
			pool.Options{Load: map[string]int{"derek": 50}}, "derek", false, ""},
		{"max_sessions 0 keeps a sticky member at any load", func() pool.Config { c := expiring(); c.MaxSessions = 0; return c },
			pool.Options{Current: "bob", CurrentAhead: 40}, "bob", true, ""},
		{"sticky leaves for a member at least sticky_leave_ratio times as urgent", func() pool.Config { c := expiring(); c.StickyLeaveRatio = 2; return c },
			pool.Options{Current: "bob"}, "derek", false, "left bob: derek is at least 2 times as urgent"},
		{"sticky stays below sticky_leave_ratio", func() pool.Config { c := expiring(); c.StickyLeaveRatio = 4; return c },
			pool.Options{Current: "bob"}, "bob", true, "sticky"},
		{"sticky on the most urgent member stays", func() pool.Config { c := expiring(); c.StickyLeaveRatio = 1; return c },
			pool.Options{Current: "derek"}, "derek", true, "sticky"},
		{"deadline ignores load and current", team,
			pool.Options{Current: "bob", Load: map[string]int{"derek": 9}}, "derek", false, "shared account resets within"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, r := pick(t, c.cfg(), sessions(), c.o)
			if got != c.want {
				t.Fatalf("picked %q, want %q (%s)", got, c.want, r.Selected.Reason)
			}
			if r.Selected.Sticky != c.sticky {
				t.Errorf("sticky = %v, want %v", r.Selected.Sticky, c.sticky)
			}
			if !strings.Contains(r.Selected.Reason, c.reason) {
				t.Errorf("reason %q does not say %q", r.Selected.Reason, c.reason)
			}
			if r.Selected.Load != c.o.Load[got] {
				t.Errorf("selection load %d, want %d", r.Selected.Load, c.o.Load[got])
			}
		})
	}
}

// TestExpiringOnlyKeepsTighterWeek: a --pool-only weekly cap is a hard
// limit, never relaxed near the reset; the pin's reason stays.
func TestExpiringOnlyKeepsTighterWeek(t *testing.T) {
	readings := map[string]pool.Reading{"derek": read(10, 66, time.Hour)}
	got, r := pick(t, expiring(), readings, pool.Options{Only: "derek", OnlyCaps: pool.Tighter{Week: f(65)}})
	if got != "" {
		t.Errorf("picked %q over its one-run weekly cap 65 (caps %v)", got, r.Candidates[0].Caps)
	}
	got, r = pick(t, expiring(), readings, pool.Options{Only: "derek"})
	if got != "derek" || !r.Selected.Only {
		t.Errorf("pinned without a tighter cap: picked %q (only %v); the relaxed cap applies", got, r.Selected != nil && r.Selected.Only)
	}
}

// TestGroupCaps: a member's caps are the defaults, then its group's,
// then its own entry, under both strategies.
func TestGroupCaps(t *testing.T) {
	c := pool.New([]string{"alice", "bob"}, []string{"derek", "erin"})
	c.Groups = map[pool.Group]pool.GroupCaps{
		pool.GroupOwn:    {Short: f(95), Week: f(98)},
		pool.GroupShared: {Week: f(60)},
	}
	c.Accounts = map[string]pool.Account{"bob": {Week: f(90)}, "erin": {Short: f(40)}}
	want := map[string]pool.Caps{
		"alice": {Short: 95, Week: 98}, "bob": {Short: 95, Week: 90},
		"derek": {Short: 50, Week: 60}, "erin": {Short: 40, Week: 60},
	}
	for name, w := range want {
		if got := c.CapsFor(name); got != w {
			t.Errorf("%s caps %v, want %v", name, got, w)
		}
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestExpiringValidate: the strategy and its knobs are checked.
func TestExpiringValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*pool.Config)
		want string
	}{
		{"unknown strategy", func(c *pool.Config) { c.Strategy = "soonest" }, `strategy "soonest" is not one of`},
		{"negative relax window", func(c *pool.Config) { c.WeekRelaxWindow = -time.Hour }, "week_relax_window is negative"},
		{"negative max sessions", func(c *pool.Config) { c.MaxSessions = -1 }, "max_sessions is negative"},
		{"negative leave ratio", func(c *pool.Config) { c.StickyLeaveRatio = -1 }, "sticky_leave_ratio -1 is not a number of 0 or more"},
		{"infinite leave ratio", func(c *pool.Config) { c.StickyLeaveRatio = math.Inf(1) }, "sticky_leave_ratio +Inf"},
		{"relax max over 100", func(c *pool.Config) { c.WeekRelaxMax = 101 }, "defaults.week_relax_max 101 is outside 0..100"},
		{"relax max NaN", func(c *pool.Config) { c.WeekRelaxMax = math.NaN() }, "defaults.week_relax_max NaN is outside 0..100"},
		{"member relax max", func(c *pool.Config) { c.Accounts["alice"] = pool.Account{WeekRelaxMax: f(-1)} }, "accounts.alice.week_relax_max -1"},
		{"group cap", func(c *pool.Config) {
			c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupShared: {Short: f(200)}}
		}, "shared_caps.short 200 is outside 0..100"},
		{"group relax max", func(c *pool.Config) {
			c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupOwn: {WeekRelaxMax: f(120)}}
		}, "own_caps.week_relax_max 120"},
		{"group that is not own or shared", func(c *pool.Config) {
			c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupUnlisted: {Short: f(10)}}
		}, `group "unlisted" has caps`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := expiring()
			c.edit(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Validate = %v, want %q", err, c.want)
			}
		})
	}
	if err := expiring().Validate(); err != nil {
		t.Errorf("defaults: %v", err)
	}
	if pool.New(nil, nil).Strategy != pool.StrategyDeadline {
		t.Error("New's strategy is not deadline")
	}
}

// TestExpiringDefaults pins the defaults the simulator chose.
func TestExpiringDefaults(t *testing.T) {
	c := pool.New(nil, nil)
	if c.WeekRelaxWindow != 24*time.Hour || c.WeekRelaxMax != 95 || c.MaxSessions != 3 || !c.Sticky || c.StickyLeaveRatio != 0 {
		t.Errorf("defaults: window %s, max %v, sessions %d, sticky %v", c.WeekRelaxWindow, c.WeekRelaxMax, c.MaxSessions, c.Sticky)
	}
}

// TestExpiringJSON: the report's new fields encode.
func TestExpiringJSON(t *testing.T) {
	_, r := pick(t, expiring(), sessions(), pool.Options{Current: "bob", Load: map[string]int{"bob": 1}})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"strategy":"expiring"`, `"sticky":true`, `"urgency":`, `"load":1`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON lacks %s: %s", want, b)
		}
	}
}

// snapshotAt is a real pool cache read of 2026-11-20 (made-up names).
var snapshotAt = time.Date(2026, 11, 20, 7, 39, 52, 0, time.UTC)

// snap is one member of that read: 5-hour (-1 = no window), week and
// hours to the weekly reset.
func snap(short, week, hours float64) pool.Reading {
	at := snapshotAt.Add(time.Duration(hours * float64(time.Hour)))
	r := pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: week, ResetsAt: &at}}
	if short >= 0 {
		end := snapshotAt.Add(2 * time.Hour)
		r.Short = &pool.Window{Percent: short, ResetsAt: &end}
	}
	return r
}

// snapshotPool is that pool: three own (95/98), nine shared.
func snapshotPool(s pool.Strategy) (pool.Config, map[string]pool.Reading) {
	c := pool.New([]string{"ava", "ben", "cara"}, []string{"dan", "eli", "fay", "gus", "hal", "ivy", "jon", "kim", "lea"})
	c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupOwn: {Short: f(95), Week: f(98)}}
	c.Strategy = s
	return c, map[string]pool.Reading{
		"ava": snap(-1, 52, 118.3), "ben": snap(0, 0, 154.3), "cara": snap(56, 98, 20.3),
		"dan": snap(4, 7, 128.3), "eli": snap(6, 7, 115.3), "fay": snap(12, 19, 49.3),
		"gus": snap(52, 36, 0.3), "hal": snap(46, 48, 43.3), "ivy": snap(66, 70, 4.3),
		"jon": snap(15, 90, 19.3), "kim": snap(2, 73, 88.3), "lea": snap(3, 4, 148.3),
	}
}

// TestSnapshotShortBlocksNearReset is the case that started the expiring strategy: a
// shared account 27 minutes from its weekly reset with 36% used, skipped
// for its 5-hour 52% (cap 50). The 5-hour cap still blocks it under the
// expiring strategy (that window outlives the weekly reset and is the
// teammate's working time; 27 minutes could not have spent its 34% of
// weekly room anyway, the 5-hour cap allows about 6% of a week per
// window); its weekly cap is relaxed to about 94.7% and the reason says the
// 5-hour cap is what blocks it.
func TestSnapshotShortBlocksNearReset(t *testing.T) {
	for _, s := range []pool.Strategy{pool.StrategyDeadline, pool.StrategyExpiring} {
		cfg, readings := snapshotPool(s)
		_, r := pick(t, cfg, readings, pool.Options{Now: snapshotAt})
		gus := candidateOf(t, r, "gus")
		if gus.Eligible || strings.Join(gus.Reasons, "; ") != "5h is 52% (cap 50%)" {
			t.Errorf("%s: gus eligible %v, reasons %v", s, gus.Eligible, gus.Reasons)
		}
	}
	// A day earlier, with its 5-hour window idle, the expiring strategy
	// puts it first: 34% to lose in 24 hours beats every other member.
	cfg, readings := snapshotPool(pool.StrategyExpiring)
	readings["gus"] = snap(-1, 36, 24.3)
	got, r := pick(t, cfg, readings, pool.Options{Now: snapshotAt})
	if got != "gus" {
		t.Errorf("a day before its reset: picked %s (%s), want gus", got, r.Selected.Reason)
	}
}

// TestSnapshotRelaxAfterShortWindow: a shared account 4.3 hours from its reset at week
// 70% (its cap) and 5-hour 66%. Today's rules block it twice; once its
// 5-hour window has ended the expiring strategy's relaxed weekly cap
// (90.5% at 4.3 hours) makes it usable, and it goes first: its room is
// lost in hours.
func TestSnapshotRelaxAfterShortWindow(t *testing.T) {
	cfg, readings := snapshotPool(pool.StrategyExpiring)
	_, r := pick(t, cfg, readings, pool.Options{Now: snapshotAt})
	ivy := candidateOf(t, r, "ivy")
	if ivy.Eligible || strings.Join(ivy.Reasons, "; ") != "5h is 66% (cap 50%)" {
		t.Errorf("ivy now: eligible %v, reasons %v (the week is under its relaxed cap)", ivy.Eligible, ivy.Reasons)
	}
	readings["ivy"] = snap(-1, 70, 4.3)
	got, r := pick(t, cfg, readings, pool.Options{Now: snapshotAt})
	if got != "ivy" {
		t.Errorf("5-hour window over: picked %s (%s), want ivy", got, r.Selected.Reason)
	}
	deadline, dr := snapshotPool(pool.StrategyDeadline)
	dr["ivy"] = snap(-1, 70, 4.3)
	_, res := pick(t, deadline, dr, pool.Options{Now: snapshotAt})
	if c := candidateOf(t, res, "ivy"); c.Eligible {
		t.Error("deadline: ivy at week 70% is usable; its cap is 70")
	}
}

// TestSnapshotSpread is the other real symptom: ten sessions all on one
// shared account while an own account sat at 0%. Under the expiring
// strategy ten sessions asking in turn spread over the most urgent
// members, at most three each.
func TestSnapshotSpread(t *testing.T) {
	cfg, readings := snapshotPool(pool.StrategyExpiring)
	load := map[string]int{}
	for range 10 {
		got, _ := pick(t, cfg, readings, pool.Options{Now: snapshotAt, Load: load})
		load[got]++
	}
	if len(load) < 4 {
		t.Errorf("10 sessions on %d members: %v", len(load), load)
	}
	for m, n := range load {
		if n > cfg.MaxSessions {
			t.Errorf("%s has %d sessions (max %d)", m, n, cfg.MaxSessions)
		}
	}
}
