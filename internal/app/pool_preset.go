package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// agx pool preset: a preset is a named set of rule values (pool.Presets),
// never a mode. The command writes each value into the pool's block as an
// ordinary key, through the same comment-keeping edit as agx login edit
// --pool-…, so after it runs the config shows every value and any one can
// be changed by hand or with agx config set.

// PoolPresetOptions are agx pool preset's arguments and flags.
type PoolPresetOptions struct {
	// Name is the preset (pool.PresetNames).
	Name string
	// ID is the pool login.
	ID string
	// DryRun shows the changes and writes nothing.
	DryRun bool
}

// poolCmdPreset names agx pool preset in a refusal.
const poolCmdPreset = "preset"

// PoolPreset writes preset o.Name's rule values into pool o.ID. Cost W
// (config only): no probe.
func (a *App) PoolPreset(ctx context.Context, o PoolPresetOptions) (LoginChange, error) {
	preset, ok := pool.PresetNamed(o.Name)
	if !ok {
		return LoginChange{}, apperr.Usage(fmt.Sprintf("no preset %q: use %s", o.Name, strings.Join(pool.PresetNames(), " or ")))
	}
	_, lg, err := a.namedLogin(o.ID)
	if err != nil {
		return LoginChange{}, err
	}
	if lg.Kind != config.KindPool {
		return LoginChange{}, apperr.Usage(a.notAPool(poolCmdPreset, o.ID, lg)).WithHint(poolListHint)
	}
	return a.LoginEdit(ctx, LoginEditOptions{ID: o.ID, DryRun: o.DryRun, Pool: presetSettings(preset.Rules)})
}

// presetSettings are a preset's values as the pool setting flags write
// them (the config file's spelling: "24h", "0s", 95).
func presetSettings(r pool.Rules) PoolSettings {
	str := func(v string) *string { return &v }
	return PoolSettings{
		BorrowWindow: str(configDuration(r.BorrowWindow)),
		Rules: PoolRuleSettings{
			Order:            str(string(r.Order)),
			WeekRelaxWindow:  str(configDuration(r.WeekRelaxWindow)),
			DefaultWeekRelax: str(percentText(r.WeekRelaxMax)),
			MaxSessions:      str(strconv.Itoa(r.MaxSessions)),
			Sticky:           str(strconv.FormatBool(r.Sticky)),
			StickyLeaveRatio: str(percentText(r.StickyLeaveRatio)),
		},
	}
}

// configDuration writes a duration as the docs do: "0s" for zero (off),
// else durationText ("24h", "60s").
func configDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	return durationText(d)
}
