package app

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/login"
	"github.com/ubgo/agx/pool"
)

// Per-model windows (docs/guides/login-pool.md "Per-model weekly
// limits"). A plan can limit one model on its own, and the vendor
// reports that window only when the probe asks with that model, so a
// pool probes with the model its sessions use. Which model, first set
// wins:
//
//  1. the run's model: the agx --model it was started with, else its
//     profile's model, else the model the agent's own settings choose
//     (Claude's "model", which /model writes), recorded in the run's
//     live.jsonc at launch; agx pool pick --model stands in for it;
//  2. the pool's probe_models (every one is probed and applies);
//  3. the pool agent's default profile's model, else that home's
//     settings model;
//  4. none: the agent's default probe model, which shows only the
//     all-models windows.
//
// The all-models windows of every probe apply to every session; a
// per-model window applies only to the sessions of its model.

// Where the probe model comes from (PoolReport.ProbeModelsSource).
const (
	modelSrcFlag    = "--model"
	modelSrcRun     = "run"
	modelSrcPool    = "pool probe_models"
	modelSrcProfile = "profile "
	modelSrcAgent   = "agent settings"
	modelSrcNone    = "default probe model"
)

// poolModels are the models a pick probes with and applies, and where
// they come from (the rule above). An empty list is the default probe
// model.
func (a *App) poolModels(c config.Config, p *config.Pool, ag agentio.Agent, o PoolPickOptions) ([]string, string) {
	switch {
	case o.Model != "":
		src := modelSrcFlag
		if o.RunID != "" {
			src = modelSrcRun
		}
		return []string{o.Model}, src
	case len(p.ProbeModels) > 0:
		return slices.Clone(p.ProbeModels), modelSrcPool
	}
	if m, src := a.agentDefaultModel(c, ag); m != "" {
		return []string{m}, src
	}
	return nil, modelSrcNone
}

// agentDefaultModel is the model of agent ag's default profile, else
// the model that profile's home settings choose; "" when neither says.
func (a *App) agentDefaultModel(c config.Config, ag agentio.Agent) (string, string) {
	names := make([]string, 0, len(c.Profiles))
	for name, p := range c.Profiles {
		if p.Agent == ag.ID() && p.Default {
			names = append(names, name)
		}
	}
	if len(names) != 1 {
		return "", ""
	}
	p := c.Profiles[names[0]]
	if p.Model != "" {
		return p.Model, modelSrcProfile + names[0]
	}
	cr, ok := agentio.ConfigOf(ag)
	if !ok || p.Home == "" {
		return "", ""
	}
	dir := filepath.Clean(config.ExpandPath(p.Home, a.d.Lookup, a.d.Host.UserHome))
	if d, err := cr.Defaults(ag.HomeAt(dir), ""); err == nil && d.Model != "" {
		return d.Model, modelSrcAgent
	}
	return "", ""
}

// runModel is the model a launch's session uses, as recorded for its
// key requests: --model, else the profile's model, else the agent's own
// settings for that home and folder; "" when none says.
func runModel(o LaunchOptions, res resolve.Result) (string, string) {
	switch {
	case o.Model != "":
		return o.Model, modelSrcFlag
	case res.Profile.Model != "":
		return res.Profile.Model, modelSrcProfile + res.ProfileName
	}
	if cr, ok := agentio.ConfigOf(res.Agent); ok && res.Home.Dir() != "" {
		if d, err := cr.Defaults(res.Home, o.Dir); err == nil && d.Model != "" {
			return d.Model, modelSrcAgent
		}
	}
	return "", ""
}

// modelKey is how a model is keyed in the usage cache: lowercase, a
// "[…]" suffix (a context size, not a model) dropped.
func modelKey(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	if i := strings.IndexByte(m, '['); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	return m
}

// modelFamily is a model's short name for a window label: "fable" for
// claude-fable-5-2 or the alias fable; "" for the default probe model.
func modelFamily(m string) string {
	m = strings.TrimPrefix(modelKey(m), "claude-")
	family, _, _ := strings.Cut(m, "-")
	return family
}

// scopedName labels a per-model window: "fable week", "fable 1d", or
// with the default probe model the vendor's scope ("oi week").
func scopedName(model string, w login.UsageCacheWindow) string {
	who := modelFamily(model)
	if who == "" {
		who = w.Scope
	}
	if w.Class == agentio.WindowWeek.String() {
		return who + " week"
	}
	return who + " " + cmp.Or(w.ID, w.Class)
}

// probeKeys are the cache keys a pick needs per-model windows for: one
// per model, or the default probe model.
func probeKeys(models []string) []string {
	if len(models) == 0 {
		return []string{""}
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		if k := modelKey(m); !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// freshFor reports whether entry e answers for model key k: probed with
// k (or with k's windows kept from such a probe) within maxAge of now.
func freshFor(e login.UsageEntry, k string, maxAge time.Duration, now time.Time) bool {
	young := func(t time.Time) bool { return now.Sub(t) < maxAge && !t.After(now) }
	if !young(e.ProbedAt) {
		return false
	}
	if mw, ok := e.Models[k]; ok {
		return young(mw.ProbedAt)
	}
	return modelKey(e.Model) == k
}

// readingOf turns a cached entry into the rules' reading for the models
// in use: its status ("limit-reached" blocks by itself, as in the usage
// table), its first all-usage short and weekly windows, and the
// per-model windows of those models (each an extra weekly cap).
func readingOf(e login.UsageEntry, models []string) pool.Reading {
	r := pool.Reading{Status: e.Status}
	for _, w := range e.Windows {
		if w.Scope != "" {
			continue
		}
		win := &pool.Window{Percent: w.Percent, ResetsAt: w.ResetsAt}
		switch w.Class {
		case agentio.WindowShort.String():
			if r.Short == nil {
				r.Short = win
			}
		case agentio.WindowWeek.String():
			if r.Week == nil {
				r.Week = win
			}
		}
	}
	for _, k := range probeKeys(models) {
		ws := []login.UsageCacheWindow{}
		if mw, ok := e.Models[k]; ok {
			ws = mw.Windows
		} else if modelKey(e.Model) == k {
			ws = login.ScopedOnly(e.Windows)
		}
		for _, w := range ws {
			r.Scoped = append(r.Scoped, pool.ScopedWindow{Name: scopedName(k, w), Percent: w.Percent, ResetsAt: w.ResetsAt,
				Rejected: w.Status == agentio.WindowRejected || (w.Binding && w.Percent >= percentFull)})
		}
	}
	return r
}

// probeWithModel probes one token with model ("" = the agent's default
// probe model). A prober that cannot take a model, or a model the vendor
// refuses, falls back to the default probe and says so in note.
func probeWithModel(ctx context.Context, pr agentio.TokenProber, cred agentio.TokenCredential, model string) (agentio.Usage, string, error) {
	if model == "" {
		u, err := pr.TokenUsage(ctx, cred)
		return u, "", err
	}
	mp, ok := pr.(agentio.ModelTokenProber)
	if !ok {
		u, err := pr.TokenUsage(ctx, cred)
		return u, "this agent probes without a model; " + model + " windows are not read", err
	}
	u, err := mp.TokenUsageModel(ctx, cred, model)
	if errors.Is(err, agentio.ErrModelRefused) {
		u, err = pr.TokenUsage(ctx, cred)
		return u, "the vendor refused probe model " + model + "; probed with the default model (fix probe_models or --model)", err
	}
	return u, "", err
}
