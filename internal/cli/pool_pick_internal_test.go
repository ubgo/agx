package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/pool"
)

// pickNow is the clock of the pool pick report unit tests.
var pickNow = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

// pickReport is a report with every candidate state: ready, waiting,
// blocked, a stale reset and no windows at all.
func pickReport(selected *app.PoolSelection, warnings ...string) app.PoolReport {
	f := func(v float64) *float64 { return &v }
	at := func(d time.Duration) *time.Time { t := pickNow.Add(d); return &t }
	one := 1
	return app.PoolReport{
		Pool: "auto", Agent: "claude", Now: pickNow, Source: "live agx result", Selected: selected, Warnings: warnings,
		View: app.TimeView{Now: pickNow, Loc: time.UTC},
		Candidates: []app.PoolCandidate{
			{Name: "carol", Group: pool.GroupShared, State: pool.StateReady, Rank: &one, Short: f(0.4), Week: f(26), WeeklyResetsAt: at(38 * time.Hour), RoomPercent: f(44)},
			{Name: "derek", Group: pool.GroupShared, State: pool.StateWaiting, Week: f(15), WeeklyResetsAt: at(96 * time.Hour), RoomPercent: f(55), Notes: []string{"shared reset not within 48h while an own account is usable"}},
			{Name: "bob", Group: pool.GroupOwn, State: pool.StateBlocked, Reasons: []string{"status is limit-reached"}},
			{Name: "erin", Group: pool.GroupShared, State: pool.StateBlocked, Week: f(80), WeeklyResetsAt: at(-time.Hour), RoomPercent: f(0), Reasons: []string{"week is 80% (cap 70%)"}, Notes: []string{"weekly reset time has passed; usage data is stale"}},
		},
	}
}

// TestPoolReportText: the selector's lines: Use, reason, warnings,
// Source, blank, header, one "- " line per member with "no windows",
// "reset unknown" and "reset passed" where they apply.
func TestPoolReportText(t *testing.T) {
	var b strings.Builder
	sel := &app.PoolSelection{Name: "carol", Group: pool.GroupShared, Reason: "shared account resets within 48h"}
	if err := writePoolReport(&b, pickReport(sel, "probe of bob failed (network); using its cached usage from 2m0s ago"), render.NewStyle(false)); err != nil {
		t.Fatal(err)
	}
	want := `Pool: auto (claude)
Use: carol (shared)
shared account resets within 48h
warning: probe of bob failed (network); using its cached usage from 2m0s ago
Source: live agx result

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
- derek [shared] WAIT (week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached (no windows, reset unknown)
- erin [shared] skip: week is 80% (cap 70%) (week 80%, reset passed, room 0%) (weekly reset time has passed; usage data is stale)
`
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestPoolReportForcedAndNone: a forced pick prints FORCED and one
// indented warning per ignored rule; no pick prints the selector's line,
// in red when coloured.
func TestPoolReportForcedAndNone(t *testing.T) {
	var b strings.Builder
	sel := &app.PoolSelection{Name: "bob", Group: pool.GroupOwn, Forced: true, Reason: "forced by --force; all rules ignored", Warnings: []string{"status is limit-reached"}}
	if err := writePoolReport(&b, pickReport(sel), render.NewStyle(false)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "Pool: auto (claude)\nUse: bob (FORCED, ignoring rules)\n  warning: status is limit-reached\nforced by --force; all rules ignored\n") {
		t.Errorf("forced:\n%s", b.String())
	}
	b.Reset()
	if err := writePoolReport(&b, pickReport(nil), render.NewStyle(true)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No configured account is currently usable.") || !strings.Contains(b.String(), "\x1b[") {
		t.Errorf("none:\n%q", b.String())
	}
}

// TestFirstWordAfter: the word after a group skips flags; no group or
// no word is "".
func TestFirstWordAfter(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"login", "--json", "pick", "--id", "auto"}, "pick"},
		{[]string{"--color", "never", "login", "pick"}, "pick"},
		{[]string{"login", "--json"}, ""},
		{[]string{"usage"}, ""},
	} {
		if got := firstWordAfter(c.args, "login"); got != c.want {
			t.Errorf("%v: %q, want %q", c.args, got, c.want)
		}
	}
}
