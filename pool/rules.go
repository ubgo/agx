package pool

import (
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
//  2. Order: OrderUrgency sorts the pool by the room under the weekly
//     cap divided by the hours to the reset (the rate needed to use it
//     all), highest first; OrderDeadline by the soonest reset. An
//     unknown or passed reset sorts last.
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
}

// Rules are c's rule values.
func (c Config) Rules() Rules {
	return Rules{Order: c.order(), BorrowWindow: c.BorrowWindow, WeekRelaxWindow: c.WeekRelaxWindow, WeekRelaxMax: c.WeekRelaxMax,
		MaxSessions: c.MaxSessions, Sticky: c.Sticky, StickyLeaveRatio: c.StickyLeaveRatio}
}

// WithRules is c with r's rule values.
func (c Config) WithRules(r Rules) Config {
	c.Order, c.BorrowWindow, c.WeekRelaxWindow, c.WeekRelaxMax = r.Order, r.BorrowWindow, r.WeekRelaxWindow, r.WeekRelaxMax
	c.MaxSessions, c.Sticky, c.StickyLeaveRatio = r.MaxSessions, r.Sticky, r.StickyLeaveRatio
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
		MaxSessions: DefaultMaxSessions, Sticky: DefaultSticky, StickyLeaveRatio: DefaultStickyLeaveRatio}
}

// Presets are the named rule sets, recommended first.
func Presets() []Preset {
	classic := DefaultRules()
	classic.Order, classic.WeekRelaxWindow, classic.MaxSessions, classic.Sticky, classic.StickyLeaveRatio = OrderDeadline, 0, 0, false, 0
	return []Preset{
		{Name: PresetRecommended, About: "the defaults: urgency order, weekly cap rising to 95% over the last 24h, at most 4 sessions per member, sticky sessions that leave for a member 3 times as urgent", Rules: DefaultRules()},
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

// relax raises c's weekly cap for its reading's reset and notes it.
func (c Config) relax(cand *Candidate, rd Reading, now time.Time) {
	if rd.Week == nil || rd.Week.ResetsAt == nil || rd.Week.ResetsAt.IsZero() {
		return
	}
	configured := cand.Caps.Week
	relaxMax := c.RelaxMaxFor(cand.Name)
	week := RelaxedWeek(configured, relaxMax, c.WeekRelaxWindow, rd.Week.ResetsAt.Sub(now))
	if week == configured {
		return
	}
	cand.Caps.Week = week
	cand.WeekCapConfigured = ptr(configured)
	cand.Notes = append(cand.Notes, fmt.Sprintf("weekly cap %s%% rises to %s%% over the last %s before the reset: %s%% now",
		num(configured), num(relaxMax), Hours(c.WeekRelaxWindow), num(week)))
}

// setUrgency sets the candidate's room and urgency. The room is the
// room under the weekly cap, or less when a per-model window leaves less
// (scoped.go); the urgency is that room ÷ hours to that window's reset,
// in percent per hour, times the member's capacity. It stays unset when
// the room or a future reset is unknown. Urgency shows it rounded; the
// order uses the exact value.
func setUrgency(c *Candidate, rd Reading, capacity float64) {
	if !readable(rd.Week) {
		return
	}
	room, until := math.Max(0, c.Caps.Week-rd.Week.Percent), c.UntilReset
	if !c.hasWeekReset {
		until = nil
	}
	for _, s := range c.Scoped {
		if s.until != nil && s.Room < room {
			room, until = s.Room, s.until
		}
	}
	c.Room = ptr(round(room))
	c.roomUntil = until
	if until == nil || *until < 0 {
		return
	}
	c.urgency = room * capacity / math.Max(until.Hours(), minUrgencyHours)
	c.hasUrgency = true
	c.Urgency = ptr(math.Round(c.urgency*100) / 100)
}

// byUrgency orders by urgency, highest first; an unknown urgency sorts
// last; ties keep config order.
func byUrgency(a, b *Candidate) int {
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
			if !outranked(cfg, ready[0], ready[i]) {
				return selection(ready[i], true, reasonSticky)
			}
			leaving = fmt.Sprintf("left %s: %s is at least %g times as urgent (sticky_leave_ratio); ", o.Current, ready[0].Name, cfg.StickyLeaveRatio)
		}
	}
	first := ready[0]
	for _, list := range [][]*Candidate{ready, waiting} {
		for _, c := range list {
			if full(c) {
				continue
			}
			reason := urgencyReason(c)
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

// urgencyReason says why an urgency pick comes first.
func urgencyReason(c *Candidate) string {
	if c.Urgency == nil {
		return "no member has a known weekly reset and room; first in config order"
	}
	return fmt.Sprintf("most room to lose: %s%% left under its weekly cap with %s to the reset (%s%%/h to use it all)",
		num(*c.Room), hoursText(*c.roomUntil), num(*c.Urgency))
}

// hoursText is a duration as hours with one decimal ("43.3h").
func hoursText(d time.Duration) string { return fmt.Sprintf("%sh", num(d.Hours())) }

// selection is the Selection of candidate c.
func selection(c *Candidate, sticky bool, reason string) *Selection {
	return &Selection{
		Name: c.Name, Group: c.Group, Sticky: sticky, Reason: reason, Warnings: []string{},
		WeeklyResetsAt: c.WeeklyResetsAt, Room: c.Room, Load: c.Load,
	}
}
