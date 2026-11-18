package cli_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// pathsConfig has a Claude profile whose block and args name paths with
// "~", next to values that are not paths, and a Gemini profile (no typed
// options, so nothing of it is a path).
const pathsConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true,
      "claude": { "append_system_prompt_file": "~/prompts/work.md", "mcp_configs": ["~/mcp/a.json", "/abs/b.json"], "system_prompt": "~/not-a-path" },
      "args": ["--debug-file", "~/d.log", "--plugin-dir=~/plug", "--name", "~/kept"] },
    "gm": { "agent": "gemini", "home": "~/.gemini", "default": true, "args": ["--include-directories", "~/kept"] }
  }
}`

// pathsFixture is a machine with pathsConfig and fake agents.
func pathsFixture(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	f.HomeDir(".claude-work")
	f.HomeDir(".gemini")
	f.FakeAgent("claude")
	f.FakeAgent("gemini")
	f.WriteConfig(pathsConfig)
	return f
}

// TestTildeInPathOptions: "~" is expanded in the values of options
// agentio marks as paths, in the profile block, in --option and after a
// path option's flag in config raw args; never in other values, never
// after "--" and never for an agent without typed options.
func TestTildeInPathOptions(t *testing.T) {
	f := pathsFixture(t)
	home := func(rel string) string { return filepath.Join(f.Home, rel) }

	r := f.Run("run", "--profile", "work", "--option", "system_prompt_file=~/sys.md", "--", "--add-dir", "~/raw")
	ag, ok := r.Agent()
	if r.Code != 0 || !ok {
		t.Fatalf("launch: code %d\n%s", r.Code, r.Stderr)
	}
	for flag, want := range map[string]string{
		"--append-system-prompt-file": home("prompts/work.md"),
		"--system-prompt-file":        home("sys.md"),
		"--debug-file":                home("d.log"),
		"--system-prompt":             "~/not-a-path",
		"--name":                      "~/kept",
		"--add-dir":                   "~/raw",
	} {
		if got := argAfter(ag.Argv, flag); got != want {
			t.Errorf("%s: got %q, want %q (argv %q)", flag, got, want, ag.Argv)
		}
	}
	for _, want := range []string{home("mcp/a.json"), "/abs/b.json", "--plugin-dir=" + home("plug")} {
		if !slices.Contains(ag.Argv, want) {
			t.Errorf("argv lacks %q: %q", want, ag.Argv)
		}
	}

	// The dry run's decisions keep the value as written.
	r = f.Run("run", "--profile", "work", "--dry-run", "--quiet")
	if r.Code != 0 || !strings.Contains(r.Stdout, `append_system_prompt_file=~/prompts/work.md`) {
		t.Errorf("decision not as written: code %d\n%s", r.Code, r.Stdout)
	}

	r = f.Run("run", "--profile", "gm")
	ag, ok = r.Agent()
	if r.Code != 0 || !ok || argAfter(ag.Argv, "--include-directories") != "~/kept" {
		t.Errorf("gemini args changed: code %d argv %q\n%s", r.Code, ag.Argv, r.Stderr)
	}
}
