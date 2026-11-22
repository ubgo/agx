package pool

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

// The rule values beyond the caps (docs/guides/pool-rules.md; the
// simulator in internal/poolsim chose the defaults). Each is a Config
// value with a default, and a Preset is a named set of them:
//
//  1. Weekly relax: over the last WeekRelaxWindow before a member's
//     weekly reset its weekly cap rises linearly from the configured cap
//     to its relax max (RelaxMaxFor), so room that is about to be lost
//     can be used; the 5-hour cap never relaxes (that window outlives
//     the weekly reset and is the teammate's working time). 0 = off.
//  2. Order: OrderUrgency sorts the pool by the room left before the
//     reset divided by the hours to it (the rate needed to use it all),
//     highest first. The room counts up to the cap the member reaches
//     at the reset (FinalWeek), so a cap still rising is not
//     undercounted; whether a member is usable still goes by its cap
//     now. Members whose reset is within DeadlineWindow and that have at
//     least DeadlineMinRoom of that room go before the rest, soonest
//     reset first: the room that expires first is used first, which
//     loses nothing, while urgency alone can let a member that resets
//     sooner lose its room to one that resets later with more room.
//     OrderDeadline sorts by the soonest reset only. An unknown or
//     passed reset sorts last.
//  3. A pick per session: a session stays on its member while that
//     member is still ready, fewer than MaxSessions older sessions hold
//     it and no member is StickyLeaveRatio times more urgent (Sticky);
//     otherwise it takes the first member in order with fewer than
//     MaxSessions other sessions, ready members first, then the waiting
//     ones (held back only by the borrow window); when every member is
//     full, the least loaded ready member. MaxSessions 0 and Sticky
//     false give every session the pool's first member.

// minUrgencyHours keeps a reset minutes away from dividing by zero.
const minUrgencyHours = 0.25

// order is the configured order ("" = the default).
func (c Config) order() Order {
	if c.Order == "" {
		return DefaultOrder
	}
	return c.Order
}

// validateRules checks the order and the rule values.
func (c Config) validateRules() error {
	if c.Order != "" && !slices.Contains(OrderValues(), string(c.Order)) {
		return fmt.Errorf("pool: order %q is not one of %v", c.Order, OrderValues())
	}
	if c.WeekRelaxWindow < 0 {
		return errors.New("pool: week_relax_window is negative")
	}
	if c.MaxSessions < 0 {
		return errors.New("pool: max_sessions is negative")
	}
	if c.DeadlineWindow < 0 {
		return errors.New("pool: deadline_window is negative")
	}
	if c.StickyLeaveRatio < 0 || math.IsNaN(c.StickyLeaveRatio) || math.IsInf(c.StickyLeaveRatio, 0) {
		return fmt.Errorf("pool: sticky_leave_ratio %v is not a number of 0 or more", c.StickyLeaveRatio)
	}
	return nil
}

// Rules are a pool's rule values beyond its members and caps: what a
// Preset sets.
type Rules struct {
	Order            Order
	BorrowWindow     time.Duration
	WeekRelaxWindow  time.Duration
	WeekRelaxMax     float64
	MaxSessions      int
	Sticky           bool
	StickyLeaveRatio float64
	DeadlineWindow   time.Duration
	DeadlineMinRoom  float64
}

// Rules are c's rule values.
func (c Config) Rules() Rules {
	return Rules{Order: c.order(), BorrowWindow: c.BorrowWindow, WeekRelaxWindow: c.WeekRelaxWindow, WeekRelaxMax: c.WeekRelaxMax,
		MaxSessions: c.MaxSessions, Sticky: c.Sticky, StickyLeaveRatio: c.StickyLeaveRatio, DeadlineWindow: c.DeadlineWindow, DeadlineMinRoom: c.DeadlineMinRoom}
}

// WithRules is c with r's rule values.
func (c Config) WithRules(r Rules) Config {
	c.Order, c.BorrowWindow, c.WeekRelaxWindow, c.WeekRelaxMax = r.Order, r.BorrowWindow, r.WeekRelaxWindow, r.WeekRelaxMax
	c.MaxSessions, c.Sticky, c.StickyLeaveRatio = r.MaxSessions, r.Sticky, r.StickyLeaveRatio
	c.DeadlineWindow, c.DeadlineMinRoom = r.DeadlineWindow, r.DeadlineMinRoom
	return c
}

// Preset names.
const (
	// PresetRecommended is the defaults, chosen by the simulator.
	PresetRecommended = "recommended"
	// PresetClassic is the rules before v0.4.16: soonest weekly reset
	// first, no relax, every session on the pool's first member.
	PresetClassic = "classic"
)

// Preset is a named set of rule values. agx pool preset writes them into
// a pool's config, where each stays an ordinary key.
type Preset struct {
	Name  string
	About string
	Rules Rules
}

// DefaultRules are the rule values of New.
func DefaultRules() Rules {
	return Rules{Order: DefaultOrder, BorrowWindow: DefaultBorrowWindow, WeekRelaxWindow: DefaultWeekRelaxWindow, WeekRelaxMax: DefaultWeekRelaxMax,
		MaxSessions: DefaultMaxSessions, Sticky: DefaultSticky, StickyLeaveRatio: DefaultStickyLeaveRatio,
		DeadlineWindow: DefaultDeadlineWindow, DeadlineMinRoom: DefaultDeadlineMinRoom}
}

// Presets are the named rule sets, recommended first.
func Presets() []Preset {
	classic := DefaultRules()
	classic.Order, classic.WeekRelaxWindow, classic.MaxSessions, classic.Sticky, classic.StickyLeaveRatio = OrderDeadline, 0, 0, false, 0
	// The deadline order already puts the soonest reset first.
	classic.DeadlineWindow = 0
	return []Preset{
		{Name: PresetRecommended, About: "the defaults: urgency order with members resetting within 24h first (soonest first, at least 2% room), weekly cap rising to 95% over the last 24h, at most 4 sessions per member, sticky sessions that leave for a member 3 times as urgent", Rules: DefaultRules()},
		{Name: PresetClassic, About: "the rules before v0.4.16: soonest weekly reset first, no relax, no session limit, not sticky", Rules: classic},
	}
}

// PresetNamed is the preset called name.
func PresetNamed(name string) (Preset, bool) {
	i := slices.IndexFunc(Presets(), func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, false
	}
	return Presets()[i], true
}

// PresetNames are the presets' names.
func PresetNames() []string {
	out := []string{}
	for _, p := range Presets() {
		out = append(out, p.Name)
	}
	return out
}

// validateGroups checks the group entries: only own and shared, every
// cap a percent.
func (c Config) validateGroups() error {
	names := make([]string, 0, len(c.Groups))
	for g := range c.Groups {
		names = append(names, string(g))
	}
	slices.Sort(names)
	for _, name := range names {
		g := Group(name)
		if g != GroupOwn && g != GroupShared {
			return fmt.Errorf("pool: group %q has caps; only own and shared do", name)
		}
		gc := c.Groups[g]
		for _, p := range []struct {
			label string
			v     *float64
		}{{"short", gc.Short}, {"week", gc.Week}, {"week_relax_max", gc.WeekRelaxMax}} {
			if p.v != nil && !validPercent(*p.v) {
				return fmt.Errorf("pool: %s_caps.%s %v is outside 0..100", g, p.label, *p.v)
			}
		}
	}
	return nil
}

// groupOf is the group a member is listed in (GroupUnlisted for none).
func (c Config) groupOf(name string) Group {
	switch {
	case slices.Contains(c.Own, name):
		return GroupOwn
	case slices.Contains(c.Shared, name):
		return GroupShared
	}
	return GroupUnlisted
}

// RelaxMaxFor is the weekly cap member name reaches at its reset: its
// own entry, else its group's, else the
// pool's WeekRelaxMax. A value at or below the member's weekly cap means
// it is never relaxed.
func (c Config) RelaxMaxFor(name string) float64 {
	if a, ok := c.Accounts[name]; ok && a.WeekRelaxMax != nil {
		return *a.WeekRelaxMax
	}
	if g, ok := c.Groups[c.groupOf(name)]; ok && g.WeekRelaxMax != nil {
		return *g.WeekRelaxMax
	}
	return c.WeekRelaxMax
}

// CapacityFor is member name's capacity (Account.Capacity), 1 when not
// set.
func (c Config) CapacityFor(name string) float64 {
	if a, ok := c.Accounts[name]; ok && a.Capacity != nil {
		return *a.Capacity
	}
	return 1
}

// RelaxedWeek is a weekly cap capWeek relaxed toward relaxMax over the
// last window before a reset that is until away: linear from capWeek at
// window to relaxMax at the reset. Outside the window, with a passed
// reset, or when relaxMax is not above capWeek, it is capWeek.
func RelaxedWeek(capWeek, relaxMax float64, window, until time.Duration) float64 {
	if window <= 0 || until < 0 || until >= window || relaxMax <= capWeek {
		return capWeek
	}
	gone := 1 - until.Seconds()/window.Seconds()
	return capWeek + (relaxMax-capWeek)*gone
}

// FinalWeek is the weekly cap the urgency counts up to for a reset that
// is until away: relaxMax once the cap is rising toward it (the reset at
// or within window; window above 0 and relaxMax above capWeek), else
// capWeek. Inside the window the whole rise is still to come before the
// reset, so it is room to use or lose; before the window opens none of
// it can be used yet, and counting it there would rank a teammate's
// account days from its reset above an own account (the simulator: worse
// for mixed pools). The window's end is inclusive, as deadline_window's
// is. Whether the member is usable now still goes by the cap it has now.
func FinalWeek(capWeek, relaxMax float64, window, until time.Duration) float64 {
	if window <= 0 || until < 0 || until > window || relaxMax <= capWeek {
		return capWeek
	}
	return relaxMax
}

// finalCap is the weekly cap the urgency counts up to for a reset at
// time at, until away: FinalWeek of the caps that apply then (a schedule
// or an override that ends before the reset does not count; Select sets
// atReset), else of the caps that apply now. A schedule's
// week_before_reset is not in it: it counts through the cap now
// (setUrgency counts up to the higher of the two), so only while the
// schedule applies, as the relax's rise counts only once it has begun.
func (c Config) finalCap(name string, at time.Time, until time.Duration) float64 {
	week, relaxMax := c.CapsFor(name).Week, c.RelaxMaxFor(name)
	if c.atReset != nil {
		week, relaxMax = c.atReset(name, at)
	}
	return FinalWeek(week, relaxMax, c.WeekRelaxWindow, until)
}

// relax raises c's weekly cap for its reading's reset and notes it: the
// weekly relax, or a schedule's week_before_reset when its reset falls
// before the schedule ends and the value is higher (schedule.go). The
// latter applies whole at once: the room is lost at the reset either
// way, so there is no reason to hand it out in steps.
func (c Config) relax(cand *Candidate, rd Reading, now time.Time) {
	if rd.Week == nil || rd.Week.ResetsAt == nil || rd.Week.ResetsAt.IsZero() {
		return
	}
	configured := cand.Caps.Week
	relaxMax := c.RelaxMaxFor(cand.Name)
	week := RelaxedWeek(configured, relaxMax, c.WeekRelaxWindow, rd.Week.ResetsAt.Sub(now))
	if v, src, ok := c.WeekBeforeReset(cand.Name, *rd.Week.ResetsAt, now); ok && v > week {
		cand.Caps.Week = v
		cand.WeekCapConfigured = ptr(configured)
		t := TimedCaps{}
		if cand.Timed != nil {
			t = *cand.Timed
		}
		t.Week = &src
		cand.Timed = &t
		cand.Notes = append(cand.Notes, fmt.Sprintf(noteBeforeReset, num(v), src.Name))
		return
	}
	if week == configured {
		return
	}
	cand.Caps.Week = week
	cand.WeekCapConfigured = ptr(configured)
	cand.Notes = append(cand.Notes, fmt.Sprintf("weekly cap %s%% rises to %s%% over the last %s before the reset: %s%% now",
		num(configured), num(relaxMax), Hours(c.WeekRelaxWindow), num(week)))
}

// noteBeforeReset explains a weekly cap raised by a schedule's
// week_before_reset.
const noteBeforeReset = "weekly cap %s%% (schedule %s: resets before it ends)"

// setUrgency sets the candidate's room and urgency. The room (Room) is
// the room under the weekly cap now, or less when a per-model window
// leaves less (scoped.go): what the member may use now. The urgency
// counts the room each window leaves before its reset, up to the cap it
// reaches at that reset while that cap is rising (finalCap; the cap now
// when that is higher), so a member whose cap is still rising is not
// undercounted: the least such room
// (RoomAtReset) ÷ hours to that window's reset is the rate needed to use
// it all (NeedsPerHour, in percent per hour), and times the member's
// capacity it is the urgency, the order's key. A per-model window
// without a reset of its own limits the room until the weekly reset.
// Both stay unset when the room or a future reset is unknown. The fields
// show them rounded; the order uses the exact values.
func (c Config) setUrgency(cand *Candidate, rd Reading) {
	if !readable(rd.Week) {
		return
	}
	capNow := cand.Caps.Week
	room := math.Max(0, capNow-rd.Week.Percent)
	var until *time.Duration
	upTo := capNow
	final := capNow
	if cand.hasWeekReset {
		until = cand.UntilReset
		final = c.finalCap(cand.Name, *cand.WeeklyResetsAt, *until)
		upTo = math.Max(capNow, final)
	}
	ahead, window := math.Max(0, upTo-rd.Week.Percent), roomWindow{until: until, upTo: upTo}
	if end, src, ok := cand.timedWeekEnd(); ok && capNow > final {
		// A schedule or override raises the weekly cap now and ends
		// before the reset: the room above the cap after it ends must
		// be used before then. It decides when it needs the higher rate.
		excess := math.Max(0, capNow-math.Max(final, rd.Week.Percent))
		if excess/math.Max(end.Hours(), minUrgencyHours) > ahead/math.Max(until.Hours(), minUrgencyHours) {
			ahead, window = excess, roomWindow{until: &end, upTo: capNow, ends: src}
		}
	}
	for _, s := range cand.Scoped {
		if s.Stale || s.unreadable {
			// It has reset since the reading, or its use is unknown: it
			// says nothing about the room now (scopedStates).
			continue
		}
		room = math.Min(room, s.Room)
		w := roomWindow{name: s.Name, until: until, upTo: s.Cap}
		if s.until != nil {
			w.until, w.own, w.upTo = s.until, true, math.Max(s.Cap, s.finalCap)
		}
		if r := math.Max(0, w.upTo-*s.Percent); r < ahead {
			ahead, window = r, w
		}
	}
	cand.Room = ptr(round(room))
	cand.urgencyRoom, cand.window = ahead, window
	if window.until == nil || *window.until < 0 {
		return
	}
	if round(ahead) != round(room) && window.ends == "" {
		cand.RoomAtReset = ptr(round(ahead))
	}
	needs := ahead / math.Max(window.until.Hours(), minUrgencyHours)
	cand.urgency = needs * c.CapacityFor(cand.Name)
	cand.hasUrgency = true
	cand.RoomUntil = window.until
	cand.Urgency = ptr(perHour(cand.urgency))
	cand.NeedsPerHour = ptr(perHour(needs))
}

// roomWindow is the window that leaves a candidate the least room before
// its reset: the weekly one (name "") or a per-model one.
type roomWindow struct {
	name string
	// until is the time to the reset the room must be used by: the
	// window's own (own true) or, for a per-model window without one,
	// the weekly reset.
	until *time.Duration
	own   bool
	// upTo is the cap the room counts up to.
	upTo float64
	// ends names the schedule or override whose higher weekly cap ends
	// at until, before the reset ("" = a reset window).
	ends string
}

// timedWeekEnd is how long from now the timed weekly cap (a schedule's
// or an override's) still applies and what sets it. One that ends after
// the reset never wins in setUrgency: the room above the cap that
// follows is at most the whole room, and it has longer to be used.
func (c *Candidate) timedWeekEnd() (time.Duration, string, bool) {
	if c.Timed == nil || c.Timed.Week == nil || c.Timed.Week.Until == nil || !c.hasWeekReset || *c.UntilReset < 0 {
		return 0, "", false
	}
	now := c.WeeklyResetsAt.Add(-*c.UntilReset)
	end := c.Timed.Week.Until.Sub(now)
	if end < 0 {
		return 0, "", false
	}
	src := c.Timed.Week.Kind
	if c.Timed.Week.Name != "" {
		src += " " + c.Timed.Week.Name
	}
	return end, src, true
}

// perHour rounds a rate to two decimals for the report; a rate above 0
// shows at least minShownRate, so real room never reads as "needs 0%/h".
func perHour(v float64) float64 {
	r := math.Round(v*100) / 100
	if v > 0 && r < minShownRate {
		return minShownRate
	}
	return r
}

// minShownRate is the least rate the report shows for real room.
const minShownRate = 0.01

// markDeadline puts the candidate in the deadline group (rule 2 of the
// file comment): a usable member, under the urgency order, whose room
// must be used within DeadlineWindow (the reset of the window that
// leaves it the least room) with at least DeadlineMinRoom of it before
// that reset. The minimum counts the room at the reset, not the room
// now: a member whose cap is still rising has little room now and more
// to come, which is exactly the room that is lost if it waits.
func (c Config) markDeadline(cand *Candidate) {
	cand.Deadline = cand.Eligible && c.order() == OrderUrgency && c.DeadlineWindow > 0 && cand.hasUrgency &&
		*cand.window.until <= c.DeadlineWindow && cand.urgencyRoom >= c.DeadlineMinRoom
}

// byUrgency is the urgency order: the deadline group first, soonest
// reset first; then the rest by urgency, highest first; an unknown
// urgency sorts last; ties keep config order.
func byUrgency(a, b *Candidate) int {
	switch {
	case a.Deadline && !b.Deadline:
		return -1
	case b.Deadline && !a.Deadline:
		return 1
	case a.Deadline:
		if c := cmp.Compare(*a.window.until, *b.window.until); c != 0 {
			return c
		}
		return a.order - b.order
	}
	ua, ub := urgencyKey(a), urgencyKey(b)
	switch {
	case ua > ub:
		return -1
	case ua < ub:
		return 1
	}
	return a.order - b.order
}

// urgencyKey is the candidate's urgency, -1 when unknown (below any
// real urgency, which is never negative).
func urgencyKey(c *Candidate) float64 {
	if !c.hasUrgency {
		return -1
	}
	return c.urgency
}

// assign is the pick for one session (rule 3 of the file comment).
func assign(cfg Config, ready, waiting []*Candidate, o Options, ownUsable bool) *Selection {
	limit := cfg.MaxSessions
	full := func(c *Candidate) bool { return limit > 0 && c.Load >= limit }
	leaving := ""
	if cfg.Sticky && o.Current != "" {
		if i := slices.IndexFunc(ready, func(c *Candidate) bool { return c.Name == o.Current }); i >= 0 && (limit == 0 || o.CurrentAhead < limit) {
			// The session would leave for the first ready member that
			// can take it, so that member must be the one urgent enough:
			// a full member, or one held back by the borrow window, is
			// no reason to leave.
			j := slices.IndexFunc(ready, func(c *Candidate) bool { return c == ready[i] || !full(c) })
			if !outranked(cfg, ready[j], ready[i]) {
				return selection(ready[i], true, reasonSticky)
			}
			leaving = fmt.Sprintf("left %s: %s is at least %g times as urgent (sticky_leave_ratio); ", o.Current, ready[j].Name, cfg.StickyLeaveRatio)
		}
	}
	first := ready[0]
	for _, list := range [][]*Candidate{ready, waiting} {
		for _, c := range list {
			if full(c) {
				continue
			}
			reason := urgencyReason(cfg, c, c == first)
			if cfg.order() == OrderDeadline {
				reason = pickReason(c.Group, ownUsable, cfg.BorrowWindow)
			}
			switch {
			case c != first && c.State == StateWaiting:
				reason = fmt.Sprintf("the pool's first members each have %d sessions (max_sessions %d); %s, held back only by the borrow window", limit, limit, reason)
			case c != first:
				reason = fmt.Sprintf("%s has %d other sessions (max_sessions %d); next: %s", first.Name, first.Load, limit, reason)
			}
			return selection(c, false, leaving+reason)
		}
	}
	least := ready[0]
	for _, c := range ready {
		if c.Load < least.Load {
			least = c
		}
	}
	return selection(least, false, fmt.Sprintf("every usable member has %d or more other sessions (max_sessions %d); the least loaded", limit, limit))
}

// outranked reports whether a sticky session should leave cur for
// best: best's urgency is at least StickyLeaveRatio times cur's (an
// unknown urgency counts as 0; best must have one).
func outranked(cfg Config, best, cur *Candidate) bool {
	if cfg.StickyLeaveRatio <= 0 || best == cur || !best.hasUrgency || best.urgency <= 0 {
		return false
	}
	curUrgency := 0.0
	if cur.hasUrgency {
		curUrgency = cur.urgency
	}
	return best.urgency >= cfg.StickyLeaveRatio*curUrgency
}

// reasonSticky is a sticky pick's reason.
const reasonSticky = "this session's member is still in the pool; it stays on it (sticky), which keeps its prompt cache"

// urgencyReason says why an urgency pick comes where it does: in the
// deadline group its reset comes soonest, else it has the most room to
// lose per hour. Both name the room it has before its reset and the rate
// needed to use it all. A member without a known reset and room sorts
// last; first says it leads the pool.
func urgencyReason(cfg Config, c *Candidate, first bool) string {
	switch {
	case c.Urgency == nil && first:
		return "no member has a known weekly reset and room; first in config order"
	case c.Urgency == nil:
		return "its weekly reset or room is unknown; next in config order"
	case c.Deadline:
		return fmt.Sprintf("resets soonest: %s; its reset is within %s (deadline_window), so using it first loses nothing",
			roomText(c), Hours(cfg.DeadlineWindow))
	}
	return "most room to lose: " + roomText(c)
}

// roomText is a candidate's room before its reset in words, naming the
// window that leaves the least, with the rate needed to use it all.
func roomText(c *Candidate) string {
	w := c.window
	label, reset := "weekly", "the reset"
	if w.name != "" {
		label, reset = w.name, "that window's reset"
		if !w.own {
			reset = "the weekly reset"
		}
	}
	rate := rateText(*c.NeedsPerHour)
	if w.ends != "" {
		return fmt.Sprintf("%s%% left under its weekly cap of %s%% before its %s ends in %s (%s%%/h to use it all)",
			num(c.urgencyRoom), num(w.upTo), w.ends, hoursText(*w.until), rate)
	}
	if w.upTo > c.capNow(w) {
		return fmt.Sprintf("%s%% left before its %s cap reaches %s%% at %s in %s (%s%%/h to use it all)",
			num(c.urgencyRoom), label, num(w.upTo), reset, hoursText(*w.until), rate)
	}
	return fmt.Sprintf("%s%% left under its %s cap with %s to %s (%s%%/h to use it all)",
		num(c.urgencyRoom), label, hoursText(*w.until), reset, rate)
}

// capNow is the cap window w has now: the weekly cap, or the per-model
// window's.
func (c *Candidate) capNow(w roomWindow) float64 {
	for _, s := range c.Scoped {
		if s.Name == w.name {
			return s.Cap
		}
	}
	return c.Caps.Week
}

// rateText prints a rate with at most two decimals ("1.14", "0.01").
func rateText(v float64) string { return fmt.Sprintf("%g", v) }

// hoursText is a duration as hours with one decimal ("43.3h").
func hoursText(d time.Duration) string { return fmt.Sprintf("%sh", num(d.Hours())) }

// selection is the Selection of candidate c.
func selection(c *Candidate, sticky bool, reason string) *Selection {
	return &Selection{
		Name: c.Name, Group: c.Group, Sticky: sticky, Reason: reason, Warnings: []string{},
		WeeklyResetsAt: c.WeeklyResetsAt, Room: c.Room, Load: c.Load,
	}
}
