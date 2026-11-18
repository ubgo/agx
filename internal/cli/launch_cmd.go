package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/run"
)

// Fields of the launch plan's data (help --json).
var launchFields = []string{"data.run_id", "data.run_dir", "data.agent", "data.profile", "data.home", "data.login", "data.folder", "data.binary", "data.version", "data.command", "data.notes", "decisions"}

// addLaunchFlags declares the flags run, new and resume share (13.1).
func addLaunchFlags(cmd *cobra.Command, o *app.LaunchOptions, withDir bool) {
	f := cmd.Flags()
	f.StringVarP(&o.Profile, "profile", "p", "", "the profile")
	f.StringVar((*string)(&o.Agent), "agent", "", "agent for the default or auto profile")
	f.StringVar(&o.Login, "login", "", "a login name")
	f.BoolVar(&o.HomeLogin, "home-login", false, "the home's own login")
	f.BoolVar(&o.AutoLogin, "auto-login", false, "pick the stored login with the most headroom (probes, cost Q)")
	f.StringVar(&o.TokenRef, "token-ref", "", "a one-off token: a reference (${gopass:…}, $(…), ${VAR}) or a plain value")
	f.StringVar(&o.TokenKind, "token-kind", "oauth-token", "kind of --token-ref: oauth-token, api-key or api-key-helper")
	if withDir {
		f.StringVar(&o.Dir, "dir", "", "start folder (default: the current folder)")
	}
	f.StringVar(&o.Model, "model", "", "model")
	f.StringVar(&o.Prompt, "prompt", "", "initial prompt")
	f.StringArrayVar(&o.AddDirs, "add-dir", nil, "extra folder the agent may access (repeat)")
	f.StringVar(&o.Autonomy, "autonomy", "", "default, read-only, ask, accept-edits or full")
	f.StringVar(&o.Consent, "consent", "", "full-autonomy: allow --autonomy full")
	f.BoolVar(&o.Headless, "headless", false, "machine-readable event stream")
	f.StringArrayVar(&o.Options, "option", nil, "typed agent launch option key=value (repeat)")
	f.StringArrayVar(&o.Settings, "setting", nil, "agent settings key=value; dotted keys nest (repeat)")
	f.StringVar(&o.SettingsDelivery, "settings-delivery", "", "file (default) or inline")
	f.StringArrayVar(&o.Env, "env", nil, "environment NAME=value; references expand strictly (repeat)")
	f.StringArrayVar(&o.NoEnv, "no-env", nil, "remove this variable (repeat)")
	f.StringArrayVar(&o.EnvFiles, "env-file", nil, "a dotenv file, relative to the current folder (repeat)")
	f.StringVar(&o.BeforeRun, "before-run", "", "a script to run first, for this run (relative to the current folder)")
	f.BoolVar(&o.LiveLogin, "live-login", false, "switch this session's login while it runs (agx live use)")
	f.BoolVar(&o.DryRun, "dry-run", false, "print every decision with its source; start nothing")
	f.BoolVar(&o.ResolvedSecrets, "resolved-secrets", false, "with --dry-run: resolve references (cost X) and say resolved or why not")
	f.StringVar(&o.PlanFile, "plan-file", "", "write the launch plan for the shell layer instead of starting")
	_ = f.MarkHidden("plan-file") // the flag exists, so marking it cannot fail
}

// runCmd is agx run.
func (c *cliCtx) runCmd() *cobra.Command {
	o := &app.LaunchOptions{Mode: app.ModeRun}
	cmd := c.newCmd("run", "Start an agent here", CostExec, c.launchBody(o))
	cmd.Long = "Start an agent in --dir or the current folder, on the resolved profile, home and login. Everything after -- goes to the agent untouched."
	cmd.Example = "  agx run --profile work\n  agx run --profile work --model opus --dry-run\n  agx run --agent codex -- --search"
	addLaunchFlags(cmd, o, true)
	f := cmd.Flags()
	f.BoolVar(&o.AutoProfile, "auto-profile", false, "pick the profile with the most headroom (cost N)")
	return fields(launching(cmd), launchFields...)
}

// newSessionCmd is agx new.
func (c *cliCtx) newSessionCmd() *cobra.Command {
	o := &app.LaunchOptions{Mode: app.ModeNew}
	cmd := c.newCmd("new", "Create a timestamped session folder and start an agent in it", CostWrite+CostExec, c.launchBody(o))
	cmd.Example = `  agx new --name "pglite golang" --profile work`
	addLaunchFlags(cmd, o, false)
	f := cmd.Flags()
	f.StringVar(&o.SessionName, "name", "", "slug words for the folder name")
	f.BoolVar(&o.AutoProfile, "auto-profile", false, "pick the profile with the most headroom (cost N)")
	return fields(launching(cmd), launchFields...)
}

// launchBody prepares the plan and either prints it (--dry-run), writes
// it (--plan-file) or starts the agent.
func (c *cliCtx) launchBody(o *app.LaunchOptions) runFunc {
	return func(cmd *cobra.Command, args []string) (render.Output, error) {
		o.Raw = rawArgs(cmd, args)
		// --resolved-secrets only changes what a dry run reports (a real
		// launch resolves every reference anyway), so without --dry-run it
		// would do nothing: refused, never silently ignored (spec 5.1).
		if o.ResolvedSecrets && !o.DryRun {
			return render.Output{}, apperr.Usage("--resolved-secrets only works with --dry-run; add --dry-run")
		}
		if o.PlanFile != "" && !o.DryRun {
			return c.writePlanFile(cmd, *o)
		}
		out := render.Output{Command: commandPath(cmd)}
		a := c.App()
		p, err := a.Prepare(cmd.Context(), *o)
		if err != nil {
			return out, err
		}
		for _, n := range p.Notes {
			c.note(n)
		}
		out.Decisions, out.Warnings = p.Decisions, p.Warnings
		if o.DryRun {
			data, err := render.Encode(p)
			if err != nil {
				return out, err
			}
			out.Data = data
			st := c.style()
			out.Text = func(w io.Writer) error { return dryRunText(w, p, c.env.Width, st) }
			return out, nil
		}
		return out, c.start(cmd, p)
	}
}

// start commits the plan and starts the agent: exec on Unix (never
// returns on success), a child on Windows (returns its exit code).
func (c *cliCtx) start(cmd *cobra.Command, p *app.LaunchPlan) error {
	if !c.g.Quiet {
		for _, w := range p.Warnings {
			// stderr is the last place to report to; a failed write there
			// has nowhere else to go.
			_, _ = io.WriteString(c.env.Stderr, render.WarningLine(w.Message, c.opts().Color)+"\n")
		}
	}
	if err := c.App().Commit(cmd.Context(), p); err != nil {
		return err
	}
	code, err := run.Start(p.BinaryPath, p.Command)
	if err != nil {
		return err
	}
	return agentExit{code: code}
}

// dryRunText renders the plan like spec 6.6: one line per decision, the
// "from" column aligned without letting one long value push it off screen.
func dryRunText(w io.Writer, p *app.LaunchPlan, width int, st render.Style) error {
	lines := make([]render.DecisionLine, 0, len(p.Decisions)+len(p.Warnings)+1)
	for _, d := range p.Decisions {
		lines = append(lines, render.DecisionLine{Name: d.Name, Value: d.Value, Source: d.Source})
	}
	for _, n := range p.Warnings {
		lines = append(lines, render.DecisionLine{Name: "note", Value: n.Message})
	}
	if p.ParentStays {
		lines = append(lines, render.DecisionLine{Name: "note", Value: "Windows has no exec: agx stays as the parent and returns the agent's exit code"})
	}
	return render.WriteDecisions(w, width, lines, st)
}
