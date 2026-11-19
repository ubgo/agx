package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/testkit"
)

// The app-level usage tests run against a fixture machine (testkit) whose
// agentio Host sends every HTTP request to an httptest server: the vendor
// endpoints are fixed in agentio, so the client's transport rewrites the
// origin. Nothing reaches the network; tokens are fake values that match
// no secret-scan pattern.

// Fake tokens, one per account; the server answers by token.
const (
	tokPersonal = "fake-token-PERSONAL"
	tokWork     = "fake-token-WORK"
	tokCarol    = "fake-token-CAROL"
	tokDave     = "fake-token-DAVE"
)

// account is how the fake server answers one token.
type account struct {
	// status is the HTTP status of every answer (0 = 200).
	status int
	// session and weekly are the percents of the two windows.
	session, weekly float64
	// headers is false to answer a probe without rate-limit headers.
	noHeaders bool
}

// fakeVendor is the httptest server standing in for the vendor API.
type fakeVendor struct {
	srv *httptest.Server
	mu  sync.Mutex
	// accounts maps a bearer token to its answers.
	accounts map[string]account
	// requests counts requests by path.
	requests map[string]int
}

// fixedReset is every window's reset in the fake answers.
var (
	sessionReset = time.Date(2026, 11, 13, 14, 0, 0, 0, time.UTC)
	weeklyReset  = time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)
)

func newFakeVendor(t *testing.T, accounts map[string]account) *fakeVendor {
	t.Helper()
	fv := &fakeVendor{accounts: accounts, requests: map[string]int{}}
	fv.srv = httptest.NewServer(http.HandlerFunc(fv.serve))
	t.Cleanup(fv.srv.Close)
	return fv
}

func (fv *fakeVendor) serve(w http.ResponseWriter, r *http.Request) {
	fv.mu.Lock()
	fv.requests[r.URL.Path]++
	acc, ok := fv.accounts[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	fv.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/api/oauth/usage":
		if acc.status != 0 {
			w.WriteHeader(acc.status)
			return
		}
		_, _ = fmt.Fprintf(w, `{"limits":[{"kind":"session","group":"session","percent":%g,"resets_at":%q},{"kind":"weekly_all","group":"weekly","percent":%g,"resets_at":%q}]}`,
			acc.session, sessionReset.Format(time.RFC3339), acc.weekly, weeklyReset.Format(time.RFC3339)) // a test server write error surfaces as the client's error
	case "/api/oauth/profile":
		_, _ = fmt.Fprint(w, `{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x","billing_type":"stripe_subscription","subscription_status":"active","subscription_created_at":"2025-01-02T00:00:00Z"}}`) // as above
	case "/v1/messages":
		if !acc.noHeaders {
			h := w.Header()
			h.Set("anthropic-ratelimit-unified-5h-utilization", strconv.FormatFloat(acc.session/100, 'f', -1, 64))
			h.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(sessionReset.Unix(), 10))
			h.Set("anthropic-ratelimit-unified-7d-utilization", strconv.FormatFloat(acc.weekly/100, 'f', -1, 64))
			h.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(weeklyReset.Unix(), 10))
		}
		if acc.status != 0 {
			w.WriteHeader(acc.status)
			return
		}
		_, _ = fmt.Fprint(w, `{"content":[]}`) // as above
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// count is the number of requests to path.
func (fv *fakeVendor) count(path string) int {
	fv.mu.Lock()
	defer fv.mu.Unlock()
	return fv.requests[path]
}

// redirect sends every request to the fake vendor's origin.
type redirect struct{ to *url.URL }

func (rd redirect) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme, r2.URL.Host, r2.Host = rd.to.Scheme, rd.to.Host, rd.to.Host
	return http.DefaultTransport.RoundTrip(r2)
}

// noNetwork fails every request the way a machine without network does.
type noNetwork struct{}

func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.DNSError{Err: "no such host", Name: "api.anthropic.com", IsNotFound: true}
}

// machine is a fixture machine plus its App.
type machine struct {
	f   *testkit.Fixture
	env map[string]string
	rt  http.RoundTripper
}

func newMachine(t *testing.T, cfg string) *machine {
	t.Helper()
	m := &machine{f: testkit.New(t), env: map[string]string{}}
	m.f.WriteConfig(cfg)
	return m
}

// app builds the App over the fixture (call after setting rt and env).
func (m *machine) app(t *testing.T) *App {
	t.Helper()
	client := agenttest.FailingHTTP(t)
	if m.rt != nil {
		client = &http.Client{Transport: m.rt}
	}
	h := agenttest.NewHost(t, agenttest.WithUserHome(m.f.Home), agenttest.WithEnv("HOME="+m.f.Home, "PATH="+m.f.Bin), agenttest.WithHTTP(client))
	return New(Deps{
		Host:       h,
		Lookup:     func(k string) (string, bool) { v, ok := m.env[k]; return v, ok },
		Cwd:        m.f.Work,
		ConfigFlag: filepath.Join(m.f.ConfigDir, "config.jsonc"),
	})
}

// claudeHome makes a Claude home holding a stored login with tok
// (expiring at exp) and, when cachedAgo > 0, a cached reading that old.
func (m *machine) claudeHome(rel, tok string, exp time.Time, cached float64, cachedAgo time.Duration) {
	dir := m.f.HomeDir(rel)
	if tok != "" {
		m.f.WriteFile(filepath.Join(dir, ".credentials.json"), fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`, tok, exp.UnixMilli()), 0o600)
	}
	if cachedAgo > 0 {
		fetched := agenttest.FixedNow().Add(-cachedAgo)
		m.f.WriteFile(filepath.Join(dir, ".claude.json"), fmt.Sprintf(`{"cachedUsageUtilization":{"fetchedAtMs":%d,"utilization":{"limits":[{"kind":"session","group":"session","percent":%g,"resets_at":%q},{"kind":"weekly_all","group":"weekly","percent":%g,"resets_at":%q}]}}}`,
			fetched.UnixMilli(), cached, sessionReset.Format(time.RFC3339), cached, weeklyReset.Format(time.RFC3339)), 0o600)
	}
}

// later is a login expiry after the fixture clock.
var later = agenttest.FixedNow().Add(30 * 24 * time.Hour)

// sharedConfig: personal (plan) and kimi (api) share ~/.claude-personal;
// work has its own home; carol and dave are stored logins; pi has a home.
const sharedConfig = `{
  "version": 1,
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude-personal" },
    "kimi": { "agent": "claude", "home": "~/.claude-personal", "billing": "api" },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "pi": { "agent": "pi", "home": "~/.pi-home" }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}", "account": "carol@example.test" },
    "dave": { "agents": ["claude"], "kind": "oauth-token", "token": "${DAVE_TOKEN}" },
    "key": { "agents": ["claude"], "kind": "api-key", "token": "${KEY_TOKEN}" }
  }
}`

// sharedMachine is sharedConfig with both Claude homes logged in.
func sharedMachine(t *testing.T, accounts map[string]account) (*machine, *fakeVendor) {
	t.Helper()
	m := newMachine(t, sharedConfig)
	m.claudeHome(".claude-personal", tokPersonal, later, 0, 0)
	m.claudeHome(".claude-work", tokWork, later, 0, 0)
	m.f.HomeDir(".pi-home")
	m.env["CAROL_TOKEN"], m.env["DAVE_TOKEN"], m.env["KEY_TOKEN"] = tokCarol, tokDave, "fake-key-VALUE"
	fv := newFakeVendor(t, accounts)
	u, err := url.Parse(fv.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	m.rt = redirect{to: u}
	return m, fv
}

// rowNamed finds a row by name.
func rowNamed(t *testing.T, rows []UsageRow, name string) UsageRow {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q in %+v", name, rows)
	return UsageRow{}
}

// TestUsageLiveSharedHomeOnceGolden: live usage, one request per home,
// the shared home once with both profiles and the plan profile as its
// name, pi's "usage not modelled" row, and the --json data shape.
func TestUsageLiveSharedHomeOnceGolden(t *testing.T) {
	m, fv := sharedMachine(t, map[string]account{tokPersonal: {session: 34, weekly: 12}, tokWork: {session: 5, weekly: 40}})
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{TZ: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 3 || fv.count("/api/oauth/usage") != 2 {
		t.Fatalf("rows %d, requests %d", len(rep.Rows), fv.count("/api/oauth/usage"))
	}
	p := rowNamed(t, rep.Rows, "personal")
	if strings.Join(p.Profiles, ",") != "kimi,personal" || p.Status != StatusOK || *p.Left != 66 || p.Source != SourceLive {
		t.Errorf("personal row %+v", p)
	}
	if p.ShortWindow() == nil || p.ShortWindow().Reset != "Fri 13 Nov 14:00" || p.WeeklyWindow() == nil {
		t.Errorf("windows %+v", p.Windows)
	}
	if pi := rowNamed(t, rep.Rows, "pi"); pi.Status != StatusUnsupported || pi.Reason == "" {
		t.Errorf("pi row %+v", pi)
	}
	b, err := json.Marshal(rep.Rows)
	if err != nil {
		t.Fatal(err)
	}
	m.f.Golden(t, "usage-live", string(b))
}

// TestUsagePerTokenHomeSendsNothing: a home whose every profile bills per
// token is answered without a request.
func TestUsagePerTokenHomeSendsNothing(t *testing.T) {
	m2 := newMachine(t, `{"version":1,"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`)
	m2.f.HomeDir(".claude-api")
	rep, err := m2.app(t).Usage(context.Background(), UsageOptions{})
	if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Status != StatusPerToken {
		t.Fatalf("err %v rows %+v", err, rep.Rows)
	}
}

// TestEdge57PartialAndNone: one home answers and one is rate-limited:
// partial (exit 3) with both rows; without network every row fails:
// exit 1, code network.
func TestEdge57PartialAndNone(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokPersonal: {session: 10}, tokWork: {status: http.StatusTooManyRequests}})
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Agent: "claude"})
	if apperr.Exit(err, false) != apperr.ExitPartial || apperr.As(err).Code != apperr.CodePartial {
		t.Fatalf("err %v", err)
	}
	if w := rowNamed(t, rep.Rows, "work"); w.Status != StatusRateLimited {
		t.Errorf("work %+v", w)
	}
	if !strings.Contains(err.Error(), "work (rate-limited)") {
		t.Errorf("message %q", err)
	}

	m.rt = noNetwork{}
	rep, err = m.app(t).Usage(context.Background(), UsageOptions{Agent: "claude"})
	if apperr.Exit(err, false) != apperr.ExitFailure || apperr.As(err).Code != apperr.CodeNetwork || len(rep.Rows) != 2 {
		t.Fatalf("no network: err %v rows %d", err, len(rep.Rows))
	}
	for _, r := range rep.Rows {
		if r.Status != StatusNetwork {
			t.Errorf("row %+v", r)
		}
	}
}

// TestUsageExpiredLoginSendsNothing: an expired home login is reported
// as expired without a request (never refreshed).
func TestUsageExpiredLoginSendsNothing(t *testing.T) {
	m, fv := sharedMachine(t, map[string]account{tokPersonal: {session: 1}})
	m.claudeHome(".claude-work", tokWork, agenttest.FixedNow().Add(-time.Hour), 0, 0)
	rep, _ := m.app(t).Usage(context.Background(), UsageOptions{Profiles: []string{"work"}})
	if len(rep.Rows) != 1 || rep.Rows[0].Status != StatusExpired || fv.count("/api/oauth/usage") != 0 {
		t.Fatalf("rows %+v requests %d", rep.Rows, fv.count("/api/oauth/usage"))
	}
}

// TestUsagePlan: --plan reads the subscription live.
func TestUsagePlan(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokPersonal: {session: 1}, tokWork: {session: 1}})
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Profiles: []string{"work"}, Plan: true})
	if err != nil {
		t.Fatal(err)
	}
	p := rep.Rows[0].Plan
	if p == nil || p.Plan != "Max (20x)" || p.Status != "active" || p.StartedAt == nil || p.Source != SourceLive {
		t.Fatalf("plan %+v", p)
	}
}

// TestUsageCachedReadsDiskOnly: --cached makes no request and shows age.
func TestUsageCachedReadsDiskOnly(t *testing.T) {
	m, fv := sharedMachine(t, nil)
	m.claudeHome(".claude-work", tokWork, later, 20, 4*time.Minute)
	m.rt = nil // FailingHTTP: any request fails the test
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Cached: true, Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	w := rowNamed(t, rep.Rows, "work")
	if w.Source != SourceCached || w.Age != "4m" || *w.Left != 80 {
		t.Errorf("work %+v", w)
	}
	if p := rowNamed(t, rep.Rows, "personal"); p.Status != StatusNoCache || !strings.Contains(p.Reason, "no longer store a usage reading") {
		t.Errorf("personal %+v (want the agent's note: Claude Code stopped storing it)", p)
	}
	if fv.count("/api/oauth/usage") != 0 {
		t.Error("cached sent a request")
	}
}

// TestUsageCachedStale: a reading older than StaleAfter is "stale", keeps
// its windows, and says why (FIX1 item 3: Claude Code no longer refreshes
// it); a fresh one is "ok" with no reason.
func TestUsageCachedStale(t *testing.T) {
	m, _ := sharedMachine(t, nil)
	m.claudeHome(".claude-work", tokWork, later, 20, 20*24*time.Hour)
	m.rt = nil
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Cached: true, Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	w := rowNamed(t, rep.Rows, "work")
	if w.Status != StatusStale || w.Age != "20d" || len(w.Windows) == 0 || !strings.Contains(w.Reason, "reading is 20d old") || !strings.Contains(w.Reason, "no longer store") {
		t.Errorf("work %+v", w)
	}
}

// TestUsageLoginRowsProbe: --login shows probed login rows instead of
// homes; an api-key login is per-token without a request; --login-probes
// adds every login to the homes.
func TestUsageLoginRowsProbe(t *testing.T) {
	m, fv := sharedMachine(t, map[string]account{tokPersonal: {session: 1}, tokWork: {session: 1}, tokCarol: {session: 50, weekly: 20}})
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Logins: []string{"carol", "key"}})
	if err != nil {
		t.Fatal(err)
	}
	c := rowNamed(t, rep.Rows, "carol")
	if len(rep.Rows) != 2 || c.Kind != RowLogin || c.Source != SourceProbe || *c.Left != 50 || c.Account != "carol@example.test" {
		t.Errorf("rows %+v", rep.Rows)
	}
	if k := rowNamed(t, rep.Rows, "key"); k.Status != StatusPerToken {
		t.Errorf("key %+v", k)
	}
	if fv.count("/v1/messages") != 1 {
		t.Errorf("probes %d, want 1", fv.count("/v1/messages"))
	}
	rep, _ = m.app(t).Usage(context.Background(), UsageOptions{LoginProbes: true, Agent: "claude"})
	if len(rep.Rows) != 5 {
		t.Errorf("--login-probes rows %d, want 2 homes + 3 logins", len(rep.Rows))
	}
	if d := rowNamed(t, rep.Rows, "dave"); d.Status != StatusRejected {
		t.Errorf("dave (unknown to the server) %+v", d)
	}
	for _, r := range rep.Rows {
		if strings.Contains(fmt.Sprint(r), "fake-token") {
			t.Errorf("token in row %+v", r)
		}
	}
}

// TestUsageWeeklyLimit429IsUsage: a probe answering 429 with headers is
// usage at its cap, not a failure (memory: weekly-limit-429).
func TestUsageWeeklyLimit429IsUsage(t *testing.T) {
	m, _ := sharedMachine(t, map[string]account{tokCarol: {status: http.StatusTooManyRequests, session: 40, weekly: 100}})
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{Logins: []string{"carol"}})
	if err != nil || rep.Rows[0].Status != StatusLimit || *rep.Rows[0].Left != 0 {
		t.Fatalf("err %v rows %+v", err, rep.Rows)
	}
}

// TestUsageFlagRefusals: conflicts and bad values are usage errors (exit
// 2) before anything is read.
func TestUsageFlagRefusals(t *testing.T) {
	m, _ := sharedMachine(t, nil)
	a := m.app(t)
	for name, o := range map[string]UsageOptions{
		"cached+login":       {Cached: true, Logins: []string{"carol"}},
		"login+all":          {Logins: []string{"carol"}, AllLogins: true},
		"profile+all":        {Profiles: []string{"work"}, AllLogins: true},
		"sort":               {Sort: []string{"lefty"}},
		"tz":                 {TimeOptions: TimeOptions{TZ: "Mars/Olympus"}},
		"profile+agent":      {Profiles: []string{"work"}, Agent: "pi"},
		"picker without tty": {LoginPicker: true},
	} {
		_, err := a.Usage(context.Background(), o)
		if apperr.Exit(err, false) != apperr.ExitUsage {
			t.Errorf("%s: err %v", name, err)
		}
	}
	_, err := a.Usage(context.Background(), UsageOptions{Logins: []string{"nobody"}})
	if apperr.As(err).Code != apperr.CodeNotFound {
		t.Errorf("unknown login: %v", err)
	}
}

// TestUsageSortRows covers the --sort keys, direction and missing values.
func TestUsageSortRows(t *testing.T) {
	left := func(f float64) *float64 { return &f }
	mon := time.Date(2026, 11, 16, 6, 0, 0, 0, time.UTC)
	wed := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)
	week := func(at time.Time) []UsageWindow {
		return []UsageWindow{{Class: agentio.WindowWeek.String(), Percent: 10, ResetsAt: &at}}
	}
	rows := []UsageRow{
		{Name: "a", Left: left(30), Status: StatusOK, Windows: week(wed)},
		{Name: "b", Status: StatusExpired},
		{Name: "c", Left: left(80), Status: StatusOK, Windows: week(mon)},
	}
	keys, err := ParseSortKeys([]string{"-left"})
	if err != nil {
		t.Fatal(err)
	}
	SortRows(rows, keys, time.UTC)
	if got := rows[0].Name + rows[1].Name + rows[2].Name; got != "cab" {
		t.Errorf("-left: %s", got)
	}
	keys, _ = ParseSortKeys([]string{"day", "login"})
	SortRows(rows, keys, time.UTC)
	if got := rows[0].Name + rows[1].Name + rows[2].Name; got != "cab" {
		t.Errorf("day: %s", got)
	}
	keys, _ = ParseSortKeys([]string{"-status"})
	SortRows(rows, keys, time.UTC)
	if rows[0].Name != "b" {
		t.Errorf("-status first %s", rows[0].Name)
	}
}

// TestUsageReportClockAndTimeFormat: the report carries the Host clock
// (fixed in tests, so relative resets are deterministic) and the time
// format from --time-format, else config usage.time_format, else
// relative; an unknown format is refused before anything is read.
func TestUsageReportClockAndTimeFormat(t *testing.T) {
	cfg := `{"version":1,"usage":{"time_format":"day"},"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`
	m := newMachine(t, cfg)
	m.f.HomeDir(".claude-api")
	rep, err := m.app(t).Usage(context.Background(), UsageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Now.Equal(agenttest.FixedNow()) || rep.TimeFormat != config.TimeDay {
		t.Errorf("now %v format %q", rep.Now, rep.TimeFormat)
	}
	if rep, _ = m.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{TimeFormat: config.TimeAbsolute}}); rep.TimeFormat != config.TimeAbsolute {
		t.Errorf("flag over config: %q", rep.TimeFormat)
	}
	m2 := newMachine(t, `{"version":1,"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`)
	m2.f.HomeDir(".claude-api")
	if rep, _ = m2.app(t).Usage(context.Background(), UsageOptions{}); rep.TimeFormat != config.TimeRelative {
		t.Errorf("default: %q", rep.TimeFormat)
	}
	if _, err := m2.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{TimeFormat: "weekly"}}); err == nil || apperr.As(err).Class != apperr.ClassUsage {
		t.Errorf("unknown format: %v", err)
	}
}

// TestUsageReportDateFormat: the date format comes from --date-format,
// else config usage.date_format, else the top-level date_format, else the
// long preset; a bad flag is refused (exit 2) before anything is read,
// and a bad configured value fails the config load at its key.
func TestUsageReportDateFormat(t *testing.T) {
	const profiles = `"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}`
	for _, c := range []struct {
		name, cfg, flag, want string
	}{
		{"default", `{"version":1,` + profiles + `}`, "", datefmt.PresetLong},
		{"top-level", `{"version":1,"date_format":"iso",` + profiles + `}`, "", datefmt.PresetISO},
		{"usage over top-level", `{"version":1,"date_format":"iso","usage":{"date_format":"us"},` + profiles + `}`, "", datefmt.PresetUS},
		{"flag over both", `{"version":1,"date_format":"iso","usage":{"date_format":"us"},` + profiles + `}`, "DD/MM HH:mm", "DD/MM HH:mm"},
	} {
		m := newMachine(t, c.cfg)
		m.f.HomeDir(".claude-api")
		rep, err := m.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{DateFormat: c.flag}})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := rep.DateFormat.String(); got != c.want {
			t.Errorf("%s: date format %q, want %q", c.name, got, c.want)
		}
	}
	m := newMachine(t, `{"version":1,`+profiles+`}`)
	m.f.HomeDir(".claude-api")
	_, err := m.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{DateFormat: "YYYY-Q"}})
	if e := apperr.As(err); err == nil || e.Class != apperr.ClassUsage || !strings.Contains(e.Message, `--date-format "YYYY-Q": unknown token "Q"; tokens: YYYY`) {
		t.Errorf("bad flag: %v", err)
	}
	bad := newMachine(t, `{"version":1,"usage":{"date_format":"HH:mm [x"},`+profiles+`}`)
	bad.f.HomeDir(".claude-api")
	_, err = bad.app(t).Usage(context.Background(), UsageOptions{})
	if err == nil || !strings.Contains(err.Error(), "usage.date_format") || !strings.Contains(err.Error(), "unclosed [") {
		t.Errorf("bad config value: %v", err)
	}
}

// TestClassifyUsageErrStripsLibraryPrefix: a vendor error wrapping
// agentio's sentinel reads as the finding alone, never "agentio: …".
func TestClassifyUsageErrStripsLibraryPrefix(t *testing.T) {
	err := fmt.Errorf("%w at 2026-11-17T09:00:00Z: run claude once in this home to refresh it", agentio.ErrExpired)
	status, reason := classifyUsageErr(err)
	if status != StatusExpired || reason != "login expired at 2026-11-17T09:00:00Z: run claude once in this home to refresh it" {
		t.Errorf("status %s reason %q", status, reason)
	}
}

// TestUsageReportDurationFormat: --duration-format beats config
// usage.duration_format beats truncate; another value is exit 2 naming
// the values, and the config schema refuses one at load.
func TestUsageReportDurationFormat(t *testing.T) {
	m := newMachine(t, `{"version":1,"usage":{"duration_format":"round"},"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`)
	m.f.HomeDir(".claude-api")
	if rep, err := m.app(t).Usage(context.Background(), UsageOptions{}); err != nil || rep.DurationFormat != config.DurationRound {
		t.Fatalf("config: %q %v", rep.DurationFormat, err)
	}
	if rep, _ := m.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{DurationFormat: config.DurationMinutes}}); rep.DurationFormat != config.DurationMinutes {
		t.Errorf("flag over config: %q", rep.DurationFormat)
	}
	m2 := newMachine(t, `{"version":1,"profiles":{"api":{"agent":"claude","home":"~/.claude-api","billing":"api"}}}`)
	m2.f.HomeDir(".claude-api")
	if rep, _ := m2.app(t).Usage(context.Background(), UsageOptions{}); rep.DurationFormat != config.DurationTruncate {
		t.Errorf("default: %q", rep.DurationFormat)
	}
	_, err := m2.app(t).Usage(context.Background(), UsageOptions{TimeOptions: TimeOptions{DurationFormat: "hours"}})
	if err == nil || apperr.As(err).Class != apperr.ClassUsage || !strings.Contains(err.Error(), "truncate, round, minutes") {
		t.Errorf("unknown: %v", err)
	}
	m3 := newMachine(t, `{"version":1,"usage":{"duration_format":"hours"}}`)
	if _, err := m3.app(t).Usage(context.Background(), UsageOptions{}); err == nil || apperr.As(err).Code != apperr.CodeConfig {
		t.Errorf("bad config value: %v", err)
	}
}
