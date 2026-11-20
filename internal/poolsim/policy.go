package poolsim

import (
	"math"
	"slices"
	"time"

	"github.com/ubgo/agx/pool"
)

// Order is how a policy ranks the usable accounts inside one tier.
type Order int

// Orders.
const (
	// OrderReset is soonest weekly reset first (today's rule).
	OrderReset Order = iota
	// OrderUrgency is use-it-or-lose-it: the room under the weekly cap
	// divided by the hours to the reset, highest first.
	OrderUrgency
)

// Policy is the family of rules the simulator explores. Its zero knobs
// are today's rules (strategy A); every other strategy turns some on.
type Policy struct {
	// Own and Shared are the normal caps per group.
	Own, Shared pool.Caps
	// Cutoff is the 5-hour safety cutoff for every account.
	Cutoff float64
	// Borrow is the borrow window: a shared account goes before a usable
	// own account only when its week resets within it.
	Borrow time.Duration
	// Flat ranks own and shared accounts together (no borrow tiers).
	Flat bool
	// Order ranks accounts inside a tier.
	Order Order
	// Urgent (B) puts shared accounts whose week resets within Urgent.
	// Window in a first tier with looser caps.
	Urgent UrgentTier
	// Relax (C) raises a shared account's weekly cap smoothly as its
	// reset nears.
	Relax WeekRelax
	// Idle (D) loosens a shared account's 5-hour cap while its teammate
	// looks inactive.
	Idle IdleShort
	// Pace (F) holds an own account back while it is ahead of an even
	// spend over its week.
	Pace OwnPace
	// Spread (G) is the most busy sessions one account takes before the
	// next is used; 0 = every session on the one pick.
	Spread int
	// SpreadInTier keeps the spread inside the first tier: when its
	// accounts are full the least loaded of them takes the session,
	// rather than an account of a later tier.
	SpreadInTier bool
	// Weighted (W) ranks by room in work: the urgency times the
	// member's known capacity (plan size); unknown capacity counts as 1.
	Weighted bool
	// Override sets caps instead of the scenario's (sweeps).
	Override Overrides
	// Stick (H) keeps a session on its account while it is usable, not
	// near a cap and still in the best tier.
	Stick Hysteresis
}

// UrgentTier is strategy B's knobs; a zero Window is off.
type UrgentTier struct {
	Window      time.Duration
	Short, Week float64
}

// WeekRelax is strategy C's knobs; a zero Start is off. The weekly cap
// rises from the normal cap at Start before the reset to Max at the
// reset, linearly or in Steps.
type WeekRelax struct {
	Start time.Duration
	Max   float64
	// Steps > 0 rises in that many equal steps instead of linearly.
	Steps int
	// Short is a 5-hour cap the shared account gets at the reset,
	// rising the same way from the normal one; 0 keeps the normal cap.
	Short float64
}

// IdleShort is strategy D's knobs; a zero Short is off.
type IdleShort struct {
	// Short is the 5-hour cap of a shared account whose teammate looks
	// inactive.
	Short float64
	// ActiveFor is how long a rise of the 5-hour window we did not cause
	// keeps the teammate counted as active.
	ActiveFor time.Duration
}

// OwnPace is strategy F's knobs.
type OwnPace struct {
	On bool
	// Slack is how many percent ahead of the even line an own account
	// may be before it is held back.
	Slack float64
}

// Hysteresis is strategy H's knobs.
type Hysteresis struct {
	On bool
	// Ratio > 0 leaves the account when the best one's urgency (room ÷
	// hours to its reset) is at least Ratio times the current one's.
	Ratio float64
	// Margin is how close to a cap (percent) an account may get before a
	// session leaves it for a better one.
	Margin float64
}

// Tiers, in pick order.
// An own account ahead of its pace (F) goes after every shared account:
// pacing means own room is the last resort while it is ahead.
const (
	tierUrgent = iota
	tierMain
	tierShared
	tierOwnAhead
)

// policyPicker is a Policy with its memory of one run.
type policyPicker struct {
	p Policy
	// lastRise is when a shared account's 5-hour use last rose while
	// none of our sessions were on it (strategy D).
	lastRise map[string]time.Time
	prev     map[string]float64
	started  time.Time
}

// NewPolicy makes the policy's picker.
func NewPolicy(p Policy) Picker {
	return &policyPicker{p: p, lastRise: map[string]time.Time{}, prev: map[string]float64{}}
}

// option is one usable account in a step.
type option struct {
	name  string
	group pool.Group
	order int
	tier  int
	key   float64
	// urgency is room ÷ hours to the reset, whatever the order.
	urgency float64
	caps    pool.Caps
	short   float64
	week    float64
}

// Assign implements Picker. Each busy session asks in turn (session
// order, as each run asks agx for a key on its own): it sees where every
// other session is now, the ones that asked before it in this step at
// their new account and the rest at their last one.
func (pp *policyPicker) Assign(v View) []string {
	if pp.started.IsZero() {
		pp.started = v.Now
	}
	pp.observe(v)
	// A session on the per-model window's model sees that window too
	// (an extra weekly cap); the others pick from the all-models windows.
	lists := map[bool][]option{false: pp.options(v, false)}
	lists[true] = lists[false]
	if slices.ContainsFunc(v.Sessions, func(s SessionView) bool { return s.Scoped }) {
		lists[true] = pp.options(v, true)
	}
	out := make([]string, len(v.Sessions))
	pos := positions(v.Sessions)
	limit := pp.p.Spread
	for i, s := range v.Sessions {
		opts := lists[s.Scoped]
		if !s.Busy || len(opts) == 0 {
			continue
		}
		first := 1
		for first < len(opts) && opts[first].tier == opts[0].tier {
			first++
		}
		fill := opts
		if pp.p.SpreadInTier {
			fill = opts[:first]
		}
		load, ahead := others(pos, i, s.Current)
		pick := ""
		if j := slices.IndexFunc(opts, func(o option) bool { return o.name == s.Current }); pp.p.Stick.On && j >= 0 &&
			pp.keep(opts[j], opts[0]) && (limit <= 0 || ahead < limit) {
			pick = s.Current
		}
		if pick == "" {
			j := slices.IndexFunc(fill, func(o option) bool { return limit <= 0 || load[o.name] < limit })
			if j < 0 {
				// Every account is full: the least loaded of the first
				// tier takes the session.
				j = 0
				for k := range opts[:first] {
					if load[opts[k].name] < load[opts[j].name] {
						j = k
					}
				}
				pick = opts[j].name
			} else {
				pick = fill[j].name
			}
		}
		out[i] = pick
		pos[i] = pick
	}
	return out
}

// positions are the busy sessions' accounts as the step starts.
func positions(sessions []SessionView) map[int]string {
	pos := map[int]string{}
	for i, s := range sessions {
		if s.Busy && s.Current != "" {
			pos[i] = s.Current
		}
	}
	return pos
}

// others is how many other busy sessions are on each account, and how
// many of those on cur come before session i (they started earlier, so
// they keep it first).
func others(pos map[int]string, i int, cur string) (map[string]int, int) {
	load := map[string]int{}
	ahead := 0
	for j, a := range pos {
		if j == i {
			continue
		}
		load[a]++
		if a == cur && j < i {
			ahead++
		}
	}
	return load, ahead
}

// keep reports whether a session may stay on cur although best ranks
// first: cur is in the best tier and not within the margin of a cap.
func (pp *policyPicker) keep(cur, best option) bool {
	if cur.tier > best.tier {
		return false
	}
	if r := pp.p.Stick.Ratio; r > 0 && best.name != cur.name && best.urgency > 0 && best.urgency >= r*cur.urgency {
		return false
	}
	m := pp.p.Stick.Margin
	return cur.short < math.Min(cur.caps.Short, pp.p.Cutoff)-m && cur.week < cur.caps.Week-m
}

// observe updates strategy D's memory: a 5-hour rise on an account none
// of our sessions used in the last step is the teammate's.
func (pp *policyPicker) observe(v View) {
	for _, mb := range v.Members {
		rd, ok := v.Readings[mb.Name]
		if !ok {
			continue
		}
		cur := 0.0
		if rd.Short != nil {
			cur = rd.Short.Percent
		}
		if prev, seen := pp.prev[mb.Name]; seen && cur > prev && v.Load[mb.Name] == 0 {
			pp.lastRise[mb.Name] = v.Now
		}
		pp.prev[mb.Name] = cur
	}
}

// mateActive is strategy D's guess: a recent rise we did not cause, or
// too little history to know (counted as active, the safe side).
func (pp *policyPicker) mateActive(name string, now time.Time) bool {
	if now.Sub(pp.started) < pp.p.Idle.ActiveFor {
		return true
	}
	at, ok := pp.lastRise[name]
	return ok && now.Sub(at) <= pp.p.Idle.ActiveFor
}

// options are the usable accounts in pick order; with scoped, each
// reading's per-model windows are extra weekly caps (the shipped code's
// rule: blocked at the cap, the room the least over the windows).
func (pp *policyPicker) options(v View, scoped bool) []option {
	p := pp.p
	ownUsable := false
	var opts []option
	for i, mb := range v.Members {
		rd, ok := v.Readings[mb.Name]
		if !ok || rd.Status != pool.StatusOK {
			continue
		}
		until, known := untilReset(rd, v.Now)
		caps := pp.caps(mb, until, known, v.Now)
		short, week := percent(rd.Short), percent(rd.Week)
		if short >= p.Cutoff || short >= caps.Short || week >= caps.Week {
			continue
		}
		modelRoom, modelBlocked := math.Inf(1), false
		for _, w := range rd.Scoped {
			if !scoped {
				break
			}
			if w.ResetsAt != nil && w.ResetsAt.Before(v.Now) {
				// It has reset since the reading.
				continue
			}
			modelBlocked = modelBlocked || w.Percent >= caps.Week
			modelRoom = math.Min(modelRoom, caps.Week-w.Percent)
		}
		if modelBlocked {
			continue
		}
		o := option{name: mb.Name, group: mb.Group, order: i, caps: caps, short: short, week: week}
		inWindow := known && until >= 0 && until <= p.Borrow
		switch {
		case mb.Group == pool.GroupShared && p.Urgent.Window > 0 && known && until >= 0 && until <= p.Urgent.Window:
			o.tier = tierUrgent
		case p.Flat:
			o.tier = tierMain
		case mb.Group == pool.GroupOwn && p.Pace.On && ahead(week, caps.Week, until, known, p.Pace.Slack):
			o.tier = tierOwnAhead
		case mb.Group == pool.GroupOwn || inWindow:
			o.tier = tierMain
		default:
			o.tier = tierShared
		}
		if mb.Group == pool.GroupOwn && o.tier == tierMain {
			ownUsable = true
		}
		room := math.Min(caps.Week-week, modelRoom)
		if p.Weighted && mb.Capacity > 0 {
			// Room in work, not percent: a bigger plan's percent is
			// worth more sessions.
			room *= mb.Capacity
		}
		o.key = sortKey(p.Order, room, until, known)
		if known && until >= 0 {
			o.urgency = room / math.Max(until.Hours(), minUrgencyHours)
		}
		opts = append(opts, o)
	}
	if !ownUsable && !p.Flat {
		// Today's rule: with no own account usable, every shared account
		// competes on its reset alone.
		for i := range opts {
			if opts[i].tier == tierShared {
				opts[i].tier = tierMain
			}
		}
	}
	slices.SortStableFunc(opts, func(a, b option) int {
		switch {
		case a.tier != b.tier:
			return a.tier - b.tier
		case a.key < b.key:
			return -1
		case a.key > b.key:
			return 1
		}
		return a.order - b.order
	})
	return opts
}

// caps are an account's caps now under the policy. The weekly relax
// applies to any member whose weekly cap is below the relax max, as in
// package pool; the 5-hour relax, the urgent tier and the idle cap only
// to shared members.
func (pp *policyPicker) caps(mb Member, until time.Duration, known bool, now time.Time) pool.Caps {
	p := pp.p
	base := p.Shared
	if mb.Group == pool.GroupOwn {
		base = p.Own
	}
	c := base
	inRelax := p.Relax.Start > 0 && known && until >= 0 && until < p.Relax.Start
	if inRelax {
		f := 1 - until.Hours()/p.Relax.Start.Hours()
		if p.Relax.Steps > 0 {
			f = math.Floor(f*float64(p.Relax.Steps)) / float64(p.Relax.Steps)
		}
		if p.Relax.Max > base.Week {
			c.Week = base.Week + (p.Relax.Max-base.Week)*f
		}
		if p.Relax.Short > base.Short && mb.Group == pool.GroupShared {
			c.Short = base.Short + (p.Relax.Short-base.Short)*f
		}
	}
	if mb.Group == pool.GroupOwn {
		return c
	}
	if p.Urgent.Window > 0 && known && until >= 0 && until <= p.Urgent.Window {
		c.Short = math.Max(c.Short, p.Urgent.Short)
		c.Week = math.Max(c.Week, p.Urgent.Week)
	}
	if p.Idle.Short > 0 && !pp.mateActive(mb.Name, now) {
		c.Short = math.Max(c.Short, p.Idle.Short)
	}
	return c
}

// ahead reports an own account spending faster than evenly over its
// week: its use is more than slack above cap × the part of the week
// gone.
func ahead(week, capWeek float64, until time.Duration, known bool, slack float64) bool {
	if !known || until < 0 {
		return false
	}
	gone := 1 - math.Min(1, until.Hours()/WeekPeriod.Hours())
	return week > capWeek*gone+slack
}

// sortKey is the in-tier rank (lower first); an unknown or passed reset
// sorts last.
func sortKey(o Order, room float64, until time.Duration, known bool) float64 {
	if !known || until < 0 {
		return math.MaxFloat64
	}
	if o == OrderUrgency {
		return -room / math.Max(until.Hours(), minUrgencyHours)
	}
	return until.Hours()
}

// minUrgencyHours keeps a reset minutes away from dividing by zero.
const minUrgencyHours = 0.25

// untilReset is the time to the weekly reset, if the reading has one.
func untilReset(rd pool.Reading, now time.Time) (time.Duration, bool) {
	if rd.Week == nil || rd.Week.ResetsAt == nil || rd.Week.ResetsAt.IsZero() {
		return 0, false
	}
	return rd.Week.ResetsAt.Sub(now), true
}

// percent is a window's use, 0 when it is not reported.
func percent(w *pool.Window) float64 {
	if w == nil {
		return 0
	}
	return w.Percent
}

// poolPicker runs the real rules (package pool) every step: each busy
// session asks in turn with its own load, current member and (on the
// per-model window's model) its own readings, as each run's key request
// does; with max_sessions 0 and sticky off (the classic preset) every
// session gets the same pick.
type poolPicker struct {
	cfg pool.Config
	// blind leaves out every per-model window.
	blind bool
}

// NewPoolRules is the real pool.Select as a picker.
func NewPoolRules(cfg pool.Config) Picker { return poolPicker{cfg: cfg} }

// NewPoolRulesBlind is NewPoolRules without the per-model windows: what
// agx did before it read them (every session sees the all-models
// windows only).
func NewPoolRulesBlind(cfg pool.Config) Picker { return poolPicker{cfg: cfg, blind: true} }

// withoutScoped is readings with every per-model window left out.
func withoutScoped(readings map[string]pool.Reading) map[string]pool.Reading {
	out := make(map[string]pool.Reading, len(readings))
	for k, r := range readings {
		r.Scoped = nil
		out[k] = r
	}
	return out
}

// Assign implements Picker. A session not on the per-model window's
// model (or every session, blind) picks from readings without it.
func (pp poolPicker) Assign(v View) []string {
	out := make([]string, len(v.Sessions))
	pos := positions(v.Sessions)
	unscoped := withoutScoped(v.Readings)
	for i, s := range v.Sessions {
		if !s.Busy {
			continue
		}
		load, ahead := others(pos, i, s.Current)
		readings := v.Readings
		if pp.blind || !s.Scoped {
			readings = unscoped
		}
		res, err := pool.Select(pp.cfg, readings, pool.Options{Now: v.Now, Load: load, Current: s.Current, CurrentAhead: ahead})
		if err != nil || res.Selected == nil {
			continue
		}
		out[i] = res.Selected.Name
		pos[i] = out[i]
	}
	return out
}
