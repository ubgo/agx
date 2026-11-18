package shellinit_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/shellinit"
)

// render renders the script for sh with one alias.
func render(t *testing.T, sh shellinit.Shell, osc7 bool) string {
	t.Helper()
	var b bytes.Buffer
	p := shellinit.Params{Shell: sh, OSC7: osc7, Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}}
	if err := shellinit.Write(&b, p); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// syntaxCheckers are the "parse only" commands of each shell.
var syntaxCheckers = map[shellinit.Shell][]string{
	shellinit.Bash: {"bash", "-n"},
	shellinit.Zsh:  {"zsh", "-n"},
	shellinit.Fish: {"fish", "--no-execute"},
	shellinit.Pwsh: {"pwsh", "-NoProfile", "-Command", "$e=$null; [void][System.Management.Automation.Language.Parser]::ParseFile('" + fileMark + "', [ref]$null, [ref]$e); if ($e) { $e; exit 1 }"},
}

// fileMark in a checker's argv is replaced by the script's path; without
// it the path is appended.
const fileMark = "@FILE@"

// TestShellInitScriptsParse renders every shell (with and without --osc7)
// and checks the syntax with the shell itself when it is installed.
func TestShellInitScriptsParse(t *testing.T) {
	for _, sh := range shellinit.Shells() {
		for _, osc7 := range []bool{false, true} {
			s := render(t, sh, osc7)
			for _, want := range []string{shellinit.EnvShell, "command agx", "--plan-file", "plan show", "launch", shellinit.DirField, "cw"} {
				if sh == shellinit.Pwsh && want == "command agx" {
					want = "-CommandType Application"
				}
				if !strings.Contains(s, want) {
					t.Errorf("%s: script lacks %q", sh, want)
				}
			}
			if osc7 != strings.Contains(s, "]7;file://") {
				t.Errorf("%s: osc7=%v mismatch", sh, osc7)
			}
			argv := syntaxCheckers[sh]
			bin, err := exec.LookPath(argv[0])
			if err != nil {
				t.Logf("%s not installed: syntax not checked", argv[0])
				continue
			}
			file := filepath.Join(t.TempDir(), "init")
			if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{}, argv[1:]...)
			if last := len(args) - 1; strings.Contains(args[last], fileMark) {
				args[last] = strings.ReplaceAll(args[last], fileMark, file)
			} else {
				args = append(args, file)
			}
			out, err := exec.Command(bin, args...).CombinedOutput()
			if err != nil {
				t.Errorf("%s syntax: %v\n%s", sh, err, out)
			}
		}
	}
}

func TestParseShell(t *testing.T) {
	for _, s := range shellinit.ShellNames() {
		if _, err := shellinit.ParseShell(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, bad := range []string{"", "powershell", "csh"} {
		if _, err := shellinit.ParseShell(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCheckAlias(t *testing.T) {
	good := []shellinit.Alias{{Name: "cl", Args: "run --profile personal"}, {Name: "cl_w-2", Args: "run --profile=work --model a.b/c:d@e+f"}}
	for _, a := range good {
		if err := shellinit.CheckAlias(a); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
	bad := []shellinit.Alias{
		{Name: "agx", Args: "run"}, {Name: "1x", Args: "run"}, {Name: "a b", Args: "run"}, {Name: "-x", Args: "run"},
		{Name: "x", Args: ""}, {Name: "x", Args: "run; rm -rf ~"}, {Name: "x", Args: "run $(id)"}, {Name: "x", Args: "run `id`"},
		{Name: "x", Args: "run =ls"}, {Name: "x", Args: "run @args"}, {Name: "x", Args: "run 'q'"},
	}
	for _, a := range bad {
		if err := shellinit.CheckAlias(a); err == nil {
			t.Errorf("%+v accepted", a)
		}
	}
	var b bytes.Buffer
	if err := shellinit.Write(&b, shellinit.Params{Shell: shellinit.Bash, Aliases: bad[5:6]}); err == nil {
		t.Error("Write accepted a bad alias")
	}
}

// fakeAgx stands in for the binary: it answers the wrapper's protocol
// (new/resume --plan-file F, plan show, launch, login use --terminal) so
// the scripts are tested without building agx. Its output names what ran.
//
// A leading "--config F" (the layer's own, from shell-init --config) is
// taken off first and shown as " [config=F]" after what ran (on stderr for
// login, whose stdout the wrapper parses), so the tests see exactly which
// calls carried it. More than one --config before a bare -- (the real
// binary refuses that: "given twice") makes it fail with DUPLICATE-CONFIG.
const fakeAgx = `#!/bin/sh
n=0
for a in "$@"; do
  case "$a" in
    --) break ;;
    --config|--config=*) n=$((n+1)) ;;
  esac
done
[ "$n" -gt 1 ] && { echo "DUPLICATE-CONFIG $*"; exit 2; }
cfg=""
if [ "$1" = "--config" ]; then cfg=" [config=$2]"; shift 2; fi
case "$1" in
  new|resume)
    [ "$2" = "--plan-file" ] || { echo "plan flag not first: $*" >&2; exit 9; }
    printf '%s %s%s\n' "$FAKE_DIR" "$*" "$cfg" > "$3"; exit 0 ;;
  plan) cut -d' ' -f1 "$4"; exit 0 ;;
  launch) echo "launched in $(pwd -P) args=$(cut -d' ' -f2- "$3")"; exit 7 ;;
  login) [ -n "$cfg" ] && echo "login$cfg" >&2; echo "$FAKE_LOGIN_LINE"; exit 0 ;;
esac
echo "passthrough $*$cfg"
`

// runWrapper loads the layer for sh in a shell with the fake binary
// first on PATH, runs body, and returns stdout+stderr.
func runWrapper(t *testing.T, sh shellinit.Shell, body string, env ...string) string {
	t.Helper()
	return runWrapperWith(t, shellinit.Params{Shell: sh, Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}}, body, env...)
}

// runWrapperWith is runWrapper for the layer p renders.
func runWrapperWith(t *testing.T, p shellinit.Params, body string, env ...string) string {
	t.Helper()
	sh := p.Shell
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	shellBin, err := exec.LookPath(string(sh))
	if err != nil {
		t.Skipf("%s not installed", sh)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "agx"), []byte(fakeAgx), 0o755); err != nil {
		t.Fatal(err)
	}
	var script bytes.Buffer
	if err := shellinit.Write(&script, p); err != nil {
		t.Fatal(err)
	}
	initFile := filepath.Join(dir, "init")
	if err := os.WriteFile(initFile, script.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	load := ". " + initFile + "\n"
	if sh == shellinit.Fish {
		load = "source " + initFile + "\n"
	}
	cmd := exec.Command(shellBin, "-c", load+body)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", sh, err, out)
	}
	return string(out)
}

// wrapperShells are the shells the protocol tests run in (pwsh is only
// syntax-checked: its wrapper differs in kind, not in protocol).
var wrapperShells = []shellinit.Shell{shellinit.Bash, shellinit.Zsh, shellinit.Fish}

// TestWrapperPlanProtocol: new puts --plan-file before the user's flags
// (so it never lands after --), cds to the plan folder, launches from the
// plan, returns the agent's code and removes the plan.
func TestWrapperPlanProtocol(t *testing.T) {
	for _, sh := range wrapperShells {
		t.Run(string(sh), func(t *testing.T) {
			target := t.TempDir()
			if r, err := filepath.EvalSymlinks(target); err == nil {
				target = r
			}
			status := `echo "rc=$? pwd=$PWD"; ls "$TMPDIR" | grep -c plan; true`
			if sh == shellinit.Fish {
				status = `echo "rc=$status pwd=$PWD"; ls $TMPDIR | grep -c plan; true`
			}
			out := runWrapper(t, sh, "agx new --name x -- --raw\n"+status, "FAKE_DIR="+target)
			for _, want := range []string{"launched in " + target, "rc=7 pwd=" + target, "--name x -- --raw", "\n0\n"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if !strings.Contains(runWrapper(t, sh, "agx usage --cached"), "passthrough usage --cached") {
				t.Error("other commands must pass straight through")
			}
		})
	}
}

// TestWrapperLoginTerminal: the export and unset lines are applied, and
// anything else is printed, never run.
func TestWrapperLoginTerminal(t *testing.T) {
	for _, sh := range wrapperShells {
		t.Run(string(sh), func(t *testing.T) {
			show := `echo "AGX_LOGIN=[$AGX_LOGIN]"`
			out := runWrapper(t, sh, "agx login use --id derek --terminal\n"+show, "FAKE_LOGIN_LINE=export AGX_LOGIN=derek")
			if !strings.Contains(out, "AGX_LOGIN=[derek]") {
				t.Errorf("export not applied:\n%s", out)
			}
			out = runWrapper(t, sh, "agx login use --home-login --terminal\n"+show, "AGX_LOGIN=old", "FAKE_LOGIN_LINE=unset AGX_LOGIN")
			if !strings.Contains(out, "AGX_LOGIN=[]") {
				t.Errorf("unset not applied:\n%s", out)
			}
			out = runWrapper(t, sh, "agx login use --id x --terminal\n"+show, "FAKE_LOGIN_LINE=export AGX_LOGIN=a; echo PWNED")
			if !strings.Contains(out, "AGX_LOGIN=[a; echo PWNED]") || strings.Contains(out, "\nPWNED") {
				t.Errorf("line was evaluated as code:\n%s", out)
			}
			out = runWrapper(t, sh, "agx login use --id x --terminal\n"+show, "FAKE_LOGIN_LINE=echo PWNED")
			if !strings.Contains(out, "echo PWNED") || !strings.Contains(out, "AGX_LOGIN=[]") {
				t.Errorf("other output must print unchanged:\n%s", out)
			}
		})
	}
}

// TestHeaderRepeatsConfig: with --config, the "generated by" and "Load it
// with" lines repeat it, quoted for each shell so a space or a quote in
// the path survives pasting; without it they name no --config; a path
// with a line break (which would end the comment) is refused.
func TestHeaderRepeatsConfig(t *testing.T) {
	const odd = "/tmp/a b/it's.jsonc"
	want := map[shellinit.Shell]string{
		shellinit.Bash: `eval "$(agx shell-init --shell bash --config '/tmp/a b/it'\''s.jsonc')"`,
		shellinit.Zsh:  `eval "$(agx shell-init --shell zsh --config '/tmp/a b/it'\''s.jsonc')"`,
		shellinit.Fish: `agx shell-init --shell fish --config '/tmp/a b/it\'s.jsonc' | source`,
		shellinit.Pwsh: `agx shell-init --shell pwsh --config '/tmp/a b/it''s.jsonc' | Out-String`,
	}
	for _, sh := range shellinit.Shells() {
		var b bytes.Buffer
		if err := shellinit.Write(&b, shellinit.Params{Shell: sh, ConfigPath: odd}); err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitN(b.String(), "\n", 3)
		if !strings.Contains(lines[1], want[sh]) {
			t.Errorf("%s load line %q, want %q", sh, lines[1], want[sh])
		}
		b.Reset()
		if err := shellinit.Write(&b, shellinit.Params{Shell: sh, ConfigPath: "/tmp/agx/config.jsonc"}); err != nil {
			t.Fatal(err)
		}
		if lines := strings.SplitN(b.String(), "\n", 3); !strings.Contains(lines[0], "--config /tmp/agx/config.jsonc`") || !strings.Contains(lines[1], "--config /tmp/agx/config.jsonc") {
			t.Errorf("%s plain path header %q", sh, lines[:2])
		}
		if s := render(t, sh, false); strings.Contains(strings.SplitN(s, "\n", 3)[1], "--config") {
			t.Errorf("%s: --config named without one", sh)
		}
		if err := shellinit.Write(&b, shellinit.Params{Shell: sh, ConfigPath: "/tmp/x\nrm -rf ~"}); err == nil {
			t.Errorf("%s: a path with a line break was accepted", sh)
		}
	}
	// The POSIX quoting round-trips through a real shell when one is there.
	if bash, err := exec.LookPath("bash"); err == nil {
		out, err := exec.Command(bash, "-c", `printf %s '/tmp/a b/it'\''s.jsonc'`).Output()
		if err != nil || string(out) != odd {
			t.Errorf("bash read %q (%v), want %q", out, err, odd)
		}
	}
}

// oddConfig is a --config path with a space and a quote: every shell must
// quote it to pass it as one argument.
const oddConfig = "/tmp/a b/it's.jsonc"

// TestWrapperPassesConfig: a layer generated with shell-init --config X
// runs every command against X: the wrapper passes the same --config X
// (quoted for the shell) on each call it makes for the user (straight
// through, new and resume, login --terminal), and the alias functions
// call the wrapper, so they carry it too. Before, the wrapper and the
// aliases called plain agx, so the aliases of X ran against the default
// config. A layer without --config passes none.
func TestWrapperPassesConfig(t *testing.T) {
	cw := []shellinit.Alias{{Name: "cw", Args: "run --profile work"}, {Name: "cr", Args: "resume --last"}}
	tag := " [config=" + oddConfig + "]"
	for _, sh := range wrapperShells {
		t.Run(string(sh), func(t *testing.T) {
			target := t.TempDir()
			if r, err := filepath.EvalSymlinks(target); err == nil {
				target = r
			}
			p := shellinit.Params{Shell: sh, Aliases: cw, ConfigPath: oddConfig}
			body := "agx usage --cached\ncw --model m\nagx new --name x\ncr\nagx login use --id e --terminal\n"
			out := runWrapperWith(t, p, body, "FAKE_DIR="+target, "FAKE_LOGIN_LINE=export AGX_LOGIN=e")
			for _, want := range []string{
				"passthrough usage --cached" + tag,
				"passthrough run --profile work --model m" + tag,
				"--name x" + tag,
				"--last" + tag,
				"login" + tag,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			// The user's own --config (either form, anywhere before a bare
			// --) wins: the layer adds none, so there is never a duplicate.
			// After --, a --config word is the agent's, so the layer's is
			// still passed.
			own := "agx --config /o usage\nagx usage --config=/o\ncw --config /o\nagx new --config /o --name y\ncr --config=/o\nagx login use --id e --terminal --config /o\nagx run -- --config raw\n"
			out = runWrapperWith(t, p, own, "FAKE_DIR="+target, "FAKE_LOGIN_LINE=export AGX_LOGIN=e")
			for _, want := range []string{
				"passthrough usage [config=/o]",
				"passthrough usage --config=/o\n",
				"passthrough run --profile work --config /o\n",
				"--config /o --name y\n",
				"--last --config=/o\n",
				"passthrough run -- --config raw" + tag,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "DUPLICATE-CONFIG") || strings.Count(out, tag) != 1 {
				t.Errorf("the layer's --config was added next to the user's:\n%s", out)
			}
			p.ConfigPath = ""
			if out := runWrapperWith(t, p, body, "FAKE_DIR="+target, "FAKE_LOGIN_LINE=export AGX_LOGIN=e"); strings.Contains(out, "[config=") {
				t.Errorf("a layer without --config passed one:\n%s", out)
			}
		})
	}
}

// TestScriptsQuoteConfigPerShell: the text check for every shell,
// including fish and pwsh, which the runtime tests skip when they are not
// installed: the layer's --config is quoted the way that shell reads it,
// every user-facing call goes through the one place that adds it, that
// place skips it when the arguments carry --config or --config=… before a
// bare --, and the alias functions call the wrapper (never the binary
// directly), so they follow the same rule. Without --config none of it is
// generated.
func TestScriptsQuoteConfigPerShell(t *testing.T) {
	posix := []string{
		`command agx --config '/tmp/a b/it'\''s.jsonc' "$@"`,
		`if __agx_has --config "$@"; then`, `--) return 1 ;;`, `"$__agx_f"|"$__agx_f"=*) return 0 ;;`,
		`__agx_bin "$__agx_cmd" --plan-file`, `__agx_out=$(__agx_bin "$@")`,
		`cw() { agx run --profile work "$@"; }`,
	}
	cases := map[shellinit.Shell]struct {
		want  []string
		calls string
		n     int
	}{
		shellinit.Bash: {posix, "__agx_bin ", 4},
		shellinit.Zsh:  {posix, "__agx_bin ", 4},
		shellinit.Fish: {[]string{
			`set -l -- cfg --config '/tmp/a b/it\'s.jsonc'`,
			`if test "$a" = --`, `if test "$a" = --config; or string match -q -- '--config=*' "$a"`, "set cfg\n",
			"    agx run --profile work $argv",
		}, "command agx $cfg ", 4},
		shellinit.Pwsh: {[]string{
			`$cfg = @('--config', '/tmp/a b/it''s.jsonc')`,
			`if ("$a" -eq '--') { break }`, `if ("$a" -eq '--config' -or "$a".StartsWith('--config=')) { $cfg = @(); break }`,
			"function cw { agx run --profile work @args }",
		}, "& $bin @cfg ", 3},
	}
	for sh, c := range cases {
		var b bytes.Buffer
		p := shellinit.Params{Shell: sh, Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}, ConfigPath: oddConfig}
		if err := shellinit.Write(&b, p); err != nil {
			t.Fatal(err)
		}
		s := b.String()
		if sh == shellinit.Bash || sh == shellinit.Zsh {
			// The definitions sit inside one single-quoted eval (aliases
			// off); undo that quoting to read them as the shell will.
			s = strings.ReplaceAll(s, `'\''`, `'`)
		}
		for _, w := range c.want {
			if !strings.Contains(s, w) {
				t.Errorf("%s: script lacks %q:\n%s", sh, w, s)
			}
		}
		if n := strings.Count(s, c.calls); n < c.n {
			t.Errorf("%s: %d calls through %q, want at least %d:\n%s", sh, n, c.calls, c.n, s)
		}
		if s := render(t, sh, false); strings.Contains(s, "__agx_bin") || strings.Contains(s, "cfg") {
			t.Errorf("%s: config handling generated without --config:\n%s", sh, s)
		}
	}
}

// hostileAliases are aliases real users keep in their shell rc (one owner
// has alias do='cd ~/Downloads'): the layer is eval'd after them, so they
// must neither break its parse nor hijack the commands it runs.
//
// Reserved words (do, done, then, fi, esac) broke the whole layer at
// parse time, so not one function was defined; commands (cd, printf, rm,
// mktemp, hostname) were silently replaced; an alias named like a config
// alias (cw) made its function definition a syntax error.
const hostileAliases = `alias do='cd /'
alias done='echo hijacked-done'
alias then='echo hijacked-then'
alias fi='echo hijacked-fi'
alias esac='echo hijacked-esac'
alias cd='echo hijacked-cd'
alias printf='echo hijacked-printf'
alias rm='echo hijacked-rm'
alias mktemp='echo hijacked-mktemp'
alias hostname='echo hijacked-hostname'
alias cw='echo user-cw'
`

// TestLayerSurvivesUserAliases: with those aliases defined before the
// layer loads, bash and zsh still define every function, a --terminal
// login still sets AGX_LOGIN, and new still cds to the plan folder.
func TestLayerSurvivesUserAliases(t *testing.T) {
	for _, sh := range []shellinit.Shell{shellinit.Bash, shellinit.Zsh} {
		t.Run(string(sh), func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("POSIX shells only")
			}
			shellBin, err := exec.LookPath(string(sh))
			if err != nil {
				t.Skipf("%s not installed", sh)
			}
			dir := t.TempDir()
			target := t.TempDir()
			if r, err := filepath.EvalSymlinks(target); err == nil {
				target = r
			}
			bin := filepath.Join(dir, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "agx"), []byte(fakeAgx), 0o755); err != nil {
				t.Fatal(err)
			}
			var script bytes.Buffer
			p := shellinit.Params{Shell: sh, Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}}
			if err := shellinit.Write(&script, p); err != nil {
				t.Fatal(err)
			}
			// bash expands aliases only when asked; zsh always does. The
			// layer is eval'd, as the documented load line does.
			prelude := hostileAliases
			if sh == shellinit.Bash {
				prelude = "shopt -s expand_aliases\n" + prelude
			}
			initFile := filepath.Join(dir, "init")
			if err := os.WriteFile(initFile, script.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			// typeset -f proves the functions exist (an alias named cw would
			// satisfy `type`); the alias setting must come back as it was.
			restored := `shopt -q expand_aliases && echo aliases-restored`
			if sh == shellinit.Zsh {
				restored = `[[ -o aliases ]] && echo aliases-restored`
			}
			// What an rc file does after the aliases: the load line, then
			// use. zsh -c parses its whole string before running any of
			// it, so aliases defined there would never apply; the steps are
			// eval'd from a file instead, parsed after the aliases exist.
			steps := "eval \"$(command cat " + initFile + ")\"\n" +
				"typeset -f cw agx __agx_has >/dev/null && echo has-cw\n" + restored + "\n" +
				`test -z "$__agx_aliases" && echo no-leftover` + "\n" +
				"agx login use --id derek --terminal\n" + `echo "AGX_LOGIN=[$AGX_LOGIN]"` + "\n" +
				"agx new --name x\n" + `echo "pwd=$PWD"` + "\n"
			stepsFile := filepath.Join(dir, "steps")
			if err := os.WriteFile(stepsFile, []byte(steps), 0o600); err != nil {
				t.Fatal(err)
			}
			// The user's alias still wins when typed (parsed after load).
			body := prelude + "eval \"$(command cat " + stepsFile + ")\"\n" + "eval cw\n"
			cmd := exec.Command(shellBin, "-c", body)
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "FAKE_DIR=" + target, "FAKE_LOGIN_LINE=export AGX_LOGIN=derek"}
			out, _ := cmd.CombinedOutput()
			// "user-cw": the user's own alias still wins when typed.
			for _, want := range []string{"has-cw", "aliases-restored", "user-cw", "no-leftover", "AGX_LOGIN=[derek]", "launched in " + target, "pwd=" + target} {
				if !strings.Contains(string(out), want) {
					t.Errorf("%s with user aliases: output lacks %q:\n%s", sh, want, out)
				}
			}
			if strings.Contains(string(out), "parse error") || strings.Contains(string(out), "syntax error") || strings.Contains(string(out), "hijacked") {
				t.Errorf("%s: an alias broke or hijacked the layer:\n%s", sh, out)
			}
		})
	}
}

// commandWord is a command the layers run that a user commonly aliases or
// wraps in a function; every call must go through builtin or command.
var commandWord = regexp.MustCompile(`(^|[^\w.\\-])(cd|printf|rm|mktemp|hostname)\s`)

// cmdletWord is a cmdlet the pwsh layer runs; every call must be module
// qualified (Microsoft.PowerShell.X\Name), which no alias or function shadows.
var cmdletWord = regexp.MustCompile(`(^|[^\\\w-])(Set-Location|Remove-Item|Get-Item|Get-Command|Select-Object)\b`)

// TestLayersNeverCallUserCommands: in every shell's layer (with --osc7 and
// a config, so every branch is generated), cd, printf, rm, mktemp and
// hostname are called as builtin or command, and pwsh cmdlets by their
// module-qualified names. fish and pwsh are not installed in the test
// environment, so their text is the proof; bash and zsh also run in
// TestLayerSurvivesUserAliases.
func TestLayersNeverCallUserCommands(t *testing.T) {
	for _, sh := range shellinit.Shells() {
		var b bytes.Buffer
		p := shellinit.Params{Shell: sh, OSC7: true, ConfigPath: "/tmp/c.jsonc", Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}}
		if err := shellinit.Write(&b, p); err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(b.String(), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "#") {
				continue
			}
			re := commandWord
			if sh == shellinit.Pwsh {
				re = cmdletWord
			}
			for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
				before := strings.TrimRight(line[:m[4]], " ")
				if sh != shellinit.Pwsh && (strings.HasSuffix(before, "builtin") || strings.HasSuffix(before, "command")) {
					continue
				}
				t.Errorf("%s line %d calls %s unguarded: %s", sh, i+1, line[m[4]:m[5]], code)
			}
		}
	}
}

// TestPosixBodyHasNoQuotes: the bash and zsh definitions sit inside one
// single-quoted eval, so they carry no quote of their own and read
// without '\” escapes; only a --config path with a quote adds one.
func TestPosixBodyHasNoQuotes(t *testing.T) {
	for _, sh := range []shellinit.Shell{shellinit.Bash, shellinit.Zsh} {
		var b bytes.Buffer
		p := shellinit.Params{Shell: sh, OSC7: true, ConfigPath: "/tmp/c.jsonc", Aliases: []shellinit.Alias{{Name: "cw", Args: "run --profile work"}}}
		if err := shellinit.Write(&b, p); err != nil {
			t.Fatal(err)
		}
		if s := b.String(); strings.Contains(s, `'\''`) || strings.Count(s, "'") != 2+strings.Count(strings.SplitN(s, `\builtin eval '`, 2)[0], "'") {
			t.Errorf("%s: the eval'd definitions hold a quote:\n%s", sh, s)
		}
	}
}
