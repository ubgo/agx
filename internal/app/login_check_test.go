package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
)

// probeConfig has one login per probe outcome; every value is fake.
const probeConfig = `{
  "version": 1,
  "logins": {
    "plan": { "agents": ["claude"], "kind": "oauth-token", "token": "fake-token-ALPHA" },
    "expired": { "agents": ["claude"], "kind": "oauth-token", "token": "fake-token-EXPIRED" },
    "key": { "agents": ["claude"], "kind": "api-key", "token": "fake-key-BRAVO" },
    "unset": { "agents": ["claude"], "kind": "oauth-token", "token": "${NOT_SET_ANYWHERE}" },
    "gem": { "agents": ["gemini"], "kind": "api-key", "token": "fake-key-GEM" }
  }
}`

// redirect sends every request to the test server, so no probe can reach
// the real API whatever URL the agent builds.
type redirect struct{ to *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.to.Scheme, r.to.Host
	return http.DefaultTransport.RoundTrip(req)
}

// probeApp builds an App over a fixture config whose host HTTP goes to a
// fake API: the ALPHA token answers with plan windows, anything else 401.
func probeApp(t *testing.T) (*app.App, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer fake-token-ALPHA" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid token"}}`)) // the test asserts on the status only
			return
		}
		reset := strconv.FormatInt(agenttest.FixedNow().Unix()+3600, 10)
		w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.12")
		w.Header().Set("anthropic-ratelimit-unified-5h-reset", reset)
		w.Header().Set("anthropic-ratelimit-unified-7d-utilization", "0.4")
		w.Header().Set("anthropic-ratelimit-unified-7d-reset", reset)
		_, _ = w.Write([]byte(`{}`)) // body unused by the prober
	}))
	t.Cleanup(srv.Close)
	to, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".agx", "config.jsonc")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(probeConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	host := agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithHTTP(&http.Client{Transport: redirect{to}}))
	a := app.New(app.Deps{Host: host, Lookup: func(string) (string, bool) { return "", false }, ConfigFlag: cfgPath, Cwd: home})
	return a, &calls
}

// TestLoginCheckProbes covers every probe outcome and the partial error.
func TestLoginCheckProbes(t *testing.T) {
	a, calls := probeApp(t)
	rows, err := a.LoginCheck(context.Background(), app.LoginCheckOptions{})
	if e := apperr.As(err); err == nil || e.Class != apperr.ClassPartial || !strings.Contains(e.Message, "2 of 5 logins failed: expired, unset") {
		t.Fatalf("err %v", err)
	}
	got := map[string]app.LoginCheckRow{}
	for _, r := range rows {
		got[r.Name] = r
		if strings.Contains(r.Detail, "fake-") {
			t.Errorf("%s: value in detail %q", r.Name, r.Detail)
		}
	}
	if r := got["plan"]; r.Probe != app.CheckOK || len(r.Windows) != 2 || r.Detail == "" || r.Agent != "claude" {
		t.Errorf("plan %+v", r)
	}
	if r := got["expired"]; r.Resolve != app.CheckOK || r.Probe != app.CheckFailed || r.Detail != "login expired: token rejected by the server" {
		t.Errorf("expired %+v", r)
	}
	if r := got["key"]; r.Probe != app.CheckOK || r.Detail != "billed per token: no plan windows" {
		t.Errorf("key %+v", r)
	}
	if r := got["unset"]; r.Resolve != app.CheckFailed || r.Detail != "login unset: NOT_SET_ANYWHERE is not set" {
		t.Errorf("unset %+v", r)
	}
	if r := got["gem"]; r.Probe != app.CheckSkipped {
		t.Errorf("gem %+v", r)
	}
	// Exactly one request per probed oauth login: plan and expired.
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Errorf("probe calls %d, want 2", n)
	}
}

// TestLoginCheckNoProbeSpendsNothing: --no-probe never calls the API.
func TestLoginCheckNoProbeSpendsNothing(t *testing.T) {
	a, calls := probeApp(t)
	rows, err := a.LoginCheck(context.Background(), app.LoginCheckOptions{IDs: []string{"plan", "expired"}, NoProbe: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Resolve != app.CheckOK || r.Probe != app.CheckSkipped {
			t.Errorf("%+v", r)
		}
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Errorf("probe calls %d", n)
	}
}

// TestLoginCheckOneFailureIsItsOwnError: one login checked returns its
// own error (exit 1), not a partial.
func TestLoginCheckOneFailureIsItsOwnError(t *testing.T) {
	a, _ := probeApp(t)
	_, err := a.LoginCheck(context.Background(), app.LoginCheckOptions{IDs: []string{"expired"}})
	if e := apperr.As(err); e.Class != apperr.ClassFailure || e.Code != apperr.CodeSecret || apperr.Exit(err, false) != 1 {
		t.Fatalf("err %+v", e)
	}
}
