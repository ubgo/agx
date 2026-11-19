package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// poolConfig: a pool "auto" with own members alice and bob and shared
// members carol and derek (fake tokens from the env), a log file and a
// 30s key ttl; one Claude profile so a launch can resolve.
const poolConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}", "account": "alice@example.com" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}", "account": "bob@example.com" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "derek": { "agents": ["claude"], "kind": "oauth-token", "token": "${DEREK_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob"], "shared": ["carol", "derek"],
      "accounts": { "alice": { "short": 95, "week": 98 }, "bob": { "short": 95, "week": 98 } },
      "key_ttl": "30s", "log": "~/pool.log" } }
  }
}`

// poolMachine is poolConfig with a fake vendor answering by token:
// alice = tokPersonal, bob = tokWork, carol = tokCarol, derek = tokDave.
func poolMachine(t *testing.T, accounts map[string]account) (*machine, *fakeVendor) {
	t.Helper()
	m, fv := sharedMachine(t, accounts)
	m.f.WriteConfig(poolConfig)
	m.env["ALICE_TOKEN"], m.env["BOB_TOKEN"], m.env["DEREK_TOKEN"] = tokPersonal, tokWork, tokDave
	return m, fv
}

// TestPoolPickProbesOnceThenReusesCache: the first pick probes every
// member and writes the cache 0600 with account labels and no token; a
// second pick within usage_max_age probes nothing.
func TestPoolPickProbesOnceThenReusesCache(t *testing.T) {
	m, fv := poolMachine(t, map[string]account{tokPersonal: {session: 10, weekly: 30}, tokWork: {session: 5, weekly: 20}, tokCarol: {session: 1, weekly: 1}, tokDave: {session: 60, weekly: 10}})
	a := m.app(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Every weekly reset is the same: ties keep config order (alice);
	// carol resets in 66h, outside the 48h window, so she waits.
	if rep.Selected == nil || rep.Selected.Name != "alice" || rep.Source != sourceLive {
		t.Fatalf("selected %+v source %s", rep.Selected, rep.Source)
	}
	states := map[string]pool.State{}
	for _, c := range rep.Candidates {
		states[c.Name] = c.State
	}
	if states["carol"] != pool.StateWaiting || states["derek"] != pool.StateBlocked || states["bob"] != pool.StateReady {
		t.Errorf("states %v", states)
	}
	if n := fv.count("/v1/messages"); n != 4 {
		t.Errorf("probes %d, want 4", n)
	}
	st, err := os.Stat(a.paths.LoginUsage())
	if err != nil || st.Mode().Perm() != config.TokensFileMode {
		t.Fatalf("cache %v %v", st, err)
	}
	b, _ := os.ReadFile(a.paths.LoginUsage())
	if !strings.Contains(string(b), "alice@example.com") || strings.Contains(string(b), tokPersonal) {
		t.Errorf("cache content:\n%s", b)
	}
	rep, err = a.PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil || rep.Selected.Name != "alice" || !strings.HasPrefix(rep.Source, sourceCache) {
		t.Fatalf("second pick %+v %s %v", rep.Selected, rep.Source, err)
	}
	if n := fv.count("/v1/messages"); n != 4 {
		t.Errorf("probes after reuse %d, want still 4", n)
	}
}

// writeCache seeds the login-usage cache with one entry per name, probed
// ago before the fixture clock.
func writeCache(t *testing.T, a *App, ago time.Duration, entries map[string]login.UsageEntry) {
	t.Helper()
	for name, e := range entries {
		e.Agent, e.ProbedAt = "claude", agenttest.FixedNow().Add(-ago)
		entries[name] = e
	}
	if _, err := login.UpdateUsageCache(context.Background(), a.paths, entries); err != nil {
		t.Fatal(err)
	}
}

// okEntry is an answered entry with a short and a weekly window.
func okEntry(short, week float64, weekReset time.Time) login.UsageEntry {
	return login.UsageEntry{Status: string(StatusOK), Windows: []login.UsageCacheWindow{
		{Class: "short", Percent: short, ResetsAt: &sessionReset}, {Class: "week", Percent: week, ResetsAt: &weekReset},
	}}
}

// TestPoolProbeFailureFallsBackToCache: a member whose probe fails for a
// passing reason uses its old cached reading, with a warning; --cached
// probes nothing at all.
func TestPoolProbeFailureFallsBackToCache(t *testing.T) {
	m, fv := poolMachine(t, map[string]account{tokPersonal: {status: http.StatusInternalServerError, noHeaders: true}, tokWork: {weekly: 99}, tokCarol: {weekly: 90}, tokDave: {weekly: 90}})
	a := m.app(t)
	writeCache(t, a, 10*time.Minute, map[string]login.UsageEntry{"alice": okEntry(3, 40, weeklyReset)})
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Selected.Name != "alice" || len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "probe of alice failed") || !strings.Contains(rep.Warnings[0], "10m0s ago") {
		t.Fatalf("selected %s warnings %+v", rep.Selected.Name, rep.Warnings)
	}
	before := fv.count("/v1/messages")
	rep, err = a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Cached: true}, TimeOptions{})
	if err != nil || fv.count("/v1/messages") != before {
		t.Fatalf("cached pick probed: %d → %d (%v)", before, fv.count("/v1/messages"), err)
	}
	if rep.Selected.Name != "alice" {
		t.Errorf("cached pick %s", rep.Selected.Name)
	}
}

// TestPoolNoneUsable: every member blocked is code auto-pick, exit 1,
// naming each member's reasons, with the report still returned.
func TestPoolNoneUsable(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {session: 100, weekly: 100}, tokWork: {weekly: 99}, tokCarol: {status: http.StatusUnauthorized}, tokDave: {session: 50}})
	a := m.app(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto"}, TimeOptions{})
	if err == nil || apperr.As(err).Code != apperr.CodeAutoPick || apperr.Exit(err, false) != apperr.ExitFailure {
		t.Fatalf("err %v", err)
	}
	for _, want := range []string{"alice (status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%); week is 100% (cap 98%))", "bob (week is 99% (cap 98%))", "carol (status is rejected)", "derek (5h is 50% (cap 50%))"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err %v\nmissing %s", err, want)
		}
	}
	if len(rep.Candidates) != 4 || rep.Selected != nil {
		t.Errorf("report %+v", rep)
	}
}

// TestPoolForce: --force picks a member over every rule with a warning
// per rule; forcing a login that is not one is not found.
func TestPoolForce(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {weekly: 10}, tokWork: {weekly: 100}, tokCarol: {weekly: 1}, tokDave: {weekly: 1}})
	a := m.app(t)
	rep, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Force: "bob"}, TimeOptions{})
	if err != nil || !rep.Selected.Forced || rep.Selected.Name != "bob" {
		t.Fatalf("%+v %v", rep.Selected, err)
	}
	if w := rep.StderrWarnings(); len(w) != 2 || w[1].Message != "forced bob: week is 100% (cap 98%)" {
		t.Errorf("warnings %+v", w)
	}
	if _, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "auto", Force: "zoe"}, TimeOptions{}); err == nil || apperr.As(err).Code != apperr.CodeNotFound {
		t.Errorf("unknown force: %v", err)
	}
	if _, err := a.PoolPick(context.Background(), PoolPickOptions{ID: "alice"}, TimeOptions{}); err == nil || !strings.Contains(err.Error(), "is for pool logins") {
		t.Errorf("token login: %v", err)
	}
}

// liveRun makes a live run folder whose start login is start and
// returns its id, set as AGX_RUN_ID.
func liveRun(t *testing.T, m *machine, a *App, start string) string {
	t.Helper()
	id, err := run.NewID(agenttest.FixedNow())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := run.Create(a.paths.Runs, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Create(dir, live.State{RunID: id, Agent: "claude", StartLogin: start, Helper: "agx credential"}); err != nil {
		t.Fatal(err)
	}
	m.env[run.EnvRunID] = id
	return id
}

// TestPoolCredential: agx credential on a pool run picks a member,
// returns its token, records it in use and logs one line without the
// token; agx live use pins a member for the run, probing nothing.
func TestPoolCredential(t *testing.T) {
	m, fv := poolMachine(t, map[string]account{tokPersonal: {weekly: 99}, tokWork: {weekly: 20}, tokCarol: {weekly: 1}, tokDave: {weekly: 1}})
	a := m.app(t)
	id := liveRun(t, m, a, "auto")
	cr, err := a.Credential(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cr.Login != "bob" || cr.Pool != "auto" || cr.Token.Reveal() != tokWork {
		t.Fatalf("credential %s/%s", cr.Pool, cr.Login)
	}
	st, _ := live.Read(run.Dir(a.paths.Runs, id))
	if st.InUse != "bob" || st.Want() != "auto" {
		t.Errorf("state %+v", st)
	}
	logb, err := os.ReadFile(filepath.Join(m.f.Home, "pool.log"))
	if err != nil || !strings.Contains(string(logb), `mode=credential pool="auto" picked="bob" group="own" forced=false by="rules"`) || strings.Contains(string(logb), tokWork) {
		t.Fatalf("log %q %v", logb, err)
	}
	if _, err := a.LiveUse(context.Background(), LiveUseOptions{ID: id, Login: "carol"}); err != nil {
		t.Fatal(err)
	}
	probes := fv.count("/v1/messages")
	cr, err = a.Credential(context.Background())
	if err != nil || cr.Login != "carol" || cr.Pool != "" || cr.Token.Reveal() != tokCarol || fv.count("/v1/messages") != probes {
		t.Fatalf("pinned credential %s/%s err %v", cr.Pool, cr.Login, err)
	}
}

// TestPoolCredentialNoneUsableIsOneLine: no member usable fails the
// helper with one line naming the reasons.
func TestPoolCredentialNoneUsableIsOneLine(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {weekly: 99}, tokWork: {weekly: 99}, tokCarol: {weekly: 80}, tokDave: {weekly: 80}})
	a := m.app(t)
	liveRun(t, m, a, "auto")
	_, err := a.Credential(context.Background())
	if err == nil || strings.Contains(err.Error(), "\n") || !strings.Contains(err.Error(), "pool auto: no member is usable") || !strings.Contains(err.Error(), "(try: agx pool pick --id auto --pool-force NAME)") {
		t.Fatalf("err %v", err)
	}
}

// TestPoolLoginToken: agx login token on a pool prints the picked
// member's token.
func TestPoolLoginToken(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {weekly: 5}, tokWork: {weekly: 5}, tokCarol: {weekly: 5}, tokDave: {weekly: 5}})
	a := m.app(t)
	tok, warnings, err := a.LoginTokenWarn(context.Background(), "auto", "")
	if err != nil || tok.Reveal() != tokPersonal || len(warnings) != 0 {
		t.Fatalf("token err %v warnings %v", err, warnings)
	}
}

// TestPoolLaunchIsLive: a run on a pool is live without --live-login,
// with the pool's key_ttl as the refresh; a dry run probes nothing and
// shows the pick from the cache, or "picks at launch" without one.
func TestPoolLaunchIsLive(t *testing.T) {
	m, fv := poolMachine(t, nil)
	m.f.FakeAgent("claude")
	a := m.app(t)
	o := LaunchOptions{Mode: ModeRun, DryRun: true}
	o.Login = "auto"
	p, err := a.Prepare(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	notes := decisionValues(p)
	if p.Live == nil || p.Live.StartLogin != "auto" || !strings.Contains(notes, "pool auto (claude): own alice, bob; shared carol, derek [config logins.auto.pool]") ||
		!strings.Contains(notes, "pick now: none from the cache") || !strings.Contains(notes, "refresh 30s") {
		t.Fatalf("live %+v notes:\n%s", p.Live, notes)
	}
	writeCache(t, a, time.Minute, map[string]login.UsageEntry{"alice": okEntry(1, 10, weeklyReset), "bob": okEntry(1, 10, weeklyReset)})
	p, err = a.Prepare(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if notes := decisionValues(p); !strings.Contains(notes, "pick now: alice (rules), from the cache (1m0s old)") {
		t.Errorf("notes:\n%s", notes)
	}
	if fv.count("/v1/messages") != 0 {
		t.Errorf("a dry run probed %d times", fv.count("/v1/messages"))
	}
}

// decisionValues joins a plan's decisions, one per line.
func decisionValues(p *LaunchPlan) string {
	var b strings.Builder
	for _, d := range p.Decisions {
		b.WriteString(d.Name + " " + d.Value + " [" + d.Source + "]\n")
	}
	return b.String()
}

// TestPoolAddEditRemove: login add --kind pool writes the block; edit
// replaces lists and sets or clears force; a member cannot be removed
// while the pool lists it, and --cascade drops it from the pool.
func TestPoolAddEditRemove(t *testing.T) {
	m, _ := poolMachine(t, nil)
	a := m.app(t)
	ctx := context.Background()
	ch, err := a.LoginAdd(ctx, LoginAddOptions{Name: "team", Agents: agentIDsOf("claude"), Kind: config.KindPool, PoolOwn: []string{"alice"}, PoolShared: []string{"carol"}})
	if err != nil || ch.Info.Source != "pool 1 own, 1 shared" {
		t.Fatalf("add %+v %v", ch.Info, err)
	}
	if _, err := a.LoginAdd(ctx, LoginAddOptions{Name: "bad", Agents: agentIDsOf("claude"), Kind: config.KindPool, PoolOwn: []string{"zoe"}}); err == nil || !strings.Contains(err.Error(), "unknown login zoe") {
		t.Errorf("unknown member: %v", err)
	}
	if _, err := a.LoginAdd(ctx, LoginAddOptions{Name: "bad", Agents: agentIDsOf("claude"), Kind: config.KindPool}); err == nil || !strings.Contains(err.Error(), "a pool needs members") {
		t.Errorf("no members: %v", err)
	}
	if _, err := a.LoginAdd(ctx, LoginAddOptions{Name: "bad", Agents: agentIDsOf("codex"), Kind: config.KindPool, PoolOwn: []string{"alice"}}); err == nil || apperr.Exit(err, false) != apperr.ExitUnsupported {
		t.Errorf("codex pool: %v", err)
	}
	force := "carol"
	if _, err := a.LoginEdit(ctx, LoginEditOptions{ID: "team", PoolOwn: []string{"alice", "bob"}, PoolForce: &force}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := a.Config()
	if p := cfg.Config.Logins["team"].Pool; len(p.Own) != 2 || p.Force == nil || *p.Force != "carol" {
		t.Fatalf("edited %+v", p)
	}
	none := ""
	if _, err := a.LoginEdit(ctx, LoginEditOptions{ID: "team", PoolForce: &none}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoginEdit(ctx, LoginEditOptions{ID: "alice", PoolOwn: []string{"bob"}}); err == nil || !strings.Contains(err.Error(), "are for pool logins") {
		t.Errorf("pool flags on token login: %v", err)
	}
	if _, err := a.LoginEdit(ctx, LoginEditOptions{ID: "team", Source: LoginSource{TokenRef: "x"}}); err == nil || !strings.Contains(err.Error(), "holds no value") {
		t.Errorf("value on pool: %v", err)
	}
	if _, err := a.LoginRemove(ctx, LoginRemoveOptions{ID: "carol"}); err == nil || !strings.Contains(err.Error(), "pool auto") || !strings.Contains(err.Error(), "pool team") {
		t.Fatalf("remove member: %v", err)
	}
	if _, err := a.LoginRemove(ctx, LoginRemoveOptions{ID: "carol", Cascade: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err = a.Config()
	if err != nil {
		t.Fatal(err)
	}
	if p := cfg.Config.Logins["team"].Pool; len(p.Shared) != 0 || p.Force != nil {
		t.Errorf("after cascade %+v", p)
	}
}

// TestPoolSkippedByUsageCheckAndAutoLogin: --all-logins probes members,
// never the pool; login check skips the pool; --auto-login ignores it.
func TestPoolSkippedByUsageCheckAndAutoLogin(t *testing.T) {
	m, _ := poolMachine(t, map[string]account{tokPersonal: {weekly: 5}, tokWork: {weekly: 5}, tokCarol: {weekly: 5}, tokDave: {weekly: 5}})
	a := m.app(t)
	rows, err := a.LoginCheck(context.Background(), LoginCheckOptions{IDs: []string{"auto"}})
	if err != nil || rows[0].Probe != CheckSkipped || rows[0].Detail != detailPool {
		t.Fatalf("check %+v %v", rows, err)
	}
	cfg, _ := a.Config()
	ags := []agentio.Agent{mustAgent(t, a, "claude")}
	if _, err := a.chosenLogins(context.Background(), cfg, ags, UsageOptions{Logins: []string{"auto"}}); err == nil || !strings.Contains(err.Error(), "is a pool") {
		t.Errorf("usage --login on a pool: %v", err)
	}
	all, err := a.chosenLogins(context.Background(), cfg, ags, UsageOptions{})
	if err != nil || slices.Contains(all, "auto") || len(all) != 4 {
		t.Errorf("all logins %v %v", all, err)
	}
	if got := resolve.LoginsServing(cfg.Config, "claude"); slices.Contains(got, "auto") {
		t.Errorf("auto-login candidates %v", got)
	}
}

// agentIDsOf converts names to agent ids.
func agentIDsOf(names ...string) []agentio.ID {
	out := make([]agentio.ID, len(names))
	for i, n := range names {
		out[i] = agentio.ID(n)
	}
	return out
}
