package poolsim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/pool"
)

// Scenario defaults: one-minute steps (a pool's key_ttl and
// usage_max_age are 60s, so a running session re-picks about once a
// minute; coarser steps make every strategy hit limits it would not hit
// for real) over two weeks, readings one step old, eight full 5-hour
// windows to a week.
const (
	DefaultStep      = time.Minute
	DefaultHorizon   = 2 * WeekPeriod
	DefaultLag       = 1
	DefaultWeekRatio = 8.0
)

// Plans and their capacity relative to Max 5x: the same work burns five
// times the percent on Pro and a quarter of it on Max 20x.
const (
	PlanPro   = "pro"
	PlanMax5  = "max5"
	PlanMax20 = "max20"
	// PlanUnknown draws a real plan from the three but tells the picker
	// nothing (a user who did not say).
	PlanUnknown = "unknown"
)

// knownPlans are the plans PlanUnknown draws from.
var knownPlans = []string{PlanPro, PlanMax5, PlanMax20}

// planCapacity is each plan's room relative to Max 5x.
var planCapacity = map[string]float64{PlanPro: 0.2, PlanMax5: 1, PlanMax20: 4}

// Teammate profiles a scenario file may name.
const (
	MateNone      = "none"
	MateLight     = "light"
	MateMedium    = "medium"
	MateHeavy     = "heavy"
	MateIrregular = "irregular"
)

// Teammate burns in percent of a Max 5x 5-hour window per hour while
// working.
var mateBurn = map[string]float64{MateLight: 6, MateMedium: 12, MateHeavy: 24, MateIrregular: 15}

// Session patterns: PatternAny draws steady or bursty per seed.
const (
	PatternSteady = "steady"
	PatternBursty = "bursty"
	PatternAny    = "any"
)

// Reset layouts.
const (
	ResetsRandom  = "random"
	ResetsSpread  = "spread"
	ResetsBunched = "bunched"
)

// Groups of the report, by the pool's members: solo (own accounts
// only), small and large team (teammates' accounts only, up to six
// and seven or more), mixed (both).
const (
	GroupSolo      = "solo"
	GroupSmallTeam = "small-team"
	GroupLargeTeam = "large-team"
	GroupMixed     = "mixed"
)

// GroupOrder is the report's group order.
var GroupOrder = []string{GroupSolo, GroupSmallTeam, GroupLargeTeam, GroupMixed}

// smallTeamMax is the most members an all-shared pool may have and
// still count as a small team.
const smallTeamMax = 6

// Our sessions' defaults: one busy session burns 12% of a Max 5x 5-hour
// window an hour (1.5% of a week), busy 60% of the steps in its hours.
const (
	defaultSessionBurn = 12.0
	defaultSessionDuty = 0.6
)

// Teammate defaults: working hours start at 09:00 local (moved by up
// to two hours per account) and last nine hours on weekdays, 60% of the
// steps; one workday in seven idle, one in ten heavy; one teammate in
// five also works Saturdays.
const (
	mateStart       = 9
	mateDay         = 9
	mateDuty        = 0.6
	mateIdleDay     = 0.15
	mateHeavyDay    = 0.1
	mateStartJitter = 2
	saturdayShare   = 0.2
)

// Irregular teammates: any day with this chance, a random start hour, a
// shift of irregularMinHours to irregularMaxHours.
const (
	irregularDay      = 0.6
	irregularMinHours = 2
	irregularMaxHours = 10
)

// Bursty sessions: bursts of burstBlock, each a burst with burstShare;
// in a burst a session is busy with burstDuty, outside it with what
// keeps the average at the file's duty.
const (
	burstBlock = 2 * time.Hour
	burstShare = 0.25
	burstDuty  = 0.95
)

// Default configured caps of a scenario's pool: own 95/98 (what the
// guide recommends for your own accounts), shared 50/70 (agx's
// defaults).
var (
	defaultOwnCaps    = pool.Caps{Short: 95, Week: 98}
	defaultSharedCaps = pool.Caps{Short: pool.DefaultShortCap, Week: pool.DefaultWeekCap}
)

// Made-up account names, in order: own members first, then shared.
var names = []string{
	"alice", "bob", "carol", "derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "ken", "liam", "mallory",
	"niaj", "olivia", "peggy", "quinn", "rupert", "sybil", "trent", "uma", "victor", "walter", "xena", "yara", "zoe",
}

// File is one scenario file (pool/testdata/sim/*.jsonc): a family of
// pools drawn per seed from the ranges it gives, or fixed accounts.
type File struct {
	// Name is the scenario's name (the file name without .jsonc when
	// omitted).
	Name  string `json:"name,omitempty"`
	About string `json:"about"`
	// Group is solo, small-team or large-team; derived from the members
	// when omitted (no shared member = solo; up to six members = small).
	Group string `json:"group,omitempty"`
	// Seed is the first seed; Seeds how many runs (seeds Seed, Seed+1…).
	Seed  uint64 `json:"seed"`
	Seeds int    `json:"seeds"`
	// Gate puts the scenario in the regression gate (go test).
	Gate bool `json:"gate,omitempty"`
	// Horizon is how long our sessions run (default "336h").
	Horizon string `json:"horizon,omitempty"`
	// Own and Shared are the drawn members; Accounts replaces both with
	// fixed members.
	Own      Members `json:"own,omitzero"`
	Shared   Members `json:"shared,omitzero"`
	Accounts []Fixed `json:"accounts,omitempty"`
	// Resets is random, spread or bunched (default random).
	Resets   string   `json:"resets,omitempty"`
	Sessions Sessions `json:"sessions"`
	// Caps are the pool's configured caps per group.
	Caps FileCaps `json:"caps,omitzero"`
	// WeekRatio overrides DefaultWeekRatio.
	WeekRatio float64 `json:"week_ratio,omitempty"`
	// Model is a per-model weekly window; omitted = none.
	Model *FileModel `json:"model,omitempty"`
}

// FileModel is a per-model weekly window (Fable's 7d_oi): a share of our
// sessions use a model whose own week holds week_ratio full 5-hour
// windows.
type FileModel struct {
	// Share is the part of our sessions on that model, 0 to 1 (at least
	// one session when above 0).
	Share float64 `json:"share"`
	// WeekRatio is how many full 5-hour windows its own week holds.
	WeekRatio float64 `json:"week_ratio"`
}

// Members are drawn members of one group.
type Members struct {
	// Count is [min, max] members, drawn per seed.
	Count [2]int `json:"count"`
	// Plans are drawn per member (default max5).
	Plans []string `json:"plans,omitempty"`
	// Mates are drawn per shared member (default light, medium, medium,
	// heavy).
	Mates []string `json:"mates,omitempty"`
}

// Sessions are our sessions.
type Sessions struct {
	// Count and Duty are [min, max], drawn per seed.
	Count [2]int     `json:"count"`
	Duty  [2]float64 `json:"duty,omitempty"`
	// Pattern is steady or bursty (default steady).
	Pattern string `json:"pattern,omitempty"`
	// Hours are the local [start, end) hours they work; equal = all day
	// (default [8, 2]).
	Hours *[2]int `json:"hours,omitempty"`
	// UTCOffset is their zone, a Go duration ("5h30m"; default 0).
	UTCOffset string `json:"utc_offset,omitempty"`
}

// FileCaps are the configured caps per group.
type FileCaps struct {
	Own    *pool.Caps `json:"own,omitempty"`
	Shared *pool.Caps `json:"shared,omitempty"`
}

// Fixed is one fixed member (a recorded pool, anonymised).
type Fixed struct {
	Name  string     `json:"name"`
	Group pool.Group `json:"group"`
	Plan  string     `json:"plan,omitempty"`
	Week  float64    `json:"week"`
	// Short is the 5-hour use; a negative value is no window running.
	Short float64 `json:"short"`
	// ResetHours is the time to the weekly reset.
	ResetHours float64 `json:"reset_hours"`
	// Mate is the teammate profile of a shared member (drawn from the
	// defaults when empty).
	Mate string `json:"mate,omitempty"`
}

// fileStart is the fixed start of every scenario (a Monday 06:00 UTC).
var fileStart = time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)

// LoadDir reads every scenario file of dir, sorted by name.
func LoadDir(dir string) ([]Generator, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonc"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []Generator
	for _, p := range paths {
		g, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no scenario files: %w", dir, fs.ErrNotExist)
	}
	return out, nil
}

// LoadFile reads one scenario file: JSONC, unknown keys refused.
func LoadFile(path string) (Generator, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Generator{}, err
	}
	doc, err := jsonc.Parse(b)
	if err != nil {
		return Generator{}, fmt.Errorf("%s: %w", path, err)
	}
	v, err := doc.Get("")
	if err != nil {
		return Generator{}, fmt.Errorf("%s: %w", path, err)
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return Generator{}, fmt.Errorf("%s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(plain))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return Generator{}, fmt.Errorf("%s: %w", path, err)
	}
	if f.Name == "" {
		f.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if err := f.check(); err != nil {
		return Generator{}, fmt.Errorf("%s: %w", path, err)
	}
	return f.Generator(), nil
}

// check refuses a file the generator cannot draw from.
func (f File) check() error {
	if f.Seeds <= 0 {
		return errors.New("seeds must be 1 or more")
	}
	if len(f.Accounts) == 0 && f.Own.Count[1]+f.Shared.Count[1] == 0 {
		return errors.New("no members: give own, shared or accounts")
	}
	for _, m := range []Members{f.Own, f.Shared} {
		if m.Count[0] > m.Count[1] || m.Count[0] < 0 {
			return fmt.Errorf("count %v is not [min, max]", m.Count)
		}
		for _, p := range m.Plans {
			if _, ok := planCapacity[p]; !ok && p != PlanUnknown {
				return fmt.Errorf("unknown plan %q", p)
			}
		}
		for _, mt := range m.Mates {
			if _, ok := mateBurn[mt]; !ok && mt != MateNone {
				return fmt.Errorf("unknown teammate profile %q", mt)
			}
		}
	}
	if f.Own.Count[1]+f.Shared.Count[1] > len(names) {
		return fmt.Errorf("at most %d members", len(names))
	}
	if f.Sessions.Count[0] > f.Sessions.Count[1] || f.Sessions.Count[1] <= 0 {
		return fmt.Errorf("sessions count %v is not [min, max] with max 1 or more", f.Sessions.Count)
	}
	switch f.Resets {
	case "", ResetsRandom, ResetsSpread, ResetsBunched:
	default:
		return fmt.Errorf("resets %q is not random, spread or bunched", f.Resets)
	}
	switch f.Sessions.Pattern {
	case "", PatternSteady, PatternBursty, PatternAny:
	default:
		return fmt.Errorf("pattern %q is not steady, bursty or any", f.Sessions.Pattern)
	}
	switch f.Group {
	case "", GroupSolo, GroupSmallTeam, GroupLargeTeam, GroupMixed:
	default:
		return fmt.Errorf("group %q is not solo, small-team, large-team or mixed", f.Group)
	}
	if m := f.Model; m != nil && (m.Share <= 0 || m.Share > 1 || m.WeekRatio <= 0) {
		return fmt.Errorf("model: share %v must be above 0 and at most 1, week_ratio %v above 0", m.Share, m.WeekRatio)
	}
	if _, err := f.offset(); err != nil {
		return err
	}
	_, err := f.horizon()
	return err
}

// offset is the sessions' zone.
func (f File) offset() (time.Duration, error) {
	if f.Sessions.UTCOffset == "" {
		return 0, nil
	}
	return time.ParseDuration(f.Sessions.UTCOffset)
}

// horizon is how long sessions run.
func (f File) horizon() (time.Duration, error) {
	if f.Horizon == "" {
		return DefaultHorizon, nil
	}
	return time.ParseDuration(f.Horizon)
}

// Generator makes the file's scenario for each seed.
func (f File) Generator() Generator {
	return Generator{Name: f.Name, About: f.About, Group: f.group(), Seed: f.Seed, Seeds: f.Seeds, Gate: f.Gate, Make: f.make}
}

// group is the file's report group.
func (f File) group() string {
	if f.Group != "" {
		return f.Group
	}
	own, shared := f.Own.Count[1], f.Shared.Count[1]
	if len(f.Accounts) > 0 {
		own, shared = 0, 0
		for _, a := range f.Accounts {
			if a.Group == pool.GroupShared {
				shared++
			} else {
				own++
			}
		}
	}
	switch {
	case shared == 0:
		return GroupSolo
	case own > 0:
		return GroupMixed
	case shared <= smallTeamMax:
		return GroupSmallTeam
	}
	return GroupLargeTeam
}

// draw is a seeded number in [0, 1) for scenario building.
func draw(seed uint64, entity, n int) float64 {
	return unit(seed, streamScenario, uint64(entity), uint64(n))
}

// pickInt draws an int in [lo, hi].
func pickInt(seed uint64, entity, n int, r [2]int) int {
	return r[0] + int(draw(seed, entity, n)*float64(r[1]-r[0]+1))
}

// pickFloat draws a float in [lo, hi].
func pickFloat(seed uint64, entity, n int, r [2]float64) float64 {
	return r[0] + draw(seed, entity, n)*(r[1]-r[0])
}

// pickOne draws one of opts (def when opts is empty).
func pickOne(seed uint64, entity, n int, opts []string, def []string) string {
	if len(opts) == 0 {
		opts = def
	}
	return opts[int(draw(seed, entity, n)*float64(len(opts)))]
}

// defaultMates is the teammate mix when a file names none.
var defaultMates = []string{MateLight, MateMedium, MateMedium, MateHeavy}

// Draw indices: each property of a member or session is its own
// stream, so adding one does not shift the others.
const (
	drawCount = iota + 1
	drawPlan
	drawMate
	drawReset
	drawWeekRate
	drawShortOn
	drawShortPct
	drawShortEnd
	drawMateStart
	drawMateSat
	drawSessions
	drawDuty
	drawBand
	drawPattern
	drawRealPlan
)

// anyBursty is the share of PatternAny seeds that are bursty.
const anyBursty = 0.5

// Weekly use so far at the start, as the part of a full week the
// account's pace would reach: own and shared accounts.
var (
	ownWeekRate    = [2]float64{30, 90}
	sharedWeekRate = [2]float64{20, 75}
)

// Running 5-hour windows at the start: this share of accounts, up to
// runningShortMax percent, ending in half an hour to four and a half.
const (
	runningShortShare = 0.4
	runningShortMax   = 45.0
	spreadJitter      = 0.5
	bunchBand         = 12 * time.Hour
)

// make builds one seed's scenario.
func (f File) make(seed uint64) Scenario {
	horizon, _ := f.horizon() // check validated it
	offset, _ := f.offset()
	sc := Scenario{
		Name: f.Name, Start: fileStart, Horizon: horizon, Step: DefaultStep, WeekRatio: DefaultWeekRatio, Lag: DefaultLag,
		Seed: seed, BaseCaps: map[pool.Group]pool.Caps{pool.GroupOwn: defaultOwnCaps, pool.GroupShared: defaultSharedCaps},
	}
	if f.WeekRatio > 0 {
		sc.WeekRatio = f.WeekRatio
	}
	if f.Caps.Own != nil {
		sc.BaseCaps[pool.GroupOwn] = *f.Caps.Own
	}
	if f.Caps.Shared != nil {
		sc.BaseCaps[pool.GroupShared] = *f.Caps.Shared
	}
	hours := [2]int{8, 2}
	if f.Sessions.Hours != nil {
		hours = *f.Sessions.Hours
	}
	duty := f.Sessions.Duty
	if duty == [2]float64{} {
		duty = [2]float64{defaultSessionDuty, defaultSessionDuty}
	}
	sc.Load = Load{
		Sessions: pickInt(seed, 0, drawSessions, f.Sessions.Count), Burn: defaultSessionBurn,
		Duty: pickFloat(seed, 0, drawDuty, duty), UTCOffset: offset, ActiveStart: hours[0], ActiveEnd: hours[1],
		Bursty: f.Sessions.Pattern == PatternBursty || (f.Sessions.Pattern == PatternAny && draw(seed, 0, drawPattern) < anyBursty),
	}
	if f.Model != nil {
		sc.Model = ModelWeek{Sessions: max(1, int(math.Round(f.Model.Share*float64(sc.Load.Sessions)))), WeekRatio: f.Model.WeekRatio}
	}
	if len(f.Accounts) > 0 {
		for i, a := range f.Accounts {
			sc.Accounts = append(sc.Accounts, f.withModel(sc, f.fixed(seed, i, a)))
		}
		return sc
	}
	own := pickInt(seed, 0, drawCount, f.Own.Count)
	shared := pickInt(seed, 1, drawCount, f.Shared.Count)
	n := own + shared
	band := draw(seed, 0, drawBand) * WeekPeriod.Hours()
	for i := range n {
		spec, group := f.Own, pool.GroupOwn
		if i >= own {
			spec, group = f.Shared, pool.GroupShared
		}
		var h float64
		switch f.Resets {
		case ResetsSpread:
			h = (float64(i) + draw(seed, i, drawReset)*spreadJitter) * WeekPeriod.Hours() / float64(n)
		case ResetsBunched:
			h = band + draw(seed, i, drawReset)*bunchBand.Hours()
		default:
			h = draw(seed, i, drawReset) * WeekPeriod.Hours()
		}
		h = math.Mod(h, WeekPeriod.Hours())
		plan := pickOne(seed, i, drawPlan, spec.Plans, []string{PlanMax5})
		known := plan != PlanUnknown
		if !known {
			plan = pickOne(seed, i, drawRealPlan, knownPlans, nil)
		}
		a := Account{
			Name: names[i], Group: group, Plan: plan, Capacity: planCapacity[plan], CapacityKnown: known,
			WeekResetAt: sc.Start.Add(time.Duration(h * float64(time.Hour))),
		}
		rate := ownWeekRate
		if group == pool.GroupShared {
			rate = sharedWeekRate
			a.Mate = mateOf(seed, i, pickOne(seed, i, drawMate, spec.Mates, defaultMates))
		}
		a.Week = pickFloat(seed, i, drawWeekRate, rate) * (1 - h/WeekPeriod.Hours())
		if draw(seed, i, drawShortOn) < runningShortShare {
			a.Short = draw(seed, i, drawShortPct) * runningShortMax
			a.ShortResetAt = shortReset(seed, i, sc.Start)
		}
		sc.Accounts = append(sc.Accounts, f.withModel(sc, a))
	}
	return sc
}

// withModel sets an account's per-model week at the start: the model's
// share of the week's use so far, on its smaller week.
func (f File) withModel(sc Scenario, a Account) Account {
	if sc.Model.WeekRatio > 0 && sc.Load.Sessions > 0 {
		share := float64(sc.Model.Sessions) / float64(sc.Load.Sessions)
		a.Model = math.Min(Full, a.Week*share*sc.WeekRatio/sc.Model.WeekRatio)
	}
	return a
}

// fixed is a recorded member at the start.
func (f File) fixed(seed uint64, i int, x Fixed) Account {
	plan := x.Plan
	if plan == "" {
		plan = PlanMax5
	}
	a := Account{
		Name: x.Name, Group: x.Group, Plan: plan, Capacity: planCapacity[plan], CapacityKnown: true, Week: x.Week,
		WeekResetAt: fileStart.Add(time.Duration(x.ResetHours * float64(time.Hour))),
	}
	if x.Short > 0 {
		a.Short, a.ShortResetAt = x.Short, shortReset(seed, i, fileStart)
	}
	if x.Group == pool.GroupShared {
		profile := x.Mate
		if profile == "" {
			profile = pickOne(seed, i, drawMate, nil, defaultMates)
		}
		a.Mate = mateOf(seed, i, profile)
	}
	return a
}

// shortReset is a seeded end for a running 5-hour window: between half
// an hour and four and a half hours away.
func shortReset(seed uint64, idx int, at time.Time) *time.Time {
	t := at.Add(time.Duration((0.5 + 4*draw(seed, idx, drawShortEnd)) * float64(time.Hour)))
	return &t
}

// mateOf is the teammate of profile p for account idx (nil for none).
func mateOf(seed uint64, idx int, p string) *Teammate {
	if p == MateNone {
		return nil
	}
	start := mateStart + int(draw(seed, idx, drawMateStart)*float64(2*mateStartJitter+1)) - mateStartJitter
	t := &Teammate{
		UTCOffset: 3 * time.Hour, WorkStart: start, WorkEnd: start + mateDay, Burn: mateBurn[p], Duty: mateDuty,
		IdleDay: mateIdleDay, HeavyDay: mateHeavyDay, Irregular: p == MateIrregular,
	}
	for d := time.Monday; d <= time.Friday; d++ {
		t.Workdays[d] = true
	}
	t.Workdays[time.Saturday] = draw(seed, idx, drawMateSat) < saturdayShare
	return t
}

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

// WithSeeds returns gen limited to at most n seeds (the fast gate).
func WithSeeds(gen Generator, n int) Generator {
	gen.Seeds = min(gen.Seeds, n)
	return gen
}

// WithLoadScale returns gen with our sessions' burn scaled by burn and
// every teammate's by mates (the sensitivity of the results to the
// guessed burn rates; 1 = unchanged).
func WithLoadScale(gen Generator, burn, mates float64) Generator {
	mk := gen.Make
	gen.Make = func(seed uint64) Scenario {
		sc := mk(seed)
		sc.Load.Burn *= burn
		for i := range sc.Accounts {
			if m := sc.Accounts[i].Mate; m != nil {
				scaled := *m
				scaled.Burn *= mates
				sc.Accounts[i].Mate = &scaled
			}
		}
		return sc
	}
	return gen
}
