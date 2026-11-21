package pool_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// The order's rules of v0.4.20, one table per rule: the room counted up
// to the cap at the reset, the deadline group and its edges, the rate the
// report shows, and the fixes of the interaction review (sticky leaving
// for the member it would really go to, per-model windows in the room
// and the reason, timed caps at the reset, unknown resets after a full
// member, picker warnings at now).

// ready is the ready members of r in rank order.
func ready(r pool.Result) []string { return readyOrder(r) }

// deadlineCfg is expiring() with the deadline group's values set.
func deadlineCfg(window time.Duration, minRoom float64) pool.Config {
	c := expiring()
	c.DeadlineWindow, c.DeadlineMinRoom = window, minRoom
	return c
}

// TestDeadlineGroupEdges: a member joins the deadline group with its
// reset at or within deadline_window and at least deadline_min_room of
// room at the reset; never under the deadline order, with the window at
// 0, a passed or unknown reset, or when it is blocked.
func TestDeadlineGroupEdges(t *testing.T) {
	cases := []struct {
		name    string
		cfg     pool.Config
		reading pool.Reading
		in      bool
	}{
		{"inside the window with room", deadlineCfg(day, 2), read(0, 60, 10*time.Hour), true},
		{"reset exactly at the window's end", deadlineCfg(day, 2), read(0, 60, day), true},
		{"reset a minute past the window", deadlineCfg(day, 2), read(0, 60, day+time.Minute), false},
		// An hour before the reset the cap is 93.96: 93% is usable with
		// 2% of room up to 95 at the reset.
		{"room exactly the minimum", deadlineCfg(day, 2), read(0, 93, time.Hour), true},
		{"room just under the minimum", deadlineCfg(day, 2), read(0, 93.01, time.Hour), false},
		{"minimum 0 takes any room", deadlineCfg(day, 0), read(0, 93.9, time.Hour), true},
		{"window 0 is off", deadlineCfg(0, 2), read(0, 60, time.Hour), false},
		{"window 0 is off even at the reset", deadlineCfg(0, 2), read(0, 60, 0), false},
		{"reset right now", deadlineCfg(day, 2), read(0, 60, 0), true},
		{"passed reset", deadlineCfg(day, 2), read(0, 10, -time.Hour), false},
		{"unknown reset", deadlineCfg(day, 2), pool.Reading{Status: pool.StatusOK, Week: &pool.Window{Percent: 10}}, false},
		{"blocked member", deadlineCfg(day, 2), read(0, 96, time.Hour), false},
		{"the deadline order has no group", func() pool.Config { c := deadlineCfg(day, 2); c.Order = pool.OrderDeadline; return c }(), read(0, 60, time.Hour), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, r := pick(t, c.cfg, map[string]pool.Reading{"derek": c.reading}, pool.Options{})
			if got := candidateOf(t, r, "derek"); got.Deadline != c.in {
				t.Errorf("deadline %v, want %v (room %v at reset %v, reasons %v)", got.Deadline, c.in, got.Room, got.RoomAtReset, got.Reasons)
			}
		})
	}
}

// TestDeadlineGroupOrder: the group goes first, soonest reset first,
// whatever the urgency; ties keep config order; the rest follow by
// urgency; a waiting member in the group stays behind every ready one.
func TestDeadlineGroupOrder(t *testing.T) {
	cases := []struct {
		name     string
		cfg      pool.Config
		readings map[string]pool.Reading
		order    []string
	}{
		{"soonest reset first inside the group", expiring(), map[string]pool.Reading{
			"derek": read(0, 60, 20*time.Hour), "erin": read(0, 10, 5*time.Hour), "frank": read(0, 50, 12*time.Hour),
		}, []string{"erin", "frank", "derek"}},
		{"the group before a more urgent member outside it", expiring(), map[string]pool.Reading{
			"derek": read(0, 70, 20*time.Hour), "alice": read(0, 0, 25*time.Hour),
		}, []string{"derek", "alice"}},
		{"a tie keeps config order", expiring(), map[string]pool.Reading{
			"erin": read(0, 50, 10*time.Hour), "derek": read(0, 80, 10*time.Hour),
		}, []string{"derek", "erin"}},
		// derek: 95% ÷ 20h = 4.75%/h; erin: 15% ÷ 5h = 3%/h.
		{"inside the group the sooner reset leads", expiring(), map[string]pool.Reading{
			"derek": read(0, 0, 20*time.Hour), "erin": read(0, 80, 5*time.Hour),
		}, []string{"erin", "derek"}},
		{"outside the group the more urgent leads", deadlineCfg(day, 100), map[string]pool.Reading{
			"derek": read(0, 0, 20*time.Hour), "erin": read(0, 80, 5*time.Hour),
		}, []string{"derek", "erin"}},
		// derek: 1.5% to 95 in an hour, under the 2% minimum.
		{"a scrap inside the window ranks by urgency", expiring(), map[string]pool.Reading{
			"derek": read(0, 93.5, time.Hour), "alice": read(0, 0, 30*time.Hour),
		}, []string{"alice", "derek"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, r := pick(t, c.cfg, c.readings, pool.Options{})
			if got := ready(r); !slices.Equal(got, c.order) {
				t.Errorf("order %v, want %v", got, c.order)
			}
		})
	}
	// With the borrow window off, a shared member in the group waits
	// behind the usable own member; it still leads the waiting list.
	c := expiring()
	c.BorrowWindow = 0
	_, r := pick(t, c, map[string]pool.Reading{"alice": read(0, 10, 100*time.Hour), "derek": read(0, 60, 5*time.Hour), "erin": read(0, 10, 100*time.Hour)}, pool.Options{})
	if got := ready(r); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("ready %v, want alice alone", got)
	}
	if d := candidateOf(t, r, "derek"); d.State != pool.StateWaiting || !d.Deadline || r.Candidates[1].Name != "derek" {
		t.Errorf("derek %s deadline %v at %d; want waiting, in the group, first of the waiting", d.State, d.Deadline, slices.IndexFunc(r.Candidates, func(x pool.Candidate) bool { return x.Name == "derek" }))
	}
}

// TestNeedsRate: the rate the report shows is the room ÷ hours, never
// weighed by the capacity (which only the order's key carries) and never
// 0 while there is room before a known reset.
func TestNeedsRate(t *testing.T) {
	c := expiring()
	c.Accounts["alice"] = pool.Account{Short: f(95), Week: f(98), Capacity: f(20)}
	_, r := pick(t, c, map[string]pool.Reading{"alice": read(0, 88, 10*time.Hour)}, pool.Options{})
	a := candidateOf(t, r, "alice")
	if a.NeedsPerHour == nil || *a.NeedsPerHour != 1 || a.Urgency == nil || *a.Urgency != 20 {
		t.Errorf("needs %v, urgency %v; want 1 and 20", a.NeedsPerHour, a.Urgency)
	}
	if !strings.Contains(r.Selected.Reason, "(1%/h to use it all)") {
		t.Errorf("reason %q shows the capacity-weighed rate", r.Selected.Reason)
	}
	_, r = pick(t, expiring(), map[string]pool.Reading{"alice": read(0, 97.5, 160*time.Hour)}, pool.Options{})
	a = candidateOf(t, r, "alice")
	if a.NeedsPerHour == nil || *a.NeedsPerHour != 0.01 || !strings.Contains(r.Selected.Reason, "(0.01%/h to use it all)") {
		t.Errorf("0.5%% over 160h: needs %v, reason %q; want 0.01", a.NeedsPerHour, r.Selected.Reason)
	}
}

// TestStickyLeavesForItsTarget: a sticky session leaves only for the
// member it would go to (the first ready one that can take it), when
// that member is sticky_leave_ratio times as urgent; a full leader is no
// reason to leave.
func TestStickyLeavesForItsTarget(t *testing.T) {
	readings := map[string]pool.Reading{
		"alice": read(0, 50, 100*time.Hour), // own: 48% ÷ 100h = 0.48
		"bob":   read(0, 60, 100*time.Hour), // own: 0.38
		"derek": read(0, 40, 10*time.Hour),  // shared, deadline group: 55% ÷ 10h = 5.5
	}
	got, r := pick(t, expiring(), readings, pool.Options{Current: "bob", Load: map[string]int{"derek": 4}})
	if got != "bob" || !r.Selected.Sticky {
		t.Errorf("derek full: picked %s sticky %v (%s); want bob, sticky (alice is only 1.26 times as urgent)", got, r.Selected.Sticky, r.Selected.Reason)
	}
	got, r = pick(t, expiring(), readings, pool.Options{Current: "bob", Load: map[string]int{"derek": 3}})
	if got != "derek" || r.Selected.Sticky || !strings.HasPrefix(r.Selected.Reason, "left bob: derek is at least 3 times as urgent") {
		t.Errorf("derek free: picked %s sticky %v (%s); want derek, leaving bob", got, r.Selected.Sticky, r.Selected.Reason)
	}
	// A session on a full member that is itself the leader stays.
	got, r = pick(t, expiring(), readings, pool.Options{Current: "derek", CurrentAhead: 1, Load: map[string]int{"derek": 4}})
	if got != "derek" || !r.Selected.Sticky {
		t.Errorf("on the full leader: picked %s sticky %v; want derek, sticky", got, r.Selected.Sticky)
	}
}

// TestUnknownResetAfterFullMember: a member without a known reset picked
// after a full one says its own reset is unknown, not that no member has
// one.
func TestUnknownResetAfterFullMember(t *testing.T) {
	readings := map[string]pool.Reading{"alice": read(0, 10, 100*time.Hour), "bob": {Status: pool.StatusOK, Week: &pool.Window{Percent: 10}}}
	got, r := pick(t, expiring(), readings, pool.Options{Load: map[string]int{"alice": 4}})
	want := "alice has 4 other sessions (max_sessions 4); next: its weekly reset or room is unknown; next in config order"
	if got != "bob" || r.Selected.Reason != want {
		t.Errorf("picked %s (%q), want bob (%q)", got, r.Selected.Reason, want)
	}
	got, r = pick(t, expiring(), map[string]pool.Reading{"bob": readings["bob"]}, pool.Options{})
	if got != "bob" || r.Selected.Reason != "no member has a known weekly reset and room; first in config order" {
		t.Errorf("alone: picked %s (%q)", got, r.Selected.Reason)
	}
}

// TestReasonsAtNow: Config.Reasons (a picker's warnings) applies the
// caps Select applies at now: the weekly relax, schedules, overrides,
// and a refused per-model window whose reset has passed does not count.
func TestReasonsAtNow(t *testing.T) {
	cfg := expiring()
	if got := cfg.Reasons("derek", read(10, 80, 6*time.Hour), true, now); len(got) != 0 {
		t.Errorf("80%% six hours before the reset (cap 88.75): %v", got)
	}
	if got := cfg.Reasons("derek", read(10, 80, 30*time.Hour), true, now); !slices.Contains(got, "week is 80% (cap 70%)") {
		t.Errorf("80%% thirty hours before the reset: %v", got)
	}
	if got := cfg.Reasons("derek", withScoped(10, 20, 100*time.Hour, 100, -time.Hour, true), true, now); len(got) != 0 {
		t.Errorf("refused window whose reset passed: %v", got)
	}
	cfg.Accounts["derek"] = pool.Account{Override: &pool.Override{Caps: pool.ScheduleCaps{Week: f(90)}, Until: now.Add(time.Hour)}}
	if got := cfg.Reasons("derek", read(10, 80, 100*time.Hour), true, now); len(got) != 0 {
		t.Errorf("80%% under an override of 90: %v", got)
	}
}

// TestStaleWeekStillBlocks: a weekly window whose reset has passed is an
// old reading as a whole, so it still blocks at its caps (unlike a stale
// per-model window next to a fresh weekly one); it sorts last and says
// so.
func TestStaleWeekStillBlocks(t *testing.T) {
	_, r := pick(t, expiring(), map[string]pool.Reading{"derek": read(10, 99, -time.Hour), "erin": read(10, 10, -time.Hour), "frank": read(10, 10, 50*time.Hour)}, pool.Options{})
	d := candidateOf(t, r, "derek")
	if d.Eligible || !slices.Contains(d.Reasons, "week is 99% (cap 70%)") || !slices.Contains(d.Notes, "weekly reset time has passed; usage data is stale") {
		t.Errorf("derek: eligible %v, reasons %v, notes %v", d.Eligible, d.Reasons, d.Notes)
	}
	if got := ready(r); !slices.Equal(got, []string{"frank", "erin"}) || candidateOf(t, r, "erin").NeedsPerHour != nil {
		t.Errorf("order %v, want frank then erin (stale, no rate)", got)
	}
}

// TestScopedWindowInTheRoom: a per-model window limits the room and
// names itself in the reason; one without a reset of its own limits the
// room until the weekly reset (it used to be left out of the room while
// still blocking at its cap).
func TestScopedWindowInTheRoom(t *testing.T) {
	noReset := read(0, 20, 100*time.Hour)
	noReset.Scoped = []pool.ScopedWindow{{Name: "fable week", Percent: 95}}
	_, r := pick(t, expiring(), map[string]pool.Reading{"alice": noReset}, pool.Options{})
	a := candidateOf(t, r, "alice")
	want := "most room to lose: 3% left under its fable week cap with 100h to the weekly reset (0.03%/h to use it all)"
	if a.Room == nil || *a.Room != 3 || r.Selected.Reason != want {
		t.Errorf("room %v, reason %q; want 3, %q", a.Room, r.Selected.Reason, want)
	}
	// derek's weekly window leaves 55% to 95 by its reset in 12h; his
	// fable week (72%, cap 74.2 now, its reset in 20h) leaves 23% to 95.
	_, r = pick(t, expiring(), map[string]pool.Reading{"derek": withScoped(0, 40, 12*time.Hour, 72, 20*time.Hour, false)}, pool.Options{})
	want = "resets soonest: 23% left before its fable week cap reaches 95% at that window's reset in 20h (1.15%/h to use it all); its reset is within 24h (deadline_window), so using it first loses nothing"
	if r.Selected.Reason != want {
		t.Errorf("reason %q, want %q", r.Selected.Reason, want)
	}
}

// TestTimedCapsAtTheReset: the urgency counts up to the cap the member
// has at its reset, so a schedule that ends before the reset does not
// count there; a higher cap now still counts now.
func TestTimedCapsAtTheReset(t *testing.T) {
	sat := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC) // a Saturday
	reset := sat.Add(20 * time.Hour)                      // Sunday 08:00, after the schedule
	rd := map[string]pool.Reading{"derek": {Status: pool.StatusOK, Short: &pool.Window{Percent: 0}, Week: &pool.Window{Percent: 60, ResetsAt: &reset}}}
	satOnly := func(caps pool.ScheduleCaps) pool.Config {
		c := expiring()
		c.Location = time.UTC
		c.Schedules = []pool.Schedule{{Name: "sat", Days: [7]bool{time.Saturday: true}, Start: 0, End: 1440, Members: []string{"shared"}, Caps: caps}}
		return c
	}
	_, r := pick(t, satOnly(pool.ScheduleCaps{WeekRelaxMax: f(100)}), rd, pool.Options{Now: sat})
	want := "resets soonest: 35% left before its weekly cap reaches 95% at the reset in 20h (1.75%/h to use it all)"
	if !strings.HasPrefix(r.Selected.Reason, want) {
		t.Errorf("relax max 100 until Saturday ends: %q, want %q", r.Selected.Reason, want)
	}
	if d := candidateOf(t, r, "derek"); d.Caps.Week != 75 { // 70 + 30 × 4/24: the schedule's relax max applies now
		t.Errorf("cap now %v, want 75", d.Caps.Week)
	}
	_, r = pick(t, satOnly(pool.ScheduleCaps{Week: f(99)}), rd, pool.Options{Now: sat})
	want = "resets soonest: 39% left under its weekly cap with 20h to the reset"
	if !strings.HasPrefix(r.Selected.Reason, want) {
		t.Errorf("week 99 until Saturday ends: %q, want %q", r.Selected.Reason, want)
	}
	// The same through a config already folded at now, as agx's pick
	// hands it in.
	c := satOnly(pool.ScheduleCaps{WeekRelaxMax: f(100)})
	_, r = pick(t, c.At(sat), rd, pool.Options{Now: sat})
	if want := "resets soonest: 35% left before its weekly cap reaches 95% at the reset in 20h"; !strings.HasPrefix(r.Selected.Reason, want) {
		t.Errorf("folded config: %q, want %q", r.Selected.Reason, want)
	}
	// An override of 99 that ends in an hour, the reset 100h away: the
	// 29% above the 70 that follows must be used within that hour.
	far := sat.Add(100 * time.Hour)
	ovr := expiring()
	ovr.Accounts["derek"] = pool.Account{Override: &pool.Override{Caps: pool.ScheduleCaps{Week: f(99)}, Until: sat.Add(time.Hour)}}
	farRd := map[string]pool.Reading{"derek": {Status: pool.StatusOK, Short: &pool.Window{Percent: 0}, Week: &pool.Window{Percent: 60, ResetsAt: &far}}}
	_, r = pick(t, ovr, farRd, pool.Options{Now: sat})
	want = "resets soonest: 29% left under its weekly cap of 99% before its override ends in 1h (29%/h to use it all)"
	if d := candidateOf(t, r, "derek"); !strings.HasPrefix(r.Selected.Reason, want) || d.RoomAtReset != nil || d.RoomUntil == nil || *d.RoomUntil != time.Hour {
		t.Errorf("override ending first: %q (room at reset %v, due %v), want %q", r.Selected.Reason, d.RoomAtReset, d.RoomUntil, want)
	}
	// Half a percent before the override ends: under deadline_min_room,
	// so it ranks by urgency, still due when the override ends.
	farRd["derek"].Week.Percent = 98.5
	_, r = pick(t, ovr, farRd, pool.Options{Now: sat})
	if want := "most room to lose: 0.5% left under its weekly cap of 99% before its override ends in 1h (0.5%/h to use it all)"; r.Selected.Reason != want {
		t.Errorf("override with 0.5%% left: %q, want %q", r.Selected.Reason, want)
	}
	// An override that outlasts the reset counts to the reset as usual.
	ovr.Accounts["derek"] = pool.Account{Override: &pool.Override{Caps: pool.ScheduleCaps{Week: f(99)}, Until: sat.Add(200 * time.Hour)}}
	farRd["derek"].Week.Percent = 60
	_, r = pick(t, ovr, farRd, pool.Options{Now: sat})
	if want := "most room to lose: 39% left under its weekly cap with 100h to the reset"; !strings.HasPrefix(r.Selected.Reason, want) {
		t.Errorf("override past the reset: %q, want %q", r.Selected.Reason, want)
	}
	// A pinned run's lower weekly cap holds at the reset too.
	_, r = pick(t, satOnly(pool.ScheduleCaps{WeekRelaxMax: f(100)}), rd, pool.Options{Now: sat, Only: "derek", OnlyCaps: pool.Tighter{Week: f(65)}})
	if d := candidateOf(t, r, "derek"); d.RoomAtReset != nil || d.Room == nil || *d.Room != 5 {
		t.Errorf("pinned at 65: room %v, at reset %v; want 5, none", d.Room, d.RoomAtReset)
	}
}

// TestFinalWeek: the urgency counts up to the relax max once the cap is
// rising (the reset at or within the window, inclusive at its end, as
// deadline_window is), and to the cap otherwise.
func TestFinalWeek(t *testing.T) {
	cases := []struct {
		name          string
		cap, max      float64
		window, until time.Duration
		want          float64
	}{
		{"inside the window", 70, 95, day, 12 * time.Hour, 95},
		{"at the window's end", 70, 95, day, day, 95},
		{"a minute before the window opens", 70, 95, day, day + time.Minute, 70},
		{"at the reset", 70, 95, day, 0, 95},
		{"a passed reset", 70, 95, day, -time.Minute, 70},
		{"relax off", 70, 95, 0, time.Hour, 70},
		{"relax max below the cap", 98, 95, day, time.Hour, 98},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pool.FinalWeek(c.cap, c.max, c.window, c.until); got != c.want {
				t.Errorf("FinalWeek = %v, want %v", got, c.want)
			}
		})
	}
}

// TestOrderUnset: a Config built by hand without an order (the zero
// value) ranks by the default order, and says so.
func TestOrderUnset(t *testing.T) {
	c := expiring()
	c.Order = ""
	got, r := pick(t, c, map[string]pool.Reading{"alice": read(0, 0, 100*time.Hour), "derek": read(0, 60, 5*time.Hour)}, pool.Options{})
	if r.Order != pool.DefaultOrder || c.Rules().Order != pool.DefaultOrder || got != "derek" {
		t.Errorf("order %q, rules %q, picked %s; want %q and derek (the deadline group)", r.Order, c.Rules().Order, got, pool.DefaultOrder)
	}
}

// TestScheduleWithoutDaysNeverApplies: a schedule built by hand with no
// day (Validate refuses one from config) never applies.
func TestScheduleWithoutDaysNeverApplies(t *testing.T) {
	if on, until := (pool.Schedule{Start: 0, End: 1440}).ActiveUntil(now, time.UTC); on || until != nil {
		t.Errorf("applies %v until %v", on, until)
	}
}
