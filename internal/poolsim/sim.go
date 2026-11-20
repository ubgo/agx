package poolsim

import (
	"math"
	"time"

	"github.com/ubgo/agx/pool"
)

// Member is one account as a picker sees it: its name and group, in
// config order (own first).
type Member struct {
	Name  string
	Group pool.Group
	// Capacity is the plan size relative to Max 5x when the user told
	// agx; 0 = unknown. Only the weighted strategy reads it.
	Capacity float64
}

// SessionView is one of our sessions as a picker sees it.
type SessionView struct {
	ID int
	// Busy is a session that asks for a key in this step; an idle one
	// keeps the key it has and is not counted as load.
	Busy bool
	// Current is the account it used last; "" before its first key.
	Current string
}

// View is everything a picker may look at in one step, the same facts
// the real pool code has: the cached readings (rounded, as the probe
// reports them, and Age old), the reset times inside them, each live
// run's account in use, and the clock.
type View struct {
	Now      time.Time
	Members  []Member
	Readings map[string]pool.Reading
	// Age is how old the readings are.
	Age      time.Duration
	Sessions []SessionView
	// Load is how many busy sessions each account served in the last
	// step (the live runs whose key came from it).
	Load map[string]int
}

// Picker assigns accounts to sessions. It may keep memory between
// steps of one run (a fresh Picker is made for every run).
type Picker interface {
	// Assign returns one account name per entry of v.Sessions, "" for
	// none; only busy sessions' answers are used.
	Assign(v View) []string
}

// Strategy names a picker and makes a fresh one per run.
type Strategy struct {
	Name string
	// About is one line for the report.
	About string
	// New makes the picker for one run of sc (it reads the pool's
	// configured caps and members from it).
	New func(sc Scenario) Picker
}

// statusLimit is the reading status of an account at a limit.
const statusLimit = "limit-reached"

// account is one account's hidden state during a run.
type account struct {
	Account
	idx int
	// week and short are the total use in percent; weekMate and
	// shortMate the teammate's part of it (the counterfactual "without
	// us" used to tell whether a lockout is ours).
	week, short         float64
	weekMate, shortMate float64
	weekReset           time.Time
	shortEnd            *time.Time
	// lastMate is the start of the last step the teammate worked in.
	lastMate time.Time
	// ourWeek is what we used this week (counted at reset).
	ourWeek float64
}

// teammateActiveFor is how long after their last use a teammate counts
// as active for the squeeze metric.
const teammateActiveFor = time.Hour

// Run simulates one scenario with one strategy and returns its metrics.
func Run(sc Scenario, st Strategy) Metrics {
	p := st.New(sc)
	accts := make([]*account, len(sc.Accounts))
	members := make([]Member, len(sc.Accounts))
	for i, a := range sc.Accounts {
		ac := &account{Account: a, idx: i, week: a.Week, short: a.Short, weekReset: a.WeekResetAt}
		if a.ShortResetAt != nil {
			end := *a.ShortResetAt
			ac.shortEnd = &end
		}
		if a.Group == pool.GroupShared {
			// The start's use is counted as the teammate's: the first
			// week's lockouts are then attributed to us only when our
			// own use made the difference.
			ac.weekMate, ac.shortMate = a.Week, a.Short
		}
		accts[i] = ac
		members[i] = Member{Name: a.Name, Group: a.Group, Capacity: knownCapacity(a)}
	}
	steps := int(sc.Horizon / sc.Step)
	stepH := sc.Step.Hours()
	m := Metrics{Weeks: sc.Horizon.Hours() / WeekPeriod.Hours(), Sessions: sc.Load.Sessions, Accounts: len(sc.Accounts)}
	// history holds the readings at the end of each step, newest last;
	// the initial state stands in before the first step.
	history := []map[string]pool.Reading{readings(accts)}
	sessions := make([]SessionView, sc.Load.Sessions)
	for i := range sessions {
		sessions[i].ID = i
	}
	load := map[string]int{}
	// After the horizon our sessions stop and the run goes on until
	// every account has reset once more, so each week we touched is
	// closed and counted: room left in a week still open at the end is
	// neither wasted nor saved yet, and leaving it out would reward a
	// strategy for where the horizon happens to cut.
	var tailEnd time.Time
	for k := 0; ; k++ {
		now := sc.Start.Add(time.Duration(k) * sc.Step)
		if k == steps {
			for _, a := range accts {
				if a.weekReset.After(tailEnd) {
					tailEnd = a.weekReset
				}
			}
		}
		for _, a := range accts {
			a.advance(now, sc, &m)
		}
		if k >= steps && !now.Before(tailEnd) {
			// The last reset is at or before this step: every week is
			// closed (a reset between two steps is applied at the next).
			break
		}
		if k >= steps {
			for _, a := range accts {
				a.burn(sc, now, 0, false, &m)
			}
			continue
		}
		lag := min(sc.Lag, len(history))
		view := View{
			Now: now, Members: members, Readings: history[len(history)-lag],
			Age: time.Duration(lag) * sc.Step, Sessions: sessions, Load: load,
		}
		for i := range sessions {
			sessions[i].Busy = sc.Load.busy(sc, i, now)
		}
		picks := p.Assign(view)
		newLoad := map[string]int{}
		onAcct := map[string][]int{}
		for i, s := range sessions {
			if !s.Busy {
				continue
			}
			m.DemandHours += stepH
			name := ""
			if i < len(picks) {
				name = picks[i]
			}
			if name == "" {
				m.StallHours += stepH
				continue
			}
			if s.Current != "" && s.Current != name {
				m.Switches++
			}
			sessions[i].Current = name
			newLoad[name]++
			onAcct[name] = append(onAcct[name], i)
		}
		for _, a := range accts {
			a.burn(sc, now, len(onAcct[a.Name]), true, &m)
		}
		load = newLoad
		history = append(history, readings(accts))
		if len(history) > maxHistory {
			history = history[1:]
		}
	}
	m.finish(sc)
	return m
}

// maxHistory bounds the reading history a run keeps (Lag is a few steps).
const maxHistory = 16

// advance applies the resets due at now: a weekly reset records the
// week's metrics and empties the week; an ended 5-hour window empties.
func (a *account) advance(now time.Time, sc Scenario, m *Metrics) {
	for !now.Before(a.weekReset) {
		m.reset(a, sc)
		a.week, a.weekMate, a.ourWeek = 0, 0, 0
		a.weekReset = a.weekReset.Add(WeekPeriod)
	}
	if a.shortEnd != nil && !now.Before(*a.shortEnd) {
		a.short, a.shortMate, a.shortEnd = 0, 0, nil
	}
}

// burn spends one step of use on the account: ours (n busy sessions) and
// the teammate's (inHorizon false is the tail after the horizon, where
// only teammates work). When the step would pass a limit, every user gets the
// same share of what is left: ours that is refused is a 429 for each of
// our sessions on it, the teammate's is a lockout (ours when the
// teammate alone would have stayed under the limit).
func (a *account) burn(sc Scenario, now time.Time, n int, inHorizon bool, m *Metrics) {
	stepH := sc.Step.Hours()
	base := sc.BaseCaps[a.Group]
	if inHorizon && a.Group == pool.GroupOwn && a.week >= base.Week {
		m.OwnDrainedHours += stepH
	}
	capacity := a.Capacity
	if capacity <= 0 {
		capacity = 1
	}
	ours := float64(n) * sc.Load.Burn * stepH / capacity
	mate := 0.0
	if a.Mate != nil {
		if b := a.Mate.burn(sc.Seed, a.idx, now); b > 0 {
			mate = b * stepH / capacity
			a.lastMate = now
		}
	}
	total := ours + mate
	if total <= 0 {
		return
	}
	if a.shortEnd == nil {
		end := now.Add(ShortWindow)
		a.shortEnd = &end
	}
	ratio := sc.WeekRatio
	share, weekBound := 1.0, false
	if a.short+total > Full {
		share = math.Min(share, (Full-a.short)/total)
	}
	if a.week+total/ratio > Full {
		if w := (Full - a.week) / (total / ratio); w < share {
			share, weekBound = w, true
		}
	}
	share = math.Max(0, share)
	mateAlone := a.shortMate+mate <= Full && a.weekMate+mate/ratio <= Full
	a.short += share * total
	a.week += share * total / ratio
	a.shortMate += share * mate
	a.weekMate += share * mate / ratio
	a.ourWeek += share * ours / ratio
	m.ServedHours += share * float64(n) * stepH
	if share < 1 && n > 0 {
		m.Hits429 += float64(n)
	}
	if share < 1 && mate > 0 && mateAlone {
		m.LockoutHours += (1 - share) * stepH
		if weekBound {
			m.LockoutWeekHours += (1 - share) * stepH
		}
	}
	if a.Group == pool.GroupShared && n > 0 && !a.lastMate.IsZero() && now.Sub(a.lastMate) <= teammateActiveFor {
		if a.short > base.Short || a.week > base.Week {
			m.SqueezeHours += stepH
		}
	}
}

// readings is what a probe of every account would report now: integer
// percentages (the probe's headers carry whole numbers), the reset
// times, and a limit status at 100%.
func readings(accts []*account) map[string]pool.Reading {
	out := make(map[string]pool.Reading, len(accts))
	for _, a := range accts {
		rd := pool.Reading{Status: pool.StatusOK}
		reset := a.weekReset
		rd.Week = &pool.Window{Percent: math.Round(a.week), ResetsAt: &reset}
		if a.shortEnd != nil {
			end := *a.shortEnd
			rd.Short = &pool.Window{Percent: math.Round(a.short), ResetsAt: &end}
		}
		if a.week >= Full || a.short >= Full {
			rd.Status = statusLimit
		}
		out[a.Name] = rd
	}
	return out
}

// busy reports whether session i works now.
func (l Load) busy(sc Scenario, i int, now time.Time) bool {
	if !inHours(now.Add(l.UTCOffset), l.ActiveStart, l.ActiveEnd) {
		return false
	}
	duty := l.Duty
	if l.Bursty {
		// Every session works hard in a burst and little outside one,
		// the average staying at Duty.
		burst := unit(sc.Seed, streamBurst, 0, uint64(now.Unix()/int64(burstBlock/time.Second))) < burstShare
		duty = math.Max(0, (l.Duty-burstShare*burstDuty)/(1-burstShare))
		if burst {
			duty = burstDuty
		}
	}
	return unit(sc.Seed, streamSession, uint64(i), block(now)) < duty
}

// burn is the teammate's use now, in percent of a standard 5-hour
// window per hour (0 when not working).
func (t *Teammate) burn(seed uint64, idx int, now time.Time) float64 {
	local := now.Add(t.UTCOffset)
	if t.Irregular {
		return t.irregularBurn(seed, idx, local)
	}
	if !t.Workdays[local.Weekday()] || !inHours(local, t.WorkStart, t.WorkEnd) {
		return 0
	}
	// A night shift past midnight belongs to the day it started.
	day := local
	if t.WorkEnd < t.WorkStart && local.Hour() < t.WorkEnd {
		day = local.Add(-24 * time.Hour)
	}
	dayNum := uint64(day.Unix() / int64(24*time.Hour/time.Second))
	if unit(seed, streamIdleDay, uint64(idx), dayNum) < t.IdleDay {
		return 0
	}
	if unit(seed, streamMateStep, uint64(idx), block(now)) >= t.Duty {
		return 0
	}
	if unit(seed, streamHeavyDay, uint64(idx), dayNum) < t.HeavyDay {
		return heavyFactor * t.Burn
	}
	return t.Burn
}

// dutyBlock is how long a busy or idle spell lasts: who works is drawn
// per block, not per step, so a run at any step sees the same spells
// (a session busy for a minute at a time would switch accounts on
// every step).
const dutyBlock = 15 * time.Minute

// block is the duty block now falls in.
func block(now time.Time) uint64 { return uint64(now.Unix() / int64(dutyBlock/time.Second)) }

// heavyFactor is how much faster a teammate burns on a heavy day.
const heavyFactor = 2.0

// inHours reports whether the local time is within [start, end) hours;
// start == end is the whole day.
func inHours(local time.Time, start, end int) bool {
	if start == end {
		return true
	}
	h := local.Hour()
	if start < end {
		return h >= start && h < end
	}
	return h >= start || h < end
}

// Random streams: each draw is a hash of the seed, a stream, an entity
// and a step or day, so the weeks a scenario draws do not depend on what
// a picker did (every strategy faces the same teammates and sessions).
const (
	streamSession uint64 = iota + 1
	streamIdleDay
	streamHeavyDay
	streamMateStep
	streamScenario
	streamBurst
	streamIrregular
)

// unit is a deterministic number in [0, 1) for one draw.
func unit(seed, stream, entity, step uint64) float64 {
	x := seed*0x9E3779B97F4A7C15 ^ stream*0xBF58476D1CE4E5B9 ^ entity*0x94D049BB133111EB ^ step*0xD6E8FEB86659FD93
	return float64(splitmix(x)>>11) / (1 << 53)
}

// splitmix is SplitMix64's finaliser: a well-mixed 64-bit hash.
func splitmix(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// irregularBurn is an irregular teammate's use: each local day works
// with irregularDay chance, from a random hour, for a random shift.
func (t *Teammate) irregularBurn(seed uint64, idx int, local time.Time) float64 {
	day := uint64(local.Unix() / int64(24*time.Hour/time.Second))
	if unit(seed, streamIrregular, uint64(idx), day*irregularDraws) >= irregularDay {
		return 0
	}
	start := int(unit(seed, streamIrregular, uint64(idx), day*irregularDraws+1) * hoursPerDay)
	length := irregularMinHours + int(unit(seed, streamIrregular, uint64(idx), day*irregularDraws+2)*(irregularMaxHours-irregularMinHours+1))
	h := local.Hour()
	if h < start || h >= start+length {
		return 0
	}
	if unit(seed, streamMateStep, uint64(idx), block(local)) >= t.Duty {
		return 0
	}
	return t.Burn
}

// irregularDraws are the draws per day of an irregular teammate.
const irregularDraws = 4
