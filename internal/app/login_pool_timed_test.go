package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/apperr"
)

// timedNow is Saturday 21 November 2026, 13:00 in Istanbul.
var timedNow = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

// timedConfig is a pool in Istanbul time: own alice, shared bob and
// carol, a weekend schedule, an active override for bob and an ended one
// for carol.
const timedConfig = `{
  // timed caps fixture
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice"], "shared": ["bob", "carol"],
      "timezone": "Europe/Istanbul",
      "schedules": [ { "name": "weekend", "days": ["sat", "sun"], "members": ["shared"], "caps": { "short": 95 } } ],
      "accounts": {
        "bob": { "week": 60, "override": { "short": 90, "until": "2026-11-21T20:00:00+03:00" } },
        "carol": { "override": { "week": 80, "until": "2026-11-20T18:00:00+03:00" } }
      }
    } }
  }
}`

// timedApp is an App on src with the clock at now.
func timedApp(t *testing.T, src string, now time.Time) (*App, string) {
	t.Helper()
	h := agenttest.NewHost(t, agenttest.WithNow(func() time.Time { return now }))
	agenttest.WriteFile(t, h.UserHome, ".agx/config.jsonc", src)
	lookup := func(name string) (string, bool) {
		if name == "AGX_CONFIG_DIR" {
			return filepath.Join(h.UserHome, ".agx"), true
		}
		return "", false
	}
	return New(Deps{Host: h, Lookup: lookup, Cwd: h.UserHome}), h.UserHome
}

// TestPoolUntilFlags: --until turns the cap flags into an override that
// ends, in the pool's zone; an active override keeps its other caps, an
// ended one starts over; NAME= drops a cap and an empty override goes.
func TestPoolUntilFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		s       PoolSettings
		changes []Change
		want    []string
	}{
		{"new override, day and time", PoolSettings{CapShort: []string{"alice=99"}, Timed: PoolTimedSettings{Until: strp("sun 23:59")}},
			[]Change{{"pool.accounts.alice.override", "", "short 99 until 2026-11-22T23:59:00+03:00"}},
			[]string{`"alice": {"override": {"short":99, "until":"2026-11-22T23:59:00+03:00"}}`}},
		{"duration", PoolSettings{CapWeek: []string{"alice=99"}, Timed: PoolTimedSettings{Until: strp("6h")}},
			[]Change{{"pool.accounts.alice.override", "", "week 99 until 2026-11-21T19:00:00+03:00"}}, nil},
		{"date and time", PoolSettings{CapShort: []string{"carol=95"}, Timed: PoolTimedSettings{Until: strp("2026-11-23T08:00")}},
			[]Change{{"pool.accounts.carol.override", "week 80 until 2026-11-20T18:00:00+03:00", "short 95 until 2026-11-23T08:00:00+03:00"}}, nil},
		{"active override keeps its caps, takes the new end", PoolSettings{CapWeek: []string{"bob=85"}, Timed: PoolTimedSettings{Until: strp("sun 12:00")}},
			[]Change{{"pool.accounts.bob.override", "short 90 until 2026-11-21T20:00:00+03:00", "short 90, week 85 until 2026-11-22T12:00:00+03:00"}},
			[]string{`"week": 60`}},
		{"NAME= drops one cap", PoolSettings{CapShort: []string{"bob="}, CapWeek: []string{"bob=85"}, Timed: PoolTimedSettings{Until: strp("1h")}},
			[]Change{{"pool.accounts.bob.override", "short 90 until 2026-11-21T20:00:00+03:00", "week 85 until 2026-11-21T14:00:00+03:00"}}, nil},
		{"empty override removed", PoolSettings{CapShort: []string{"bob="}, Timed: PoolTimedSettings{Until: strp("1h")}},
			[]Change{{"pool.accounts.bob.override", "short 90 until 2026-11-21T20:00:00+03:00", ""}}, nil},
		{"clear", PoolSettings{Timed: PoolTimedSettings{ClearOverride: []string{"bob", "carol"}}},
			[]Change{{"pool.accounts.bob.override", "short 90 until 2026-11-21T20:00:00+03:00", ""}, {"pool.accounts.carol.override", "week 80 until 2026-11-20T18:00:00+03:00", ""}},
			[]string{`"bob": { "week": 60 }`}},
		{"until in a new timezone", PoolSettings{CapShort: []string{"alice=99"}, Timed: PoolTimedSettings{Until: strp("2026-11-23T08:00"), Timezone: strp("UTC")}},
			[]Change{{"pool.accounts.alice.override", "", "short 99 until 2026-11-23T08:00:00Z"}, {"pool.timezone", "Europe/Istanbul", "UTC"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := timedApp(t, timedConfig, timedNow)
			ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: tc.s})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ch.Changes, tc.changes) {
				t.Errorf("changes %+v\nwant    %+v", ch.Changes, tc.changes)
			}
			got := readConfig(t, home)
			for _, w := range append(tc.want, "// timed caps fixture") {
				if !strings.Contains(compact(got), compact(w)) {
					t.Errorf("file lacks %q:\n%s", w, got)
				}
			}
			poolStillValid(t, a, "auto")
		})
	}
}

// TestPoolScheduleFlags: --pool-schedule adds or replaces by name,
// --pool-schedule-remove removes, the list goes when empty.
func TestPoolScheduleFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    PoolTimedSettings
		want []string
		gone []string
	}{
		{"add", PoolTimedSettings{Schedules: []string{"nights days=Mon,tue hours=22:00-06:00 members=bob,own timezone=UTC week=80 week_relax_max=90 release_before=1h"}},
			[]string{`"name":"weekend"`, `{"name":"nights","days":["mon","tue"],"hours":"22:00-06:00","members":["bob","own"],"timezone":"UTC","caps":{"week":80,"week_relax_max":90},"release_before":"1h"}`}, nil},
		{"replace", PoolTimedSettings{Schedules: []string{"weekend days=sun members=carol short=80"}},
			[]string{`{"name":"weekend","days":["sun"],"members":["carol"],"caps":{"short":80}}`}, []string{`"sat"`}},
		{"remove", PoolTimedSettings{ScheduleRemove: []string{"weekend"}}, nil, []string{`"schedules"`}},
		{"timezone removed", PoolTimedSettings{Timezone: strp("")}, nil, []string{`"timezone"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := timedApp(t, timedConfig, timedNow)
			if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Timed: tc.s}}); err != nil {
				t.Fatal(err)
			}
			got := readConfig(t, home)
			for _, w := range tc.want {
				if !strings.Contains(compact(got), compact(w)) {
					t.Errorf("file lacks %q:\n%s", w, got)
				}
			}
			for _, g := range tc.gone {
				if strings.Contains(got, g) {
					t.Errorf("file still has %q:\n%s", g, got)
				}
			}
			poolStillValid(t, a, "auto")
		})
	}
}

// TestPoolTimedRefusals: each bad timed-cap flag is a usage error and
// writes nothing.
func TestPoolTimedRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    PoolSettings
		want string
	}{
		{"until alone", PoolSettings{Timed: PoolTimedSettings{Until: strp("6h")}}, "give one"},
		{"until with only capacity", PoolSettings{Rules: PoolRuleSettings{Capacity: []string{"bob=2"}}, Timed: PoolTimedSettings{Until: strp("6h")}}, "give one"},
		{"until past", PoolSettings{CapShort: []string{"bob=95"}, Timed: PoolTimedSettings{Until: strp("2026-11-01T08:00")}}, "not in the future"},
		{"until junk", PoolSettings{CapShort: []string{"bob=95"}, Timed: PoolTimedSettings{Until: strp("next week")}}, "use a duration"},
		{"until zero", PoolSettings{CapShort: []string{"bob=95"}, Timed: PoolTimedSettings{Until: strp("0s")}}, "not a positive duration"},
		{"clear and set", PoolSettings{CapShort: []string{"bob=95"}, Timed: PoolTimedSettings{Until: strp("1h"), ClearOverride: []string{"bob"}}}, "pick one"},
		{"clear non-member", PoolSettings{Timed: PoolTimedSettings{ClearOverride: []string{"zoe"}}}, "not a member"},
		{"bad timezone", PoolSettings{Timed: PoolTimedSettings{Timezone: strp("Nowhere/City")}}, "not an IANA time zone"},
		{"schedule no name", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"days=sat members=shared short=90"}}}, "name comes first"},
		{"schedule no days", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w members=shared short=90"}}}, "give days"},
		{"schedule no members", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat short=90"}}}, "give members"},
		{"schedule no caps", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared"}}}, "at least one cap"},
		{"schedule unknown key", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared short=90 when=now"}}}, "unknown key"},
		{"schedule key twice", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat days=sun members=shared short=90"}}}, "given twice"},
		{"schedule not key=value", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared short"}}}, "not key=value"},
		{"schedule bad cap", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared short=NaN"}}}, "is not a number"},
		{"schedule bad day", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat,xyz members=shared short=90"}}}, `day "xyz"`},
		{"schedule bad hours", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat hours=8-9 members=shared short=90"}}}, "hours"},
		{"schedule bad zone", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared short=90 timezone=Atlantis"}}}, "timezone"},
		{"schedule bad release", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=shared short=90 release_before=soon"}}}, "release_before"},
		{"schedule unknown member", PoolSettings{Timed: PoolTimedSettings{Schedules: []string{"w days=sat members=zoe short=90"}}}, "neither a member"},
		{"remove unknown", PoolSettings{Timed: PoolTimedSettings{ScheduleRemove: []string{"nights"}}}, "has no schedule nights"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := timedApp(t, timedConfig, timedNow)
			_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: tc.s})
			var ae *apperr.Error
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.As(err, &ae) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
			if readConfig(t, home) != timedConfig {
				t.Error("a refused flag wrote the file")
			}
		})
	}
}

// TestPoolPrune: prune removes only the overrides that have ended;
// nothing ended writes nothing.
func TestPoolPrune(t *testing.T) {
	a, home := timedApp(t, timedConfig, timedNow)
	ch, err := a.PoolPrune(context.Background(), PoolPruneOptions{ID: "auto", DryRun: true})
	if err != nil || !ch.DryRun || len(ch.Changes) != 1 || ch.Changes[0].Key != "pool.accounts.carol.override" {
		t.Fatalf("dry run %+v %v", ch, err)
	}
	if readConfig(t, home) != timedConfig {
		t.Fatal("dry run wrote")
	}
	if _, err := a.PoolPrune(context.Background(), PoolPruneOptions{ID: "auto"}); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, home)
	if strings.Contains(got, `"week": 80`) || !strings.Contains(got, `"until": "2026-11-21T20:00:00+03:00"`) {
		t.Errorf("after prune:\n%s", got)
	}
	poolStillValid(t, a, "auto")
	a.Reload()
	ch, err = a.PoolPrune(context.Background(), PoolPruneOptions{ID: "auto"})
	if err != nil || len(ch.Changes) != 0 || ch.Written {
		t.Errorf("second prune %+v %v", ch, err)
	}
	if _, err := a.PoolPrune(context.Background(), PoolPruneOptions{ID: "alice"}); err == nil {
		t.Error("prune on a token login")
	}
}

// TestPoolShowTimed: pool show lists the schedules with whether they
// apply now, the timezone setting, and each override with its state.
func TestPoolShowTimed(t *testing.T) {
	a, _ := timedApp(t, timedConfig, timedNow)
	info, err := a.PoolShow(PoolShowOptions{ID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Schedules) != 1 || !info.Schedules[0].Active || info.Schedules[0].ReleaseBeforeEffective != "5h" {
		t.Fatalf("schedules %+v", info.Schedules)
	}
	if want := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC); !info.Schedules[0].Until.Equal(want) {
		t.Errorf("until %s", info.Schedules[0].Until)
	}
	var bob, carol PoolMember
	for _, m := range info.Members {
		switch m.Name {
		case "bob":
			bob = m
		case "carol":
			carol = m
		}
	}
	if bob.Override == nil || !bob.Override.Active || carol.Override == nil || carol.Override.Active {
		t.Errorf("overrides bob %+v carol %+v", bob.Override, carol.Override)
	}
	if !slices.Contains(info.Settings, PoolSetting{Key: "timezone", Value: "Europe/Istanbul", Source: SourcePool}) {
		t.Errorf("settings %+v", info.Settings)
	}
	// Monday: the schedule is off.
	a, _ = timedApp(t, timedConfig, timedNow.Add(48*time.Hour))
	if info, err = a.PoolShow(PoolShowOptions{ID: "auto"}); err != nil || info.Schedules[0].Active || info.Schedules[0].Until != nil {
		t.Errorf("Monday %+v %v", info.Schedules, err)
	}
}

// TestTimedSourcesNamed: a timed cap names its kind as the source.
func TestTimedSourcesNamed(t *testing.T) {
	a, _ := timedApp(t, timedConfig, timedNow)
	cfg, err := a.Config()
	if err != nil {
		t.Fatal(err)
	}
	rules, err := cfg.Config.Logins["auto"].Pool.Rules()
	if err != nil {
		t.Fatal(err)
	}
	r := rules.At(timedNow)
	pc := PoolCandidate{CapsSource: CapSources{Short: SourceMember, Week: SourceMember}}
	tm := r.Timed("bob")
	pc.TimedCaps = &tm
	timedSources(&pc)
	if pc.CapsSource.Short != "override" || pc.CapsSource.Week != SourceMember {
		t.Errorf("bob sources %+v", pc.CapsSource)
	}
	pc = PoolCandidate{CapsSource: CapSources{Short: SourceDefault, Week: SourceDefault}}
	timedSources(&pc)
	if pc.CapsSource.Short != SourceDefault {
		t.Errorf("untimed %+v", pc.CapsSource)
	}
}

// TestPoolScheduleBeforeReset: --pool-schedule takes week_before_reset
// like the other caps (alone or with them), writes it inside caps, shows
// it in the before/after line and in pool show, refuses a bad value, and
// a preset leaves it alone.
func TestPoolScheduleBeforeReset(t *testing.T) {
	a, home := timedApp(t, timedConfig, timedNow)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Timed: PoolTimedSettings{
		Schedules: []string{"weekend days=sat,sun members=shared short=95 week_before_reset=95 release_before=5h", "late days=sun members=bob week_before_reset=90"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Changes) != 1 || ch.Changes[0].To != "weekend days=sat,sun members=shared short=95 week_before_reset=95 release_before=5h; late days=sun members=bob week_before_reset=90" {
		t.Errorf("changes %+v", ch.Changes)
	}
	got := compact(readConfig(t, home))
	for _, w := range []string{`"caps":{"short":95,"week_before_reset":95},"release_before":"5h"`, `{"name":"late","days":["sun"],"members":["bob"],"caps":{"week_before_reset":90}}`} {
		if !strings.Contains(got, w) {
			t.Errorf("file lacks %s:\n%s", w, got)
		}
	}
	poolStillValid(t, a, "auto")
	a.Reload()
	info, err := a.PoolShow(PoolShowOptions{ID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Schedules) != 2 || info.Schedules[0].Caps.WeekBeforeReset == nil || *info.Schedules[0].Caps.WeekBeforeReset != 95 ||
		info.Schedules[1].ReleaseBeforeEffective != "0h" {
		t.Errorf("pool show schedules %+v", info.Schedules)
	}
	// A preset writes rule values only: the schedules stay as they are.
	before := readConfig(t, home)
	if _, err := a.PoolPreset(context.Background(), PoolPresetOptions{ID: "auto", Name: "classic"}); err != nil {
		t.Fatal(err)
	}
	if after := compact(readConfig(t, home)); !strings.Contains(after, `"week_before_reset":95`) || !strings.Contains(after, `"week_before_reset":90`) {
		t.Errorf("preset dropped week_before_reset:\n%s\nwas\n%s", after, before)
	}
	for _, tc := range []struct{ spec, want string }{
		{"w days=sat members=shared week_before_reset=101", "week_before_reset"},
		{"w days=sat members=shared week_before_reset=-1", "week_before_reset"},
		{"w days=sat members=shared week_before_reset=lots", "is not a number"},
		{"w days=sat members=shared week_before_reset=95 week_before_reset=90", "given twice"},
	} {
		a, home := timedApp(t, timedConfig, timedNow)
		_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Timed: PoolTimedSettings{Schedules: []string{tc.spec}}}})
		var ae *apperr.Error
		if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.As(err, &ae) || readConfig(t, home) != timedConfig {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}
	// agx config set edits one value, never a list entry: it points to
	// login edit for the schedules.
	a, _ = timedApp(t, timedConfig, timedNow)
	for _, key := range []string{"logins.auto.pool.schedules", "logins.auto.pool.schedules.0.caps.week_before_reset"} {
		if _, err := a.ConfigSet(context.Background(), key, "95"); err == nil || !strings.Contains(err.Error(), "list") {
			t.Errorf("config set %s: %v", key, err)
		}
	}
}
