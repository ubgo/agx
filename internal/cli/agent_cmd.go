package cli

import (
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/textout"
)

// Fields of agent list's data (help --json).
var agentListFields = []string{"data.*.id", "data.*.name", "data.*.kind", "data.*.binary", "data.*.path", "data.*.version", "data.*.method", "data.*.tested", "data.*.compat", "data.*.capabilities"}

// Fields of agent show's data (help --json).
var agentShowFields = []string{"data.id", "data.name", "data.kind", "data.binary", "data.path", "data.version", "data.method", "data.tested", "data.compat", "data.capabilities", "data.shadowed", "data.autonomy", "data.token_kinds", "data.capability_detail", "data.vendor_version", "data.vendor_error"}

// agentCmd is agx agent: what each agent can do (spec 3.2, 13.8).
func (c *cliCtx) agentCmd() *cobra.Command {
	return group("agent", "Known agents and their capabilities", c.agentListCmd(), c.agentShowCmd())
}

// agentListCmd is agx agent list.
func (c *cliCtx) agentListCmd() *cobra.Command {
	cmd := c.newCmd("list", "Known agents: kind, binary, version, tested range, capabilities", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			list, err := c.App().AgentList()
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(list)
			return render.Output{Data: data, Table: []render.Column{
				{Header: "AGENT", Key: "id"}, {Header: "KIND", Key: "kind"}, {Header: "BINARY", Key: "binary"},
				{Header: "VERSION", Key: "version"}, {Header: "TESTED", Key: "tested"}, {Header: "COMPAT", Key: "compat", Tone: compatTones},
				{Header: "CAPABILITIES", Key: "capabilities"},
			}}, err
		})
	cmd.Long = "Every agent agx knows: the agentio built-ins and the agents defined in config. The version comes from install metadata; the binary is never run. Capabilities are agentio's, generated, never a hand table."
	return fields(cmd, agentListFields...)
}

// agentShowCmd is agx agent show.
func (c *cliCtx) agentShowCmd() *cobra.Command {
	var o app.AgentShowOptions
	cmd := c.newCmd("show", "One agent with each capability and the reason when missing", CostRead,
		func(cmd *cobra.Command, args []string) (render.Output, error) {
			d, err := c.App().AgentShow(cmd.Context(), o)
			if err != nil {
				return render.Output{}, err
			}
			data, err := render.Encode(d)
			return render.Output{Data: data, Text: func(w io.Writer) error { return writeAgentShow(w, d) }}, err
		})
	cmd.Example = "  agx agent show --id codex\n  agx agent show --id claude --vendor-checks"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the agent id (agx agent list)")
	f.BoolVar(&o.VendorChecks, "vendor-checks", false, "also run the binary's --version (cost X)")
	return fields(cmd, agentShowFields...)
}

// writeAgentShow prints one agent: its facts, then a capability table.
func writeAgentShow(w io.Writer, d app.AgentDetail) error {
	ew := textout.New(w)
	kv := func(k, v string) {
		if v == "" {
			v = render.Dash
		}
		ew.Line(k + ": " + v)
	}
	kv("agent", string(d.ID)+" ("+d.Name+", "+d.Kind+")")
	kv("binary", d.Binary)
	kv("path", d.Path)
	kv("version", d.Version)
	kv("method", d.Method)
	kv("tested", d.Tested)
	kv("compat", d.Compat)
	if len(d.Shadowed) > 0 {
		kv("shadowed", strings.Join(d.Shadowed, ", "))
	}
	kv("autonomy", strings.Join(d.Autonomy, ", "))
	kv("token kinds", strings.Join(d.TokenKinds, ", "))
	if d.VendorVersion != "" {
		kv("--version", d.VendorVersion)
	}
	if d.VendorError != "" {
		kv("--version error", d.VendorError)
	}
	ew.Line("")
	if ew.Err() != nil {
		return ew.Err()
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	tew := textout.New(tw)
	tew.Line("CAPABILITY\tSUPPORTED\tCOST\tREASON")
	for _, ci := range d.CapabilityDetail {
		yes := "no"
		if ci.Supported {
			yes = "yes"
		}
		reason := ci.Reason
		if reason == "" {
			reason = render.Dash
		}
		tew.Line(ci.Name + "\t" + yes + "\t" + ci.Cost + "\t" + reason)
	}
	if tew.Err() != nil {
		return tew.Err()
	}
	return tw.Flush()
}
