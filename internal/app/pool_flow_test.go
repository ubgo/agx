package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
)

// The stand-alone selector's test/flow.test.mjs (its docs/guides/
// testing.md list 1 to 12), ported end to end: Claude Code's key helper
// on a pool run is agx credential, called again and again on a fresh
// App each time (a new process), against an httptest vendor whose usage
// the test changes between calls and a clock the test moves. Names are
// fake: alice and carol are own, grace and ivan shared.

// flowConfig is the flow's pool: own alice, carol (caps 99/100), shared
// grace, ivan (50/70), usage_max_age 60s, a log file; extra is spliced
// into the pool block.
func flowConfig(extra string) string {
	return `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "grace": { "agents": ["claude"], "kind": "oauth-token", "token": "${GRACE_TOKEN}" },
    "ivan": { "agents": ["claude"], "kind": "oauth-token", "token": "${IVAN_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "carol"], "shared": ["grace", "ivan"],
      "accounts": { "alice": { "short": 99, "week": 100 }, "carol": { "short": 99, "week": 100 }` + extra + ` },
      ` + poolClassicKeys + `"usage_max_age": "60s", "log": "~/helper.log" } }
  }
}`
}

// flowForce is flowConfig with the pool's force set.
func flowForce(name string) string {
	return strings.Replace(flowConfig(""), `"usage_max_age"`, `"force": "`+name+`", "usage_max_age"`, 1)
}

// flow is the running fixture: machine, vendor, movable clock, run.
type flow struct {
	t   *testing.T
	m   *machine
	fv  *fakeVendor
	mu  sync.Mutex
	now time.Time
	run string
}

// Tokens of the flow's members (fake values; the vendor answers by them).
const (
	tokAlice  = tokPersonal
	tokCarol2 = tokWork
	tokGrace  = tokCarol
	tokIvan   = tokDave
)

// newFlow builds the machine and the live pool run.
func newFlow(t *testing.T) *flow {
	m, fv := sharedMachine(t, map[string]account{})
	m.f.WriteConfig(flowConfig(""))
	m.env["ALICE_TOKEN"], m.env["CAROL_TOKEN"], m.env["GRACE_TOKEN"], m.env["IVAN_TOKEN"] = tokAlice, tokCarol2, tokGrace, tokIvan
	fl := &flow{t: t, m: m, fv: fv, now: agenttest.FixedNow()}
	m.clock = func() time.Time {
		fl.mu.Lock()
		defer fl.mu.Unlock()
		return fl.now
	}
	fl.run = liveRun(t, m, m.app(t), "auto")
	return fl
}

// advance moves the clock.
func (fl *flow) advance(d time.Duration) {
	fl.mu.Lock()
	fl.now = fl.now.Add(d)
	fl.mu.Unlock()
}

// member is one member's answer: week percent, reset in hours, short.
type member struct {
	tok        string
	short      float64
	week       float64
	resetH     float64
	limited    bool
	unreadable bool
}

// setUsage replaces what the vendor answers for each member, resets
// relative to the clock now.
func (fl *flow) setUsage(ms ...member) {
	now := fl.m.clock()
	for _, m := range ms {
		acc := account{session: m.short, weekly: m.week, weekReset: now.Add(time.Duration(m.resetH * float64(time.Hour))), sessionReset: now.Add(3 * time.Hour), fail: m.unreadable}
		if m.limited {
			acc.session = 100
		}
		fl.fv.set(m.tok, acc)
	}
}

// startUsage is the flow's first usage: grace's room expires soonest.
func (fl *flow) startUsage() {
	fl.setUsage(
		member{tok: tokAlice, week: 30, resetH: 140},
		member{tok: tokCarol2, short: 1, week: 59, resetH: 60},
		member{tok: tokGrace, short: 0, week: 26, resetH: 38},
		member{tok: tokIvan, short: 4, week: 43, resetH: 42},
	)
}

// helperResult is one key request: the token, the exit code, stderr.
type helperResult struct {
	token string
	code  int
	warns []string
	// refused is the reason when no key was handed out under
	// when_none_usable "stop" (token is then the placeholder).
	refused string
}

// helper is one key request as Claude makes it: a new agx process.
func (fl *flow) helper() helperResult {
	a := fl.m.app(fl.t)
	cr, err := a.Credential(context.Background())
	var warns []string
	for _, w := range cr.Warnings {
		warns = append(warns, w.Message)
	}
	if err != nil {
		return helperResult{code: apperr.Exit(err, false), warns: append(warns, err.Error())}
	}
	if cr.Refused != "" {
		// What the CLI prints: the placeholder, exit 0.
		return helperResult{token: NoKeyPlaceholder, warns: warns, refused: cr.Refused}
	}
	return helperResult{token: cr.Token.Reveal(), warns: warns}
}

// state is the run's live.jsonc now.
func (fl *flow) state() live.State {
	st, err := live.Read(run.Dir(fl.m.app(fl.t).paths.Runs, fl.run))
	if err != nil {
		fl.t.Fatal(err)
	}
	return st
}

// probes is how many probes the vendor has answered.
func (fl *flow) probes() int { return fl.fv.count("/v1/messages") }

// lastLog is the log's last line.
func (fl *flow) lastLog() string {
	b, _ := os.ReadFile(filepath.Join(fl.m.f.Home, "helper.log"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

// TestPortFlow is describe("apiKeyHelper flow"), its twelve steps in
// order on one machine.
func TestPortFlow(t *testing.T) {
	fl := newFlow(t)
	fl.startUsage()
	cache := filepath.Join(fl.m.f.ConfigDir, "cache", "login-usage.jsonc")

	t.Run("1. first call: no cache, agx runs once, the soonest-expiring account's key is returned", func(t *testing.T) {
		r := fl.helper()
		if r.code != 0 || r.token != tokGrace || len(r.warns) != 0 {
			t.Fatalf("%+v", r)
		}
		if fl.probes() != 4 {
			t.Errorf("probes %d, want one per member", fl.probes())
		}
		if _, err := os.Stat(cache); err != nil {
			t.Error(err)
		}
	})
	t.Run("2. calls within max-age reuse the cache: agx usage is not run again", func(t *testing.T) {
		for range 5 {
			fl.advance(5 * time.Second)
			if r := fl.helper(); r.token != tokGrace {
				t.Fatalf("%+v", r)
			}
		}
		if fl.probes() != 4 {
			t.Errorf("probes %d", fl.probes())
		}
		if l := fl.lastLog(); !strings.Contains(l, `source="cache (25s old)"`) {
			t.Errorf("log %s", l)
		}
	})
	t.Run("3. usage changes but the cache is fresh: the old pick stays until max-age passes", func(t *testing.T) {
		fl.setUsage(member{tok: tokGrace, short: 55, week: 30, resetH: 38})
		if r := fl.helper(); r.token != tokGrace || fl.probes() != 4 {
			t.Errorf("%+v probes %d", r, fl.probes())
		}
	})
	t.Run("4. after max-age, agx runs again and the helper switches to the next account", func(t *testing.T) {
		fl.advance(61 * time.Second)
		r := fl.helper()
		if r.token != tokIvan || fl.probes() != 8 || !strings.Contains(fl.lastLog(), `picked="ivan"`) {
			t.Errorf("%+v probes %d log %s", r, fl.probes(), fl.lastLog())
		}
	})
	t.Run("5. teammates over cap, own accounts take over (soonest reset first)", func(t *testing.T) {
		fl.setUsage(member{tok: tokIvan, short: 10, week: 70, resetH: 42})
		fl.advance(61 * time.Second)
		if r := fl.helper(); r.token != tokCarol2 {
			t.Errorf("%+v", r)
		}
	})
	t.Run("6. own account hits its limit: next own account", func(t *testing.T) {
		fl.setUsage(member{tok: tokCarol2, week: 60, resetH: 60, limited: true})
		fl.advance(61 * time.Second)
		if r := fl.helper(); r.token != tokAlice {
			t.Errorf("%+v", r)
		}
	})
	t.Run("7. agx fails: the helper falls back to the cache and still returns a key", func(t *testing.T) {
		fl.setUsage(member{tok: tokAlice, unreadable: true}, member{tok: tokCarol2, unreadable: true}, member{tok: tokGrace, unreadable: true}, member{tok: tokIvan, unreadable: true})
		fl.advance(61 * time.Second)
		r := fl.helper()
		if r.code != 0 || r.token != tokAlice || len(r.warns) != 4 || !strings.Contains(r.warns[0], "failed (network") {
			t.Fatalf("%+v", r)
		}
		if l := fl.lastLog(); !strings.Contains(l, `source="cache (`) {
			t.Errorf("log %s", l)
		}
	})
	t.Run("8. disabled account in config is skipped from the next call on", func(t *testing.T) {
		fl.startUsage()
		fl.advance(61 * time.Second)
		fl.m.f.WriteConfig(flowConfig(`, "grace": { "enabled": false }`))
		if r := fl.helper(); r.token != tokIvan {
			t.Errorf("%+v", r)
		}
	})
	t.Run("9. force in config wins on the next call, even within max-age", func(t *testing.T) {
		fl.m.f.WriteConfig(flowForce("alice"))
		r := fl.helper()
		if r.token != tokAlice || !strings.Contains(fl.lastLog(), "forced=true") {
			t.Errorf("%+v log %s", r, fl.lastLog())
		}
		fl.m.f.WriteConfig(flowConfig(""))
	})
	t.Run("10. nothing usable: the placeholder, exit 0 (Claude stops spending)", func(t *testing.T) {
		before := fl.state().KeyAt
		fl.setUsage(
			member{tok: tokAlice, week: 100, resetH: 140, limited: true},
			member{tok: tokCarol2, week: 100, resetH: 60, limited: true},
			member{tok: tokGrace, short: 60, week: 30, resetH: 38},
			member{tok: tokIvan, short: 10, week: 80, resetH: 42},
		)
		fl.advance(61 * time.Second)
		r := fl.helper()
		if r.code != 0 || r.token != NoKeyPlaceholder || !strings.Contains(r.refused, "no member is usable") || !strings.Contains(fl.lastLog(), "picked=null") {
			t.Errorf("%+v log %s", r, fl.lastLog())
		}
		st := fl.state()
		if st.InUse != "" || st.NoKeyBrief != briefAllOverCaps || st.WhenNoneUsable != "stop" || !st.NoKeySince.Equal(fl.now) || !st.KeyAt.Equal(before) || !strings.Contains(st.NoKey, "no member is usable") {
			t.Errorf("live.jsonc %+v", st)
		}
		if got := InUseText(st, fl.now); !strings.HasPrefix(got, "none (all over caps) since ") {
			t.Errorf("in use %q", got)
		}
		// A second refusal keeps the first one's time.
		first := st.NoKeySince
		fl.advance(10 * time.Second)
		if r := fl.helper(); r.token != NoKeyPlaceholder {
			t.Errorf("%+v", r)
		}
		if st := fl.state(); !st.NoKeySince.Equal(first) || !st.InUseAt.Equal(fl.now) {
			t.Errorf("second refusal %+v", st)
		}
	})
	t.Run("10b. nothing usable with when_none_usable keep: exit 1, empty stdout, the agent keeps its key", func(t *testing.T) {
		fl.m.f.WriteConfig(strings.Replace(flowConfig(""), `"usage_max_age"`, `"when_none_usable": "keep", "usage_max_age"`, 1))
		if _, err := live.Update(context.Background(), run.Dir(fl.m.app(t).paths.Runs, fl.run), func(s *live.State) error {
			s.SetInUse("grace", fl.now)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		fl.advance(time.Second)
		r := fl.helper()
		if r.code != apperr.ExitFailure || r.token != "" || r.refused != "" {
			t.Errorf("%+v", r)
		}
		st := fl.state()
		if st.InUse != "grace" || st.WhenNoneUsable != "keep" || st.NoKeyBrief != briefAllOverCaps {
			t.Errorf("live.jsonc %+v", st)
		}
		fl.m.f.WriteConfig(flowConfig(""))
	})
	t.Run("11. several sessions' helpers at once with a stale cache: all succeed, cache stays valid", func(t *testing.T) {
		fl.startUsage()
		fl.advance(61 * time.Second)
		var wg sync.WaitGroup
		results := make([]helperResult, 6)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = fl.helper()
			}()
		}
		wg.Wait()
		for _, r := range results {
			if r.code != 0 || r.token != tokGrace {
				t.Errorf("%+v", r)
			}
		}
		// Recovery: a member is usable again, so the refusal is over.
		if st := fl.state(); st.InUse != "grace" || st.NoKey != "" || !st.NoKeySince.IsZero() || !st.KeyAt.Equal(fl.now) || st.WhenNoneUsable != "" {
			t.Errorf("after recovery %+v", st)
		}
		b, err := os.ReadFile(cache)
		var c login.UsageCache
		if err != nil || json.Unmarshal(b, &c) != nil || len(c.Logins) != 4 {
			t.Fatalf("cache %s %v", b, err)
		}
		entries, _ := os.ReadDir(filepath.Dir(cache))
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp") || strings.HasSuffix(e.Name(), "~") {
				t.Errorf("temp file left: %s", e.Name())
			}
		}
	})
	t.Run("12. the log records every call and never contains a token", func(t *testing.T) {
		b, _ := os.ReadFile(filepath.Join(fl.m.f.Home, "helper.log"))
		log := string(b)
		lines := strings.Split(strings.TrimSpace(log), "\n")
		if len(lines) < 15 {
			t.Errorf("%d lines", len(lines))
		}
		for _, tok := range []string{tokAlice, tokCarol2, tokGrace, tokIvan, "fake-token"} {
			if strings.Contains(log, tok) {
				t.Errorf("log holds a token")
			}
		}
		if !strings.Contains(log, "mode=credential") {
			t.Errorf("log %s", log)
		}
	})
}
