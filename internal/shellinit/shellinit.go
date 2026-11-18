// Package shellinit generates the shell layer (spec 12): the script that
// `eval "$(agx shell-init --shell zsh)"` (bash, fish and pwsh alike)
// loads into the user's shell.
//
// The script defines one agx() wrapper that calls the real binary as
// `command agx` and does only what a child process cannot do for its
// shell:
//
//   - cd after new / resume: the wrapper passes a mktemp file as the
//     hidden --plan-file, reads the folder back with `agx plan show
//     --plan-file F --field data.dir`, cds there and runs `agx launch
//     --plan-file F`, which execs the agent;
//   - `agx login use ... --terminal`: the binary validates the login and
//     prints the POSIX line `export AGX_LOGIN=X` or `unset AGX_LOGIN`;
//     the wrapper matches exactly those two shapes and sets or unsets the
//     variable itself (never eval, so no output can run as code), which
//     also translates them for fish and pwsh.
//
// It also exports AGX_SHELL (doctor reports the layer as loaded), adds
// OSC 7 cwd reporting with --osc7, and one function per config alias.
// All logic stays in tested Go; the scripts only route.
//
// A layer generated with shell-init --config X is the layer of X: every
// call the wrapper makes for the user (straight through, new and resume,
// login --terminal) passes the same --config X, quoted for the shell, and
// the alias functions call the wrapper, never the binary, so they carry it
// too. The aliases come from X, so they must run against X. One documented
// exception: when the user's own arguments already carry --config (or
// --config=…) before a bare --, the wrapper adds nothing and the typed one
// wins; the binary refuses a single-value flag given twice, so adding X
// there would break every explicit --config typed in that shell. A
// --config after -- belongs to the agent and does not count. The internal
// plan show and launch calls take none: the plan file records the config
// it was made with.
package shellinit

import (
	"embed"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/shellinit/aliasrule"
)

// Shell names a supported shell (the --shell value of shell-init and
// completion).
type Shell string

// Supported shells. bash and zsh share one POSIX-style script.
const (
	Zsh  Shell = "zsh"
	Bash Shell = "bash"
	Fish Shell = "fish"
	Pwsh Shell = "pwsh"
)

// Shells is every supported shell, in help order.
func Shells() []Shell { return []Shell{Zsh, Bash, Fish, Pwsh} }

// ShellNames is Shells as strings, for messages and flag help.
func ShellNames() []string {
	out := make([]string, 0, len(Shells()))
	for _, s := range Shells() {
		out = append(out, string(s))
	}
	return out
}

// ParseShell checks a --shell value.
func ParseShell(v string) (Shell, error) {
	for _, s := range Shells() {
		if string(s) == v {
			return s, nil
		}
	}
	if v == "" {
		return "", apperr.Usage("--shell is required: one of " + strings.Join(ShellNames(), ", "))
	}
	return "", apperr.Usage(fmt.Sprintf("--shell %q: use one of %s", v, strings.Join(ShellNames(), ", ")))
}

// EnvShell is exported by the loaded layer with the shell's name; agx
// doctor reads it to report the layer as loaded. An output only: no
// launch decision depends on it.
const EnvShell = "AGX_SHELL"

// Protocol words the scripts share with the binary. They name flags and
// commands declared in internal/cli; a rename there must change them here
// (the end-to-end wrapper test fails otherwise).
const (
	binaryName   = aliasrule.BinaryName
	planFlag     = "--plan-file"
	terminalFlag = "--terminal"
	// configFlag is agx's global --config; the wrapper adds the layer's
	// own unless the user's arguments already carry one.
	configFlag = "--config"
	// DirField is the --field path of the plan folder in agx plan show.
	DirField = "data.dir"
)

// planCommands end in another folder, so the wrapper routes them through
// a plan file.
var planCommands = []string{"new", "resume"}

// exitLaunchFailed is the wrapper's own failure code (mktemp, unreadable
// plan, cd failed): the launching-command code for "agx failed before the
// agent started" (spec 14.2).
const exitLaunchFailed = 125

// Alias is one generated alias function.
type Alias struct {
	// Name is the function name.
	Name string
	// Args are named-flag agx arguments ("run --profile work").
	Args string
}

// CheckAlias validates one alias for the scripts (aliasrule.Check: the
// name, then the words). Write refuses an alias that fails it. The rule
// lives in aliasrule so config load and agx alias add apply the same one.
func CheckAlias(a Alias) error { return aliasrule.Check(a.Name, a.Args) }

// CheckAliasName checks an alias name (aliasrule.CheckName): a function
// name every shell accepts without quoting, and not a reserved name.
func CheckAliasName(name string) error { return aliasrule.CheckName(name) }

// CheckAliasArgs checks an alias value (aliasrule.CheckArgs): the charset,
// no word starting with "=" or "@", not empty.
func CheckAliasArgs(args string) error { return aliasrule.CheckArgs(args) }

// Params are one script's inputs.
type Params struct {
	Shell Shell
	// OSC7 adds terminal cwd reporting after the wrapper's cd.
	OSC7 bool
	// Aliases must each pass CheckAlias (Write refuses otherwise).
	Aliases []Alias
	// ConfigPath is the --config value shell-init ran with ("" = none). The
	// header's "generated by" and "Load it with" lines repeat it, quoted
	// for the shell, so pasting the load line loads the same config, and
	// the wrapper passes it on every user-facing call (see the package
	// doc). It must pass CheckConfigPath (Write refuses otherwise).
	ConfigPath string
}

// CheckConfigPath refuses a --config path the script cannot name: the
// path is written into comment lines, and a line break there would end
// the comment and turn the rest of the path into shell code.
func CheckConfigPath(path string) error {
	if strings.ContainsFunc(path, unicode.IsControl) {
		return fmt.Errorf("--config %q has a control character; the shell layer cannot name it", path)
	}
	return nil
}

// quoteArg quotes one argument for sh's own syntax: as is when every byte
// is plain, else in single quotes, with a quote inside escaped the way sh
// reads it (POSIX: close, backslash-quote, reopen; fish: backslash; pwsh:
// doubled), so no shell expands any part of it.
func quoteArg(sh Shell, s string) string {
	if plainArgPattern.MatchString(s) {
		return s
	}
	switch sh {
	case Fish:
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
	case Pwsh:
		return pwshString(s)
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// pwshString is s as a pwsh single-quoted string literal, always quoted:
// inside an array expression @(...) a bare --config would parse as the
// decrement operator, not as a word.
func pwshString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// plainArgPattern is an argument every shell reads literally, unquoted.
var plainArgPattern = regexp.MustCompile(`^[A-Za-z0-9_./:+=-]+$`)

//go:embed scripts/*.tmpl
var scriptFS embed.FS

// templates parses the embedded scripts once; a parse error is a build
// defect caught by every test.
var templates = template.Must(template.ParseFS(scriptFS, "scripts/*.tmpl"))

// scriptFile is the template of each shell.
var scriptFile = map[Shell]string{Zsh: posixFile, Bash: posixFile, Fish: "fish.tmpl", Pwsh: "pwsh.tmpl"}

// data is what the templates see.
type data struct {
	Name, Shell, EnvShell, EnvLogin  string
	PlanFlag, TerminalFlag, DirField string
	ExportPrefix, UnsetLine          string
	PlanCase, PlanWords, PlanList    string
	ExitLaunch                       int
	OSC7                             bool
	Aliases                          []Alias
	// ConfigArg is " --config <quoted path>" or "" (leading space, so the
	// templates append it right after a word): the header repeats it and
	// the wrapper's calls pass it (unless the user typed one).
	ConfigArg string
	// ConfigList is ConfigArg as a pwsh array body ("'--config', '<path>'").
	ConfigList string
	// Body is the bash and zsh definitions (posix-body.tmpl), rendered and
	// quoted for the single-quoted eval posix.tmpl runs with aliases off.
	Body string
	// Bin is how the POSIX wrapper runs the binary for the user:
	// "__agx_bin" (adds ConfigArg unless the args carry --config) when the
	// layer has a config, else "command agx".
	Bin string
}

// Write renders the script for p.Shell to w.
func Write(w io.Writer, p Params) error {
	file, ok := scriptFile[p.Shell]
	if !ok {
		_, err := ParseShell(string(p.Shell))
		return err
	}
	for _, a := range p.Aliases {
		if err := CheckAlias(a); err != nil {
			return apperr.Usage(err.Error())
		}
	}
	if err := CheckConfigPath(p.ConfigPath); err != nil {
		return apperr.Usage(err.Error())
	}
	configArg, configList, bin := "", "", "command "+binaryName
	if p.ConfigPath != "" {
		configArg = " " + configFlag + " " + quoteArg(p.Shell, p.ConfigPath)
		configList = pwshString(configFlag) + ", " + pwshString(p.ConfigPath)
		bin = "__" + binaryName + "_bin"
	}
	quoted := make([]string, len(planCommands))
	for i, c := range planCommands {
		quoted[i] = "'" + c + "'"
	}
	d := data{
		Name: binaryName, Shell: string(p.Shell), EnvShell: EnvShell, EnvLogin: resolve.EnvLogin,
		PlanFlag: planFlag, TerminalFlag: terminalFlag, DirField: DirField,
		ExportPrefix: "export " + resolve.EnvLogin + "=", UnsetLine: "unset " + resolve.EnvLogin,
		PlanCase: strings.Join(planCommands, "|"), PlanWords: strings.Join(planCommands, " "), PlanList: strings.Join(quoted, ", "),
		ExitLaunch: exitLaunchFailed, OSC7: p.OSC7, Aliases: p.Aliases, ConfigArg: configArg, ConfigList: configList, Bin: bin,
	}
	if file == posixFile {
		var body strings.Builder
		if err := templates.ExecuteTemplate(&body, posixBodyFile, d); err != nil {
			return err
		}
		// Inside '…' only a quote ends the string: close, escaped quote,
		// reopen, as quoteArg does.
		d.Body = "\n" + strings.ReplaceAll(body.String(), "'", `'\''`) + "\n"
	}
	return templates.ExecuteTemplate(w, file, d)
}

// The bash and zsh templates: the layer, and the definitions it evals with
// aliases off (posix.tmpl explains why).
const (
	posixFile     = "posix.tmpl"
	posixBodyFile = "posix-body.tmpl"
)
