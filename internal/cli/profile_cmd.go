package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/human"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/internal/textout"
)

// profileCmd is agx profile (spec 13.4).
func (c *cliCtx) profileCmd() *cobra.Command {
	return group("profile", "Profiles: a name for one home plus launch defaults",
		c.profileListCmd(), c.profileShowCmd(), c.profileAddCmd(), c.profileImportCmd(),
		c.profileEditCmd(), c.profileRemoveCmd(), c.profileUseCmd(),
	)
}

// profileListOpts are agx profile list's flags.
type profileListOpts struct {
	app.ProfileListOptions
	Discovered bool
	Search     []string
}

// profileListCmd is agx profile list.
func (c *cliCtx) profileListCmd() *cobra.Command {
	var o profileListOpts
	cmd := c.newCmd("list", "Every profile; --discovered lists homes found on this machine", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		a := c.App()
		if o.Discovered {
			if o.Plan || len(o.Profiles) > 0 {
				return render.Output{}, apperr.Usage("--plan and --profile list profiles; they do not apply to --discovered")
			}
			rows, warns, err := a.Discover(cmd.Context(), app.DiscoverOptions{Agent: o.Agent, Search: o.Search})
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(rows)
			return render.Output{Data: data, Warnings: warns, Text: func(w io.Writer) error { return discoveredText(w, rows) }}, err
		}
		if len(o.Search) > 0 {
			return render.Output{}, apperr.Usage("--search needs --discovered")
		}
		rows, warns, err := a.ProfileList(cmd.Context(), o.ProfileListOptions)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		st, now := c.style(), a.Now()
		return render.Output{Data: data, Warnings: warns, Text: func(w io.Writer) error { return profileListText(w, rows, o.Plan, now, st) }}, err
	})
	cmd.Example = "  agx profile list\n  agx profile list --agent claude --plan\n  agx profile list --discovered --search ~/accounts"
	f := cmd.Flags()
	f.StringVar((*string)(&o.Agent), "agent", "", "only this agent")
	f.StringArrayVarP(&o.Profiles, "profile", "p", nil, "only these profiles (repeat)")
	f.BoolVar(&o.Plan, "plan", false, "add each home's plan (cost N)")
	f.BoolVar(&o.Discovered, "discovered", false, "list agent homes found on this machine")
	f.StringArrayVar(&o.Search, "search", nil, "with --discovered: also look in this folder and its sub-folders (repeat)")
	return fields(cmd, "data.*.name", "data.*.agent", "data.*.home", "data.*.default", "data.*.billing", "data.*.login", "data.*.home_exists", "data.*.plan",
		"data.*.account", "data.*.plan_label", "data.*.state", "data.*.expires_at",
		"data.*.path", "data.*.profiles", "data.*.found_by", "data.*.looks_like_home")
}

// profile list's table, ported from agx v0.3's `agx profiles`: the
// file's order, "*" before the default profile of each agent, whose
// account each home is (email · plan) and the home login's STATE, so one
// glance says which homes can run.
const (
	// markDefault prefixes the default profile's name.
	markDefault = "*"
	// accountSep joins the email and the plan ("me@x.com · Max (20x)").
	accountSep = " · "
	// homeMissingNote follows a home folder that does not exist.
	homeMissingNote = " (missing)"
)

// loginStateTones colours STATE: ok blue, a login to fix amber.
var loginStateTones = map[app.LoginState]render.Tone{
	app.LoginStateOK:      render.ToneGood,
	app.LoginStateExpired: render.ToneWarn,
	app.LoginStateNone:    render.ToneWarn,
	app.LoginStateUnknown: render.ToneDim,
}

// profileListText prints PROFILE PROVIDER HOME ACCOUNT STATE LOGIN
// BILLING (and PLAN with --plan).
func profileListText(w io.Writer, rows []app.ProfileRow, plan bool, now time.Time, st render.Style) error {
	header := []string{"PROFILE", "PROVIDER", "HOME", "ACCOUNT", "STATE", "LOGIN", "BILLING"}
	if plan {
		header = append(header, "PLAN")
	}
	cells := make([][]render.StyledCell, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if r.Default {
			name = markDefault + name
		}
		home := render.StyledCell{Text: r.Home}
		if !r.HomeExists {
			home = render.StyledCell{Text: r.Home + homeMissingNote, Tone: render.ToneBad}
		}
		account := r.Account
		if r.PlanLabel != "" {
			account = strings.TrimPrefix(account+accountSep+r.PlanLabel, accountSep)
		}
		state := string(r.State)
		if r.State == app.LoginStateOK && r.ExpiresAt != nil {
			state += " (" + human.Until(r.ExpiresAt.Sub(now)) + ")"
		}
		row := []render.StyledCell{{Text: name}, {Text: string(r.Agent)}, home, {Text: account},
			{Text: state, Tone: loginStateTones[r.State]}, {Text: r.Login}, {Text: r.Billing}}
		if plan {
			row = append(row, render.StyledCell{Text: r.Plan})
		}
		cells = append(cells, row)
	}
	return render.StyledTable(w, header, cells, st)
}

// discoveredText renders profile list --discovered like spec 6.2.
func discoveredText(w io.Writer, rows []app.DiscoveredHome) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ew := textout.New(tw)
	ew.Line("AGENT\tPATH\tPROFILE\tFOUND BY\tLOOKS LIKE A HOME")
	for _, r := range rows {
		prof := render.Dash
		if len(r.Profiles) > 0 {
			prof = strings.Join(r.Profiles, ",")
		}
		looks := "yes"
		if !r.LooksLikeHome {
			looks = "no (name only)"
		}
		ew.Line(fmt.Sprintf("%s\t%s\t%s\t%s\t%s", r.Agent, r.Path, prof, strings.Join(r.FoundBy, "; "), looks))
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	return tw.Flush()
}

// profileShowCmd is agx profile show.
func (c *cliCtx) profileShowCmd() *cobra.Command {
	var id string
	cmd := c.newCmd("show", "One profile and the source of each value", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProfileShow(id)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error { return valuesText(w, res.Values, c.style()) }}, err
	})
	cmd.Example = "  agx profile show --id work"
	cmd.Flags().StringVar(&id, "id", "", "the profile")
	return fields(cmd, "data.name", "data.agent", "data.values", "data.values.*.key", "data.values.*.value", "data.values.*.source")
}

// valuesText renders KEY VALUE SOURCE rows: a bold header, the sources
// dim so the values stand out.
func valuesText(w io.Writer, rows []app.ValueSource, st render.Style) error {
	cells := make([][]render.StyledCell, 0, len(rows))
	for _, r := range rows {
		cells = append(cells, []render.StyledCell{{Text: r.Key}, plainOrDash(r.Value), {Text: r.Source, Tone: render.ToneDim}})
	}
	return render.StyledTable(w, []string{"KEY", "VALUE", "SOURCE"}, cells, st)
}

// profileAddCmd is agx profile add.
func (c *cliCtx) profileAddCmd() *cobra.Command {
	var o app.ProfileAddOptions
	cmd := c.newCmd("add", "Create a profile for a home; links the default home's shared files unless --bare", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProfileAdd(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		if o.LoginSetup && !o.DryRun {
			if err := c.loginSetup(cmd, &res); err != nil {
				return render.Output{}, err
			}
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error { return profileAddText(w, res) }}, err
	})
	cmd.Example = "  agx profile add --name work --agent claude\n  agx profile add --name kimi --agent claude --home ~/.claude --billing api\n  agx profile add --name work --agent claude --dry-run"
	f := cmd.Flags()
	f.StringVar(&o.Name, "name", "", "the new profile's name")
	f.StringVar((*string)(&o.Agent), "agent", "", "the agent")
	f.StringVar(&o.Home, "home", "", "the home folder (default: the agent's default home plus -NAME, such as ~/.claude-work)")
	f.StringVar(&o.Login, "login", "", "the profile's login")
	f.BoolVar(&o.HomeLogin, "home-login", false, "use the home's own login (also over an agent default login)")
	f.StringVar(&o.Billing, "billing", "", "plan (default) or api")
	f.BoolVar(&o.Bare, "bare", false, "do not link the default home's shared files")
	f.BoolVar(&o.LoginSetup, "login-setup", false, "run the agent's token setup command in the new home, then store the pasted token as login NAME (cost X; terminal)")
	f.BoolVar(&o.DryRun, "dry-run", false, "show the entry and the shared files it would link; write nothing")
	return fields(cmd, "data.name", "data.agent", "data.home", "data.home_source", "data.home_created", "data.default", "data.login", "data.billing", "data.links", "data.login_setup", "data.written")
}

// loginSetup runs the token setup command of the profile just added,
// attached to the terminal (its output goes to stderr with --json, so
// stdout stays one envelope), then stores the pasted token as the
// profile's login.
func (c *cliCtx) loginSetup(cmd *cobra.Command, res *app.ProfileAddResult) error {
	a := c.App()
	path, setup, err := a.LoginSetupCommand(res.Name)
	if err != nil {
		return err
	}
	out := c.env.Stdout
	if c.g.JSON {
		out = c.env.Stderr
	}
	code, err := run.RunAttached(cmd.Context(), path, setup, c.env.Stdin, out, c.env.Stderr)
	if err != nil {
		return err
	}
	if code != 0 {
		return apperr.Failure(apperr.CodeIO, fmt.Sprintf("%s exited %d; profile %s was added without a login", res.LoginSetup, code, res.Name)).
			WithHint("agx login add --name " + res.Name + " --agent " + string(res.Agent) + " --stdin\nagx profile edit --id " + res.Name + " --login " + res.Name)
	}
	login, err := a.StoreSetupToken(cmd.Context(), res.Name)
	if err != nil {
		return err
	}
	res.Login = login
	return nil
}

// profileAddText renders profile add's result.
func profileAddText(w io.Writer, r app.ProfileAddResult) error {
	ew := textout.New(w)
	verb := "added"
	if !r.Written {
		verb = "would add"
	}
	line := fmt.Sprintf("%s profile %s: %s home %s (%s), login %s, billing %s", verb, r.Name, r.Agent, r.Home, r.HomeSource, r.Login, r.Billing)
	if r.Default {
		line += ", default (first " + string(r.Agent) + " profile)"
	}
	ew.Line(line)
	if r.HomeCreated {
		ew.Line("  create " + r.Home)
	}
	if r.LoginSetup != "" && !r.Written {
		ew.Line("  then run " + r.LoginSetup + " and store the pasted token as login " + r.Name)
	}
	for _, l := range r.Links {
		state := "link"
		if l.Exists {
			state = "keep (exists)"
		}
		ew.Line(fmt.Sprintf("  %s %s -> %s", state, l.To, l.From))
	}
	return ew.Err()
}

// profileImportCmd is agx profile import.
func (c *cliCtx) profileImportCmd() *cobra.Command {
	var o app.ProfileImportOptions
	cmd := c.newCmd("import", "Write profiles for discovered homes; a plan unless --yes", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProfileImport(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error { return importText(w, res) }}, err
	})
	cmd.Example = "  agx profile import --discovered\n  agx profile import --discovered --yes\n  agx profile import --id ~/.claude-acmecorp --name acmecorp --yes"
	f := cmd.Flags()
	f.BoolVar(&o.Discovered, "discovered", false, "every discovered home that looks like a home and has no profile")
	f.StringArrayVar(&o.IDs, "id", nil, "a home folder to import (repeat)")
	f.StringVar(&o.Name, "name", "", "the profile name (one --id only)")
	f.StringVar((*string)(&o.Agent), "agent", "", "only this agent's homes; decides the agent of an --id folder")
	f.StringArrayVar(&o.Search, "search", nil, "also look in this folder and its sub-folders (repeat)")
	f.BoolVar(&o.Yes, "yes", false, "write the profiles (without it: the plan)")
	return fields(cmd, "data.profiles", "data.profiles.*.name", "data.profiles.*.agent", "data.profiles.*.home", "data.profiles.*.default", "data.skipped", "data.written")
}

// importText renders profile import's plan or result. The table is
// flushed before the skipped/wrote/plan lines, which are plain lines:
// written through the tabwriter they would widen the NAME column to their
// own length.
func importText(w io.Writer, r app.ProfileImportResult) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	tew := textout.New(tw)
	if len(r.Profiles) == 0 {
		tew.Line("nothing to import")
	} else {
		tew.Line("NAME\tAGENT\tHOME\tDEFAULT\tFOUND BY")
		for _, p := range r.Profiles {
			tew.Line(fmt.Sprintf("%s\t%s\t%s\t%s\t%s", p.Name, p.Agent, p.Home, yesNoText(p.Default), strings.Join(p.FoundBy, "; ")))
		}
	}
	if tew.Err() != nil {
		return tew.Err()
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	ew := textout.New(w)
	for _, s := range r.Skipped {
		ew.Line(fmt.Sprintf("skipped %s %s: %s", s.Agent, s.Home, s.Reason))
	}
	switch {
	case r.Written:
		ew.Line("wrote " + countOf(len(r.Profiles), "profile", "profiles"))
	case len(r.Profiles) > 0:
		ew.Line("plan only: add --yes to write these profiles")
	}
	return ew.Err()
}

// yesNoText renders a bool as yes or no.
func yesNoText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// profileEditOpts are agx profile edit's flags; the inherit pair are two
// on/off flags (--inherit-defaults / --no-inherit-defaults) for one key.
type profileEditOpts struct {
	app.ProfileEditOptions
	Inherit   bool
	NoInherit bool
}

// profileEditCmd is agx profile edit.
func (c *cliCtx) profileEditCmd() *cobra.Command {
	var o profileEditOpts
	cmd := c.newCmd("edit", "Change a profile's login, home, args, env or billing", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		if o.Inherit && o.NoInherit {
			return render.Output{}, apperr.Conflict("--inherit-defaults and --no-inherit-defaults both set inherit_defaults; pick one")
		}
		if o.Inherit || o.NoInherit {
			v := o.Inherit
			o.InheritDefaults = &v
		}
		res, err := c.App().ProfileEdit(cmd.Context(), o.ProfileEditOptions)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error { return changesText(w, "profile "+res.Name, res.Changes, res.Written) }}, err
	})
	cmd.Example = "  agx profile edit --id work --login carol\n  agx profile edit --id kimi --no-inherit-defaults --args --dangerously-skip-permissions\n  agx profile edit --id work --env DEBUG=1 --dry-run"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the profile")
	f.StringVar(&o.Login, "login", "", "set the profile's login")
	f.BoolVar(&o.HomeLogin, "home-login", false, "use the home's own login")
	f.StringVar(&o.Home, "home", "", "move the profile to another home folder (the old one is kept)")
	f.StringArrayVar(&o.Args, "args", nil, "replace the profile's raw args (repeat, one arg each; --args \"\" removes them)")
	f.BoolVar(&o.Inherit, "inherit-defaults", false, "use agents.<agent>.defaults.args first (the default)")
	f.BoolVar(&o.NoInherit, "no-inherit-defaults", false, "drop agents.<agent>.defaults.args for this profile")
	f.StringArrayVar(&o.Env, "env", nil, "set a literal variable NAME=value; NAME= removes it (repeat)")
	f.StringVar(&o.Billing, "billing", "", "plan or api")
	f.BoolVar(&o.DryRun, "dry-run", false, "show the changes; write nothing")
	return fields(cmd, "data.name", "data.changes", "data.changes.*.key", "data.changes.*.from", "data.changes.*.to", "data.written")
}

// changesText renders a write's changes.
func changesText(w io.Writer, what string, changes []app.Change, written bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ew := textout.New(tw)
	verb := "changed"
	if !written {
		verb = "would change (--dry-run)"
	}
	ew.Line(what + ": " + verb + "\t\t\t")
	for _, ch := range changes {
		from, to := ch.From, ch.To
		if from == "" {
			from = render.Dash
		}
		if to == "" {
			to = render.Dash
		}
		ew.Line("  " + ch.Key + "\t" + from + "\t->\t" + to)
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	return tw.Flush()
}

// profileRemoveCmd is agx profile remove.
func (c *cliCtx) profileRemoveCmd() *cobra.Command {
	var id string
	var yes bool
	cmd := c.newCmd("remove", "Remove a profile entry; the home is never deleted", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProfileRemove(cmd.Context(), id, yes)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error { return profileRemoveText(w, res) }}, err
	})
	cmd.Example = "  agx profile remove --id old\n  agx profile remove --id old --yes"
	f := cmd.Flags()
	f.StringVar(&id, "id", "", "the profile")
	f.BoolVar(&yes, "yes", false, "remove it (without it: the plan)")
	return fields(cmd, "data.name", "data.home", "data.bindings", "data.removed")
}

// profileRemoveText renders profile remove's plan or result.
func profileRemoveText(w io.Writer, r app.ProfileRemoveResult) error {
	ew := textout.New(w)
	verb := "removed"
	if !r.Removed {
		verb = "would remove"
	}
	ew.Line(fmt.Sprintf("%s profile %s (home %s is kept)", verb, r.Name, r.Home))
	for _, b := range r.Bindings {
		ew.Line("  and its binding in " + b)
	}
	if !r.Removed {
		ew.Line("plan only: add --yes to remove it")
	}
	return ew.Err()
}

// profileUseCmd is agx profile use.
func (c *cliCtx) profileUseCmd() *cobra.Command {
	var id string
	cmd := c.newCmd("use", "Make a profile its agent's default", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProfileUse(cmd.Context(), id)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ew := textout.New(w)
			ew.Line(fmt.Sprintf("profile %s is the default %s profile", res.Name, res.Agent))
			if len(res.Cleared) > 0 {
				ew.Line("  (was: " + strings.Join(res.Cleared, ", ") + ")")
			}
			return ew.Err()
		}}, err
	})
	cmd.Example = "  agx profile use --id work"
	cmd.Flags().StringVar(&id, "id", "", "the profile")
	return fields(cmd, "data.name", "data.agent", "data.cleared")
}
