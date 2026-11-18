package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/testkit"
)

// profilesConfig is a machine with two Claude homes (one shared by two
// profiles), a Codex home, a login and a binding.
const profilesConfig = `{
  "version": 1,
  "default_agent": "claude",
  // keep this comment
  "agents": { "claude": { "defaults": { "args": ["--chrome"] } } },
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "kimi": {
      "agent": "claude",
      "home": "~/.claude",
      "billing": "api",
      "inherit_defaults": false,
      "args": ["--verbose"],
      "env": { "ANTHROPIC_AUTH_TOKEN": "fake-token-ALPHA" },
      "env_refs": { "OTHER": "${TOKVAR}" }
    },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${TOKVAR}" }
  },
  "projects": {}
}`

// fixtureProfiles builds the machine of profilesConfig: homes with
// markers and shared files, fake agents on PATH.
func fixtureProfiles(t *testing.T) *testkit.Fixture {
	t.Helper()
	f := testkit.New(t)
	claude := f.HomeDir(".claude")
	f.WriteFile(filepath.Join(claude, "settings.json"), "{}", 0o644)
	f.WriteFile(filepath.Join(claude, "CLAUDE.md"), "memory", 0o644)
	f.HomeDir(".claude/skills")
	work := f.HomeDir(".claude-work")
	f.WriteFile(filepath.Join(work, "settings.json"), "{}", 0o644)
	f.WriteFile(filepath.Join(f.HomeDir(".codex"), "config.toml"), "", 0o644)
	f.FakeAgent("claude")
	f.FakeAgent("codex")
	f.WriteConfig(profilesConfig)
	return f
}

// pfEnvelope is the JSON pfEnvelope's common part.
type pfEnvelope struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	Decisions []struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Source string `json:"source"`
	} `json:"decisions"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

// pfParse decodes r.Stdout as the pfEnvelope.
func pfParse(t *testing.T, r testkit.Result) pfEnvelope {
	t.Helper()
	var e pfEnvelope
	if err := json.Unmarshal([]byte(r.Stdout), &e); err != nil {
		t.Fatalf("not an pfEnvelope: %v\nstdout %s\nstderr %s", err, r.Stdout, r.Stderr)
	}
	return e
}

// wantCode fails unless r exited with code.
func wantCode(t *testing.T, r testkit.Result, code int) {
	t.Helper()
	if r.Code != code {
		t.Fatalf("exit %d, want %d\nstdout %s\nstderr %s", r.Code, code, r.Stdout, r.Stderr)
	}
}

// wantErr fails unless r is a JSON failure with code and a message
// containing msg.
func wantErr(t *testing.T, r testkit.Result, exit int, code, msg string) {
	t.Helper()
	wantCode(t, r, exit)
	e := pfParse(t, r)
	if e.Error == nil || e.Error.Code != code || !strings.Contains(e.Error.Message, msg) {
		t.Fatalf("error %+v, want code %s with %q", e.Error, code, msg)
	}
}

func TestProfileListJSONGolden(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("profile", "list", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "profile-list", r.Stdout)

	r = f.Run("profile", "list", "--agent", "codex", "--json", "--field", "data.*.name")
	wantCode(t, r, 0)
	if strings.TrimSpace(r.Stdout) != `["codex"]` {
		t.Errorf("--agent codex: %s", r.Stdout)
	}
	r = f.Run("profile", "list", "--profile", "work", "--profile", "kimi", "--json", "--field", "data.*.name")
	// The file's order (personal, work, kimi, codex), as v0.3 listed.
	if strings.TrimSpace(r.Stdout) != `["work","kimi"]` {
		t.Errorf("--profile filter: %s", r.Stdout)
	}
	wantErr(t, f.Run("profile", "list", "--profile", "nope", "--json"), 1, "not-found", "unknown profile nope")
}

func TestProfileListTableAndPlanNotModelled(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("profile", "list")
	wantCode(t, r, 0)
	// v0.3's columns: "*" marks each agent's default, STATE the home login.
	for _, want := range []string{"PROFILE", "PROVIDER", "ACCOUNT", "STATE", "*personal", "*codex", "kimi", "~/.claude-work", "api", "home login"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("table lacks %q:\n%s", want, r.Stdout)
		}
	}
}

func TestProfileShowSourcesNeverPrintsEnvValues(t *testing.T) {
	f := fixtureProfiles(t)
	f.Env["TOKVAR"] = "fake-token-BETA"
	r := f.Run("profile", "show", "--id", "kimi", "--json")
	wantCode(t, r, 0)
	if strings.Contains(r.Stdout, "fake-token-ALPHA") || strings.Contains(r.Stdout, "fake-token-BETA") {
		t.Fatalf("a secret value was printed: %s", r.Stdout)
	}
	f.Golden(t, "profile-show", r.Stdout)

	r = f.Run("profile", "show", "--id", "work")
	wantCode(t, r, 0)
	for _, want := range []string{"agents.claude.defaults (config.jsonc:", "--chrome", "profile work (config.jsonc:"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("show work lacks %q:\n%s", want, r.Stdout)
		}
	}
	wantErr(t, f.Run("profile", "show", "--json"), 2, "usage", "--id is required")
}

func TestProfileAddDerivedHomeLinksSharedFiles(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("profile", "add", "--name", "team", "--agent", "claude", "--dry-run", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "profile-add-dry-run", r.Stdout)
	if _, err := os.Stat(filepath.Join(f.Home, ".claude-team")); err == nil {
		t.Fatal("--dry-run created the home")
	}
	if strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), "team") {
		t.Fatal("--dry-run wrote the config")
	}

	r = f.Run("profile", "add", "--name", "team", "--agent", "claude")
	wantCode(t, r, 0)
	home := filepath.Join(f.Home, ".claude-team")
	st, err := os.Stat(home)
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("home not created 0700: %v %v", st, err)
	}
	for _, name := range []string{"settings.json", "CLAUDE.md", "skills"} {
		to, err := os.Readlink(filepath.Join(home, name))
		if err != nil || to != filepath.Join(f.Home, ".claude", name) {
			t.Errorf("link %s: %q %v", name, to, err)
		}
	}
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if !strings.Contains(cfg, "// keep this comment") || !strings.Contains(cfg, `"home": "~/.claude-team"`) {
		t.Errorf("config after add:\n%s", cfg)
	}
	r = f.Run("config", "check")
	wantCode(t, r, 0)
}

func TestProfileAddBareSharedHomeAndFirstDefault(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("profile", "add", "--name", "plain", "--agent", "claude", "--bare", "--json")
	wantCode(t, r, 0)
	entries, _ := os.ReadDir(filepath.Join(f.Home, ".claude-plain"))
	if len(entries) != 0 {
		t.Errorf("--bare linked files: %v", entries)
	}
	// A second profile on an existing home: nothing created, nothing linked.
	r = f.Run("profile", "add", "--name", "kimi2", "--agent", "claude", "--home", "~/.claude", "--billing", "api", "--json")
	wantCode(t, r, 0)
	var res struct {
		HomeCreated bool              `json:"home_created"`
		Links       []json.RawMessage `json:"links"`
		Default     bool              `json:"default"`
	}
	if err := json.Unmarshal(pfParse(t, r).Data, &res); err != nil || res.HomeCreated || len(res.Links) != 0 || res.Default {
		t.Errorf("shared home add: %+v %v", res, err)
	}
	// The first gemini profile becomes gemini's default.
	f.HomeDir("gem/.gemini")
	r = f.Run("profile", "add", "--name", "gem", "--agent", "gemini", "--home", "~/gem/.gemini", "--json")
	wantCode(t, r, 0)
	if err := json.Unmarshal(pfParse(t, r).Data, &res); err != nil || !res.Default {
		t.Errorf("first gemini profile not default: %+v", res)
	}
}

func TestProfileAddRefusals(t *testing.T) {
	f := fixtureProfiles(t)
	wantErr(t, f.Run("profile", "add", "--name", "work", "--agent", "claude", "--json"), 2, "conflict", "profile work already exists")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "claude", "--login", "carol", "--home-login", "--json"), 2, "conflict", "pick one")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "codex", "--login", "carol", "--json"), 2, "wrong-agent", "login carol serves claude, not codex")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "nope", "--json"), 1, "not-found", "unknown agent nope")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "claude", "--billing", "free", "--json"), 2, "usage", "--billing")
	wantErr(t, f.Run("profile", "add", "--name", "bad name", "--agent", "claude", "--json"), 2, "usage", "--name")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--json"), 2, "usage", "--agent is required")
	// Gemini homes are PARENT/.gemini: no derived home, and a wrong name is refused.
	wantErr(t, f.Run("profile", "add", "--name", "g", "--agent", "gemini", "--json"), 2, "usage", "--home is required for gemini")
	wantErr(t, f.Run("profile", "add", "--name", "g", "--agent", "gemini", "--home", "~/gemini-x", "--json"), 4, "unsupported", "gemini homes must be named .gemini")
	// --login-setup needs a terminal for the pasted token (no TTY here).
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "claude", "--login-setup", "--json"), 2, "no-tty",
		"no terminal: --login-setup runs the setup command, then asks you to paste the token it prints; run it at a terminal, or use agx login add --stdin")
	wantErr(t, f.Run("profile", "add", "--name", "x", "--agent", "codex", "--login-setup", "--json"), 4, "unsupported", "codex")
	r := f.Run("profile", "add", "--name", "x", "--agent", "claude", "--login-setup", "--dry-run", "--json")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, `"login_setup": "claude setup-token"`) {
		t.Errorf("dry-run login setup: %s", r.Stdout)
	}
}

func TestProfileEditChangesAndDryRun(t *testing.T) {
	f := fixtureProfiles(t)
	cfgPath := filepath.Join(f.ConfigDir, "config.jsonc")
	before := f.ReadFile(cfgPath)
	r := f.Run("profile", "edit", "--id", "work", "--login", "carol", "--env", "DEBUG=1", "--billing", "api", "--dry-run", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "profile-edit-dry-run", r.Stdout)
	if f.ReadFile(cfgPath) != before {
		t.Fatal("--dry-run wrote the config")
	}
	r = f.Run("profile", "edit", "--id", "work", "--login", "carol", "--env", "DEBUG=1", "--args", "--x", "--args", "--y", "--no-inherit-defaults")
	wantCode(t, r, 0)
	r = f.Run("profile", "show", "--id", "work")
	for _, want := range []string{"carol", "--x --y", "inherit_defaults", "DEBUG=(set)"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("show after edit lacks %q:\n%s", want, r.Stdout)
		}
	}
	// --home-login replaces the login; --env NAME= removes; --args "" clears.
	wantCode(t, f.Run("profile", "edit", "--id", "work", "--home-login", "--env", "DEBUG=", "--args", "", "--inherit-defaults"), 0)
	cfg := f.ReadFile(cfgPath)
	if !strings.Contains(cfg, "// keep this comment") || strings.Contains(cfg, "DEBUG") || strings.Contains(cfg, `"--x"`) || strings.Contains(cfg, `"login": "carol"`) {
		t.Errorf("config after second edit:\n%s", cfg)
	}
	wantErr(t, f.Run("profile", "edit", "--id", "work", "--json"), 2, "usage", "nothing to change")
	wantErr(t, f.Run("profile", "edit", "--id", "codex", "--login", "carol", "--json"), 2, "wrong-agent", "serves claude")
	wantErr(t, f.Run("profile", "edit", "--id", "work", "--inherit-defaults", "--no-inherit-defaults", "--json"), 2, "conflict", "pick one")
	wantErr(t, f.Run("profile", "edit", "--id", "work", "--home", "~/missing", "--json"), 1, "home-missing", "does not exist")
}

func TestProfileUseMovesTheDefault(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("profile", "use", "--id", "work", "--json")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, `"cleared": [`) || !strings.Contains(r.Stdout, `"personal"`) {
		t.Errorf("use: %s", r.Stdout)
	}
	r = f.Run("profile", "list", "--json", "--field", "data.*.default")
	if strings.TrimSpace(r.Stdout) != `[false,true,false,true]` {
		t.Errorf("defaults after use (personal, work, kimi, codex): %s", r.Stdout)
	}
	wantCode(t, f.Run("config", "check"), 0)
}

func TestProfileRemovePlanThenRemoveKeepsHome(t *testing.T) {
	f := fixtureProfiles(t)
	wantCode(t, f.Run("project", "use", "--profile", "work"), 0)
	r := f.Run("profile", "remove", "--id", "work")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, "would remove profile work") || !strings.Contains(r.Stdout, "binding in") {
		t.Errorf("plan: %s", r.Stdout)
	}
	if !strings.Contains(f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc")), `"work"`) {
		t.Fatal("plan removed the profile")
	}
	r = f.Run("profile", "remove", "--id", "work", "--yes", "--json")
	wantCode(t, r, 0)
	if _, err := os.Stat(filepath.Join(f.Home, ".claude-work")); err != nil {
		t.Fatal("home deleted")
	}
	cfg := f.ReadFile(filepath.Join(f.ConfigDir, "config.jsonc"))
	if strings.Contains(cfg, `"work"`) {
		t.Errorf("profile or binding left:\n%s", cfg)
	}
	wantCode(t, f.Run("config", "check"), 0)
	wantErr(t, f.Run("profile", "remove", "--id", "work", "--json"), 1, "not-found", "unknown profile work")
}

// discoveredRows decodes profile list --discovered rows, keeping only the
// homes under the fixture root (the process source also sees the real
// machine's agents; their paths are never asserted on).
func discoveredRows(t *testing.T, f *testkit.Fixture, r testkit.Result) []map[string]json.RawMessage {
	t.Helper()
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(pfParse(t, r).Data, &rows); err != nil {
		t.Fatal(err)
	}
	var out []map[string]json.RawMessage
	for _, row := range rows {
		var p string
		if json.Unmarshal(row["path"], &p) == nil && (strings.HasPrefix(p, "~") || strings.HasPrefix(p, f.Root)) {
			out = append(out, row)
		}
	}
	return out
}

func TestProfileDiscoveredSources(t *testing.T) {
	f := fixtureProfiles(t)
	f.HomeDir(".claude-old")
	accounts := f.Mkdir("accounts")
	// A searched folder holding only settings.json is no agent's home:
	// that name alone counts only in an agent's own folders, so it was
	// once listed as a Claude, Gemini CLI and pi home at once.
	f.WriteFile(filepath.Join(accounts, "client", "settings.json"), "{}", 0o644)
	f.HomeDir("gem/.gemini")
	f.Env["CLAUDE_CONFIG_DIR"] = filepath.Join(f.Home, ".claude-work")
	r := f.Run("profile", "list", "--discovered", "--search", accounts, "--search", filepath.Join(f.Home, "gem"), "--json")
	wantCode(t, r, 0)
	rows := discoveredRows(t, f, r)
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	f.Golden(t, "profile-list-discovered", string(b))
	if strings.Contains(string(b), "accounts/client") {
		t.Errorf("a folder with only settings.json was discovered as a home: %s", b)
	}

	r = f.Run("profile", "list", "--discovered")
	wantCode(t, r, 0)
	for _, want := range []string{"LOOKS LIKE A HOME", "no (name only)", "pattern .claude-*", "variable CLAUDE_CONFIG_DIR", "default home"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("table lacks %q:\n%s", want, r.Stdout)
		}
	}
	wantErr(t, f.Run("profile", "list", "--search", accounts, "--json"), 2, "usage", "--search needs --discovered")
	wantErr(t, f.Run("profile", "list", "--discovered", "--search", filepath.Join(f.Root, "nope"), "--json"), 1, "not-found", "no such folder")
}

func TestProfileImportPlanThenYes(t *testing.T) {
	f := testkit.New(t)
	f.WriteFile(filepath.Join(f.HomeDir(".claude"), "settings.json"), "{}", 0o644)
	f.WriteFile(filepath.Join(f.HomeDir(".claude-acmecorp"), "settings.json"), "{}", 0o644)
	f.HomeDir(".claude-old")
	f.WriteFile(filepath.Join(f.HomeDir(".codex"), "config.toml"), "", 0o644)
	r := f.Run("profile", "import", "--discovered", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "profile-import-plan", r.Stdout)
	if _, err := os.Stat(filepath.Join(f.ConfigDir, "config.jsonc")); err == nil {
		t.Fatal("the plan wrote a config")
	}
	wantCode(t, f.Run("profile", "import", "--discovered", "--yes"), 0)
	r = f.Run("profile", "list", "--json", "--field", "data.*.name")
	if strings.TrimSpace(r.Stdout) != `["personal","acmecorp","codex"]` {
		t.Errorf("imported: %s", r.Stdout)
	}
	// The name-only home is imported by --id with a name; a second import
	// of a configured home is refused.
	r = f.Run("profile", "import", "--id", "~/.claude-old", "--name", "old", "--agent", "claude", "--yes")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, "wrote 1 profile") || strings.Contains(r.Stdout, "wrote 1 profiles") {
		t.Errorf("one import reads %q", r.Stdout)
	}
	wantErr(t, f.Run("profile", "import", "--id", "~/.claude-old", "--yes", "--json"), 2, "conflict", "already the home of claude profile old")
	wantErr(t, f.Run("profile", "import", "--json"), 2, "usage", "--discovered or --id")
	wantErr(t, f.Run("profile", "import", "--discovered", "--name", "x", "--json"), 2, "usage", "--name needs exactly one --id")
	wantErr(t, f.Run("profile", "import", "--id", f.Work, "--json"), 1, "not-found", "does not look like a home")
	wantCode(t, f.Run("config", "check"), 0)
}
