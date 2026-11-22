package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/internal/textout"
)

// Fields of the login commands' data (help --json).
var (
	loginInfoFields   = []string{"data.name", "data.agents", "data.kind", "data.source", "data.source_type", "data.account", "data.provider", "data.users", "data.detected", "data.detected_mismatch", "data.detected_view"}
	loginListFields   = []string{"data.*.name", "data.*.agents", "data.*.kind", "data.*.source", "data.*.source_type", "data.*.account", "data.*.provider", "data.*.users", "data.*.detected", "data.*.detected_mismatch", "data.*.detected_view"}
	loginCheckFields  = []string{"data.*.name", "data.*.kind", "data.*.source", "data.*.resolve", "data.*.detected", "data.*.detected_mismatch", "data.*.detected_view", "data.*.probe", "data.*.agent", "data.*.detail", "data.*.windows"}
	loginUseFields    = []string{"data.login", "data.home_login", "data.target", "data.name", "data.line"}
	loginRemoveFields = []string{"data.name", "data.cleared"}
	loginChangeFields = append(append([]string{}, loginInfoFields...), "data.changes", "data.changes.*.key", "data.changes.*.from", "data.changes.*.to", "data.written")
)

// loginCmd is agx login (spec 13.5).
func (c *cliCtx) loginCmd() *cobra.Command {
	return group("login", "Logins: what a launch authenticates with",
		c.loginListCmd(), c.loginShowCmd(), c.loginAddCmd(), c.loginEditCmd(),
		c.loginRemoveCmd(), c.loginUseCmd(), c.loginCheckCmd(), c.loginTokenCmd(),
	)
}

// loginListCmd is agx login list.
func (c *cliCtx) loginListCmd() *cobra.Command {
	var o app.LoginListOptions
	cmd := c.newCmd("list", "Every login, never a value", CostRead, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		rows, err := c.App().LoginList(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		return render.Output{Data: data, Table: []render.Column{
			{Header: "NAME", Key: "name"}, {Header: "AGENTS", Key: "agents"}, {Header: "KIND", Key: "kind"},
			{Header: "SOURCE", Key: "source"}, {Header: "DETECTED", Key: "detected_view"}, {Header: "ACCOUNT", Key: "account"},
		}, Empty: "no logins — add one with `agx login add --name NAME --agent AGENT` (see `agx login add --help`)"}, err
	})
	cmd.Flags().StringVar((*string)(&o.Agent), "agent", "", "only logins serving this agent")
	cmd.Flags().BoolVar(&o.Verify, "verify", false, "resolve every token (runs command and plugin references, cost X) to detect its real kind")
	return fields(cmd, loginListFields...)
}

// loginShowCmd is agx login show.
func (c *cliCtx) loginShowCmd() *cobra.Command {
	var o app.LoginShowOptions
	cmd := c.newCmd("show", "One login: kind, source, account, agents and what uses it", CostRead, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		info, err := c.App().LoginShow(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(info)
		return render.Output{Data: data, Text: func(w io.Writer) error { return loginShowText(w, info) }}, err
	})
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the login name")
	f.StringVar((*string)(&o.Agent), "agent", "", "the agent it must serve; narrows the users to it")
	return fields(cmd, loginInfoFields...)
}

// loginShowText renders one login as labelled lines.
func loginShowText(w io.Writer, info app.LoginInfo) error {
	ids := make([]string, len(info.Agents))
	for i, id := range info.Agents {
		ids[i] = string(id)
	}
	var rows [][2]string
	line := func(k, v string) {
		if v == "" {
			v = render.Dash
		}
		rows = append(rows, [2]string{k, v})
	}
	line("name", info.Name)
	line("agents", strings.Join(ids, ", "))
	line("kind", info.Kind)
	line("source", info.Source)
	if info.Detected != "" {
		line("detected", info.DetectedView)
	}
	line("account", info.Account)
	if info.Provider != "" {
		line("provider", info.Provider)
	}
	users := make([]string, len(info.Users))
	for i, u := range info.Users {
		users[i] = u.String()
	}
	line("users", strings.Join(users, "; "))
	return keyValues(w, rows)
}

// addSourceFlags declares the value flags add and edit share.
func addSourceFlags(cmd *cobra.Command, s *app.LoginSource) {
	f := cmd.Flags()
	f.BoolVar(&s.Stdin, "stdin", false, "read the value from stdin (stored in tokens.jsonc, 0600)")
	f.BoolVar(&s.LoginSetup, "login-setup", false, "run the first agent's token setup command (claude setup-token; cost X), then paste the token")
	f.StringVar(&s.TokenRef, "token-ref", "", "a reference (${gopass:…}, $(…), ${VAR}) or a plain value, written to config")
	f.StringVar(&s.HelperCommand, "helper-command", "", "an api-key-helper command the agent runs itself")
}

// setupRunner runs a token setup command on agx's own terminal.
func (c *cliCtx) setupRunner() app.SetupRunner {
	return func(ctx context.Context, path string, cmd agentio.Command) (int, error) {
		return run.RunAttached(ctx, path, cmd, c.env.Stdin, c.env.Stdout, c.env.Stderr)
	}
}

// loginAddCmd is agx login add.
func (c *cliCtx) loginAddCmd() *cobra.Command {
	var (
		o      app.LoginAddOptions
		agents []string
		ps     = &poolSettingFlags{}
	)
	cmd := c.newCmd("add", "Add a login", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		o.Agents = agentIDs(agents)
		o.Pool = ps.settings(cmd)
		o.Source.RunSetup = c.setupRunner()
		ch, err := c.App().LoginAdd(cmd.Context(), o)
		return loginChangeOutput("add", "added", ch, err)
	})
	cmd.Example = "  agx login add --name personal --agent claude --kind oauth-token --token-ref '${gopass:personal/claude-token}'\n" +
		"  pbpaste | agx login add --name pasted --agent claude --kind oauth-token --stdin\n" +
		"  agx login add --name carol --agent claude --kind oauth-token --login-setup\n" +
		"  agx login add --name auto --agent claude --kind pool --pool-own alice --pool-own bob --pool-shared carol\n" +
		"  agx login add --name auto --agent claude --kind pool --pool-own alice --pool-shared carol --pool-cap-short alice=95 --pool-cap-week alice=98 --dry-run"
	f := cmd.Flags()
	f.StringVar(&o.Name, "name", "", "the new login's name")
	f.StringArrayVar(&agents, "agent", nil, "an agent it serves (repeat)")
	f.StringVar(&o.Kind, "kind", "", "oauth-token, api-key, api-key-helper or pool (default: the first agent's default kind)")
	f.StringVar(&o.Account, "account", "", "a display label, such as the account email")
	f.StringVar(&o.Provider, "provider", "", "the model provider, for agents with a key per provider (pi)")
	f.StringArrayVar(&o.PoolOwn, "pool-own", nil, "a pool member that is your own account, used up almost fully (repeat; --kind pool)")
	f.StringArrayVar(&o.PoolShared, "pool-shared", nil, "a pool member that is a teammate's account, borrowed up to its caps (repeat; --kind pool)")
	addPoolSettingFlags(cmd, ps)
	f.BoolVar(&o.DryRun, "dry-run", false, "show the pool's keys; write nothing (--kind pool)")
	addSourceFlags(cmd, &o.Source)
	return fields(cmd, loginChangeFields...)
}

// loginEditCmd is agx login edit.
func (c *cliCtx) loginEditCmd() *cobra.Command {
	var (
		o                      app.LoginEditOptions
		agents, own, shared    []string
		account, provider, frc string
		ps                     = &poolSettingFlags{}
	)
	cmd := c.newCmd("edit", "Rotate or change a login", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		o.Agents = agentIDs(agents)
		if cmd.Flags().Changed("account") {
			o.Account = &account
		}
		if cmd.Flags().Changed("provider") {
			o.Provider = &provider
		}
		if cmd.Flags().Changed("pool-own") {
			o.PoolOwn = append([]string{}, own...)
		}
		if cmd.Flags().Changed("pool-shared") {
			o.PoolShared = append([]string{}, shared...)
		}
		if cmd.Flags().Changed("pool-force") {
			o.PoolForce = &frc
		}
		o.Pool = ps.settings(cmd)
		o.Source.RunSetup = c.setupRunner()
		ch, err := c.App().LoginEdit(cmd.Context(), o)
		return loginChangeOutput("change", "changed", ch, err)
	})
	cmd.Example = "  pbpaste | agx login edit --id pasted --stdin\n  agx login edit --id personal --account me@example.com\n  agx login edit --id auto --pool-force alice\n  agx login edit --id auto --pool-force ''\n  agx login edit --id auto --pool-cap-short alice=95 --pool-cap-week alice=98 --pool-disable carol --dry-run\n  agx login edit --id auto --pool-cap-short bob=95 --until 'sun 23:59'\n  agx login edit --id auto --pool-timezone Europe/Istanbul --pool-schedule 'weekend days=sat,sun members=shared short=95'"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the login to change")
	f.StringArrayVar(&agents, "agent", nil, "replace the agents it serves (repeat)")
	f.StringVar(&o.Kind, "kind", "", "change the kind")
	f.StringVar(&account, "account", "", "set the display label (empty clears it)")
	f.StringVar(&provider, "provider", "", "set the model provider (empty clears it)")
	f.StringArrayVar(&own, "pool-own", nil, "replace a pool's own members (repeat)")
	f.StringArrayVar(&shared, "pool-shared", nil, "replace a pool's shared members (repeat)")
	f.StringVar(&frc, "pool-force", "", "make a pool always pick this member (empty clears it)")
	addPoolSettingFlags(cmd, ps)
	f.BoolVar(&o.DryRun, "dry-run", false, "show a pool's changes; write nothing")
	addSourceFlags(cmd, &o.Source)
	return fields(cmd, loginChangeFields...)
}

// poolSettingFlags hold the pool setting flags of login add and edit
// until the command runs (a flag that was not given stays nil).
type poolSettingFlags struct {
	capShort, capWeek, disable, enable                        []string
	defShort, defWeek, cutoff, borrow, maxAge, keyTTL, picker string
	// The rule flags.
	order, relaxWindow, maxSessions, sticky, defRelax, leaveRatio      string
	deadlineWindow, deadlineMinRoom                                    string
	capRelax, groupShort, groupWeek, groupRelax, capacity, probeModels []string
	// The timed-cap flags.
	until, timezone                       string
	clearOverride, schedule, scheduleDrop []string
}

// addPoolSettingFlags declares the pool setting flags add and edit share.
func addPoolSettingFlags(cmd *cobra.Command, ps *poolSettingFlags) {
	f := cmd.Flags()
	f.StringArrayVar(&ps.capShort, "pool-cap-short", nil, "a member's own 5-hour cap: NAME=PERCENT; NAME= removes it (repeat)")
	f.StringArrayVar(&ps.capWeek, "pool-cap-week", nil, "a member's own weekly cap: NAME=PERCENT; NAME= removes it (repeat)")
	f.StringArrayVar(&ps.disable, "pool-disable", nil, "leave a member out of the pool: enabled false (repeat)")
	f.StringArrayVar(&ps.enable, "pool-enable", nil, "take a disabled member back (repeat)")
	f.StringVar(&ps.defShort, "pool-default-short", "", "the 5-hour cap of shared members without their own, in percent (empty: default 50)")
	f.StringVar(&ps.defWeek, "pool-default-week", "", "the weekly cap of shared members without their own, in percent (empty: default 70)")
	f.StringVar(&ps.cutoff, "pool-short-cutoff", "", "the 5-hour safety cutoff for every member, in percent (empty: default 99)")
	f.StringVar(&ps.borrow, "pool-borrow-window", "", "a shared member goes first when its weekly reset is this close, like 48h (empty: default 48h)")
	f.StringVar(&ps.maxAge, "pool-usage-max-age", "", "how old the usage cache may be before members are probed again, like 60s (empty: default 60s)")
	f.StringVar(&ps.keyTTL, "pool-key-ttl", "", "how long the agent keeps a key before it asks again, like 60s (empty: default 60s)")
	f.StringVar(&ps.picker, "pool-picker", "", "a shell command that picks the member instead of the built-in rules (empty removes it)")
	f.StringVar(&ps.order, "pool-order", "", "how the pool ranks its members: urgency (the most room to lose per hour first) or deadline (the soonest weekly reset first) (empty: default urgency)")
	f.StringVar(&ps.deadlineWindow, "pool-deadline-window", "", "under the urgency order, members whose weekly reset is this close (and that have deadline_min_room of room) go first, soonest reset first, like 24h; 0s = never (empty: default 24h)")
	f.StringVar(&ps.deadlineMinRoom, "pool-deadline-min-room", "", "the least room, in percent up to the weekly cap reached at the reset, a member inside the deadline window needs to go first (empty: default 2)")
	f.StringVar(&ps.relaxWindow, "pool-week-relax-window", "", "a member's weekly cap rises to its relax max over this long before its reset, like 24h; 0s = never (empty: default 24h)")
	f.StringVar(&ps.maxSessions, "pool-max-sessions", "", "how many sessions one member takes before the next is used; 0 = no limit (empty: default 4)")
	f.StringVar(&ps.sticky, "pool-sticky", "", "keep a session on its member while it is still in the pool: true or false (empty: default true)")
	f.StringVar(&ps.leaveRatio, "pool-sticky-leave-ratio", "", "a sticky session leaves its member when another is at least this many times as urgent; 0 = never (empty: default 3)")
	f.StringVar(&ps.defRelax, "pool-default-week-relax", "", "the weekly cap members reach at their reset, in percent (empty: default 95)")
	f.StringArrayVar(&ps.capRelax, "pool-cap-week-relax", nil, "a member's own weekly cap at its reset: NAME=PERCENT; NAME= removes it (repeat)")
	f.StringArrayVar(&ps.capacity, "pool-capacity", nil, "a member's plan size relative to the others (only ratios matter), so its room counts for more work: NAME=N; NAME= removes it (repeat)")
	f.StringArrayVar(&ps.groupShort, "pool-group-cap-short", nil, "the 5-hour cap of a group's members without their own: own=PERCENT or shared=PERCENT; GROUP= removes it (repeat)")
	f.StringArrayVar(&ps.groupWeek, "pool-group-cap-week", nil, "the weekly cap of a group's members without their own: own=PERCENT or shared=PERCENT; GROUP= removes it (repeat)")
	f.StringArrayVar(&ps.probeModels, "pool-probe-model", nil, "a model to probe the members with when a session's own model is unknown, so its per-model weekly window applies (repeat; '' removes probe_models)")
	f.StringArrayVar(&ps.groupRelax, "pool-group-cap-week-relax", nil, "the weekly cap a group's members reach at their reset: own=PERCENT or shared=PERCENT; GROUP= removes it (repeat)")
	f.StringVar(&ps.until, "until", "", "make this command's --pool-cap-short, --pool-cap-week and --pool-cap-week-relax an override that ends then: 6h, 2026-11-23T08:00, 'sun 23:59' or 18:00, in the pool's timezone")
	f.StringArrayVar(&ps.clearOverride, "pool-clear-override", nil, "end a member's override now (repeat)")
	f.StringVar(&ps.timezone, "pool-timezone", "", "the IANA time zone of the pool's schedules and --until times, like Europe/Istanbul (empty: this machine's local time)")
	f.StringArrayVar(&ps.schedule, "pool-schedule", nil, "add or replace a schedule: 'NAME days=sat,sun [hours=00:00-24:00] members=shared [timezone=ZONE] [short=N] [week=N] [week_relax_max=N] [week_before_reset=N] [release_before=5h]' (repeat)")
	f.StringArrayVar(&ps.scheduleDrop, "pool-schedule-remove", nil, "remove a schedule by name (repeat)")
}

// settings returns the flags that were given; an empty value is kept
// (it removes the key).
func (ps *poolSettingFlags) settings(cmd *cobra.Command) app.PoolSettings {
	var s app.PoolSettings
	given := cmd.Flags().Changed
	list := func(name string, v []string) []string {
		if given(name) {
			return append([]string{}, v...)
		}
		return nil
	}
	one := func(name string, v string) *string {
		if given(name) {
			return &v
		}
		return nil
	}
	s.CapShort, s.CapWeek = list("pool-cap-short", ps.capShort), list("pool-cap-week", ps.capWeek)
	s.Disable, s.Enable = list("pool-disable", ps.disable), list("pool-enable", ps.enable)
	s.DefaultShort, s.DefaultWeek = one("pool-default-short", ps.defShort), one("pool-default-week", ps.defWeek)
	s.ShortCutoff, s.BorrowWindow = one("pool-short-cutoff", ps.cutoff), one("pool-borrow-window", ps.borrow)
	s.UsageMaxAge, s.KeyTTL = one("pool-usage-max-age", ps.maxAge), one("pool-key-ttl", ps.keyTTL)
	s.Picker = one("pool-picker", ps.picker)
	s.Rules = app.PoolRuleSettings{
		Order: one("pool-order", ps.order), WeekRelaxWindow: one("pool-week-relax-window", ps.relaxWindow),
		MaxSessions: one("pool-max-sessions", ps.maxSessions), Sticky: one("pool-sticky", ps.sticky),
		StickyLeaveRatio: one("pool-sticky-leave-ratio", ps.leaveRatio),
		DeadlineWindow:   one("pool-deadline-window", ps.deadlineWindow), DeadlineMinRoom: one("pool-deadline-min-room", ps.deadlineMinRoom),
		DefaultWeekRelax: one("pool-default-week-relax", ps.defRelax), CapWeekRelax: list("pool-cap-week-relax", ps.capRelax),
		Capacity:      list("pool-capacity", ps.capacity),
		GroupCapShort: list("pool-group-cap-short", ps.groupShort), GroupCapWeek: list("pool-group-cap-week", ps.groupWeek),
		GroupCapWeekRelax: list("pool-group-cap-week-relax", ps.groupRelax),
		ProbeModels:       list("pool-probe-model", ps.probeModels),
	}
	s.Timed = app.PoolTimedSettings{
		Until: one("until", ps.until), ClearOverride: list("pool-clear-override", ps.clearOverride),
		Timezone: one("pool-timezone", ps.timezone), Schedules: list("pool-schedule", ps.schedule),
		ScheduleRemove: list("pool-schedule-remove", ps.scheduleDrop),
	}
	return s
}

// agentIDs converts flag values to agent ids.
func agentIDs(in []string) []agentio.ID {
	out := make([]agentio.ID, len(in))
	for i, s := range in {
		out[i] = agentio.ID(s)
	}
	return out
}

// loginChangeData is login add and edit's JSON: the login, plus the
// keys a pool command changed and whether the file was written.
type loginChangeData struct {
	app.LoginInfo
	Changes []app.Change `json:"changes,omitempty"`
	Written bool         `json:"written"`
}

// loginChangeOutput renders add and edit: one summary line, then a
// before/after line per changed pool key (as profile edit does). verb is
// the plain verb ("add") for --dry-run, done its past form ("added").
func loginChangeOutput(verb, done string, ch app.LoginChange, err error) (render.Output, error) {
	out := render.Output{Decisions: ch.Decisions, Warnings: ch.Warnings}
	if err != nil {
		return out, err
	}
	data, err := render.Encode(loginChangeData{LoginInfo: ch.Info, Changes: ch.Changes, Written: ch.Written})
	out.Data = data
	out.Text = func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		ew := textout.New(tw)
		switch {
		case !ch.Written && len(ch.Changes) == 0:
			ew.Line(fmt.Sprintf("login %s: no change (every value given is already set)", ch.Info.Name))
		case ch.DryRun:
			ew.Line(fmt.Sprintf("would %s login %s (--dry-run)", verb, ch.Info.Name))
		default:
			ew.Line(fmt.Sprintf("%s login %s (%s, %s)", done, ch.Info.Name, ch.Info.Kind, ch.Info.Source))
		}
		changeRows(ew, ch.Changes)
		if ew.Err() != nil {
			return ew.Err()
		}
		return tw.Flush()
	}
	return out, err
}

// loginRemoveCmd is agx login remove.
func (c *cliCtx) loginRemoveCmd() *cobra.Command {
	var o app.LoginRemoveOptions
	cmd := c.newCmd("remove", "Remove a login from config and tokens.jsonc", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		res, err := c.App().LoginRemove(cmd.Context(), o)
		out := render.Output{Warnings: res.Warnings}
		if err != nil {
			return out, err
		}
		data, err := render.Encode(res)
		out.Data = data
		out.Text = func(w io.Writer) error {
			ew := textout.New(w)
			ew.Line("removed login " + res.Name)
			for _, u := range res.Cleared {
				ew.Line("  cleared from " + u.String())
			}
			return ew.Err()
		}
		return out, err
	})
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the login to remove")
	f.BoolVar(&o.Cascade, "cascade", false, "also clear it from profiles, agent defaults and bindings")
	return fields(cmd, loginRemoveFields...)
}

// loginUseCmd is agx login use.
func (c *cliCtx) loginUseCmd() *cobra.Command {
	var o app.LoginUseOptions
	cmd := c.newCmd("use", "Set the login of a profile, an agent or this terminal", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		res, err := c.App().LoginUse(cmd.Context(), o)
		out := render.Output{Warnings: res.Warnings}
		if err != nil {
			return out, err
		}
		data, err := render.Encode(res)
		out.Data = data
		out.Text = func(w io.Writer) error { return loginUseText(w, res) }
		return out, err
	})
	cmd.Long = "Set the default login of a profile (--profile) or an agent (--agent), or print the line that sets this terminal's AGX_LOGIN (--terminal; the shell layer evaluates it, otherwise copy it)."
	cmd.Example = "  agx login use --id personal --profile work\n  agx login use --home-login --profile work\n  agx login use --id personal --agent claude\n  agx login use --id derek --terminal"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the login")
	f.BoolVar(&o.HomeLogin, "home-login", false, "the home's own login instead of a named one")
	f.StringVar(&o.Profile, "profile", "", "set it as this profile's login")
	f.StringVar((*string)(&o.Agent), "agent", "", "set it as this agent's default login")
	f.BoolVar(&o.Terminal, "terminal", false, "print the AGX_LOGIN line for this terminal; writes nothing")
	return fields(cmd, loginUseFields...)
}

// loginUseText prints the shell line alone for --terminal (so the shell
// layer can evaluate stdout), else a sentence.
func loginUseText(w io.Writer, res app.LoginUsed) error {
	ew := textout.New(w)
	if res.Target == app.UseTerminal {
		ew.Line(res.Line)
		return ew.Err()
	}
	what := "login " + res.Login
	if res.HomeLogin {
		what = "the home login"
	}
	ew.Line(fmt.Sprintf("%s %s now uses %s", res.Target, res.Name, what))
	return ew.Err()
}

// loginCheckCmd is agx login check.
func (c *cliCtx) loginCheckCmd() *cobra.Command {
	var o app.LoginCheckOptions
	cmd := c.newCmd("check", "Resolve logins and probe them (spends a few tokens unless --no-probe)", CostExec+CostQuota, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		rows, err := c.App().LoginCheck(cmd.Context(), o)
		out := render.Output{KeepData: rows != nil, Table: []render.Column{
			{Header: "NAME", Key: "name"}, {Header: "KIND", Key: "kind"}, {Header: "SOURCE", Key: "source"},
			{Header: "RESOLVE", Key: "resolve", Tone: checkTones}, {Header: "DETECTED", Key: "detected_view"}, {Header: "PROBE", Key: "probe", Tone: checkTones}, {Header: "DETAIL", Key: "detail"},
		}}
		data, eerr := render.Encode(rows)
		if eerr != nil {
			return out, eerr
		}
		out.Data = data
		return out, err
	})
	f := cmd.Flags()
	f.StringArrayVar(&o.IDs, "id", nil, "a login to check (repeat; default: every login)")
	f.BoolVar(&o.NoProbe, "no-probe", false, "resolve only; never spend quota")
	return fields(cmd, loginCheckFields...)
}

// loginTokenTerminal is agx login token's refusal of a terminal, with
// what to do in the same line (the command prints no hint lines).
const loginTokenTerminal = "stdout is a terminal; pipe it (| pbcopy) or add --reveal"

// oneLineBreaks joins an error message's lines for agx login token.
var oneLineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ")

// bareError is err as exactly one stderr line: its message with line
// breaks joined and no hint. agx login token is read by scripts and by an
// agent's apiKeyHelper (Claude Code shows the helper's stderr in /status),
// where a second "next step" line is noise; the code and exit class stay.
func bareError(err error) error {
	if err == nil {
		return nil
	}
	e := apperr.As(err)
	return apperr.Wrap(e, e.Code, e.Class, oneLineBreaks.Replace(e.Message))
}

// loginTokenCmd is agx login token: the one command that prints a login's
// token, on request, for a pipe or an apiKeyHelper. Stdout carries the
// bare value and a newline, nothing else, so a script or an agent reads it
// as is. A terminal is refused unless --reveal says so, because a token on
// a screen ends up in scrollback, screen shares and recordings. --json and
// --field are refused instead of wrapping the value: an envelope around a
// secret would be logged and stored by tools that expect harmless data.
func (c *cliCtx) loginTokenCmd() *cobra.Command {
	var (
		id, poolForce string
		reveal        bool
		only          app.PoolOnly
	)
	cmd := c.newCmd("token", "Print one login's token for a pipe or an apiKeyHelper (a terminal needs --reveal)", CostExec, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		out, err := c.loginToken(cmd, id, poolForce, only, reveal)
		return out, bareError(err)
	})
	cmd.Long = "Resolve a login's token from its source (a stored value, ${gopass:…}, $(…), ${VAR} or a plain value) and print only the value and a newline on stdout. It reads what is stored or referenced and never refreshes a token; a $(…) or gopass reference runs that local command. Stdout must be a pipe or a file unless --reveal is given. An api-key-helper login has no token and is refused. Every error is one line on stderr, without hint lines, and stdout stays empty."
	cmd.Example = "  agx login token --id work-key | pbcopy\n" +
		"  agx login add --name console --agent claude --kind api-key-helper --helper-command 'agx login token --id work-key'"
	f := cmd.Flags()
	f.StringVar(&id, "id", "", "the login name")
	f.BoolVar(&reveal, "reveal", false, "print the token even when stdout is a terminal")
	f.StringVar(&poolForce, "pool-force", "", "on a pool login: print this member's token, over the pool's rules")
	addPoolOnlyFlags(f, &only, "on a pool login: print this member's token while its caps allow, else none (never another member)", "this call")
	return cmd
}

// loginToken is agx login token's body; loginTokenCmd makes its error one
// line.
func (c *cliCtx) loginToken(cmd *cobra.Command, id, poolForce string, only app.PoolOnly, reveal bool) (render.Output, error) {
	if c.g.JSON || len(c.g.Fields) > 0 {
		// The refusal prints as the command's other errors do, one line
		// on stderr with stdout empty: a --field path would select from
		// an envelope without data and drop the message, and a JSON
		// envelope on stdout is what a script would take for the value.
		c.g.JSON, c.g.Fields = false, nil
		return render.Output{}, apperr.Usage("login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field")
	}
	if c.env.StdoutTTY && !reveal {
		return render.Output{}, apperr.Usage(loginTokenTerminal)
	}
	tok, warnings, err := c.App().LoginTokenWarn(cmd.Context(), id, poolForce, only)
	if err != nil {
		return render.Output{Warnings: warnings}, err
	}
	return render.Output{Warnings: warnings, Text: func(w io.Writer) error {
		_, err := io.WriteString(w, tok.Reveal()+"\n")
		return err
	}}, nil
}
