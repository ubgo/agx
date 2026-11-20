package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agx/internal/apperr"
)

// TestPoolStrategyFlags: login edit writes every strategy flag, the
// result loads with the values, and empty values remove the keys again.
func TestPoolStrategyFlags(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Strategy: PoolStrategySettings{
		Strategy: strp("expiring"), WeekRelaxWindow: strp("12h"), MaxSessions: strp("2"), Sticky: strp("false"), StickyLeaveRatio: strp("3"),
		DefaultWeekRelax: strp("90"), CapWeekRelax: []string{"carol=70"}, Capacity: []string{"alice=20", "carol=5"},
		GroupCapShort: []string{"own=95"}, GroupCapWeek: []string{"own=98", "shared=60"}, GroupCapWeekRelax: []string{"shared=85"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"pool.accounts.carol.week_relax_max", "", "70"}, {"pool.accounts.carol.capacity", "", "5"}, {"pool.accounts.alice.capacity", "", "20"}, {"pool.defaults.week_relax_max", "", "90"},
		{"pool.strategy", "", "expiring"}, {"pool.max_sessions", "", "2"}, {"pool.sticky", "", "false"}, {"pool.sticky_leave_ratio", "", "3"},
		{"pool.own_caps.short", "", "95"}, {"pool.own_caps.week", "", "98"},
		{"pool.shared_caps.week", "", "60"}, {"pool.shared_caps.week_relax_max", "", "85"},
		{"pool.week_relax_window", "", "12h"},
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
	if err != nil || r.Strategy != "expiring" || r.MaxSessions != 2 || r.Sticky || r.RelaxMaxFor("carol") != 70 || r.CapsFor("bob").Short != 95 {
		t.Errorf("rules %+v %v", r, err)
	}
	empty := ""
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Strategy: PoolStrategySettings{
		Strategy: &empty, WeekRelaxWindow: &empty, MaxSessions: &empty, Sticky: &empty, StickyLeaveRatio: &empty, DefaultWeekRelax: &empty,
		CapWeekRelax: []string{"carol="}, Capacity: []string{"alice=", "carol="}, GroupCapShort: []string{"own="}, GroupCapWeek: []string{"own=", "shared="}, GroupCapWeekRelax: []string{"shared="},
	}}}); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, home)
	for _, gone := range []string{"strategy", "max_sessions", "sticky", "week_relax", "own_caps", "shared_caps"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s left after removing it:\n%s", gone, got)
		}
	}
	poolStillValid(t, a, "auto")
}

// TestPoolStrategyFlagsRefused: bad strategy flag values are usage
// errors that name the flag.
func TestPoolStrategyFlagsRefused(t *testing.T) {
	cases := []struct {
		name string
		s    PoolStrategySettings
		want string
	}{
		{"strategy", PoolStrategySettings{Strategy: strp("soonest")}, `--pool-strategy "soonest": use deadline or expiring`},
		{"max sessions word", PoolStrategySettings{MaxSessions: strp("lots")}, "--pool-max-sessions"},
		{"max sessions negative", PoolStrategySettings{MaxSessions: strp("-1")}, "--pool-max-sessions"},
		{"sticky", PoolStrategySettings{Sticky: strp("maybe")}, `--pool-sticky "maybe": use true or false`},
		{"leave ratio", PoolStrategySettings{StickyLeaveRatio: strp("-2")}, "--pool-sticky-leave-ratio"},
		{"window", PoolStrategySettings{WeekRelaxWindow: strp("soon")}, "--pool-week-relax-window"},
		{"relax over 100", PoolStrategySettings{DefaultWeekRelax: strp("120")}, "--pool-default-week-relax 120"},
		{"group name", PoolStrategySettings{GroupCapShort: []string{"team=10"}}, `--pool-group-cap-short "team=10": use own=N or shared=N`},
		{"group twice", PoolStrategySettings{GroupCapWeek: []string{"own=1", "own=2"}}, "--pool-group-cap-week own is given twice"},
		{"member relax", PoolStrategySettings{CapWeekRelax: []string{"zed=10"}}, "zed is not a member"},
		{"capacity zero", PoolStrategySettings{Capacity: []string{"alice=0"}}, "--pool-capacity alice=0: 0 is not a number above 0"},
		{"capacity form", PoolStrategySettings{Capacity: []string{"alice"}}, `--pool-capacity "alice": use NAME=N`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, home := fixtureApp(t, flagConfig, nil, nil)
			_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Strategy: c.s}})
			if e := apperr.As(err); e == nil || e.Code != apperr.CodeUsage || !strings.Contains(e.Message, c.want) {
				t.Errorf("%v, want usage %q", err, c.want)
			}
			if readConfig(t, home) != flagConfig {
				t.Error("a refused edit wrote the file")
			}
		})
	}
}

// TestPoolStrategyConfigSet: agx config set types the strategy keys from
// the schema: the enum, a whole number of 0 or more, a bool, a percent.
func TestPoolStrategyConfigSet(t *testing.T) {
	for _, c := range []struct {
		key, value string
		ok         bool
	}{
		{"logins.auto.pool.strategy", "expiring", true},
		{"logins.auto.pool.strategy", "soonest", false},
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
