package resolve_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
)

// machine is a fixture: user home with homes, a config, a resolver.
type machine struct {
	home string
	rs   *resolve.Resolver
	env  map[string]string
}

func newMachine(t *testing.T, cfg string) *machine {
	t.Helper()
	home := t.TempDir()
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r
	}
	for _, d := range []string{".claude", ".claude-work", ".codex", "proj/client/sub", "real"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg = strings.ReplaceAll(cfg, "HOME", home)
	l, err := config.Parse([]byte(cfg), config.Paths{File: filepath.Join(home, "config.jsonc")}, config.Known{Agents: agents.BuiltinIDs()})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := agents.New(agentio.Host{UserHome: home}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := &machine{home: home, env: map[string]string{}}
	m.rs = &resolve.Resolver{Cfg: l, Agents: reg, UserHome: home, Lookup: func(k string) (string, bool) { v, ok := m.env[k]; return v, ok }}
	return m
}

const base = `{
  "version": 1,
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  },
  "logins": {
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "t" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "t" }
  },
  "projects": {
    "HOME/proj/client": { "claude": { "profile": "work", "home_login": true }, "codex": { "profile": "codex" } },
    "HOME/proj/client/sub": { "claude": { "login": "carol" } }
  }
}`

func value(r resolve.Result, name string) (string, string) {
	for _, d := range r.Decisions {
		if d.Name == name {
			return d.Value, d.Source
		}
	}
	return "", ""
}

func TestProfileFlagDecides(t *testing.T) {
	m := newMachine(t, base)
	r, err := m.rs.Resolve(resolve.Inputs{Profile: "work", Dir: m.home})
	if err != nil {
		t.Fatal(err)
	}
	if v, s := value(r, "agent"); v != "claude" || s != "profile work" {
		t.Errorf("agent %s from %s", v, s)
	}
	if r.Home.IsDefault() || r.Login.Kind != resolve.LoginHome {
		t.Errorf("home %v login %v", r.Home, r.Login)
	}
}

// TestEdge4NoAgent: nothing names an agent, no terminal.
func TestEdge4NoAgent(t *testing.T) {
	m := newMachine(t, base)
	_, err := m.rs.Resolve(resolve.Inputs{Dir: m.home})
	e := apperr.As(err)
	if e.Code != apperr.CodeNoAgent || !strings.Contains(e.Hint, "agx run --agent claude") || !strings.Contains(e.Hint, "agx config set --key default_agent --value claude") {
		t.Fatalf("%v / %s", err, e.Hint)
	}
}

// TestEdge29NestedBindings: the nearest bound folder per agent wins; the
// login comes from the nearer folder, the profile from the outer one.
func TestEdge29NestedBindings(t *testing.T) {
	m := newMachine(t, base)
	r, err := m.rs.Resolve(resolve.Inputs{Agent: "claude", Dir: filepath.Join(m.home, "proj/client/sub")})
	if err != nil {
		t.Fatal(err)
	}
	if r.ProfileName != "work" || r.Login.Name != "carol" {
		t.Fatalf("profile %s login %+v", r.ProfileName, r.Login)
	}
	if len(r.Notes) == 0 || !strings.Contains(strings.Join(r.Notes, "\n"), "folder binding") {
		t.Errorf("notes %v", r.Notes)
	}
}

// TestEdge30Symlink: a folder reached through a symlink matches.
func TestEdge30Symlink(t *testing.T) {
	m := newMachine(t, base)
	link := filepath.Join(m.home, "link")
	if err := os.Symlink(filepath.Join(m.home, "proj/client"), link); err != nil {
		t.Skip(err)
	}
	r, err := m.rs.Resolve(resolve.Inputs{Agent: "codex", Dir: link})
	if err != nil || r.ProfileName != "codex" {
		t.Fatalf("%v %s", err, r.ProfileName)
	}
	if v, s := value(r, "profile"); v != "codex" || !strings.Contains(s, "folder binding") {
		t.Errorf("profile %s from %s", v, s)
	}
}

// TestEdge11WrongAgentLogin: AGX_LOGIN naming a claude login on a codex run.
func TestEdge11WrongAgentLogin(t *testing.T) {
	m := newMachine(t, base)
	m.env["AGX_LOGIN"] = "derek"
	_, err := m.rs.Resolve(resolve.Inputs{Profile: "codex", Dir: m.home})
	if e := apperr.As(err); e.Code != apperr.CodeWrongAgent || e.Message != "login derek serves claude; this run is codex" {
		t.Fatalf("%v", err)
	}
}

// TestEdge12LoginConflict: two login choices in one command.
func TestEdge12LoginConflict(t *testing.T) {
	m := newMachine(t, base)
	_, err := m.rs.Resolve(resolve.Inputs{Profile: "work", Login: "carol", HomeLogin: true, Dir: m.home})
	if e := apperr.As(err); e.Code != apperr.CodeConflict || e.Message != "--login and --home-login both choose the login; pick one" {
		t.Fatalf("%v", err)
	}
}

// TestLoginPrecedence walks the chain of 6.1 step 4.
func TestLoginPrecedence(t *testing.T) {
	m := newMachine(t, base)
	m.env["AGX_LOGIN"] = "derek"
	m.env["AGX_LOGIN_CLAUDE"] = "carol"
	r, err := m.rs.Resolve(resolve.Inputs{Profile: "personal", Dir: m.home})
	if err != nil || r.Login.Name != "carol" || r.Login.Source != "AGX_LOGIN_CLAUDE" {
		t.Fatalf("%v %+v", err, r.Login)
	}
	r, err = m.rs.Resolve(resolve.Inputs{Profile: "personal", Login: "derek", Dir: m.home})
	if err != nil || r.Login.Source != "--login" {
		t.Fatalf("%v %+v", err, r.Login)
	}
}

// TestEdge6HomeMissing: a profile whose home is gone.
func TestEdge6HomeMissing(t *testing.T) {
	m := newMachine(t, strings.Replace(base, "~/.claude-work", "~/.claude-gone", 1))
	_, err := m.rs.Resolve(resolve.Inputs{Profile: "work", Dir: m.home})
	if e := apperr.As(err); e.Code != apperr.CodeHomeMissing || e.Message != "profile work: home ~/.claude-gone does not exist" {
		t.Fatalf("%v", err)
	}
}

// TestEdge28SharedHomeNoDefault: two plan profiles on one home, no TTY.
func TestEdge28SharedHomeNoDefault(t *testing.T) {
	m := newMachine(t, `{"version":1,"profiles":{"a":{"agent":"claude","home":"~/.claude"},"b":{"agent":"claude","home":"~/.claude"},"k":{"agent":"claude","home":"~/.claude","billing":"api"}}}`)
	_, _, err := m.rs.ProfileForHome("claude", filepath.Join(m.home, ".claude"), nil)
	if e := apperr.As(err); e.Code != apperr.CodeAmbiguous || e.Message != "~/.claude has 3 profiles (a, b, k) and no default; add --profile" {
		t.Fatalf("%v", err)
	}
	m2 := newMachine(t, `{"version":1,"profiles":{"a":{"agent":"claude","home":"~/.claude"},"k":{"agent":"claude","home":"~/.claude","billing":"api"}}}`)
	n, _, err := m2.rs.ProfileForHome("claude", filepath.Join(m2.home, ".claude"), nil)
	if err != nil || n != "a" {
		t.Fatalf("plan profile beats api: %v %s", err, n)
	}
}

func TestLoginEnvFor(t *testing.T) {
	if got := resolve.LoginEnvFor("my-cli"); got != "AGX_LOGIN_MY_CLI" {
		t.Fatal(got)
	}
}

// TestBindingMacOSFolding: under the macOS rule (forced on any machine
// with FoldOS) a binding written in another case and reached through a
// symlink still decides, the folder is named in its stored spelling, and
// the Linux rule keeps the two folders apart.
func TestBindingMacOSFolding(t *testing.T) {
	const cfg = `{
  "version": 1,
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" }
  },
  "projects": { "HOME/PROJ/Client": { "claude": { "profile": "work" } } }
}`
	m := newMachine(t, cfg)
	link := filepath.Join(m.home, "link")
	if err := os.Symlink(filepath.Join(m.home, "proj/client"), link); err != nil {
		t.Skip(err)
	}
	m.rs.FoldOS = "darwin"
	r, err := m.rs.Resolve(resolve.Inputs{Agent: "claude", Dir: filepath.Join(link, "sub")})
	if err != nil || r.ProfileName != "work" {
		t.Fatalf("darwin rule: %v %s", err, r.ProfileName)
	}
	if _, s := value(r, "profile"); !strings.Contains(s, "PROJ/Client") {
		t.Errorf("source %q: want the stored spelling HOME/PROJ/Client", s)
	}
	m.rs.FoldOS = "linux"
	if r, err := m.rs.Resolve(resolve.Inputs{Agent: "claude", Dir: filepath.Join(link, "sub")}); err != nil || r.ProfileName != "personal" {
		t.Errorf("linux rule: %v %s (want personal: the case differs)", err, r.ProfileName)
	}
}
