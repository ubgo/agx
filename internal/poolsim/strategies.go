package poolsim

import (
	"time"

	"github.com/ubgo/agx/pool"
)

// Today's rules as a Policy (strategy A): own 95/98, shared 50/70, the
// 99% cutoff, the 48h borrow window, soonest reset first.
func Today() Policy {
	return Policy{
		Own:    pool.Caps{Short: ownShortCap, Week: ownWeekCap},
		Shared: pool.Caps{Short: pool.DefaultShortCap, Week: pool.DefaultWeekCap},
		Cutoff: pool.DefaultShortUnavailableAt, Borrow: pool.DefaultBorrowWindow,
	}
}

// TodayConfig is today's rules as the real pool.Config of the members.
func TodayConfig(members []Member) pool.Config {
	var own, shared []string
	accounts := map[string]pool.Account{}
	for _, m := range members {
		if m.Group == pool.GroupOwn {
			own = append(own, m.Name)
			s, w := ownShortCap, ownWeekCap
			accounts[m.Name] = pool.Account{Short: &s, Week: &w}
			continue
		}
		shared = append(shared, m.Name)
	}
	c := pool.New(own, shared)
	c.Accounts = accounts
	return c
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
	spreadPer     = 3
	stickMargin   = 0.0
)

// policy wraps a Policy as a Strategy.
func policy(name, about string, p Policy) Strategy {
	return Strategy{Name: name, About: about, New: func() Picker { return NewPolicy(p) }}
}

// Strategies are the named strategies of the brief (A to H), the
// recommendation (REC) and its ablations (REC without one part, REC with
// one more).
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
		policy("C", "A + shared weekly cap rising from 70 to 95 over the last 24h", from(Today(), relax)),
		policy("D", "A + 5h cap 80 while the teammate looks idle (no rise we did not cause for 2h)", from(Today(), idle)),
		policy("E", "use-it-or-lose-it: room / hours to reset, own and shared ranked together", from(Today(), flat)),
		policy("E2", "A with use-it-or-lose-it order inside the borrow tiers", from(Today(), urgency)),
		policy("F", "A + own accounts paced (10% slack), ahead = last resort", from(Today(), pace)),
		policy("G", "A + spread: at most 3 busy sessions per account", from(Today(), spread)),
		policy("H", "A + sticky: stay while usable and in the first tier", from(Today(), stick)),
		policy("REC", "recommended: C + E2 + G + H", rec),
		policy("REC-C", "REC without the weekly relax", from(rec, func(p *Policy) { p.Relax = WeekRelax{} })),
		policy("REC-E2", "REC with soonest-reset order", from(rec, func(p *Policy) { p.Order = OrderReset })),
		policy("REC-G", "REC without spread", from(rec, func(p *Policy) { p.Spread = 0 })),
		policy("REC-H", "REC without sticky", from(rec, func(p *Policy) { p.Stick = Hysteresis{} })),
		policy("REC+B", "REC + urgent tier", from(rec, urgent)),
		policy("REC+D", "REC + idle 5h cap", from(rec, idle)),
		policy("REC+F", "REC + own pacing", from(rec, pace)),
		policy("REC+T", "REC with the spread kept in the first tier", from(rec, func(p *Policy) { p.SpreadInTier = true })),
	}
}

// Recommended is the recommended policy: today's caps and tiers, the
// shared weekly cap rising from its cap to 95 over the last 24h before
// the reset, use-it-or-lose-it order, at most 3 busy sessions per
// account, and sessions kept on their account while it is usable and in
// the first tier.
func Recommended() Policy {
	p := Today()
	p.Relax = WeekRelax{Start: relaxStart, Max: relaxMax}
	p.Order = OrderUrgency
	p.Spread = spreadPer
	p.Stick = Hysteresis{On: true, Margin: stickMargin}
	return p
}

// RealToday runs the real pool.Select with today's config, to check the
// Policy's A against the shipped code.
func RealToday(members []Member) Strategy {
	return Strategy{Name: "A-real", About: "pool.Select with today's config", New: func() Picker { return NewPoolRules(TodayConfig(members)) }}
}

// Candidate is the policy the sweeps vary one knob at a time.
func Candidate() Policy { return Recommended() }

// SweepScenarios are the scenarios a sweep scores on: the realistic
// ones and the stress cases where the knobs matter.
func SweepScenarios() []Generator {
	var out []Generator
	for _, g := range Scenarios() {
		switch g.Name {
		case "snapshot", "snapshot-busy", "normal", "heavy", "mate-on-preferred", "near-caps":
			out = append(out, g)
		}
	}
	return out
}

// RecommendedConfig is the recommendation as the real pool.Config of
// the members: today's caps with the expiring strategy at its defaults.
func RecommendedConfig(members []Member) pool.Config {
	c := TodayConfig(members)
	c.Strategy = pool.StrategyExpiring
	return c
}

// RealRecommended runs the real pool.Select with the recommended config,
// to check the shipped expiring strategy against the Policy's REC.
func RealRecommended(members []Member) Strategy {
	return Strategy{Name: "REC-real", About: "pool.Select with the expiring strategy", New: func() Picker { return NewPoolRules(RecommendedConfig(members)) }}
}
