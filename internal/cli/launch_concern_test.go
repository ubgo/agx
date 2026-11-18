package cli_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// headlessConfig is optConfig plus a "batch" profile whose Claude block
// sets two options of one concern (print and output_format: both
// headless output), and one whose block sets two rival options of one
// concern (resume and continue).
var headlessConfig = strings.Replace(optConfig, `"cx": {`,
	`"batch": { "agent": "claude", "home": "~/.claude-work", "claude": { "print": true, "output_format": "json" } },
    "both": { "agent": "claude", "home": "~/.claude-work", "claude": { "resume": "abc", "continue": true } },
    "cx": {`, 1)

// TestConcernOneInputManyKeys: options of one concern from one input (a
// profile block, or the --option flags) are one choice: kept together and
// checked by the agent's own rules, never dropped against each other.
func TestConcernOneInputManyKeys(t *testing.T) {
	f := optFixture(t)
	f.WriteConfig(headlessConfig)

	a := optAgent(t, f.Run("run", "--profile", "batch"))
	if !slices.Contains(a.Argv, "-p") || argAfter(a.Argv, "--output-format") != "json" {
		t.Errorf("profile block: argv %q, want -p and --output-format json", a.Argv)
	}

	a = optAgent(t, f.Run("run", "--option", "print=true", "--option", "output_format=json"))
	if !slices.Contains(a.Argv, "-p") || argAfter(a.Argv, "--output-format") != "json" {
		t.Errorf("--option pair: argv %q, want -p and --output-format json", a.Argv)
	}

	// The command line beats the profile, and the block's keys give way
	// as one input, named together in one note.
	r := f.Run("run", "--profile", "batch", "--headless", "--dry-run")
	if r.Code != 0 || !strings.Contains(r.Stdout, "--headless wins the headless; profile batch claude.print, claude.output_format is not used") {
		t.Errorf("override: code %d\n%s%s", r.Code, r.Stdout, r.Stderr)
	}
}

// TestConcernContradictionRefused: two keys of one input that the agent
// itself rejects together are refused with the agent's reason, named
// once (no doubled "claude: claude:" prefix).
func TestConcernContradictionRefused(t *testing.T) {
	f := optFixture(t)
	f.WriteConfig(headlessConfig)
	r := f.Run("run", "--profile", "both", "--json")
	optFail(t, r, "usage", "--resume and --continue are exclusive")
	if strings.Contains(r.Stdout, "claude: claude:") {
		t.Errorf("doubled agent prefix: %s", r.Stdout)
	}
	r = f.Run("run", "--option", "output_format=json", "--json")
	optFail(t, r, "usage", "claude: invalid launch options: --output-format needs -p")
	if strings.Contains(r.Stdout, "claude: claude:") {
		t.Errorf("doubled agent prefix: %s", r.Stdout)
	}
}

// TestConcernTwoCommandLineInputs: two command-line inputs for one
// concern are still refused, naming both (edge 37).
func TestConcernTwoCommandLineInputs(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--headless", "--option", "output_format=json", "--json"), "conflict", "--headless and --option output_format both set the headless; pick one")
}

// lineCol is the 1-based line and byte column of the first needle in src.
func lineCol(t *testing.T, src, needle string) string {
	t.Helper()
	i := strings.Index(src, needle)
	if i < 0 {
		t.Fatalf("%q not in source", needle)
	}
	line := strings.Count(src[:i], "\n") + 1
	col := i - strings.LastIndex(src[:i], "\n")
	return fmt.Sprintf("%d:%d", line, col)
}

// TestSettingTypeErrorPosition: a settings type error names the file,
// line and column of the key, both for settings written inline in the
// profile (config.jsonc) and for a settings file.
func TestSettingTypeErrorPosition(t *testing.T) {
	f := optFixture(t)
	cfg := strings.Replace(optConfig, `"cleanupPeriodDays": 7,`, `"cleanupPeriodDays": "seven",`, 1)
	f.WriteConfig(cfg)
	optFail(t, f.Run("run", "--profile", "inline", "--json"), "usage",
		"config.jsonc:"+lineCol(t, cfg, `"cleanupPeriodDays"`)+": profile inline settings cleanupPeriodDays: ")

	bad := strings.Replace(strictSettings, `"cleanupPeriodDays": 7`, `"cleanupPeriodDays": "seven"`, 1)
	f.WriteFile(filepath.Join(f.ConfigDir, "settings", "strict.jsonc"), bad, 0o644)
	optFail(t, f.Run("run", "--profile", "work", "--json"), "usage",
		"~/.agx/settings/strict.jsonc:"+lineCol(t, bad, `"cleanupPeriodDays"`)+": profile work settings cleanupPeriodDays: ")

	// A --setting has no file: it is named by its flag, as before.
	optFail(t, f.Run("run", "--profile", "inline", "--setting", "cleanupPeriodDays=\"x\"", "--json"), "usage", "--setting cleanupPeriodDays: ")
}

// TestSettingsIsOneThing: "settings" is agx's own key (a JSONC file or an
// object, merged with --setting, checked and delivered by agx), never the
// agent's raw settings option as well: --option settings is refused with
// the two ways to give settings, and the valid --option keys leave it out.
func TestSettingsIsOneThing(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--option", "settings=/tmp/s.json", "--json"), "usage",
		"--option settings: settings is not a launch option; use --setting KEY=VALUE, or \"settings\" in the profile's claude block (a JSONC file or an object)")
	r := f.Run("run", "--option", "efort=max", "--json")
	optFail(t, r, "usage", "valid keys: ")
	if strings.Contains(r.Stdout, " settings,") || strings.Contains(r.Stdout, ", settings\"") {
		t.Errorf("valid keys list settings: %s", r.Stdout)
	}
	// The block's settings keep working as agx's settings.
	a := optAgent(t, f.Run("run", "--profile", "work"))
	if p := argAfter(a.Argv, "--settings"); !strings.HasSuffix(p, ".json") {
		t.Errorf("block settings not delivered: argv %q", a.Argv)
	}
}
