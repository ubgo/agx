package pool_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// Real-snapshot replays: a real pool's `agx pool pick --cached --json`,
// anonymised by scripts/anon-pool-snapshot.py (made-up names, the same
// numbers, statuses, resets and caps), replayed through pool.Select under
// the default rules. Each fixture is one moment a rule had to get right.

// pickFixture is an anonymised agx pool pick envelope's data.
type pickFixture struct {
	Now        time.Time `json:"now"`
	Candidates []struct {
		Name              string     `json:"name"`
		Group             pool.Group `json:"group"`
		Status            string     `json:"status"`
		Short             *float64   `json:"short"`
		Week              *float64   `json:"week"`
		WeeklyResetsAt    *time.Time `json:"weekly_resets_at"`
		Caps              pool.Caps  `json:"caps"`
		WeekCapConfigured *float64   `json:"week_cap_configured"`
		Sessions          int        `json:"sessions"`
	} `json:"candidates"`
}

// loadPick reads a fixture into a pool under the default rules (each
// member's caps as its own entry: the configured weekly cap, not the
// relaxed one, and the 5-hour cap in effect), its readings and the
// sessions each member had.
func loadPick(t *testing.T, file string) (pool.Config, map[string]pool.Reading, map[string]int, time.Time) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var fx pickFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	var own, shared []string
	accounts := map[string]pool.Account{}
	readings := map[string]pool.Reading{}
	load := map[string]int{}
	for _, c := range fx.Candidates {
		if c.Group == pool.GroupOwn {
			own = append(own, c.Name)
		} else {
			shared = append(shared, c.Name)
		}
		week := c.Caps.Week
		if c.WeekCapConfigured != nil {
			week = *c.WeekCapConfigured
		}
		accounts[c.Name] = pool.Account{Short: f(c.Caps.Short), Week: f(week)}
		rd := pool.Reading{Status: c.Status}
		if c.Short != nil {
			rd.Short = &pool.Window{Percent: *c.Short}
		}
		if c.Week != nil {
			rd.Week = &pool.Window{Percent: *c.Week, ResetsAt: c.WeeklyResetsAt}
		}
		readings[c.Name] = rd
		load[c.Name] = c.Sessions
	}
	cfg := pool.New(own, shared)
	cfg.Accounts = accounts
	return cfg, readings, load, fx.Now
}

// readyOrder is the ready members in rank order.
func readyOrder(r pool.Result) []string {
	var out []string
	for _, c := range r.Candidates {
		if c.State == pool.StateReady {
			out = append(out, c.Name)
		}
	}
	return out
}

// TestSnapshotPickDeadlineFirst replays a real pool on a Saturday
// evening: derek (shared) resets in 6.7h at week 87% with his cap risen
// to 88% (1% of room now, 8% before it reaches 95% at the reset), bob
// (shared) resets in 12.7h at 78% (17% before the reset), and two own
// members reset days later. agx v0.4.19 counted derek's room to his cap
// now and ranked him last of the four, with no session on him, so his 8%
// was about to be lost. Now he leads: both shared members are inside the
// 24h deadline window, and the sooner reset goes first.
func TestSnapshotPickDeadlineFirst(t *testing.T) {
	cfg, readings, load, at := loadPick(t, "testdata/pick-2026-11-21-anon.json")
	got, r := pick(t, cfg, readings, pool.Options{Now: at})
	if want := []string{"derek", "bob", "alice", "carol"}; got != "derek" || !slices.Equal(readyOrder(r), want) {
		t.Fatalf("picked %s, order %v; want derek, order %v", got, readyOrder(r), want)
	}
	if !strings.HasPrefix(r.Selected.Reason, "resets soonest: 8% left before its weekly cap reaches 95% at the reset in 6.7h") {
		t.Errorf("reason %q", r.Selected.Reason)
	}
	d := candidateOf(t, r, "derek")
	if d.Room == nil || *d.Room != 1 || d.RoomAtReset == nil || *d.RoomAtReset != 8 || d.NeedsPerHour == nil || *d.NeedsPerHour != 1.19 {
		t.Errorf("derek room %v, at reset %v, needs %v; want 1, 8, 1.19", d.Room, d.RoomAtReset, d.NeedsPerHour)
	}
	// With the sessions the pool had (alice 4, bob 3), a new session
	// still goes to derek.
	if got, _ := pick(t, cfg, readings, pool.Options{Now: at, Load: load}); got != "derek" {
		t.Errorf("with the real load: picked %s, want derek", got)
	}
	// Urgency alone (deadline_window 0s) ranks bob first: 17/12.7 =
	// 1.34%/h beats 8/6.7 = 1.19%/h, and derek's room can expire.
	cfg.DeadlineWindow = 0
	if _, r := pick(t, cfg, readings, pool.Options{Now: at}); !slices.Equal(readyOrder(r), []string{"bob", "derek", "alice", "carol"}) {
		t.Errorf("urgency alone: order %v, want bob, derek, alice, carol", readyOrder(r))
	}
	checkInvariants(t, "snapshot", cfg, readings, pool.Options{Now: at, Load: load})
}
