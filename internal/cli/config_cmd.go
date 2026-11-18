package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// configCmd is agx config (spec 13.8).
func (c *cliCtx) configCmd() *cobra.Command {
	return group("config", "agx's own config file",
		c.configInitCmd(), c.configShowCmd(), c.configPathCmd(), c.configCheckCmd(), c.configSetCmd(),
	)
}

// configInitCmd is agx config init.
func (c *cliCtx) configInitCmd() *cobra.Command {
	var o app.ConfigInitOptions
	cmd := c.newCmd("init", "Write a commented starter config.jsonc", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ConfigInit(cmd.Context(), o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ew := textout.New(w)
			if !res.Written {
				ew.Str(res.Content)
				ew.Line("(--dry-run: " + res.File + " not written)")
				return ew.Err()
			}
			ew.Line(fmt.Sprintf("wrote %s (%s)", res.File, countOf(len(res.Profiles), "profile", "profiles")))
			return ew.Err()
		}}, err
	})
	cmd.Example = "  agx config init --default-agent claude --discovered\n  agx config init --dry-run"
	f := cmd.Flags()
	f.StringVar((*string)(&o.DefaultAgent), "default-agent", "", "write default_agent (at a terminal without it: asked)")
	f.BoolVar(&o.Discovered, "discovered", false, "add every discovered home as a profile (at a terminal without it: offered)")
	f.BoolVar(&o.DryRun, "dry-run", false, "print the file; write nothing")
	return fields(cmd, "data.file", "data.default_agent", "data.profiles", "data.content", "data.written")
}

// configShowCmd is agx config show.
func (c *cliCtx) configShowCmd() *cobra.Command {
	cmd := c.newCmd("show", "The resolved config and each value's source", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		rows, err := c.App().ConfigShow()
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		return render.Output{Data: data, Text: func(w io.Writer) error { return valuesText(w, rows, c.style()) }}, err
	})
	return fields(cmd, "data.*.key", "data.*.value", "data.*.source")
}

// configPathCmd is agx config path.
func (c *cliCtx) configPathCmd() *cobra.Command {
	cmd := c.newCmd("path", "The config file path", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res := c.App().ConfigPath()
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ew := textout.New(w)
			ew.Line(res.File)
			return ew.Err()
		}}, err
	})
	return fields(cmd, "data.file", "data.dir", "data.exists", "data.source")
}

// configCheckCmd is agx config check.
func (c *cliCtx) configCheckCmd() *cobra.Command {
	cmd := c.newCmd("check", "Load, validate and cross-check the config; lists every problem", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ConfigCheck()
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ew := textout.New(w)
			if !res.Exists {
				ew.Line(res.File + ": no config file (agx config init)")
				return ew.Err()
			}
			ew.Line(fmt.Sprintf("%s: ok (%s, %s, %s, %s, %s)", res.File,
				countOf(res.Agents, "agent", "agents"), countOf(res.Profiles, "profile", "profiles"), countOf(res.Logins, "login", "logins"),
				countOf(res.Projects, "project", "projects"), countOf(res.Aliases, "alias", "aliases")))
			return ew.Err()
		}}, err
	})
	return fields(cmd, "data.file", "data.exists", "data.agents", "data.profiles", "data.logins", "data.projects", "data.aliases")
}

// configSetCmd is agx config set.
func (c *cliCtx) configSetCmd() *cobra.Command {
	var key, value string
	cmd := c.newCmd("set", "Set a scalar key, keeping comments", CostWrite, func(cmd *cobra.Command, args []string) (render.Output, error) {
		res, err := c.App().ConfigSet(cmd.Context(), key, value)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(res)
		return render.Output{Data: data, Text: func(w io.Writer) error {
			ew := textout.New(w)
			ew.Line(res.Key + " = " + res.Value)
			return ew.Err()
		}}, err
	})
	cmd.Example = "  agx config set --key default_agent --value claude\n  agx config set --key sessions.root --value ~/sessions\n  agx config set --key profiles.work.model --value opus"
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "a dotted scalar key (default_agent, sessions.root, profiles.NAME.model, …)")
	f.StringVar(&value, "value", "", "the value (true/false for switches, digits for numbers)")
	return fields(cmd, "data.key", "data.value", "data.type")
}
