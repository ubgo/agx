package app

import (
	"context"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/run"
)

// TestEdge48And49Liveness drives runLiveness through agentio's fake
// process table: a pid reused by a process started later (edge 48) is
// ended; a reused pid whose env lacks the run's AGX_RUN_ID is ended; a
// pid that runs with neither a start time nor a readable env is unknown
// and kept (edge 49); a matching env is alive.
func TestEdge48And49Liveness(t *testing.T) {
	const id = "20261114-101500-k3x9"
	started := time.Date(2026, 11, 14, 10, 15, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{
		List: []agentio.Process{
			{PID: 10, Start: started.Add(time.Hour)},
			{PID: 11},
			{PID: 12},
			{PID: 13},
		},
		Envs: map[int][]string{11: {"AGX_RUN_ID=other"}, 13: {"AGX_RUN_ID=" + id}},
	}
	h := agenttest.NewHost(t, agenttest.WithProcesses(procs))
	a := New(Deps{Host: h, Lookup: func(string) (string, bool) { return "", false }})
	cases := []struct {
		pid   int
		start time.Time
		want  agentio.Liveness
	}{
		{10, started, agentio.LivenessEnded},
		{11, time.Time{}, agentio.LivenessEnded},
		{12, time.Time{}, agentio.LivenessUnknown},
		{13, time.Time{}, agentio.LivenessAlive},
		{99, started, agentio.LivenessEnded},
	}
	for _, c := range cases {
		f := run.Folder{ID: id, Dir: t.TempDir(), Record: run.Record{ID: id, PID: c.pid, StartedAt: c.start}}
		if got, reason := a.runLiveness(context.Background(), f); got != c.want {
			t.Errorf("pid %d: %s (%s), want %s", c.pid, got, reason, c.want)
		}
	}
}
