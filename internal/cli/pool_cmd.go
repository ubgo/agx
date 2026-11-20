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

// poolManageNote ends every pool command's help: a pool is a login kind,
// so it is made and changed with the login commands and used by a launch.
const poolManageNote = "A pool is a login of kind pool: create it with agx login add --kind pool, change it with agx login edit --pool-…, and run on it with agx run --login POOL."

// poolPickFields are agx pool pick's --field paths.
var poolPickFields = []string{"data.pool", "data.agent", "data.selected", "data.selected.name", "data.selected.group",
	"data.selected.forced", "data.selected.forced_by", "data.selected.picked_by", "data.selected.reason", "data.selected.warnings",
	"data.selected.weekly_resets_at", "data.selected.room_percent", "data.source", "data.now", "data.warnings",
	"data.short_unavailable_at_percent", "data.shared_borrow_window_hours", "data.order", "data.settings", "data.settings.*.key",
	"data.settings.*.value", "data.settings.*.source", "data.selected.sticky", "data.selected.sessions", "data.candidates",
	"data.candidates.*.name", "data.candidates.*.group", "data.candidates.*.state", "data.candidates.*.rank",
	"data.candidates.*.eligible", "data.candidates.*.enabled", "data.candidates.*.status", "data.candidates.*.account",
	"data.candidates.*.short", "data.candidates.*.week", "data.candidates.*.weekly_resets_at", "data.candidates.*.hours_to_reset",
	"data.candidates.*.room_percent", "data.candidates.*.caps", "data.candidates.*.caps_source", "data.candidates.*.week_cap_configured",
	"data.candidates.*.urgency_percent_per_hour", "data.candidates.*.sessions", "data.candidates.*.scoped", "data.probe_models", "data.probe_models_source", "data.candidates.*.reasons", "data.candidates.*.notes",
	"data.candidates.*.source", "data.candidates.*.age_seconds"}

// Fields of agx pool list and agx pool show.
var (
	poolListFields = []string{"data.*.name", "data.*.agents", "data.*.own", "data.*.shared", "data.*.force", "data.*.picker"}
	poolShowFields = []string{"data.name", "data.agents", "data.own", "data.shared", "data.force", "data.picker",
		"data.members", "data.members.*.name", "data.members.*.group", "data.members.*.enabled", "data.members.*.enabled_source",
		"data.members.*.short", "data.members.*.short_source", "data.members.*.week", "data.members.*.week_source",
		"data.members.*.week_relax_max", "data.members.*.week_relax_max_source", "data.members.*.capacity", "data.members.*.capacity_source",
		"data.settings", "data.settings.*.key", "data.settings.*.value", "data.settings.*.source"}
)

// poolCmd is agx pool: the commands that read a login pool. Picking
// applies to pools only, so it lives here rather than under agx login,
// whose subcommands treat a login as a noun.
func (c *cliCtx) poolCmd() *cobra.Command {
	return group("pool", "Login pools: list them, show their settings, see which member they pick",
		c.poolListCmd(), c.poolShowCmd(), c.poolPickCmd(), c.poolPresetCmd(),
	)
}

// poolListCmd is agx pool list.
func (c *cliCtx) poolListCmd() *cobra.Command {
	cmd := c.newCmd("list", "Every pool login: agents, own and shared members, force and picker", CostRead, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		rows, err := c.App().PoolList()
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		return render.Output{Data: data, Table: []render.Column{
			{Header: "NAME", Key: "name"}, {Header: "AGENTS", Key: "agents"}, {Header: "OWN", Key: "own"},
			{Header: "SHARED", Key: "shared"}, {Header: "FORCE", Key: "force"}, {Header: "PICKER", Key: "picker"},
		}, Empty: "no pool logins — add one with `agx login add --name NAME --agent claude --kind pool --pool-own LOGIN` (see `agx login add --help`)"}, err
	})
	cmd.Long = "List every login of kind pool from config, in the file's order: the agents it serves, its own and shared members, the member it always picks (force) and its picker command. It reads config only: no probe, no cache. " + poolManageNote
	cmd.Example = "  agx pool list\n  agx pool list --json"
	return fields(cmd, poolListFields...)
}

// poolShowCmd is agx pool show.
func (c *cliCtx) poolShowCmd() *cobra.Command {
	var o app.PoolShowOptions
	cmd := c.newCmd("show", "One pool's settings, each with where it comes from (default, pool, group or member)", CostRead, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		info, err := c.App().PoolShow(o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(info)
		st := c.style()
		return render.Output{Data: data, Text: func(w io.Writer) error { return poolShowText(w, info, st) }}, err
	})
	cmd.Long = "Show one pool login's full settings: its agents, own and shared members with each member's caps, weekly relax max, capacity and enabled state, then force, the rule values (order, week_relax_window, week_relax_max, max_sessions, sticky, sticky_leave_ratio, the shared borrow window, the 5-hour cutoff), the group defaults, usage_max_age, key_ttl, the picker and the log. Each value says where it comes from: default (agx's default for an omitted key), pool (a pool-wide key, defaults.* included), group (own_caps or shared_caps) or member (the member's accounts entry). It reads config only: no probe, no cache. agx pool preset writes a named set of rule values. " + poolManageNote
	cmd.Example = "  agx pool show --id auto\n  agx pool show --id auto --json --field data.settings"
	cmd.Flags().StringVar(&o.ID, "id", "", "the pool login")
	return fields(cmd, poolShowFields...)
}

// poolPresetCmd is agx pool preset: write a named set of rule values
// into a pool's config.
func (c *cliCtx) poolPresetCmd() *cobra.Command {
	var o app.PoolPresetOptions
	cmd := c.newCmd("preset", "Write a named set of rule values (recommended or classic) into a pool's config", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		ch, err := c.App().PoolPreset(cmd.Context(), o)
		return loginChangeOutput("change", "changed", ch, err)
	})
	var about []string
	for _, p := range pool.Presets() {
		about = append(about, p.Name+" ("+p.About+")")
	}
	cmd.Long = "Write a preset's rule values into a pool login's block in config.jsonc: order, week_relax_window, defaults.week_relax_max, max_sessions, sticky, sticky_leave_ratio and shared_borrow_window. A preset is a CLI shortcut, not a mode: afterwards each value is an ordinary key, shown by agx pool show, that you can change by hand or with agx config set. Caps are not touched (they are your team's agreement). Comments in the file are kept. Presets: " + strings.Join(about, "; ") + ". --dry-run shows each change and writes nothing. " + poolManageNote
	cmd.Example = "  agx pool preset --name recommended --id auto --dry-run\n  agx pool preset --name classic --id auto\n  agx pool show --id auto"
	f := cmd.Flags()
	f.StringVar(&o.Name, "name", "", "the preset: "+strings.Join(pool.PresetNames(), " or "))
	f.StringVar(&o.ID, "id", "", "the pool login")
	f.BoolVar(&o.DryRun, "dry-run", false, "show the changes; write nothing")
	return cmd
}

// poolShowText is the pool's labelled lines, one line per member with
// its caps and enabled state, then one line per setting. Each value ends
// with its source, dimmed.
func poolShowText(w io.Writer, info app.PoolInfo, st render.Style) error {
	dash := func(v string) string { return cmp.Or(v, render.Dash) }
	src := func(s string) string { return st.Paint(render.ToneDim, "("+s+")") }
	if err := keyValues(w, [][2]string{
		{"name", info.Name},
		{"agents", dash(strings.Join(info.Agents, ", "))},
		{"own", dash(strings.Join(info.Own, ", "))},
		{"shared", dash(strings.Join(info.Shared, ", "))},
	}); err != nil {
		return err
	}
	ew := textout.New(w)
	ew.Line("")
	ew.Line(st.Bold(poolMembersHeader))
	if len(info.Members) == 0 {
		ew.Line(render.Dash)
	}
	for _, m := range info.Members {
		state := poolEnabled
		if !m.Enabled {
			state = st.Paint(render.ToneWarn, poolDisabled)
		}
		ew.Line(fmt.Sprintf("- %s [%s] %s %s, short %s %s, week %s %s, week relax max %s %s, capacity %s %s", m.Name, m.Group, state, src(m.EnabledSource),
			numText(m.Short), src(m.ShortSource), numText(m.Week), src(m.WeekSource),
			numText(m.WeekRelaxMax), src(m.WeekRelaxMaxSource), numText(m.Capacity), src(m.CapacitySource)))
	}
	ew.Line("")
	ew.Line(st.Bold(poolSettingsHeader))
	if ew.Err() != nil {
		return ew.Err()
	}
	rows := make([][2]string, 0, len(info.Settings))
	for _, s := range info.Settings {
		rows = append(rows, [2]string{s.Key, s.Value + " " + src(s.Source)})
	}
	return keyValues(w, rows)
}

// numText writes a number with at most one decimal, the shortest way
// (95, 97.5, 91.9 for a relaxed 91.875).
func numText(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) }

// agx pool show's headings and member states.
const (
	poolMembersHeader  = "Members (caps in percent):"
	poolSettingsHeader = "Settings:"
	poolEnabled        = "enabled"
	poolDisabled       = "disabled"
)

// poolPickCmd is agx pool pick: the pick a pool login makes now, with
// every member's state and reasons, in the stand-alone selector's report
// (line for line). It prints no token.
func (c *cliCtx) poolPickCmd() *cobra.Command {
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
			rep, err := c.App().PoolPick(cmd.Context(), o, to)
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
	cmd.Long = "Read the usage of a pool login's members (a probe each, spending a few tokens, unless the login-usage cache is younger than the pool's usage_max_age), decide as a key request would (--pool-force, the pool's force, its picker command, else the built-in rules) and print the member it picks, why, where the usage came from, the probe model (--model, else the pool's probe_models, else the default profile's model: a per-model weekly window such as Fable's shows only for its model) and every rule value with its source, then every member: #N READY in pick order, WAIT (usable, held back by the borrow window) or skip with every reason. Reset times follow --time-format, --duration-format, --tz and --date-format, as in agx usage. Exit 0 when a member is picked; exit 1 (error code auto-pick) when none is usable. It never prints a token: agx login token --id POOL prints the picked member's. " + poolManageNote
	cmd.Example = "  agx pool pick --id auto\n  agx pool pick --id auto --cached\n  agx pool pick --id auto --pool-force alice\n  agx pool pick --id auto --time-format both\n  agx pool pick --id auto --json --field data.selected.name"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the pool login")
	f.BoolVar(&o.Cached, "cached", false, "use the login-usage cache only, of any age; probe nothing")
	f.DurationVar(&o.MaxAge, "max-age", 0, "reuse cached usage younger than this; 0 probes every member (default: the pool's usage_max_age)")
	f.StringVar(&o.Force, "pool-force", "", "pick this member for this one run, over the pool's rules, force and picker")
	f.StringVar(&o.Model, "model", "", "the model the session uses: members are probed with it, so its per-model weekly window applies (default: the pool's probe_models, else the default profile's model)")
	addPoolOnlyFlags(f, &o.Only, "pick only this member, with its caps enforced; none when it is blocked", "this pick")
	f.StringVar(&now, "now", "", "pretend it is this RFC 3339 time (to read an old cache)")
	_ = f.MarkHidden("now") // the flag exists, so marking it cannot fail
	timeFlags(cmd, &to)
	return fields(cmd, poolPickFields...)
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
	pickRules     = "Rules: "
	// pickDefaultModel is the probe line without a model.
	pickDefaultModel = "Probe model: the agent's default (all-models windows only)"
)

// writePoolReport prints the pick: "Use: NAME (GROUP)" (or FORCED with a
// warning line per ignored rule), the reason, the warnings, the source, a
// blank line, then one line per member.
func writePoolReport(w io.Writer, rep app.PoolReport, st render.Style) error {
	ew := textout.New(w)
	head := fmt.Sprintf("Pool: %s (%s)", rep.Pool, rep.Agent)
	if rep.Order != "" {
		head = fmt.Sprintf("Pool: %s (%s, order %s)", rep.Pool, rep.Agent, rep.Order)
	}
	ew.Line(st.Paint(render.ToneDim, head))
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
	switch {
	case len(rep.ProbeModels) > 0:
		ew.Line(st.Paint(render.ToneDim, "Probe model: "+strings.Join(rep.ProbeModels, ", ")+" ("+rep.ProbeModelsSource+")"))
	case rep.ProbeModelsSource != "":
		ew.Line(st.Paint(render.ToneDim, pickDefaultModel))
	}
	if len(rep.Settings) > 0 {
		parts := make([]string, 0, len(rep.Settings))
		for _, s := range rep.Settings {
			parts = append(parts, s.Key+" "+s.Value+" "+st.Paint(render.ToneDim, "("+s.Source+")"))
		}
		ew.Line(pickRules + strings.Join(parts, ", "))
	}
	ew.Line("")
	ew.Line(st.Bold(pickHeader))
	v := timeViewOf(rep.View, st)
	for _, c := range rep.Candidates {
		ew.Line(candidateLine(c, v, st))
	}
	return ew.Err()
}

// timeViewOf is the usage view of a time view, so a reset reads the same
// in agx usage and agx pool pick.
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
	for _, sw := range c.Scoped {
		if sw.Percent != nil && !sw.Stale {
			usage = append(usage, fmt.Sprintf("%s %.0f%%", sw.Name, math.Round(*sw.Percent)))
		}
	}
	figures := strings.Join(usage, ", ")
	if figures == "" {
		figures = pickNoWindows
	}
	figures += ", " + pickReset(c, v)
	if c.RoomPercent != nil {
		figures += ", room " + strconv.FormatFloat(*c.RoomPercent, 'f', -1, 64) + "%"
	}
	if c.CapsSource.Short != "" {
		figures += ", caps " + capsText(c)
	}
	if c.UrgencyPercentPerHour != nil && c.State != pool.StateBlocked {
		figures += ", needs " + strconv.FormatFloat(*c.UrgencyPercentPerHour, 'f', -1, 64) + "%/h"
	}
	if c.Sessions > 0 {
		figures += fmt.Sprintf(", %d other sessions", c.Sessions)
	}
	notes := ""
	if len(c.Notes) > 0 {
		notes = st.Paint(render.ToneDim, " ("+strings.Join(c.Notes, "; ")+")")
	}
	return fmt.Sprintf("- %s [%s] %s (%s)%s", c.Name, c.Group, label, figures, notes)
}

// capsText is a member's caps with their sources: "50/70 (group)", or
// "95 (member)/70 (group)" when the sources differ.
func capsText(c app.PoolCandidate) string {
	if c.CapsSource.Short == c.CapsSource.Week {
		return fmt.Sprintf("%s/%s (%s)", numText(c.Caps.Short), numText(c.Caps.Week), c.CapsSource.Short)
	}
	return fmt.Sprintf("%s (%s)/%s (%s)", numText(c.Caps.Short), c.CapsSource.Short, numText(c.Caps.Week), c.CapsSource.Week)
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
