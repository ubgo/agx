package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// optConfig has a Claude profile with a typed block (options plus a
// settings file), one with inline settings and a login, and Codex,
// Gemini and pi profiles.
const optConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true,
      "claude": { "effort": "high", "settings": "settings/strict.jsonc" } },
    "inline": { "agent": "claude", "home": "~/.claude-work", "login": "carol",
      "claude": { "settings": { "cleanupPeriodDays": 7, "env": { "DEBUG_LEVEL": "1" } } } },
    "cx": { "agent": "codex", "home": "~/.codex", "default": true },
    "gm": { "agent": "gemini", "home": "~/.gemini", "default": true },
    "pi": { "agent": "pi", "home": "~/.pi", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" },
    "cxlogin": { "agents": ["codex"], "kind": "api-key", "token": "${TOKVAR}" }
  }
}`

// strictSettings is a JSONC settings file (comments allowed, recipe 1).
const strictSettings = `{
  // keep a week of history
  "cleanupPeriodDays": 7,
  "permissions": { "allow": ["Read"] }
}`

// optFixture is a machine with optConfig and fake agents.
func optFixture(t *testing.T) *testkit.Fixture {
	f := testkit.New(t)
	for _, h := range []string{".claude-work", ".codex", ".gemini", ".pi"} {
		f.HomeDir(h)
	}
	for _, b := range []string{"claude", "codex", "gemini", "pi"} {
		f.FakeAgent(b)
	}
	f.WriteConfig(optConfig)
	f.WriteFile(filepath.Join(f.ConfigDir, "settings", "strict.jsonc"), strictSettings, 0o644)
	f.Env["TOKVAR"] = "fake-token-ALPHA"
	return f
}

// argAfter returns the argument following flag in argv ("" when absent).
func argAfter(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

// readJSONObject decodes a settings file or inline value.
func readJSONObject(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("settings %q: %v", s, err)
	}
	return m
}

// TestOptOptionOverridesProfile is recipe 7: --option beats the block.
func TestOptOptionOverridesProfile(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--option", "effort=max"))
	if argAfter(a.Argv, "--effort") != "max" {
		t.Errorf("argv %q", a.Argv)
	}
	r := f.Run("run", "--option", "effort=max", "--dry-run")
	if !strings.Contains(r.Stdout, "effort=max") || !strings.Contains(r.Stdout, "overrides profile work: high") {
		t.Errorf("dry run: %s", r.Stdout)
	}
}

// TestOptUnknownOption: an unknown --option key lists a suggestion.
func TestOptUnknownOption(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--option", "efort=max", "--json"), "usage", "--option efort: unknown key (did you mean effort?)")
}

// TestOptBlockUnknownKeyFailsConfig: an unknown block key fails the load.
func TestOptBlockUnknownKeyFailsConfig(t *testing.T) {
	f := optFixture(t)
	f.WriteConfig(strings.Replace(optConfig, `"effort": "high"`, `"efort": "high"`, 1))
	r := f.Run("run", "--json")
	optFail(t, r, "config", "efort")
}

// TestSettingFileDelivery is recipe 1 and 3: the profile's JSONC file and
// --setting merged into <run>/settings-<id>.json (0600), passed by path.
func TestSettingFileDelivery(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--setting", "statusLine.type=command", "--setting", "statusLine.command=~/bin/s.sh"))
	path := argAfter(a.Argv, "--settings")
	runID := a.Env["AGX_RUN_ID"]
	if filepath.Base(path) != "settings-"+runID+".json" || filepath.Dir(path) != filepath.Join(f.ConfigDir, "runs", runID) {
		t.Fatalf("settings path %q", path)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("settings file mode: %v %v", st, err)
		}
	}
	m := readJSONObject(t, f.ReadFile(path))
	if string(m["cleanupPeriodDays"]) != "7" || !strings.Contains(string(m["statusLine"]), `"command":"~/bin/s.sh"`) || !strings.Contains(string(m["statusLine"]), `"type":"command"`) {
		t.Errorf("settings %s", f.ReadFile(path))
	}
	r := f.Run("run", "--dry-run")
	if !strings.Contains(r.Stdout, "cleanupPeriodDays, permissions.allow") || !strings.Contains(r.Stdout, "settings/strict.jsonc") || !strings.Contains(r.Stdout, "delivered as file") {
		t.Errorf("dry run: %s", r.Stdout)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.ConfigDir, "runs")); len(entries) != 1 {
		t.Errorf("dry run wrote a run folder: %d entries", len(entries))
	}
}

// TestEdge40SettingsInline: inline delivery passes the JSON and notes ps.
func TestEdge40SettingsInline(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--setting", "env.DEBUG_LEVEL=debug", "--settings-delivery", "inline")
	a := optAgent(t, r)
	m := readJSONObject(t, argAfter(a.Argv, "--settings"))
	if !strings.Contains(string(m["env"]), `"DEBUG_LEVEL":"debug"`) {
		t.Errorf("inline settings %v", m)
	}
	if !strings.Contains(r.Stderr, "settings are visible in ps (inline delivery") {
		t.Errorf("stderr %s", r.Stderr)
	}
	optFail(t, f.Run("run", "--setting", "a=1", "--settings-delivery", "pipe", "--json"), "usage", "--settings-delivery")
}

// TestEdge35SettingOverridesProfile: the flag wins; the dry run shows both.
func TestEdge35SettingOverridesProfile(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "inline", "--setting", "cleanupPeriodDays=30", "--dry-run", "--json", "--field", "decisions")
	if r.Code != 0 {
		t.Fatalf("code %d %s", r.Code, r.Stderr)
	}
	if !strings.Contains(r.Stdout, `"cleanupPeriodDays, env.DEBUG_LEVEL"`) || !strings.Contains(r.Stdout, "--setting (overrides profile inline: cleanupPeriodDays)") {
		t.Errorf("decisions %s", r.Stdout)
	}
	a := optAgent(t, f.Run("run", "--profile", "inline", "--setting", "cleanupPeriodDays=30", "--settings-delivery", "inline"))
	if m := readJSONObject(t, argAfter(a.Argv, "--settings")); string(m["cleanupPeriodDays"]) != "30" {
		t.Errorf("settings %v", m)
	}
}

// TestEdge36UnknownSettingKey: passed on with a "did you mean" warning.
func TestEdge36UnknownSettingKey(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--setting", "apiKeyHelpr=x", "--settings-delivery", "inline")
	a := optAgent(t, r)
	if !strings.Contains(argAfter(a.Argv, "--settings"), `"apiKeyHelpr":"x"`) {
		t.Errorf("unknown key dropped: %q", a.Argv)
	}
	if !strings.Contains(r.Stderr, "--setting apiKeyHelpr: not modelled (did you mean apiKeyHelper?)") {
		t.Errorf("stderr %s", r.Stderr)
	}
}

// TestSettingTypeError: a modelled key with the wrong type is refused.
func TestSettingTypeError(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--setting", "cleanupPeriodDays=\"thirty\"", "--json"), "usage", "--setting cleanupPeriodDays")
}

// TestEdge37TwoTypedChoices: two command-line choices for one concern.
func TestEdge37TwoTypedChoices(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--model", "opus", "--setting", "model=sonnet", "--json"), "conflict", "--model and --setting model both set the model; pick one")
	optFail(t, f.Run("run", "--model", "opus", "--option", "model=sonnet", "--json"), "conflict", "--model and --option model both set the model")
	optFail(t, f.Run("run", "--login", "carol", "--setting", "apiKeyHelper=/bin/h", "--json"), "conflict", "--login carol and --setting apiKeyHelper both set the credential; pick one")
}

// TestOptCommandLineBeatsProfile: a --setting credential beats the
// profile's login (noted, token not set); --model beats a profile setting.
func TestOptCommandLineBeatsProfile(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "inline", "--setting", "apiKeyHelper=/bin/h")
	a := optAgent(t, r)
	if _, ok := a.Env["CLAUDE_CODE_OAUTH_TOKEN"]; ok {
		t.Errorf("profile login token set although apiKeyHelper won")
	}
	r = f.Run("run", "--profile", "inline", "--setting", "apiKeyHelper=/bin/h", "--dry-run")
	if !strings.Contains(r.Stdout, "--setting apiKeyHelper wins the credential; login carol (from profile inline) is not used") {
		t.Errorf("dry run %s", r.Stdout)
	}
	f.WriteConfig(strings.Replace(optConfig, `"cleanupPeriodDays": 7,`, `"cleanupPeriodDays": 7, "model": "haiku",`, 1))
	a = optAgent(t, f.Run("run", "--profile", "inline", "--model", "opus", "--settings-delivery", "inline"))
	if argAfter(a.Argv, "--model") != "opus" || strings.Contains(argAfter(a.Argv, "--settings"), "haiku") {
		t.Errorf("argv %q", a.Argv)
	}
}

// TestEdge38RawDuplicate: a raw arg repeating a typed option is passed
// last, with a note.
func TestEdge38RawDuplicate(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--", "--effort", "low")
	a := optAgent(t, r)
	if n := len(a.Argv); n < 2 || a.Argv[n-2] != "--effort" || a.Argv[n-1] != "low" {
		t.Errorf("raw not last: %q", a.Argv)
	}
	if !strings.Contains(r.Stderr, "raw --effort repeats a typed effort") {
		t.Errorf("stderr %s", r.Stderr)
	}
}

// TestEdge39RawSettingsMerged: a raw --settings is merged into agx's
// settings (one --settings reaches the agent), and a raw apiKeyHelper is
// the run's credential, noted like a profile one.
func TestEdge39RawSettingsMerged(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--settings-delivery", "inline", "--", "--settings", `{"apiKeyHelper":"/bin/h"}`))
	count := 0
	for _, arg := range a.Argv {
		if arg == "--settings" {
			count++
		}
	}
	m := readJSONObject(t, argAfter(a.Argv, "--settings"))
	if count != 1 || string(m["apiKeyHelper"]) != `"/bin/h"` || string(m["cleanupPeriodDays"]) != "7" {
		t.Errorf("argv %q", a.Argv)
	}
	r := f.Run("run", "--dry-run", "--", "--settings", `{"apiKeyHelper":"/bin/h"}`)
	if !strings.Contains(r.Stdout, "raw --settings apiKeyHelper supplies the credential; no agx login is used for this run") {
		t.Errorf("dry run %s", r.Stdout)
	}
}

// TestEdge41CodexUnknownSetting: Codex --setting becomes -c, unknown keys
// are passed with a warning.
func TestEdge41CodexUnknownSetting(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "cx", "--setting", "model_reasoning_effort=high", "--setting", "brand_new_key=1")
	a := optAgent(t, r)
	joined := strings.Join(a.Argv, " ")
	if !strings.Contains(joined, `-c model_reasoning_effort="high"`) || !strings.Contains(joined, "-c brand_new_key=1") {
		t.Errorf("argv %q", a.Argv)
	}
	if !strings.Contains(r.Stderr, "--setting brand_new_key: not modelled") {
		t.Errorf("stderr %s", r.Stderr)
	}
}

// TestOptGeminiRefusesSetting is recipe 10: no settings flag, typed reason.
func TestOptGeminiRefusesSetting(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--profile", "gm", "--setting", "a=1", "--json"), "unsupported", "gemini has no --option or --setting")
	optFail(t, f.Run("run", "--profile", "pi", "--option", "a=1", "--json"), "unsupported", "pi has no --option or --setting")
}

// TestEdge10OptionNeedsVersion: an option newer than the installed
// version (from npm install metadata) is refused.
func TestEdge10OptionNeedsVersion(t *testing.T) {
	f := optFixture(t)
	pkg := filepath.Join(f.Root, "npm", "lib", "node_modules", "@anthropic-ai", "claude-code")
	f.WriteFile(filepath.Join(pkg, "package.json"), `{"name":"@anthropic-ai/claude-code","version":"1.0.0"}`, 0o644)
	cli := f.WriteFile(filepath.Join(pkg, "cli.js"), f.ReadFile(filepath.Join(f.Bin, "claude")), 0o755)
	if err := os.Remove(filepath.Join(f.Bin, "claude")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cli, filepath.Join(f.Bin, "claude")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	r := f.Run("run", "--option", "max_budget_usd=5", "--json")
	optFail(t, r, "feature-unavailable", "1.0.0")
}

// TestEdge33CodexAcceptEdits, 34, 31, 32, 32a: autonomy and login rules
// agentio reports, surfaced as launch refusals.
func TestEdge33CodexAcceptEdits(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "cx", "--autonomy", "accept-edits", "--json")
	optFail(t, r, "unsupported", "no edits-only mode")
}

// TestEdge34CodexStoredLogin: Codex refuses a stored token login.
func TestEdge34CodexStoredLogin(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "cx", "--login", "cxlogin", "--json")
	optFail(t, r, "unsupported", "codex has no token login")
}

// TestEdge31PiNotice: pi without --autonomy runs with a notice.
func TestEdge31PiNotice(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--profile", "pi")
	optAgent(t, r)
	if !strings.Contains(r.Stderr, "pi runs with full autonomy and never asks") {
		t.Errorf("stderr %s", r.Stderr)
	}
}

// TestEdge32PiAsk: pi refuses ask and accept-edits.
func TestEdge32PiAsk(t *testing.T) {
	f := optFixture(t)
	for _, lvl := range []string{"ask", "accept-edits"} {
		optFail(t, f.Run("run", "--profile", "pi", "--autonomy", lvl, "--json"), "unsupported", "pi has no permission modes; it always runs without prompts")
	}
}

// TestEdge32aPiReadOnly: pi read-only runs with the tools allowlist.
func TestEdge32aPiReadOnly(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--profile", "pi", "--autonomy", "read-only"))
	if !strings.Contains(strings.Join(a.Argv, " "), "--tools read,grep,find,ls --no-mcp --no-extensions") {
		t.Errorf("argv %q", a.Argv)
	}
}

// TestOptDryRunJSONGolden pins the plan with options and settings.
func TestOptDryRunJSONGolden(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--option", "effort=max", "--setting", "cleanupPeriodDays=30", "--env", "DEBUG=1", "--dry-run", "--json")
	if r.Code != 0 {
		t.Fatalf("code %d: %s", r.Code, r.Stderr)
	}
	f.Golden(t, "run-dry-run-options", r.Stdout)
}

// TestSettingApiKeyHelperHomeLoginNote is recipe 2/4 and spec 7.1 rule 4:
// a settings key that replaces the login is allowed, with a note.
func TestSettingApiKeyHelperHomeLoginNote(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--setting", "apiKeyHelper=/bin/h", "--dry-run")
	if r.Code != 0 || !strings.Contains(r.Stdout, "--setting apiKeyHelper supplies the credential; no agx login is used for this run") {
		t.Errorf("code %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
}

// specCitation matches a citation of the internal design spec ("spec 7.1",
// "spec section 15"): readers of agx's output never have that document,
// so user-facing text says the rule in words instead.
var specCitation = regexp.MustCompile(`(?i)\bspec (section )?§?[0-9]`)

// TestUserTextCitesNoSpecSection guards the texts that cited spec
// sections: the dry-run winner note, the config init starter and every
// command's help (help --json carries every flag's usage).
func TestUserTextCitesNoSpecSection(t *testing.T) {
	// config init refuses over an existing config: it gets a bare machine.
	bare := testkit.New(t)
	for name, run := range map[string]func() testkit.Result{
		"winner note": func() testkit.Result {
			return optFixture(t).Run("run", "--profile", "inline", "--setting", "apiKeyHelper=/bin/h", "--dry-run")
		},
		"config init": func() testkit.Result {
			return bare.Run("config", "init", "--default-agent", "claude", "--dry-run", "--json")
		},
		"help": func() testkit.Result { return bare.Run("help", "--json") },
	} {
		r := run()
		if r.Code != 0 {
			t.Fatalf("%s: code %d: %s", name, r.Code, r.Stderr)
		}
		if m := specCitation.FindString(r.Stdout + r.Stderr); m != "" {
			t.Errorf("%s cites the spec: %q", name, m)
		}
	}
}
