package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
)

// mustAgent is agent id of a's registry.
func mustAgent(t *testing.T, a *App, id string) agentio.Agent {
	t.Helper()
	reg, err := a.Registry()
	if err != nil {
		t.Fatal(err)
	}
	ag, ok := reg.Get(agentio.ID(id))
	if !ok {
		t.Fatalf("no agent %s", id)
	}
	return ag
}

// TestAutoProfilePicksMostHeadroom: the lowest highest-window wins; the
// shared home is read once; the reason is pick's explanation.
func TestAutoProfilePicksMostHeadroom(t *testing.T) {
	m, fv := sharedMachine(t, map[string]account{tokPersonal: {session: 77, weekly: 3}, tokWork: {session: 5, weekly: 2}})
	a := m.app(t)
	ag := mustAgent(t, a, "claude")
	name, reason, err := a.autoProfile(context.Background(), ag, []string{"kimi", "personal", "work"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "work" || !strings.Contains(reason, "auto → work (max 5%) over personal (max 77%)") {
		t.Errorf("name %s reason %q", name, reason)
	}
	if fv.count("/api/oauth/usage") != 2 {
		t.Errorf("requests %d, want one per home", fv.count("/api/oauth/usage"))
	}
}

// TestAutoProfileRateLimitedFallsBackToCache: a 429 home uses its cached
// reading, and the explanation shows its age.
func TestAutoProfileRateLimitedFallsBackToCache(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokPersonal: {session: 50}, tokWork: {status: http.StatusTooManyRequests}})
	m.claudeHome(".claude-work", tokWork, later, 10, 4*time.Minute)
	a := m.app(t)
	name, reason, err := a.autoProfile(context.Background(), mustAgent(t, a, "claude"), []string{"personal", "work"})
	if err != nil || name != "work" || !strings.Contains(reason, "cached 4m ago") {
		t.Fatalf("name %s reason %q err %v", name, reason, err)
	}
}

// TestEdge13AutoProfileNoneEligible: every candidate at its cap or
// failed is refused listing each reason (exit 125 when launching).
func TestEdge13AutoProfileNoneEligible(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokPersonal: {session: 100, weekly: 20}})
	m.claudeHome(".claude-work", tokWork, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 0, 0)
	a := m.app(t)
	_, _, err := a.autoProfile(context.Background(), mustAgent(t, a, "claude"), []string{"personal", "work"})
	want := "auto profile: none eligible: personal (rate-limited, resets " + sessionReset.In(time.Local).Format(resetClock) + "); work (expired)"
	if err == nil || err.Error() != want {
		t.Fatalf("err %v\nwant %s", err, want)
	}
	if apperr.As(err).Code != apperr.CodeAutoPick || apperr.Exit(err, true) != apperr.ExitLaunchFailed {
		t.Errorf("code %s exit %d", apperr.As(err).Code, apperr.Exit(err, true))
	}
}

// TestEdge14AutoProfileOnlyPerToken: only per-token profiles is refused,
// without a request.
func TestEdge14AutoProfileOnlyPerToken(t *testing.T) {
	m, fv := sharedMachine(t, nil)
	a := m.app(t)
	_, _, err := a.autoProfile(context.Background(), mustAgent(t, a, "claude"), []string{"kimi"})
	if err == nil || err.Error() != "auto profile: every candidate bills per token" || apperr.Exit(err, true) != apperr.ExitLaunchFailed {
		t.Fatalf("err %v", err)
	}
	if fv.count("/api/oauth/usage") != 0 {
		t.Error("a per-token profile was read")
	}
}

// TestEdge15AutoProfileNoUsage: an agent without usage is refused.
func TestEdge15AutoProfileNoUsage(t *testing.T) {
	m, _ := sharedMachine(t, nil)
	a := m.app(t)
	_, _, err := a.autoProfile(context.Background(), mustAgent(t, a, "pi"), []string{"pi"})
	if err == nil || err.Error() != "pi has no usage; choose a profile" || apperr.As(err).Code != apperr.CodeUnsupported || apperr.Exit(err, true) != apperr.ExitLaunchFailed {
		t.Fatalf("err %v", err)
	}
}

// TestAutoLoginProbesAndPicks: logins are probed (cost Q); an api-key
// login is per-token; the one with most headroom wins.
func TestAutoLoginProbesAndPicks(t *testing.T) {
	m, fv := sharedMachine(t, map[string]account{tokCarol: {session: 60, weekly: 10}, tokDave: {session: 20, weekly: 30}})
	a := m.app(t)
	name, reason, err := a.autoLogin(context.Background(), mustAgent(t, a, "claude"), []string{"carol", "dave", "key"})
	if err != nil || name != "dave" || !strings.Contains(reason, "auto → dave (max 30%) over carol (max 60%)") {
		t.Fatalf("name %s reason %q err %v", name, reason, err)
	}
	if fv.count("/v1/messages") != 2 {
		t.Errorf("probes %d", fv.count("/v1/messages"))
	}
	m.env["CAROL_TOKEN"], m.env["DAVE_TOKEN"] = "", ""
	_, _, err = m.app(t).autoLogin(context.Background(), mustAgent(t, a, "claude"), []string{"carol", "dave"})
	if err == nil || !strings.Contains(err.Error(), "auto login: none eligible: carol (") {
		t.Errorf("unresolved tokens: %v", err)
	}
}

// TestAutoLoginNoProber: an agent that cannot probe a token is refused.
func TestAutoLoginNoProber(t *testing.T) {
	m, _ := sharedMachine(t, nil)
	a := m.app(t)
	_, _, err := a.autoLogin(context.Background(), mustAgent(t, a, "codex"), []string{"x"})
	if apperr.As(err).Code != apperr.CodeUnsupported || !strings.HasPrefix(err.Error(), "codex cannot probe a login's usage") {
		t.Fatalf("err %v", err)
	}
}

// TestAutoProfileDryRunDecision: through the launch path, --auto-profile
// is a decision whose source carries the explanation.
func TestAutoProfileDryRunDecision(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokPersonal: {session: 70}, tokWork: {session: 20}})
	m.f.FakeAgent("claude")
	a := m.app(t)
	o := LaunchOptions{Mode: ModeRun, DryRun: true}
	o.AutoProfile, o.Agent = true, "claude"
	p, err := a.Prepare(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Profile != "work" {
		t.Errorf("profile %s", p.Profile)
	}
	found := false
	for _, d := range p.Decisions {
		if d.Name == resolve.DecProfile && strings.HasPrefix(d.Source, "--auto-profile (auto → work (max 20%)") {
			found = true
		}
	}
	if !found {
		t.Errorf("decisions %+v", p.Decisions)
	}
}
