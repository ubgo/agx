package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/atomicfile"

	"github.com/ubgo/agx/internal/testkit"
)

func TestConfigInitDryRunThenWrite(t *testing.T) {
	f := testkit.New(t)
	f.WriteFile(filepath.Join(f.HomeDir(".claude"), "settings.json"), "{}", 0o644)
	f.WriteFile(filepath.Join(f.HomeDir(".codex"), "config.toml"), "", 0o644)
	cfgPath := filepath.Join(f.ConfigDir, "config.jsonc")

	r := f.Run("config", "init", "--default-agent", "claude", "--discovered", "--dry-run", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "config-init-dry-run", r.Stdout)
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("--dry-run wrote the file")
	}

	// Without a terminal and without flags: no question, no profiles.
	r = f.Run("config", "init", "--dry-run")
	wantCode(t, r, 0)
	if strings.Contains(r.Stdout, `"profiles"`) || strings.Contains(r.Stdout, "default_agent\":") {
		t.Errorf("bare init offered something without a terminal:\n%s", r.Stdout)
	}

	wantCode(t, f.Run("config", "init", "--default-agent", "claude", "--discovered"), 0)
	cfg := f.ReadFile(cfgPath)
	for _, want := range []string{"// The agent used when no --profile", `"default_agent": "claude"`, `"personal"`, `"codex"`, `"$schema"`} {
		if !strings.Contains(cfg, want) {
			t.Errorf("written config lacks %q:\n%s", want, cfg)
		}
	}
	if strings.Index(cfg, `"$schema"`) > strings.Index(cfg, `"version"`) {
		t.Errorf("$schema is not first:\n%s", cfg)
	}
	wantCode(t, f.Run("config", "check"), 0)
	// A run now works from the starter.
	f.FakeAgent("claude")
	wantCode(t, f.Run("run", "--dry-run"), 0)

	wantErr(t, f.Run("config", "init", "--json"), 1, "config", "already exists")
	wantErr(t, testkit.New(t).Run("config", "init", "--default-agent", "nope", "--json"), 1, "not-found", "unknown agent nope")
}

func TestConfigPathShowCheck(t *testing.T) {
	f := fixtureProfiles(t)
	r := f.Run("config", "path")
	wantCode(t, r, 0)
	if strings.TrimSpace(r.Stdout) != filepath.Join(f.ConfigDir, "config.jsonc") {
		t.Errorf("path: %s", r.Stdout)
	}
	r = f.Run("config", "path", "--json")
	if !strings.Contains(r.Stdout, `"source": "AGX_CONFIG_DIR"`) || !strings.Contains(r.Stdout, `"exists": true`) {
		t.Errorf("path json: %s", r.Stdout)
	}

	f.Env["TOKVAR"] = "fake-token-BETA"
	r = f.Run("config", "show", "--json")
	wantCode(t, r, 0)
	if strings.Contains(r.Stdout, "fake-token-ALPHA") || strings.Contains(r.Stdout, "fake-token-BETA") {
		t.Fatalf("config show printed a secret: %s", r.Stdout)
	}
	f.Golden(t, "config-show", r.Stdout)

	r = f.Run("config", "check", "--json")
	wantCode(t, r, 0)
	f.Golden(t, "config-check", r.Stdout)

	// Without a file: path and check still work.
	g := testkit.New(t)
	wantCode(t, g.Run("config", "path"), 0)
	r = g.Run("config", "check")
	wantCode(t, r, 0)
	if !strings.Contains(r.Stdout, "no config file") {
		t.Errorf("check without a file: %s", r.Stdout)
	}
}

func TestConfigSetValidatesAndKeepsComments(t *testing.T) {
	f := fixtureProfiles(t)
	cfgPath := filepath.Join(f.ConfigDir, "config.jsonc")
	wantCode(t, f.Run("config", "set", "--key", "default_agent", "--value", "codex"), 0)
	wantCode(t, f.Run("config", "set", "--key", "profiles.work.model", "--value", "opus"), 0)
	wantCode(t, f.Run("config", "set", "--key", "profiles.work.live_login", "--value", "true"), 0)
	r := f.Run("config", "set", "--key", "logins.carol.token", "--value", "fake-token-GAMMA", "--json")
	wantCode(t, r, 0)
	if strings.Contains(r.Stdout+r.Stderr, "fake-token-GAMMA") {
		t.Errorf("config set echoed a token: %s", r.Stdout)
	}
	cfg := f.ReadFile(cfgPath)
	for _, want := range []string{"// keep this comment", `"default_agent": "codex"`, `"model": "opus"`, `"live_login": true`} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %q:\n%s", want, cfg)
		}
	}
	before := f.ReadFile(cfgPath)
	wantErr(t, f.Run("config", "set", "--key", "default_agent", "--value", "nope", "--json"), 2, "config", "unknown agent nope")
	wantErr(t, f.Run("config", "set", "--key", "profiles.work.billing", "--value", "free", "--json"), 2, "config", "billing")
	wantErr(t, f.Run("config", "set", "--key", "profiles", "--value", "x", "--json"), 2, "usage", "--key profiles is an object, not a scalar")
	wantErr(t, f.Run("config", "set", "--key", "profiles.work.args", "--value", "x", "--json"), 2, "usage", "--key profiles.work.args is a list, not a scalar")
	wantErr(t, f.Run("config", "set", "--key", "defualt_agent", "--value", "x", "--json"), 2, "usage", "unknown key defualt_agent")
	wantErr(t, f.Run("config", "set", "--key", "profiles.work.live_login", "--value", "maybe", "--json"), 2, "usage", "true or false")
	wantErr(t, f.Run("config", "set", "--value", "x", "--json"), 2, "usage", "--key is required")
	if f.ReadFile(cfgPath) != before {
		t.Error("a refused set wrote the file")
	}
}

// TestEdge05TwoDefaultProfiles: config load fails with the position and
// both names, exit 2.
func TestEdge05TwoDefaultProfiles(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(`{
  "version": 1,
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true }
  }
}`)
	r := f.Run("profile", "list")
	wantCode(t, r, 2)
	if !strings.Contains(r.Stderr, "config.jsonc:5:60: claude has two default profiles (personal, work); keep one") {
		t.Errorf("message: %s", r.Stderr)
	}
	wantCode(t, f.Run("config", "check"), 2)
}

// TestEdge44ConfigLocked: a second writer waits 5 s for the lock, then
// fails with exit 1.
func TestEdge44ConfigLocked(t *testing.T) {
	f := fixtureProfiles(t)
	lk, err := atomicfile.Lock(context.Background(), filepath.Join(f.ConfigDir, "config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lk.Close(); err != nil {
			t.Error(err)
		}
	}()
	r := f.Run("config", "set", "--key", "default_agent", "--value", "codex", "--json")
	wantErr(t, r, 1, "locked", "is being written by another agx")
	// Reads never take the lock.
	wantCode(t, f.Run("profile", "list"), 0)
}

// TestEdge45InvalidJSONC: the load fails at the syntax error's position;
// config check lists every problem (here: several schema problems).
func TestEdge45InvalidJSONC(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig("{\n  \"version\": 1,\n  \"profiles\": {\n    \"a\": { \"agent\": \"claude\" \"home\": \"~/x\" }\n  }\n}\n")
	r := f.Run("profile", "list")
	wantCode(t, r, 1)
	if !strings.Contains(r.Stderr, "config.jsonc:4:") {
		t.Errorf("no position: %s", r.Stderr)
	}
	f.FakeAgent("claude")
	wantCode(t, f.Run("run", "--profile", "a"), 125)

	f.WriteConfig(`{
  "version": 1,
  "profles": {},
  "profiles": { "a": { "agent": "claude", "home": "~/x", "biling": "api", "default": "yes" } },
  "aliases": { "x": 3 }
}`)
	r = f.Run("config", "check", "--json")
	wantCode(t, r, 1)
	e := pfParse(t, r)
	if e.Error == nil || !strings.Contains(e.Error.Message, "problems") || strings.Count(e.Error.Hint, "config.jsonc:") < 4 {
		t.Errorf("check did not list every problem: %+v", e.Error)
	}
	r = f.Run("config", "check")
	for _, want := range []string{"profles", "biling", "default", "aliases.x"} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("check lacks %q:\n%s", want, r.Stderr)
		}
	}
}

// TestEdge46NewerConfigVersion: a config written by a newer agx is
// refused, exit 1.
func TestEdge46NewerConfigVersion(t *testing.T) {
	f := testkit.New(t)
	f.WriteConfig(`{ "version": 2 }`)
	r := f.Run("config", "check", "--json")
	wantErr(t, r, 1, "config-version", "config version 2 is newer than this agx (1)")
	wantCode(t, f.Run("profile", "list"), 1)
}
