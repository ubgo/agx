package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// fablePool is poolMachine with the classic keys dropped (default rules)
// and the members' answers: alice's Fable week is fable percent.
func fablePool(t *testing.T, fable float64, rejected bool, extra string) (*machine, *fakeVendor) {
	t.Helper()
	m, fv := poolMachine(t, map[string]account{
		tokPersonal: {session: 5, weekly: 20, fable: &fable, fableRejected: rejected},
		tokWork:     {session: 5, weekly: 30},
		tokCarol:    {session: 1, weekly: 1},
		tokDave:     {session: 60, weekly: 10},
	})
	m.f.WriteConfig(strings.Replace(poolConfig, poolClassicKeys, extra, 1))
	return m, fv
}

// candidateNamed finds one member of a report.
func candidateNamed(t *testing.T, rep PoolReport, name string) PoolCandidate {
	t.Helper()
	for _, c := range rep.Candidates {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no candidate %s", name)
	return PoolCandidate{}
}

// TestPoolProbeModelChoice: the probe model comes from --model (or the
// run), else probe_models, else the default profile's model, else the
// agent's default probe model; each is what the probe asks with.
func TestPoolProbeModelChoice(t *testing.T) {
	cases := []struct {
		name, extra, model string
		profileModel       string
		wantModels         []string
		wantSrc            string
		wantSent           string
	}{
		{"default probe model", "", "", "", []string{}, modelSrcNone, "claude-haiku-5-5"},
		{"--model alias", "", "fable", "", []string{"fable"}, modelSrcFlag, "claude-fable-5-2"},
		{"probe_models", `"probe_models": ["claude-fable-5-2"], `, "", "", []string{"claude-fable-5-2"}, modelSrcPool, "claude-fable-5-2"},
		{"--model beats probe_models", `"probe_models": ["claude-fable-5-2"], `, "opus", "", []string{"opus"}, modelSrcFlag, "claude-opus-5-5"},
		{"default profile's model", "", "", "fable", []string{"fable"}, modelSrcProfile + "personal", "claude-fable-5-2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, fv := fablePool(t, 40, false, c.extra)
			if c.profileModel != "" {
				cfgText := strings.Replace(poolConfig, poolClassicKeys, c.extra, 1)
				cfgText = strings.Replace(cfgText, `"default": true }`, `"default": true, "model": "`+c.profileModel+`" }`, 1)
				m.f.WriteConfig(cfgText)
			}
			rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: c.model}, TimeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(rep.ProbeModels, c.wantModels) || rep.ProbeModelsSource != c.wantSrc {
				t.Errorf("models %v from %q, want %v from %q", rep.ProbeModels, rep.ProbeModelsSource, c.wantModels, c.wantSrc)
			}
			sent := fv.probeModels()
			if len(sent) != 4 || sent[0] != c.wantSent {
				t.Errorf("probes asked with %v, want 4 × %s", sent, c.wantSent)
			}
		})
	}
}

// TestPoolFableWindow: a Fable session sees alice's Fable week as an
// extra weekly cap: at 99% (over her 98) she is blocked for Fable and
// the pick moves on, while an Opus session still picks her; the cache
// keeps each model's windows, so a second pick within max age probes
// nothing.
func TestPoolFableWindow(t *testing.T) {
	m, fv := fablePool(t, 99, false, "")
	a := m.app(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: "fable"}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	alice := candidateNamed(t, rep, "alice")
	if rep.Selected.Name == "alice" || !slices.Contains(alice.Reasons, "fable week is 99% (cap 98%)") || len(alice.Scoped) != 1 || alice.Scoped[0].Name != "fable week" {
		t.Fatalf("fable pick %s; alice %+v", rep.Selected.Name, alice)
	}
	rep, err = a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: "opus"}, TimeOptions{})
	if err != nil || rep.Selected.Name != "alice" || len(candidateNamed(t, rep, "alice").Scoped) != 0 {
		t.Fatalf("opus pick %+v %v", rep.Selected, err)
	}
	probes := fv.count("/v1/messages")
	for _, model := range []string{"fable", "opus"} {
		if _, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: model}, TimeOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if fv.count("/v1/messages") != probes {
		t.Errorf("a fresh cache for both models still probed: %d → %d", probes, fv.count("/v1/messages"))
	}
	logb, _ := os.ReadFile(filepath.Join(m.f.Home, "pool.log"))
	if !strings.Contains(string(logb), `picked="bob" group="own" forced=false by="rules" source="live agx result" usage="5h=5 week=30" models="fable"`) {
		t.Errorf("pool log:\n%s", logb)
	}
	cache, _ := login.ReadUsageCache(a.paths)
	e := cache.Logins["alice"]
	if len(e.Models["fable"].Windows) != 1 || e.Models["fable"].Windows[0].ID != "7d_oi" || len(e.Models["opus"].Windows) != 0 {
		t.Errorf("cache models %+v", e.Models)
	}
}

// TestPoolFableRejected: a rejected Fable window blocks the member for
// Fable until its reset even under its cap, and a full Fable window does
// not mark the member limit-reached for other models.
func TestPoolFableRejected(t *testing.T) {
	m, _ := fablePool(t, 50, true, "")
	rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: "fable"}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if alice := candidateNamed(t, rep, "alice"); alice.State != pool.StateBlocked || !strings.Contains(strings.Join(alice.Reasons, ";"), "fable week is refused by the vendor") {
		t.Errorf("alice %+v", alice)
	}
	m2, _ := fablePool(t, 100, false, "")
	rep, err = m2.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto", Model: "opus"}, TimeOptions{})
	if err != nil || rep.Selected.Name != "alice" || candidateNamed(t, rep, "alice").Status != string(StatusOK) {
		t.Errorf("opus after a full fable probe: %+v %v", rep.Selected, err)
	}
}

// TestPoolRunModel: a run records its model at launch (--model, else the
// profile's) and its key requests probe with it; the dry run names it.
func TestPoolRunModel(t *testing.T) {
	m, fv := fablePool(t, 10, false, "")
	m.f.FakeAgent("claude")
	a := m.app(t)
	p, err := dryRun(t, a, func(o *LaunchOptions) { o.Login, o.Model = "auto", "fable" })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decisionValues(p), "pool probe model fable (its per-model windows apply) [--model]") || p.Live.Model != "fable" {
		t.Errorf("dry run:\n%s", decisionValues(p))
	}
	id := liveRun(t, m, a, "auto")
	if _, err := live.Update(context.Background(), run.Dir(a.paths.Runs, id), func(s *live.State) error { s.Model = "fable"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.app(t).Credential(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sent := fv.probeModels(); len(sent) == 0 || sent[0] != "claude-fable-5-2" {
		t.Errorf("run probes asked with %v", sent)
	}
}

// TestPoolProbeModelRefused: a model the vendor refuses falls back to
// the default probe with a warning, never blocking the pool.
func TestPoolProbeModelRefused(t *testing.T) {
	m, fv := fablePool(t, 10, false, `"probe_models": ["claude-nope-1"], `)
	fv.refuse = "claude-nope-1"
	rep, err := m.app(t).PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil || rep.Selected == nil {
		t.Fatalf("%+v %v", rep, err)
	}
	if !strings.Contains(strings.Join(rep.Warnings, ";"), "the vendor refused probe model claude-nope-1") {
		t.Errorf("warnings %v", rep.Warnings)
	}
}

// TestPoolProbeModelsKey: probe_models loads, an empty name is refused,
// the flag writes and removes it, and agx config set (scalars only)
// sends it to login edit.
func TestPoolProbeModelsKey(t *testing.T) {
	l := config.Pool{Own: []string{"alice"}, ProbeModels: []string{" "}}
	if _, err := l.Rules(); err == nil || !strings.Contains(err.Error(), "probe_models") {
		t.Errorf("empty model: %v", err)
	}
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: PoolRuleSettings{ProbeModels: []string{"claude-fable-5-2", "opus"}}}})
	if err != nil || !slices.Equal(ch.Changes, []Change{{"pool.probe_models", "", "claude-fable-5-2 opus"}}) {
		t.Fatalf("%+v %v", ch.Changes, err)
	}
	if !strings.Contains(compact(readConfig(t, home)), `"probe_models":["claude-fable-5-2","opus"]`) {
		t.Errorf("file:\n%s", readConfig(t, home))
	}
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: PoolRuleSettings{ProbeModels: []string{""}}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readConfig(t, home), "probe_models") {
		t.Error("probe_models left after removing it")
	}
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: PoolRuleSettings{ProbeModels: []string{"two words"}}}}); err == nil {
		t.Error("a model with a space was written")
	}
	// A list is not a scalar: agx config set sends it to login edit.
	a2, _ := fixtureApp(t, flagConfig, nil, nil)
	if _, err := a2.ConfigSet(context.Background(), "logins.auto.pool.probe_models", `["claude-fable-5-2"]`); err == nil || !strings.Contains(err.Error(), "is a list") {
		t.Errorf("config set probe_models: %v", err)
	}
}

// TestPickedUsage: the log's usage names every window of the picked
// member, per-model ones included; none without a pick.
func TestPickedUsage(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := PoolReport{Selected: &PoolSelection{Name: "alice"}, Candidates: []PoolCandidate{
		{Name: "bob", Week: f(1)},
		{Name: "alice", Short: f(12.4), Week: f(40), Scoped: []pool.ScopedState{{Name: "fable week", Percent: f(61.5)}}},
	}}
	if got := pickedUsage(rep); got != "5h=12 week=40 fable_week=62" {
		t.Errorf("usage %q", got)
	}
	rep.Selected = nil
	if got := pickedUsage(rep); got != "" {
		t.Errorf("no pick: %q", got)
	}
}
