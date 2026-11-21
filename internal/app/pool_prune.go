package app

import (
	"context"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// agx pool prune: an override that has ended is ignored by every pick,
// but stays in config.jsonc until removed. prune removes each one through
// the same comment-keeping edit as agx login edit --pool-clear-override.

// PoolPruneOptions are agx pool prune's flags.
type PoolPruneOptions struct {
	// ID is the pool login.
	ID string
	// DryRun shows the changes and writes nothing.
	DryRun bool
}

// poolCmdPrune names agx pool prune in a refusal.
const poolCmdPrune = "prune"

// PoolPrune removes pool o.ID's overrides that have ended. Cost W
// (config only). Nothing ended = no change and no write.
func (a *App) PoolPrune(ctx context.Context, o PoolPruneOptions) (LoginChange, error) {
	_, lg, err := a.namedLogin(o.ID)
	if err != nil {
		return LoginChange{}, err
	}
	if lg.Kind != config.KindPool {
		return LoginChange{}, apperr.Usage(a.notAPool(poolCmdPrune, o.ID, lg)).WithHint(poolListHint)
	}
	p, now := poolOf(lg), a.Now()
	ended := []string{}
	for _, m := range p.Members() {
		ov := p.Accounts[m].Override
		if ov == nil {
			continue
		}
		if r, err := ov.Rule(m); err == nil && !r.Active(now) {
			ended = append(ended, m)
		}
	}
	return a.LoginEdit(ctx, LoginEditOptions{ID: o.ID, DryRun: o.DryRun, Pool: PoolSettings{Timed: PoolTimedSettings{ClearOverride: ended}}})
}
