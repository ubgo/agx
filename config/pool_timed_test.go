package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/config"
)

// TestPoolSchedulesLoad: a pool's timezone, schedules and overrides load
// into the rules; release_before defaults to 5h when a schedule sets
// short and 0 otherwise.
func TestPoolSchedulesLoad(t *testing.T) {
	l, err := parsePool(t, `"auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice"], "shared": ["carol", "derek"], "timezone": "Europe/Istanbul",
      "schedules": [
        { "name": "weekend", "days": ["sat", "sun"], "members": ["shared"], "caps": { "short": 95 } },
        { "name": "nights", "days": ["mon"], "hours": "22:00-06:00", "members": ["derek"], "timezone": "UTC", "caps": { "week": 80 } },
        { "name": "late", "days": ["fri"], "members": ["carol"], "caps": { "short": 90 }, "release_before": "0s" }
      ],
      "accounts": { "carol": { "short": 40, "override": { "week": 90, "until": "2026-11-22T23:59:00+03:00" } } } } }`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Config.Logins["auto"].Pool.Rules()
	if err != nil {
		t.Fatal(err)
	}
	if r.Location == nil || r.Location.String() != "Europe/Istanbul" || len(r.Schedules) != 3 {
		t.Fatalf("location %v, %d schedules", r.Location, len(r.Schedules))
	}
	w, n, late := r.Schedules[0], r.Schedules[1], r.Schedules[2]
	if w.ReleaseBefore != 5*time.Hour || n.ReleaseBefore != 0 || late.ReleaseBefore != 0 {
		t.Errorf("release_before %s %s %s", w.ReleaseBefore, n.ReleaseBefore, late.ReleaseBefore)
	}
	if w.Location != nil || n.Location.String() != "UTC" || n.Start != 22*60 || n.End != 6*60 || w.End != 24*60 {
		t.Errorf("weekend %+v nights %+v", w, n)
	}
	o := r.Accounts["carol"].Override
	if o == nil || *o.Caps.Week != 90 || o.Caps.Short != nil || o.Until.UTC() != time.Date(2026, 11, 22, 20, 59, 0, 0, time.UTC) {
		t.Errorf("override %+v", o)
	}
	// Saturday in Istanbul: carol's week from the override, her short
	// from the weekend schedule (over her own 40).
	sat := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	if caps := r.At(sat).CapsFor("carol"); caps.Short != 95 || caps.Week != 90 {
		t.Errorf("Saturday carol %+v", caps)
	}
	if caps := r.At(sat.Add(7 * 24 * time.Hour)).CapsFor("carol"); caps.Short != 95 || caps.Week != 70 {
		t.Errorf("next Saturday carol (override ended) %+v", caps)
	}
}

// TestPoolSchedulesProblems: each timed-cap value is checked at load,
// with its position.
func TestPoolSchedulesProblems(t *testing.T) {
	block := func(extra string) string {
		return `"auto": { "agents": ["claude"], "kind": "pool", "pool": { "own": ["alice"], "shared": ["carol"], ` + extra + ` } }`
	}
	sched := func(body string) string { return block(`"schedules": [ { "name": "w", ` + body + ` } ]`) }
	for _, tc := range []struct {
		name, pool, want string
	}{
		{"bad pool zone", block(`"timezone": "Mars/Olympus"`), `timezone "Mars/Olympus" is not an IANA time zone`},
		{"local zone", block(`"timezone": "Local"`), "leave it out"},
		{"bad schedule zone", sched(`"days": ["sat"], "members": ["shared"], "timezone": "nowhere", "caps": { "short": 90 }`), `schedule w timezone "nowhere"`},
		{"bad day", sched(`"days": ["sat", "funday"], "members": ["shared"], "caps": { "short": 90 }`), `must be one of "mon"`},
		{"no days", sched(`"days": [], "members": ["shared"], "caps": { "short": 90 }`), "days is empty"},
		{"bad hours", sched(`"days": ["sat"], "hours": "9-17", "members": ["shared"], "caps": { "short": 90 }`), `hours "9-17"`},
		{"empty hours", sched(`"days": ["sat"], "hours": "10:00-10:00", "members": ["shared"], "caps": { "short": 90 }`), "start and end are the same"},
		{"unknown member", sched(`"days": ["sat"], "members": ["zoe"], "caps": { "short": 90 }`), `names "zoe"`},
		{"no members", sched(`"days": ["sat"], "members": [], "caps": { "short": 90 }`), "names no members"},
		{"no caps", sched(`"days": ["sat"], "members": ["shared"], "caps": {}`), "sets no caps"},
		{"cap over 100", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": 101 }`), "must be <= 100"},
		{"null cap", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": null }`), "short"},
		{"string cap", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": "95" }`), "short"},
		{"week_before_reset over 100", sched(`"days": ["sat"], "members": ["shared"], "caps": { "week_before_reset": 101 }`), "must be <= 100"},
		{"week_before_reset negative", sched(`"days": ["sat"], "members": ["shared"], "caps": { "week_before_reset": -1 }`), "must be >= 0"},
		{"week_before_reset null", sched(`"days": ["sat"], "members": ["shared"], "caps": { "week_before_reset": null }`), "week_before_reset"},
		{"week_before_reset string", sched(`"days": ["sat"], "members": ["shared"], "caps": { "week_before_reset": "95" }`), "week_before_reset"},
		{"week_before_reset outside caps", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": 90 }, "week_before_reset": 95`), "week_before_reset"},
		{"bad release", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": 90 }, "release_before": "5 hours"`), `release_before "5 hours" is not a duration`},
		{"negative release", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": 90 }, "release_before": "-1h"`), "is negative"},
		{"unknown schedule key", sched(`"days": ["sat"], "members": ["shared"], "caps": { "short": 90 }, "weeks": 2`), "weeks"},
		{"unknown caps key", sched(`"days": ["sat"], "members": ["shared"], "caps": { "daily": 90 }`), "daily"},
		{"duplicate name", block(`"schedules": [ { "name": "w", "days": ["sat"], "members": ["shared"], "caps": { "short": 90 } }, { "name": "w", "days": ["sun"], "members": ["shared"], "caps": { "short": 90 } } ]`), "listed more than once"},
		{"override until", block(`"accounts": { "carol": { "override": { "short": 90, "until": "sunday" } } }`), "not an RFC 3339 time"},
		{"override no caps", block(`"accounts": { "carol": { "override": { "until": "2026-11-22T23:59:00+03:00" } } }`), "sets no caps"},
		{"override cap", block(`"accounts": { "carol": { "override": { "week": -5, "until": "2026-11-22T23:59:00+03:00" } } }`), "must be >= 0"},
		{"override unknown key", block(`"accounts": { "carol": { "override": { "short": 90, "until": "2026-11-22T23:59:00+03:00", "from": "x" } } }`), "from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePool(t, tc.pool)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v\nwant %q", err, tc.want)
			}
		})
	}
}

// TestScheduleRulesDirect: ScheduleRules is what the simulator reads its
// scenario files through.
func TestScheduleRulesDirect(t *testing.T) {
	short := 95.0
	got, err := config.ScheduleRules([]config.PoolSchedule{{Name: "w", Days: []string{"sat"}, Members: []string{"shared"}, Caps: config.PoolScheduleCaps{Short: &short}, ReleaseBefore: "2h"}})
	if err != nil || len(got) != 1 || got[0].ReleaseBefore != 2*time.Hour || !got[0].Days[time.Saturday] {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := config.ScheduleRules([]config.PoolSchedule{{Name: "w", Days: []string{"sat"}, Hours: "x"}}); err == nil {
		t.Error("bad hours accepted")
	}
}

// TestPoolScheduleBeforeResetLoad: caps.week_before_reset loads into the
// schedule, alone (release_before then defaults to 0s, the schedule's
// whole window) or with short (5h), and raises the weekly cap of a
// member whose reset falls before the schedule ends.
func TestPoolScheduleBeforeResetLoad(t *testing.T) {
	l, err := parsePool(t, `"auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice"], "shared": ["carol"], "timezone": "Europe/Istanbul",
      "schedules": [
        { "name": "weekend", "days": ["sat", "sun"], "members": ["shared"], "caps": { "short": 95, "week_before_reset": 95 } },
        { "name": "only", "days": ["sat"], "members": ["carol"], "caps": { "week_before_reset": 90 } }
      ] } }`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Config.Logins["auto"].Pool.Rules()
	if err != nil {
		t.Fatal(err)
	}
	w, only := r.Schedules[0], r.Schedules[1]
	if w.WeekBeforeReset == nil || *w.WeekBeforeReset != 95 || w.ReleaseBefore != 5*time.Hour || w.Caps.Week != nil {
		t.Errorf("weekend %+v", w)
	}
	if only.WeekBeforeReset == nil || *only.WeekBeforeReset != 90 || only.ReleaseBefore != 0 {
		t.Errorf("only %+v", only)
	}
	// Saturday 13:00 in Istanbul: "only" names carol and ends Sunday
	// 00:00, so it wins for a reset before then; "weekend" (the group)
	// covers one up to Sunday 19:00.
	sat := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   time.Duration
		want float64
		from string
	}{{5 * time.Hour, 90, "only"}, {20 * time.Hour, 95, "weekend"}, {40 * time.Hour, 0, ""}} {
		v, src, ok := r.At(sat).WeekBeforeReset("carol", sat.Add(tc.in), sat)
		if ok != (tc.from != "") || v != tc.want || src.Name != tc.from {
			t.Errorf("reset in %s: %v %q %v", tc.in, v, src.Name, ok)
		}
	}
}
