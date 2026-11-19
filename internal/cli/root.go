// Package cli is agx's command line: cobra declarations only. Each
// command parses its flags into the typed options of an internal/app use
// case, calls it, and hands the typed result to internal/render. No
// command logic lives here (spec 16.1), and this is the only package that
// imports cobra (internal/archtest).
//
// Rules every command follows (spec section 5): every input is a named
// flag (a bare word is a usage error naming the flag form); "--" ends
// agx's flags; the action is the command and flags only name things.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/internal/render"
)

// envColumns is the conventional terminal-width variable, used when
// stdout is not a terminal (a pipe, a test).
const envColumns = "COLUMNS"

// Name is the binary name used in messages and help.
const Name = "agx"

// Env is the process the CLI runs in, injected so tests run the command
// tree in-process against fixtures, and main wires the real one.
type Env struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Host is the agentio host (real: agentio.HostFromOS).
	Host agentio.Host
	// Lookup reads agx's own environment.
	Lookup func(string) (string, bool)
	// Cwd is the current folder.
	Cwd string
	// TTY is true when stdin and stderr are terminals (pickers allowed).
	TTY bool
	// StderrTTY is true when stderr alone is a terminal: --color auto
	// colours agx's stderr diagnostics only then (color.go).
	StderrTTY bool
	// StdoutTTY is true when stdout is a terminal: --color auto colours
	// the human output (bold headers, coloured statuses) only then.
	StdoutTTY bool
	// StdinFD is stdin's descriptor for hidden input; -1 when not a file.
	StdinFD int
	// Self is the running agx binary path.
	Self string
	// Width is the human line width: the terminal's when stdout is one,
	// else COLUMNS, else 0 (render.DefaultWidth).
	Width int
}

// Annotation keys on cobra commands, read by help --json and Main.
const (
	annCost      = "agx.cost"
	annLaunching = "agx.launching"
	annFields    = "agx.fields"
)

// Cost classes (spec 14.4), shown in help --json.
const (
	CostRead    = "R"
	CostNetwork = "N"
	CostQuota   = "Q"
	CostExec    = "X"
	CostWrite   = "W"
)

// dateFormatFlag is the global --date-format flag's name.
const dateFormatFlag = "date-format"

// globals are the flags every command takes (spec 13 "Global flags").
type globals struct {
	JSON    bool
	Fields  []string
	Quiet   bool
	Color   string
	Config  string
	Timeout time.Duration
	// DateFormat is --date-format: how human dates and times read.
	DateFormat string
}

// cliCtx is shared by every command of one invocation.
type cliCtx struct {
	env Env
	g   globals
	a   *app.App
	// root is the command tree, for validating alias values.
	root *cobra.Command
}

// App returns the app, built on first use (after flags are parsed, so
// --config and --timeout apply).
func (c *cliCtx) App() *app.App {
	if c.a == nil {
		var pk picker.Picker
		if c.env.TTY {
			pk = picker.NewLine(c.env.Stdin, c.env.Stderr, c.env.StdinFD)
		}
		c.a = app.New(app.Deps{
			Host: c.env.Host, Lookup: c.env.Lookup, Cwd: c.env.Cwd, ConfigFlag: c.g.Config,
			Picker: pk, Stdin: c.env.Stdin, Self: c.env.Self, Timeout: c.g.Timeout,
			AliasCheck: c.aliasCheck,
		})
	}
	return c.a
}

// opts are the render options from the globals.
func (c *cliCtx) opts() render.Options {
	return render.Options{JSON: c.g.JSON, Fields: c.g.Fields, Quiet: c.g.Quiet, Width: c.env.Width,
		Color: colorOn(c.g.Color, c.env.Lookup, c.env.StderrTTY), OutColor: c.style().On(), Date: c.dateFormat()}
}

// dateFormat is how human dates read outside agx usage (table timestamp
// cells, conversation show): --date-format, else config date_format when
// this command loaded the config, else the iso preset, the look these
// screens had before the format existed. The flag is checked in do and
// the config value when it loads, so a parse failure keeps the preset.
func (c *cliCtx) dateFormat() datefmt.Format {
	spec := c.g.DateFormat
	if spec == "" && c.a != nil {
		if l, err := c.a.Config(); err == nil && l != nil {
			spec = l.Config.DateFormat
		}
	}
	if spec != "" {
		if f, err := datefmt.Parse(spec); err == nil {
			return f
		}
	}
	return datefmt.Preset(datefmt.PresetISO)
}

// checkDateFormat refuses a --date-format that does not parse (exit 2),
// before the command runs; a flag given empty is refused too, never read
// as "not given".
func (c *cliCtx) checkDateFormat(cmd *cobra.Command) error {
	if fl := cmd.Flag(dateFormatFlag); fl == nil || !fl.Changed {
		return nil
	}
	if _, err := datefmt.Parse(c.g.DateFormat); err != nil {
		return apperr.Usage(fmt.Sprintf("--date-format %q: %s", c.g.DateFormat, err))
	}
	return nil
}

// style paints a command's human stdout output: on when --color always,
// or --color auto with stdout a terminal and no NO_COLOR (color.go).
func (c *cliCtx) style() render.Style {
	return render.NewStyle(colorOn(c.g.Color, c.env.Lookup, c.env.StdoutTTY))
}

// note prints one stderr note unless --quiet (principle 2).
func (c *cliCtx) note(msg string) {
	if !c.g.Quiet {
		_, _ = io.WriteString(c.env.Stderr, Name+": "+msg+"\n") // a failed stderr write has nowhere to go
	}
}

// printed marks an error already shown to the user, so Main only maps it
// to the exit code.
type printed struct{ err error }

func (p printed) Error() string { return p.err.Error() }
func (p printed) Unwrap() error { return p.err }

// agentExit is the agent's own exit code (Windows, where agx stays the
// parent), passed through as agx's exit code.
type agentExit struct{ code int }

func (e agentExit) Error() string { return "agent exited" }

// runFunc is a command body: it returns typed output or an error.
type runFunc func(cmd *cobra.Command, args []string) (render.Output, error)

// do wraps a command body: it prints the output (or the error) per the
// global flags and returns a printed error for the exit code.
func (c *cliCtx) do(fn runFunc) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := checkBareWords(cmd, args); err != nil {
			return c.fail(cmd, err)
		}
		if err := c.checkDateFormat(cmd); err != nil {
			return c.fail(cmd, err)
		}
		out, err := fn(cmd, args)
		if out.Command == "" {
			out.Command = commandPath(cmd)
		}
		var ae agentExit
		if errors.As(err, &ae) {
			return ae
		}
		opts := c.opts()
		opts.Declared = declaredFields(cmd)
		if perr := render.Print(c.env.Stdout, c.env.Stderr, out, err, opts); perr != nil {
			render.PrintError(c.env.Stdout, c.env.Stderr, out.Command, perr, render.Options{JSON: false, Color: c.opts().Color})
			return printed{perr}
		}
		if err != nil {
			return printed{err}
		}
		return nil
	}
}

// fail prints err for cmd and returns it as printed.
func (c *cliCtx) fail(cmd *cobra.Command, err error) error {
	render.PrintError(c.env.Stdout, c.env.Stderr, commandPath(cmd), err, c.opts())
	return printed{err}
}

// commandPath is "profile list" for `agx profile list`.
func commandPath(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), Name+" ")
}

// checkBareWords refuses positional words before "--" (spec 5.1 rule 1).
func checkBareWords(cmd *cobra.Command, args []string) error {
	n := len(args)
	if d := cmd.ArgsLenAtDash(); d >= 0 {
		n = d
	}
	if n > 0 {
		return apperr.Usage("unexpected argument " + quote(args[0]) + "; every input is a named flag (see agx " + commandPath(cmd) + " --help)")
	}
	return nil
}

// rawArgs are the args after "--" (passed to the agent verbatim).
func rawArgs(cmd *cobra.Command, args []string) []string {
	if d := cmd.ArgsLenAtDash(); d >= 0 {
		return args[d:]
	}
	return nil
}

// quote wraps s in double quotes for messages.
func quote(s string) string { return `"` + s + `"` }

// countOf is "1 profile" / "2 profiles": a count with its noun in the right
// number. Text output uses it wherever a count can be 1.
func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// newCmd builds a leaf command with its cost class and body.
func (c *cliCtx) newCmd(use, short, cost string, fn runFunc) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short,
		Annotations: map[string]string{annCost: cost},
		Args:        cobra.ArbitraryArgs,
		RunE:        c.do(fn),
	}
}

// group builds a parent command (profile, login, …): bare it prints help.
func group(use, short string, subs ...*cobra.Command) *cobra.Command {
	g := &cobra.Command{Use: use, Short: short, Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return apperr.Usage("unknown command " + quote(args[0]) + " for agx " + commandPath(cmd))
			}
			return cmd.Help()
		}}
	g.AddCommand(subs...)
	return g
}

// launching marks a launching command (exit codes 125/126/127, 14.2).
func launching(cmd *cobra.Command) *cobra.Command {
	cmd.Annotations[annLaunching] = "true"
	return cmd
}

// fields records the --field paths of a command's data for help --json
// and for checking a --field path past an empty list (declaredFields).
func fields(cmd *cobra.Command, paths ...string) *cobra.Command {
	cmd.Annotations[annFields] = strings.Join(paths, ",")
	return cmd
}

// declaredFields are the paths fields recorded on cmd; nil when none.
func declaredFields(cmd *cobra.Command) []string {
	if f := cmd.Annotations[annFields]; f != "" {
		return strings.Split(f, ",")
	}
	return nil
}

// NewRoot builds the command tree.
func NewRoot(env Env) (*cobra.Command, *cliCtx) {
	c := &cliCtx{env: env}
	root := &cobra.Command{
		Use:           Name,
		Short:         "One CLI for every coding agent and every account",
		Long:          "agx runs Claude Code, Codex, Gemini CLI, pi and config-defined agents across every account on the machine: usage, launch and resume, logins, project bindings, session folders and conversations.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return c.fail(cmd, apperr.Usage("unknown command "+quote(args[0])+"; see agx --help"))
			}
			return cmd.Help()
		},
	}
	c.root = root
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	pf := root.PersistentFlags()
	pf.BoolVar(&c.g.JSON, "json", false, "print the JSON envelope")
	pf.StringArrayVar(&c.g.Fields, "field", nil, "keep only this path of the envelope (repeat); picks table columns with data.*.KEY")
	pf.BoolVar(&c.g.Quiet, "quiet", false, "no stderr notes")
	c.g.Color = ColorAuto
	pf.Var(&colorFlag{mode: &c.g.Color}, "color", "colour: auto (each stream that is a terminal, unless NO_COLOR is set), always or never; tables, usage bars and agx's error and warning lines")
	pf.StringVar(&c.g.Config, "config", "", "config file (default: $AGX_CONFIG_DIR, else $AGX_HOME, else ~/.agx)/config.jsonc")
	pf.DurationVar(&c.g.Timeout, "timeout", app.DefaultTimeout, "per-request network timeout")
	pf.StringVar(&c.g.DateFormat, dateFormatFlag, "", "how dates and times read: tokens such as \"ddd D MMM HH:mm\" or a preset ("+strings.Join(datefmt.Presets(), ", ")+"); default: config usage.date_format (agx usage), else date_format, else long for usage and iso elsewhere")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		// A flag Value that refuses with its own agx error (a repeated
		// single-value flag) keeps it, so the user reads "pick one" and not
		// pflag's "invalid argument … for … flag:" wrapper around it.
		var own *apperr.Error
		if errors.As(err, &own) {
			return c.fail(cmd, own)
		}
		return c.fail(cmd, apperr.Usage(err.Error()))
	})
	root.AddCommand(c.commands()...)
	root.SetHelpCommand(c.helpCmd())
	for _, h := range []*cobra.Command{c.launchCmd(), c.planCmd(), c.credentialCmd()} {
		h.Hidden = true
		root.AddCommand(h)
	}
	// Last, once every flag is declared: a single-value flag given twice
	// is refused by the parser itself (spec 5.1 rule 9).
	onceFlags(root)
	return root, c
}

// commands is every top-level command, in help order (spec section 13).
func (c *cliCtx) commands() []*cobra.Command {
	return []*cobra.Command{
		c.usageCmd(),
		c.runCmd(), c.newSessionCmd(), c.resumeCmd(), c.whoamiCmd(),
		c.conversationCmd(), c.liveCmd(),
		c.profileCmd(), c.loginCmd(), c.projectCmd(), c.sessionCmd(),
		c.agentCmd(), c.doctorCmd(), c.cleanCmd(), c.configCmd(), c.aliasCmd(),
		c.shellInitCmd(), c.completionCmd(), c.skillCmd(), c.versionCmd(),
	}
}

// Main runs agx with env and returns the process exit code.
func Main(ctx context.Context, env Env) int {
	root, c := NewRoot(env)
	root.SetArgs(env.Args)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return apperr.ExitOK
	}
	var ae agentExit
	if errors.As(err, &ae) {
		return ae.code
	}
	var p printed
	if !errors.As(err, &p) {
		// cobra's own errors (unknown command or flag before our wrapper).
		err = apperr.Usage(err.Error())
		_ = c.fail(cmd, err)
	}
	if ctx.Err() != nil && apperr.As(err).Class != apperr.ClassDismissed {
		err = apperr.Wrap(ctx.Err(), apperr.CodeInterrupted, apperr.ClassInterrupted, "interrupted")
	}
	return apperr.Exit(err, cmd != nil && cmd.Annotations[annLaunching] == "true")
}

// pinClock lets a test build fix the host clock (testclock.go, built
// only with the agxtestclock tag that internal/testkit uses), so the
// examples' relative times ("5 d ago", "2 hr 50 min") read the same on
// every run. A release build leaves the clock alone: there is no knob.
var pinClock = func(*agentio.Host, func(string) (string, bool)) {}

// SystemEnv is the real process environment.
func SystemEnv(args []string) (Env, error) {
	h, err := agentio.HostFromOS()
	if err != nil {
		return Env{}, err
	}
	pinClock(&h, os.LookupEnv)
	cwd, err := os.Getwd()
	if err != nil {
		return Env{}, err
	}
	self, err := os.Executable()
	if err != nil {
		self = Name
	}
	fd := int(os.Stdin.Fd())
	width := 0
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	} else if n, err := strconv.Atoi(os.Getenv(envColumns)); err == nil {
		width = n
	}
	errTTY := term.IsTerminal(int(os.Stderr.Fd()))
	outTTY := term.IsTerminal(int(os.Stdout.Fd()))
	tty := term.IsTerminal(fd) && errTTY
	return Env{
		Args: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Host: h, Lookup: os.LookupEnv, Cwd: cwd, TTY: tty, StderrTTY: errTTY, StdoutTTY: outTTY, StdinFD: fd, Self: self, Width: width,
	}, nil
}

// flagRepeatable reports a flag that can be given more than once.
func flagRepeatable(f *pflag.Flag) bool {
	t := f.Value.Type()
	return strings.HasSuffix(t, "Array") || strings.HasSuffix(t, "Slice")
}
