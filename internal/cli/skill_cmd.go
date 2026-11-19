package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/versioncmd"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/render"
	"github.com/ubgo/agx/internal/skill"
	"github.com/ubgo/agx/internal/textout"
)

// skillsDirDefault is where Claude Code reads a user's skills, under the
// home folder.
const skillsDirDefault = ".claude/skills"

// skillCmd is agx skill: a Claude Code skill (SKILL.md) generated from
// the command tree the parser is built from (the same tree help --json
// prints and the docs drift test reads), so it cannot drift from the
// binary. --install writes it where Claude Code finds it.
func (c *cliCtx) skillCmd() *cobra.Command {
	var (
		install bool
		dir     string
	)
	cmd := c.newCmd("skill", "Print a Claude Code skill (SKILL.md) that teaches an agent every agx command; --install writes it", CostRead+CostWrite,
		func(cmd *cobra.Command, _ []string) (render.Output, error) {
			if c.g.JSON || len(c.g.Fields) > 0 {
				c.g.JSON, c.g.Fields = false, nil
				return render.Output{}, apperr.Usage("skill prints markdown (SKILL.md); it takes no --json or --field")
			}
			if dir != "" && !install {
				return render.Output{}, apperr.Usage("--dir chooses where --install writes; add --install")
			}
			md := SkillMarkdown(HelpTree(cmd.Root()), versioncmd.Collect(Name).Version)
			if !install {
				return render.Output{Text: func(w io.Writer) error {
					_, err := io.WriteString(w, md)
					return err
				}}, nil
			}
			target := dir
			if target == "" {
				target = filepath.Join(c.userHome(), skillsDirDefault)
			}
			res, err := skill.Install(target, md, c.App().Now())
			if err != nil {
				return render.Output{}, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "install the skill: "+err.Error())
			}
			return render.Output{Text: func(w io.Writer) error {
				ew := textout.New(w)
				if res.Backup != "" {
					ew.Line("backed up " + res.Backup)
				}
				ew.Line("wrote " + res.Path)
				return ew.Err()
			}}, nil
		})
	cmd.Long = "Print a Claude Code skill: frontmatter (name agx and when to use it), how agx works (profiles, logins, pools, the resolution chain, --dry-run, --json and --field, exit codes), then every visible command with its descriptions and flags, generated from the command tree of this binary. --install writes it to ~/.claude/skills/agx/SKILL.md (folder 0755, file 0644), renaming an existing file to SKILL.md.bak-<timestamp> first; --dir chooses another skills folder, such as a project's .claude/skills."
	cmd.Example = "  agx skill | less\n  agx skill --install\n  agx skill --install --dir .claude/skills"
	f := cmd.Flags()
	f.BoolVar(&install, "install", false, "write the skill to <dir>/agx/SKILL.md instead of printing it (an existing file is backed up)")
	f.StringVar(&dir, "dir", "", "the skills folder --install writes into (default ~/.claude/skills)")
	return cmd
}

// userHome is the home folder the skill installs under.
func (c *cliCtx) userHome() string {
	if h := c.App().Deps().Host.UserHome; h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// skillDescription is the frontmatter's one line: when an agent should
// load the skill.
const skillDescription = "Use when running, resuming or switching Claude Code / Codex / Gemini CLI / pi sessions and accounts with agx: profiles and homes, named logins, login pools that switch Claude accounts by usage, live login, plan usage, conversations, folder bindings, shell aliases, and agx's own config."

// skillIntro is the hand-written part: how agx works. The command list
// below it is generated, never written by hand.
const skillIntro = `## How agx works

agx is one CLI for AI coding agents (Claude Code, OpenAI Codex, Gemini CLI, pi and config-defined agents) across every account on the machine. Its config is ` + "`~/.agx/config.jsonc`" + ` (JSONC, checked strictly: an unknown key or a wrong type is an error at its line).

- **Profiles** name an agent home (` + "`~/.claude-work`" + `) plus launch defaults: ` + "`agx run --profile work`" + `.
- **Logins** run any home on any account: a plan token or API key from a reference (` + "`${gopass:…}`, `$(…)`, `${VAR}`" + `), a pasted value in ` + "`tokens.jsonc`" + ` (0600), or a key helper. ` + "`agx run --login carol`" + `. agx never prints a token, except ` + "`agx login token`" + ` on request (to a pipe, or with ` + "`--reveal`" + `).
- **Login pools** (Claude Code only): a login of kind ` + "`pool`" + ` lists your own logins and teammates' shared ones; ` + "`agx run --login auto`" + ` is a live run in which each key request picks a member by usage (soonest weekly reset first, own accounts almost fully, shared ones up to their caps). ` + "`agx login pick --id auto`" + ` shows the pick and why; ` + "`--pool-force NAME`" + ` forces one member; ` + "`agx live use --id RUN --login NAME`" + ` pins a running session and ` + "`--login auto`" + ` unpins it.
- **Live login**: ` + "`--live-login`" + ` makes a Claude session's login switchable while it runs (` + "`agx live use`" + `).
- **Which account a session is on**: run ` + "`! agx whoami`" + ` inside Claude (` + "`in use:`, `next:`" + `); Claude's /status only says apiKeyHelper.
- **Resolution chain**: agent, profile, home, login, binary, folder and args are each decided by the first source that sets them: flags, then ` + "`AGX_LOGIN`/`AGX_LOGIN_<AGENT>`" + `, a folder binding (` + "`agx project use`" + `), the profile, the agent defaults, the default profile. ` + "`--dry-run`" + ` on ` + "`run`/`new`/`resume`" + ` prints every decision with its source and starts nothing: use it before launching.
- **Scripting**: ` + "`--json`" + ` prints one envelope (` + "`ok`, `data`, `decisions`, `warnings`, `error`" + `); ` + "`--field data.PATH`" + ` prints one value; ` + "`--quiet`" + ` drops notes. ` + "`agx help --json`" + ` prints the command tree.
- **Exit codes**: 0 ok, 1 failure, 2 usage error (a bad flag or value), 3 partial (some rows failed), 4 unsupported by the agent; launching commands use 125 for agx's own errors, 126 and 127 for the agent binary.
- **Cost**: some commands spend quota (login probes: ` + "`agx login check`, `agx usage --all-logins`, `agx login pick`" + ` on a stale cache) or hit the network (` + "`agx usage`" + `); prefer ` + "`--cached`, `--no-probe`, `--dry-run`" + ` when you only need to look.

`

// SkillMarkdown is the SKILL.md for tree (agx help --json's tree) built
// at version: frontmatter, the intro, the global flags, then every
// visible command depth first with its descriptions, examples and flags.
func SkillMarkdown(tree HelpCommand, version string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %s\ndescription: %s\n---\n\n", Name, skillDescription)
	fmt.Fprintf(&b, "# agx\n\nGenerated by `agx skill` from agx %s. Regenerate it after upgrading agx: `agx skill --install`.\n\n", version)
	b.WriteString(skillIntro)
	b.WriteString("## Global flags\n\nEvery command takes these.\n\n")
	writeFlagTable(&b, tree.Flags)
	b.WriteString("## Commands\n\nEvery command takes named flags only; a bare `agx` or a bare group prints help.\n\n")
	for _, sub := range tree.Commands {
		writeSkillCommand(&b, sub)
	}
	return b.String()
}

// writeSkillCommand writes one visible command and its visible
// subcommands, depth first.
func writeSkillCommand(b *strings.Builder, h HelpCommand) {
	if h.Hidden {
		return
	}
	fmt.Fprintf(b, "### agx %s\n\n%s.\n\n", h.Path, strings.TrimSuffix(h.Short, "."))
	if h.Long != "" {
		b.WriteString(h.Long + "\n\n")
	}
	if h.Example != "" {
		b.WriteString("```sh\n")
		for _, l := range strings.Split(h.Example, "\n") {
			b.WriteString(strings.TrimSpace(l) + "\n")
		}
		b.WriteString("```\n\n")
	}
	if flags := visibleFlags(h.Flags); len(flags) > 0 {
		writeFlagTable(b, flags)
	}
	for _, sub := range h.Commands {
		writeSkillCommand(b, sub)
	}
}

// visibleFlags drops hidden flags.
func visibleFlags(in []HelpFlag) []HelpFlag {
	out := make([]HelpFlag, 0, len(in))
	for _, f := range in {
		if !f.Hidden {
			out = append(out, f)
		}
	}
	return out
}

// valueWord is the placeholder of a flag's value in the table.
func valueWord(typ string) string {
	switch typ {
	case "duration":
		return "DURATION"
	case "int", "int64", "uint":
		return "N"
	}
	return "VALUE"
}

// writeFlagTable writes flags as a markdown table: the flag (with its
// value type), and its help text with the default and repeatability.
func writeFlagTable(b *strings.Builder, flags []HelpFlag) {
	b.WriteString("| Flag | Meaning |\n|---|---|\n")
	for _, f := range visibleFlags(flags) {
		name := "`--" + f.Name
		if f.TakesValue {
			name += " " + valueWord(f.Type)
		}
		name += "`"
		if f.Short != "" {
			name += ", `-" + f.Short + "`"
		}
		help := f.Help
		if f.Repeatable {
			help += " (repeatable)"
		}
		if f.Default != "" {
			help += " (default " + f.Default + ")"
		}
		fmt.Fprintf(b, "| %s | %s |\n", name, strings.ReplaceAll(help, "|", `\|`))
	}
	b.WriteString("\n")
}
