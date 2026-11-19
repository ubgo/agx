package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/login"
)

// secretPicker answers a pasted secret; every other question fails the
// test, so a test proves exactly which questions were asked.
type secretPicker struct {
	t      *testing.T
	secret string
	asked  int
}

func (p *secretPicker) Choose(context.Context, string, []picker.Item) (int, error) {
	p.t.Fatal("unexpected Choose")
	return 0, nil
}

func (p *secretPicker) Checklist(context.Context, string, []picker.Item) ([]int, error) {
	p.t.Fatal("unexpected Checklist")
	return nil, nil
}

func (p *secretPicker) Confirm(context.Context, string, bool) (bool, error) {
	p.t.Fatal("unexpected Confirm")
	return false, nil
}

func (p *secretPicker) Secret(context.Context, string) (string, error) {
	p.asked++
	return p.secret, nil
}

// fixtureApp builds an App on a fixture host with config src (written to
// <home>/.agx/config.jsonc), the given processes and picker.
func fixtureApp(t *testing.T, src string, procs *agenttest.FakeProcesses, pk picker.Picker) (*App, string) {
	t.Helper()
	opts := []agenttest.HostOption{}
	if procs != nil {
		opts = append(opts, agenttest.WithProcesses(procs))
	}
	h := agenttest.NewHost(t, opts...)
	if src != "" {
		agenttest.WriteFile(t, h.UserHome, ".agx/config.jsonc", src)
	}
	lookup := func(name string) (string, bool) {
		if name == "AGX_CONFIG_DIR" {
			return filepath.Join(h.UserHome, ".agx"), true
		}
		return "", false
	}
	a := New(Deps{Host: h, Lookup: lookup, Cwd: h.UserHome, Picker: pk})
	return a, h.UserHome
}

// TestDiscoverProcessSource is source 4 of spec 6.2: a running agent's
// start environment names a home; an unreadable process is counted.
func TestDiscoverProcessSource(t *testing.T) {
	procs := &agenttest.FakeProcesses{
		List: []agentio.Process{
			{PID: 42, Exe: "/opt/bin/claude", Argv: []string{"claude"}},
			{PID: 43, Exe: "/opt/bin/claude", Argv: []string{"claude"}},
			{PID: 44, Exe: "/usr/bin/vim", Argv: []string{"vim"}},
			{PID: 45, Exe: "/opt/bin/claude", Argv: []string{"claude"}},
		},
	}
	a, home := fixtureApp(t, "", procs, nil)
	dir := filepath.Join(home, "accounts", "proc-home")
	agenttest.WriteFile(t, dir, "settings.json", "{}")
	// pid 45 runs under another user home (the real machine around a test
	// fixture, a sandbox): its home is not this user's and is not offered.
	other := t.TempDir()
	foreign := filepath.Join(other, "foreign-home")
	agenttest.WriteFile(t, foreign, "settings.json", "{}")
	procs.Envs = map[int][]string{42: {"HOME=" + home, "CLAUDE_CONFIG_DIR=" + dir}, 45: {"HOME=" + other, "CLAUDE_CONFIG_DIR=" + foreign}}

	rows, warns, err := a.Discover(context.Background(), DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var found *DiscoveredHome
	for i := range rows {
		if rows[i].dir == dir {
			found = &rows[i]
		}
	}
	if found == nil || found.Agent != "claude" || found.FoundBy[0] != "process 42" || !found.LooksLikeHome {
		t.Fatalf("process home: %+v (rows %+v)", found, rows)
	}
	for _, r := range rows {
		if r.dir == foreign {
			t.Errorf("a process of another user home was offered: %+v", r)
		}
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "1 agent processes could not be read") {
		t.Errorf("warnings %+v", warns)
	}
}

// TestStoreSetupTokenWritesLoginAndProfile: the pasted token goes to
// tokens.jsonc (never to config.jsonc), and the profile gets the login.
func TestStoreSetupTokenWritesLoginAndProfile(t *testing.T) {
	pk := &secretPicker{t: t, secret: "  fake-token-SETUP\n"}
	a, home := fixtureApp(t, `{"version": 1, "profiles": {"work": {"agent": "claude", "home": "~/.claude-work"}}}`, nil, pk)
	if err := os.MkdirAll(filepath.Join(home, ".claude-work"), 0o700); err != nil {
		t.Fatal(err)
	}
	name, err := a.StoreSetupToken(context.Background(), "work")
	if err != nil || name != "work" || pk.asked != 1 {
		t.Fatalf("store: %q %v asked %d", name, err, pk.asked)
	}
	v, ok, err := login.ReadStored(a.Paths(), "work")
	if err != nil || !ok || v != "fake-token-SETUP" {
		t.Fatalf("stored %q %v %v", v, ok, err)
	}
	cfg, err := os.ReadFile(a.Paths().File)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "fake-token-SETUP") || !strings.Contains(string(cfg), `"stored":true`) || !strings.Contains(string(cfg), `"login": "work"`) {
		t.Errorf("config:\n%s", cfg)
	}

	pk.secret = "  "
	a.Reload()
	_, err = a.StoreSetupToken(context.Background(), "work")
	if e := apperr.As(err); e.Code != apperr.CodeSecret || !strings.Contains(e.Message, "token is empty") {
		t.Errorf("empty token: %v", err)
	}
}

// TestUniqueName covers profile import's naming.
func TestUniqueName(t *testing.T) {
	a, home := fixtureApp(t, "", nil, nil)
	reg, err := a.Registry()
	if err != nil {
		t.Fatal(err)
	}
	codex, _ := reg.Get("codex")
	claude, _ := reg.Get("claude")
	taken := map[string]bool{"personal": true}
	if got := uniqueName(suggestName(codex, filepath.Join(home, ".codex")), codex, filepath.Join(home, ".codex"), taken); got != "codex" {
		t.Errorf("codex default home: %s", got)
	}
	work := filepath.Join(home, ".claude-work")
	if got := suggestName(claude, work); got != "work" {
		t.Errorf("suggest work: %s", got)
	}
	taken["work"], taken["work-claude"] = true, true
	if got := uniqueName("work", claude, work, taken); got != "work-claude-2" {
		t.Errorf("taken work: %s", got)
	}
	if got := suggestName(claude, filepath.Join(home, "acct", ".claude")); got != "acct" {
		t.Errorf("parent-named home: %s", got)
	}
}

// TestProfileListPlanFailureIsAWarning: --plan reads each home's plan
// (cost N); a home that cannot answer is a warning on its row, never a
// failed listing, and an agent without a plan reader says so.
func TestProfileListPlanFailureIsAWarning(t *testing.T) {
	a, home := fixtureApp(t, `{"version": 1, "profiles": {
		"work": {"agent": "claude", "home": "~/.claude-work"},
		"pi": {"agent": "pi", "home": "~/.pi/agent"}
	}}`, nil, nil)
	for _, d := range []string{".claude-work", ".pi/agent"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rows, warns, err := a.ProfileList(context.Background(), ProfileListOptions{Plan: true})
	if err != nil {
		t.Fatal(err)
	}
	plans := map[string]string{}
	for _, r := range rows {
		plans[r.Name] = r.Plan
	}
	if plans["work"] != "error" || plans["pi"] != notModelledText {
		t.Errorf("plans %v", plans)
	}
	if len(warns) != 1 || warns[0].Code != WarnPlan || !strings.HasPrefix(warns[0].Message, "profile work: plan: ") {
		t.Errorf("warnings %+v", warns)
	}
}
