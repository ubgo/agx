package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
)

// Pool flag names (login add and edit), named in messages.
const (
	flagPoolOwn    = "--pool-own"
	flagPoolShared = "--pool-shared"
	flagPoolForce  = "--pool-force"
)

// poolGiven reports a pool flag on login edit.
func (o LoginEditOptions) poolGiven() bool {
	return o.PoolOwn != nil || o.PoolShared != nil || o.PoolForce != nil || o.Pool.given()
}

// poolFlagsText names the pool flags in "nothing to change" and kind
// messages.
const poolFlagsText = flagPoolOwn + ", " + flagPoolShared + ", " + flagPoolForce + ", " + flagPoolCapShort + ", " + flagPoolCapWeek + ", " +
	flagPoolDisable + ", " + flagPoolEnable + ", " + flagPoolDefaultShort + ", " + flagPoolDefaultWeek + ", " + flagPoolShortCutoff + ", " +
	flagPoolBorrowWindow + ", " + flagPoolUsageMaxAge + ", " + flagPoolKeyTTL + ", " + flagPoolPicker + ", " + poolStrategyFlagsText

// addPool is login add --kind pool: a login with members and no value,
// plus any pool setting flags. The members, the agent and every number
// are checked like a load before the file is written (editConfig), so a
// member that does not exist or serves another agent is refused with the
// config's own message. --dry-run shows the keys it would write.
func (a *App) addPool(ctx context.Context, o LoginAddOptions, cfg *config.Loaded) (LoginChange, error) {
	var ch LoginChange
	switch {
	case o.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("the pool flags (%s, %s, …) are for --kind %s logins", flagPoolOwn, flagPoolShared, config.KindPool))
	case len(o.Source.given()) > 0:
		return ch, apperr.Usage(fmt.Sprintf("a %s login holds no value; its members do (drop %s)", config.KindPool, o.Source.given()[0]))
	case o.Provider != "":
		return ch, apperr.Usage("a pool login takes no --provider; its members do")
	case len(o.Agents) != 1:
		return ch, apperr.Usage("a pool login serves exactly one agent: give one --agent")
	case len(o.PoolOwn)+len(o.PoolShared) == 0:
		return ch, apperr.Usage(fmt.Sprintf("a pool needs members: %s NAME or %s NAME (repeat)", flagPoolOwn, flagPoolShared))
	}
	if err := a.poolAgent(o.Agents[0]); err != nil {
		return ch, err
	}
	plan := poolPlan{login: o.Name}
	plan.change(keyPoolOwn, absent, strings.Join(o.PoolOwn, " "))
	plan.change(keyPoolShared, absent, strings.Join(o.PoolShared, " "))
	if err := plan.planSettings(config.Pool{}, slices.Concat(o.PoolOwn, o.PoolShared), o.Pool); err != nil {
		return ch, err
	}
	ch.Decisions = append(ch.Decisions, report.Decision{Name: decLoginKind, Value: config.KindPool, Source: "--kind"})
	ch.Changes = plan.changes
	l := config.Login{Agents: o.Agents, Kind: config.KindPool, Account: o.Account, Pool: &config.Pool{Own: o.PoolOwn, Shared: o.PoolShared}}
	if o.DryRun {
		ch.Info, ch.DryRun = a.loginInfo(cfg.Config, o.Name, l), true
		return ch, nil
	}
	err := a.editConfig(ctx, func(d *jsonc.Doc) error {
		if err := d.Set(config.Pointer("logins", o.Name), l); err != nil {
			return err
		}
		return applyEdits(d, plan.edits)
	})
	if err != nil {
		return ch, err
	}
	return a.changed(ch, o.Name)
}

// poolAgent refuses an agent that cannot use a pool, with agentio's
// typed reason (exit 4), and an unknown one.
func (a *App) poolAgent(id agentio.ID) error {
	if _, err := a.loginAgent(id); err != nil {
		return err
	}
	if reason := agents.PoolReason(id); reason != "" {
		return apperr.Unsupported(fmt.Sprintf("%s cannot use a login pool (%s)", id, reason))
	}
	return nil
}

// editPool is login edit on a pool: the member lists, the force, every
// pool setting and the account label change; a pool's kind, agent and
// value do not (a pool has no value, and a token login never becomes a
// pool in place). Each changed key gets a before/after line; --dry-run
// stops there, and flags that match the file change nothing (no write).
func (a *App) editPool(ctx context.Context, o LoginEditOptions, cfg *config.Loaded, cur config.Login) (LoginChange, error) {
	var ch LoginChange
	switch {
	case cur.Kind != config.KindPool && o.Kind == config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("login %s is %s; a login cannot become a pool in place: add one with agx login add --kind pool", o.ID, cur.Kind))
	case cur.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("the pool flags (%s, %s, %s, …) are for pool logins; %s is %s", flagPoolOwn, flagPoolShared, flagPoolForce, o.ID, cur.Kind))
	case o.Kind != "" && o.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("login %s is a pool; it cannot change kind: remove it and add a %s login", o.ID, o.Kind))
	case len(o.Source.given()) > 0:
		return ch, apperr.Usage(fmt.Sprintf("a pool login holds no value; its members do (drop %s)", o.Source.given()[0]))
	case len(o.Agents) > 0 || (o.Provider != nil && *o.Provider != ""):
		return ch, apperr.Usage("a pool login's agent and provider do not change; change its members with " + flagPoolOwn + " and " + flagPoolShared)
	case !o.poolGiven() && o.Account == nil:
		return ch, apperr.Usage("nothing to change: give " + poolFlagsText + " or --account")
	}
	p := config.Pool{}
	if cur.Pool != nil {
		p = *cur.Pool
	}
	own, shared := p.Own, p.Shared
	if o.PoolOwn != nil {
		own = o.PoolOwn
	}
	if o.PoolShared != nil {
		shared = o.PoolShared
	}
	plan := poolPlan{login: o.ID}
	plan.planMembers(p, o.PoolOwn, o.PoolShared, o.PoolForce)
	if err := plan.planSettings(p, slices.Concat(own, shared), o.Pool); err != nil {
		return ch, err
	}
	if o.Account != nil && *o.Account != cur.Account {
		plan.changes = append(plan.changes, Change{Key: keyAccount, From: cur.Account, To: *o.Account})
		plan.edits = append(plan.edits, setOrDeleteEdit([]string{"logins", o.ID, keyAccount}, *o.Account))
	}
	ch.Changes = plan.changes
	if ch.Changes == nil {
		ch.Changes = []Change{}
	}
	if len(plan.changes) == 0 || o.DryRun {
		ch.Info, ch.DryRun = a.loginInfo(cfg.Config, o.ID, cur), o.DryRun
		return ch, nil
	}
	if err := a.editConfig(ctx, func(d *jsonc.Doc) error { return applyEdits(d, plan.edits) }); err != nil {
		return ch, err
	}
	return a.changed(ch, o.ID)
}
