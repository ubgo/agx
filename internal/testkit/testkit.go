// Package testkit runs the real agx binary against fixture machines: a
// temporary user home, config dir, fake agent binaries on PATH and a
// clean environment. Tests never touch real accounts, real vendor
// binaries, the network or quota (CLAUDE.md); a fake agent is a shell
// script that prints its argv, folder and environment.
//
// The binary is built once per test process (Binary) and every Run starts
// it with exactly the fixture's environment, so the host's HOME, PATH and
// AGX_* never leak in.
package testkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	buildPath string
	buildErr  error
)

// moduleRoot finds go.mod above the working directory.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("testkit: go.mod not found")
		}
		dir = parent
	}
}

// TestClockTag is the build tag of the test binary's fixed-clock hook.
const TestClockTag = "agxtestclock"

// EnvTestNow fixes the test binary's clock (RFC 3339), for relative times
// such as "5 d ago" that would otherwise change every day.
const EnvTestNow = "AGX_TEST_NOW"

// Binary builds agx once for this test process and returns its path.
func Binary(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		root, err := moduleRoot()
		if err != nil {
			buildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "agx-testbin-")
		if err != nil {
			buildErr = err
			return
		}
		buildPath = filepath.Join(dir, "agx")
		if runtime.GOOS == "windows" {
			buildPath += ".exe"
		}
		// agxtestclock lets a test fix the binary's clock with
		// AGX_TEST_NOW (internal/cli/testclock.go), so relative times in
		// goldens and examples are stable; release builds lack the tag.
		cmd := exec.Command("go", "build", "-tags", TestClockTag, "-o", buildPath, ".")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = errors.New("testkit: build agx: " + err.Error() + "\n" + string(out))
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildPath
}

// Fixture is one fake machine.
type Fixture struct {
	t testing.TB
	// Root holds everything below.
	Root string
	// Home is the user home ($HOME).
	Home string
	// ConfigDir is AGX_CONFIG_DIR.
	ConfigDir string
	// Bin is first on PATH; fake agents go here.
	Bin string
	// Work is a plain folder to run commands in.
	Work string
	// Env are extra environment variables for every Run.
	Env map[string]string
}

// New builds an empty fixture machine.
func New(t testing.TB) *Fixture {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	f := &Fixture{t: t, Root: root, Home: filepath.Join(root, "home"), ConfigDir: filepath.Join(root, "home", ".agx"),
		Bin: filepath.Join(root, "bin"), Work: filepath.Join(root, "work"), Env: map[string]string{}}
	for _, d := range []string{f.Home, f.Bin, f.Work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// Mkdir creates rel under Root and returns its path.
func (f *Fixture) Mkdir(rel string) string {
	f.t.Helper()
	p := filepath.Join(f.Root, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// HomeDir creates rel under Home (an agent home such as ".claude-work").
func (f *Fixture) HomeDir(rel string) string {
	f.t.Helper()
	p := filepath.Join(f.Home, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// WriteFile writes data to path (absolute, or relative to Root).
func (f *Fixture) WriteFile(path, data string, mode os.FileMode) string {
	f.t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(f.Root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		f.t.Fatal(err)
	}
	// os.WriteFile applies mode only to a new file, minus the umask; the
	// chmod makes it exact, since tests assert on modes (tokens.jsonc).
	if err := os.Chmod(path, mode); err != nil {
		f.t.Fatal(err)
	}
	return path
}

// WriteConfig writes config.jsonc in ConfigDir.
func (f *Fixture) WriteConfig(jsonc string) string {
	return f.WriteFile(filepath.Join(f.ConfigDir, "config.jsonc"), jsonc, 0o644)
}

// ReadFile reads a file (absolute, or relative to Root).
func (f *Fixture) ReadFile(path string) string {
	f.t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(f.Root, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// fakeAgentScript prints what the agent received: one "argv" line with
// each argument bracketed, its folder, and its sorted environment.
//
// Why the grep: the variables the shell itself exports (SHLVL, _,
// OLDPWD) differ per OS (macOS /bin/sh adds SHLVL=1 and _=/usr/bin/env,
// Linux dash adds neither), and agx never sets them, so they are dropped
// to keep the printed environment identical on every OS.
//
// Why --version answers alone: agentio reads the first version-looking
// number in the output, and the folder and environment lines carry the
// temporary path, whose digits (macOS TMPDIR /var/folders/…, a test's
// 001 folder) would decide the version. A fixed answer keeps
// --vendor-checks the same on every machine.
const fakeAgentScript = `#!/bin/sh
if [ "$1" = "--version" ]; then printf '%s 1.0.0 (fake)\n' "$(basename "$0")"; exit 0; fi
printf 'agent %s\n' "$(basename "$0")"
printf 'argv'
for a in "$@"; do printf ' [%s]' "$a"; done
printf '\n'
printf 'pwd %s\n' "$(pwd -P)"
env | LC_ALL=C sort | grep -Ev '^(SHLVL|_|OLDPWD)=' | sed 's/^/env /'
exit ${FAKE_AGENT_EXIT:-0}
`

// FakeAgent installs a fake agent binary named name in Bin.
func (f *Fixture) FakeAgent(name string) string {
	return f.WriteFile(filepath.Join(f.Bin, name), fakeAgentScript, 0o755)
}

// Result is one agx run.
type Result struct {
	Stdout, Stderr string
	Code           int
}

// Agent parses a fake agent's output (when agx exec'd it).
type Agent struct {
	Name string
	Argv []string
	Pwd  string
	Env  map[string]string
}

// Agent parses r.Stdout as fake-agent output; ok is false when no agent ran.
func (r Result) Agent() (Agent, bool) {
	var a Agent
	a.Env = map[string]string{}
	found := false
	for line := range strings.SplitSeq(r.Stdout, "\n") {
		switch {
		case strings.HasPrefix(line, "agent "):
			a.Name, found = strings.TrimPrefix(line, "agent "), true
		case strings.HasPrefix(line, "argv"):
			rest := strings.TrimPrefix(line, "argv")
			for _, part := range strings.Split(rest, "] [") {
				part = strings.TrimPrefix(strings.TrimSpace(part), "[")
				part = strings.TrimSuffix(part, "]")
				if part != "" || rest != "" {
					a.Argv = append(a.Argv, part)
				}
			}
			if strings.TrimSpace(rest) == "" {
				a.Argv = nil
			}
		case strings.HasPrefix(line, "pwd "):
			a.Pwd = strings.TrimPrefix(line, "pwd ")
		case strings.HasPrefix(line, "env "):
			k, v, _ := strings.Cut(strings.TrimPrefix(line, "env "), "=")
			a.Env[k] = v
		}
	}
	return a, found
}

// environ is the fixture's full environment.
func (f *Fixture) environ() []string {
	base := map[string]string{
		"HOME":           f.Home,
		"PATH":           f.Bin + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin",
		"AGX_CONFIG_DIR": f.ConfigDir,
		"TMPDIR":         f.Mkdir("tmp"),
		"LANG":           "C",
		"TZ":             "UTC",
	}
	if runtime.GOOS == "windows" {
		base["USERPROFILE"] = f.Home
	}
	for k, v := range f.Env {
		base[k] = v
	}
	out := make([]string, 0, len(base))
	for k, v := range base {
		if v != "" {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}

// Filter runs a shell filter (the part of an example command after
// "|", such as "wc -c") in dir with input on its stdin and the fixture's
// environment, as a shell pipeline would after agx. Only standard tools
// on /usr/bin and /bin are meant here.
func (f *Fixture) Filter(dir, input, script string) Result {
	f.t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = f.environ()
	cmd.Stdin = strings.NewReader(input)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatal(err)
	}
	return Result{Stdout: out.String(), Stderr: errb.String(), Code: code}
}

// Run runs agx in Work with args.
func (f *Fixture) Run(args ...string) Result { return f.RunIn(f.Work, "", args...) }

// RunIn runs agx in dir with stdin and args.
func (f *Fixture) RunIn(dir, stdin string, args ...string) Result {
	f.t.Helper()
	cmd := exec.Command(Binary(f.t), args...)
	cmd.Dir = dir
	cmd.Env = f.environ()
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatal(err)
	}
	return Result{Stdout: out.String(), Stderr: errb.String(), Code: code}
}
