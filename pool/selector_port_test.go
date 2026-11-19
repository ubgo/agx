package pool_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/pool"
)

// The stand-alone selector's test/selector.test.mjs, ported one to one:
// every subtest name is the JavaScript test's name (the snapshot tests
// with the anonymised names), so testdata/SELECTOR_PORT.md can map each
// old test to its Go test. Config validation cases live in
// config/pool_port_test.go, where agx validates a pool.

// portNow is the JavaScript tests' NOW.
var portNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// at is portNow plus h hours.
func at(h float64) *time.Time {
	t := portNow.Add(time.Duration(h * float64(time.Hour)))
	return &t
}

// rd is the JavaScript login(): status (default ok), an optional short
// window (resets in 3 h) and an optional week window with an optional
// reset in resetH hours.
type rd struct {
	status      string
	short, week *float64
	resetH      *float64
}

// p is a pointer to a float.
func p(v float64) *float64 { return &v }

// reading builds the pool reading.
func (r rd) reading() pool.Reading {
	out := pool.Reading{Status: r.status}
	if out.Status == "" {
		out.Status = pool.StatusOK
	}
	if r.short != nil {
		out.Short = &pool.Window{Percent: *r.short, ResetsAt: at(3)}
	}
	if r.week != nil {
		w := &pool.Window{Percent: *r.week}
		if r.resetH != nil {
			w.ResetsAt = at(*r.resetH)
		}
		out.Week = w
	}
	return out
}

// usage is the JavaScript report(...): readings by name.
func usage(logins map[string]rd) map[string]pool.Reading {
	out := map[string]pool.Reading{}
	for n, l := range logins {
		out[n] = l.reading()
	}
	return out
}

// portConfig is the JavaScript config(): own o1, o2 (caps 99/100), shared
// s1, s2 (default 50/70), window 48 h, cutoff 99.
func portConfig(edit ...func(*pool.Config)) pool.Config {
	c := pool.New([]string{"o1", "o2"}, []string{"s1", "s2"})
	c.Accounts = map[string]pool.Account{"o1": {Short: p(99), Week: p(100)}, "o2": {Short: p(99), Week: p(100)}}
	for _, e := range edit {
		e(&c)
	}
	return c
}

// pickPort is the JavaScript pick(): Select at portNow.
func pickPort(t *testing.T, readings map[string]pool.Reading, c pool.Config, force string) pool.Result {
	t.Helper()
	r, err := pool.Select(c, readings, pool.Options{Now: portNow, Force: force})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// cand is the JavaScript cand(): one candidate by name.
func cand(t *testing.T, r pool.Result, name string) pool.Candidate {
	t.Helper()
	for _, c := range r.Candidates {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no candidate %s", name)
	return pool.Candidate{}
}

// selected is the selected name, "" for none.
func selected(r pool.Result) string {
	if r.Selected == nil {
		return ""
	}
	return r.Selected.Name
}

// names lists candidates' names, optionally only one state.
func names(r pool.Result, state pool.State) []string {
	var out []string
	for _, c := range r.Candidates {
		if state == "" || c.State == state {
			out = append(out, c.Name)
		}
	}
	return out
}

// TestPortRanking is describe("ranking by soonest weekly reset").
func TestPortRanking(t *testing.T) {
	t.Run("own account with sooner reset wins over config order", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(100)}, "o2": {week: p(10), resetH: p(20)}}), portConfig(), "")
		if selected(r) != "o2" || cand(t, r, "o2").Rank != 1 || cand(t, r, "o1").Rank != 2 {
			t.Errorf("selected %s ranks o2 %d o1 %d", selected(r), cand(t, r, "o2").Rank, cand(t, r, "o1").Rank)
		}
	})
	t.Run("shared within borrow window goes ahead of own with later reset", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(100)}, "s1": {short: p(0), week: p(10), resetH: p(24)}}), portConfig(), "")
		if selected(r) != "s1" || !strings.Contains(r.Selected.Reason, "within 48h") {
			t.Errorf("selected %s %q", selected(r), r.Selected.Reason)
		}
	})
	t.Run("shared outside borrow window waits while an own account is usable", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(200)}, "s1": {short: p(0), week: p(10), resetH: p(60)}}), portConfig(), "")
		if selected(r) != "o1" || cand(t, r, "s1").State != pool.StateWaiting || cand(t, r, "s1").Rank != 0 {
			t.Errorf("selected %s s1 %+v", selected(r), cand(t, r, "s1"))
		}
	})
	t.Run("own with sooner reset beats shared inside the window", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(10)}, "s1": {short: p(0), week: p(10), resetH: p(30)}}), portConfig(), "")
		if selected(r) != "o1" {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("borrow window boundary: exactly 48h is inside, just over is outside", func(t *testing.T) {
		own := rd{week: p(10), resetH: p(200)}
		if s := selected(pickPort(t, usage(map[string]rd{"o1": own, "s1": {week: p(10), resetH: p(48)}}), portConfig(), "")); s != "s1" {
			t.Errorf("48h: %s", s)
		}
		if s := selected(pickPort(t, usage(map[string]rd{"o1": own, "s1": {week: p(10), resetH: p(48.01)}}), portConfig(), "")); s != "o1" {
			t.Errorf("48.01h: %s", s)
		}
	})
	t.Run("borrow window 0 means shared never goes before a usable own account", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.BorrowWindow = 0 })
		if s := selected(pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(200)}, "s1": {week: p(10), resetH: p(1)}}), c, "")); s != "o1" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("no own account usable: soonest shared wins regardless of window", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{
			"o1": {status: "limit-reached", week: p(100), resetH: p(5)},
			"s1": {week: p(10), resetH: p(150)},
			"s2": {week: p(10), resetH: p(90)},
		}), portConfig(), "")
		if selected(r) != "s2" || !strings.Contains(r.Selected.Reason, "no own account is usable") || cand(t, r, "s1").State != pool.StateReady {
			t.Errorf("selected %s %q s1 %s", selected(r), r.Selected.Reason, cand(t, r, "s1").State)
		}
	})
	t.Run("tie on reset: own before shared, then config order", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{
			"s1": {week: p(10), resetH: p(24)}, "o2": {week: p(10), resetH: p(24)},
			"o1": {week: p(10), resetH: p(24)}, "s2": {week: p(10), resetH: p(24)},
		}), portConfig(), "")
		if got := names(r, ""); !slices.Equal(got, []string{"o1", "o2", "s1", "s2"}) {
			t.Errorf("order %v", got)
		}
	})
	t.Run("unknown reset sorts after known resets", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10)}, "o2": {week: p(10), resetH: p(150)}}), portConfig(), "")
		if selected(r) != "o2" {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("passed reset is treated as stale: sorts last and is noted", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(-2)}, "o2": {week: p(10), resetH: p(150)}}), portConfig(), "")
		if selected(r) != "o2" || !strings.Contains(strings.Join(cand(t, r, "o1").Notes, ","), "stale") {
			t.Errorf("selected %s notes %v", selected(r), cand(t, r, "o1").Notes)
		}
	})
	t.Run("stale shared reset is not inside the borrow window", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(150)}, "s1": {week: p(10), resetH: p(-1)}}), portConfig(), "")
		if selected(r) != "o1" || cand(t, r, "s1").State != pool.StateWaiting {
			t.Errorf("selected %s s1 %s", selected(r), cand(t, r, "s1").State)
		}
	})
}

// TestPortCaps is describe("caps always apply").
func TestPortCaps(t *testing.T) {
	t.Run("shared 5h at 50% is blocked, 49.9% is usable", func(t *testing.T) {
		own := rd{status: "limit-reached", week: p(100), resetH: p(5)}
		blocked := pickPort(t, usage(map[string]rd{"o1": own, "s1": {short: p(50), week: p(10), resetH: p(24)}}), portConfig(), "")
		if blocked.Selected != nil || !slices.Contains(cand(t, blocked, "s1").Reasons, "5h is 50% (cap 50%)") {
			t.Errorf("blocked %+v", cand(t, blocked, "s1"))
		}
		if s := selected(pickPort(t, usage(map[string]rd{"o1": own, "s1": {short: p(49.9), week: p(10), resetH: p(24)}}), portConfig(), "")); s != "s1" {
			t.Errorf("49.9: %s", s)
		}
	})
	t.Run("shared week at 70% is blocked even if it resets in 1 hour", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(150)}, "s1": {short: p(0), week: p(70), resetH: p(1)}}), portConfig(), "")
		if selected(r) != "o1" || !slices.Contains(cand(t, r, "s1").Reasons, "week is 70% (cap 70%)") {
			t.Errorf("selected %s s1 %v", selected(r), cand(t, r, "s1").Reasons)
		}
	})
	t.Run("shared 5h over cap is blocked even if week resets today", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(150)}, "s1": {short: p(55), week: p(10), resetH: p(2)}}), portConfig(), "")
		if selected(r) != "o1" {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("shared cap counts total usage: teammate at 60% leaves 10% room", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"s1": {week: p(60), resetH: p(24)}}), portConfig(func(c *pool.Config) { c.Own = nil; c.Accounts = nil }), "")
		if selected(r) != "s1" || r.Selected.Room == nil || *r.Selected.Room != 10 {
			t.Errorf("selected %s room %v", selected(r), r.Selected.Room)
		}
	})
	t.Run("own 5h at 99% hits the safety cutoff", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {short: p(99), week: p(10), resetH: p(10)}, "o2": {short: p(0), week: p(10), resetH: p(100)}}), portConfig(), "")
		if selected(r) != "o2" || !strings.Contains(strings.Join(cand(t, r, "o1").Reasons, ","), "unavailable at 99%") {
			t.Errorf("selected %s o1 %v", selected(r), cand(t, r, "o1").Reasons)
		}
	})
	t.Run("own week at 100% is blocked even with status ok", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(100), resetH: p(10)}, "o2": {week: p(10), resetH: p(100)}}), portConfig(), "")
		if selected(r) != "o2" {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("safety cutoff applies even when an account cap is higher", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Accounts = map[string]pool.Account{"o1": {Short: p(100), Week: p(100)}} })
		if r := pickPort(t, usage(map[string]rd{"o1": {short: p(99.5), week: p(10), resetH: p(10)}}), c, ""); r.Selected != nil {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("a window agx does not report is not capped", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(10)}}), portConfig(), "")
		if selected(r) != "o1" || cand(t, r, "o1").Short != nil {
			t.Errorf("selected %s short %v", selected(r), cand(t, r, "o1").Short)
		}
	})
	t.Run("defaults apply to accounts without an entry, and can be changed", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Own, c.Accounts, c.Defaults = nil, nil, pool.Caps{Short: 40, Week: 70} })
		if r := pickPort(t, usage(map[string]rd{"s1": {short: p(41), week: p(10), resetH: p(10)}}), c, ""); r.Selected != nil {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("per-account entry overrides defaults", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Own, c.Accounts = nil, map[string]pool.Account{"s1": {Short: p(80)}} })
		if s := selected(pickPort(t, usage(map[string]rd{"s1": {short: p(60), week: p(10), resetH: p(10)}}), c, "")); s != "s1" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("all reasons are listed, not only the first", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Own, c.Accounts = nil, nil })
		r := pickPort(t, usage(map[string]rd{"s1": {status: "error", short: p(60), week: p(80), resetH: p(10)}}), c, "")
		if n := len(cand(t, r, "s1").Reasons); n != 3 {
			t.Errorf("reasons %v", cand(t, r, "s1").Reasons)
		}
	})
}

// TestPortSkipping is describe("skipping accounts").
func TestPortSkipping(t *testing.T) {
	t.Run("non-ok status is skipped", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o1": {status: "limit-reached", week: p(10), resetH: p(1)}, "o2": {week: p(10), resetH: p(100)}}), portConfig(), "")
		if selected(r) != "o2" {
			t.Errorf("selected %s", selected(r))
		}
	})
	t.Run("missing account is skipped", func(t *testing.T) {
		r := pickPort(t, usage(map[string]rd{"o2": {week: p(10), resetH: p(100)}}), portConfig(), "")
		if selected(r) != "o2" || !slices.Equal(cand(t, r, "o1").Reasons, []string{"no usage reading"}) {
			t.Errorf("selected %s o1 %v", selected(r), cand(t, r, "o1").Reasons)
		}
	})
	t.Run("enabled: false skips the account", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) {
			c.Accounts = map[string]pool.Account{"o1": {Short: p(99), Week: p(100), Enabled: f2(false)}, "o2": {Short: p(99), Week: p(100)}}
		})
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(1)}, "o2": {week: p(10), resetH: p(100)}}), c, "")
		if selected(r) != "o2" || !slices.Equal(cand(t, r, "o1").Reasons, []string{"disabled in config"}) || cand(t, r, "o1").Enabled {
			t.Errorf("selected %s o1 %+v", selected(r), cand(t, r, "o1"))
		}
	})
	t.Run("disabled account missing from agx still says disabled", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Accounts = map[string]pool.Account{"o1": {Enabled: f2(false)}} })
		r := pickPort(t, usage(map[string]rd{"o2": {week: p(10), resetH: p(100)}}), c, "")
		if got := cand(t, r, "o1").Reasons; !slices.Equal(got, []string{"disabled in config", "no usage reading"}) {
			t.Errorf("reasons %v", got)
		}
	})
	t.Run("disabled own accounts do not hold shared accounts back", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) {
			c.Own, c.Accounts = []string{"o1"}, map[string]pool.Account{"o1": {Enabled: f2(false)}}
		})
		if s := selected(pickPort(t, usage(map[string]rd{"o1": {week: p(10), resetH: p(1)}, "s1": {week: p(10), resetH: p(150)}}), c, "")); s != "s1" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("enabled: true behaves like no entry", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Own, c.Accounts = nil, map[string]pool.Account{"s1": {Enabled: f2(true)}} })
		r := pickPort(t, usage(map[string]rd{"s1": {short: p(10), week: p(69), resetH: p(10)}}), c, "")
		if selected(r) != "s1" || r.Candidates[0].Caps != (pool.Caps{Short: 50, Week: 70}) {
			t.Errorf("selected %s caps %+v", selected(r), r.Candidates[0].Caps)
		}
	})
	t.Run("nothing usable returns selected null", func(t *testing.T) {
		if r := pickPort(t, usage(map[string]rd{"o1": {status: "limit-reached", week: p(100), resetH: p(5)}}), portConfig(), ""); r.Selected != nil {
			t.Errorf("selected %s", selected(r))
		}
	})
}

// f2 is a pointer to a bool.
func f2(b bool) *bool { return &b }

// TestPortForce is describe("force").
func TestPortForce(t *testing.T) {
	data := usage(map[string]rd{
		"o1": {status: "limit-reached", week: p(100), resetH: p(5)},
		"o2": {week: p(10), resetH: p(1)},
		"s1": {short: p(70), week: p(90), resetH: p(3)},
		"x9": {week: p(10), resetH: p(50)},
	})
	t.Run("config force ignores every rule and reports why it would be skipped", func(t *testing.T) {
		r := pickPort(t, data, portConfig(func(c *pool.Config) { c.Force = "o1" }), "")
		s := r.Selected
		if s.Name != "o1" || !s.Forced || s.ForcedBy != pool.ForcedByConfig || !slices.Equal(s.Warnings, []string{"status is limit-reached", "week is 100% (cap 100%)"}) {
			t.Errorf("selected %+v", s)
		}
	})
	t.Run("--force option overrides config force", func(t *testing.T) {
		r := pickPort(t, data, portConfig(func(c *pool.Config) { c.Force = "o1" }), "s1")
		if r.Selected.Name != "s1" || r.Selected.ForcedBy != pool.ForcedByFlag {
			t.Errorf("selected %+v", r.Selected)
		}
	})
	t.Run("force overrides enabled: false", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Accounts["o2"] = pool.Account{Enabled: f2(false)} })
		r := pickPort(t, data, c, "o2")
		if r.Selected.Name != "o2" || !slices.Equal(r.Selected.Warnings, []string{"disabled in config"}) {
			t.Errorf("selected %+v", r.Selected)
		}
	})
	t.Run("force may pick an account not listed in own/shared, with a warning", func(t *testing.T) {
		r := pickPort(t, data, portConfig(), "x9")
		if r.Selected.Name != "x9" || r.Selected.Group != pool.GroupUnlisted || !slices.Equal(r.Selected.Warnings, []string{"not listed in own or shared"}) {
			t.Errorf("selected %+v", r.Selected)
		}
	})
	t.Run("force of a name not in agx data is an error", func(t *testing.T) {
		if _, err := pool.Select(portConfig(func(c *pool.Config) { c.Force = "ghost" }), data, pool.Options{Now: portNow}); !errors.Is(err, pool.ErrForcedMissing) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("force: null means normal selection", func(t *testing.T) {
		if s := selected(pickPort(t, data, portConfig(), "")); s != "o2" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("candidates are still ranked when forced", func(t *testing.T) {
		if r := pickPort(t, data, portConfig(), "o1"); cand(t, r, "o2").Rank != 1 {
			t.Errorf("o2 rank %d", cand(t, r, "o2").Rank)
		}
	})
}

// TestPortStability is describe("stability").
func TestPortStability(t *testing.T) {
	t.Run("same input twice gives the same result and inputs are not mutated", func(t *testing.T) {
		data := usage(map[string]rd{"o1": {week: p(10), resetH: p(100)}, "s1": {week: p(10), resetH: p(24)}})
		c := portConfig()
		before, _ := json.Marshal([]any{data, c.Own, c.Shared, c.Accounts, c.Defaults})
		a, b := pickPort(t, data, c, ""), pickPort(t, data, c, "")
		if !reflect.DeepEqual(a, b) {
			t.Errorf("results differ:\n%+v\n%+v", a, b)
		}
		after, _ := json.Marshal([]any{data, c.Own, c.Shared, c.Accounts, c.Defaults})
		if string(before) != string(after) {
			t.Errorf("inputs changed")
		}
	})
}

// snapshotFile is the anonymised real snapshot (2026-11-18 23:40 IST):
// the selector's fixture with the same numbers, statuses and resets,
// every name replaced by a fake one and every email @example.com.
const snapshotFile = "testdata/snapshot-2026-11-18-anon.json"

// snapshotNow is the snapshot's clock.
var snapshotNow = time.Date(2026, 11, 18, 18, 10, 0, 0, time.UTC)

// Snapshot loads the anonymised agx usage JSON into readings (records of
// kind login and agent claude only, as the selector filtered them).
func Snapshot(t *testing.T) map[string]pool.Reading {
	t.Helper()
	b, err := os.ReadFile(snapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OK   bool `json:"ok"`
		Data []struct {
			Kind, Name, Agent, Status string
			Windows                   []struct {
				Class    string     `json:"class"`
				Percent  float64    `json:"percent"`
				ResetsAt *time.Time `json:"resets_at"`
			}
		}
	}
	if err := json.Unmarshal(b, &doc); err != nil || !doc.OK {
		t.Fatalf("snapshot: %v", err)
	}
	out := map[string]pool.Reading{}
	for _, d := range doc.Data {
		if d.Kind != "login" || d.Agent != "claude" {
			continue
		}
		r := pool.Reading{Status: d.Status}
		for _, w := range d.Windows {
			win := &pool.Window{Percent: w.Percent, ResetsAt: w.ResetsAt}
			switch w.Class {
			case "short":
				r.Short = win
			case "week":
				r.Week = win
			}
		}
		out[d.Name] = r
	}
	return out
}

// snapshotConfig is the real config with fake names: own alice, bob,
// carol (caps 99/100); shared derek, erin, frank, grace, heidi, ivan,
// judy, mallory, niaj.
func snapshotConfig() pool.Config {
	c := pool.New([]string{"alice", "bob", "carol"}, []string{"derek", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "niaj"})
	c.Accounts = map[string]pool.Account{"alice": {Short: p(99), Week: p(100)}, "bob": {Short: p(99), Week: p(100)}, "carol": {Short: p(99), Week: p(100)}}
	return c
}

// TestPortSnapshot is describe("real snapshot (2026-11-18 23:40 IST)"),
// anonymised.
func TestPortSnapshot(t *testing.T) {
	snap := Snapshot(t)
	t.Run("picks grace, then ivan, carol, alice", func(t *testing.T) {
		r := pickPort2(t, snap, snapshotConfig())
		if selected(r) != "grace" || !slices.Equal(names(r, pool.StateReady), []string{"grace", "ivan", "carol", "alice"}) {
			t.Errorf("selected %s ready %v", selected(r), names(r, pool.StateReady))
		}
		if cand(t, r, "bob").State != pool.StateBlocked || cand(t, r, "niaj").State != pool.StateBlocked || slices.Contains(names(r, ""), "oscar") {
			t.Errorf("states %v", r.Candidates)
		}
	})
	t.Run("with no borrow window, own accounts come first by reset", func(t *testing.T) {
		c := snapshotConfig()
		c.BorrowWindow = 0
		if got := names(pickPort2(t, snap, c), pool.StateReady); !slices.Equal(got, []string{"carol", "alice"}) {
			t.Errorf("ready %v", got)
		}
	})
	t.Run("after grace and ivan are disabled, carol is next", func(t *testing.T) {
		c := snapshotConfig()
		c.Accounts["grace"] = pool.Account{Enabled: f2(false)}
		c.Accounts["ivan"] = pool.Account{Enabled: f2(false)}
		if s := selected(pickPort2(t, snap, c)); s != "carol" {
			t.Errorf("selected %s", s)
		}
	})
}

// pickPort2 is Select at the snapshot's clock.
func pickPort2(t *testing.T, readings map[string]pool.Reading, c pool.Config) pool.Result {
	t.Helper()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	r, err := pool.Select(c, readings, pool.Options{Now: snapshotNow})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestPortOddData is describe("no silent surprises: odd agx data").
func TestPortOddData(t *testing.T) {
	t.Run("cap 0 always blocks; cap 100 blocks only at 100", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) {
			c.Accounts = map[string]pool.Account{"o1": {Week: p(0)}, "o2": {Short: p(99), Week: p(100)}}
		})
		r := pickPort(t, usage(map[string]rd{"o1": {week: p(0), resetH: p(1)}, "o2": {week: p(99.9), resetH: p(2)}}), c, "")
		if selected(r) != "o2" {
			t.Errorf("selected %s", selected(r))
		}
		if r := pickPort(t, usage(map[string]rd{"o2": {week: p(100), resetH: p(2)}}), c, ""); r.Selected != nil {
			t.Errorf("100 picked")
		}
	})
	t.Run("usage that isn't a number blocks the account instead of skipping the cap", func(t *testing.T) {
		c := portConfig(func(c *pool.Config) { c.Own, c.Accounts = nil, nil })
		for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			r := pickPort(t, map[string]pool.Reading{"s1": {Status: pool.StatusOK, Week: &pool.Window{Percent: v, ResetsAt: at(5)}}}, c, "")
			s1 := cand(t, r, "s1")
			if r.Selected != nil || !slices.Contains(s1.Reasons, "week usage is unreadable") || s1.Week != nil || s1.Room != nil {
				t.Errorf("%v: selected %s %+v", v, selected(r), s1)
			}
		}
		r := pickPort(t, map[string]pool.Reading{"s1": {Status: pool.StatusOK, Short: &pool.Window{Percent: math.NaN()}, Week: &pool.Window{Percent: 1, ResetsAt: at(5)}}}, c, "")
		if !slices.Contains(cand(t, r, "s1").Reasons, "5h usage is unreadable") || cand(t, r, "s1").Short != nil {
			t.Errorf("short %+v", cand(t, r, "s1"))
		}
	})
	t.Run("windows missing or not a list: account usable, reset unknown, sorted last", func(t *testing.T) {
		r := pickPort(t, map[string]pool.Reading{
			"o1": {Status: pool.StatusOK}, "o2": {Status: pool.StatusOK},
			"s1": rd{week: p(1), resetH: p(200)}.reading(),
		}, portConfig(), "")
		if got := names(r, pool.StateReady); !slices.Equal(got, []string{"o1", "o2"}) || cand(t, r, "o1").UntilReset != nil {
			t.Errorf("ready %v", got)
		}
	})
	t.Run("unreadable reset time: noted, treated as unknown, sorted last", func(t *testing.T) {
		zero := time.Time{}
		r := pickPort(t, map[string]pool.Reading{
			"o1": {Status: pool.StatusOK, Week: &pool.Window{Percent: 10, ResetsAt: &zero}},
			"o2": rd{week: p(10), resetH: p(150)}.reading(),
		}, portConfig(), "")
		if selected(r) != "o2" || !strings.Contains(strings.Join(cand(t, r, "o1").Notes, ","), "unreadable") {
			t.Errorf("selected %s notes %v", selected(r), cand(t, r, "o1").Notes)
		}
	})
	t.Run("both resets unknown: config order", func(t *testing.T) {
		if s := selected(pickPort(t, usage(map[string]rd{"o2": {week: p(10)}, "o1": {week: p(10)}}), portConfig(), "")); s != "o1" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("duplicate login record: first one used, with a warning", func(t *testing.T) {
		// Not a case agx can meet: readings are a map keyed by login
		// name, and the cache and config are JSON objects with one entry
		// per name (SELECTOR_PORT.md). What it guards, one reading per
		// name decides, holds by type.
		data := usage(map[string]rd{"o1": {week: p(10), resetH: p(1)}, "o2": {week: p(10), resetH: p(100)}})
		if s := selected(pickPort(t, data, portConfig(), "")); s != "o1" {
			t.Errorf("selected %s", s)
		}
	})
	t.Run("record order from agx never changes the pick", func(t *testing.T) {
		data := usage(map[string]rd{
			"o1": {week: p(10), resetH: p(100)}, "o2": {week: p(50), resetH: p(30)},
			"s1": {short: p(5), week: p(10), resetH: p(20)}, "s2": {short: p(5), week: p(10), resetH: p(20)},
		})
		base := pickPort(t, data, portConfig(), "")
		// Go randomises map iteration on every range: many runs cover
		// many orders.
		for range 200 {
			r := pickPort(t, data, portConfig(), "")
			if selected(r) != selected(base) || !slices.Equal(names(r, ""), names(base, "")) {
				t.Fatalf("order changed: %v vs %v", names(r, ""), names(base, ""))
			}
		}
	})
	t.Run("forced name that exists only for another agent is an error", func(t *testing.T) {
		// agx reads only the pool's agent; a login of another agent has no
		// reading here, so forcing it is ErrForcedMissing (and config
		// refuses such a force at load, config TestPoolProblems).
		if _, err := pool.Select(portConfig(func(c *pool.Config) { c.Force = "x" }), usage(map[string]rd{"o1": {week: p(1), resetH: p(1)}}), pool.Options{Now: portNow}); !errors.Is(err, pool.ErrForcedMissing) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("bad now is an error, not a silent wrong order", func(t *testing.T) {
		if _, err := pool.Select(portConfig(), usage(map[string]rd{"o1": {week: p(1), resetH: p(1)}}), pool.Options{}); !errors.Is(err, pool.ErrBadNow) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("other statuses all block", func(t *testing.T) {
		for _, st := range []string{"limit-reached", "error", "unknown", " ", "missing"} {
			r := pool.Reading{Status: st, Week: &pool.Window{Percent: 1, ResetsAt: at(1)}}
			if res := pickPort(t, map[string]pool.Reading{"o1": r}, portConfig(), ""); res.Selected != nil {
				t.Errorf("status %q picked", st)
			}
		}
		// An empty status is not "ok" either.
		empty := pool.Reading{Status: "", Week: &pool.Window{Percent: 1, ResetsAt: at(1)}}
		if res, _ := pool.Select(portConfig(), map[string]pool.Reading{"o1": empty}, pool.Options{Now: portNow}); res.Selected != nil {
			t.Errorf("empty status picked")
		}
	})
}

// TestPortDefaults is describe("built-in defaults when a setting is left
// out"): New gives the documented defaults.
func TestPortDefaults(t *testing.T) {
	minimal := func(own []string) pool.Config {
		c := pool.New(own, []string{"s1"})
		c.Accounts = map[string]pool.Account{"o1": {Short: p(100), Week: p(100)}}
		return c
	}
	t.Run("5h safety cutoff defaults to 99%", func(t *testing.T) {
		if s := selected(pickPort(t, usage(map[string]rd{"o1": {short: p(98.9), week: p(1), resetH: p(1)}}), minimal([]string{"o1"}), "")); s != "o1" {
			t.Errorf("98.9: %s", s)
		}
		r := pickPort(t, usage(map[string]rd{"o1": {short: p(99), week: p(1), resetH: p(1)}}), minimal([]string{"o1"}), "")
		if r.Selected != nil || r.ShortUnavailableAt != 99 {
			t.Errorf("99: %s %v", selected(r), r.ShortUnavailableAt)
		}
	})
	t.Run("borrow window defaults to 48h", func(t *testing.T) {
		own := rd{week: p(1), resetH: p(200)}
		if s := selected(pickPort(t, usage(map[string]rd{"o1": own, "s1": {short: p(0), week: p(1), resetH: p(48)}}), minimal([]string{"o1"}), "")); s != "s1" {
			t.Errorf("48h: %s", s)
		}
		r := pickPort(t, usage(map[string]rd{"o1": own, "s1": {short: p(0), week: p(1), resetH: p(48.5)}}), minimal([]string{"o1"}), "")
		if selected(r) != "o1" || r.BorrowWindow != 48*time.Hour {
			t.Errorf("48.5h: %s %s", selected(r), r.BorrowWindow)
		}
	})
	t.Run("caps default to 5h 50% and week 70%", func(t *testing.T) {
		c := minimal(nil)
		if s := selected(pickPort(t, usage(map[string]rd{"s1": {short: p(49.9), week: p(69.9), resetH: p(1)}}), c, "")); s != "s1" {
			t.Errorf("under: %s", s)
		}
		if r := pickPort(t, usage(map[string]rd{"s1": {short: p(50), week: p(1), resetH: p(1)}}), c, ""); r.Selected != nil {
			t.Error("short 50 picked")
		}
		if r := pickPort(t, usage(map[string]rd{"s1": {short: p(1), week: p(70), resetH: p(1)}}), c, ""); r.Selected != nil {
			t.Error("week 70 picked")
		}
	})
}
