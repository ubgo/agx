package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
)

// TestWhenNoneUsableSources: the pool's own setting wins, then the live
// block, then stop by default; agx pool show names the same source.
func TestWhenNoneUsableSources(t *testing.T) {
	pl := func(mode string) config.Login {
		return config.Login{Kind: config.KindPool, Pool: &config.Pool{WhenNoneUsable: mode}}
	}
	cases := []struct {
		name               string
		c                  config.Config
		login              string
		mode, src, showSrc string
	}{
		{"default", config.Config{Logins: map[string]config.Login{"auto": pl("")}}, "auto", "stop", SourceDefault, SourceDefault},
		{"live block", config.Config{Live: config.LiveConfig{WhenNoneUsable: "keep"}, Logins: map[string]config.Login{"auto": pl("")}}, "auto", "keep", liveKeyWhenNoneUsable, SourceLiveBlock},
		{"pool wins", config.Config{Live: config.LiveConfig{WhenNoneUsable: "keep"}, Logins: map[string]config.Login{"auto": pl("stop")}}, "auto", "stop", "config logins.auto.pool.when_none_usable", SourcePool},
		{"token login: live block", config.Config{Live: config.LiveConfig{WhenNoneUsable: "keep"}, Logins: map[string]config.Login{"carol": {Kind: config.KindOAuthToken}}}, "carol", "keep", liveKeyWhenNoneUsable, SourceLiveBlock},
		{"unknown login: default", config.Config{}, "nosuch", "stop", SourceDefault, SourceDefault},
	}
	for _, c := range cases {
		mode, src := whenNoneUsable(c.c, c.login)
		if mode != c.mode || src != c.src {
			t.Errorf("%s: %s from %s, want %s from %s", c.name, mode, src, c.mode, c.src)
		}
		if s := whenNoneUsableSetting(c.c, c.login); s.Value != c.mode || s.Source != c.showSrc || s.Key != "when_none_usable" {
			t.Errorf("%s: pool show %+v", c.name, s)
		}
	}
	if !strings.HasPrefix(whenNoneUsableText("stop"), "stop") || !strings.HasPrefix(whenNoneUsableText("keep"), "keep") {
		t.Error("texts")
	}
}

// TestWhenNoneUsableRejectsTypos: the config refuses a mode it does not
// know, at both levels.
func TestWhenNoneUsableRejectsTypos(t *testing.T) {
	for _, cfg := range []string{
		`{"version":1,"live":{"when_none_usable":"pause"}}`,
		`{"version":1,"logins":{"auto":{"agents":["claude"],"kind":"pool","pool":{"own":["a"],"when_none_usable":"wait"}},"a":{"agents":["claude"],"kind":"oauth-token","token":"${A}"}}}`,
	} {
		m := newMachine(t, cfg)
		if _, err := m.app(t).Config(); err == nil {
			t.Errorf("accepted %s", cfg)
		}
	}
}

// TestRefusalBriefs: the few words a refusal is shown with.
func TestRefusalBriefs(t *testing.T) {
	capped := PoolReport{Candidates: []PoolCandidate{{Name: "a", Reasons: []string{"5h is 95% (cap 90%)"}}, {Name: "b", Reasons: []string{"status is rejected", "week is 99% (cap 70%)"}}}}
	mixed := PoolReport{Candidates: []PoolCandidate{{Name: "a", Reasons: []string{"5h is 95% (cap 90%)"}}, {Name: "b", Reasons: []string{"no usage reading"}}}}
	none := apperr.Failure(apperr.CodeAutoPick, "pool auto: no member is usable")
	cases := []struct {
		err  error
		rep  PoolReport
		want string
	}{
		{none, capped, briefAllOverCaps},
		{none, mixed, briefNoneUsable},
		{none, PoolReport{}, briefNoneUsable},
		{apperr.Failure(apperr.CodePerUseKey, "x"), capped, briefAPIKey},
		{apperr.Failure(apperr.CodeSecret, "x"), PoolReport{}, briefNotResolved},
		{apperr.Failure(apperr.CodeIO, "x"), PoolReport{}, briefRequestFailed},
	}
	for _, c := range cases {
		if got := poolBrief(c.err, c.rep); got != c.want {
			t.Errorf("%v: %q, want %q", c.err, got, c.want)
		}
	}
	if onlyWhat([]string{"week is 70% (cap 70%)"}) != briefOverCap || onlyWhat([]string{"disabled"}) != briefNotUsable {
		t.Error("pin words")
	}
}

// tokenLiveConfig is a live run on a token login whose token comes
// from a variable, so a test can take it away.
const tokenLiveConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude", "default": true } },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" } }%s
}`

// TestCredentialTokenLoginStopAndRecover: a live run on a token login
// whose token is gone gets the placeholder (stop), live.jsonc says why,
// and the next request after the token is back hands it out again.
func TestCredentialTokenLoginStopAndRecover(t *testing.T) {
	m := newMachine(t, strings.Replace(tokenLiveConfig, "%s", "", 1))
	a := m.app(t)
	id := liveRun(t, m, a, "carol")
	dir := run.Dir(a.paths.Runs, id)
	m.env["CAROL_TOKEN"] = "fake-token-CAROL"
	if cr, err := m.app(t).Credential(context.Background()); err != nil || cr.Token.Reveal() != "fake-token-CAROL" || cr.Refused != "" {
		t.Fatalf("first: %+v %v", cr, err)
	}
	delete(m.env, "CAROL_TOKEN")
	cr, err := m.app(t).Credential(context.Background())
	if err != nil || cr.Token.Reveal() != "" || !strings.Contains(cr.Refused, "CAROL_TOKEN is not set") || cr.RefusedBrief != briefNotResolved {
		t.Fatalf("refused: %+v %v", cr, err)
	}
	st, _ := live.Read(dir)
	if st.InUse != "" || st.NoKeyBrief != briefNotResolved || st.KeyAt.IsZero() || st.NoKeySince.IsZero() || strings.Contains(st.NoKey, "(try:") {
		t.Errorf("live.jsonc %+v", st)
	}
	m.env["CAROL_TOKEN"] = "fake-token-CAROL"
	if cr, err := m.app(t).Credential(context.Background()); err != nil || cr.Token.Reveal() != "fake-token-CAROL" {
		t.Fatalf("recovered: %+v %v", cr, err)
	}
	if st, _ := live.Read(dir); st.InUse != "carol" || st.NoKey != "" || !st.NoKeySince.IsZero() {
		t.Errorf("after recovery %+v", st)
	}
}

// TestCredentialBeforeTheRunIsAnError: with no run to record the
// refusal in, agx credential fails whatever the setting.
func TestCredentialBeforeTheRunIsAnError(t *testing.T) {
	m := newMachine(t, strings.Replace(tokenLiveConfig, "%s", "", 1))
	m.env[run.EnvRunID] = "20261114-101500-none"
	if cr, err := m.app(t).Credential(context.Background()); err == nil || cr.Refused != "" {
		t.Fatalf("%+v %v", cr, err)
	}
}

// TestCredentialBrokenConfigStops: a config that no longer loads leaves
// the setting unknown, so the default (stop) answers.
func TestCredentialBrokenConfigStops(t *testing.T) {
	m := newMachine(t, strings.Replace(tokenLiveConfig, "%s", "", 1))
	a := m.app(t)
	id := liveRun(t, m, a, "carol")
	m.f.WriteConfig(`{ "version": 1, "logins": `)
	cr, err := m.app(t).Credential(context.Background())
	if err != nil || cr.Refused == "" {
		t.Fatalf("%+v %v", cr, err)
	}
	if st, _ := live.Read(run.Dir(a.paths.Runs, id)); st.WhenNoneUsable != "stop" || st.NoKeyBrief != briefRequestFailed {
		t.Errorf("%+v", st)
	}
}

// TestLaunchLabelAndRecords: --label names the run, else AGX_RUN_LABEL
// does; the input is removed from the agent's env; live.record_env
// values are kept; the process table gives the terminal and the parent.
func TestLaunchLabelAndRecords(t *testing.T) {
	m := newMachine(t, strings.Replace(tokenLiveConfig, "%s", `, "live": { "record_env": ["TERM_SESSION", "UNSET_ONE"] }`, 1))
	m.f.FakeAgent("claude")
	m.f.HomeDir(".claude")
	m.env["AGX_RUN_LABEL"] = "build window"
	m.env["TERM_SESSION"] = "pane-7"
	a := m.app(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login = "carol"; o.LiveLogin = true })
	if err != nil {
		t.Fatal(err)
	}
	if p.Record.Label != "build window" || !strings.Contains(decisionValues(p), "label build window [env AGX_RUN_LABEL]") {
		t.Errorf("env label %q\n%s", p.Record.Label, decisionValues(p))
	}
	if len(p.Record.Env) != 1 || p.Record.Env["TERM_SESSION"] != "pane-7" {
		t.Errorf("record env %v", p.Record.Env)
	}
	unset := false
	for _, e := range a.runEnv(p) {
		if e.Name() == run.EnvRunLabel && e.Op() == agentio.EnvUnsetOp {
			unset = true
		}
	}
	if !unset {
		t.Error("AGX_RUN_LABEL stays in the agent's env")
	}
	p, err = dryRun(t, a, func(o *LaunchOptions) { o.Login = "carol"; o.Label = " api\nfix " })
	if err != nil {
		t.Fatal(err)
	}
	if p.Record.Label != "api fix" || !strings.Contains(decisionValues(p), "label api fix [--label]") {
		t.Errorf("flag label %q\n%s", p.Record.Label, decisionValues(p))
	}
	delete(m.env, "AGX_RUN_LABEL")
	p, _ = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "carol" })
	if p.Record.Label != "" || strings.Contains(decisionValues(p), "label ") {
		t.Errorf("no label %q", p.Record.Label)
	}
	for _, e := range m.app(t).runEnv(p) {
		if e.Name() == run.EnvRunLabel {
			t.Error("unset of a variable that is not set")
		}
	}
}

// TestOwnProcessFacts: the start time, terminal and parent's name come
// from the process table; without one, the clock and nothing else.
func TestOwnProcessFacts(t *testing.T) {
	self := os.Getpid()
	start := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{
		{PID: self, PPID: 77, Start: start, TTY: "pts/3"},
		{PID: 77, Exe: "/usr/bin/zsh", Argv: []string{"-zsh"}},
	}}
	a := New(Deps{Host: agenttest.NewHost(t, agenttest.WithProcesses(procs)), Lookup: func(string) (string, bool) { return "", false }})
	got, tty, parent := a.ownProcess(context.Background())
	if !got.Equal(start) || tty != "pts/3" || parent != "zsh" {
		t.Errorf("%v %q %q", got, tty, parent)
	}
	if processName(agentio.Process{Argv: []string{"-bash"}}) != "bash" {
		t.Error("login shell name")
	}
	a = New(Deps{Host: agenttest.NewHost(t), Lookup: func(string) (string, bool) { return "", false }})
	if _, tty, parent := a.ownProcess(context.Background()); tty != "" || parent != "" {
		t.Errorf("no table: %q %q", tty, parent)
	}
}

// TestStatusLineText: the line for a pool, a refusal, a token login,
// a run that has not asked yet, and outside a run.
func TestStatusLineText(t *testing.T) {
	m, _, a := freshPool(t)
	if s := a.StatusLine(); s.Line != "" || s.RunID != "" {
		t.Errorf("outside a run: %+v", s)
	}
	id := liveRun(t, m, a, "auto")
	dir := run.Dir(a.paths.Runs, id)
	if s := m.app(t).StatusLine(); s.Line != "pool auto: waiting for the first key" {
		t.Errorf("not asked: %q", s.Line)
	}
	now := m.app(t).Now()
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error { s.SetInUse("alice", now); return nil }); err != nil {
		t.Fatal(err)
	}
	if s := m.app(t).StatusLine(); s.Line != "pool auto: alice · 5h 1% · week 30%" || !s.Pool || s.InUse != "alice" {
		t.Errorf("in use: %+v", s)
	}
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error {
		s.SetNoKey("pool auto: no member is usable: …", briefAllOverCaps, config.WhenNoneUsableStop, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := "pool auto: none (all over caps) since " + now.In(time.Local).Format("15:04")
	if s := m.app(t).StatusLine(); s.Line != want {
		t.Errorf("refused: %q, want %q", s.Line, want)
	}
	if _, err := live.Update(context.Background(), dir, func(s *live.State) error { s.ChosenLogin = "bob"; s.SetInUse("bob", now); return nil }); err != nil {
		t.Fatal(err)
	}
	if s := m.app(t).StatusLine(); s.Line != "login bob · 5h 1% · week 20%" || s.Pool {
		t.Errorf("token login: %+v", s)
	}
	m.env[run.EnvRunID] = "not-a-run"
	if s := m.app(t).StatusLine(); s.Line != "" {
		t.Errorf("bad id: %q", s.Line)
	}
}

// TestLaunchStatusLine: a live run gets agx statusline as its status
// line; a status line in the run's settings or the user's settings file
// is kept; live.status_line false turns it off.
func TestLaunchStatusLine(t *testing.T) {
	m, _, a := freshPool(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login = "auto" })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Settings), `"statusLine":{"type":"command","command":"agx statusline --config `) || !strings.Contains(decisionValues(p), "statusLine agx statusline (the account in use) [default]") {
		t.Errorf("added: %s\n%s", p.Settings, decisionValues(p))
	}
	p, err = dryRun(t, a, func(o *LaunchOptions) { o.Login = "auto"; o.Settings = []string{"statusLine.command=my-line"} })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Settings), "my-line") || strings.Contains(string(p.Settings), " statusline") || !strings.Contains(decisionValues(p), "statusLine from the run's settings kept") {
		t.Errorf("run settings: %s\n%s", p.Settings, decisionValues(p))
	}
	user := filepath.Join(m.f.Home, ".claude-personal", "settings.json")
	m.f.WriteFile(user, `{ "statusLine": { "type": "command", "command": "~/bin/line" } }`, 0o644)
	p, err = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "auto" })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(p.Settings), "statusLine") || !strings.Contains(decisionValues(p), "statusLine of ~/.claude-personal/settings.json kept") {
		t.Errorf("user settings: %s\n%s", p.Settings, decisionValues(p))
	}
	if err := os.Remove(user); err != nil {
		t.Fatal(err)
	}
	cfg := m.f.ReadFile(filepath.Join(m.f.ConfigDir, "config.jsonc"))
	m.f.WriteConfig(strings.Replace(cfg, `"version": 1,`, `"version": 1, "live": { "status_line": false },`, 1))
	p, err = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "auto" })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(p.Settings), "statusLine") || strings.Contains(decisionValues(p), "statusLine") {
		t.Errorf("off: %s\n%s", p.Settings, decisionValues(p))
	}
	// Not a live run: no status line.
	p, err = dryRun(t, m.app(t), func(o *LaunchOptions) { o.Login = "carol" })
	if err != nil || strings.Contains(string(p.Settings), "statusLine") {
		t.Errorf("token run: %s %v", p.Settings, err)
	}
}

// TestLiveListConversation: live list names the conversation running in
// a run's process (the agent's live sessions, by pid) and its title.
func TestLiveListConversation(t *testing.T) {
	a, home, proj := liveApp(t)
	id := "20261110-090000-conv"
	dir, err := run.Create(a.paths.Runs, id)
	if err != nil {
		t.Fatal(err)
	}
	rec := run.Record{ID: id, PID: livePID, StartedAt: time.UnixMilli(liveStart), Agent: "claude", Profile: "personal", Home: filepath.Join(home, ".claude"), Folder: proj, Label: "api", TTY: "pts/3", Parent: "zsh"}
	if err := run.WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	if err := live.Create(dir, live.State{RunID: id, Agent: "claude", StartLogin: "carol", Helper: "agx credential"}); err != nil {
		t.Fatal(err)
	}
	runs, err := a.LiveList(context.Background(), "")
	if err != nil || len(runs) != 1 {
		t.Fatalf("%+v %v", runs, err)
	}
	r := runs[0]
	if r.ConversationID != liveConv || r.Label != "api" || r.PID != livePID || r.TTY != "pts/3" || r.Parent != "zsh" || r.FolderShort != "~/proj" {
		t.Errorf("%+v", r)
	}
	if got := shortPath("~/projects/acme/services/api/internal", 20); got != "…rvices/api/internal" {
		t.Errorf("short %q", got)
	}
	if got := shortPath("~/proj", 20); got != "~/proj" {
		t.Errorf("short kept %q", got)
	}
}
