package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/pool"
)

// pickNow is the clock of the login pick report tests.
var pickNow = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

// pickReport is a report with every candidate state: a forced pick, a
// ready member, a waiting one, a blocked one and one with a stale reset.
func pickReport(selected *pool.Selection) app.PoolReport {
	f := func(v float64) *float64 { return &v }
	at := func(d time.Duration) *time.Time { t := pickNow.Add(d); return &t }
	return app.PoolReport{
		Pool: "auto", Agent: "claude", Now: pickNow, Source: "live probe", BorrowWindow: "48h", Selected: selected,
		Candidates: []app.PoolCandidate{
			{Candidate: pool.Candidate{Name: "carol", Group: pool.GroupShared, State: pool.StateReady, Rank: 1, Short: f(0), Week: f(26), WeeklyResetsAt: at(38 * time.Hour), Room: f(44)}},
			{Candidate: pool.Candidate{Name: "derek", Group: pool.GroupShared, State: pool.StateWaiting, Week: f(15), WeeklyResetsAt: at(96 * time.Hour), Room: f(55), Notes: []string{"shared reset not within 48h while an own account is usable"}}},
			{Candidate: pool.Candidate{Name: "bob", Group: pool.GroupOwn, State: pool.StateBlocked, Reasons: []string{"status is limit-reached"}}},
			{Candidate: pool.Candidate{Name: "erin", Group: pool.GroupShared, State: pool.StateBlocked, Week: f(80), WeeklyResetsAt: at(-time.Hour), Reasons: []string{"week is 80% (cap 70%)"}, Notes: []string{"weekly reset time has passed; usage data is stale"}}},
		},
	}
}

// TestPoolReportText: the header names the pick and its rule; each
// member reads #N READY, WAIT or skip: with its figures and notes.
func TestPoolReportText(t *testing.T) {
	var b strings.Builder
	sel := &pool.Selection{Name: "carol", Group: pool.GroupShared, Reason: "shared account resets within 48h"}
	if err := writePoolReport(&b, pickReport(sel), render.NewStyle(false)); err != nil {
		t.Fatal(err)
	}
	want := `Use: carol (shared)
shared account resets within 48h
pool auto · claude · usage from live probe · borrow window 48h

Members (ready in pick order):
  carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
  derek [shared] WAIT (week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
  bob [own] skip: status is limit-reached
  erin [shared] skip: week is 80% (cap 70%) (week 80%, reset passed) (weekly reset time has passed; usage data is stale)
`
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestPoolReportForcedAndNone: a forced pick says so; no pick says no
// member is usable, in red when coloured.
func TestPoolReportForcedAndNone(t *testing.T) {
	var b strings.Builder
	sel := &pool.Selection{Name: "bob", Group: pool.GroupOwn, Forced: true, Reason: "forced by --force; all rules ignored"}
	if err := writePoolReport(&b, pickReport(sel), render.NewStyle(false)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "Use: bob (FORCED, ignoring rules)\nforced by --force; all rules ignored\n") {
		t.Errorf("forced:\n%s", b.String())
	}
	b.Reset()
	if err := writePoolReport(&b, pickReport(nil), render.NewStyle(true)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No member of pool auto is usable") || !strings.Contains(b.String(), "\x1b[") {
		t.Errorf("none:\n%q", b.String())
	}
}
