package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
)

// writePlanFile is the plan phase of new / resume / run --plan-file F
// (spec 12): resolve like --dry-run, write F for the shell wrapper and
// start nothing. Notes and warnings show now; agx launch repeats none.
func (c *cliCtx) writePlanFile(cmd *cobra.Command, o app.LaunchOptions) (render.Output, error) {
	out := render.Output{Command: commandPath(cmd)}
	p, v, err := c.App().WritePlan(cmd.Context(), o)
	if err != nil {
		return out, err
	}
	for _, n := range p.Notes {
		c.note(n)
	}
	out.Decisions, out.Warnings = p.Decisions, p.Warnings
	data, err := render.Encode(v)
	if err != nil {
		return out, err
	}
	out.Data = data
	// The human form prints nothing: stdout is the wrapper's, and the
	// plan is read back with agx plan show.
	out.Text = func(io.Writer) error { return nil }
	return out, nil
}

// planText is agx plan show's human form.
func planText(w io.Writer, v app.PlanView) error {
	var rows [][2]string
	for _, l := range [][2]string{
		{"mode", v.Mode}, {"dir", v.Dir}, {"agent", string(v.Agent)}, {"profile", v.Profile},
		{"home", v.Home}, {"login", v.Login}, {"binary", v.Binary}, {"resume", v.ResumeID}, {"command", v.Command.Line},
	} {
		if l[1] != "" {
			rows = append(rows, l)
		}
	}
	return keyValues(w, rows)
}
