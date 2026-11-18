package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// Raw --settings (spec decision 27, edge 39): agx reads every raw
// --settings value (inline JSON or a JSONC file), deep-merges it over its
// own settings in argv order, takes it out of the args and passes exactly
// one --settings. Claude itself keeps only the last --settings wholesale
// (live check V7), so two would silently drop agx's settings, including
// the live-login helper.

// rawSettingsFlag is Claude's settings flag as the fake agent receives it.
const rawSettingsFlag = "--settings"

// settingsCount counts the --settings flags in argv (both forms).
func settingsCount(argv []string) int {
	n := 0
	for _, a := range argv {
		if a == rawSettingsFlag || strings.HasPrefix(a, rawSettingsFlag+"=") {
			n++
		}
	}
	return n
}

// deliveredSettings is the one settings object the agent got: the file
// agx wrote, or the inline value.
func deliveredSettings(t *testing.T, f *testkit.Fixture, a testkit.Agent) map[string]json.RawMessage {
	t.Helper()
	if n := settingsCount(a.Argv); n != 1 {
		t.Fatalf("%d --settings in argv %q, want exactly one", n, a.Argv)
	}
	v := argAfter(a.Argv, rawSettingsFlag)
	if strings.HasPrefix(strings.TrimSpace(v), "{") {
		return readJSONObject(t, v)
	}
	return readJSONObject(t, f.ReadFile(v))
}

// TestRawSettingsInlineMerged: an inline raw --settings after -- merges
// over the profile's settings key by key (raw wins a shared key; objects
// merge, so permissions keeps the profile's allow and gains deny).
func TestRawSettingsInlineMerged(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--", "--settings", `{"theme":"dark","cleanupPeriodDays":30,"permissions":{"deny":["Bash"]}}`)
	m := deliveredSettings(t, f, optAgent(t, r))
	if string(m["cleanupPeriodDays"]) != "30" || string(m["theme"]) != `"dark"` {
		t.Errorf("settings %v", m)
	}
	if p := string(m["permissions"]); !strings.Contains(p, `"allow":["Read"]`) || !strings.Contains(p, `"deny":["Bash"]`) {
		t.Errorf("permissions not merged: %s", p)
	}
	if strings.Contains(r.Stderr, "not applied") {
		t.Errorf("old warning still printed: %s", r.Stderr)
	}
}

// TestRawSettingsArrayReplaces: an array (or scalar) replaces, never
// appends: the raw allow list is the whole list.
func TestRawSettingsArrayReplaces(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--settings-delivery", "inline", "--", "--settings", `{"permissions":{"allow":["Edit"]}}`))
	if p := string(deliveredSettings(t, f, a)["permissions"]); p != `{"allow":["Edit"]}` {
		t.Errorf("permissions %s", p)
	}
}

// TestRawSettingsFile: a JSONC file named relative to the launch folder,
// in the --settings=FILE form, from profile args with "~", and with no
// agx settings at all (still read and passed as one file agx wrote).
func TestRawSettingsFile(t *testing.T) {
	f := optFixture(t)
	f.WriteFile(filepath.Join(f.Work, "team.jsonc"), "{\n  // shared\n  \"theme\": \"dark\",\n}\n", 0o644)
	m := deliveredSettings(t, f, optAgent(t, f.Run("run", "--", "--settings=team.jsonc")))
	if string(m["theme"]) != `"dark"` || string(m["cleanupPeriodDays"]) != "7" {
		t.Errorf("settings %v", m)
	}

	f.WriteFile(filepath.Join(f.Home, "raw.json"), `{"theme":"light"}`, 0o644)
	f.WriteConfig(strings.Replace(optConfig, `"cx": {`, `"plain": { "agent": "claude", "home": "~/.claude-work", "args": ["--settings", "~/raw.json"] },
    "cx": {`, 1))
	a := optAgent(t, f.Run("run", "--profile", "plain"))
	path := argAfter(a.Argv, rawSettingsFlag)
	if filepath.Base(path) != "settings-"+a.Env["AGX_RUN_ID"]+".json" {
		t.Errorf("raw settings not normalised into the run folder: %q", a.Argv)
	}
	if m := deliveredSettings(t, f, a); string(m["theme"]) != `"light"` || len(m) != 1 {
		t.Errorf("settings %v", m)
	}
}

// TestRawSettingsTwoValues: two raw --settings merge in argv order (the
// later wins key by key), unlike Claude's own last-wins-wholesale.
func TestRawSettingsTwoValues(t *testing.T) {
	f := optFixture(t)
	a := optAgent(t, f.Run("run", "--", "--settings", `{"theme":"dark","cleanupPeriodDays":1}`, "--settings={\"cleanupPeriodDays\":2}"))
	m := deliveredSettings(t, f, a)
	if string(m["theme"]) != `"dark"` || string(m["cleanupPeriodDays"]) != "2" {
		t.Errorf("settings %v", m)
	}
}

// TestRawSettingsDryRunSources: the dry run sources each raw key "from
// raw --settings", names the agx keys it overrode, keeps the rest under
// the profile, and the args line no longer shows the raw value.
func TestRawSettingsDryRunSources(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--dry-run", "--json", "--field", "decisions", "--", "--settings", `{"theme":"dark","cleanupPeriodDays":30}`)
	if r.Code != 0 {
		t.Fatalf("code %d %s", r.Code, r.Stderr)
	}
	var decs []struct{ Name, Value, Source string }
	if err := json.Unmarshal([]byte(r.Stdout), &decs); err != nil {
		t.Fatalf("decisions %v: %s", err, r.Stdout)
	}
	got := map[string]string{}
	for _, d := range decs {
		if d.Name == "settings" || d.Name == "args" {
			got[d.Value] = d.Source
		}
	}
	if src := got["cleanupPeriodDays, theme"]; !strings.HasPrefix(src, "raw --settings") || !strings.Contains(src, "overrides profile work") || !strings.Contains(src, "cleanupPeriodDays") {
		t.Errorf("raw decision source %q in %v", src, got)
	}
	if src, ok := got["permissions.allow"]; !ok || !strings.HasPrefix(src, "profile work") {
		t.Errorf("profile decision %v", got)
	}
	for v := range got {
		if strings.Contains(v, "--settings") {
			t.Errorf("args still show the raw --settings: %q", v)
		}
	}
	text := f.Run("run", "--dry-run", "--", "--settings", `{"theme":"dark"}`)
	if !strings.Contains(text.Stdout, "from raw --settings") {
		t.Errorf("dry run text %s", text.Stdout)
	}
}

// TestRawSettingsBadInput: an unreadable or invalid raw --settings is an
// error naming the file (line:col for bad JSONC), a flag without a value
// is a usage error, and a raw type error names the key.
func TestRawSettingsBadInput(t *testing.T) {
	f := optFixture(t)
	optFail(t, f.Run("run", "--json", "--", "--settings", "missing.json"), "not-found", "raw --settings file "+filepath.Join(f.Work, "missing.json")+" not found")
	f.WriteFile(filepath.Join(f.Work, "bad.jsonc"), "{\n  \"theme\": ,\n}\n", 0o644)
	optFail(t, f.Run("run", "--json", "--", "--settings", "bad.jsonc"), "config", "bad.jsonc:2:")
	f.WriteFile(filepath.Join(f.Work, "list.json"), "[1]", 0o644)
	optFail(t, f.Run("run", "--json", "--", "--settings", "list.json"), "usage", "list.json: not a JSON object")
	optFail(t, f.Run("run", "--json", "--", "--settings", `{"theme": }`), "usage", "raw --settings:1:")
	optFail(t, f.Run("run", "--json", "--", "--settings"), "usage", "raw --settings needs a value")
	optFail(t, f.Run("run", "--json", "--", "--settings", `{"cleanupPeriodDays":"x"}`), "usage", "raw --settings cleanupPeriodDays")
}

// TestRawSettingsLiveLogin: with --live-login, a raw --settings merges
// with the helper settings (no longer refused), and one that sets the
// helper key itself is a conflict naming both (exit 125).
func TestRawSettingsLiveLogin(t *testing.T) {
	f, _ := fixtureResume(t)
	r := f.Run("run", "--profile", "work", "--login", "carol", "--live-login", "--settings-delivery", "inline", "--", "--settings", `{"theme":"dark"}`)
	a := optAgent(t, r)
	m := deliveredSettings(t, f, a)
	var helper string
	if err := json.Unmarshal(m["apiKeyHelper"], &helper); err != nil || !strings.HasSuffix(helper, "credential") || string(m["theme"]) != `"dark"` {
		t.Errorf("settings %v", m)
	}
	c := f.Run("run", "--profile", "work", "--login", "carol", "--live-login", "--dry-run", "--json", "--", "--settings", `{"apiKeyHelper":"/bin/h"}`)
	e := parseEnv(t, c)
	if c.Code != 125 || e.Error.Code != "conflict" || !strings.Contains(e.Error.Message, "raw --settings apiKeyHelper and --live-login both set") {
		t.Errorf("code %d %+v", c.Code, e.Error)
	}
}

// TestRawSettingsInExtraArgs: a --settings in a Claude block's extra_args
// is a raw arg too: merged first (extra_args go before the other raw
// args), and the rest of extra_args still reach the agent.
func TestRawSettingsInExtraArgs(t *testing.T) {
	f := optFixture(t)
	f.WriteConfig(strings.Replace(optConfig, `"cx": {`, `"xa": { "agent": "claude", "home": "~/.claude-work", "claude": { "extra_args": ["--verbose", "--settings", "{\"theme\":\"dark\",\"cleanupPeriodDays\":3}"], "settings": { "cleanupPeriodDays": 9 } } },
    "cx": {`, 1))
	a := optAgent(t, f.Run("run", "--profile", "xa", "--", "--settings", `{"theme":"light"}`))
	m := deliveredSettings(t, f, a)
	if string(m["theme"]) != `"light"` || string(m["cleanupPeriodDays"]) != "3" {
		t.Errorf("settings %v", m)
	}
	if !strings.Contains(strings.Join(a.Argv, " "), "--verbose") {
		t.Errorf("extra_args lost: %q", a.Argv)
	}
}

// TestRawSettingsOneWinner: raw settings keys rank as settings from the
// profile side: a command-line --login or --model beats them, with a note
// naming both, and is never a conflict.
func TestRawSettingsOneWinner(t *testing.T) {
	f := optFixture(t)
	r := f.Run("run", "--login", "carol", "--model", "opus", "--dry-run", "--", "--settings", `{"apiKeyHelper":"/bin/h","model":"haiku"}`)
	if r.Code != 0 {
		t.Fatalf("code %d %s", r.Code, r.Stdout+r.Stderr)
	}
	for _, want := range []string{
		"--login carol wins the credential; raw --settings apiKeyHelper is not used",
		"--model wins the model; raw --settings model is not used",
	} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("dry run lacks %q:\n%s", want, r.Stdout)
		}
	}
}
