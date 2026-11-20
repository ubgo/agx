package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// TestPoolPresetWrites: a preset writes each of its rule values as an
// ordinary key (comments kept), the file then loads with exactly those
// values, a second run changes nothing, and --dry-run writes nothing.
func TestPoolPresetWrites(t *testing.T) {
	a, home := fixtureApp(t, flagConfig, nil, nil)
	dry, err := a.PoolPreset(context.Background(), PoolPresetOptions{Name: pool.PresetClassic, ID: "auto", DryRun: true})
	if err != nil || dry.Written || !dry.DryRun || len(dry.Changes) != 7 {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	if readConfig(t, home) != flagConfig {
		t.Fatal("--dry-run wrote the file")
	}
	ch, err := a.PoolPreset(context.Background(), PoolPresetOptions{Name: pool.PresetClassic, ID: "auto"})
	if err != nil || !ch.Written {
		t.Fatalf("classic %+v %v", ch, err)
	}
	want := []Change{
		{"pool.defaults.week_relax_max", "", "95"}, {"pool.order", "", "deadline"}, {"pool.max_sessions", "", "0"}, {"pool.sticky", "", "false"},
		{"pool.sticky_leave_ratio", "", "0"}, {"pool.shared_borrow_window", "", "48h"}, {"pool.week_relax_window", "", "0s"},
	}
	if !slices.Equal(ch.Changes, want) {
		t.Errorf("changes %+v", ch.Changes)
	}
	got := readConfig(t, home)
	if !strings.Contains(got, "// alice is mine") || !strings.Contains(got, "// pool flags fixture") {
		t.Errorf("comments lost:\n%s", got)
	}
	cfg, err := a.Config()
	if err != nil {
		t.Fatal(err)
	}
	r, err := cfg.Config.Logins["auto"].Pool.Rules()
	classic, _ := pool.PresetNamed(pool.PresetClassic)
	if err != nil || r.Rules() != classic.Rules {
		t.Errorf("loaded rules %+v, want %+v (%v)", r.Rules(), classic.Rules, err)
	}
	again, err := a.PoolPreset(context.Background(), PoolPresetOptions{Name: pool.PresetClassic, ID: "auto"})
	if err != nil || again.Written || len(again.Changes) != 0 {
		t.Errorf("second run %+v %v", again, err)
	}
	if _, err := a.PoolPreset(context.Background(), PoolPresetOptions{Name: pool.PresetRecommended, ID: "auto"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = a.Config()
	r, _ = cfg.Config.Logins["auto"].Pool.Rules()
	if r.Rules() != pool.DefaultRules() || r.WeekRelaxWindow != 24*time.Hour {
		t.Errorf("recommended %+v", r.Rules())
	}
	info, _ := a.PoolShow(PoolShowOptions{ID: "auto"})
	for _, s := range info.Settings {
		if slices.Contains([]string{poolKeyOrder, poolKeyRelaxWindow, poolKeyRelaxMax, poolKeyMaxSessions, poolKeySticky, poolKeyStickyLeave, poolKeyBorrow}, s.Key) && s.Source != SourcePool {
			t.Errorf("%s after a preset: %s (%s), want pool", s.Key, s.Value, s.Source)
		}
	}
}

// TestPoolPresetRefused: an unknown preset names the presets; a token
// login gets the pool commands' wording.
func TestPoolPresetRefused(t *testing.T) {
	a, _ := fixtureApp(t, flagConfig, nil, nil)
	_, err := a.PoolPreset(context.Background(), PoolPresetOptions{Name: "fast", ID: "auto"})
	if e := apperr.As(err); e == nil || e.Code != apperr.CodeUsage || !strings.Contains(e.Message, `no preset "fast": use recommended or classic`) {
		t.Errorf("unknown preset: %v", err)
	}
	_, err = a.PoolPreset(context.Background(), PoolPresetOptions{Name: pool.PresetClassic, ID: "alice"})
	if err == nil || !strings.HasPrefix(err.Error(), "pool preset is for pool logins") {
		t.Errorf("token login: %v", err)
	}
}
