package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
)

// Whoami is agx whoami's data (spec 13.1): the current session from the
// AGX_RUN_* variables a launch set, and the per-terminal logins
// (AGX_LOGIN, AGX_LOGIN_<AGENT>) set in this shell.
type Whoami struct {
	// InSession is true when AGX_RUN_ID is set (agx runs inside an agent
	// it started).
	InSession bool   `json:"in_session"`
	RunID     string `json:"run_id"`
	Agent     string `json:"agent"`
	Profile   string `json:"profile"`
	Login     string `json:"login"`
	Home      string `json:"home"`
	// RunDir is the run folder when it exists in this config dir.
	RunDir string `json:"run_dir"`
	// LoginKind is the run login's kind in config ("pool", "oauth-token",
	// …); "" when it is not a named login.
	LoginKind string `json:"login_kind"`
	// InUse is the login whose key the agent got last (live runs; for a
	// pool, the member): what the session is on now. "" = not asked yet.
	InUse string `json:"in_use"`
	// Next is the login the agent gets at its next key request when that
	// differs from InUse: a pin (agx live use), a run's --pool-force, or
	// the member the pool picks now from the login-usage cache. "" = no
	// switch pending.
	Next string `json:"next"`
	// Only is the member a pool run is pinned to with its caps enforced
	// (--pool-only); "" = none.
	Only string `json:"only"`
	// Live is the live-login state of the run; nil = not a live run.
	Live *live.State `json:"live"`
	// TerminalLogins are the AGX_LOGIN* variables set, sorted by name.
	TerminalLogins []TerminalLogin `json:"terminal_logins"`
}

// TerminalLogin is one AGX_LOGIN* variable.
type TerminalLogin struct {
	Variable string `json:"variable"`
	Login    string `json:"login"`
}

// Whoami reads the session variables. Cost R: env plus, inside a
// session, the run folder's live.jsonc. The AGX_RUN_* values are outputs
// agx wrote at launch; reading them here chooses nothing (spec 11).
func (a *App) Whoami() (Whoami, error) {
	w := Whoami{TerminalLogins: a.terminalLogins()}
	get := func(name string) string {
		v, _ := a.d.Lookup(name)
		return v
	}
	w.RunID = get(run.EnvRunID)
	w.InSession = w.RunID != ""
	w.Agent, w.Profile, w.Login, w.Home = get(run.EnvRunAgent), get(run.EnvRunProfile), get(run.EnvRunLogin), get(run.EnvRunHome)
	if !w.InSession || !run.ValidID(w.RunID) {
		return w, nil
	}
	dir := run.Dir(a.paths.Runs, w.RunID)
	if !statDir(dir) {
		return w, nil
	}
	w.RunDir = dir
	st, err := live.Read(dir)
	switch {
	case errors.Is(err, live.ErrNotLive):
	case err != nil:
		return w, liveErr(err)
	default:
		w.Live = &st
	}
	if cfg, err := a.Config(); err == nil {
		lg, ok := cfg.Config.Logins[w.Login]
		if ok {
			w.LoginKind = lg.Kind
		}
		if w.Live != nil {
			w.InUse, w.Only = w.Live.InUse, w.Live.PoolOnly
			if next := a.nextLogin(cfg.Config.Logins, *w.Live); next != w.InUse {
				w.Next = next
			}
		}
	}
	return w, nil
}

// nextLogin is the login a live run's next key request gets: the pinned
// or start login when it is a token login; for a pool, the run's
// --pool-force, else the member the pool picks now from the login-usage
// cache (cost R: nothing is probed). "" when it cannot be told.
func (a *App) nextLogin(logins map[string]config.Login, st live.State) string {
	want := st.Want()
	lg, ok := logins[want]
	if !ok || lg.Kind != config.KindPool {
		return want
	}
	if st.PoolForce != "" {
		return st.PoolForce
	}
	only := stateOnly(st)
	rep, err := a.poolPick(context.Background(), want, lg, PoolPickOptions{Cached: true, Only: only})
	if err != nil || rep.Selected == nil {
		return ""
	}
	return rep.Selected.Name
}

// terminalLogins lists AGX_LOGIN and every AGX_LOGIN_<AGENT> set to a
// non-empty value in agx's environment.
func (a *App) terminalLogins() []TerminalLogin {
	out := []TerminalLogin{}
	for _, kv := range a.d.Host.Env {
		k, v, _ := strings.Cut(kv, "=")
		if v == "" || (k != resolve.EnvLogin && !strings.HasPrefix(k, resolve.EnvLogin+"_")) {
			continue
		}
		out = append(out, TerminalLogin{Variable: k, Login: v})
	}
	slices.SortFunc(out, func(x, y TerminalLogin) int { return strings.Compare(x.Variable, y.Variable) })
	return out
}
