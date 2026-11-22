package pool_test

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// Invariants of the order and the pick over thousands of seeded random
// pools under the default rule family (random rule values, schedules,
// overrides, per-model windows, capacities, sessions). A failure names
// its seed: SEED=1234 go test -run TestInvariant ./pool/ replays it;
// CASES=50000 runs more.

// invMembers are the random pools' member names.
var (
	invOwn    = []string{"o1", "o2", "o3"}
	invShared = []string{"s1", "s2", "s3", "s4", "s5"}
)

// invConfig is a random valid pool under the urgency order with random
// rule values; schedules and overrides around propNow.
func invConfig(g gen) pool.Config {
	c := pool.New(g.shuffle(invOwn)[:g.intn(0, 3)], g.shuffle(invShared)[:g.intn(0, 5)])
	if len(c.Own)+len(c.Shared) == 0 {
		c.Shared = []string{"s1"}
	}
	hours := func(list ...float64) time.Duration { return time.Duration(g.pickF(list...) * float64(time.Hour)) }
	c.BorrowWindow = hours(0, 12, 24, 48, 48, 72)
	c.WeekRelaxWindow = hours(0, 12, 24, 24, 48)
	c.WeekRelaxMax = g.pickF(80, 90, 95, 95, 98)
	c.DeadlineWindow = hours(0, 6, 12, 24, 24, 48)
	c.DeadlineMinRoom = g.pickF(0, 1, 2, 2, 5, 10)
	c.MaxSessions = g.intn(0, 4)
	c.Sticky = g.chance(0.7)
	c.StickyLeaveRatio = g.pickF(0, 1.5, 3, 3)
	if g.chance(0.15) {
		c.Order = pool.OrderDeadline
	}
	if g.chance(0.2) {
		c.Groups = map[pool.Group]pool.GroupCaps{pool.GroupShared: {Week: f(g.pickF(60, 80)), WeekRelaxMax: f(g.pickF(85, 98))}}
	}
	c.Accounts = map[string]pool.Account{}
	for _, m := range slices.Concat(c.Own, c.Shared) {
		a := pool.Account{}
		if g.chance(0.3) {
			a.Week = f(g.pickF(50, 60, 70, 80, 98))
		}
		if g.chance(0.2) {
			a.WeekRelaxMax = f(g.pickF(70, 90, 95))
		}
		if g.chance(0.3) {
			a.Capacity = f(g.pickF(1, 5, 20))
		}
		if g.chance(0.25) {
			// An override that may have ended already.
			a.Override = &pool.Override{Caps: pool.ScheduleCaps{Short: f(g.pickF(60, 95)), Week: f(g.pickF(80, 95, 99)), WeekRelaxMax: f(g.pickF(90, 100))},
				Until: propNow.Add(hours(-30, -1, -0.01, 0.01, 1, 30))}
		}
		c.Accounts[m] = a
	}
	if g.chance(0.4) {
		var days [7]bool
		for i := range days {
			days[i] = g.chance(0.4)
		}
		// At least one day; today often, so the schedule may apply now.
		days[propNow.Weekday()] = days[propNow.Weekday()] || g.chance(0.5) || !slices.Contains(days[:], true)
		start := g.intn(0, 23) * 60
		c.Schedules = []pool.Schedule{{Name: "w", Days: days, Start: start, End: (start + g.intn(1, 23)*60) % 1440,
			Members: []string{g.shuffle([]string{"own", "shared"})[0]}, Caps: pool.ScheduleCaps{Short: f(95), Week: f(g.pickF(75, 90))},
			ReleaseBefore: hours(0, 1, 5)}}
		if c.Schedules[0].End == 0 {
			c.Schedules[0].End = 1440
		}
	}
	return c
}

// invReadings are random readings for cfg's members: usage that leans
// to the caps, resets from passed to a week away, some unknown, some
// statuses not ok, some per-model windows.
func invReadings(g gen, cfg pool.Config) map[string]pool.Reading {
	out := map[string]pool.Reading{}
	for _, m := range slices.Concat(cfg.Own, cfg.Shared) {
		if g.chance(0.05) {
			continue
		}
		rd := pool.Reading{Status: pool.StatusOK}
		if g.chance(0.05) {
			rd.Status = "limit-reached"
		}
		rd.Short = &pool.Window{Percent: g.pickF(0, 10, 40, 49.9, 50, 80, 94.9, 99, float64(g.intn(0, 100)))}
		reset := propNow.Add(time.Duration(g.pickF(-2, 0, 0.1, 3, 6, 6.7, 12, 12.7, 23.9, 24, 24.1, 48, 100, 167) * float64(time.Hour)))
		w := &pool.Window{Percent: g.pickF(0, 20, 50, 69.9, 70, 85, 87, 93, 97, 97.9, 99, float64(g.intn(0, 100))), ResetsAt: &reset}
		switch {
		case g.chance(0.04):
			w.ResetsAt = nil
		case g.chance(0.03):
			w.ResetsAt = &time.Time{}
		case g.chance(0.02):
			w.Percent = math.NaN()
		}
		rd.Week = w
		if g.chance(0.2) {
			sr := propNow.Add(time.Duration(g.pickF(-1, 5, 20, 60, 150) * float64(time.Hour)))
			rd.Scoped = []pool.ScopedWindow{{Name: "model week", Percent: g.pickF(0, 30, 60, 90, 99), ResetsAt: &sr, Rejected: g.chance(0.05)}}
			switch {
			case g.chance(0.2):
				rd.Scoped[0].ResetsAt = nil
			case g.chance(0.05):
				rd.Scoped[0].Percent = math.NaN()
			}
		}
		out[m] = rd
	}
	return out
}

// invOptions are a random session: other sessions per member, the
// current member and how many sessions on it are older.
func invOptions(g gen, cfg pool.Config) pool.Options {
	o := pool.Options{Now: propNow, Load: map[string]int{}}
	members := slices.Concat(cfg.Own, cfg.Shared)
	for _, m := range members {
		o.Load[m] = g.intn(0, 5)
	}
	if g.chance(0.6) {
		o.Current = members[g.intn(0, len(members)-1)]
		o.CurrentAhead = g.intn(0, o.Load[o.Current])
	}
	return o
}

// TestInvariantRandomPools checks checkInvariants on random pools.
func TestInvariantRandomPools(t *testing.T) {
	for _, seed := range propSeeds() {
		g := newGen(seed)
		cfg := invConfig(g)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("seed %d: generated an invalid config: %v", seed, err)
		}
		readings, o := invReadings(g, cfg), invOptions(g, cfg)
		label := fmt.Sprintf("seed %d", seed)
		checkInvariants(t, label, cfg, readings, o)
		// A config already folded at now (as agx's pick hands it in)
		// selects the same: Select reads the caps at each reset from the
		// normal caps, not from the ones folded in now.
		a, errA := pool.Select(cfg, readings, o)
		b, errB := pool.Select(cfg.At(o.Now), readings, o)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if errA != nil || errB != nil || string(ja) != string(jb) {
			t.Errorf("%s: Select of the folded config differs:\n%s\n%s", label, ja, jb)
		}
	}
}

// checkInvariants checks one selection:
//   - a pick is never above a cap in effect: eligible, status ok, every
//     window under its cap and the 5-hour cutoff;
//   - the pick carries the sessions the caller counted for it;
//   - ready ranks run 1..n; the deadline group comes first, soonest
//     reset first, and only under the urgency order, inside the deadline
//     window and with at least the minimum room;
//   - "needs %/h" is never 0 while there is room before a known reset;
//   - an override that has ended or a schedule that does not apply now
//     never sets a cap.
func checkInvariants(t *testing.T, label string, cfg pool.Config, readings map[string]pool.Reading, o pool.Options) {
	t.Helper()
	r, err := pool.Select(cfg, readings, o)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	byName := map[string]pool.Candidate{}
	for _, c := range r.Candidates {
		byName[c.Name] = c
	}
	if s := r.Selected; s != nil && !s.Forced {
		c := byName[s.Name]
		rd := readings[s.Name]
		switch {
		case !c.Eligible || c.State == pool.StateBlocked || rd.Status != pool.StatusOK:
			t.Errorf("%s: picked %s, which is not usable (%v)", label, s.Name, c.Reasons)
		case rd.Week != nil && rd.Week.Percent >= c.Caps.Week:
			t.Errorf("%s: picked %s at week %v, cap %v", label, s.Name, rd.Week.Percent, c.Caps.Week)
		case rd.Short != nil && (rd.Short.Percent >= c.Caps.Short || rd.Short.Percent >= cfg.ShortUnavailableAt):
			t.Errorf("%s: picked %s at 5h %v, cap %v", label, s.Name, rd.Short.Percent, c.Caps.Short)
		}
		for _, sw := range c.Scoped {
			if !sw.Stale && sw.Percent != nil && *sw.Percent >= sw.Cap {
				t.Errorf("%s: picked %s with %s at %v, cap %v", label, s.Name, sw.Name, *sw.Percent, sw.Cap)
			}
		}
		if s.Load != o.Load[s.Name] {
			t.Errorf("%s: pick %s carries %d sessions, the caller counted %d", label, s.Name, s.Load, o.Load[s.Name])
		}
	}
	rank, inGroup := 0, true
	var lastUntil time.Duration
	for _, c := range r.Candidates {
		if c.Deadline {
			switch {
			case cfg.Order == pool.OrderDeadline || cfg.DeadlineWindow == 0 || c.NeedsPerHour == nil || c.RoomUntil == nil || !c.Eligible:
				t.Errorf("%s: %s is in the deadline group without the urgency order, a window, a known room or being usable", label, c.Name)
			case *c.RoomUntil > cfg.DeadlineWindow || *c.RoomUntil < 0:
				t.Errorf("%s: %s is in the deadline group with its room due in %s (window %s)", label, c.Name, *c.RoomUntil, cfg.DeadlineWindow)
			case *c.NeedsPerHour*math.Max(c.RoomUntil.Hours(), 0.25) < cfg.DeadlineMinRoom-0.005*math.Max(c.RoomUntil.Hours(), 0.25)-0.05:
				// NeedsPerHour is rounded to 0.01, so the room it gives back is off by up to 0.005 an hour.
				t.Errorf("%s: %s is in the deadline group with less than %v%% of room", label, c.Name, cfg.DeadlineMinRoom)
			}
		}
		if c.Room != nil && c.RoomAtReset != nil && *c.RoomAtReset < *c.Room {
			t.Errorf("%s: %s has room %v now but %v by the reset", label, c.Name, *c.Room, *c.RoomAtReset)
		}
		if c.NeedsPerHour != nil && *c.NeedsPerHour > 0.05 && math.Abs(*c.Urgency-*c.NeedsPerHour*cfg.CapacityFor(c.Name)) > 0.02*cfg.CapacityFor(c.Name)+0.01 {
			t.Errorf("%s: %s urgency %v is not needs %v times capacity %v", label, c.Name, *c.Urgency, *c.NeedsPerHour, cfg.CapacityFor(c.Name))
		}
		if c.State != pool.StateReady {
			continue
		}
		rank++
		if c.Rank != rank {
			t.Errorf("%s: %s has rank %d, want %d", label, c.Name, c.Rank, rank)
		}
		switch {
		case c.Deadline && !inGroup:
			t.Errorf("%s: %s is in the deadline group after a member that is not", label, c.Name)
		case c.Deadline:
			if c.RoomUntil == nil || *c.RoomUntil < lastUntil {
				t.Errorf("%s: deadline group out of reset order at %s", label, c.Name)
			} else {
				lastUntil = *c.RoomUntil
			}
		case !c.Deadline:
			inGroup = false
		}
	}
	for _, c := range r.Candidates {
		room := c.Room != nil && *c.Room > 0 || c.RoomAtReset != nil && *c.RoomAtReset > 0
		if c.NeedsPerHour != nil && room && (*c.NeedsPerHour <= 0 || c.Urgency == nil || *c.Urgency <= 0) {
			t.Errorf("%s: %s shows needs %v%%/h, urgency %v with room %v / %v", label, c.Name, *c.NeedsPerHour, c.Urgency, c.Room, c.RoomAtReset)
		}
		if c.Timed == nil {
			continue
		}
		for _, src := range []*pool.CapSource{c.Timed.Short, c.Timed.Week, c.Timed.WeekRelaxMax} {
			if src != nil && src.Until != nil && !src.Until.After(o.Now) {
				t.Errorf("%s: %s has a cap from a %s that ended %v", label, c.Name, src.Kind, src.Until)
			}
		}
	}
}

// TestInvariantSoonerResetNeverLater: of two members with the same
// group, caps, capacity and room before their resets (each week set so
// the room up to the cap it reaches at its reset is equal), the one that
// resets sooner never ranks after the other, at any room, either inside
// or outside the deadline and relax windows.
func TestInvariantSoonerResetNeverLater(t *testing.T) {
	for _, seed := range propSeeds() {
		g := newGen(seed)
		cfg := invConfig(g)
		cfg.Schedules = nil
		readings := invReadings(g, cfg)
		group := pool.GroupShared
		if g.chance(0.4) {
			group = pool.GroupOwn
		}
		pair := []string{"p1", "p2"}
		if group == pool.GroupOwn {
			cfg.Own = append(cfg.Own, pair...)
		} else {
			cfg.Shared = append(cfg.Shared, pair...)
		}
		capacity := g.pickF(1, 5)
		weekCap := g.pickF(70, 80, 98)
		room := g.pickF(0.5, 1, 1.99, 2, 2.01, 5, 8, 20)
		resets := []float64{g.pickF(0.1, 3, 6.7, 12, 23.9, 24, 30, 60), 0}
		resets[1] = resets[0] + g.pickF(0.5, 1, 6, 20, 50)
		for i, name := range pair {
			cfg.Accounts[name] = pool.Account{Short: f(95), Week: f(weekCap), Capacity: f(capacity)}
			until := time.Duration(resets[i] * float64(time.Hour))
			final := pool.FinalWeek(weekCap, cfg.RelaxMaxFor(name), cfg.WeekRelaxWindow, until)
			reset := propNow.Add(until)
			readings[name] = pool.Reading{Status: pool.StatusOK, Short: &pool.Window{Percent: 1}, Week: &pool.Window{Percent: final - room, ResetsAt: &reset}}
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		r, err := pool.Select(cfg, readings, pool.Options{Now: propNow})
		if err != nil {
			t.Fatal(err)
		}
		pos := map[string]int{}
		for i, c := range r.Candidates {
			if c.Eligible {
				pos[c.Name] = i
			}
		}
		i1, ok1 := pos["p1"]
		i2, ok2 := pos["p2"]
		if ok1 && ok2 && i1 > i2 {
			t.Errorf("seed %d: p1 (resets in %.1fh) ranks after p2 (%.1fh), room %v each", seed, resets[0], resets[1], room)
		}
	}
}

// TestInvariantWeekBeforeReset adds week_before_reset schedules that
// apply now (one naming a group, often one naming members) to the random
// pools, checks every invariant above with them, and against the same
// pool without the values:
//   - a member whose weekly reset falls before the schedule's end is
//     never blocked below the raised cap: its weekly cap is the higher of
//     its cap without the rule and the value, and no reason blocks it at
//     a weekly use under the value;
//   - a member that resets after the end (or at an unknown or passed
//     reset) never gets the raised weekly cap: its cap is the one it has
//     without the rule, and no cap names week_before_reset;
//   - each per-model window follows the same rule by its own reset.
//
// The expected value is worked out here from the schedules, apart from
// the code: the schedules naming the member first, then those naming its
// group, the highest value of the first level with one that covers the
// reset; none when an override that applies sets the weekly cap, and at
// most a pinned run's weekly cap.
func TestInvariantWeekBeforeReset(t *testing.T) {
	for _, seed := range propSeeds() {
		g := newGen(seed)
		cfg := invConfig(g)
		readings, o := invReadings(g, cfg), invOptions(g, cfg)
		h := newGen(seed ^ 0x5bd1e995)
		members := slices.Concat(cfg.Own, cfg.Shared)
		today, tomorrow := propNow.Weekday(), (propNow.Weekday()+1)%7
		add := func(name string, names []string) {
			var days [7]bool
			days[today] = true
			days[tomorrow] = h.chance(0.5)
			start := h.intn(0, propNow.Hour()) * 60
			end := h.intn(propNow.Hour()+1, 24) * 60
			s := pool.Schedule{Name: name, Days: days, Start: start, End: end, Members: names,
				WeekBeforeReset: f(h.pickF(60, 75, 90, 95, 99, 100)), ReleaseBefore: time.Duration(h.pickF(0, 1, 5) * float64(time.Hour))}
			if h.chance(0.3) {
				s.Caps.Week = f(h.pickF(60, 85))
			}
			cfg.Schedules = append(cfg.Schedules, s)
		}
		add("b-group", []string{h.shuffle([]string{"own", "shared"})[0]})
		if h.chance(0.5) {
			add("b-members", h.shuffle(members)[:h.intn(1, len(members))])
		}
		if h.chance(0.1) {
			// A pinned run with a weekly cap of its own, at or below the
			// member's: the pool's own check refuses a higher one.
			o.Only = members[h.intn(0, len(members)-1)]
			o.OnlyCaps = pool.Tighter{Week: f(math.Min(cfg.At(propNow).CapsFor(o.Only).Week, h.pickF(50, 80)))}
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		label := fmt.Sprintf("seed %d", seed)
		checkInvariants(t, label, cfg, readings, o)

		without := cfg
		without.Schedules = slices.Clone(cfg.Schedules)
		for i := range without.Schedules {
			without.Schedules[i].WeekBeforeReset = nil
		}
		r, err := pool.Select(cfg, readings, o)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		r0, err := pool.Select(without, readings, o)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		before := map[string]pool.Candidate{}
		for _, c := range r0.Candidates {
			before[c.Name] = c
		}
		for _, c := range r.Candidates {
			c0, rd := before[c.Name], readings[c.Name]
			var reset *time.Time
			if rd.Week != nil {
				reset = rd.Week.ResetsAt
			}
			want, ok := expectBeforeReset(cfg, c.Name, reset, o)
			if ok && want > c0.Caps.Week {
				if math.Abs(c.Caps.Week-want) > 1e-9 {
					t.Errorf("%s: %s resets before the end: weekly cap %v, want %v (without the rule %v)", label, c.Name, c.Caps.Week, want, c0.Caps.Week)
				}
				if readable(rd.Week) && rd.Week.Percent < want && slices.ContainsFunc(c.Reasons, func(s string) bool { return strings.HasPrefix(s, "week is ") }) {
					t.Errorf("%s: %s blocked at week %v below the raised cap %v: %v", label, c.Name, rd.Week.Percent, want, c.Reasons)
				}
			} else if c.Caps.Week != c0.Caps.Week || c.Timed != nil && c.Timed.Week != nil && c.Timed.Week.BeforeReset {
				t.Errorf("%s: %s does not reset before the end but has weekly cap %v (without the rule %v), timed %+v", label, c.Name, c.Caps.Week, c0.Caps.Week, c.Timed)
			}
			for i, s := range c.Scoped {
				s0 := c0.Scoped[i]
				w, ok := expectBeforeReset(cfg, c.Name, rd.Scoped[i].ResetsAt, o)
				switch {
				case ok && math.Abs(s.Cap-math.Max(s0.Cap, w)) > 1e-9:
					t.Errorf("%s: %s %s resets before the end: cap %v, want max(%v, %v)", label, c.Name, s.Name, s.Cap, s0.Cap, w)
				case !ok && s.Cap != s0.Cap:
					t.Errorf("%s: %s %s does not reset before the end but has cap %v (without the rule %v)", label, c.Name, s.Name, s.Cap, s0.Cap)
				}
			}
		}
	}
}

// readable is a window whose use is a number.
func readable(w *pool.Window) bool {
	return w != nil && !math.IsNaN(w.Percent) && !math.IsInf(w.Percent, 0)
}

// expectBeforeReset is the week_before_reset member name should get for
// a window resetting at reset (TestInvariantWeekBeforeReset's rule).
func expectBeforeReset(cfg pool.Config, name string, reset *time.Time, o pool.Options) (float64, bool) {
	now := o.Now
	if reset == nil || reset.IsZero() || reset.Before(now) {
		return 0, false
	}
	if ov := cfg.Accounts[name].Override; ov != nil && ov.Active(now) && ov.Caps.Week != nil {
		return 0, false
	}
	group := "shared"
	if slices.Contains(cfg.Own, name) {
		group = "own"
	}
	for _, byName := range []bool{true, false} {
		best, found := 0.0, false
		for _, s := range cfg.Schedules {
			if s.WeekBeforeReset == nil {
				continue
			}
			named := slices.Contains(s.Members, name)
			if byName != named || !byName && !slices.Contains(s.Members, group) {
				continue
			}
			on, until := s.ActiveUntil(now, cfg.Location)
			if !on || until != nil && reset.After(*until) {
				continue
			}
			best, found = math.Max(best, *s.WeekBeforeReset), true
		}
		if found {
			if o.Only == name && o.OnlyCaps.Week != nil {
				best = math.Min(best, *o.OnlyCaps.Week)
			}
			return best, true
		}
	}
	return 0, false
}
