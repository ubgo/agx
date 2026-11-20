package app

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// flagConfig is a pool "auto" (own alice and bob, shared carol) with one
// member entry, plus a token login dave outside the pool; comments
// in several places must survive every edit.
const flagConfig = `{
  // pool flags fixture
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "dave": { "agents": ["claude"], "kind": "oauth-token", "token": "${DAVE_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob"], "shared": ["carol"],
      // alice is mine
      "accounts": { "alice": { "short": 95, "week": 98 } }
    } }
  }
}`

// compact drops every space and line break, so a check does not depend
// on how the editor laid out a new block.
func compact(s string) string { return strings.Join(strings.Fields(s), "") }

// strp is a pointer to s.
func strp(s string) *string { return &s }

// readConfig reads a fixtureApp's config.jsonc.
func readConfig(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(configPath(home))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// poolStillValid reloads the file and checks it loads (agx config check)
// and the pool's rules validate.
func poolStillValid(t *testing.T, a *App, name string) {
	t.Helper()
	a.Reload()
	cfg, err := a.Config()
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	if _, err := cfg.Config.Logins[name].Pool.Rules(); err != nil {
		t.Fatalf("pool rules: %v", err)
	}
}

// TestPoolFlagsEdit is login edit with each pool setting flag: the
// before/after lines, the keys written, comments kept, and a result that
// passes config check and the pool validation.
func TestPoolFlagsEdit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		s       PoolSettings
		changes []Change
		want    []string
		gone    []string
	}{
		{"cap short on an entry", PoolSettings{CapShort: []string{"alice=90"}},
			[]Change{{"pool.accounts.alice.short", "95", "90"}}, []string{`"short": 90`}, nil},
		{"cap week on a new entry", PoolSettings{CapWeek: []string{"bob=97.5"}},
			[]Change{{"pool.accounts.bob.week", "", "97.5"}}, []string{`"bob": {"week":97.5}`}, nil},
		{"both caps, two members", PoolSettings{CapShort: []string{"bob=95", "carol=40"}, CapWeek: []string{"bob=98"}},
			[]Change{{"pool.accounts.bob.short", "", "95"}, {"pool.accounts.bob.week", "", "98"}, {"pool.accounts.carol.short", "", "40"}}, []string{`"bob": {"short":95, "week":98}`, `"carol": {"short":40}`}, nil},
		{"cap removed", PoolSettings{CapShort: []string{"alice="}},
			[]Change{{"pool.accounts.alice.short", "95", ""}}, []string{`"alice": { "week": 98 }`}, nil},
		{"entry removed when empty", PoolSettings{CapShort: []string{"alice="}, CapWeek: []string{"alice="}},
			[]Change{{"pool.accounts.alice.short", "95", ""}, {"pool.accounts.alice.week", "98", ""}}, nil, []string{`"accounts"`}},
		{"disable", PoolSettings{Disable: []string{"carol"}},
			[]Change{{"pool.accounts.carol.enabled", "true", "false"}}, []string{`"carol": {"enabled":false}`}, nil},
		{"default short", PoolSettings{DefaultShort: strp("60")},
			[]Change{{"pool.defaults.short", "", "60"}}, []string{`"defaults": {"short":60}`}, nil},
		{"default week", PoolSettings{DefaultWeek: strp("80")},
			[]Change{{"pool.defaults.week", "", "80"}}, []string{`"defaults": {"week":80}`}, nil},
		{"short cutoff", PoolSettings{ShortCutoff: strp("98")},
			[]Change{{"pool.short_unavailable_at_percent", "", "98"}}, []string{`"short_unavailable_at_percent": 98`}, nil},
		{"borrow window", PoolSettings{BorrowWindow: strp("24h")},
			[]Change{{"pool.shared_borrow_window", "", "24h"}}, []string{`"shared_borrow_window": "24h"`}, nil},
		{"borrow window never", PoolSettings{BorrowWindow: strp("0s")},
			[]Change{{"pool.shared_borrow_window", "", "0s"}}, []string{`"shared_borrow_window": "0s"`}, nil},
		{"usage max age", PoolSettings{UsageMaxAge: strp("2m")},
			[]Change{{"pool.usage_max_age", "", "2m"}}, []string{`"usage_max_age": "2m"`}, nil},
		{"key ttl", PoolSettings{KeyTTL: strp("1s")},
			[]Change{{"pool.key_ttl", "", "1s"}}, []string{`"key_ttl": "1s"`}, nil},
		{"picker", PoolSettings{Picker: strp("my-picker --pool auto")},
			[]Change{{"pool.picker.command", "", "my-picker --pool auto"}}, []string{`"picker": {"command":"my-picker --pool auto"}`}, nil},
		{"clearing an unset key changes nothing", PoolSettings{KeyTTL: strp(""), DefaultShort: strp(""), Picker: strp(""), ShortCutoff: strp("")},
			[]Change{}, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := fixtureApp(t, flagConfig, nil, nil)
			ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: tc.s})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ch.Changes, tc.changes) {
				t.Errorf("changes %+v, want %+v", ch.Changes, tc.changes)
			}
			if ch.Written != (len(tc.changes) > 0) || ch.DryRun {
				t.Errorf("written %v dry-run %v", ch.Written, ch.DryRun)
			}
			got := readConfig(t, home)
			if len(tc.changes) == 0 && got != flagConfig {
				t.Errorf("no change wrote the file:\n%s", got)
			}
			for _, w := range append(tc.want, "// pool flags fixture") {
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

// pooledConfig is flagConfig after every setting flag: the start of the
// clearing tests.
const pooledConfig = `{
  "version": 1,
  "default_agent": "claude",
  "profiles": { "personal": { "agent": "claude", "home": "~/.claude-personal", "default": true } },
  "logins": {
    "alice": { "agents": ["claude"], "kind": "oauth-token", "token": "${ALICE_TOKEN}" },
    "bob": { "agents": ["claude"], "kind": "oauth-token", "token": "${BOB_TOKEN}" },
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "auto": { "agents": ["claude"], "kind": "pool", "pool": {
      "own": ["alice", "bob"], "shared": ["carol"],
      "accounts": { "carol": { "enabled": false } },
      "defaults": { "short": 60, "week": 80 },
      "short_unavailable_at_percent": 98, "shared_borrow_window": "24h", "usage_max_age": "2m", "key_ttl": "30s",
      "picker": { "command": "my-picker", "timeout": "5s" }
    } }
  }
}`

// TestPoolFlagsClear: an empty value removes each key, and a block left
// empty (a member entry, accounts, defaults, the picker) goes with it.
func TestPoolFlagsClear(t *testing.T) {
	a, home := fixtureApp(t, pooledConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{
		Enable: []string{"carol"}, DefaultShort: strp(""), DefaultWeek: strp(""), ShortCutoff: strp(""),
		BorrowWindow: strp(""), UsageMaxAge: strp(""), KeyTTL: strp(""), Picker: strp(""),
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"pool.accounts.carol.enabled", "false", "true"},
		{"pool.defaults.short", "60", ""},
		{"pool.defaults.week", "80", ""},
		{"pool.short_unavailable_at_percent", "98", ""},
		{"pool.shared_borrow_window", "24h", ""},
		{"pool.usage_max_age", "2m", ""},
		{"pool.key_ttl", "30s", ""},
		{"pool.picker.command", "my-picker", ""},
	}
	if !slices.Equal(ch.Changes, want) {
		t.Errorf("changes %+v", ch.Changes)
	}
	got := readConfig(t, home)
	for _, k := range []string{"accounts", "defaults", "short_unavailable_at_percent", "shared_borrow_window", "usage_max_age", "key_ttl", "picker"} {
		if strings.Contains(got, `"`+k+`"`) {
			t.Errorf("%s left:\n%s", k, got)
		}
	}
	poolStillValid(t, a, "auto")

	// A member entry keeps its other keys when one is cleared; a picker
	// command change keeps the timeout.
	a, home = fixtureApp(t, pooledConfig, nil, nil)
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{CapShort: []string{"carol=30"}, Picker: strp("other"), DefaultShort: strp("")}}); err != nil {
		t.Fatal(err)
	}
	got = readConfig(t, home)
	for _, w := range []string{`"carol": { "enabled": false, "short": 30 }`, `"command": "other", "timeout": "5s"`, `"defaults": { "week": 80 }`} {
		if !strings.Contains(got, w) {
			t.Errorf("file lacks %q:\n%s", w, got)
		}
	}
	if _, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: PoolSettings{Enable: []string{"carol"}}}); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, home); !strings.Contains(got, `"carol": { "short": 30 }`) {
		t.Errorf("enable kept the cap? \n%s", got)
	}
	poolStillValid(t, a, "auto")
}

// TestPoolFlagsWithMembersAndAccount: settings, members, force and the
// account label change in one edit, in that order of lines; a cap may
// name a member this same command adds.
func TestPoolFlagsWithMembersAndAccount(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{
		ID: "auto", PoolShared: []string{"carol", "dave"}, PoolForce: strp("bob"), Account: strp("team pool"),
		Pool: PoolSettings{CapShort: []string{"dave=20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"pool.shared", "carol", "carol dave"},
		{"pool.force", "", "bob"},
		{"pool.accounts.dave.short", "", "20"},
		{"account", "", "team pool"},
	}
	if !slices.Equal(ch.Changes, want) {
		t.Errorf("changes %+v", ch.Changes)
	}
	if got := readConfig(t, home); !strings.Contains(got, `"force": "bob"`) || !strings.Contains(got, `"account": "team pool"`) {
		t.Errorf("file:\n%s", got)
	}
	poolStillValid(t, a, "auto")
	ch, err = a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", PoolForce: strp(""), Account: strp("")})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Change{{"pool.force", "bob", ""}, {"account", "team pool", ""}}; !slices.Equal(ch.Changes, want) {
		t.Errorf("clear: %+v", ch.Changes)
	}
	if got := readConfig(t, home); !strings.Contains(got, `"force": null`) || strings.Contains(got, `"account"`) {
		t.Errorf("file:\n%s", got)
	}
}

// TestPoolFlagsNoChange: flags that match the file report no change and
// write nothing, --dry-run or not (the owner's final check).
func TestPoolFlagsNoChange(t *testing.T) {
	for _, dry := range []bool{false, true} {
		a, home := fixtureApp(t, flagConfig, nil, nil)
		ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", DryRun: dry, Pool: PoolSettings{
			CapShort: []string{"alice=95"}, CapWeek: []string{"alice=98.0"}, Enable: []string{"bob"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if len(ch.Changes) != 0 || ch.Changes == nil || ch.Written || ch.DryRun != dry || ch.Info.Name != "auto" {
			t.Errorf("dry %v: %+v", dry, ch)
		}
		if readConfig(t, home) != flagConfig {
			t.Errorf("dry %v: the file changed", dry)
		}
	}
}

// TestPoolFlagsDryRun: --dry-run lists the changes of add and edit and
// writes nothing.
func TestPoolFlagsDryRun(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", DryRun: true, Pool: PoolSettings{CapShort: []string{"bob=95"}, KeyTTL: strp("30s")}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Change{{"pool.accounts.bob.short", "", "95"}, {"pool.key_ttl", "", "30s"}}; !slices.Equal(ch.Changes, want) || ch.Written || !ch.DryRun {
		t.Errorf("edit: %+v", ch)
	}
	ach, err := a.LoginAdd(context.Background(), LoginAddOptions{Name: "team", Agents: []agentio.ID{"claude"}, Kind: "pool", DryRun: true,
		PoolOwn: []string{"alice"}, PoolShared: []string{"carol"}, Pool: PoolSettings{CapWeek: []string{"alice=99"}, Disable: []string{"carol"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{"pool.own", "", "alice"}, {"pool.shared", "", "carol"}, {"pool.accounts.alice.week", "", "99"}, {"pool.accounts.carol.enabled", "true", "false"}}
	if !slices.Equal(ach.Changes, want) || ach.Written || !ach.DryRun || ach.Info.Name != "team" || ach.Info.Kind != "pool" {
		t.Errorf("add: %+v", ach)
	}
	if readConfig(t, home) != flagConfig {
		t.Error("--dry-run wrote the file")
	}
}

// TestPoolFlagsAdd: login add --kind pool with every setting flag writes
// them with the new login, and the result loads.
func TestPoolFlagsAdd(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ch, err := a.LoginAdd(context.Background(), LoginAddOptions{Name: "team", Agents: []agentio.ID{"claude"}, Kind: "pool",
		PoolOwn: []string{"alice"}, PoolShared: []string{"carol", "dave"},
		Pool: PoolSettings{
			CapShort: []string{"alice=95"}, CapWeek: []string{"alice=98"}, Disable: []string{"dave"}, Enable: []string{"carol"},
			DefaultShort: strp("40"), DefaultWeek: strp("60"), ShortCutoff: strp("97"), BorrowWindow: strp("12h"),
			UsageMaxAge: strp("90s"), KeyTTL: strp("45s"), Picker: strp("pick-one"),
		}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"pool.own", "", "alice"}, {"pool.shared", "", "carol dave"},
		{"pool.accounts.alice.short", "", "95"}, {"pool.accounts.alice.week", "", "98"}, {"pool.accounts.dave.enabled", "true", "false"},
		{"pool.defaults.short", "", "40"}, {"pool.defaults.week", "", "60"}, {"pool.short_unavailable_at_percent", "", "97"},
		{"pool.shared_borrow_window", "", "12h"}, {"pool.usage_max_age", "", "90s"}, {"pool.key_ttl", "", "45s"}, {"pool.picker.command", "", "pick-one"},
	}
	if !slices.Equal(ch.Changes, want) || !ch.Written {
		t.Errorf("changes %+v", ch.Changes)
	}
	got := readConfig(t, home)
	for _, w := range []string{`"team"`, `"short":95`, `"enabled":false`, `"key_ttl": "45s"`, `"command":"pick-one"`} {
		if !strings.Contains(compact(got), compact(w)) {
			t.Errorf("file lacks %q:\n%s", w, got)
		}
	}
	poolStillValid(t, a, "team")
}

// TestPoolFlagsRefused: every bad value is refused with a message that
// names the flag, and nothing is written.
func TestPoolFlagsRefused(t *testing.T) {
	members := " is not a member of pool auto (members: alice, bob, carol)"
	for _, tc := range []struct {
		name string
		s    PoolSettings
		code apperr.Code
		msg  string
	}{
		{"cap over 100", PoolSettings{CapShort: []string{"alice=101"}}, apperr.CodeUsage, "--pool-cap-short alice=101: 101 is not a number from 0 to 100"},
		{"cap below 0", PoolSettings{CapWeek: []string{"alice=-1"}}, apperr.CodeUsage, "--pool-cap-week alice=-1: -1 is not a number from 0 to 100"},
		{"cap NaN", PoolSettings{CapShort: []string{"alice=NaN"}}, apperr.CodeUsage, "--pool-cap-short alice=NaN: NaN is not a number from 0 to 100"},
		{"cap infinite", PoolSettings{CapShort: []string{"alice=Inf"}}, apperr.CodeUsage, "--pool-cap-short alice=Inf: Inf is not a number from 0 to 100"},
		{"cap word", PoolSettings{CapShort: []string{"alice=high"}}, apperr.CodeUsage, "--pool-cap-short alice=high: high is not a number from 0 to 100"},
		{"cap without =", PoolSettings{CapShort: []string{"alice"}}, apperr.CodeUsage, `--pool-cap-short "alice": use NAME=N (a number from 0 to 100; NAME= removes the member's own cap)`},
		{"cap without a name", PoolSettings{CapWeek: []string{"=5"}}, apperr.CodeUsage, `--pool-cap-week "=5": use NAME=N (a number from 0 to 100; NAME= removes the member's own cap)`},
		{"cap twice", PoolSettings{CapShort: []string{"alice=1", "alice=2"}}, apperr.CodeUsage, "--pool-cap-short alice is given twice"},
		{"cap unknown member", PoolSettings{CapShort: []string{"zed=50"}}, apperr.CodeUsage, "--pool-cap-short zed=50: zed" + members},
		{"cap for a login outside the pool", PoolSettings{CapWeek: []string{"dave=50"}}, apperr.CodeUsage, "--pool-cap-week dave=50: dave" + members},
		{"disable a non-member", PoolSettings{Disable: []string{"dave"}}, apperr.CodeUsage, "--pool-disable dave: dave" + members},
		{"enable a non-member", PoolSettings{Enable: []string{"zed"}}, apperr.CodeUsage, "--pool-enable zed: zed" + members},
		{"enable and disable", PoolSettings{Disable: []string{"bob"}, Enable: []string{"bob"}}, apperr.CodeConflict, "--pool-disable bob and --pool-enable bob both set bob's enabled; pick one"},
		{"default over 100", PoolSettings{DefaultShort: strp("150")}, apperr.CodeUsage, "--pool-default-short 150: 150 is not a number from 0 to 100"},
		{"default word", PoolSettings{DefaultWeek: strp("lots")}, apperr.CodeUsage, "--pool-default-week lots: lots is not a number from 0 to 100"},
		{"cutoff over 100", PoolSettings{ShortCutoff: strp("101")}, apperr.CodeUsage, "--pool-short-cutoff 101: 101 is not a number from 0 to 100"},
		{"bad borrow window", PoolSettings{BorrowWindow: strp("2 days")}, apperr.CodeUsage, `--pool-borrow-window "2 days" is not a duration (write it like "48h", "60s")`},
		{"negative borrow window", PoolSettings{BorrowWindow: strp("-1h")}, apperr.CodeUsage, `--pool-borrow-window "-1h" is negative`},
		{"bad usage max age", PoolSettings{UsageMaxAge: strp("soon")}, apperr.CodeUsage, `--pool-usage-max-age "soon" is not a duration (write it like "48h", "60s")`},
		{"bad key ttl", PoolSettings{KeyTTL: strp("60")}, apperr.CodeUsage, `--pool-key-ttl "60" is not a duration (write it like "48h", "60s")`},
		{"key ttl below 1s", PoolSettings{KeyTTL: strp("1ms")}, apperr.CodeUsage, "--pool-key-ttl 1ms is below 1s"},
		{"blank picker", PoolSettings{Picker: strp("  ")}, apperr.CodeUsage, "--pool-picker: the command is blank; give '' to remove the picker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, home := fixtureApp(t, flagConfig, nil, nil)
			_, err := a.LoginEdit(context.Background(), LoginEditOptions{ID: "auto", Pool: tc.s})
			if e := apperr.As(err); e == nil || e.Code != tc.code || e.Message != tc.msg {
				t.Errorf("edit: %v, want %s %q", err, tc.code, tc.msg)
			}
			if readConfig(t, home) != flagConfig {
				t.Error("a refused edit wrote the file")
			}
		})
	}
}

// TestPoolFlagsRefusedOnAdd: add checks members against its own lists,
// and refuses pool flags on another kind.
func TestPoolFlagsRefusedOnAdd(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ctx := context.Background()
	_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "team", Agents: []agentio.ID{"claude"}, Kind: "pool", PoolOwn: []string{"alice"}, Pool: PoolSettings{CapShort: []string{"bob=50"}}})
	if e := apperr.As(err); e == nil || e.Message != "--pool-cap-short bob=50: bob is not a member of pool team (members: alice)" {
		t.Errorf("member: %v", err)
	}
	_, err = a.LoginAdd(ctx, LoginAddOptions{Name: "team", Agents: []agentio.ID{"claude"}, Kind: "oauth-token", Pool: PoolSettings{KeyTTL: strp("30s")}})
	if e := apperr.As(err); e == nil || e.Message != "the pool flags (--pool-own, --pool-shared, …) are for --kind pool logins" {
		t.Errorf("kind: %v", err)
	}
	_, err = a.LoginAdd(ctx, LoginAddOptions{Name: "team", Agents: []agentio.ID{"claude"}, Kind: "pool", PoolOwn: []string{"alice"}, Pool: PoolSettings{CapShort: []string{"alice=500"}}})
	if e := apperr.As(err); e == nil || e.Message != "--pool-cap-short alice=500: 500 is not a number from 0 to 100" {
		t.Errorf("range: %v", err)
	}
	if readConfig(t, home) != flagConfig {
		t.Error("a refused add wrote the file")
	}
}

// TestDryRunIsForPools: a token login's value is read once, so --dry-run
// is refused on its add and edit; pool flags on a token login are too,
// and a pool edit with no flag says what it takes.
func TestDryRunIsForPools(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ctx := context.Background()
	const msg = "--dry-run is for pool logins; a token login's value is read once and written as given"
	_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "erin", Agents: []agentio.ID{"claude"}, Source: LoginSource{TokenRef: "${ERIN_TOKEN}"}, DryRun: true})
	if e := apperr.As(err); e == nil || e.Message != msg {
		t.Errorf("add: %v", err)
	}
	_, err = a.LoginEdit(ctx, LoginEditOptions{ID: "dave", Account: strp("d"), DryRun: true})
	if e := apperr.As(err); e == nil || e.Message != msg {
		t.Errorf("edit: %v", err)
	}
	_, err = a.LoginEdit(ctx, LoginEditOptions{ID: "dave", Pool: PoolSettings{Disable: []string{"dave"}}})
	if e := apperr.As(err); e == nil || e.Message != "the pool flags (--pool-own, --pool-shared, --pool-force, …) are for pool logins; dave is oauth-token" {
		t.Errorf("pool flag on a token login: %v", err)
	}
	_, err = a.LoginEdit(ctx, LoginEditOptions{ID: "auto"})
	if e := apperr.As(err); e == nil || !strings.HasPrefix(e.Message, "nothing to change: give --pool-own, --pool-shared, --pool-force, --pool-cap-short, --pool-cap-week, --pool-disable, --pool-enable, --pool-default-short, --pool-default-week, --pool-short-cutoff, --pool-borrow-window, --pool-usage-max-age, --pool-key-ttl, --pool-picker, --pool-strategy, --pool-week-relax-window, --pool-max-sessions, --pool-sticky, --pool-sticky-leave-ratio, --pool-default-week-relax, --pool-cap-week-relax, --pool-group-cap-short, --pool-group-cap-week, --pool-group-cap-week-relax or --account") {
		t.Errorf("nothing: %v", err)
	}
	if readConfig(t, home) != flagConfig {
		t.Error("a refused command wrote the file")
	}
}

// TestPoolAddEditGuards: the shape checks of a pool add and edit, each
// refused before any write with its message, and a change the config
// would refuse (an unknown member) refused by the load check.
func TestPoolAddEditGuards(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	ctx := context.Background()
	claude := []agentio.ID{"claude"}
	for _, tc := range []struct {
		name string
		run  func() error
		msg  string
	}{
		{"add with a value", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: claude, Kind: "pool", PoolOwn: []string{"alice"}, Source: LoginSource{TokenRef: "${X}"}})
			return err
		}, "a pool login holds no value; its members do (drop --token-ref)"},
		{"add with a provider", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: claude, Kind: "pool", PoolOwn: []string{"alice"}, Provider: "anthropic"})
			return err
		}, "a pool login takes no --provider; its members do"},
		{"add for two agents", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: []agentio.ID{"claude", "pi"}, Kind: "pool", PoolOwn: []string{"alice"}})
			return err
		}, "a pool login serves exactly one agent: give one --agent"},
		{"add without members", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: claude, Kind: "pool"})
			return err
		}, "a pool needs members: --pool-own NAME or --pool-shared NAME (repeat)"},
		{"add for an unknown agent", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: []agentio.ID{"nope"}, Kind: "pool", PoolOwn: []string{"alice"}})
			return err
		}, "unknown agent nope"},
		{"add an unknown member", func() error {
			_, err := a.LoginAdd(ctx, LoginAddOptions{Name: "p", Agents: claude, Kind: "pool", PoolOwn: []string{"zoe"}})
			return err
		}, "refused: the change would make config.jsonc invalid: unknown login zoe"},
		{"edit a token login into a pool", func() error {
			_, err := a.LoginEdit(ctx, LoginEditOptions{ID: "dave", Kind: "pool"})
			return err
		}, "login dave is oauth-token; a login cannot become a pool in place: add one with agx login add --kind pool"},
		{"edit a pool into a token login", func() error {
			_, err := a.LoginEdit(ctx, LoginEditOptions{ID: "auto", Kind: "oauth-token"})
			return err
		}, "login auto is a pool; it cannot change kind: remove it and add a oauth-token login"},
		{"edit a pool's value", func() error {
			_, err := a.LoginEdit(ctx, LoginEditOptions{ID: "auto", Source: LoginSource{TokenRef: "${X}"}})
			return err
		}, "a pool login holds no value; its members do (drop --token-ref)"},
		{"edit a pool's agents", func() error {
			_, err := a.LoginEdit(ctx, LoginEditOptions{ID: "auto", Agents: claude})
			return err
		}, "a pool login's agent and provider do not change; change its members with --pool-own and --pool-shared"},
		{"edit to an unknown member", func() error {
			_, err := a.LoginEdit(ctx, LoginEditOptions{ID: "auto", PoolOwn: []string{"zoe"}})
			return err
		}, "refused: the change would make config.jsonc invalid: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := apperr.As(tc.run())
			if e == nil || !strings.HasPrefix(e.Message, tc.msg) {
				t.Errorf("error %v, want %q", e, tc.msg)
			}
			if readConfig(t, home) != flagConfig {
				t.Error("a refused command wrote the file")
			}
		})
	}
}
