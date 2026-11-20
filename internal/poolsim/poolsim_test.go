package poolsim

import (
	"reflect"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// testSeeds and testStep keep the regression runs fast: five-minute
// steps (the report uses one minute) and a few seeds per scenario.
const (
	testSeeds = 6
	testStep  = 5 * time.Minute
)

// scenario finds a report scenario by name, at the test step.
func scenario(t *testing.T, name string) Generator {
	t.Helper()
	for _, g := range Scenarios() {
		if g.Name == name {
			return WithTiming(g, testStep, DefaultLag)
		}
	}
	t.Fatalf("no scenario %s", name)
	return Generator{}
}

// members are a scenario's members as pickers see them.
func members(sc Scenario) []Member {
	out := make([]Member, len(sc.Accounts))
	for i, a := range sc.Accounts {
		out[i] = Member{Name: a.Name, Group: a.Group}
	}
	return out
}

// real are the shipped strategies (package pool) on gen's members.
func real(gen Generator) (today, rec Strategy) {
	m := members(gen.Make(1))
	return RealToday(m), RealRecommended(m)
}

// TestPolicyMatchesPool: the simulator's Policy for today's rules and
// for the recommendation make exactly the picks of the shipped code, so
// the tables measure what agx does.
func TestPolicyMatchesPool(t *testing.T) {
	for _, name := range []string{"snapshot", "heavy", "near-caps"} {
		gen := scenario(t, name)
		today, rec := real(gen)
		sums := Evaluate(gen, []Strategy{policy("A", "", Today()), today, policy("REC", "", Recommended()), rec}, 3, DefaultWeights)
		if !reflect.DeepEqual(sums[0].Mean, sums[1].Mean) {
			t.Errorf("%s: Policy A %+v\nvs pool %+v", name, sums[0].Mean, sums[1].Mean)
		}
		if !reflect.DeepEqual(sums[2].Mean, sums[3].Mean) {
			t.Errorf("%s: Policy REC %+v\nvs pool %+v", name, sums[2].Mean, sums[3].Mean)
		}
	}
}

// TestRegressionExpiringBeatsDeadline keeps the recommendation's gains:
// on every scenario the shipped expiring strategy wastes no more weekly
// quota than today's rules and serves no less of our demand; where quota
// is short it wastes clearly less; it locks teammates out less, hits
// almost no limits and switches accounts less. A change to the pool
// rules that loses one of these fails here.
func TestRegressionExpiringBeatsDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("simulates weeks")
	}
	cases := []struct {
		name string
		// short is a scenario where sessions want more than the pool
		// has, so the waste must drop by at least minGain points.
		short   bool
		minGain float64
	}{
		{"snapshot", false, 0},
		{"snapshot-busy", true, 1.5},
		{"normal", false, 0},
		{"heavy", true, 1.5},
		{"mate-on-preferred", false, 0},
		{"near-caps", false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gen := scenario(t, c.name)
			today, rec := real(gen)
			sums := Evaluate(gen, []Strategy{today, rec}, testSeeds, DefaultWeights)
			a, r := sums[0].Mean, sums[1].Mean
			if r.WasteTotal > a.WasteTotal+wasteSlack {
				t.Errorf("waste %.2f%% per account-week, today %.2f%%", r.WasteTotal, a.WasteTotal)
			}
			if c.short && a.WasteTotal-r.WasteTotal < c.minGain {
				t.Errorf("quota is short but waste only fell from %.2f%% to %.2f%% (want -%.1f)", a.WasteTotal, r.WasteTotal, c.minGain)
			}
			if r.Served < a.Served-servedSlack {
				t.Errorf("served %.3f, today %.3f", r.Served, a.Served)
			}
			if r.LockoutHours > a.LockoutHours+lockoutSlack {
				t.Errorf("teammates locked out %.2f h/week, today %.2f", r.LockoutHours, a.LockoutHours)
			}
			if r.Switches > a.Switches {
				t.Errorf("switches %.2f per session-day, today %.2f", r.Switches, a.Switches)
			}
			if !c.short && r.Hits429 > maxHits429 {
				t.Errorf("%.2f limit hits a week with quota to spare", r.Hits429)
			}
			if r.Hits429 > a.Hits429+maxHits429 {
				t.Errorf("limit hits %.2f a week, today %.2f", r.Hits429, a.Hits429)
			}
		})
	}
}

// Regression tolerances: seed noise, not a loss.
const (
	wasteSlack   = 0.2
	servedSlack  = 0.002
	lockoutSlack = 0.1
	maxHits429   = 1.0
)

// TestDeterministic: a scenario and a seed always give the same run.
func TestDeterministic(t *testing.T) {
	gen := scenario(t, "normal")
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
		Name: "one", Start: start, Horizon: 24 * time.Hour, Step: time.Hour, WeekRatio: DefaultWeekRatio, Lag: 1,
		BaseCaps: defaultBaseCaps(), Seed: 1,
		Accounts: []Account{{Name: "own", Group: pool.GroupOwn, Capacity: 1, WeekResetAt: start.Add(12 * time.Hour)}},
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
	return Strategy{Name: name, New: func() Picker { return always(name) }}
}

// TestRunMechanics: the weekly reset records the room left, the 5-hour
// window caps a step's use and counts the refused session as a hit, and
// every hour of demand is counted.
func TestRunMechanics(t *testing.T) {
	m := Run(oneAccount(1), on("own"))
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
	heavy := Run(oneAccount(12), on("own"))
	if heavy.Hits429 == 0 || heavy.Served >= 1 {
		t.Errorf("12 sessions: hits %v, served %v", heavy.Hits429, heavy.Served)
	}
	none := Run(oneAccount(2), Strategy{Name: "none", New: func() Picker { return always("") }})
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
	if m := Run(mk(30), on("own")); m.LockoutHours == 0 {
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
	rec := Strategy{Name: "rec", New: func() Picker { return &recorder{seen: &seen} }}
	Run(sc, rec)
	if seen[0].Short != nil {
		t.Errorf("before any use the 5-hour window is %+v", seen[0].Short)
	}
	want := sc.Start.Add(ShortWindow)
	if seen[2].Short == nil || !seen[2].Short.ResetsAt.Equal(want) {
		t.Errorf("after the first hour: %+v, want a window ending %s", seen[2].Short, want)
	}
}

// recorder records the readings of "own" and puts sessions on it.
type recorder struct{ seen *[]pool.Reading }

// Assign implements Picker.
func (r *recorder) Assign(v View) []string {
	*r.seen = append(*r.seen, v.Readings["own"])
	return always("own").Assign(v)
}
