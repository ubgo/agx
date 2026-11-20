package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/pool"
)

// TestPoolExpiringSessions: under the expiring strategy each run's key
// request is its own pick: the most room first, at most max_sessions
// runs per member (counted from the live runs that asked within two
// key_ttls), the overflow to a waiting member, and a run stays on its
// member (sticky). agx pool pick, outside a run, shows the sessions per
// member.
func TestPoolExpiringSessions(t *testing.T) {
	m, _, _ := freshPool(t)
	m.f.WriteConfig(strings.Replace(poolConfig, `"key_ttl": "30s"`, `"strategy": "expiring", "max_sessions": 1, "key_ttl": "30s"`, 1))
	a := m.app(t)
	key := func(start string) (string, string) {
		t.Helper()
		id := liveRun(t, m, a, start)
		cr, err := m.app(t).Credential(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return id, cr.Login
	}
	// Every reset is the same: bob has the most room (78 under 98), then
	// alice (68); carol waits outside the borrow window; derek is over
	// his 5-hour cap.
	first, got := key("auto")
	if got != "bob" {
		t.Fatalf("first run on %s, want bob", got)
	}
	second, got := key("auto")
	if got != "alice" {
		t.Errorf("second run on %s, want alice (bob has max_sessions 1)", got)
	}
	third, got := key("auto")
	if got != "carol" {
		t.Errorf("third run on %s, want carol (overflow to the waiting member)", got)
	}
	// Each run asks again and stays where it is: its own seat is not
	// counted against it (without its run id every member would look
	// full and the least loaded, bob, would take the second run).
	for id, want := range map[string]string{first: "bob", second: "alice", third: "carol"} {
		m.env[run.EnvRunID] = id
		if cr, err := m.app(t).Credential(context.Background()); err != nil || cr.Login != want {
			t.Errorf("run on %s again: %+v %v (sticky)", want, cr, err)
		}
	}
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Strategy != pool.StrategyExpiring {
		t.Errorf("strategy %q", rep.Strategy)
	}
	sessions := map[string]int{}
	for _, c := range rep.Candidates {
		sessions[c.Name] = c.Sessions
		if c.State == pool.StateReady && c.UrgencyPercentPerHour == nil {
			t.Errorf("%s has no urgency", c.Name)
		}
	}
	if sessions["bob"] != 1 || sessions["alice"] != 1 || sessions["carol"] != 1 {
		t.Errorf("sessions %v", sessions)
	}
	// A run that stopped asking no longer counts.
	if _, err := live.Update(context.Background(), run.Dir(a.paths.Runs, third), func(s *live.State) error {
		s.InUseAt = agenttest.FixedNow().Add(-time.Hour)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rep, _ = a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	for _, c := range rep.Candidates {
		if c.Name == "carol" && c.Sessions != 0 {
			t.Errorf("an idle run still counts on carol: %d", c.Sessions)
		}
	}
}

// TestPoolDeadlineIgnoresSessions: today's rules give every run the one
// pick and read no live runs.
func TestPoolDeadlineIgnoresSessions(t *testing.T) {
	m, _, a := freshPool(t)
	for range 3 {
		liveRun(t, m, a, "auto")
		cr, err := m.app(t).Credential(context.Background())
		if err != nil || cr.Login != "alice" {
			t.Fatalf("deadline run on %+v %v, want alice", cr, err)
		}
	}
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	if err != nil || rep.Strategy != pool.StrategyDeadline {
		t.Fatalf("%v %v", rep.Strategy, err)
	}
	for _, c := range rep.Candidates {
		if c.Sessions != 0 || c.UrgencyPercentPerHour != nil {
			t.Errorf("deadline report counts sessions or urgency: %+v", c)
		}
	}
}
