package pool_test

import (
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// The selector's test/property.test.mjs, ported: thousands of seeded
// random pools and readings, each checked against a second, independent
// implementation of the rules written plainly below, and against
// invariants. A failure names its seed: SEED=1234 go test -run
// TestPortRandomised ./pool/ replays it; CASES=50000 runs more.

// propNow is the property tests' clock.
var propNow = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

// propCases is CASES (default 4000).
func propCases() int {
	if n, err := strconv.Atoi(os.Getenv("CASES")); err == nil && n > 0 {
		return n
	}
	return 4000
}

// propSeeds is SEED alone, or CASES seeds from 1.
func propSeeds() []uint64 {
	if s, err := strconv.ParseUint(os.Getenv("SEED"), 10, 64); err == nil {
		return []uint64{s}
	}
	out := make([]uint64, propCases())
	for i := range out {
		out[i] = uint64(i + 1)
	}
	return out
}

// gen is one seed's random source.
type gen struct{ r *rand.Rand }

func newGen(seed uint64) gen                { return gen{rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} }
func (g gen) chance(p float64) bool         { return g.r.Float64() < p }
func (g gen) intn(lo, hi int) int           { return lo + g.r.IntN(hi-lo+1) }
func (g gen) pickF(list ...float64) float64 { return list[g.r.IntN(len(list))] }
func (g gen) shuffle(list []string) []string {
	out := slices.Clone(list)
	g.r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// Member name pools, as the selector's generator.
var (
	propOwn    = []string{"o1", "o2", "o3"}
	propShared = []string{"s1", "s2", "s3", "s4"}
	propOther  = []string{"x1", "x2"}
)

// genConfig is a random valid pool; caps lean to edge values.
func genConfig(g gen) pool.Config {
	capV := func() float64 { return g.pickF(0, 1, 30, 50, 69, 70, 71, 95, 98, 99, 100, float64(g.intn(0, 100))) }
	c := pool.New(g.shuffle(propOwn)[:g.intn(0, 3)], g.shuffle(propShared)[:g.intn(0, 4)])
	c.BorrowWindow = time.Duration(g.pickF(0, 1, 24, 48, 48, 72, 1000)) * time.Hour
	c.ShortUnavailableAt = g.pickF(99, 99, 90, 100, float64(g.intn(50, 100)))
	if g.chance(0.7) {
		c.Defaults = pool.Caps{Short: capV(), Week: capV()}
	}
	if g.chance(0.8) {
		c.Accounts = map[string]pool.Account{}
		for _, m := range slices.Concat(c.Own, c.Shared) {
			if !g.chance(0.6) {
				continue
			}
			var a pool.Account
			if g.chance(0.6) {
				v := capV()
				a.Short = &v
			}
			if g.chance(0.6) {
				v := capV()
				a.Week = &v
			}
			if g.chance(0.2) {
				b := g.chance(0.5)
				a.Enabled = &b
			}
			c.Accounts[m] = a
		}
	}
	return c
}

// genReadings is a random reading per name, odd ones included: missing,
// no windows, usage that is not a number, stale and unreadable resets,
// bad statuses.
func genReadings(g gen) map[string]pool.Reading {
	percent := func() float64 {
		if g.chance(0.04) {
			return math.NaN()
		}
		return g.pickF(0, 49.9, 50, 69.9, 70, 94.9, 95, 97.9, 98, 98.9, 99, 100, float64(g.intn(0, 100)), math.Round(g.r.Float64()*1000)/10)
	}
	out := map[string]pool.Reading{}
	for _, name := range slices.Concat(propOwn, propShared, propOther) {
		if !g.chance(0.85) {
			continue
		}
		r := pool.Reading{Status: pool.StatusOK}
		if !g.chance(0.8) {
			r.Status = []string{"limit-reached", "error"}[g.r.IntN(2)]
		}
		if g.chance(0.97) {
			if g.chance(0.75) {
				r.Short = &pool.Window{Percent: percent()}
			}
			if g.chance(0.9) {
				w := &pool.Window{Percent: percent()}
				if g.chance(0.9) {
					var t time.Time
					if !g.chance(0.04) {
						h := g.pickF(-5, 0.5, 1, 24, 47.9, 48, 48.1, 72, 100, float64(g.intn(-10, 200)))
						t = propNow.Add(time.Duration(h * float64(time.Hour)))
					}
					w.ResetsAt = &t
				}
				r.Week = w
			}
		}
		out[name] = r
	}
	return out
}

// refAccount is one member in the reference implementation.
type refAccount struct {
	name   string
	own    bool
	order  int
	usable bool
	hours  float64
}

// refUsable is the reference's usability rule, written independently.
func refUsable(r pool.Reading, ok bool, name string, c pool.Config) bool {
	a := c.Accounts[name]
	if a.Enabled != nil && !*a.Enabled {
		return false
	}
	if !ok || r.Status != "ok" {
		return false
	}
	short, week := c.Defaults.Short, c.Defaults.Week
	if a.Short != nil {
		short = *a.Short
	}
	if a.Week != nil {
		week = *a.Week
	}
	for _, w := range []struct {
		win     *pool.Window
		cap     float64
		isShort bool
	}{{r.Short, short, true}, {r.Week, week, false}} {
		if w.win == nil {
			continue
		}
		v := w.win.Percent
		if v != v || v > math.MaxFloat64 || v < -math.MaxFloat64 {
			return false
		}
		if w.isShort && v >= c.ShortUnavailableAt {
			return false
		}
		if v >= w.cap {
			return false
		}
	}
	return true
}

// refHours is the hours to the weekly reset; +Inf when unknown, zero
// (unreadable) or passed.
func refHours(r pool.Reading, ok bool) float64 {
	if !ok || r.Week == nil || r.Week.ResetsAt == nil || r.Week.ResetsAt.IsZero() || r.Week.ResetsAt.Before(propNow) {
		return math.Inf(1)
	}
	return r.Week.ResetsAt.Sub(propNow).Hours()
}

// refPick is the reference selection: the ready order and the usable set.
func refPick(c pool.Config, data map[string]pool.Reading) (pool []string, usable []string, accts []refAccount) {
	for i, n := range c.Own {
		accts = append(accts, refAccount{name: n, own: true, order: i})
	}
	for i, n := range c.Shared {
		accts = append(accts, refAccount{name: n, order: len(c.Own) + i})
	}
	anyOwn := false
	for i := range accts {
		r, ok := data[accts[i].name]
		accts[i].usable = refUsable(r, ok, accts[i].name, c)
		accts[i].hours = refHours(r, ok)
		if accts[i].usable {
			usable = append(usable, accts[i].name)
			anyOwn = anyOwn || accts[i].own
		}
	}
	var in []refAccount
	for _, a := range accts {
		if a.usable && (a.own || !anyOwn || a.hours <= c.BorrowWindow.Hours()) {
			in = append(in, a)
		}
	}
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].hours == in[j].hours {
			return in[i].order < in[j].order
		}
		return in[i].hours < in[j].hours
	})
	for _, a := range in {
		pool = append(pool, a.name)
	}
	return pool, usable, accts
}

// runCase draws one case and selects.
func runCase(t *testing.T, seed uint64) (pool.Config, map[string]pool.Reading, pool.Result) {
	t.Helper()
	g := newGen(seed)
	c := genConfig(g)
	data := genReadings(g)
	r, err := pool.Select(c, data, pool.Options{Now: propNow})
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	return c, data, r
}

// TestPortRandomised is describe("randomised (N cases per test, seeded)").
func TestPortRandomised(t *testing.T) {
	seeds := propSeeds()
	t.Run("generated configs are valid (the generator itself is sound)", func(t *testing.T) {
		for _, s := range seeds {
			if err := genConfig(newGen(s)).Validate(); err != nil {
				t.Fatalf("seed %d: %v", s, err)
			}
		}
	})
	t.Run("pick and full pick order match the independent reference", func(t *testing.T) {
		for _, s := range seeds {
			c, data, r := runCase(t, s)
			want, usable, _ := refPick(c, data)
			if got := names(r, pool.StateReady); !slices.Equal(got, want) {
				t.Fatalf("seed %d: ready %v, reference %v", s, got, want)
			}
			wantSel := ""
			if len(want) > 0 {
				wantSel = want[0]
			}
			if selected(r) != wantSel {
				t.Fatalf("seed %d: selected %q, reference %q", s, selected(r), wantSel)
			}
			var eligible []string
			for _, cd := range r.Candidates {
				if cd.Eligible {
					eligible = append(eligible, cd.Name)
				}
			}
			slices.Sort(eligible)
			slices.Sort(usable)
			if !slices.Equal(eligible, usable) {
				t.Fatalf("seed %d: usable %v, reference %v", s, eligible, usable)
			}
		}
	})
	t.Run("invariants hold for every case", func(t *testing.T) {
		picked, sharedOverOwn := 0, 0
		for _, s := range seeds {
			c, data, r := runCase(t, s)
			none := true
			for _, cd := range r.Candidates {
				none = none && !cd.Eligible
			}
			if (r.Selected == nil) != none {
				t.Fatalf("seed %d: nothing selected %v, nothing usable %v", s, r.Selected == nil, none)
			}
			if r.Selected == nil {
				continue
			}
			picked++
			cd := cand(t, r, r.Selected.Name)
			if !cd.Eligible || len(cd.Reasons) != 0 || !cd.Enabled || cd.Status != pool.StatusOK {
				t.Fatalf("seed %d: pick %+v not usable", s, cd)
			}
			if cd.Short != nil && (*cd.Short >= cd.Caps.Short || *cd.Short >= r.ShortUnavailableAt) {
				t.Fatalf("seed %d: pick over the 5h cap or cutoff", s)
			}
			if cd.Week != nil && *cd.Week >= cd.Caps.Week {
				t.Fatalf("seed %d: pick over the weekly cap", s)
			}
			if !slices.Contains(slices.Concat(c.Own, c.Shared), cd.Name) {
				t.Fatalf("seed %d: unlisted %s picked", s, cd.Name)
			}
			var ownUsable []pool.Candidate
			for _, x := range r.Candidates {
				if x.Group == pool.GroupOwn && x.Eligible {
					ownUsable = append(ownUsable, x)
				}
			}
			if cd.Group == pool.GroupShared && len(ownUsable) > 0 {
				sharedOverOwn++
				if cd.UntilReset == nil || *cd.UntilReset < 0 || *cd.UntilReset > c.BorrowWindow {
					t.Fatalf("seed %d: shared %s borrowed outside the window", s, cd.Name)
				}
				for _, o := range ownUsable {
					if o.UntilReset != nil && *o.UntilReset >= 0 && *o.UntilReset < *cd.UntilReset {
						t.Fatalf("seed %d: shared %s not sooner than own %s", s, cd.Name, o.Name)
					}
				}
			}
			want, _, accts := refPick(c, data)
			hoursOf := func(n string) float64 {
				for _, a := range accts {
					if a.name == n {
						return a.hours
					}
				}
				return math.Inf(1)
			}
			for _, n := range want {
				if hoursOf(n) < hoursOf(r.Selected.Name) {
					t.Fatalf("seed %d: %s in the pool resets sooner than the pick", s, n)
				}
			}
		}
		if len(seeds) > 1 && (picked < len(seeds)*3/10 || sharedOverOwn < len(seeds)/200) {
			t.Fatalf("generator too narrow: %d picks, %d borrows of %d", picked, sharedOverOwn, len(seeds))
		}
	})
	t.Run("agx record order never changes the result (unique records)", func(t *testing.T) {
		for _, s := range seeds {
			g := newGen(s)
			c := genConfig(g)
			data := genReadings(g)
			// Rebuild the map in a shuffled insertion order; Go's map
			// order is also random on every range.
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			shuffled := map[string]pool.Reading{}
			for _, k := range newGen(s + 99991).shuffle(keys) {
				shuffled[k] = data[k]
			}
			a, _ := pool.Select(c, data, pool.Options{Now: propNow})
			b, _ := pool.Select(c, shuffled, pool.Options{Now: propNow})
			if selected(a) != selected(b) || !slices.Equal(names(a, ""), names(b, "")) {
				t.Fatalf("seed %d: order changed the result", s)
			}
		}
	})
	t.Run("force always returns the forced account, whatever its state", func(t *testing.T) {
		for _, s := range seeds[:min(len(seeds), 1000)] {
			g := newGen(s)
			c := genConfig(g)
			data := genReadings(g)
			if len(data) == 0 {
				continue
			}
			present := make([]string, 0, len(data))
			for k := range data {
				present = append(present, k)
			}
			slices.Sort(present)
			name := present[g.r.IntN(len(present))]
			r, err := pool.Select(c, data, pool.Options{Now: propNow, Force: name})
			if err != nil || r.Selected.Name != name || !r.Selected.Forced {
				t.Fatalf("seed %d: force %s gave %+v %v", s, name, r.Selected, err)
			}
			if _, err := pool.Select(c, data, pool.Options{Now: propNow, Force: "nobody"}); err == nil {
				t.Fatalf("seed %d: forcing nobody succeeded", s)
			}
		}
	})
	t.Run("selection is pure: same input, same output, inputs untouched", func(t *testing.T) {
		for _, s := range seeds[:min(len(seeds), 500)] {
			g := newGen(s)
			c := genConfig(g)
			data := genReadings(g)
			before := snapshotOf(c, data)
			a, _ := pool.Select(c, data, pool.Options{Now: propNow})
			b, _ := pool.Select(c, data, pool.Options{Now: propNow})
			if !reflect.DeepEqual(a, b) || snapshotOf(c, data) != before {
				t.Fatalf("seed %d: not pure", s)
			}
		}
	})
}

// snapshotOf renders inputs for a before/after comparison (NaN-safe).
func snapshotOf(c pool.Config, data map[string]pool.Reading) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := strings.Join(c.Own, ",") + "|" + strings.Join(c.Shared, ",") + "|"
	for _, k := range keys {
		r := data[k]
		out += k + r.Status
		for _, w := range []*pool.Window{r.Short, r.Week} {
			if w != nil {
				out += strconv.FormatFloat(w.Percent, 'g', -1, 64)
				if w.ResetsAt != nil {
					out += w.ResetsAt.String()
				}
			}
		}
	}
	return out
}
