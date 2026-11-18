package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// projectCmd is agx project (spec 13.6).
func (c *cliCtx) projectCmd() *cobra.Command {
	return group("project", "Folder bindings per agent",
		c.projectListCmd(), c.projectShowCmd(), c.projectUseCmd(), c.projectRemoveCmd(),
	)
}

// projectListCmd is agx project list.
func (c *cliCtx) projectListCmd() *cobra.Command {
	cmd := c.newCmd("list", "Every binding, per agent", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		rows, err := c.App().ProjectList()
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		return render.Output{Data: data, Table: []render.Column{{Header: "DIR", Key: "dir"}, {Header: "AGENT", Key: "agent"}, {Header: "PROFILE", Key: "profile"}, {Header: "LOGIN", Key: "login"}},
			Empty: "no project bindings — bind this folder with `agx project use --profile PROFILE`"}, err
	})
	return fields(cmd, "data.*.dir", "data.*.agent", "data.*.profile", "data.*.login")
}

// projectShowCmd is agx project show.
func (c *cliCtx) projectShowCmd() *cobra.Command {
	var o app.ProjectShowOptions
	cmd := c.newCmd("show", "What a run here would do, every decision with its source", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, decisions, err := c.App().ProjectShow(o)
		if err != nil {
			return render.Output{}, err
		}
		for _, n := range res.Notes {
			c.note(n)
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Decisions: decisions, Text: func(w io.Writer) error {
			lines := make([]render.DecisionLine, len(decisions))
			for i, d := range decisions {
				lines[i] = render.DecisionLine{Name: d.Name, Value: d.Value, Source: d.Source}
			}
			return render.WriteDecisions(w, c.env.Width, lines, c.style())
		}}, err
	})
	cmd.Example = "  agx project show\n  agx project show --dir ~/projects/client --agent codex"
	f := cmd.Flags()
	f.StringVar(&o.Dir, "dir", "", "the folder (default: the current folder)")
	f.StringVar((*string)(&o.Agent), "agent", "", "as run --agent")
	f.StringVarP(&o.Profile, "profile", "p", "", "as run --profile")
	f.StringVar(&o.Login, "login", "", "as run --login")
	return fields(cmd, "data.dir", "data.agent", "data.profile", "data.home", "data.login", "data.binding", "data.notes", "decisions")
}

// projectUseCmd is agx project use.
func (c *cliCtx) projectUseCmd() *cobra.Command {
	var o app.ProjectUseOptions
	cmd := c.newCmd("use", "Bind a folder for one agent", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProjectUse(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			var parts []string
			if res.Profile != "" {
				parts = append(parts, "profile "+res.Profile)
			}
			switch res.Login {
			case "":
			case app.HomeLoginText:
				parts = append(parts, res.Login) // already reads "home login"
			default:
				parts = append(parts, "login "+res.Login)
			}
			ew := textout.New(w)
			ew.Line(fmt.Sprintf("%s: %s %s", res.Dir, res.Agent, strings.Join(parts, ", ")))
			return ew.Err()
		}}, err
	})
	cmd.Example = "  agx project use --profile work\n  agx project use --dir ~/projects/client --agent codex --login gem-work\n  agx project use --agent claude --home-login"
	f := cmd.Flags()
	f.StringVar(&o.Dir, "dir", "", "the folder (default: the current folder)")
	f.StringVarP(&o.Profile, "profile", "p", "", "bind this profile (its agent is the binding's agent)")
	f.StringVar((*string)(&o.Agent), "agent", "", "the agent (needed with --login or --home-login alone)")
	f.StringVar(&o.Login, "login", "", "bind this login")
	f.BoolVar(&o.HomeLogin, "home-login", false, "bind the home's own login")
	return fields(cmd, "data.dir", "data.agent", "data.profile", "data.login")
}

// projectRemoveCmd is agx project remove.
func (c *cliCtx) projectRemoveCmd() *cobra.Command {
	var dir string
	var agent agentio.ID
	cmd := c.newCmd("remove", "Remove a folder's binding for one agent or all", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ProjectRemove(cmd.Context(), dir, agent)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ids := make([]string, len(res.Agents))
			for i, id := range res.Agents {
				ids[i] = string(id)
			}
			ew := textout.New(w)
			ew.Line(fmt.Sprintf("removed the %s binding of %s", strings.Join(ids, ", "), res.Dir))
			return ew.Err()
		}}, err
	})
	cmd.Example = "  agx project remove\n  agx project remove --dir ~/projects/client --agent codex"
	f := cmd.Flags()
	f.StringVar(&dir, "dir", "", "the folder (default: the current folder)")
	f.StringVar((*string)(&agent), "agent", "", "only this agent's binding (default: all)")
	return fields(cmd, "data.dir", "data.agents")
}
