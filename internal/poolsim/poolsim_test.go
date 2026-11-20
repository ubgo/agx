package poolsim

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// update rewrites the gate's baseline: go test ./internal/poolsim/ -run
// TestSimGate -args -update (only after a deliberate change of the rules,
// with the new numbers reviewed).
var update = flag.Bool("update", false, "rewrite pool/testdata/sim/baseline.json")

// simDir holds the scenario files, relative to this package.
var simDir = filepath.Join("..", "..", "pool", "testdata", "sim")

// baselineFile is the gate's recorded numbers.
var baselineFile = filepath.Join(simDir, "baseline.json")

// The gate runs faster than task sim: five-minute steps and a few seeds
// of each gate scenario.
const (
	gateSeeds = 4
	gateStep  = 5 * time.Minute
)

// loadAll reads every scenario file.
func loadAll(t *testing.T) []Generator {
	t.Helper()
	gens, err := LoadDir(simDir)
	if err != nil {
		t.Fatal(err)
	}
	return gens
}

// scenario finds a scenario file by name, at the gate's timing.
func scenario(t *testing.T, name string) Generator {
	t.Helper()
	for _, g := range loadAll(t) {
		if g.Name == name {
			return WithSeeds(WithTiming(g, gateStep, DefaultLag), gateSeeds)
		}
	}
	t.Fatalf("no scenario %s", name)
	return Generator{}
}

// madeUp are the account names a scenario may use.
var madeUp = regexp.MustCompile(`^(alice|bob|carol|derek|erin|frank|grace|heidi|ivan|judy|ken|liam|mallory|niaj|olivia|peggy|quinn|rupert|sybil|trent|uma|victor|walter|xena|yara|zoe)$`)

// TestScenarioFiles: every file loads, every group has scenarios, every
// account name is a made-up one, and the range the brief asks for is
// covered (1 to 20+ members, all-own and all-shared, every plan, no
// teammates, bursty sessions, 1 to many sessions).
func TestScenarioFiles(t *testing.T) {
	gens := loadAll(t)
	groups := map[string]int{}
	var minMembers, maxMembers, maxSessions = 99, 0, 0
	allOwn, allShared, bursty, noMates, unknownPlan := false, false, false, false, false
	plans := map[string]bool{}
	for _, g := range gens {
		groups[g.Group]++
		for s := range g.Seeds {
			sc := g.Make(g.Seed + uint64(s))
			minMembers, maxMembers = min(minMembers, len(sc.Accounts)), max(maxMembers, len(sc.Accounts))
			maxSessions = max(maxSessions, sc.Load.Sessions)
			own, mates := 0, 0
			for _, a := range sc.Accounts {
				if !madeUp.MatchString(a.Name) {
					t.Errorf("%s: account %q is not a made-up name", g.Name, a.Name)
				}
				plans[a.Plan] = true
				unknownPlan = unknownPlan || !a.CapacityKnown
				if a.Group == pool.GroupOwn {
					own++
				}
				if a.Mate != nil {
					mates++
				}
			}
			allOwn = allOwn || own == len(sc.Accounts)
			allShared = allShared || own == 0
			noMates = noMates || (own < len(sc.Accounts) && mates == 0)
			bursty = bursty || sc.Load.Bursty
		}
	}
	for _, grp := range GroupOrder {
		if groups[grp] < 3 {
			t.Errorf("group %s has %d scenarios, want 3 or more", grp, groups[grp])
		}
	}
	if minMembers != 1 || maxMembers < 20 || maxSessions < 15 || !allOwn || !allShared || !bursty || !noMates || len(plans) != 3 || !unknownPlan {
		t.Errorf("range: members %d..%d, sessions up to %d, all-own %v, all-shared %v, bursty %v, no teammates %v, plans %v, unknown plan %v",
			minMembers, maxMembers, maxSessions, allOwn, allShared, bursty, noMates, plans, unknownPlan)
	}
}

// TestPolicyMatchesPool: the simulator's Policy for today's rules and
// for the recommendation make exactly the picks of the shipped code, so
// the tables measure what agx does.
func TestPolicyMatchesPool(t *testing.T) {
	for _, name := range []string{"solo-3-mixed-plans", "small-all-shared", "large-mixed", "snapshot-12-busy", "mixed-1-2", "team-6"} {
		gen := WithSeeds(scenario(t, name), 2)
		w := Recommended()
		w.Weighted = true
		sums := Evaluate(gen, []Strategy{policy("A", "", Today()), RealToday(), policy("REC", "", Recommended()), RealRecommended(), policy("REC+W", "", w), RealWeighted()}, DefaultWeights)
		if !reflect.DeepEqual(sums[0].Mean, sums[1].Mean) {
			t.Errorf("%s: Policy A %+v\nvs pool %+v", name, sums[0].Mean, sums[1].Mean)
		}
		if !reflect.DeepEqual(sums[2].Mean, sums[3].Mean) {
			t.Errorf("%s: Policy REC %+v\nvs pool %+v", name, sums[2].Mean, sums[3].Mean)
		}
		if !reflect.DeepEqual(sums[4].Mean, sums[5].Mean) {
			t.Errorf("%s: Policy REC+W %+v\nvs pool %+v", name, sums[4].Mean, sums[5].Mean)
		}
	}
}

// gateMetrics are the numbers the gate records per scenario and
// strategy.
type gateMetrics struct {
	WasteTotal   float64 `json:"waste_total"`
	Hits429      float64 `json:"hits_429"`
	StallHours   float64 `json:"stall_hours"`
	LockoutHours float64 `json:"lockout_hours"`
	Served       float64 `json:"served"`
	Switches     float64 `json:"switches"`
}

// of is m's gate numbers.
func of(m Metrics) gateMetrics {
	return gateMetrics{WasteTotal: m.WasteTotal, Hits429: m.Hits429, StallHours: m.StallHours, LockoutHours: m.LockoutHours, Served: m.Served, Switches: m.Switches}
}

// Gate tolerances above the baseline: seed noise is zero (the runs are
// deterministic), so these only absorb float rounding across platforms.
const (
	gateWaste   = 0.05
	gateHits    = 0.05
	gateStall   = 0.05
	gateLockout = 0.05
	gateServed  = 0.0005
)

// Defaults-versus-classic slack on the same runs.
const (
	wasteSlack   = 0.2
	servedSlack  = 0.002
	lockoutSlack = 0.15
	// switchSlack: a one-tier pool switches almost never under the
	// classic preset, and sticky_leave_ratio may add a few switches there.
	switchSlack = 1.0
)

// Baseline keys: the classic preset and the default rules.
const (
	gateClassic  = pool.PresetClassic
	gateDefaults = "defaults"
)

// TestSimGate is the regression gate (go test, task check): the default
// rules and the classic preset (reference A, made only of config values)
// on the gate scenarios must not waste more quota, hit more limits,
// stall more, lock teammates out more or serve less than the recorded
// baseline; and the defaults must not lose to the classic preset on
// waste, service, lockouts or switches. A change to
// the pool rules that loses one of these fails here; -update records new
// numbers after a deliberate change.
func TestSimGate(t *testing.T) {
	if testing.Short() {
		t.Skip("simulates weeks")
	}
	got := map[string]map[string]gateMetrics{}
	gated := map[string]bool{}
	for _, g := range loadAll(t) {
		if !g.Gate {
			continue
		}
		gated[g.Group] = true
		gen := WithSeeds(WithTiming(g, gateStep, DefaultLag), gateSeeds)
		sums := Evaluate(gen, []Strategy{RealToday(), RealRecommended()}, DefaultWeights)
		got[g.Name] = map[string]gateMetrics{gateClassic: of(sums[0].Mean), gateDefaults: of(sums[1].Mean)}
		a, r := sums[0].Mean, sums[1].Mean
		if r.WasteTotal > a.WasteTotal+wasteSlack || r.Served < a.Served-servedSlack || r.LockoutHours > a.LockoutHours+lockoutSlack || r.Switches > a.Switches+switchSlack {
			t.Errorf("%s: the defaults %+v lose to the classic preset %+v", g.Name, of(r), of(a))
		}
	}
	for _, grp := range GroupOrder {
		if !gated[grp] {
			t.Errorf("no gate scenario in group %s", grp)
		}
	}
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(baselineFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("%v (record it with -args -update)", err)
	}
	var base map[string]map[string]gateMetrics
	if err := json.Unmarshal(b, &base); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		for st, m := range got[n] {
			b, ok := base[n][st]
			if !ok {
				t.Errorf("%s %s: no baseline (record it with -args -update)", n, st)
				continue
			}
			if m.WasteTotal > b.WasteTotal+gateWaste || m.Hits429 > b.Hits429+gateHits || m.StallHours > b.StallHours+gateStall ||
				m.LockoutHours > b.LockoutHours+gateLockout || m.Served < b.Served-gateServed {
				t.Errorf("%s %s: %+v is worse than the baseline %+v", n, st, m, b)
			}
		}
	}
}

// TestDeterministic: a scenario and a seed always give the same run.
func TestDeterministic(t *testing.T) {
	gen := scenario(t, "small-mixed")
	st := policy("REC", "", Recommended())
	a, b := Run(gen.Make(7), st), Run(gen.Make(7), st)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("two runs differ:\n%+v\n%+v", a, b)
	}
}

// oneAccount is a scenario with one own account and no teammate.
func oneAccount(sessions int) Scenario {
	start := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	return Scenario{
		Name: "alice", Start: start, Horizon: 24 * time.Hour, Step: time.Hour, WeekRatio: DefaultWeekRatio, Lag: 1,
		BaseCaps: defaultBaseCaps(), Seed: 1,
		Accounts: []Account{{Name: "alice", Group: pool.GroupOwn, Capacity: 1, WeekResetAt: start.Add(12 * time.Hour)}},
		Load:     Load{Sessions: sessions, Burn: 10, Duty: 1},
	}
}

// always is a picker that puts every busy session on one account.
type always string

// Assign implements Picker.
func (a always) Assign(v View) []string {
	out := make([]string, len(v.Sessions))
	for i, s := range v.Sessions {
		if s.Busy {
			out[i] = string(a)
		}
	}
	return out
}

// on is a strategy of always.
func on(name string) Strategy {
	return Strategy{Name: name, New: func(Scenario) Picker { return always(name) }}
}

// TestRunMechanics: the weekly reset records the room left, the 5-hour
// window caps a step's use and counts the refused session as a hit, and
// every hour of demand is counted.
func TestRunMechanics(t *testing.T) {
	m := Run(oneAccount(1), on("alice"))
	// 12 hours at 10%/h of a 5-hour window = 1.25% of the week an hour:
	// 15% of the week used when it resets at 12:00.
	if len(m.Resets) == 0 || m.Resets[0].Left != Full-15 {
		t.Errorf("resets %+v, want 85%% left at the first", m.Resets)
	}
	if m.Served != 1 || m.Hits429 != 0 {
		t.Errorf("served %v, hits %v", m.Served, m.Hits429)
	}
	// Twelve sessions burn 120% of a 5-hour window an hour: the first
	// hour is cut at 100% and every session on it is refused.
	heavy := Run(oneAccount(12), on("alice"))
	if heavy.Hits429 == 0 || heavy.Served >= 1 {
		t.Errorf("12 sessions: hits %v, served %v", heavy.Hits429, heavy.Served)
	}
	none := Run(oneAccount(2), Strategy{Name: "none", New: func(Scenario) Picker { return always("") }})
	if none.StallHours == 0 || none.Served != 0 {
		t.Errorf("no account: stall %v, served %v", none.StallHours, none.Served)
	}
}

// TestLockoutAttribution: a teammate refused because our use filled the
// window is our lockout; one who would hit the limit alone is not.
func TestLockoutAttribution(t *testing.T) {
	mk := func(burn float64) Scenario {
		sc := oneAccount(5)
		sc.Accounts[0].Group = pool.GroupShared
		sc.Accounts[0].Mate = &Teammate{WorkStart: 0, WorkEnd: 0, Burn: burn, Duty: 1}
		for d := range sc.Accounts[0].Mate.Workdays {
			sc.Accounts[0].Mate.Workdays[d] = true
		}
		return sc
	}
	if m := Run(mk(30), on("alice")); m.LockoutHours == 0 {
		t.Error("teammate at 30%/h with our 50%/h on top: no lockout counted")
	}
	if m := Run(mk(200), on("")); m.LockoutHours != 0 {
		t.Errorf("teammate alone over the limit: %v lockout hours counted as ours", m.LockoutHours)
	}
}

// TestShortWindowStartsWithFirstUse: the 5-hour window opens with the
// first use and its reading carries the end five hours later.
func TestShortWindowStartsWithFirstUse(t *testing.T) {
	sc := oneAccount(1)
	var seen []pool.Reading
	rec := Strategy{Name: "rec", New: func(Scenario) Picker { return &recorder{seen: &seen} }}
	Run(sc, rec)
	if seen[0].Short != nil {
		t.Errorf("before any use the 5-hour window is %+v", seen[0].Short)
	}
	want := sc.Start.Add(ShortWindow)
	if seen[2].Short == nil || !seen[2].Short.ResetsAt.Equal(want) {
		t.Errorf("after the first hour: %+v, want a window ending %s", seen[2].Short, want)
	}
}

// recorder records the readings of "alice" and puts sessions on it.
type recorder struct{ seen *[]pool.Reading }

// Assign implements Picker.
func (r *recorder) Assign(v View) []string {
	*r.seen = append(*r.seen, v.Readings["alice"])
	return always("alice").Assign(v)
}
