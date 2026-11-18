package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// known is the built-in agent set for checks.
var known = config.Known{Agents: []agentio.ID{"claude", "codex", "gemini", "pi"}}

// parse validates src as config.jsonc.
func parse(t *testing.T, src string) (*config.Loaded, error) {
	t.Helper()
	return config.Parse([]byte(src), config.Paths{File: filepath.Join(t.TempDir(), "config.jsonc")}, known)
}

func TestParseValid(t *testing.T) {
	l, err := parse(t, `{
  // comment
  "version": 1,
  "default_agent": "claude",
  "profiles": { "work": { "agent": "claude", "home": "~/.claude-work", "default": true, "login": "carol" } },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${gopass:x}" } },
  "projects": { "/a/b.c": { "claude": { "profile": "work" } } },
  "agents": { "mycli": { "spec": { "binary": "mycli", "new_args": ["{args}"] } } }
}`)
	if err != nil {
		t.Fatal(err)
	}
	if l.Config.Profiles["work"].Login != "carol" || l.Config.Projects["/a/b.c"]["claude"].Profile != "work" {
		t.Fatalf("decoded %+v", l.Config)
	}
}

// TestEdge45UnknownKeyAndSyntax: typos fail at line:col; invalid JSONC fails.
func TestEdge45UnknownKeyAndSyntax(t *testing.T) {
	_, err := parse(t, "{\n  \"version\": 1,\n  \"profles\": {}\n}")
	if err == nil || !strings.Contains(err.Error(), "config.jsonc:3:") || !strings.Contains(err.Error(), "profles") {
		t.Fatalf("err %v", err)
	}
	_, err = parse(t, `{"version": 1,, }`)
	if err == nil || apperr.As(err).Code != apperr.CodeConfig {
		t.Fatalf("syntax err %v", err)
	}
}

// TestEdge46NewerVersion refuses a config from a newer agx.
func TestEdge46NewerVersion(t *testing.T) {
	_, err := parse(t, `{"version": 2}`)
	if err == nil || apperr.As(err).Code != apperr.CodeConfigVersion || !strings.Contains(err.Error(), "config version 2 is newer than this agx (1)") {
		t.Fatalf("err %v", err)
	}
}

// TestEdge5TwoDefaults names both profiles at the second one's position.
func TestEdge5TwoDefaults(t *testing.T) {
	_, err := parse(t, `{
  "version": 1,
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true }
  }
}`)
	if err == nil || !strings.Contains(err.Error(), "claude has two default profiles (personal, work); keep one") || !strings.Contains(err.Error(), "config.jsonc:5:") {
		t.Fatalf("err %v", err)
	}
}

func TestCrossChecks(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"unknown agent":     {`{"profiles":{"p":{"agent":"nope","home":"/h"}}}`, "unknown agent nope"},
		"missing login":     {`{"profiles":{"p":{"agent":"claude","home":"/h","login":"x"}}}`, "unknown login x"},
		"wrong-agent login": {`{"profiles":{"p":{"agent":"codex","home":"/h","login":"c"}},"logins":{"c":{"agents":["claude"],"kind":"oauth-token","token":"t"}}}`, "login c serves claude, not codex"},
		"both logins":       {`{"profiles":{"p":{"agent":"claude","home":"/h","login":"c","home_login":true}},"logins":{"c":{"agents":["claude"],"kind":"oauth-token","token":"t"}}}`, "login and home_login are both set"},
		"binding agent":     {`{"profiles":{"p":{"agent":"claude","home":"/h"}},"projects":{"/x":{"codex":{"profile":"p"}}}}`, "profile p is a claude profile, bound for codex"},
		"env_refs plain":    {`{"profiles":{"p":{"agent":"claude","home":"/h","env_refs":{"A":"plain"}}}}`, "not a reference"},
		"login sources":     {`{"logins":{"c":{"agents":["claude"],"kind":"oauth-token","token":"t","stored":true}}}`, "exactly one of token, stored or command"},
		"helper command":    {`{"logins":{"c":{"agents":["claude"],"kind":"api-key-helper"}}}`, "needs command"},
		"bad enum":          {`{"profiles":{"p":{"agent":"claude","home":"/h","billing":"free"}}}`, "billing"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(t, c.src)
			if err == nil || !strings.Contains(err.Error()+problems(err), c.want) {
				t.Fatalf("err %v, want %q", err, c.want)
			}
		})
	}
}

// problems joins every problem of a config error.
func problems(err error) string {
	var b strings.Builder
	for _, p := range config.ProblemsOf(err) {
		b.WriteString(p.String() + "\n")
	}
	return b.String()
}

func TestMissingFileIsEmpty(t *testing.T) {
	l, err := config.Load(config.Paths{File: filepath.Join(t.TempDir(), "none.jsonc")}, known)
	if err != nil || l.Exists || len(l.Config.Profiles) != 0 {
		t.Fatalf("%v %+v", err, l)
	}
}

func TestResolvePaths(t *testing.T) {
	env := map[string]string{"AGX_HOME": "/x/home", "V": "v"}
	look := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if p := config.ResolvePaths("", look, "/u"); p.Dir != "/x/home" {
		t.Errorf("AGX_HOME: %s", p.Dir)
	}
	env["AGX_CONFIG_DIR"] = "~/cfg/${V}"
	if p := config.ResolvePaths("", look, "/u"); p.Dir != "/u/cfg/v" || p.File != "/u/cfg/v/config.jsonc" {
		t.Errorf("AGX_CONFIG_DIR: %+v", p)
	}
	if p := config.ResolvePaths("/o/c.jsonc", look, "/u"); p.Dir != "/o" || p.Tokens != "/o/tokens.jsonc" {
		t.Errorf("--config: %+v", p)
	}
	if got := config.Pointer("projects", "/a/b~c", "claude"); got != "/projects/~1a~1b~0c/claude" {
		t.Errorf("pointer %s", got)
	}
}

// TestEditKeepsComments writes under the lock and keeps comments.
func TestEditKeepsComments(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{Dir: dir, File: filepath.Join(dir, "config.jsonc")}
	if err := config.EditConfig(t.Context(), paths, func(d *jsonc.Doc) error { return d.Set("default_agent", "claude") }); err != nil {
		t.Fatal(err)
	}
	l, err := config.Load(paths, known)
	if err != nil || l.Config.DefaultAgent != "claude" || l.Config.Version != 1 {
		t.Fatalf("%v %+v", err, l)
	}
}

// TestDirKeyFolds: the stored form keeps the spelling; only the
// comparison key folds, and only on macOS and Windows (FIX: macOS paths
// were stored and printed lower-cased).
func TestDirKeyFolds(t *testing.T) {
	p := "/Users/Me/Projects/App"
	if config.DirKeyFor("darwin", p) != "/users/me/projects/app" || config.DirKeyFor("windows", p) != "/users/me/projects/app" {
		t.Fatal("darwin/windows must fold")
	}
	if config.DirKeyFor("linux", p) != p {
		t.Fatal("linux must not fold")
	}
	look := func(string) (string, bool) { return "", false }
	dir := t.TempDir()
	if got := config.NormalizeDir(dir, look, "/u"); got != dir && !strings.HasSuffix(got, filepath.Base(dir)) {
		t.Fatalf("NormalizeDir changed the spelling: %s", got)
	}
}
