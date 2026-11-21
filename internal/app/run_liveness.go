package app

import (
	"context"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/run"
)

// runLive is one run folder's liveness and its reason (runLiveness).
type runLive struct {
	liveness agentio.Liveness
	reason   string
}

// runsLiveness decides the liveness of many run folders with one read of
// the process table: a run whose process is gone (or whose pid now
// belongs to a later process) is ended at once, and only the rest get
// runLiveness's full check (a start environment read). Checking each
// folder on its own read the whole table once per folder: with a few
// thousand run folders on a busy machine agx live list took minutes.
// When the table cannot be read every folder gets the full check.
func (a *App) runsLiveness(ctx context.Context, folders []run.Folder) map[string]runLive {
	out := make(map[string]runLive, len(folders))
	var refs []agentio.ProcessRef
	var idx []int
	for i, f := range folders {
		if f.RecordErr == nil && f.Record.PID != 0 {
			refs = append(refs, agentio.ProcessRef{PID: f.Record.PID, Start: f.Record.StartedAt})
			idx = append(idx, i)
		}
	}
	if len(refs) > 0 {
		if lives, err := a.d.Host.ProcessesAlive(ctx, refs); err == nil {
			for j, l := range lives {
				if l == agentio.LivenessEnded {
					out[folders[idx[j]].ID] = runLive{agentio.LivenessEnded, reasonEnded}
				}
			}
		}
	}
	for _, f := range folders {
		if _, done := out[f.ID]; done {
			continue
		}
		l, reason := a.runLiveness(ctx, f)
		out[f.ID] = runLive{l, reason}
	}
	return out
}
