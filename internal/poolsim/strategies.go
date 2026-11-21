package poolsim

import (
	"time"

	"github.com/ubgo/agx/pool"
)

// Today's rules as a Policy (strategy A): the pool's configured caps
// (filled per scenario), the 99% cutoff, the 48h borrow window, soonest
// reset first.
func Today() Policy {
	return Policy{Cutoff: pool.DefaultShortUnavailableAt, Borrow: pool.DefaultBorrowWindow}
}

// members are a scenario's members as pickers see them.
func members(sc Scenario) []Member {
	out := make([]Member, len(sc.Accounts))
	for i, a := range sc.Accounts {
		out[i] = Member{Name: a.Name, Group: a.Group, Capacity: knownCapacity(a)}
	}
	return out
}

// TodayConfig is a scenario's pool as the real pool.Config under the
// classic preset (the rules before v0.4.16, strategy A), made only of
// config values: its members, its configured caps per group and the
// preset's rule values.
func TodayConfig(sc Scenario) pool.Config {
	var own, shared []string
	for _, m := range members(sc) {
		if m.Group == pool.GroupOwn {
			own = append(own, m.Name)
		} else {
			shared = append(shared, m.Name)
		}
	}
	classic, _ := pool.PresetNamed(pool.PresetClassic)
	c := pool.New(own, shared).WithRules(classic.Rules)
	o, s := sc.BaseCaps[pool.GroupOwn], sc.BaseCaps[pool.GroupShared]
	c.Groups = map[pool.Group]pool.GroupCaps{
		pool.GroupOwn:    {Short: &o.Short, Week: &o.Week},
		pool.GroupShared: {Short: &s.Short, Week: &s.Week},
	}
	c.Location, c.Schedules = sc.Location, sc.Schedules
	return c
}

// RecommendedConfig is the recommendation as the real pool.Config: the
// scenario's caps with every rule value at its default.
func RecommendedConfig(sc Scenario) pool.Config {
	return TodayConfig(sc).WithRules(pool.DefaultRules())
}

// Knob values of the named strategies (the sweep tunes them).
const (
	urgentWindow  = 6 * time.Hour
	urgentShort   = 80.0
	urgentWeek    = 95.0
	relaxStart    = 24 * time.Hour
	relaxMax      = 95.0
	idleShort     = 80.0
	idleActiveFor = 2 * time.Hour
	paceSlack     = 10.0
	spreadPer     = 4
	stickMargin   = 0.0
	stickRatio    = 3.0
)

// Overrides are caps a sweep sets instead of the scenario's (0 = the
// scenario's).
type Overrides struct {
	OwnWeek, SharedShort, SharedWeek float64
}

// policy wraps a Policy as a Strategy: the scenario's configured caps
// fill the Policy's caps, then any override.
func policy(name, about string, p Policy) Strategy {
	return Strategy{Name: name, About: about, New: func(sc Scenario) Picker {
		q := p
		q.Own, q.Shared = sc.BaseCaps[pool.GroupOwn], sc.BaseCaps[pool.GroupShared]
		if q.Override.OwnWeek > 0 {
			q.Own.Week = q.Override.OwnWeek
		}
		if q.Override.SharedShort > 0 {
			q.Shared.Short = q.Override.SharedShort
		}
		if q.Override.SharedWeek > 0 {
			q.Shared.Week = q.Override.SharedWeek
		}
		return NewPolicy(q)
	}}
}

// Strategies are the named strategies of the brief (A to H), the
// recommendation (REC), its ablations and additions, and the shipped
// code (A-real, REC-real).
func Strategies() []Strategy {
	urgent := func(p *Policy) { p.Urgent = UrgentTier{Window: urgentWindow, Short: urgentShort, Week: urgentWeek} }
	relax := func(p *Policy) { p.Relax = WeekRelax{Start: relaxStart, Max: relaxMax} }
	idle := func(p *Policy) { p.Idle = IdleShort{Short: idleShort, ActiveFor: idleActiveFor} }
	urgency := func(p *Policy) { p.Order = OrderUrgency }
	flat := func(p *Policy) { p.Flat, p.Order = true, OrderUrgency }
	pace := func(p *Policy) { p.Pace = OwnPace{On: true, Slack: paceSlack} }
	spread := func(p *Policy) { p.Spread = spreadPer }
	stick := func(p *Policy) { p.Stick = Hysteresis{On: true, Margin: stickMargin} }
	from := func(base Policy, fns ...func(*Policy)) Policy {
		for _, fn := range fns {
			fn(&base)
		}
		return base
	}
	rec := Recommended()
	return []Strategy{
		policy("A", "today's rules (control)", Today()),
		policy("B", "A + urgent tier: shared reset within 6h, caps 80/95", from(Today(), urgent)),
		policy("C", "A + weekly cap rising to 95 over the last 24h", from(Today(), relax)),
		policy("D", "A + 5h cap 80 while the teammate looks idle (no rise we did not cause for 2h)", from(Today(), idle)),
		policy("E", "use-it-or-lose-it: room / hours to reset, own and shared ranked together", from(Today(), flat)),
		policy("E2", "A with use-it-or-lose-it order inside the borrow tiers", from(Today(), urgency)),
		policy("F", "A + own accounts paced (10% slack), ahead = last resort", from(Today(), pace)),
		policy("G", "A + spread: at most 4 busy sessions per account", from(Today(), spread)),
		policy("H", "A + sticky: stay while usable and in the first tier", from(Today(), stick)),
		policy("REC", "recommended: C + E2 + G + H", rec),
		policy("REC-C", "REC without the weekly relax", from(rec, func(p *Policy) { p.Relax = WeekRelax{} })),
		policy("REC-E2", "REC with soonest-reset order", from(rec, func(p *Policy) { p.Order = OrderReset })),
		policy("REC-G", "REC without spread", from(rec, func(p *Policy) { p.Spread = 0 })),
		policy("REC-H", "REC without sticky", from(rec, func(p *Policy) { p.Stick = Hysteresis{} })),
		policy("REC-L", "REC with sticky sessions that never leave for a more urgent member", from(rec, func(p *Policy) { p.Stick.Ratio = 0 })),
		policy("REC+B", "REC + urgent tier", from(rec, urgent)),
		policy("REC+D", "REC + idle 5h cap", from(rec, idle)),
		policy("REC+F", "REC + own pacing", from(rec, pace)),
		policy("REC+W", "REC ranking by room in work (plan size known)", from(rec, func(p *Policy) { p.Weighted = true })),
		RealToday(), RealRecommended(), RealWeighted(), RealBlind(),
	}
}

// Recommended is the recommended policy: the configured caps and
// tiers, the weekly cap rising to 95 over the last 24h before the
// reset, use-it-or-lose-it order, at most 4 busy sessions per account,
// and sessions kept on their account while it is usable and in the
// first tier.
func Recommended() Policy {
	p := Today()
	p.Relax = WeekRelax{Start: relaxStart, Max: relaxMax}
	p.Order = OrderUrgency
	p.Spread = spreadPer
	p.Stick = Hysteresis{On: true, Margin: stickMargin, Ratio: stickRatio}
	return p
}

// Candidate is the policy the sweeps vary one knob at a time.
func Candidate() Policy { return Recommended() }

// RealToday runs the real pool.Select with today's config, to check the
// Policy's A against the shipped code.
func RealToday() Strategy {
	return Strategy{Name: "A-real", About: "pool.Select, the classic preset", New: func(sc Scenario) Picker { return NewPoolRules(TodayConfig(sc)) }}
}

// RealRecommended runs the real pool.Select with the default rules, to
// check it against the Policy's REC.
func RealRecommended() Strategy {
	return Strategy{Name: "REC-real", About: "pool.Select, the default rules", New: func(sc Scenario) Picker { return NewPoolRules(RecommendedConfig(sc)) }}
}

// RealBlind runs the real pool.Select with the default rules but
// without the per-model windows (agx before it read them), to show what
// they save in a scenario with one.
func RealBlind() Strategy {
	return Strategy{Name: "REC-blind", About: "pool.Select, the defaults without per-model windows", New: func(sc Scenario) Picker { return NewPoolRulesBlind(RecommendedConfig(sc)) }}
}

// knownCapacity is what a picker may know of an account's plan size:
// its capacity when the user told agx, else 0 (unknown).
func knownCapacity(a Account) float64 {
	if !a.CapacityKnown {
		return 0
	}
	return a.Capacity
}

// WeightedConfig is RecommendedConfig with each member's capacity set
// where the scenario's user knows the plan (accounts.NAME.capacity).
func WeightedConfig(sc Scenario) pool.Config {
	c := RecommendedConfig(sc)
	c.Accounts = map[string]pool.Account{}
	for _, a := range sc.Accounts {
		if capacity := knownCapacity(a); capacity > 0 {
			c.Accounts[a.Name] = pool.Account{Capacity: &capacity}
		}
	}
	return c
}

// RealWeighted runs the real pool.Select with the default rules and the
// known plan sizes as capacities, to check it against REC+W.
func RealWeighted() Strategy {
	return Strategy{Name: "REC+W-real", About: "pool.Select, the defaults with capacities where plans are known", New: func(sc Scenario) Picker { return NewPoolRules(WeightedConfig(sc)) }}
}
