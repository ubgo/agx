package app

import (
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// TestPoolListAndShow: pool list keeps only pool logins in file order;
// pool show reports each value with its source and refuses a token
// login with the pool commands' wording.
func TestPoolListAndShow(t *testing.T) {
	m, _ := poolMachine(t, nil)
	a := m.app(t)
	rows, err := a.PoolList()
	if err != nil || len(rows) != 1 || rows[0].Name != "auto" || strings.Join(rows[0].Shared, ",") != "carol,derek" || rows[0].Force != "" || rows[0].Picker != "" {
		t.Fatalf("list: %+v %v", rows, err)
	}
	info, err := a.PoolShow(PoolShowOptions{ID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range info.Settings {
		got[s.Key] = s.Value + " " + s.Source
	}
	if got[poolKeyKeyTTL] != "30s config" || got[poolKeyLog] != "~/pool.log config" || got[poolKeyMaxAge] != "60s default" || got[poolKeyBorrow] != "48h default" {
		t.Errorf("settings: %v", got)
	}
	if mb := info.Members[0]; mb.Short != 95 || mb.ShortSource != SourceConfig || mb.EnabledSource != SourceDefault {
		t.Errorf("alice: %+v", mb)
	}
	_, err = a.PoolShow(PoolShowOptions{ID: "alice"})
	if err == nil || apperr.As(err).Code != apperr.CodeUsage || !strings.HasPrefix(err.Error(), "pool show is for pool logins; login alice is oauth-token") || apperr.As(err).Hint != poolListHint {
		t.Errorf("token login: %v", err)
	}
	if _, err := a.PoolShow(PoolShowOptions{ID: "zoe"}); err == nil || apperr.As(err).Code != apperr.CodeNotFound {
		t.Errorf("unknown: %v", err)
	}
}

// TestPoolInfoWithoutBlock: a pool login missing its pool block (config
// check reports it) still shows, with no members and every default.
func TestPoolInfoWithoutBlock(t *testing.T) {
	info := poolInfo("p", config.Login{Kind: config.KindPool})
	if len(info.Members) != 0 || info.Own == nil || info.Shared == nil || len(info.Agents) != 0 || info.Settings[0].Value != poolOffForce {
		t.Errorf("%+v", info)
	}
}

// TestDurationText: whole hours and seconds read as config writes them;
// anything finer keeps time's form.
func TestDurationText(t *testing.T) {
	for d, want := range map[time.Duration]string{48 * time.Hour: "48h", time.Minute: "60s", 10 * time.Second: "10s", 1500 * time.Millisecond: "1.5s"} {
		if got := durationText(d); got != want {
			t.Errorf("%s: %q, want %q", d, got, want)
		}
	}
}

// TestPoolInfoEverySet: a pool that sets every key reports config for
// each, a member cap without its own entry comes from the pool's
// defaults, and an enabled false entry is the member's own.
func TestPoolInfoEverySet(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	off, force := false, "bob"
	lg := config.Login{Kind: config.KindPool, Agents: []agentio.ID{"claude"}, Pool: &config.Pool{
		Own: []string{"alice"}, Shared: []string{"bob"}, Force: &force, SharedBorrowWindow: "24h",
		ShortUnavailableAtPercent: f(95), Defaults: config.PoolCaps{Short: f(40), Week: f(60)},
		Accounts:    map[string]config.PoolAccount{"bob": {Enabled: &off, Week: f(65)}},
		UsageMaxAge: "2m", KeyTTL: "30s", Log: "~/p.log", Picker: &config.PoolPicker{Command: "pk"},
	}}
	info := poolInfo("team", lg)
	if info.Force != "bob" || info.Picker != "pk" || strings.Join(info.Agents, ",") != "claude" {
		t.Errorf("summary: %+v", info.PoolSummary)
	}
	for _, s := range info.Settings {
		want := SourceConfig
		if s.Key == poolKeyPickerTime {
			want = SourceDefault
		}
		if s.Source != want {
			t.Errorf("%s: %s, want %s", s.Key, s.Source, want)
		}
	}
	a, b := info.Members[0], info.Members[1]
	if a.ShortSource != SourcePoolDefaults || a.Short != 40 || b.Enabled || b.EnabledSource != SourceConfig || b.Week != 65 || b.WeekSource != SourceConfig {
		t.Errorf("members: %+v %+v", a, b)
	}
}
