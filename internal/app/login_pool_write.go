package app

import (
	"context"
	"fmt"
	"slices"

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
	return o.PoolOwn != nil || o.PoolShared != nil || o.PoolForce != nil
}

// addPool is login add --kind pool: a login with members and no value.
// The members, the agent and every number are checked like a load before
// the file is written (editConfig), so a member that does not exist or
// serves another agent is refused with the config's own message.
func (a *App) addPool(ctx context.Context, o LoginAddOptions) (LoginChange, error) {
	var ch LoginChange
	switch {
	case o.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("%s and %s are for --kind %s logins", flagPoolOwn, flagPoolShared, config.KindPool))
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
	ch.Decisions = append(ch.Decisions, report.Decision{Name: decLoginKind, Value: config.KindPool, Source: "--kind"})
	l := config.Login{Agents: o.Agents, Kind: config.KindPool, Account: o.Account, Pool: &config.Pool{Own: o.PoolOwn, Shared: o.PoolShared}}
	if err := a.editConfig(ctx, func(d *jsonc.Doc) error { return d.Set(config.Pointer("logins", o.Name), l) }); err != nil {
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

// editPool is login edit on a pool: the member lists, the force and the
// account label change; a pool's kind, agent and value do not (a pool
// has no value, and a token login never becomes a pool in place).
func (a *App) editPool(ctx context.Context, o LoginEditOptions, cur config.Login) (LoginChange, error) {
	var ch LoginChange
	switch {
	case cur.Kind != config.KindPool && o.Kind == config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("login %s is %s; a login cannot become a pool in place: add one with agx login add --kind pool", o.ID, cur.Kind))
	case cur.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("%s, %s and %s are for pool logins; %s is %s", flagPoolOwn, flagPoolShared, flagPoolForce, o.ID, cur.Kind))
	case o.Kind != "" && o.Kind != config.KindPool:
		return ch, apperr.Usage(fmt.Sprintf("login %s is a pool; it cannot change kind: remove it and add a %s login", o.ID, o.Kind))
	case len(o.Source.given()) > 0:
		return ch, apperr.Usage(fmt.Sprintf("a pool login holds no value; its members do (drop %s)", o.Source.given()[0]))
	case len(o.Agents) > 0 || (o.Provider != nil && *o.Provider != ""):
		return ch, apperr.Usage("a pool login's agent and provider do not change; change its members with " + flagPoolOwn + " and " + flagPoolShared)
	}
	p := config.Pool{}
	if cur.Pool != nil {
		p = *cur.Pool
	}
	changed := false
	err := a.editConfig(ctx, func(d *jsonc.Doc) error {
		ptr := func(k string) string { return config.Pointer("logins", o.ID, keyPool, k) }
		for _, f := range []struct {
			key       string
			old, want []string
		}{{keyPoolOwn, p.Own, o.PoolOwn}, {keyPoolShared, p.Shared, o.PoolShared}} {
			if f.want == nil || slices.Equal(f.old, f.want) {
				continue
			}
			changed = true
			if err := d.Set(ptr(f.key), f.want); err != nil {
				return err
			}
		}
		if o.PoolForce != nil {
			old := ""
			if p.Force != nil {
				old = *p.Force
			}
			if old != *o.PoolForce {
				changed = true
				var v *string
				if *o.PoolForce != "" {
					v = o.PoolForce
				}
				if err := d.Set(ptr(keyPoolForce), v); err != nil {
					return err
				}
			}
		}
		if o.Account != nil && *o.Account != cur.Account {
			changed = true
			if err := setOrDelete(d, config.Pointer("logins", o.ID, keyAccount), true, *o.Account); err != nil {
				return err
			}
		}
		if !changed {
			return apperr.Usage("nothing to change: give " + flagPoolOwn + ", " + flagPoolShared + ", " + flagPoolForce + " or --account")
		}
		return nil
	})
	if err != nil {
		return ch, err
	}
	return a.changed(ch, o.ID)
}
