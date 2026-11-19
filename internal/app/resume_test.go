package app

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/live"
)

// fakePicker answers from a script: Choose returns the next index (or
// err), Confirm returns confirm. asked records every title.
type fakePicker struct {
	choices []int
	confirm bool
	err     error
	asked   []string
}

func (p *fakePicker) Choose(_ context.Context, title string, items []picker.Item) (int, error) {
	p.asked = append(p.asked, title)
	if p.err != nil {
		return 0, p.err
	}
	i := p.choices[0]
	p.choices = p.choices[1:]
	return i, nil
}

func (p *fakePicker) Checklist(context.Context, string, []picker.Item) ([]int, error) {
	return nil, picker.Dismissed()
}

func (p *fakePicker) Confirm(_ context.Context, q string, _ bool) (bool, error) {
	p.asked = append(p.asked, q)
	return p.confirm, nil
}

func (p *fakePicker) Secret(context.Context, string) (string, error) { return "", picker.Dismissed() }

// resumeSharedConfig: ~/.claude-shared holds profiles a and b, no default for
// claude at all (so remembering is offered).
const resumeSharedConfig = `{
  "version": 1,
  // the shared home
  "profiles": {
    "a": { "agent": "claude", "home": "~/.claude-shared" },
    "b": { "agent": "claude", "home": "~/.claude-shared" }
  },
  "logins": { "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "fake-token-CAROL" } }
}`

// testApp builds an App over a fixture host whose user home holds
// ~/.claude-shared with one conversation started in <home>/proj.
func testApp(t *testing.T, cfg string, pk picker.Picker) (*App, string) {
	t.Helper()
	h := agenttest.NewHost(t)
	proj := filepath.Join(h.UserHome, "proj")
	conv := filepath.Join(h.UserHome, ".claude-shared", "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(proj, "-"))
	for _, d := range []string{proj, conv} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	tpl, err := os.ReadFile(filepath.Join("..", "cli", "testdata", "resume", "claude-transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer("{{ID}}", "5b5b5b5b-0000-4000-8000-000000000004", "{{CWD}}", proj, "{{TITLE}}", "shared chat").Replace(string(tpl))
	if err := os.WriteFile(filepath.Join(conv, "5b5b5b5b-0000-4000-8000-000000000004.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(h.UserHome, ".agx", "config.jsonc")
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	lookup := func(string) (string, bool) { return "", false }
	a := New(Deps{Host: h, Lookup: lookup, Cwd: proj, ConfigFlag: cfgFile, Picker: pk, Self: "/opt/my tools/agx"})
	return a, proj
}

// TestEdge27SharedHomeAsksAndRemembers: at a terminal the shared-home
// rule asks which profile and offers to remember it as default; the
// answer is written to config.jsonc keeping its comments.
func TestEdge27SharedHomeAsksAndRemembers(t *testing.T) {
	pk := &fakePicker{choices: []int{1}, confirm: true}
	a, proj := testApp(t, resumeSharedConfig, pk)
	o := LaunchOptions{Mode: ModeResume, ResumeID: "5b5b"}
	if err := a.resumeTarget(context.Background(), &o); err != nil {
		t.Fatal(err)
	}
	if o.Profile != "b" || o.Dir != proj || o.ProfileFrom != "home ~/.claude-shared (asked)" {
		t.Fatalf("options %+v", o)
	}
	b, err := os.ReadFile(a.Paths().File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "// the shared home") || !regexp.MustCompile(`"b": \{[^}]*"default": true`).Match(b) {
		t.Fatalf("config not remembered:\n%s", b)
	}
	if len(pk.asked) != 2 {
		t.Fatalf("asked %q", pk.asked)
	}
}

// TestEdge27DryRunAsksButNeverWrites: a dry run asks the question but
// does not offer to remember.
func TestEdge27DryRunAsksButNeverWrites(t *testing.T) {
	pk := &fakePicker{choices: []int{0}, confirm: true}
	a, _ := testApp(t, resumeSharedConfig, pk)
	o := LaunchOptions{Mode: ModeResume, ResumeID: "5b5b", DryRun: true}
	if err := a.resumeTarget(context.Background(), &o); err != nil || o.Profile != "a" {
		t.Fatalf("%v %+v", err, o)
	}
	if len(pk.asked) != 1 {
		t.Fatalf("asked %q", pk.asked)
	}
	b, err := os.ReadFile(a.Paths().File)
	if err != nil || strings.Contains(string(b), `"default"`) {
		t.Fatalf("dry run wrote config: %v\n%s", err, b)
	}
}

// TestResumePickerChoosesChat: no --id: the picker lists this folder's
// chats and the choice decides the conversation.
func TestResumePickerChoosesChat(t *testing.T) {
	pk := &fakePicker{choices: []int{0, 0}}
	a, _ := testApp(t, resumeSharedConfig, pk)
	o := LaunchOptions{Mode: ModeResume}
	if err := a.resumeTarget(context.Background(), &o); err != nil {
		t.Fatal(err)
	}
	if o.ResumeID != "5b5b5b5b-0000-4000-8000-000000000004" || pk.asked[0] != "Resume which conversation?" {
		t.Fatalf("%+v %q", o, pk.asked)
	}
	pk = &fakePicker{choices: []int{0, 0, 0}}
	a, _ = testApp(t, resumeSharedConfig, pk)
	o = LaunchOptions{Mode: ModeResume, Resume: ResumeOptions{Picker: ResumePickerFolder}}
	if err := a.resumeTarget(context.Background(), &o); err != nil || pk.asked[0] != "Resume a chat from which folder?" {
		t.Fatalf("%v %q", err, pk.asked)
	}
}

// TestEdge52And58PickerStops: Esc in the picker is "dismissed" (exit 1
// everywhere); Ctrl-C is "interrupted" (130). Nothing starts.
func TestEdge52And58PickerStops(t *testing.T) {
	for _, c := range []struct {
		err  error
		exit int
	}{
		{picker.Dismissed(), apperr.ExitFailure},
		{apperr.Wrap(context.Canceled, apperr.CodeInterrupted, apperr.ClassInterrupted, "interrupted"), apperr.ExitInterrupted},
	} {
		a, _ := testApp(t, resumeSharedConfig, &fakePicker{err: c.err})
		o := LaunchOptions{Mode: ModeResume}
		err := a.resumeTarget(context.Background(), &o)
		if got := apperr.Exit(err, true); got != c.exit {
			t.Errorf("%v: exit %d, want %d", c.err, got, c.exit)
		}
	}
	// A cancelled context before any listing stops too.
	a, _ := testApp(t, resumeSharedConfig, &fakePicker{choices: []int{0}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := LaunchOptions{Mode: ModeResume, ResumeID: "5b5b"}
	if got := apperr.Exit(a.resumeTarget(ctx, &o), true); got != apperr.ExitInterrupted {
		t.Errorf("cancelled: exit %d", got)
	}
}

// TestLivePlanAndStart: a live run gets agentio's plan, a quoted helper
// command, p.Live and a decision; Commit's writeLiveStart writes
// live.jsonc without any token.
func TestLivePlanAndStart(t *testing.T) {
	a, _ := testApp(t, resumeSharedConfig, nil)
	reg, err := a.Registry()
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := a.Config()
	rs := &resolve.Resolver{Cfg: cfg, Agents: reg, Lookup: a.d.Lookup, UserHome: a.d.Host.UserHome}
	res, err := rs.Resolve(resolve.Inputs{Profile: "a", Login: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	p := &LaunchPlan{RunID: "20261114-101500-k3x9", RunDir: t.TempDir(), Agent: res.Agent.ID()}
	plan, err := a.livePlan(context.Background(), LaunchOptions{LiveLogin: true}, res, p)
	if err != nil || plan == nil {
		t.Fatalf("%v %v", err, plan)
	}
	wantHelper := `'/opt/my tools/agx' credential --config ` + a.Paths().File
	if p.Live == nil || p.Live.StartLogin != "carol" || p.Live.Helper != wantHelper || !strings.Contains(string(plan.Settings), "credential") || len(plan.Scrub) == 0 {
		t.Fatalf("live %+v plan %+v", p.Live, plan)
	}
	if err := a.writeLiveStart(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	st, err := live.Read(p.RunDir)
	if err != nil || st.StartLogin != "carol" || st.Want() != "carol" || st.Helper != wantHelper {
		t.Fatalf("%v %+v", err, st)
	}
	none, err := a.livePlan(context.Background(), LaunchOptions{}, res, &LaunchPlan{})
	if none != nil || err != nil {
		t.Fatalf("not live: %v %v", none, err)
	}
}

// TestShellQuote: plain paths stay as they are; anything else is single
// quoted with embedded quotes escaped.
func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/agx": "/usr/local/bin/agx",
		"/opt/my tools/agx":  "'/opt/my tools/agx'",
		"/o'k/agx":           `'/o'\''k/agx'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
