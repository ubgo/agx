package cli

import (
	"bytes"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/internal/shellinit"
)

// shellFlagHelp is the --shell help of shell-init and completion.
var shellFlagHelp = "the shell: " + strings.Join(shellinit.ShellNames(), ", ")

// shellInitCmd is agx shell-init (spec 12, 13.8).
func (c *cliCtx) shellInitCmd() *cobra.Command {
	var shell string
	var osc7 bool
	cmd := c.newCmd("shell-init", "Print the shell layer", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		sh, err := shellinit.ParseShell(shell)
		if err != nil {
			return render.Output{}, err
		}
		s, err := c.App().ShellInit(sh, osc7)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(s)
		if err != nil {
			return render.Output{}, err
		}
		return render.Output{Data: data, Warnings: s.Warnings, Text: textOf(s.Script)}, nil
	})
	cmd.Long = "Print the shell layer: one agx() wrapper that cds after new and resume and sets AGX_LOGIN for login use --terminal, plus one function per config alias. Evaluate it from the shell's startup file."
	cmd.Example = "  eval \"$(agx shell-init --shell zsh)\"        # ~/.zshrc (bash: ~/.bashrc)\n  agx shell-init --shell fish | source         # ~/.config/fish/config.fish\n  agx shell-init --shell pwsh | Out-String | Invoke-Expression   # $PROFILE"
	f := cmd.Flags()
	f.StringVar(&shell, "shell", "", shellFlagHelp)
	f.BoolVar(&osc7, "osc7", false, "report the current folder to the terminal (OSC 7) after the wrapper's cd")
	return fields(cmd, "data.shell", "data.osc7", "data.aliases", "data.script")
}

// Completion is agx completion's data.
type Completion struct {
	Shell  shellinit.Shell `json:"shell"`
	Script string          `json:"script"`
}

// completionCmd is agx completion (13.8): cobra's generators, with the
// shell as a named flag.
func (c *cliCtx) completionCmd() *cobra.Command {
	var shell string
	cmd := c.newCmd("completion", "Print a completion script", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		sh, err := shellinit.ParseShell(shell)
		if err != nil {
			return render.Output{}, err
		}
		var b bytes.Buffer
		root := cmd.Root()
		switch sh {
		case shellinit.Bash:
			err = root.GenBashCompletionV2(&b, true)
		case shellinit.Zsh:
			err = root.GenZshCompletion(&b)
		case shellinit.Fish:
			err = root.GenFishCompletion(&b, true)
		case shellinit.Pwsh:
			err = root.GenPowerShellCompletionWithDesc(&b)
		}
		if err != nil {
			return render.Output{}, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "completion: "+err.Error())
		}
		data, err := render.Encode(Completion{Shell: sh, Script: b.String()})
		if err != nil {
			return render.Output{}, err
		}
		return render.Output{Data: data, Text: textOf(b.String())}, nil
	})
	cmd.Example = "  source <(agx completion --shell bash)\n  agx completion --shell zsh > \"${fpath[1]}/_agx\"\n  agx completion --shell fish | source"
	cmd.Flags().StringVar(&shell, "shell", "", shellFlagHelp)
	return fields(cmd, "data.shell", "data.script")
}

// textOf prints s verbatim as the human form.
func textOf(s string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	}
}

// launchCmd is the hidden agx launch --plan-file F (spec 12): the shell
// wrapper's last step, after it cd'd to the plan's folder. It prepares
// from the plan (secrets resolve now), commits and execs the agent; like
// run it is a launching command (exit 125 family, edge 53).
func (c *cliCtx) launchCmd() *cobra.Command {
	var path string
	cmd := c.newCmd("launch", "Start the agent from a plan file (shell layer)", CostExec, func(cmd *cobra.Command, args []string) (render.Output, error) {
		p, err := c.App().LaunchPlanFile(cmd.Context(), path)
		if err != nil {
			return render.Output{}, err
		}
		code, err := run.Start(p.BinaryPath, p.Command)
		if err != nil {
			return render.Output{}, err
		}
		return render.Output{}, agentExit{code: code}
	})
	cmd.Flags().StringVar(&path, "plan-file", "", "the plan file written by new or resume --plan-file")
	return launching(cmd)
}

// planFields are the --field paths of agx plan show's data.
var planFields = []string{"data.schema_version", "data.created_at", "data.dir", "data.mode", "data.agent", "data.profile", "data.home", "data.login", "data.binary", "data.folder", "data.resume_id", "data.command"}

// planCmd is the hidden agx plan show --plan-file F: the wrapper reads
// the folder to cd into with --field data.dir. It checks the plan like
// launch does (edge 53), so a stale or foreign plan never moves the shell.
func (c *cliCtx) planCmd() *cobra.Command {
	var path string
	show := c.newCmd("show", "Print a plan file", CostRead, func(cmd *cobra.Command, args []string) (render.Output, error) {
		v, err := c.App().ShowPlan(path)
		if err != nil {
			return render.Output{}, err
		}
		data, err := render.Encode(v)
		if err != nil {
			return render.Output{}, err
		}
		return render.Output{Data: data, Text: func(w io.Writer) error { return planText(w, v) }}, nil
	})
	show.Flags().StringVar(&path, "plan-file", "", "the plan file")
	return group("plan", "Shell plan files", fields(show, planFields...))
}
