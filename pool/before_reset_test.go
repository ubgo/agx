package pool

import (
	"math"
	"strings"
	"testing"
	"time"
)

// Tests of a schedule's week_before_reset (schedule.go): the weekly cap
// of a member whose weekly reset falls before the schedule ends.

// ist is the teammates' zone of these tests.
func ist(t *testing.T) *time.Location { t.Helper(); return zone(t, "Europe/Istanbul") }

// weekendPool is own alice and shared carol, derek and erin (caps 50/70,
// relax to 95 over 24h) with the weekend schedule in Istanbul: short 95
// and week_before_reset 95 on Saturday and Sunday, released 5h before
// Monday, so it ends Sunday 19:00.
func weekendPool(t *testing.T) Config {
	t.Helper()
	c := New([]string{"alice"}, []string{"carol", "derek", "erin"})
	c.Location = ist(t)
	st, en := hours(t, "")
	c.Schedules = []Schedule{{Name: "weekend", Days: days(t, "sat", "sun"), Start: st, End: en, Members: []string{"shared"},
		Caps: ScheduleCaps{Short: fp(95)}, ReleaseBefore: 5 * time.Hour, WeekBeforeReset: fp(95)}}
	return c
}

// weekendEnd is the weekend schedule's end: Sunday 22 Nov 19:00 Istanbul.
func weekendEnd(t *testing.T) time.Time { t.Helper(); return at(ist(t), 2026, 11, 22, 19, 0) }

// reading is an ok reading at short and week percent whose week resets
// at reset.
func reading(short, week float64, reset time.Time) Reading {
	return Reading{Status: StatusOK, Short: &Window{Percent: short}, Week: &Window{Percent: week, ResetsAt: &reset}}
}

// candidateOf finds name in a result.
func candidateOf(t *testing.T, r Result, name string) Candidate {
	t.Helper()
	for _, c := range r.Candidates {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s not in the result", name)
	return Candidate{}
}

// TestBeforeResetCase is the case the rule was made for (made-up
// names): a teammate's account at week 91% on Saturday, its reset 4h45m
// away, its relaxed cap about 90%. Without the rule it is skipped
// although its room is lost at the reset before its teammate is back;
// with it, its cap is 95 at once, so it is usable with 4% of room, in
// the deadline group, and first.
func TestBeforeResetCase(t *testing.T) {
	now := at(ist(t), 2026, 11, 21, 13, 0)
	reset := now.Add(4*time.Hour + 45*time.Minute)
	rd := map[string]Reading{
		"alice": reading(10, 40, now.Add(80*time.Hour)),
		"carol": reading(20, 91, reset),
		"derek": reading(10, 30, now.Add(100*time.Hour)),
	}
	c := weekendPool(t)
	without := c
	without.Schedules = []Schedule{c.Schedules[0]}
	without.Schedules[0].WeekBeforeReset = nil
	r0, err := Select(without, rd, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	carol0 := candidateOf(t, r0, "carol")
	if carol0.Eligible || math.Abs(carol0.Caps.Week-90.05) > 0.01 || !strings.Contains(strings.Join(carol0.Reasons, ";"), "week is 91% (cap 90.1%)") {
		t.Fatalf("without week_before_reset: eligible %v, cap %v, reasons %v", carol0.Eligible, carol0.Caps.Week, carol0.Reasons)
	}

	r, err := Select(c, rd, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	carol := candidateOf(t, r, "carol")
	switch {
	case !carol.Eligible || carol.Caps.Week != 95:
		t.Fatalf("carol: eligible %v, cap %v, reasons %v", carol.Eligible, carol.Caps.Week, carol.Reasons)
	case carol.WeekCapConfigured == nil || *carol.WeekCapConfigured != 70:
		t.Errorf("configured cap %v, want 70", carol.WeekCapConfigured)
	case *carol.Room != 4 || carol.RoomAtReset != nil:
		t.Errorf("room %v, by the reset %v", *carol.Room, carol.RoomAtReset)
	case *carol.NeedsPerHour != 0.84 || !carol.Deadline:
		t.Errorf("needs %v, deadline %v", *carol.NeedsPerHour, carol.Deadline)
	case carol.Timed == nil || carol.Timed.Week == nil || !carol.Timed.Week.BeforeReset || carol.Timed.Week.Name != "weekend" ||
		!carol.Timed.Week.Until.Equal(weekendEnd(t)) || carol.Timed.Short == nil:
		t.Errorf("timed %+v", carol.Timed)
	case strings.Join(carol.Notes, ";") != "weekly cap 95% (schedule weekend: resets before it ends)":
		t.Errorf("notes %q", carol.Notes)
	case r.Selected == nil || r.Selected.Name != "carol":
		t.Errorf("selected %+v", r.Selected)
	case !strings.HasPrefix(r.Selected.Reason, "resets soonest: 4% left under its weekly cap with 4.8h to the reset (0.84%/h"):
		t.Errorf("reason %q", r.Selected.Reason)
	}
	// derek resets Wednesday, after the schedule ends: weekly cap 70.
	if derek := candidateOf(t, r, "derek"); derek.Caps.Week != 70 || derek.Timed.Week != nil || derek.WeekCapConfigured != nil {
		t.Errorf("derek: cap %v, timed %+v", derek.Caps.Week, derek.Timed)
	}
	// The picker's warnings read the same caps.
	if reasons := c.Reasons("carol", rd["carol"], true, now); len(reasons) != 0 {
		t.Errorf("Reasons %v", reasons)
	}
}

// TestBeforeResetCovers: which resets the schedule covers, through
// release_before, at its very end, unknown and passed resets.
func TestBeforeResetCovers(t *testing.T) {
	loc := ist(t)
	now := at(loc, 2026, 11, 21, 13, 0)
	end := weekendEnd(t)
	for _, tc := range []struct {
		name  string
		reset time.Time
		want  bool
	}{
		{"zero reset (unreadable)", time.Time{}, false},
		{"passed", now.Add(-time.Minute), false},
		{"now", now, true},
		{"before the end", end.Add(-time.Minute), true},
		{"exactly at the end", end, true},
		{"just after the end", end.Add(time.Minute), false},
		{"after release_before, before Monday", at(loc, 2026, 11, 22, 23, 0), false},
		{"next week", now.Add(5 * 24 * time.Hour), false},
	} {
		v, src, ok := weekendPool(t).At(now).WeekBeforeReset("carol", tc.reset, now)
		if ok != tc.want || ok && (v != 95 || src.Name != "weekend" || !src.BeforeReset) {
			t.Errorf("%s: %v %+v %v", tc.name, v, src, ok)
		}
	}
	// Outside the schedule, for an own member and a non-member: none.
	friday := at(loc, 2026, 11, 20, 22, 0)
	for _, tc := range []struct {
		name, member string
		now          time.Time
	}{{"friday", "carol", friday}, {"own member", "alice", now}, {"unknown member", "zed", now}} {
		if _, _, ok := weekendPool(t).At(tc.now).WeekBeforeReset(tc.member, tc.now.Add(time.Hour), tc.now); ok {
			t.Errorf("%s: covered", tc.name)
		}
	}
	// A schedule that never ends (every day, all day) covers every reset
	// still to come.
	c := weekendPool(t)
	c.Schedules[0].Days, c.Schedules[0].ReleaseBefore = days(t, "mon", "tue", "wed", "thu", "fri", "sat", "sun"), 0
	if v, src, ok := c.At(now).WeekBeforeReset("carol", now.Add(160*time.Hour), now); !ok || v != 95 || src.Until != nil {
		t.Errorf("never-ending: %v %+v %v", v, src, ok)
	}
}

// TestBeforeResetLevels: a schedule naming the member beats one naming
// its group, the highest value wins at one level (the first on a tie),
// a level whose schedules all end before the reset leaves it to the
// next, and an override that sets the weekly cap wins outright.
func TestBeforeResetLevels(t *testing.T) {
	loc := ist(t)
	now := at(loc, 2026, 11, 21, 13, 0)
	st, en := hours(t, "")
	sat := days(t, "sat")
	sched := func(name string, d [7]bool, members []string, v float64) Schedule {
		return Schedule{Name: name, Days: d, Start: st, End: en, Members: members, WeekBeforeReset: fp(v)}
	}
	sunday := at(loc, 2026, 11, 22, 10, 0)
	for _, tc := range []struct {
		name      string
		schedules []Schedule
		override  *Override
		reset     time.Time
		want      float64
		from      string
	}{
		{"member beats group", []Schedule{sched("group", days(t, "sat", "sun"), []string{"shared"}, 95), sched("carol", days(t, "sat", "sun"), []string{"carol"}, 80)}, nil, sunday, 80, "carol"},
		{"member level ends first: group", []Schedule{sched("group", days(t, "sat", "sun"), []string{"shared"}, 95), sched("carol", sat, []string{"carol"}, 80)}, nil, sunday, 95, "group"},
		{"highest at one level", []Schedule{sched("a", days(t, "sat", "sun"), []string{"shared"}, 85), sched("b", days(t, "sat", "sun"), []string{"shared"}, 92)}, nil, sunday, 92, "b"},
		{"tie: the first", []Schedule{sched("a", days(t, "sat", "sun"), []string{"shared"}, 90), sched("b", days(t, "sat", "sun"), []string{"shared"}, 90)}, nil, sunday, 90, "a"},
		{"the one that covers", []Schedule{sched("a", sat, []string{"shared"}, 99), sched("b", days(t, "sat", "sun"), []string{"shared"}, 90)}, nil, sunday, 90, "b"},
		{"override with week wins", []Schedule{sched("a", days(t, "sat", "sun"), []string{"shared"}, 95)}, &Override{Caps: ScheduleCaps{Week: fp(60)}, Until: now.Add(time.Hour)}, sunday, 0, ""},
		{"override without week leaves it", []Schedule{sched("a", days(t, "sat", "sun"), []string{"shared"}, 95)}, &Override{Caps: ScheduleCaps{Short: fp(60)}, Until: now.Add(time.Hour)}, sunday, 95, "a"},
		{"ended override", []Schedule{sched("a", days(t, "sat", "sun"), []string{"shared"}, 95)}, &Override{Caps: ScheduleCaps{Week: fp(60)}, Until: now.Add(-time.Hour)}, sunday, 95, "a"},
	} {
		c := New([]string{"alice"}, []string{"carol", "derek"})
		c.Location, c.Schedules = loc, tc.schedules
		if tc.override != nil {
			c.Accounts = map[string]Account{"carol": {Override: tc.override}}
		}
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		v, src, ok := c.At(now).WeekBeforeReset("carol", tc.reset, now)
		if ok != (tc.from != "") || v != tc.want || src.Name != tc.from {
			t.Errorf("%s: %v from %q (%v)", tc.name, v, src.Name, ok)
		}
	}
}

// TestBeforeResetCaps: the cap a member gets against its other caps and
// the relax: never lower than the cap it has (an own member at 98 keeps
// it), at once and not ramped, a relax already higher is kept, and a
// schedule's own week cap still applies to members resetting after the
// end.
func TestBeforeResetCaps(t *testing.T) {
	loc := ist(t)
	now := at(loc, 2026, 11, 21, 13, 0)
	for _, tc := range []struct {
		name     string
		mod      func(*Config)
		member   string
		resetIn  time.Duration
		cap      float64
		timed    bool
		relaxing bool
	}{
		{"raised at once, 30h out (outside the relax window)", nil, "carol", 30 * time.Hour, 95, true, false},
		{"after the end: normal cap", nil, "carol", 40 * time.Hour, 70, false, false},
		{"own member keeps 98", func(c *Config) { c.Schedules[0].Members = []string{"own", "shared"} }, "alice", 10 * time.Hour, 98, false, false},
		{"relax higher than the value is kept", func(c *Config) { c.Schedules[0].WeekBeforeReset = fp(80) }, "carol", 2 * time.Hour, 70 + 25*(22.0/24), false, true},
		{"value higher than the relax", func(c *Config) { c.Schedules[0].WeekBeforeReset = fp(90) }, "carol", 12 * time.Hour, 90, true, false},
		{"value at the cap changes nothing", func(c *Config) { c.Schedules[0].WeekBeforeReset = fp(70) }, "carol", 30 * time.Hour, 70, false, false},
		{"schedule week for a later reset", func(c *Config) { c.Schedules[0].Caps.Week = fp(60) }, "carol", 40 * time.Hour, 60, true, false},
		{"schedule week below the value", func(c *Config) { c.Schedules[0].Caps.Week = fp(60) }, "carol", 30 * time.Hour, 95, true, false},
	} {
		c := weekendPool(t)
		if tc.mod != nil {
			tc.mod(&c)
		}
		rd := map[string]Reading{tc.member: reading(10, 50, now.Add(tc.resetIn))}
		r, err := Select(c, rd, Options{Now: now})
		if err != nil {
			t.Fatal(err)
		}
		got := candidateOf(t, r, tc.member)
		before := got.Timed != nil && got.Timed.Week != nil && got.Timed.Week.BeforeReset
		relaxNote := strings.Contains(strings.Join(got.Notes, ";"), "rises to")
		if math.Abs(got.Caps.Week-tc.cap) > 1e-9 || before != (tc.timed && tc.cap != 60) || relaxNote != tc.relaxing {
			t.Errorf("%s: cap %v, before-reset %v, notes %q", tc.name, got.Caps.Week, before, got.Notes)
		}
		if tc.name == "schedule week for a later reset" && (got.Timed == nil || got.Timed.Week == nil || got.Timed.Week.BeforeReset) {
			t.Errorf("%s: timed %+v", tc.name, got.Timed)
		}
	}
}

// TestBeforeResetUrgency: the raised cap counts in the room and the
// urgency while it applies; before the schedule starts it does not
// (Friday evening, a reset on Saturday): only the relax counts there.
func TestBeforeResetUrgency(t *testing.T) {
	loc := ist(t)
	sat := at(loc, 2026, 11, 21, 13, 0)
	r, err := Select(weekendPool(t), map[string]Reading{"carol": reading(10, 50, sat.Add(30*time.Hour))}, Options{Now: sat})
	if err != nil {
		t.Fatal(err)
	}
	c := candidateOf(t, r, "carol")
	if *c.Room != 45 || c.RoomAtReset != nil || *c.NeedsPerHour != 1.5 || c.Deadline {
		t.Errorf("Saturday: room %v / %v, needs %v, deadline %v", *c.Room, c.RoomAtReset, *c.NeedsPerHour, c.Deadline)
	}
	fri := at(loc, 2026, 11, 20, 20, 0)
	reset := at(loc, 2026, 11, 21, 10, 0) // 14h away, inside the relax window
	r, err = Select(weekendPool(t), map[string]Reading{"carol": reading(10, 80, reset)}, Options{Now: fri})
	if err != nil {
		t.Fatal(err)
	}
	c = candidateOf(t, r, "carol")
	// Cap now 70 + 25 × 10/24 ≈ 80.4: usable; room by the reset up to
	// the relax max 95, not more.
	if !c.Eligible || c.Timed != nil || *c.RoomAtReset != 15 {
		t.Errorf("Friday: eligible %v, timed %+v, by the reset %v", c.Eligible, c.Timed, c.RoomAtReset)
	}
}

// TestBeforeResetScoped: a per-model window gets the raised cap when its
// own reset falls before the end, even when the weekly reset does not;
// one without a known reset keeps the normal cap.
func TestBeforeResetScoped(t *testing.T) {
	now := at(ist(t), 2026, 11, 21, 13, 0)
	soon, late := now.Add(10*time.Hour), now.Add(60*time.Hour)
	rd := reading(10, 50, late)
	rd.Scoped = []ScopedWindow{{Name: "fable week", Percent: 85, ResetsAt: &soon}, {Name: "late week", Percent: 60, ResetsAt: &late}, {Name: "unknown week", Percent: 20}}
	r, err := Select(weekendPool(t), map[string]Reading{"carol": rd}, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	c := candidateOf(t, r, "carol")
	if c.Caps.Week != 70 || !c.Eligible {
		t.Fatalf("weekly cap %v, eligible %v %v", c.Caps.Week, c.Eligible, c.Reasons)
	}
	want := map[string]float64{"fable week": 95, "late week": 70, "unknown week": 70}
	for _, s := range c.Scoped {
		if s.Cap != want[s.Name] {
			t.Errorf("%s cap %v, want %v", s.Name, s.Cap, want[s.Name])
		}
	}
	// The fable window at 85 would block it at its relaxed cap (70 +
	// 25 × 14/24 ≈ 84.6) without the rule.
	c0 := weekendPool(t)
	c0.Schedules[0].WeekBeforeReset = nil
	r, _ = Select(c0, map[string]Reading{"carol": rd}, Options{Now: now})
	if candidateOf(t, r, "carol").Eligible {
		t.Error("eligible without week_before_reset")
	}
}

// TestBeforeResetOnly: a pinned run's lower weekly cap is a hard limit
// the schedule cannot raise; without one the pin gets the raised cap.
func TestBeforeResetOnly(t *testing.T) {
	now := at(ist(t), 2026, 11, 21, 13, 0)
	rd := map[string]Reading{"carol": reading(10, 85, now.Add(30*time.Hour)), "derek": reading(10, 85, now.Add(30*time.Hour))}
	c := weekendPool(t)
	c.Schedules[0].Members = []string{"carol", "derek"}
	r, err := Select(c, rd, Options{Now: now, Only: "carol", OnlyCaps: Tighter{Week: fp(65)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateOf(t, r, "carol"); got.Caps.Week != 65 || got.Eligible {
		t.Errorf("limited: cap %v, eligible %v", got.Caps.Week, got.Eligible)
	}
	r, err = Select(c, rd, Options{Now: now, Only: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateOf(t, r, "carol"); got.Caps.Week != 95 || r.Selected == nil {
		t.Errorf("pinned: cap %v, selected %v", got.Caps.Week, r.Selected)
	}
	// The limit is the pin's only: the unpinned pool still raises derek.
	r, _ = Select(c, rd, Options{Now: now})
	if got := candidateOf(t, r, "derek"); got.Caps.Week != 95 {
		t.Errorf("derek after a pin: %v", got.Caps.Week)
	}
	// A pin of a member without values changes nothing.
	cfg := c.At(now)
	cfg.limitBeforeReset("alice", 50)
	if _, ok := cfg.beforeReset["alice"]; ok {
		t.Error("alice got values")
	}
}

// TestBeforeResetSessions: sticky and max_sessions treat the raised
// member as any usable one: a session on it stays, and a full one passes
// the next session on.
func TestBeforeResetSessions(t *testing.T) {
	now := at(ist(t), 2026, 11, 21, 13, 0)
	rd := map[string]Reading{
		"alice": reading(10, 40, now.Add(60*time.Hour)),
		"carol": reading(10, 91, now.Add(5*time.Hour)),
		"derek": reading(10, 60, now.Add(100*time.Hour)),
	}
	c := weekendPool(t)
	r, err := Select(c, rd, Options{Now: now, Current: "carol", Load: map[string]int{"carol": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected == nil || r.Selected.Name != "carol" || !r.Selected.Sticky {
		t.Errorf("sticky: %+v", r.Selected)
	}
	r, _ = Select(c, rd, Options{Now: now, Load: map[string]int{"carol": c.MaxSessions}})
	if r.Selected == nil || r.Selected.Name == "carol" || !strings.Contains(r.Selected.Reason, "carol has 4 other sessions") {
		t.Errorf("full: %+v", r.Selected)
	}
}

// TestBeforeResetEndsAtReset: a raised cap whose schedule ends exactly
// at the reset never makes its excess due earlier than the reset (the
// end is not before the reset).
func TestBeforeResetEndsAtReset(t *testing.T) {
	now := at(ist(t), 2026, 11, 21, 13, 0)
	end := weekendEnd(t)
	r, err := Select(weekendPool(t), map[string]Reading{"carol": reading(10, 60, end)}, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	c := candidateOf(t, r, "carol")
	if c.Caps.Week != 95 || c.window.ends != "" || *c.RoomUntil != end.Sub(now) {
		t.Errorf("cap %v, window %+v", c.Caps.Week, c.window)
	}
}

// TestBeforeResetDST: the schedule's end follows the clock across a
// daylight-saving change. In New York the weekend of 31 Oct 2026 has a
// 25-hour Sunday, so Monday 00:00 is 05:00 UTC, not 04:00: a reset at
// 04:30 UTC is still before the end.
func TestBeforeResetDST(t *testing.T) {
	ny := zone(t, "America/New_York")
	c := weekendPool(t)
	c.Location, c.Schedules[0].ReleaseBefore = ny, 0
	now := at(ny, 2026, 10, 31, 12, 0)
	for _, tc := range []struct {
		reset time.Time
		want  bool
	}{
		{time.Date(2026, 11, 2, 4, 30, 0, 0, time.UTC), true},
		{time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 11, 2, 5, 1, 0, 0, time.UTC), false},
	} {
		if _, _, ok := c.At(now).WeekBeforeReset("carol", tc.reset, now); ok != tc.want {
			t.Errorf("%s: %v", tc.reset, ok)
		}
	}
}

// TestBeforeResetValidate: a percent like the other caps; a schedule may
// set only week_before_reset.
func TestBeforeResetValidate(t *testing.T) {
	for _, v := range []float64{-1, 100.5, math.NaN()} {
		c := weekendPool(t)
		c.Schedules[0].WeekBeforeReset = fp(v)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "week_before_reset") {
			t.Errorf("%v: %v", v, err)
		}
	}
	c := weekendPool(t)
	c.Schedules[0].Caps = ScheduleCaps{}
	if err := c.Validate(); err != nil {
		t.Errorf("only week_before_reset: %v", err)
	}
	c.Schedules[0].WeekBeforeReset = nil
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "week_before_reset") {
		t.Errorf("no caps: %v", err)
	}
	for _, v := range []float64{0, 100} {
		c := weekendPool(t)
		c.Schedules[0].WeekBeforeReset = fp(v)
		if err := c.Validate(); err != nil {
			t.Errorf("%v: %v", v, err)
		}
	}
}
