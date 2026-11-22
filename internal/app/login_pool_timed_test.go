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

// TestPoolScheduleFlags: --pool-schedule adds or changes by name,
// --pool-schedule-replace replaces, --pool-schedule-remove removes, the
// list goes when empty.
func TestPoolScheduleFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    PoolTimedSettings
		want []string
		gone []string
	}{
		{"add", PoolTimedSettings{Schedules: []string{"nights days=Mon,tue hours=22:00-06:00 members=bob,own timezone=UTC week=80 week_relax_max=90 release_before=1h"}},
			[]string{`"name":"weekend"`, `{"name":"nights","days":["mon","tue"],"hours":"22:00-06:00","members":["bob","own"],"timezone":"UTC","caps":{"week":80,"week_relax_max":90},"release_before":"1h"}`}, nil},
		{"replace", PoolTimedSettings{ScheduleReplace: []string{"weekend days=sun members=carol short=80"}},
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
	if want := []Change{
		{"pool.schedules.weekend.caps.week_before_reset", "", "95"}, {"pool.schedules.weekend.release_before", "", "5h"},
		{"pool.schedules.late.days", "", "sun"}, {"pool.schedules.late.members", "", "bob"}, {"pool.schedules.late.caps.week_before_reset", "", "90"},
	}; !slices.Equal(ch.Changes, want) {
		t.Errorf("changes %+v\nwant    %+v", ch.Changes, want)
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

// mergeConfig is timedConfig with a weekend schedule that sets every
// kind of key: a merge must keep the ones an edit does not name.
var mergeConfig = strings.Replace(timedConfig,
	`"schedules": [ { "name": "weekend", "days": ["sat", "sun"], "members": ["shared"], "caps": { "short": 95 } } ],`,
	`"schedules": [
        { "name": "weekend", "days": ["sat", "sun"], "hours": "06:00-23:00", "members": ["shared"], "timezone": "Asia/Tokyo",
          "caps": { "short": 90, "week": 75 }, "release_before": "2h" },
        { "name": "nights", "days": ["mon"], "hours": "22:00-06:00", "members": ["bob"], "caps": { "short": 80 } }
      ],`, 1)

// weekendMerged is the weekend schedule of mergeConfig as the file holds
// it, for checks that it is unchanged.
const weekendMerged = `{"name":"weekend","days":["sat","sun"],"hours":"06:00-23:00","members":["shared"],"timezone":"Asia/Tokyo","caps":{"short":90,"week":75},"release_before":"2h"}`

// TestPoolScheduleMerge: --pool-schedule on an existing name changes only
// the keys it gives (the owner's case: an edit without timezone keeps
// it), key= removes one, --pool-schedule-replace writes the whole
// schedule, a new name creates one, and each changed key gets its own
// before/after line, removals included.
func TestPoolScheduleMerge(t *testing.T) {
	if !strings.Contains(mergeConfig, `"nights"`) {
		t.Fatal("mergeConfig did not replace the fixture's schedules")
	}
	for _, tc := range []struct {
		name    string
		s       PoolTimedSettings
		changes []Change
		want    []string
		gone    []string
	}{
		{"owner case: timezone, hours, release_before and week kept",
			PoolTimedSettings{Schedules: []string{"weekend days=sat,sun members=shared short=95 week_before_reset=95"}},
			[]Change{{"pool.schedules.weekend.caps.short", "90", "95"}, {"pool.schedules.weekend.caps.week_before_reset", "", "95"}},
			[]string{`{"name":"weekend","days":["sat","sun"],"hours":"06:00-23:00","members":["shared"],"timezone":"Asia/Tokyo","caps":{"short":95,"week":75,"week_before_reset":95},"release_before":"2h"}`}, nil},
		{"one key", PoolTimedSettings{Schedules: []string{"weekend week_before_reset=95"}},
			[]Change{{"pool.schedules.weekend.caps.week_before_reset", "", "95"}},
			[]string{`"timezone":"Asia/Tokyo","caps":{"short":90,"week":75,"week_before_reset":95},"release_before":"2h"`}, nil},
		{"key= removes", PoolTimedSettings{Schedules: []string{"weekend timezone= week= hours= release_before="}},
			[]Change{{"pool.schedules.weekend.hours", "06:00-23:00", ""}, {"pool.schedules.weekend.timezone", "Asia/Tokyo", ""},
				{"pool.schedules.weekend.caps.week", "75", ""}, {"pool.schedules.weekend.release_before", "2h", ""}},
			[]string{`{"name":"weekend","days":["sat","sun"],"members":["shared"],"caps":{"short":90}}`}, []string{"Asia/Tokyo", `"2h"`}},
		{"list keys replace their list", PoolTimedSettings{Schedules: []string{"weekend days=SUN members=bob,carol"}},
			[]Change{{"pool.schedules.weekend.days", "sat,sun", "sun"}, {"pool.schedules.weekend.members", "shared", "bob,carol"}},
			[]string{`"days":["sun"],"hours":"06:00-23:00","members":["bob","carol"],"timezone":"Asia/Tokyo"`}, nil},
		{"replace drops the keys it does not give", PoolTimedSettings{ScheduleReplace: []string{"weekend days=sat,sun members=shared short=95"}},
			[]Change{{"pool.schedules.weekend.hours", "06:00-23:00", ""}, {"pool.schedules.weekend.timezone", "Asia/Tokyo", ""},
				{"pool.schedules.weekend.caps.short", "90", "95"}, {"pool.schedules.weekend.caps.week", "75", ""}, {"pool.schedules.weekend.release_before", "2h", ""}},
			[]string{`{"name":"weekend","days":["sat","sun"],"members":["shared"],"caps":{"short":95}}`}, []string{"Asia/Tokyo"}},
		{"replace of a new name creates it", PoolTimedSettings{ScheduleReplace: []string{"holiday days=fri members=own week=99"}},
			[]Change{{"pool.schedules.holiday.days", "", "fri"}, {"pool.schedules.holiday.members", "", "own"}, {"pool.schedules.holiday.caps.week", "", "99"}},
			[]string{weekendMerged, `{"name":"holiday","days":["fri"],"members":["own"],"caps":{"week":99}}`}, nil},
		{"new schedule", PoolTimedSettings{Schedules: []string{"holiday days=thu,fri members=shared short=95 timezone=UTC"}},
			[]Change{{"pool.schedules.holiday.days", "", "thu,fri"}, {"pool.schedules.holiday.members", "", "shared"}, {"pool.schedules.holiday.timezone", "", "UTC"},
				{"pool.schedules.holiday.caps.short", "", "95"}},
			[]string{weekendMerged, `{"name":"holiday","days":["thu","fri"],"members":["shared"],"timezone":"UTC","caps":{"short":95}}`}, nil},
		{"key= on a new schedule removes nothing", PoolTimedSettings{Schedules: []string{"holiday days=fri members=shared short=95 hours="}},
			[]Change{{"pool.schedules.holiday.days", "", "fri"}, {"pool.schedules.holiday.members", "", "shared"}, {"pool.schedules.holiday.caps.short", "", "95"}},
			[]string{`{"name":"holiday","days":["fri"],"members":["shared"],"caps":{"short":95}}`}, nil},
		{"repeated flags, one per schedule", PoolTimedSettings{Schedules: []string{"weekend week_before_reset=95", "nights hours=23:00-05:00"},
			ScheduleReplace: []string{"holiday days=fri members=bob short=85"}},
			[]Change{{"pool.schedules.weekend.caps.week_before_reset", "", "95"}, {"pool.schedules.nights.hours", "22:00-06:00", "23:00-05:00"},
				{"pool.schedules.holiday.days", "", "fri"}, {"pool.schedules.holiday.members", "", "bob"}, {"pool.schedules.holiday.caps.short", "", "85"}},
			[]string{`"timezone":"Asia/Tokyo","caps":{"short":90,"week":75,"week_before_reset":95}`,
				`{"name":"nights","days":["mon"],"hours":"23:00-05:00","members":["bob"],"caps":{"short":80}}`,
				`{"name":"holiday","days":["fri"],"members":["bob"],"caps":{"short":85}}`}, nil},
		{"remove shows every key going", PoolTimedSettings{ScheduleRemove: []string{"nights"}},
			[]Change{{"pool.schedules.nights.days", "mon", ""}, {"pool.schedules.nights.hours", "22:00-06:00", ""}, {"pool.schedules.nights.members", "bob", ""},
				{"pool.schedules.nights.caps.short", "80", ""}},
			[]string{weekendMerged}, []string{`"nights"`}},
		{"value already set: no change", PoolTimedSettings{Schedules: []string{"weekend short=90 timezone=Asia/Tokyo"}}, nil, []string{weekendMerged}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := timedApp(t, mergeConfig, timedNow)
			ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Timed: tc.s}})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ch.Changes, tc.changes) {
				t.Errorf("changes %+v\nwant    %+v", ch.Changes, tc.changes)
			}
			got := readConfig(t, home)
			if len(tc.changes) == 0 && got != mergeConfig {
				t.Errorf("no change wrote the file:\n%s", got)
			}
			for _, w := range append(tc.want, "// timed caps fixture") {
				if !strings.Contains(compact(got), compact(w)) {
					t.Errorf("file lacks %s:\n%s", w, got)
				}
			}
			for _, g := range tc.gone {
				if strings.Contains(got, g) {
					t.Errorf("file still has %s:\n%s", g, got)
				}
			}
			poolStillValid(t, a, "auto")
		})
	}
}

// TestPoolScheduleMergeDryRun: --dry-run lists the same per-key lines
// and writes nothing.
func TestPoolScheduleMergeDryRun(t *testing.T) {
	a, home := timedApp(t, mergeConfig, timedNow)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", DryRun: true, Pool: PoolSettings{Timed: PoolTimedSettings{
		Schedules: []string{"weekend week_before_reset=95 timezone="}}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{"pool.schedules.weekend.timezone", "Asia/Tokyo", ""}, {"pool.schedules.weekend.caps.week_before_reset", "", "95"}}
	if !ch.DryRun || ch.Written || !slices.Equal(ch.Changes, want) {
		t.Errorf("dry run %+v", ch)
	}
	if readConfig(t, home) != mergeConfig {
		t.Error("dry run wrote the file")
	}
}

// TestPoolScheduleMergeRefusals: the schedule is checked after the
// merge (a key= that removes a needed key names it), a schedule named
// twice in one command is a conflict, and nothing is written.
func TestPoolScheduleMergeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    PoolTimedSettings
		code apperr.Code
		want string
	}{
		{"days removed", PoolTimedSettings{Schedules: []string{"weekend days="}}, apperr.CodeUsage, "days= removes the schedule's days"},
		{"members removed", PoolTimedSettings{Schedules: []string{"weekend members="}}, apperr.CodeUsage, "members= removes the schedule's members"},
		{"every cap removed", PoolTimedSettings{Schedules: []string{"weekend short= week="}}, apperr.CodeUsage, "short= week= leaves the schedule no cap"},
		{"merged hours bad", PoolTimedSettings{Schedules: []string{"weekend hours=25:00-26:00"}}, apperr.CodeUsage, "hours"},
		{"merged zone bad", PoolTimedSettings{Schedules: []string{"weekend timezone=Atlantis"}}, apperr.CodeUsage, "timezone"},
		{"merged member unknown", PoolTimedSettings{Schedules: []string{"weekend members=zoe"}}, apperr.CodeUsage, "neither a member"},
		{"merged cap bad", PoolTimedSettings{Schedules: []string{"weekend week=150"}}, apperr.CodeUsage, "is not a number"},
		{"new schedule needs days", PoolTimedSettings{Schedules: []string{"holiday members=shared short=95"}}, apperr.CodeUsage, "give days"},
		{"new schedule needs a cap", PoolTimedSettings{Schedules: []string{"holiday days=fri members=shared"}}, apperr.CodeUsage, "give at least one cap"},
		{"replace needs members", PoolTimedSettings{ScheduleReplace: []string{"weekend days=sat short=95"}}, apperr.CodeUsage, "give members"},
		{"replace checks keys", PoolTimedSettings{ScheduleReplace: []string{"weekend days=sat members=shared short=95 when=now"}}, apperr.CodeUsage, "unknown key"},
		{"name first in replace", PoolTimedSettings{ScheduleReplace: []string{"days=sat"}}, apperr.CodeUsage, "--pool-schedule-replace"},
		{"same name twice", PoolTimedSettings{Schedules: []string{"weekend short=95", "weekend week=80"}}, apperr.CodeConflict, "give each schedule once"},
		{"edit and replace", PoolTimedSettings{Schedules: []string{"weekend short=95"}, ScheduleReplace: []string{"weekend days=sat members=shared short=95"}}, apperr.CodeConflict, "both change schedule weekend"},
		{"edit and remove", PoolTimedSettings{Schedules: []string{"weekend short=95"}, ScheduleRemove: []string{"weekend"}}, apperr.CodeConflict, "--pool-schedule-remove weekend and --pool-schedule weekend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := timedApp(t, mergeConfig, timedNow)
			_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Timed: tc.s}})
			var ae *apperr.Error
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.As(err, &ae) || ae.Code != tc.code {
				t.Fatalf("err %v, want %s %q", err, tc.code, tc.want)
			}
			if readConfig(t, home) != mergeConfig {
				t.Error("a refused flag wrote the file")
			}
		})
	}
}
