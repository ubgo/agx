package app

import (
	"slices"
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
	if got[poolKeyKeyTTL] != "30s pool" || got[poolKeyLog] != "~/pool.log pool" || got[poolKeyMaxAge] != "60s default" || got[poolKeyBorrow] != "48h default" ||
		got[poolKeyProbeModels] != "none default" {
		t.Errorf("settings: %v", got)
	}
	if mb := info.Members[0]; mb.Short != 95 || mb.ShortSource != SourceMember || mb.EnabledSource != SourceDefault {
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

// TestPoolInfoEverySet: a pool that sets every key reports pool for
// each, a shared member cap without its own entry comes from its
// group's block, else the pool's defaults, an own member's from own_caps
// else agx's 95/98 (never the pool's defaults), and an enabled false
// entry is the member's own.
func TestPoolInfoEverySet(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	off, force := false, "bob"
	lg := config.Login{Kind: config.KindPool, Agents: []agentio.ID{"claude"}, Pool: &config.Pool{
		Own: []string{"alice"}, Shared: []string{"bob"}, Force: &force, SharedBorrowWindow: "24h",
		ShortUnavailableAtPercent: f(95), Defaults: config.PoolCaps{Short: f(40), Week: f(60), WeekRelaxMax: f(90)},
		Order: "deadline", WeekRelaxWindow: "12h", MaxSessions: intp(2), Sticky: boolp(false), StickyLeaveRatio: f(4),
		DeadlineWindow: "6h", DeadlineMinRoom: f(5),
		OwnCaps: config.PoolCaps{Week: f(97)}, SharedCaps: config.PoolCaps{Short: f(30)},
		Accounts:    map[string]config.PoolAccount{"bob": {Enabled: &off, Week: f(65), Capacity: f(5)}},
		UsageMaxAge: "2m", KeyTTL: "30s", Log: "~/p.log", Picker: &config.PoolPicker{Command: "pk"}, Timezone: "UTC",
		ProbeModels: []string{"claude-fable-5-2", "opus"},
	}}
	info := poolInfo("team", lg)
	if !slices.Contains(info.Settings, PoolSetting{Key: poolKeyProbeModels, Value: "claude-fable-5-2,opus", Source: SourcePool}) {
		t.Errorf("probe_models: %+v", info.Settings)
	}
	if info.Force != "bob" || info.Picker != "pk" || strings.Join(info.Agents, ",") != "claude" {
		t.Errorf("summary: %+v", info.PoolSummary)
	}
	for _, s := range info.Settings {
		want := SourcePool
		switch s.Key {
		case poolKeyPickerTime, poolKeyOwnShort:
			want = SourceDefault
		case poolKeyOwnWeek:
			want = SourceGroup
		}
		if s.Source != want {
			t.Errorf("%s: %s, want %s", s.Key, s.Source, want)
		}
	}
	a, b := info.Members[0], info.Members[1]
	if a.Short != 95 || a.ShortSource != SourceDefault || a.Week != 97 || a.WeekSource != SourceGroup || a.WeekRelaxMax != 90 || a.WeekRelaxMaxSource != SourcePool ||
		a.Capacity != 1 || a.CapacitySource != SourceDefault {
		t.Errorf("own member: %+v", a)
	}
	if b.Enabled || b.EnabledSource != SourceMember || b.Week != 65 || b.WeekSource != SourceMember || b.Short != 30 || b.ShortSource != SourceGroup ||
		b.Capacity != 5 || b.CapacitySource != SourceMember {
		t.Errorf("shared member: %+v", b)
	}
	lg.Pool.SharedCaps = config.PoolCaps{}
	if c := poolInfo("team", lg).Members[1]; c.Short != 40 || c.ShortSource != SourcePool {
		t.Errorf("shared member on the pool's defaults: %+v", c)
	}
}

// intp and boolp are pointers to v.
func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }
