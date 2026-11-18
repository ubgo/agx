package cli

import (
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/ubgo/voltkit/versioncmd"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/render"
)

// HelpFlag is one flag in help --json.
type HelpFlag struct {
	Name       string `json:"name"`
	Short      string `json:"short,omitempty"`
	Type       string `json:"type"`
	TakesValue bool   `json:"takes_value"`
	Repeatable bool   `json:"repeatable"`
	Required   bool   `json:"required"`
	Hidden     bool   `json:"hidden,omitempty"`
	Default    string `json:"default,omitempty"`
	Help       string `json:"help"`
}

// HelpCommand is one command in help --json, with its subcommands.
type HelpCommand struct {
	Path      string        `json:"path"`
	Name      string        `json:"name"`
	Short     string        `json:"short"`
	Cost      string        `json:"cost,omitempty"`
	Launching bool          `json:"launching,omitempty"`
	Hidden    bool          `json:"hidden,omitempty"`
	Fields    []string      `json:"fields,omitempty"`
	Flags     []HelpFlag    `json:"flags"`
	Commands  []HelpCommand `json:"commands,omitempty"`
}

// requiredAnnotation is cobra's marker for a required flag.
const requiredAnnotation = cobra.BashCompOneRequiredFlag

// helpCmd is agx help; with --json it prints the command tree generated
// from the declarations that build the parser (spec 5.1 rule 10, 14.3).
func (c *cliCtx) helpCmd() *cobra.Command {
	cmd := c.newCmd("help", "Help; --json prints the command tree", CostRead, nil)
	cmd.RunE = c.do(func(cmd *cobra.Command, args []string) (render.Output, error) {
		out := render.Output{Command: "help"}
		if !c.g.JSON && len(c.g.Fields) == 0 {
			out.Text = func(w io.Writer) error { return cmd.Root().Help() }
			return out, nil
		}
		data, err := render.Encode(HelpTree(cmd.Root()))
		out.Data = data
		return out, err
	})
	return cmd
}

// HelpTree describes cmd and every subcommand.
func HelpTree(cmd *cobra.Command) HelpCommand {
	h := HelpCommand{
		Path: strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), Name)), Name: cmd.Name(), Short: cmd.Short,
		Cost: cmd.Annotations[annCost], Launching: cmd.Annotations[annLaunching] == "true", Hidden: cmd.Hidden,
		Flags: []HelpFlag{},
	}
	h.Fields = declaredFields(cmd)
	add := func(f *pflag.Flag) {
		_, req := f.Annotations[requiredAnnotation]
		h.Flags = append(h.Flags, HelpFlag{
			Name: f.Name, Short: f.Shorthand, Type: f.Value.Type(), TakesValue: f.NoOptDefVal == "",
			Repeatable: flagRepeatable(f), Required: req, Hidden: f.Hidden, Default: defaultText(f), Help: f.Usage,
		})
	}
	if cmd.HasParent() {
		cmd.LocalFlags().VisitAll(add)
	} else {
		cmd.PersistentFlags().VisitAll(add)
	}
	subs := cmd.Commands()
	slices.SortStableFunc(subs, func(a, b *cobra.Command) int { return strings.Compare(a.Name(), b.Name()) })
	for _, s := range subs {
		h.Commands = append(h.Commands, HelpTree(s))
	}
	return h
}

// defaultText is a flag's default unless it is the type's zero value.
func defaultText(f *pflag.Flag) string {
	switch f.DefValue {
	case "", "false", "[]", "0":
		return ""
	}
	return f.DefValue
}

// versionCmd is agx version: version, build provenance and components.
func (c *cliCtx) versionCmd() *cobra.Command {
	return c.newCmd("version", "Version, build provenance, components", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		info := versioncmd.Collect(Name)
		data, err := render.Encode(info)
		if err != nil {
			return render.Output{}, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, err.Error())
		}
		return render.Output{Data: data, Text: func(w io.Writer) error { return versioncmd.WriteText(w, info) }}, nil
	})
}
