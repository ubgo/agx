package login_test

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/login"
)

// historyAt is the history tests' clock.
var historyAt = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

// point is a reading of one week window at minute m, resetting at reset.
func point(m int, pct float64, reset time.Time) login.UsagePoint {
	return login.UsagePoint{At: historyAt.Add(time.Duration(m) * time.Minute), Windows: []login.PointWindow{{ID: "7d", Percent: pct, ResetsAt: &reset}}}
}

// TestUsageHistoryBounds: a point is kept only after the minimum gap or
// when a reset changed; older than the maximum age or past the maximum
// count is dropped, oldest first.
func TestUsageHistoryBounds(t *testing.T) {
	reset := historyAt.Add(48 * time.Hour)
	var h login.UsageHistory
	if !h.Add("alice", point(0, 10, reset)) {
		t.Fatal("the first point was dropped")
	}
	if h.Add("alice", point(5, 11, reset)) {
		t.Error("a point inside the minimum gap was kept")
	}
	if h.Add("alice", point(0, 10, reset)) {
		t.Error("a point at the same time was kept")
	}
	if !h.Add("alice", point(6, 0, reset.Add(login.HistoryMaxAge))) {
		t.Error("a point with a new reset was dropped")
	}
	if !h.Add("alice", point(16, 1, reset.Add(login.HistoryMaxAge))) {
		t.Error("a point after the minimum gap was dropped")
	}
	if n := len(h.Logins["alice"]); n != 3 {
		t.Fatalf("points %d, want 3", n)
	}
	late := int((login.HistoryMaxAge + time.Hour) / time.Minute)
	h.Add("alice", point(late, 2, reset.Add(login.HistoryMaxAge)))
	if pts := h.Logins["alice"]; len(pts) != 1 || pts[0].Windows[0].Percent != 2 {
		t.Errorf("after the maximum age: %+v", pts)
	}
	var big login.UsageHistory
	for i := range login.HistoryMaxPoints + 50 {
		big.Add("bob", point(i*int(login.HistoryMinGap/time.Minute), float64(i%100), reset))
	}
	if pts := big.Logins["bob"]; len(pts) > login.HistoryMaxPoints {
		t.Errorf("%d points, at most %d", len(pts), login.HistoryMaxPoints)
	}
}

// TestUsageHistoryRecordedWithCache: a cache update records each answered
// probe's windows and model in the history (0600); a failed probe adds
// nothing, nor does writing the same entry again (agx credential notes
// a detected kind that way).
func TestUsageHistoryRecordedWithCache(t *testing.T) {
	paths := config.Paths{Dir: t.TempDir()}
	reset := historyAt.Add(30 * time.Hour)
	ok := login.UsageEntry{Agent: "claude", ProbedAt: historyAt, Status: "ok", Model: "fable", Windows: []login.UsageCacheWindow{
		{Class: "short", ID: "5h", Percent: 12, ResetsAt: &reset}, {Class: "week", ID: "7d", Percent: 40, ResetsAt: &reset},
		{Class: "week", Scope: "oi", ID: "7d_oi", Percent: 61, ResetsAt: &reset}, {Class: "week", Scope: "zz", Percent: 3},
	}}
	failed := login.UsageEntry{Agent: "claude", ProbedAt: historyAt, Status: "rejected", Reason: "token rejected"}
	ctx := context.Background()
	if _, err := login.UpdateUsageCache(ctx, paths, map[string]login.UsageEntry{"alice": ok, "bob": failed}); err != nil {
		t.Fatal(err)
	}
	if _, err := login.UpdateUsageCache(ctx, paths, map[string]login.UsageEntry{"alice": ok}); err != nil {
		t.Fatal(err)
	}
	h, err := login.ReadUsageHistory(paths)
	if err != nil {
		t.Fatal(err)
	}
	pts := h.Logins["alice"]
	if len(pts) != 1 || pts[0].Model != "fable" || len(pts[0].Windows) != 4 || pts[0].Windows[2].ID != "7d_oi" || pts[0].Windows[3].ID != "week/zz" {
		t.Fatalf("alice %+v", pts)
	}
	if _, ok := h.Logins["bob"]; ok {
		t.Error("a failed probe was recorded")
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(paths.LoginUsageHistory()); st == nil || st.Mode().Perm() != config.TokensFileMode {
			t.Errorf("history mode %v", st)
		}
	}
	if err := os.WriteFile(paths.LoginUsageHistory(), []byte(`{"version": 9}`), config.TokensFileMode); err != nil {
		t.Fatal(err)
	}
	if h, err := login.ReadUsageHistory(paths); err == nil || len(h.Logins) != 0 {
		t.Errorf("foreign version: %+v %v", h, err)
	}
}
