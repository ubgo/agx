package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// TestPoolSessions: under the default rules each run's key
// request is its own pick: the most room first, at most max_sessions
// runs per member (counted from the live runs that asked within two
// key_ttls), the overflow to a waiting member, and a run stays on its
// member (sticky). agx pool pick, outside a run, shows the sessions per
// member.
func TestPoolSessions(t *testing.T) {
	m, _, _ := freshPool(t)
	m.f.WriteConfig(strings.Replace(poolConfig, poolClassicKeys, `"max_sessions": 1, `, 1))
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
	if rep.Order != pool.OrderUrgency {
		t.Errorf("order %q", rep.Order)
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

// TestPoolClassicIgnoresSessions: the classic preset (deadline order, no
// session limit, not sticky) gives every run the one pick and reads no
// live runs.
func TestPoolClassicIgnoresSessions(t *testing.T) {
	m, _, a := freshPool(t)
	for range 3 {
		liveRun(t, m, a, "auto")
		cr, err := m.app(t).Credential(context.Background())
		if err != nil || cr.Login != "alice" {
			t.Fatalf("deadline run on %+v %v, want alice", cr, err)
		}
	}
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	if err != nil || rep.Order != pool.OrderDeadline {
		t.Fatalf("%v %v", rep.Order, err)
	}
	for _, c := range rep.Candidates {
		if c.Sessions != 0 {
			t.Errorf("classic report counts sessions: %+v", c)
		}
	}
}

// TestPoolDeadlineReport: agx pool pick under the default rules carries
// the deadline group, the room by the reset and the unweighted rate into
// the report, and lists the two deadline settings with their source.
// carol (shared) resets in 7h at week 87% (8% to 95 by the reset), derek
// in 13h at 78%; alice (own) days later: carol goes first.
func TestPoolDeadlineReport(t *testing.T) {
	m, _ := poolMachine(t, nil)
	m.f.WriteConfig(strings.Replace(poolConfig, poolClassicKeys, "", 1))
	a := m.app(t)
	now := agenttest.FixedNow()
	writeCache(t, a, 0, map[string]login.UsageEntry{
		"alice": okEntry(36, 52, now.Add(151*time.Hour)), "bob": okEntry(10, 99, now.Add(100*time.Hour)),
		"carol": okEntry(13, 87, now.Add(7*time.Hour)), "derek": okEntry(41, 78, now.Add(13*time.Hour)),
	})
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Selected == nil || rep.Selected.Name != "carol" || !strings.HasPrefix(rep.Selected.Reason, "resets soonest: 8% left before its weekly cap reaches 95% at the reset in 7h (1.14%/h") {
		t.Fatalf("selected %+v", rep.Selected)
	}
	byName := map[string]PoolCandidate{}
	for _, c := range rep.Candidates {
		byName[c.Name] = c
	}
	c := byName["carol"]
	if !c.Deadline || c.RoomAtResetPercent == nil || *c.RoomAtResetPercent != 8 || c.NeedsPercentPerHour == nil || *c.NeedsPercentPerHour != 1.14 {
		t.Errorf("carol deadline %v, room at reset %v, needs %v", c.Deadline, c.RoomAtResetPercent, c.NeedsPercentPerHour)
	}
	if !byName["derek"].Deadline || byName["alice"].Deadline || byName["bob"].Deadline {
		t.Errorf("deadline: derek %v, alice %v, bob %v", byName["derek"].Deadline, byName["alice"].Deadline, byName["bob"].Deadline)
	}
	settings := map[string]PoolSetting{}
	for _, s := range rep.Settings {
		settings[s.Key] = s
	}
	if s := settings["deadline_window"]; s.Value != "24h" || s.Source != SourceDefault {
		t.Errorf("deadline_window %+v", s)
	}
	if s := settings["deadline_min_room"]; s.Value != "2" || s.Source != SourceDefault {
		t.Errorf("deadline_min_room %+v", s)
	}
}
