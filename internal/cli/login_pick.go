package cli

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
	"github.com/ubgo/agx/pool"
)

// loginPickFields are agx login pick's --field paths.
var loginPickFields = []string{"data.pool", "data.agent", "data.selected", "data.selected.name", "data.selected.group",
	"data.selected.forced", "data.selected.forced_by", "data.selected.picked_by", "data.selected.reason", "data.selected.warnings",
	"data.selected.weekly_resets_at", "data.selected.room_percent", "data.source", "data.now", "data.warnings",
	"data.short_unavailable_at_percent", "data.shared_borrow_window_hours", "data.candidates",
	"data.candidates.*.name", "data.candidates.*.group", "data.candidates.*.state", "data.candidates.*.rank",
	"data.candidates.*.eligible", "data.candidates.*.enabled", "data.candidates.*.status", "data.candidates.*.account",
	"data.candidates.*.short", "data.candidates.*.week", "data.candidates.*.weekly_resets_at", "data.candidates.*.hours_to_reset",
	"data.candidates.*.room_percent", "data.candidates.*.caps", "data.candidates.*.reasons", "data.candidates.*.notes",
	"data.candidates.*.source", "data.candidates.*.age_seconds"}

// loginPickCmd is agx login pick: the pick a pool login makes now, with
// every member's state and reasons, in the stand-alone selector's report
// (line for line). It prints no token.
func (c *cliCtx) loginPickCmd() *cobra.Command {
	var (
		o   app.PoolPickOptions
		to  app.TimeOptions
		now string
	)
	cmd := c.newCmd("pick", "Show which member a pool login picks now, and why (probes members whose cached usage is old)", CostQuota+CostWrite,
		func(cmd *cobra.Command, _ []string) (render.Output, error) {
			o.MaxAgeSet = cmd.Flags().Changed("max-age")
			if now != "" {
				t, err := time.Parse(time.RFC3339, now)
				if err != nil {
					return render.Output{}, apperr.Usage(fmt.Sprintf("--now %q is not an RFC 3339 time (2026-11-19T10:00:00Z)", now))
				}
				o.Now = t
			}
			to.DateFormat = c.g.DateFormat
			rep, err := c.App().LoginPick(cmd.Context(), o, to)
			out := render.Output{KeepData: rep.Pool != ""}
			data, eerr := render.Encode(rep)
			if eerr != nil {
				return out, eerr
			}
			out.Data = data
			st := c.style()
			out.Text = func(w io.Writer) error { return writePoolReport(w, rep, st) }
			return out, err
		})
	cmd.Long = "Read the usage of a pool login's members (a probe each, spending a few tokens, unless the login-usage cache is younger than the pool's usage_max_age), decide as a key request would (--pool-force, the pool's force, its picker command, else the built-in rules) and print the member it picks, why, where the usage came from, then every member: #N READY in pick order, WAIT (usable, held back by the borrow window) or skip with every reason. Reset times follow --time-format, --duration-format, --tz and --date-format, as in agx usage. Exit 0 when a member is picked; exit 1 (error code auto-pick) when none is usable. It never prints a token: agx login token --id POOL prints the picked member's."
	cmd.Example = "  agx login pick --id auto\n  agx login pick --id auto --cached\n  agx login pick --id auto --pool-force alice\n  agx login pick --id auto --time-format both\n  agx login pick --id auto --json --field data.selected.name"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the pool login")
	f.BoolVar(&o.Cached, "cached", false, "use the login-usage cache only, of any age; probe nothing")
	f.DurationVar(&o.MaxAge, "max-age", 0, "reuse cached usage younger than this; 0 probes every member (default: the pool's usage_max_age)")
	f.StringVar(&o.Force, "pool-force", "", "pick this member for this one run, over the pool's rules, force and picker")
	f.StringVar(&now, "now", "", "pretend it is this RFC 3339 time (to read an old cache)")
	_ = f.MarkHidden("now") // the flag exists, so marking it cannot fail
	timeFlags(cmd, &to)
	return fields(cmd, loginPickFields...)
}

// Report words: the stand-alone selector's, kept line for line.
const (
	pickReady     = "READY"
	pickWait      = "WAIT"
	pickSkip      = "skip: "
	pickForced    = "FORCED, ignoring rules"
	pickNone      = "No configured account is currently usable."
	pickHeader    = "Candidates (ready accounts in pick order):"
	pickNoWindows = "no windows"
	pickUnknown   = "reset unknown"
	pickPassed    = "reset passed"
)

// writePoolReport prints the pick: "Use: NAME (GROUP)" (or FORCED with a
// warning line per ignored rule), the reason, the warnings, the source, a
// blank line, then one line per member.
func writePoolReport(w io.Writer, rep app.PoolReport, st render.Style) error {
	ew := textout.New(w)
	ew.Line(st.Paint(render.ToneDim, fmt.Sprintf("Pool: %s (%s)", rep.Pool, rep.Agent)))
	if s := rep.Selected; s != nil {
		label := string(s.Group)
		if s.Forced {
			label = pickForced
		}
		ew.Line(st.Bold(fmt.Sprintf("Use: %s (%s)", s.Name, label)))
		for _, warn := range s.Warnings {
			ew.Line(st.Paint(render.ToneWarn, "  warning: "+warn))
		}
		ew.Line(s.Reason)
	} else {
		ew.Line(st.Paint(render.ToneBad, pickNone))
	}
	for _, warn := range rep.Warnings {
		ew.Line(st.Paint(render.ToneWarn, "warning: "+warn))
	}
	ew.Line(st.Paint(render.ToneDim, "Source: "+rep.Source))
	ew.Line("")
	ew.Line(st.Bold(pickHeader))
	v := timeViewOf(rep.View, st)
	for _, c := range rep.Candidates {
		ew.Line(candidateLine(c, v, st))
	}
	return ew.Err()
}

// timeViewOf is the usage view of a time view, so a reset reads the same
// in agx usage and agx login pick.
func timeViewOf(tv app.TimeView, st render.Style) usageView {
	loc := tv.Loc
	if loc == nil {
		loc = time.Local
	}
	date := tv.DateFormat
	if date.IsZero() {
		date = datefmt.Preset(datefmt.PresetLong)
	}
	return usageView{now: tv.Now, loc: loc, format: cmp.Or(tv.TimeFormat, config.TimeRelative), date: date, duration: durationMode(tv.DurationFormat), st: st}
}

// candidateLine is "- alice [own] #1 READY (5h 3%, week 29%, resets in
// 6 d 0 hr, room 69%)", "- frank [shared] WAIT (…) (note)" or "- bob
// [own] skip: reason; reason (…)".
func candidateLine(c app.PoolCandidate, v usageView, st render.Style) string {
	var label string
	switch c.State {
	case pool.StateReady:
		rank := 0
		if c.Rank != nil {
			rank = *c.Rank
		}
		label = st.Paint(render.ToneGood, fmt.Sprintf("#%d %s", rank, pickReady))
	case pool.StateWaiting:
		label = st.Paint(render.ToneWarn, pickWait)
	default:
		label = st.Paint(render.ToneBad, pickSkip+strings.Join(c.Reasons, "; "))
	}
	var usage []string
	if c.Short != nil {
		usage = append(usage, fmt.Sprintf("5h %.0f%%", math.Round(*c.Short)))
	}
	if c.Week != nil {
		usage = append(usage, fmt.Sprintf("week %.0f%%", math.Round(*c.Week)))
	}
	figures := strings.Join(usage, ", ")
	if figures == "" {
		figures = pickNoWindows
	}
	figures += ", " + pickReset(c, v)
	if c.RoomPercent != nil {
		figures += ", room " + strconv.FormatFloat(*c.RoomPercent, 'f', -1, 64) + "%"
	}
	notes := ""
	if len(c.Notes) > 0 {
		notes = st.Paint(render.ToneDim, " ("+strings.Join(c.Notes, "; ")+")")
	}
	return fmt.Sprintf("- %s [%s] %s (%s)%s", c.Name, c.Group, label, figures, notes)
}

// pickReset is the weekly reset as agx usage writes it ("resets in 1 d
// 14 hr", "resets Fri 13:30"), "reset passed" for stale data, "reset
// unknown" without one.
func pickReset(c app.PoolCandidate, v usageView) string {
	switch {
	case c.WeeklyResetsAt == nil:
		return pickUnknown
	case !c.WeeklyResetsAt.After(v.now):
		return pickPassed
	}
	return resetPhrase(*c.WeeklyResetsAt, v)
}
