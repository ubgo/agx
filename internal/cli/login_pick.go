package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/human"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
	"github.com/ubgo/agx/pool"
)

// loginPickFields are agx login pick's --field paths.
var loginPickFields = []string{"data.pool", "data.agent", "data.selected", "data.selected.name", "data.selected.group",
	"data.selected.forced", "data.selected.reason", "data.selected.warnings", "data.candidates", "data.now", "data.source",
	"data.shared_borrow_window", "data.short_unavailable_at_percent", "data.usage_max_age", "data.key_ttl"}

// loginPickCmd is agx login pick: the pick a pool login makes now, with
// every member's state and reasons. It prints no token.
func (c *cliCtx) loginPickCmd() *cobra.Command {
	var o app.PoolPickOptions
	cmd := c.newCmd("pick", "Show which member a pool login picks now, and why (probes members whose cached usage is old)", CostQuota+CostWrite,
		func(cmd *cobra.Command, _ []string) (render.Output, error) {
			rep, err := c.App().LoginPick(cmd.Context(), o)
			out := render.Output{Warnings: rep.Warnings, KeepData: rep.Pool != ""}
			data, eerr := render.Encode(rep)
			if eerr != nil {
				return out, eerr
			}
			out.Data = data
			st := c.style()
			out.Text = func(w io.Writer) error { return writePoolReport(w, rep, st) }
			return out, err
		})
	cmd.Long = "Read the usage of a pool login's members (a probe each, spending a few tokens, unless the login-usage cache is younger than the pool's usage_max_age), apply the pool's rules and print the member it picks, then every member: #N READY in pick order, WAIT (usable, held back by the borrow window) or skip with every reason. Exit 0 when a member is picked; exit 1 (error code auto-pick) when none is usable. It never prints a token: agx login token --id POOL prints the picked member's."
	cmd.Example = "  agx login pick --id auto\n  agx login pick --id auto --cached\n  agx login pick --id auto --force alice\n  agx login pick --id auto --json --field data.selected.name"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the pool login")
	f.BoolVar(&o.Cached, "cached", false, "use the login-usage cache only, of any age; probe nothing")
	f.DurationVar(&o.MaxAge, "max-age", 0, "reuse cached usage younger than this (default: the pool's usage_max_age)")
	f.StringVar(&o.Force, "force", "", "pick this login for this one run, over every rule and the pool's force")
	return fields(cmd, loginPickFields...)
}

// Report words.
const (
	pickReady   = "READY"
	pickWait    = "WAIT"
	pickSkip    = "skip:"
	pickForced  = "FORCED, ignoring rules"
	pickNoneFmt = "No member of pool %s is usable"
)

// writePoolReport prints the pick in agx v0.3's look: a bold header with
// the pick and its reason, then one line per member.
func writePoolReport(w io.Writer, rep app.PoolReport, st render.Style) error {
	ew := textout.New(w)
	if s := rep.Selected; s != nil {
		label := string(s.Group)
		if s.Forced {
			label = pickForced
		}
		ew.Line(st.Bold(fmt.Sprintf("Use: %s (%s)", s.Name, label)))
		ew.Line(s.Reason)
	} else {
		ew.Line(st.Paint(render.ToneBad, fmt.Sprintf(pickNoneFmt, rep.Pool)))
	}
	ew.Line(st.Paint(render.ToneDim, fmt.Sprintf("pool %s · %s · usage from %s · borrow window %s", rep.Pool, rep.Agent, rep.Source, rep.BorrowWindow)))
	ew.Line("")
	ew.Line(st.Bold("Members (ready in pick order):"))
	for _, c := range rep.Candidates {
		ew.Line(candidateLine(c, rep.Now, st))
	}
	return ew.Err()
}

// candidateLine is "  alice [own] #1 READY (5h 3%, week 29%, resets in
// 6 d, room 69%)", "  frank [shared] WAIT (…) (note)" or "  bob [own]
// skip: status is limit-reached; week is 100% (cap 98%)".
func candidateLine(c app.PoolCandidate, now time.Time, st render.Style) string {
	head := fmt.Sprintf("  %s [%s] ", st.Bold(c.Name), c.Group)
	figures := ""
	if f := candidateFigures(c.Candidate, now); f != "" {
		figures = " (" + f + ")"
	}
	notes := ""
	if len(c.Notes) > 0 {
		notes = " " + st.Paint(render.ToneDim, "("+strings.Join(c.Notes, "; ")+")")
	}
	switch c.State {
	case pool.StateReady:
		return head + st.Paint(render.ToneGood, fmt.Sprintf("#%d %s", c.Rank, pickReady)) + figures + notes
	case pool.StateWaiting:
		return head + st.Paint(render.ToneWarn, pickWait) + figures + notes
	}
	return head + st.Paint(render.ToneBad, pickSkip+" "+strings.Join(c.Reasons, "; ")) + figures + notes
}

// candidateFigures are a member's windows, reset and room.
func candidateFigures(c pool.Candidate, now time.Time) string {
	var parts []string
	if c.Short != nil {
		parts = append(parts, fmt.Sprintf("5h %g%%", *c.Short))
	}
	if c.Week != nil {
		parts = append(parts, fmt.Sprintf("week %g%%", *c.Week))
	}
	if c.WeeklyResetsAt != nil {
		if until := c.WeeklyResetsAt.Sub(now); until > 0 {
			parts = append(parts, "resets in "+human.Until(until))
		} else {
			parts = append(parts, "reset passed")
		}
	}
	if c.Room != nil {
		parts = append(parts, fmt.Sprintf("room %g%%", *c.Room))
	}
	return strings.Join(parts, ", ")
}
