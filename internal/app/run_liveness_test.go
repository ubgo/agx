package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
)

// livenessNow is the clock of the liveness tests.
var livenessNow = time.Date(2026, 11, 21, 19, 13, 0, 0, time.UTC)

// livenessApp is an App on flagConfig (pool auto: own alice and bob,
// shared carol) with the given process table and the clock at
// livenessNow.
func livenessApp(t *testing.T, procs *agenttest.FakeProcesses) *App {
	t.Helper()
	h := agenttest.NewHost(t, agenttest.WithProcesses(procs), agenttest.WithNow(func() time.Time { return livenessNow }))
	agenttest.WriteFile(t, h.UserHome, ".agx/config.jsonc", flagConfig)
	lookup := func(name string) (string, bool) {
		if name == "AGX_CONFIG_DIR" {
			return filepath.Join(h.UserHome, ".agx"), true
		}
		return "", false
	}
	return New(Deps{Host: h, Lookup: lookup, Cwd: h.UserHome})
}

// writeRun writes run folder id with its record (pid, start) and a live
// state on pool auto using member, which asked for a key at inUseAt.
func writeRun(t *testing.T, a *App, id string, pid int, start time.Time, member string, inUseAt time.Time) {
	t.Helper()
	dir := run.Dir(a.paths.Runs, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	rec, _ := json.Marshal(run.Record{ID: id, PID: pid, StartedAt: start, Agent: "claude", Login: "auto", LiveLogin: true})
	st, _ := json.Marshal(live.State{RunID: id, Agent: "claude", StartLogin: "auto", InUse: member, InUseAt: inUseAt})
	for name, b := range map[string][]byte{run.RecordFile: rec, run.LiveFile: st} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPoolLoadSkipsEndedRuns: a run that asked for a key a moment ago
// but whose agent has ended (a short headless run) is not a session on
// its member; the live ones are, and the process table is read once for
// all the ended ones.
func TestPoolLoadSkipsEndedRuns(t *testing.T) {
	start := livenessNow.Add(-time.Hour)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: 100, Start: start}, {PID: 101, Start: start}, {PID: 300, Start: livenessNow}}}
	a := livenessApp(t, procs)
	recent := livenessNow.Add(-10 * time.Second)
	writeRun(t, a, "20261121-120000-aaaa", 100, start, "alice", recent)
	writeRun(t, a, "20261121-120001-bbbb", 101, start, "alice", recent)
	for i := range 20 {
		// Ended: the pid is gone, or now belongs to a later process.
		pid := 200 + i
		if i == 0 {
			pid = 300
		}
		writeRun(t, a, fmt.Sprintf("20261121-1201%02d-c%03d", i, i), pid, start, "bob", recent)
	}
	cfg, err := a.Config()
	if err != nil {
		t.Fatal(err)
	}
	load, _, _ := a.poolLoad(context.Background(), "auto", cfg.Config.Logins["auto"], "", livenessNow, func(w string) { t.Error(w) })
	if load["alice"] != 2 || load["bob"] != 0 {
		t.Errorf("load %v, want alice 2, bob 0", load)
	}
	// One table read for the batch, one more per live run's full check.
	if procs.TableReads > 3 {
		t.Errorf("%d process-table reads for 22 runs", procs.TableReads)
	}
}

// TestLiveListOneTableRead: agx live list decides every ended run with
// one process-table read, not one per run folder.
func TestLiveListOneTableRead(t *testing.T) {
	start := livenessNow.Add(-time.Hour)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: 100, Start: start}}}
	a := livenessApp(t, procs)
	writeRun(t, a, "20261121-120000-aaaa", 100, start, "alice", livenessNow)
	for i := range 50 {
		writeRun(t, a, fmt.Sprintf("20261121-1100%02d-e%03d", i, i), 500+i, start, "bob", start)
	}
	runs, err := a.LiveList(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "20261121-120000-aaaa" || runs[0].InUse != "alice" {
		t.Errorf("live runs %+v", runs)
	}
	if procs.TableReads > 2 {
		t.Errorf("%d process-table reads for 51 runs", procs.TableReads)
	}
	// A table that cannot be read leaves every run to the full check:
	// none is called ended without evidence.
	procs.Err = os.ErrPermission
	runs, err = a.LiveList(context.Background(), "")
	if err != nil || len(runs) != 51 {
		t.Errorf("unreadable table: %d runs, %v", len(runs), err)
	}
}
