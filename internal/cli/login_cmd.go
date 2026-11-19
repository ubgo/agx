package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

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
	loginInfoFields   = []string{"data.name", "data.agents", "data.kind", "data.source", "data.source_type", "data.account", "data.provider", "data.users"}
	loginListFields   = []string{"data.*.name", "data.*.agents", "data.*.kind", "data.*.source", "data.*.source_type", "data.*.account", "data.*.provider", "data.*.users"}
	loginCheckFields  = []string{"data.*.name", "data.*.kind", "data.*.source", "data.*.resolve", "data.*.probe", "data.*.agent", "data.*.detail", "data.*.windows"}
	loginUseFields    = []string{"data.login", "data.home_login", "data.target", "data.name", "data.line"}
	loginRemoveFields = []string{"data.name", "data.cleared"}
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
		rows, err := c.App().LoginList(o)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(rows)
		return render.Output{Data: data, Table: []render.Column{
			{Header: "NAME", Key: "name"}, {Header: "AGENTS", Key: "agents"}, {Header: "KIND", Key: "kind"},
			{Header: "SOURCE", Key: "source"}, {Header: "ACCOUNT", Key: "account"},
		}, Empty: "no logins — add one with `agx login add --name NAME --agent AGENT` (see `agx login add --help`)"}, err
	})
	cmd.Flags().StringVar((*string)(&o.Agent), "agent", "", "only logins serving this agent")
	return fields(cmd, loginListFields...)
}

// loginShowCmd is agx login show.
func (c *cliCtx) loginShowCmd() *cobra.Command {
	var o app.LoginShowOptions
	cmd := c.newCmd("show", "One login: kind, source, account, agents and what uses it", CostRead, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		info, err := c.App().LoginShow(o)
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
	)
	cmd := c.newCmd("add", "Add a login", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		o.Agents = agentIDs(agents)
		o.Source.RunSetup = c.setupRunner()
		ch, err := c.App().LoginAdd(cmd.Context(), o)
		return loginChangeOutput("added", ch, err)
	})
	cmd.Example = "  agx login add --name personal --agent claude --kind oauth-token --token-ref '${gopass:personal/claude-token}'\n" +
		"  pbpaste | agx login add --name pasted --agent claude --kind oauth-token --stdin\n" +
		"  agx login add --name carol --agent claude --kind oauth-token --login-setup"
	f := cmd.Flags()
	f.StringVar(&o.Name, "name", "", "the new login's name")
	f.StringArrayVar(&agents, "agent", nil, "an agent it serves (repeat)")
	f.StringVar(&o.Kind, "kind", "", "oauth-token, api-key or api-key-helper (default: the first agent's default kind)")
	f.StringVar(&o.Account, "account", "", "a display label, such as the account email")
	f.StringVar(&o.Provider, "provider", "", "the model provider, for agents with a key per provider (pi)")
	addSourceFlags(cmd, &o.Source)
	return fields(cmd, loginInfoFields...)
}

// loginEditCmd is agx login edit.
func (c *cliCtx) loginEditCmd() *cobra.Command {
	var (
		o                 app.LoginEditOptions
		agents            []string
		account, provider string
	)
	cmd := c.newCmd("edit", "Rotate or change a login", CostWrite, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		o.Agents = agentIDs(agents)
		if cmd.Flags().Changed("account") {
			o.Account = &account
		}
		if cmd.Flags().Changed("provider") {
			o.Provider = &provider
		}
		o.Source.RunSetup = c.setupRunner()
		ch, err := c.App().LoginEdit(cmd.Context(), o)
		return loginChangeOutput("changed", ch, err)
	})
	cmd.Example = "  pbpaste | agx login edit --id pasted --stdin\n  agx login edit --id personal --account me@example.com"
	f := cmd.Flags()
	f.StringVar(&o.ID, "id", "", "the login to change")
	f.StringArrayVar(&agents, "agent", nil, "replace the agents it serves (repeat)")
	f.StringVar(&o.Kind, "kind", "", "change the kind")
	f.StringVar(&account, "account", "", "set the display label (empty clears it)")
	f.StringVar(&provider, "provider", "", "set the model provider (empty clears it)")
	addSourceFlags(cmd, &o.Source)
	return fields(cmd, loginInfoFields...)
}

// agentIDs converts flag values to agent ids.
func agentIDs(in []string) []agentio.ID {
	out := make([]agentio.ID, len(in))
	for i, s := range in {
		out[i] = agentio.ID(s)
	}
	return out
}

// loginChangeOutput renders add and edit.
func loginChangeOutput(verb string, ch app.LoginChange, err error) (render.Output, error) {
	out := render.Output{Decisions: ch.Decisions, Warnings: ch.Warnings}
	if err != nil {
		return out, err
	}
	data, err := render.Encode(ch.Info)
	out.Data = data
	out.Text = func(w io.Writer) error {
		ew := textout.New(w)
		ew.Line(fmt.Sprintf("%s login %s (%s, %s)", verb, ch.Info.Name, ch.Info.Kind, ch.Info.Source))
		return ew.Err()
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
			{Header: "RESOLVE", Key: "resolve", Tone: checkTones}, {Header: "PROBE", Key: "probe", Tone: checkTones}, {Header: "DETAIL", Key: "detail"},
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
		id     string
		reveal bool
	)
	cmd := c.newCmd("token", "Print one login's token for a pipe or an apiKeyHelper (a terminal needs --reveal)", CostExec, func(cmd *cobra.Command, _ []string) (render.Output, error) {
		out, err := c.loginToken(cmd, id, reveal)
		return out, bareError(err)
	})
	cmd.Long = "Resolve a login's token from its source (a stored value, ${gopass:…}, $(…), ${VAR} or a plain value) and print only the value and a newline on stdout. It reads what is stored or referenced and never refreshes a token; a $(…) or gopass reference runs that local command. Stdout must be a pipe or a file unless --reveal is given. An api-key-helper login has no token and is refused. Every error is one line on stderr, without hint lines, and stdout stays empty."
	cmd.Example = "  agx login token --id work-key | pbcopy\n" +
		"  agx login add --name console --agent claude --kind api-key-helper --helper-command 'agx login token --id work-key'"
	f := cmd.Flags()
	f.StringVar(&id, "id", "", "the login name")
	f.BoolVar(&reveal, "reveal", false, "print the token even when stdout is a terminal")
	return cmd
}

// loginToken is agx login token's body; loginTokenCmd makes its error one
// line.
func (c *cliCtx) loginToken(cmd *cobra.Command, id string, reveal bool) (render.Output, error) {
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
	tok, err := c.App().LoginToken(cmd.Context(), id)
	if err != nil {
		return render.Output{}, err
	}
	return render.Output{Text: func(w io.Writer) error {
		_, err := io.WriteString(w, tok.Reveal()+"\n")
		return err
	}}, nil
}
