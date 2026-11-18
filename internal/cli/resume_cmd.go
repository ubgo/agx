package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// resumeCmd is agx resume (spec 6.5, 13.1): continue a conversation on
// the home that stores it, in its folder.
func (c *cliCtx) resumeCmd() *cobra.Command {
	o := &app.LaunchOptions{Mode: app.ModeResume}
	cmd := c.newCmd("resume", "Continue a conversation on the home that stores it", CostRead+CostExec, c.launchBody(o))
	cmd.Long = "Continue a conversation in its own folder, on the home that stores it. Without --id or --last, a picker of this folder's chats (needs a terminal). The login is resolved as for agx run, with the binding of the chat's folder."
	cmd.Example = "  agx resume\n  agx resume --id 3f2a9c1e --login carol --dry-run\n  agx resume --last --scope all\n  agx resume --picker folder"
	addLaunchFlags(cmd, o, false)
	f := cmd.Flags()
	f.StringVar(&o.ResumeID, "id", "", "the conversation, exact or a unique prefix")
	f.BoolVar(&o.Resume.Last, "last", false, "the newest chat, no picker")
	f.StringVar(&o.Resume.Query, "query", "", "narrow the list by title, folder or id")
	f.StringVar(&o.Resume.Picker, "picker", app.ResumePickerChat, strings.Join(app.ResumePickerValues(), " or ")+": pick a chat, or a folder then a chat")
	f.StringVar(&o.Resume.Scope, "scope", app.ResumeScopeFolder, strings.Join(app.ResumeScopeValues(), " or ")+": this folder's chats, or every configured home's")
	f.IntVar(&o.Resume.Limit, "limit", app.DefaultResumeLimit, "conversations considered, newest first (0 = all)")
	return fields(launching(cmd), launchFields...)
}

// whoamiCmd is agx whoami (spec 13.1).
func (c *cliCtx) whoamiCmd() *cobra.Command {
	cmd := c.newCmd("whoami", "Profile, login, home and run id of the current session", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			w, err := c.App().Whoami()
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(w)
			if err != nil {
				return render.Output{}, err
			}
			return render.Output{Data: data, Text: func(out io.Writer) error { return whoamiText(out, w) }}, nil
		})
	return fields(cmd, "data.in_session", "data.run_id", "data.agent", "data.profile", "data.login", "data.home", "data.run_dir", "data.live", "data.terminal_logins")
}

// whoamiText prints the session as name/value lines.
func whoamiText(w io.Writer, s app.Whoami) error {
	var pairs [][2]string
	line := func(key, value string) { pairs = append(pairs, [2]string{key + ":", value}) }
	if s.InSession {
		for _, kv := range [][2]string{{"run", s.RunID}, {"agent", s.Agent}, {"profile", s.Profile}, {"login", s.Login}, {"home", s.Home}} {
			line(kv[0], orDash(kv[1]))
		}
		if s.Live != nil {
			line("live", "started on "+s.Live.StartLogin+", next "+s.Live.Want()+", in use "+orDash(s.Live.InUse))
		}
	} else {
		line("session", "not inside a session started by agx (AGX_RUN_ID is not set)")
	}
	for _, t := range s.TerminalLogins {
		line(t.Variable, t.Login)
	}
	width := whoamiLabelWidth
	for _, p := range pairs {
		width = max(width, len(p[0]))
	}
	ew := textout.New(w)
	for _, p := range pairs {
		ew.Line(fmt.Sprintf("%-*s %s", width, p[0], p[1]))
	}
	return ew.Err()
}

// whoamiLabelWidth is the narrowest "label:" column (v0.3's "profile:  x").
const whoamiLabelWidth = 9

// orDash shows an empty value as a dash.
func orDash(s string) string {
	if s == "" {
		return render.Dash
	}
	return s
}

// cleanCmd is agx clean (spec 11).
func (c *cliCtx) cleanCmd() *cobra.Command {
	var o app.CleanOptions
	cmd := c.newCmd("clean", "Delete run folders whose agent has ended", CostWrite,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			res, err := c.App().Clean(cmd.Context(), o)
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(res)
			if err != nil {
				return render.Output{}, err
			}
			out := render.Output{Data: data, Text: func(w io.Writer) error { return runCleanText(w, res) }}
			for _, r := range res.Runs {
				if r.Status == agentio.LivenessUnknown.String() {
					out.Warnings = append(out.Warnings, render.Warning{Code: "kept-unknown", Message: "kept " + r.ID + ": " + r.Reason})
				}
			}
			return out, nil
		})
	cmd.Example = "  agx clean            # the plan\n  agx clean --yes      # delete ended runs"
	f := cmd.Flags()
	f.BoolVar(&o.Yes, "yes", false, "delete the ended runs (without it: the plan only)")
	f.BoolVar(&o.DryRun, "dry-run", false, "show the plan; delete nothing")
	return fields(cmd, "data.runs", "data.runs.*.id", "data.runs.*.liveness", "data.runs.*.action", "data.applied", "data.deleted", "data.kept")
}

// runCleanText prints one line per run, then the summary.
func runCleanText(w io.Writer, res app.CleanResult) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ew := textout.New(tw)
	if len(res.Runs) == 0 {
		ew.Line("no run folders")
	} else {
		ew.Line("RUN\tAGENT\tPID\tLIVENESS\tACTION\tREASON")
		for _, r := range res.Runs {
			ew.Line(strings.Join([]string{r.ID, orDash(r.Agent), strconv.Itoa(r.PID), r.Status, string(r.Action), r.Reason}, "\t"))
		}
	}
	if ew.Err() != nil {
		return ew.Err()
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	ew = textout.New(w)
	planned := 0
	for _, r := range res.Runs {
		if r.Action == app.CleanDelete {
			planned++
		}
	}
	switch {
	case res.Applied:
		ew.Line(fmt.Sprintf("deleted %d, kept %d", res.Deleted, res.Kept))
	case planned > 0:
		ew.Line(fmt.Sprintf("would delete %d, keep %d; run agx clean --yes to delete", planned, res.Kept))
	}
	return ew.Err()
}

// credentialCmd is the hidden agx credential (spec 10): the live-login
// helper the agent runs. It prints the credential alone on stdout (the
// agent reads it); --json carries only the run and login names.
func (c *cliCtx) credentialCmd() *cobra.Command {
	return fields(c.newCmd("credential", "Print the live-login credential (run by the agent)", CostExec,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			cr, err := c.App().Credential(cmd.Context())
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(cr)
			if err != nil {
				return render.Output{}, err
			}
			return render.Output{Data: data, Text: func(w io.Writer) error {
				_, err := io.WriteString(w, cr.Token.Reveal()+"\n")
				return err
			}}, nil
		}), "data.run_id", "data.login")
}

// liveColumns are agx live list's table columns.
var liveColumns = []render.Column{
	{Header: "RUN", Key: "id"}, {Header: "AGENT", Key: "agent"}, {Header: "PROFILE", Key: "profile"},
	{Header: "STARTED ON", Key: "start_login"}, {Header: "NEXT", Key: "next"}, {Header: "IN USE", Key: "in_use"},
	{Header: "LIVENESS", Key: "liveness", Tone: livenessTones},
}

// liveFields are the --field paths of one live run.
var liveFields = []string{"id", "dir", "agent", "profile", "home", "folder", "start_login", "chosen_login", "in_use", "in_use_at", "next", "liveness", "reason"}

// prefixed returns each key with prefix ("data.*." + "id").
func prefixed(prefix string, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = prefix + k
	}
	return out
}

// liveCmd is agx live (spec 10, 13.3).
func (c *cliCtx) liveCmd() *cobra.Command {
	var agent string
	list := c.newCmd("list", "Running sessions started with --live-login", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			runs, err := c.App().LiveList(cmd.Context(), agentio.ID(agent))
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(runs)
			return render.Output{Data: data, Table: liveColumns}, err
		})
	list.Flags().StringVar(&agent, "agent", "", "only this agent's runs")
	fields(list, prefixed("data.*.", liveFields)...)

	var showID string
	show := c.newCmd("show", "One run's live state", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			r, err := c.App().LiveShow(cmd.Context(), showID)
			if err != nil {
				return render.Output{}, err
			}
			return liveOutput(r)
		})
	show.Flags().StringVar(&showID, "id", "", "the run id")
	fields(show, prefixed("data.", liveFields)...)

	var uo app.LiveUseOptions
	use := c.newCmd("use", "Choose the login of a live run", CostWrite,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			r, err := c.App().LiveUse(cmd.Context(), uo)
			if err != nil {
				return render.Output{}, err
			}
			return liveOutput(r)
		})
	use.Example = "  agx live use --id 20261114-101500-k3x9 --login alice\n  agx live use --id 20261114-101500-k3x9 --start-login"
	uf := use.Flags()
	uf.StringVar(&uo.ID, "id", "", "the run id")
	uf.StringVar(&uo.Login, "login", "", "the login the agent gets at its next refresh")
	uf.BoolVar(&uo.StartLogin, "start-login", false, "back to the login the run started with")
	fields(use, prefixed("data.", liveFields)...)

	return group("live", "Running sessions started with --live-login", list, show, use)
}

// liveOutput renders one live run as name/value lines.
func liveOutput(r app.LiveRun) (render.Output, error) {
	data, err := render.Encode(r)
	if err != nil {
		return render.Output{}, err
	}
	return render.Output{Data: data, Text: func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		ew := textout.New(tw)
		for _, kv := range [][2]string{
			{"run", r.ID}, {"agent", r.Agent}, {"profile", r.Profile}, {"folder", r.Folder},
			{"started on", r.StartLogin}, {"chosen", r.ChosenLogin}, {"next", r.Next}, {"in use", r.InUse},
			{"liveness", r.Liveness + " (" + r.Reason + ")"},
		} {
			ew.Line(kv[0] + "\t" + orDash(kv[1]))
		}
		if ew.Err() != nil {
			return ew.Err()
		}
		return tw.Flush()
	}}, nil
}
