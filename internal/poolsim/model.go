// Package poolsim is an offline simulator of a login pool: our sessions
// and teammates spending the plan quota of a set of accounts over one or
// two weeks, with a picker deciding every step which account each of our
// sessions uses. It scores a picker on the pool's goals (weekly quota
// wasted at resets, our sessions hitting a limit, teammates squeezed, own
// accounts drained early, account switches) so pickers and their knobs
// can be compared on the same randomised weeks.
//
// Everything is deterministic: a scenario and a seed give the same run.
// No network, no clock, no files: a picker sees only what the real pool
// code sees (rounded cached percentages, reset times, the age of a
// reading), never the simulator's hidden state.
package poolsim

import (
	"time"

	"github.com/ubgo/agx/pool"
)

// Time constants of Anthropic's unified limits.
const (
	// ShortWindow is the rolling 5-hour window: it starts with the first
	// request after the previous one ended and lasts this long.
	ShortWindow = 5 * time.Hour
	// WeekPeriod is the weekly window: each account resets at its own
	// fixed time once a week.
	WeekPeriod = 7 * 24 * time.Hour
	// Full is a window at its limit, in percent.
	Full = 100.0
)

// Teammate is the person who owns a shared account and uses it on their
// own schedule, whatever the pool does.
type Teammate struct {
	// UTCOffset is the teammate's zone (Turkey is +3h).
	UTCOffset time.Duration
	// WorkStart and WorkEnd are local hours (0..24) of a working day;
	// WorkEnd below WorkStart runs past midnight.
	WorkStart, WorkEnd int
	// Workdays are the local weekdays they work (time.Sunday = 0).
	Workdays [7]bool
	// Burn is their use while working, in percent of a standard 5-hour
	// window per hour.
	Burn float64
	// Duty is the chance they are working in a step inside their hours.
	Duty float64
	// IdleDay is the chance a workday has no use at all.
	IdleDay float64
	// HeavyDay is the chance a workday burns twice as fast.
	HeavyDay float64
}

// Account is one member of the pool and its state when the run starts.
type Account struct {
	Name  string
	Group pool.Group
	// Capacity scales the account's limits: 1 is the standard plan, 2
	// is twice the room (a bigger plan burns half the percent).
	Capacity float64
	// WeekResetAt is the first weekly reset after the start; later ones
	// follow every WeekPeriod.
	WeekResetAt time.Time
	// Week and Short are the percentages at the start; ShortResetAt is
	// the running 5-hour window's end (nil = no window running).
	Week, Short  float64
	ShortResetAt *time.Time
	// Mate is the teammate of a shared account; nil for an own one.
	Mate *Teammate
}

// Load is our side: N sessions that each burn while busy.
type Load struct {
	// Sessions is how many of our sessions run.
	Sessions int
	// Burn is one busy session's use, in percent of a standard 5-hour
	// window per hour.
	Burn float64
	// Duty is the chance a session is busy in a step inside the active
	// hours.
	Duty float64
	// UTCOffset, ActiveStart and ActiveEnd are the hours our sessions
	// work (local hours; equal values = around the clock).
	UTCOffset              time.Duration
	ActiveStart, ActiveEnd int
}

// Scenario is one simulated situation; a Generator makes a fresh one for
// each seed.
type Scenario struct {
	Name    string
	Start   time.Time
	Horizon time.Duration
	Step    time.Duration
	// WeekRatio is how many full 5-hour windows one week holds: a 100%
	// 5-hour window adds 100/WeekRatio percent to the week.
	WeekRatio float64
	// Lag is how many steps old the readings a picker sees are (the
	// cache and probe delay); 1 = the state at the end of the last step.
	Lag      int
	Accounts []Account
	Load     Load
	// BaseCaps are the normal caps per group the squeeze and own-room
	// metrics measure against, whatever caps a picker uses.
	BaseCaps map[pool.Group]pool.Caps
	// Seed drives the teammates' days and our sessions' busy steps.
	Seed uint64
}

// Generator builds a scenario for one seed.
type Generator struct {
	Name string
	// About is one line for the report.
	About string
	Make  func(seed uint64) Scenario
}

// defaultBaseCaps are today's caps: own 5h 95 / week 98, shared 50 / 70.
func defaultBaseCaps() map[pool.Group]pool.Caps {
	return map[pool.Group]pool.Caps{
		pool.GroupOwn:    {Short: ownShortCap, Week: ownWeekCap},
		pool.GroupShared: {Short: pool.DefaultShortCap, Week: pool.DefaultWeekCap},
	}
}

// Today's own-account caps on the owner's machines.
const (
	ownShortCap = 95.0
	ownWeekCap  = 98.0
)
