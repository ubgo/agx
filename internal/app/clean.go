package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
)

// CleanAction says what agx clean does with one run folder.
type CleanAction string

// Clean actions.
const (
	// CleanDelete is an ended run that --yes would delete (the plan).
	CleanDelete CleanAction = "delete"
	// CleanDeleted is an ended run deleted by this invocation.
	CleanDeleted CleanAction = "deleted"
	// CleanKeep is a running run, or one whose liveness agx cannot check
	// (edge 49: kept and listed, never guessed).
	CleanKeep CleanAction = "keep"
)

// neverStartedAge is how old a run folder without run.jsonc must be
// before clean treats it as a launch that never started. Prepare creates
// the folder and Commit writes run.jsonc a moment later; a younger folder
// may be a launch in progress, so it is kept as unknown.
const neverStartedAge = time.Minute

// Liveness reasons shown beside each run.
const (
	reasonEnded        = "process ended"
	reasonAlive        = "running"
	reasonNeverStarted = "never started (no run.jsonc)"
	reasonUnreadable   = "process info unreadable"
)

// CleanOptions are agx clean's flags.
type CleanOptions struct {
	// Yes deletes the ended runs; without it clean only shows the plan.
	Yes bool
	// DryRun shows the plan (the same as no --yes, stated explicitly).
	DryRun bool
}

// CleanRun is one run folder in agx clean's result.
type CleanRun struct {
	ID     string `json:"id"`
	Dir    string `json:"dir"`
	Agent  string `json:"agent"`
	PID    int    `json:"pid"`
	Status string `json:"liveness"`
	// Reason says how the liveness was decided (or why it could not be).
	Reason string      `json:"reason"`
	Action CleanAction `json:"action"`
}

// CleanResult is agx clean's data.
type CleanResult struct {
	Runs []CleanRun `json:"runs"`
	// Applied is true when folders were deleted (--yes).
	Applied bool `json:"applied"`
	Deleted int  `json:"deleted"`
	Kept    int  `json:"kept"`
}

// Clean lists every run folder with its liveness and deletes those whose
// agent has ended, only with --yes (spec 11, edges 47 to 49). A run whose
// liveness is unknown is kept and listed with the reason.
func (a *App) Clean(ctx context.Context, o CleanOptions) (CleanResult, error) {
	res := CleanResult{Runs: []CleanRun{}}
	if o.Yes && o.DryRun {
		return res, apperr.Conflict("--yes and --dry-run: one deletes, one only shows; pick one")
	}
	folders, err := run.List(a.paths.Runs)
	if err != nil {
		return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read "+a.paths.Runs+": "+err.Error())
	}
	for _, f := range folders {
		if err := ctxErr(ctx); err != nil {
			return res, err
		}
		l, reason := a.runLiveness(ctx, f)
		r := CleanRun{ID: f.ID, Dir: f.Dir, Agent: f.Record.Agent, PID: f.Record.PID, Status: l.String(), Reason: reason, Action: CleanKeep}
		if l == agentio.LivenessEnded {
			r.Action = CleanDelete
			if o.Yes {
				if err := run.Remove(a.paths.Runs, f.ID); err != nil {
					return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "delete "+f.Dir+": "+err.Error())
				}
				r.Action = CleanDeleted
				res.Deleted++
			}
		}
		if r.Action == CleanKeep {
			res.Kept++
		}
		res.Runs = append(res.Runs, r)
	}
	res.Applied = o.Yes
	return res, nil
}

// runLiveness decides whether a run's agent still runs (spec 11) through
// agentio's Host.ProcessAlive: the pid, the start time recorded in
// run.jsonc and the AGX_RUN_ID entry of the process's start environment
// (pid reuse, edge 48). A folder without run.jsonc is a launch that never
// reached Commit; it counts as ended only once older than
// neverStartedAge.
func (a *App) runLiveness(ctx context.Context, f run.Folder) (agentio.Liveness, string) {
	if f.RecordErr != nil || f.Record.PID == 0 {
		if f.RecordErr == nil || errors.Is(f.RecordErr, fs.ErrNotExist) {
			if st, err := os.Stat(f.Dir); err == nil && a.Now().Sub(st.ModTime()) > neverStartedAge {
				return agentio.LivenessEnded, reasonNeverStarted
			}
		}
		// The reason says which of the three cases this is: one message for
		// all of them claimed a run.jsonc that was read fine was unreadable.
		switch {
		case f.RecordErr == nil:
			return agentio.LivenessUnknown, reasonUnreadable + ": " + run.RecordFile + " records no process id"
		case errors.Is(f.RecordErr, fs.ErrNotExist):
			return agentio.LivenessUnknown, reasonUnreadable + ": no " + run.RecordFile + " yet"
		}
		return agentio.LivenessUnknown, reasonUnreadable + ": " + apperr.Text(f.RecordErr)
	}
	l, err := a.d.Host.ProcessAlive(ctx, f.Record.PID, f.Record.StartedAt, run.EnvRunID, f.ID)
	switch {
	case l == agentio.LivenessEnded:
		return l, reasonEnded
	case l == agentio.LivenessAlive:
		return l, reasonAlive
	case err != nil:
		return agentio.LivenessUnknown, reasonUnreadable + ": " + apperr.Text(err)
	}
	return agentio.LivenessUnknown, reasonUnreadable
}
