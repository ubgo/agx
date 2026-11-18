package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// builtinAutonomy pins the autonomy levels each built-in agent's launch
// builder really accepts (agentio's mapping, never a spec map alone):
// Codex has no edits-only mode, and pi has no permission modes at all (it
// always runs without prompts, so default and full both mean that, and
// read-only is its read-only tool set).
var builtinAutonomy = map[agentio.ID][]agentio.Autonomy{
	claude.ID: {agentio.AutonomyDefault, agentio.AutonomyReadOnly, agentio.AutonomyAsk, agentio.AutonomyAcceptEdits, agentio.AutonomyFull},
	codex.ID:  {agentio.AutonomyDefault, agentio.AutonomyReadOnly, agentio.AutonomyAsk, agentio.AutonomyFull},
	gemini.ID: {agentio.AutonomyDefault, agentio.AutonomyReadOnly, agentio.AutonomyAsk, agentio.AutonomyAcceptEdits, agentio.AutonomyFull},
	pi.ID:     {agentio.AutonomyDefault, agentio.AutonomyReadOnly, agentio.AutonomyFull},
}

// levelNames is the wire spelling of levels, as agent show lists them.
func levelNames(ls []agentio.Autonomy) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.String()
	}
	return out
}

// builtinMachine is a machine with every built-in agent installed (a fake
// binary on PATH), its default home on disk and one profile per agent,
// named like the agent. It returns the App over it and the built-ins.
func builtinMachine(t *testing.T) (*App, []agentio.Agent) {
	t.Helper()
	m := newMachine(t, `{"version": 1}`)
	reg, err := m.app(t).Registry()
	if err != nil {
		t.Fatal(err)
	}
	var builtins []agentio.Agent
	var profiles []string
	for _, ag := range reg.All() {
		if reg.IsCustom(ag.ID()) {
			continue
		}
		builtins = append(builtins, ag)
		m.f.FakeAgent(ag.Spec().Binary)
		rel, err := filepath.Rel(m.f.Home, ag.DefaultHome().Dir())
		if err != nil {
			t.Fatal(err)
		}
		m.f.HomeDir(rel)
		profiles = append(profiles, fmt.Sprintf(`%q: {"agent": %q, "home": %q}`, ag.ID(), ag.ID(), "~/"+filepath.ToSlash(rel)))
	}
	m.f.WriteConfig(`{"version": 1, "profiles": {` + strings.Join(profiles, ", ") + `}}`)
	return m.app(t), builtins
}

// TestAgentShowAutonomyMatchesLaunch: agent show lists, for every
// built-in agent, exactly the levels its launch builder accepts, and
// agx run --dry-run accepts a level exactly when agent show lists it
// (full with --consent full-autonomy). Fake binaries on PATH, fixture
// homes, no network.
func TestAgentShowAutonomyMatchesLaunch(t *testing.T) {
	ctx := context.Background()
	a, builtins := builtinMachine(t)

	// The pinned rows and the built-ins are the same set: a new, renamed or
	// dropped agent shows on both sides of one message.
	var got, pinned []string
	for _, ag := range builtins {
		got = append(got, string(ag.ID()))
	}
	for id := range builtinAutonomy {
		pinned = append(pinned, string(id))
	}
	slices.Sort(got)
	slices.Sort(pinned)
	if !slices.Equal(got, pinned) {
		t.Fatalf("built-in agents %v, pinned rows %v", got, pinned)
	}

	for _, ag := range builtins {
		id := ag.ID()
		want := levelNames(builtinAutonomy[id])
		d, err := a.AgentShow(ctx, AgentShowOptions{ID: string(id)})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !slices.Equal(d.Autonomy, want) {
			t.Errorf("%s: agent show autonomy %v, want %v", id, d.Autonomy, want)
		}
		for _, l := range agentio.AutonomyValues() {
			o := LaunchOptions{Mode: ModeRun, DryRun: true, Autonomy: l.String()}
			o.Agent, o.Profile = id, string(id)
			if l == agentio.AutonomyFull {
				o.Consent = ConsentFullAutonomy
			}
			_, err := a.Prepare(ctx, o)
			if listed := slices.Contains(d.Autonomy, l.String()); listed != (err == nil) {
				t.Errorf("%s --autonomy %s: listed by agent show %v, run --dry-run error %v", id, l, listed, err)
			}
		}
	}
}
