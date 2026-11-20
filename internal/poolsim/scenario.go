package poolsim

import (
	"fmt"
	"time"

	"github.com/ubgo/agx/pool"
)

// Scenario defaults: one-minute steps (the pool's key_ttl and
// usage_max_age are 60s, so a running session re-picks about once a
// minute; 15-minute steps make every strategy hit limits it would not
// hit for real) over two weeks, readings one step old, eight full
// 5-hour windows to a week.
const (
	DefaultStep      = time.Minute
	DefaultHorizon   = 2 * WeekPeriod
	DefaultLag       = 1
	DefaultWeekRatio = 8.0
)

// Our sessions' defaults: one busy session burns 12% of a standard
// 5-hour window an hour (1.5% of a week), busy 60% of the steps between
// 08:00 and 02:00 India time.
const (
	defaultSessionBurn = 12.0
	defaultSessionDuty = 0.6
	istOffset          = 5*time.Hour + 30*time.Minute
	ourStart, ourEnd   = 8, 2
)

// Teammates: Turkey time, 09:00 to 18:00 on weekdays (their start moves
// by up to two hours per account), working 60% of those steps; a light,
// medium or heavy burn; one workday in seven idle, one in ten heavy.
const (
	trOffset        = 3 * time.Hour
	mateStart       = 9
	mateDay         = 9
	mateDuty        = 0.6
	mateIdleDay     = 0.15
	mateHeavyDay    = 0.1
	mateBurnLight   = 6.0
	mateBurnMedium  = 12.0
	mateBurnHeavy   = 24.0
	mateStartJitter = 2
)

// snapshotAt is when a real pool cache was read (agx pool pick
// --cached --json, 2026-11-20).
var snapshotAt = time.Date(2026, 11, 20, 7, 30, 0, 0, time.UTC)

// snapMember is one member of the real snapshot under a made-up name:
// its group, 5-hour and weekly use and hours to the weekly reset.
type snapMember struct {
	name       string
	group      pool.Group
	short      float64
	hasShort   bool
	week       float64
	resetHours float64
}

// snapshotMembers is the real pool of 2026-11-20 in config order: three
// own accounts and nine shared ones. "gus" is the account whose week
// reset 27 minutes after the read with 36% used, skipped for its 5-hour
// 52% (cap 50); "ivy" reset 4.3 hours later at 5-hour 66%, week 70%.
var snapshotMembers = []snapMember{
	{"ava", pool.GroupOwn, 0, false, 52, 118.3},
	{"ben", pool.GroupOwn, 0, true, 0, 154.3},
	{"cara", pool.GroupOwn, 56, true, 98, 20.3},
	{"dan", pool.GroupShared, 4, true, 7, 128.3},
	{"eli", pool.GroupShared, 6, true, 7, 115.3},
	{"fay", pool.GroupShared, 12, true, 19, 49.3},
	{"gus", pool.GroupShared, 52, true, 36, 0.3},
	{"hal", pool.GroupShared, 46, true, 48, 43.3},
	{"ivy", pool.GroupShared, 66, true, 70, 4.3},
	{"jon", pool.GroupShared, 15, true, 90, 19.3},
	{"kim", pool.GroupShared, 2, true, 73, 88.3},
	{"lea", pool.GroupShared, 3, true, 4, 148.3},
}

// base is a scenario with the defaults and our load.
func base(name string, seed uint64, start time.Time, sessions int, duty float64) Scenario {
	return Scenario{
		Name: name, Start: start, Horizon: DefaultHorizon, Step: DefaultStep, WeekRatio: DefaultWeekRatio,
		Lag: DefaultLag, BaseCaps: defaultBaseCaps(), Seed: seed,
		Load: Load{
			Sessions: sessions, Burn: defaultSessionBurn, Duty: duty,
			UTCOffset: istOffset, ActiveStart: ourStart, ActiveEnd: ourEnd,
		},
	}
}

// draw is a seeded number in [0, 1) for scenario building.
func draw(seed uint64, entity, n int) float64 {
	return unit(seed, streamScenario, uint64(entity), uint64(n))
}

// mate is a random teammate for account idx.
func mate(seed uint64, idx int) *Teammate {
	burns := []float64{mateBurnLight, mateBurnMedium, mateBurnMedium, mateBurnHeavy}
	start := mateStart + int(draw(seed, idx, 1)*float64(2*mateStartJitter+1)) - mateStartJitter
	t := &Teammate{
		UTCOffset: trOffset, WorkStart: start, WorkEnd: start + mateDay,
		Burn: burns[int(draw(seed, idx, 2)*float64(len(burns)))], Duty: mateDuty,
		IdleDay: mateIdleDay, HeavyDay: mateHeavyDay,
	}
	for d := time.Monday; d <= time.Friday; d++ {
		t.Workdays[d] = true
	}
	// One teammate in five also works Saturdays.
	t.Workdays[time.Saturday] = draw(seed, idx, 3) < saturdayShare
	return t
}

// saturdayShare is the part of teammates who also work on Saturday.
const saturdayShare = 0.2

// shortReset is a seeded end for a running 5-hour window: between half
// an hour and four and a half hours away.
func shortReset(seed uint64, idx int, at time.Time) *time.Time {
	t := at.Add(time.Duration((0.5 + 4*draw(seed, idx, 4)) * float64(time.Hour)))
	return &t
}

// Snapshot is a real 12-member pool as cached on 2026-11-20, with sessions of
// ours busy duty of the steps; the teammates are drawn per seed.
func Snapshot(name string, sessions int, duty float64) Generator {
	return Generator{
		Name:  name,
		About: fmt.Sprintf("real pool cache of 2026-11-20 (3 own, 9 shared), %d sessions busy %.0f%%", sessions, duty*100),
		Make: func(seed uint64) Scenario {
			sc := base(name, seed, snapshotAt, sessions, duty)
			for i, m := range snapshotMembers {
				a := Account{
					Name: m.name, Group: m.group, Capacity: 1, Week: m.week,
					WeekResetAt: snapshotAt.Add(time.Duration(m.resetHours * float64(time.Hour))),
				}
				if m.hasShort && m.short > 0 {
					a.Short, a.ShortResetAt = m.short, shortReset(seed, i, snapshotAt)
				}
				if m.group == pool.GroupShared {
					a.Mate = mate(seed, i)
				}
				sc.Accounts = append(sc.Accounts, a)
			}
			return sc
		},
	}
}

// Resets says how a synthetic pool's weekly resets fall.
type Resets int

// Reset layouts.
const (
	// ResetsRandom draws each account's reset anywhere in the week.
	ResetsRandom Resets = iota
	// ResetsSpread spaces them evenly over the week.
	ResetsSpread
	// ResetsBunched puts every shared reset inside one 12-hour band.
	ResetsBunched
)

// bunchBand is the width of the band of bunched resets.
const bunchBand = 12 * time.Hour

// Synthetic options.
type Synthetic struct {
	Name     string
	About    string
	Own      int
	Shared   int
	Sessions int
	Duty     float64
	Resets   Resets
	// NearCaps starts most shared accounts close to their caps.
	NearCaps bool
	// MateOnPreferred gives the shared account that resets soonest a
	// heavy teammate who works during our hours.
	MateOnPreferred bool
}

// syntheticStart is a fixed Monday for synthetic weeks.
var syntheticStart = time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)

// Generator makes the synthetic scenario for each seed.
func (s Synthetic) Generator() Generator {
	return Generator{Name: s.Name, About: s.About, Make: s.make}
}

// make builds one seed's pool: resets as laid out, each account's use
// so far in line with how much of its week has gone.
func (s Synthetic) make(seed uint64) Scenario {
	sc := base(s.Name, seed, syntheticStart, s.Sessions, s.Duty)
	n := s.Own + s.Shared
	bandStart := draw(seed, n, 5) * WeekPeriod.Hours()
	soonest, soonestIdx := WeekPeriod, -1
	for i := range n {
		group := pool.GroupOwn
		if i >= s.Own {
			group = pool.GroupShared
		}
		var hours float64
		switch {
		case s.Resets == ResetsSpread:
			hours = (float64(i) + draw(seed, i, 6)*spreadJitter) * WeekPeriod.Hours() / float64(n)
		case s.Resets == ResetsBunched && group == pool.GroupShared:
			hours = bandStart + draw(seed, i, 6)*bunchBand.Hours()
		default:
			hours = draw(seed, i, 6) * WeekPeriod.Hours()
		}
		for hours >= WeekPeriod.Hours() {
			hours -= WeekPeriod.Hours()
		}
		gone := 1 - hours/WeekPeriod.Hours()
		a := Account{Name: fmt.Sprintf("m%02d", i+1), Group: group, Capacity: 1,
			WeekResetAt: sc.Start.Add(time.Duration(hours * float64(time.Hour)))}
		rate := ownWeekRate[0] + draw(seed, i, 7)*(ownWeekRate[1]-ownWeekRate[0])
		if group == pool.GroupShared {
			rate = sharedWeekRate[0] + draw(seed, i, 7)*(sharedWeekRate[1]-sharedWeekRate[0])
			a.Mate = mate(seed, i)
		}
		a.Week = rate * gone
		if s.NearCaps && group == pool.GroupShared {
			a.Week = nearCapsWeek[0] + draw(seed, i, 8)*(nearCapsWeek[1]-nearCapsWeek[0])
			a.Short = nearCapsShort[0] + draw(seed, i, 9)*(nearCapsShort[1]-nearCapsShort[0])
			a.ShortResetAt = shortReset(seed, i, sc.Start)
		} else if draw(seed, i, 10) < runningShortShare {
			a.Short = draw(seed, i, 9) * runningShortMax
			a.ShortResetAt = shortReset(seed, i, sc.Start)
		}
		if group == pool.GroupShared && time.Duration(hours*float64(time.Hour)) < soonest {
			soonest, soonestIdx = time.Duration(hours*float64(time.Hour)), i
		}
		sc.Accounts = append(sc.Accounts, a)
	}
	if s.MateOnPreferred && soonestIdx >= 0 {
		m := sc.Accounts[soonestIdx].Mate
		m.Burn, m.Duty, m.UTCOffset, m.WorkStart, m.WorkEnd = mateBurnHeavy, preferredMateDuty, istOffset, ourStart, ourEnd
		for d := range m.Workdays {
			m.Workdays[d] = true
		}
		m.IdleDay = 0
	}
	return sc
}

// Synthetic ranges.
var (
	// ownWeekRate and sharedWeekRate are the week's use a full week
	// would reach at the account's pace so far (percent).
	ownWeekRate    = [2]float64{30, 90}
	sharedWeekRate = [2]float64{20, 75}
	// nearCapsWeek and nearCapsShort start a near-caps shared account.
	nearCapsWeek  = [2]float64{55, 72}
	nearCapsShort = [2]float64{30, 55}
)

// Synthetic constants.
const (
	// spreadJitter moves an evenly spaced reset by up to this part of
	// its slot.
	spreadJitter = 0.5
	// runningShortShare of accounts have a 5-hour window running at the
	// start, up to runningShortMax percent used.
	runningShortShare = 0.4
	runningShortMax   = 45.0
	// preferredMateDuty is the heavy teammate's duty on the preferred
	// account.
	preferredMateDuty = 0.8
)

// Scenarios are the report's scenarios, realistic first.
func Scenarios() []Generator {
	return []Generator{
		Snapshot("snapshot", 10, snapshotDuty),
		Snapshot("snapshot-busy", 10, defaultSessionDuty),
		Synthetic{Name: "light", About: "2 sessions, random resets", Own: 3, Shared: 9, Sessions: 2, Duty: defaultSessionDuty}.Generator(),
		Synthetic{Name: "normal", About: "5 sessions, random resets", Own: 3, Shared: 9, Sessions: 5, Duty: defaultSessionDuty}.Generator(),
		Synthetic{Name: "heavy", About: "11 sessions, random resets", Own: 3, Shared: 9, Sessions: 11, Duty: defaultSessionDuty}.Generator(),
		Synthetic{Name: "mate-on-preferred", About: "5 sessions; a heavy teammate works our hours on the soonest-reset shared account", Own: 3, Shared: 9, Sessions: 5, Duty: defaultSessionDuty, MateOnPreferred: true}.Generator(),
		Synthetic{Name: "near-caps", About: "5 sessions; shared accounts start at week 55-72%, 5h 30-55%", Own: 3, Shared: 9, Sessions: 5, Duty: defaultSessionDuty, NearCaps: true}.Generator(),
		Synthetic{Name: "bunched", About: "5 sessions; every shared reset inside one 12h band", Own: 3, Shared: 9, Sessions: 5, Duty: defaultSessionDuty, Resets: ResetsBunched}.Generator(),
		Synthetic{Name: "spread", About: "5 sessions; resets evenly spaced over the week", Own: 3, Shared: 9, Sessions: 5, Duty: defaultSessionDuty, Resets: ResetsSpread}.Generator(),
	}
}

// snapshotDuty: ten sessions that are busy 30% of the time (most wait
// on the owner) spend about what five busy-60% sessions do.
const snapshotDuty = 0.3

// WithTiming returns gen with every scenario's step and lag replaced.
func WithTiming(gen Generator, step time.Duration, lag int) Generator {
	mk := gen.Make
	gen.Make = func(seed uint64) Scenario {
		sc := mk(seed)
		sc.Step, sc.Lag = step, lag
		return sc
	}
	return gen
}

// WithWeekRatio returns gen with every scenario's week ratio replaced
// (the sensitivity of the results to how many 5-hour windows a week
// holds, which Anthropic does not publish).
func WithWeekRatio(gen Generator, ratio float64) Generator {
	mk := gen.Make
	gen.Make = func(seed uint64) Scenario {
		sc := mk(seed)
		sc.WeekRatio = ratio
		return sc
	}
	return gen
}
