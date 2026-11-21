package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/apperr"
)

// TestPoolRuleFlags: login edit writes every rule flag, the
// result loads with the values, and empty values remove the keys again.
func TestPoolRuleFlags(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: PoolRuleSettings{
		Order: strp("deadline"), WeekRelaxWindow: strp("12h"), MaxSessions: strp("2"), Sticky: strp("false"), StickyLeaveRatio: strp("3"),
		DeadlineWindow: strp("6h"), DeadlineMinRoom: strp("5"),
		DefaultWeekRelax: strp("90"), CapWeekRelax: []string{"carol=70"}, Capacity: []string{"alice=20", "carol=5"},
		GroupCapShort: []string{"own=95"}, GroupCapWeek: []string{"own=98", "shared=60"}, GroupCapWeekRelax: []string{"shared=85"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"pool.accounts.carol.week_relax_max", "", "70"}, {"pool.accounts.carol.capacity", "", "5"}, {"pool.accounts.alice.capacity", "", "20"}, {"pool.defaults.week_relax_max", "", "90"},
		{"pool.order", "", "deadline"}, {"pool.max_sessions", "", "2"}, {"pool.sticky", "", "false"}, {"pool.sticky_leave_ratio", "", "3"},
		{"pool.deadline_min_room", "", "5"},
		{"pool.own_caps.short", "", "95"}, {"pool.own_caps.week", "", "98"},
		{"pool.shared_caps.week", "", "60"}, {"pool.shared_caps.week_relax_max", "", "85"},
		{"pool.week_relax_window", "", "12h"}, {"pool.deadline_window", "", "6h"},
	}
	if !slices.Equal(ch.Changes, want) || !ch.Written {
		t.Errorf("changes %+v", ch.Changes)
	}
	poolStillValid(t, a, "auto")
	cfg, err := a.Config()
	if err != nil {
		t.Fatal(err)
	}
	r, err := cfg.Config.Logins["auto"].Pool.Rules()
	if err != nil || r.Order != "deadline" || r.MaxSessions != 2 || r.Sticky || r.RelaxMaxFor("carol") != 70 || r.CapsFor("bob").Short != 95 ||
		r.DeadlineWindow != 6*time.Hour || r.DeadlineMinRoom != 5 {
		t.Errorf("rules %+v %v", r, err)
	}
	empty := ""
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: PoolRuleSettings{
		Order: &empty, WeekRelaxWindow: &empty, MaxSessions: &empty, Sticky: &empty, StickyLeaveRatio: &empty, DefaultWeekRelax: &empty,
		DeadlineWindow: &empty, DeadlineMinRoom: &empty,
		CapWeekRelax: []string{"carol="}, Capacity: []string{"alice=", "carol="}, GroupCapShort: []string{"own="}, GroupCapWeek: []string{"own=", "shared="}, GroupCapWeekRelax: []string{"shared="},
	}}}); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, home)
	for _, gone := range []string{`"order"`, "max_sessions", "sticky", "week_relax", "own_caps", "shared_caps", "deadline"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s left after removing it:\n%s", gone, got)
		}
	}
	poolStillValid(t, a, "auto")
}

// TestPoolRuleFlagsRefused: bad rule flag values are usage
// errors that name the flag.
func TestPoolRuleFlagsRefused(t *testing.T) {
	cases := []struct {
		name string
		s    PoolRuleSettings
		want string
	}{
		{"order", PoolRuleSettings{Order: strp("soonest")}, `--pool-order "soonest": use urgency or deadline`},
		{"max sessions word", PoolRuleSettings{MaxSessions: strp("lots")}, "--pool-max-sessions"},
		{"max sessions negative", PoolRuleSettings{MaxSessions: strp("-1")}, "--pool-max-sessions"},
		{"sticky", PoolRuleSettings{Sticky: strp("maybe")}, `--pool-sticky "maybe": use true or false`},
		{"leave ratio", PoolRuleSettings{StickyLeaveRatio: strp("-2")}, "--pool-sticky-leave-ratio"},
		{"window", PoolRuleSettings{WeekRelaxWindow: strp("soon")}, "--pool-week-relax-window"},
		{"deadline window word", PoolRuleSettings{DeadlineWindow: strp("soon")}, "--pool-deadline-window"},
		{"deadline window negative", PoolRuleSettings{DeadlineWindow: strp("-1h")}, "--pool-deadline-window"},
		{"deadline min room over 100", PoolRuleSettings{DeadlineMinRoom: strp("120")}, "--pool-deadline-min-room 120"},
		{"deadline min room negative", PoolRuleSettings{DeadlineMinRoom: strp("-1")}, "--pool-deadline-min-room -1"},
		{"deadline min room word", PoolRuleSettings{DeadlineMinRoom: strp("lots")}, "--pool-deadline-min-room lots"},
		{"relax over 100", PoolRuleSettings{DefaultWeekRelax: strp("120")}, "--pool-default-week-relax 120"},
		{"group name", PoolRuleSettings{GroupCapShort: []string{"team=10"}}, `--pool-group-cap-short "team=10": use own=N or shared=N`},
		{"group twice", PoolRuleSettings{GroupCapWeek: []string{"own=1", "own=2"}}, "--pool-group-cap-week own is given twice"},
		{"member relax", PoolRuleSettings{CapWeekRelax: []string{"zed=10"}}, "zed is not a member"},
		{"capacity zero", PoolRuleSettings{Capacity: []string{"alice=0"}}, "--pool-capacity alice=0: 0 is not a number above 0"},
		{"capacity form", PoolRuleSettings{Capacity: []string{"alice"}}, `--pool-capacity "alice": use NAME=N`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, home := fixtureApp(t, flagConfig, nil, nil)
			_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Rules: c.s}})
			if e := apperr.As(err); e == nil || e.Code != apperr.CodeUsage || !strings.Contains(e.Message, c.want) {
				t.Errorf("%v, want usage %q", err, c.want)
			}
			if readConfig(t, home) != flagConfig {
				t.Error("a refused edit wrote the file")
			}
		})
	}
}

// TestPoolRuleConfigSet: agx config set types the rule keys from
// the schema: the enum, a whole number of 0 or more, a bool, a percent.
func TestPoolRuleConfigSet(t *testing.T) {
	for _, c := range []struct {
		key, value string
		ok         bool
	}{
		{"logins.auto.pool.order", "deadline", true},
		{"logins.auto.pool.order", "soonest", false},
		{"logins.auto.pool.strategy", "expiring", false},
		{"logins.auto.pool.week_relax_window", "0s", true},
		{"logins.auto.pool.deadline_window", "0s", true},
		{"logins.auto.pool.deadline_window", "12h", true},
		{"logins.auto.pool.deadline_window", "-1h", false},
		{"logins.auto.pool.deadline_window", "soon", false},
		{"logins.auto.pool.deadline_min_room", "0", true},
		{"logins.auto.pool.deadline_min_room", "5", true},
		{"logins.auto.pool.deadline_min_room", "101", false},
		{"logins.auto.pool.deadline_min_room", "-1", false},
		{"logins.auto.pool.deadline_min_room", "null", false},
		{"logins.auto.pool.max_sessions", "0", true},
		{"logins.auto.pool.max_sessions", "4", true},
		{"logins.auto.pool.max_sessions", "-1", false},
		{"logins.auto.pool.max_sessions", "1.5", false},
		{"logins.auto.pool.sticky", "false", true},
		{"logins.auto.pool.sticky", "null", false},
		{"logins.auto.pool.sticky_leave_ratio", "3", true},
		{"logins.auto.pool.sticky_leave_ratio", "-1", false},
		{"logins.auto.pool.defaults.week_relax_max", "98", true},
		{"logins.auto.pool.shared_caps.week", "101", false},
		{"logins.auto.pool.accounts.alice.week_relax_max", "90", true},
		{"logins.auto.pool.accounts.alice.capacity", "20", true},
	} {
		a, _ := fixtureApp(t, flagConfig, nil, nil)
		_, err := a.ConfigSet(context.Background(), c.key, c.value)
		if (err == nil) != c.ok {
			t.Errorf("set %s=%s: %v, want ok %v", c.key, c.value, err, c.ok)
		}
	}
}
