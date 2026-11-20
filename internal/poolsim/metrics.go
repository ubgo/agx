package poolsim

import (
	"math"
	"slices"

	"github.com/ubgo/agx/pool"
)

// Metrics are one run's scores, normalised so runs of any length
// compare: per week, per own account-week, per session-day.
type Metrics struct {
	Weeks    float64 `json:"weeks"`
	Sessions int     `json:"sessions"`
	Accounts int     `json:"accounts"`
	// WasteShared and WasteOwn are the weekly quota left unused when a
	// weekly window reset (100 minus the week's use), averaged over the
	// account-weeks of each group weighted by plan capacity (a Max 20x
	// account's percent is four times a Max 5x one's work);
	// WasteTotal over every account-week. Goal 1.
	WasteShared float64 `json:"waste_shared"`
	WasteOwn    float64 `json:"waste_own"`
	WasteTotal  float64 `json:"waste_total"`
	// Hits429 are steps in which one of our sessions was refused because
	// its account reached a limit, per week. Goal 2.
	Hits429 float64 `json:"hits_429"`
	// StallHours are busy session-hours with no usable account, per
	// week (the pool handed out no key).
	StallHours float64 `json:"stall_hours"`
	// SqueezeHours are hours a shared account we were using was above
	// its normal caps while its teammate was active, per week; Lockout
	// hours are the teammate's refused hours that our use caused (they
	// alone would have stayed under the limit), per week. Goal 3.
	SqueezeHours float64 `json:"squeeze_hours"`
	LockoutHours float64 `json:"lockout_hours"`
	// LockoutWeekHours is the part of LockoutHours where the weekly
	// limit was the one reached.
	LockoutWeekHours float64 `json:"lockout_week_hours"`
	// OwnRoom is the average room an own account had left under its
	// normal weekly cap when its week reset; OwnDrainedHours the average
	// hours per own account-week it sat at that cap before the reset.
	// Goal 4.
	OwnRoom         float64 `json:"own_room"`
	OwnDrainedHours float64 `json:"own_drained_hours"`
	// Switches are account changes per session per day. Goal 5.
	Switches float64 `json:"switches"`
	// Served is the part of our busy session-hours that got quota.
	Served float64 `json:"served"`

	// Resets are every weekly reset of the run, in time order, for the
	// per-account breakdown.
	Resets []ResetRecord `json:"-"`

	// Raw counters, normalised by finish.
	DemandHours, ServedHours float64 `json:"-"`
	ownResets, sharedResets  float64
	ownCap, sharedCap        float64
}

// ResetRecord is one account's weekly reset: how much of the week was
// left and how much of it we had used.
type ResetRecord struct {
	Account string
	Left    float64
	Ours    float64
}

// reset records one account's weekly reset.
func (m *Metrics) reset(a *account, sc Scenario) {
	left := math.Max(0, Full-a.week)
	m.Resets = append(m.Resets, ResetRecord{Account: a.Name, Left: left, Ours: a.ourWeek})
	capacity := a.Capacity
	if capacity <= 0 {
		capacity = 1
	}
	if a.Group == pool.GroupOwn {
		m.WasteOwn += left * capacity
		m.ownCap += capacity
		m.OwnRoom += math.Max(0, sc.BaseCaps[pool.GroupOwn].Week-a.week)
		m.ownResets++
		return
	}
	m.WasteShared += left * capacity
	m.sharedCap += capacity
	m.sharedResets++
}

// finish turns the raw sums into the reported rates.
func (m *Metrics) finish(sc Scenario) {
	weeks := m.Weeks
	days := sc.Horizon.Hours() / hoursPerDay
	own := 0.0
	for _, a := range sc.Accounts {
		if a.Group == pool.GroupOwn {
			own++
		}
	}
	ownWeeks := own * weeks
	if total := m.sharedCap + m.ownCap; total > 0 {
		m.WasteTotal = (m.WasteShared + m.WasteOwn) / total
	}
	if m.sharedCap > 0 {
		m.WasteShared /= m.sharedCap
	}
	if m.ownCap > 0 {
		m.WasteOwn /= m.ownCap
	}
	m.Hits429 /= weeks
	m.StallHours /= weeks
	m.SqueezeHours /= weeks
	m.LockoutHours /= weeks
	m.LockoutWeekHours /= weeks
	if m.ownResets > 0 {
		m.OwnRoom /= m.ownResets
	}
	if ownWeeks > 0 {
		m.OwnDrainedHours /= ownWeeks
	}
	if sc.Load.Sessions > 0 {
		m.Switches /= float64(sc.Load.Sessions) * days
	}
	m.Served = 1
	if m.DemandHours > 0 {
		m.Served = m.ServedHours / m.DemandHours
	}
}

// hoursPerDay converts a horizon to days.
const hoursPerDay = 24.0

// Summary is a strategy's metrics over many seeds of one scenario: the
// mean and the worst run of each metric.
type Summary struct {
	Strategy string  `json:"strategy"`
	Scenario string  `json:"scenario"`
	Runs     int     `json:"runs"`
	Mean     Metrics `json:"mean"`
	Worst    Metrics `json:"worst"`
	// Score is the weighted cost (Weights) of the mean; lower is better.
	Score float64 `json:"score"`
}

// field reads and writes one metric, so mean and worst are one loop.
type field struct {
	get func(*Metrics) *float64
	// higherWorse is true for every cost; Served is the one metric
	// whose worst is its lowest value.
	higherWorse bool
}

// fields are the summarised metrics.
var fields = []field{
	{func(m *Metrics) *float64 { return &m.WasteShared }, true},
	{func(m *Metrics) *float64 { return &m.WasteOwn }, true},
	{func(m *Metrics) *float64 { return &m.WasteTotal }, true},
	{func(m *Metrics) *float64 { return &m.Hits429 }, true},
	{func(m *Metrics) *float64 { return &m.StallHours }, true},
	{func(m *Metrics) *float64 { return &m.SqueezeHours }, true},
	{func(m *Metrics) *float64 { return &m.LockoutHours }, true},
	{func(m *Metrics) *float64 { return &m.LockoutWeekHours }, true},
	{func(m *Metrics) *float64 { return &m.OwnRoom }, true},
	{func(m *Metrics) *float64 { return &m.OwnDrainedHours }, true},
	{func(m *Metrics) *float64 { return &m.Switches }, true},
	{func(m *Metrics) *float64 { return &m.Served }, false},
}

// Summarise folds runs into a Summary.
func Summarise(strategy, scenario string, runs []Metrics, w Weights) Summary {
	s := Summary{Strategy: strategy, Scenario: scenario, Runs: len(runs)}
	if len(runs) == 0 {
		return s
	}
	s.Mean.Weeks, s.Worst.Weeks = runs[0].Weeks, runs[0].Weeks
	s.Mean.Sessions, s.Worst.Sessions = runs[0].Sessions, runs[0].Sessions
	s.Mean.Accounts, s.Worst.Accounts = runs[0].Accounts, runs[0].Accounts
	for _, f := range fields {
		vals := make([]float64, len(runs))
		sum := 0.0
		for i := range runs {
			vals[i] = *f.get(&runs[i])
			sum += vals[i]
		}
		*f.get(&s.Mean) = sum / float64(len(runs))
		if f.higherWorse {
			*f.get(&s.Worst) = slices.Max(vals)
		} else {
			*f.get(&s.Worst) = slices.Min(vals)
		}
	}
	s.Score = w.Cost(s.Mean)
	return s
}

// Weights turn the metrics into one cost to rank strategies and tune
// knobs. Waste is the main goal (weight 1 per percent of one account
// wasted, summed over the pool's accounts: average waste per
// account-week times the account count);
// the others are priced in the same unit: what one event is worth in
// wasted percent.
type Weights struct {
	Hit429, Stall, Squeeze, Lockout, OwnDrained, Switch float64
}

// DefaultWeights: a 429 costs as much as 2% of an account wasted, a
// stalled session-hour 1.5% (it is quota we needed and did not get),
// a squeezed teammate-hour 3%, a teammate locked out by us 25% an hour
// (close to forbidden), an hour an own account sat drained 0.5%, and a
// switch per session-day 2% (prompt-cache reuse lost).
var DefaultWeights = Weights{Hit429: 2, Stall: 1.5, Squeeze: 3, Lockout: 25, OwnDrained: 0.5, Switch: 2}

// Cost is the weighted cost of one set of metrics.
func (w Weights) Cost(m Metrics) float64 {
	return m.WasteTotal*float64(m.Accounts) + w.Hit429*m.Hits429 + w.Stall*m.StallHours + w.Squeeze*m.SqueezeHours +
		w.Lockout*m.LockoutHours + w.OwnDrained*m.OwnDrainedHours + w.Switch*m.Switches
}
