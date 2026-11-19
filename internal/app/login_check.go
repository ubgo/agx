package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/login"
)

// CheckStatus is the outcome of one step of login check.
type CheckStatus string

// Check statuses.
const (
	// CheckOK is a step that succeeded.
	CheckOK CheckStatus = "ok"
	// CheckFailed is a step that failed; Detail says why (never a value).
	CheckFailed CheckStatus = "failed"
	// CheckSkipped is a step not run; Detail says why.
	CheckSkipped CheckStatus = "skipped"
)

// CheckStatusValues is the canonical list.
func CheckStatusValues() []CheckStatus { return []CheckStatus{CheckOK, CheckFailed, CheckSkipped} }

// Skip and success details of login check.
const (
	detailHelper  = "the agent runs the helper itself; agx never does"
	detailNoProbe = "--no-probe"
	// detailPool: a pool holds no token; its members are checked as
	// logins of their own, and agx pool pick reads their usage.
	detailPool     = "a pool: its members are checked as logins; agx pool pick shows their usage"
	detailPerToken = "billed per token: no plan windows"
)

// LoginCheckOptions are agx login check's flags.
type LoginCheckOptions struct {
	// IDs are the logins to check; empty = every login.
	IDs []string
	// NoProbe resolves only (cost X), never probes (cost Q).
	NoProbe bool
}

// CheckWindow is one usage window a probe reported.
type CheckWindow struct {
	// Label is the window name ("5h", "7d" or the vendor label).
	Label string `json:"label"`
	// Percent used, 0-100.
	Percent float64 `json:"percent"`
	// ResetsAt is when it resets; nil when unknown.
	ResetsAt *time.Time `json:"resets_at"`
}

// LoginCheckRow is one login's result.
type LoginCheckRow struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	// Resolve is the reference/stored-value resolution (cost X for a
	// command or plugin reference).
	Resolve CheckStatus `json:"resolve"`
	// Probe is the quota probe (cost Q); skipped with --no-probe, for a
	// helper, or when no agent of the login can probe.
	Probe CheckStatus `json:"probe"`
	// Agent is the agent whose prober was used; "" when not probed.
	Agent agentio.ID `json:"agent"`
	// Detail is the failure, the skip reason or a usage summary.
	Detail string `json:"detail"`
	// Windows are the probed usage windows.
	Windows []CheckWindow `json:"windows"`

	// err is the failure behind a failed row, for the command's error.
	err error
}

// LoginCheck resolves each login and, unless NoProbe, probes it once with
// the first of its agents that has a token prober. It never prints a
// value. The error is nil when every row passed; one login's own error
// when only one was checked; else a partial (exit 3) or failure (exit 1)
// naming the failed logins. Rows are returned either way.
func (a *App) LoginCheck(ctx context.Context, o LoginCheckOptions) ([]LoginCheckRow, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, err
	}
	names := o.IDs
	if len(names) == 0 {
		names = cfg.LoginOrder()
	}
	for _, n := range names {
		if _, ok := cfg.Config.Logins[n]; !ok {
			return nil, apperr.Failure(apperr.CodeNotFound, "unknown login "+n).WithHint("agx login list")
		}
	}
	rows := make([]LoginCheckRow, 0, len(names))
	for _, n := range names {
		if err := ctxErr(ctx); err != nil {
			return rows, err
		}
		rows = append(rows, a.checkOne(ctx, n, cfg.Config.Logins[n], o.NoProbe))
	}
	return rows, checkError(rows)
}

// checkOne resolves and probes one login.
func (a *App) checkOne(ctx context.Context, name string, l config.Login, noProbe bool) LoginCheckRow {
	row := LoginCheckRow{Name: name, Kind: l.Kind, Source: a.loginInfo(config.Config{}, name, l).Source, Windows: []CheckWindow{}}
	if l.Kind == config.KindAPIKeyHelper {
		row.Resolve, row.Probe, row.Detail = CheckSkipped, CheckSkipped, detailHelper
		return row
	}
	if l.Kind == config.KindPool {
		row.Resolve, row.Probe, row.Detail = CheckSkipped, CheckSkipped, detailPool
		return row
	}
	cred, err := login.Credential(ctx, a.exp, a.paths, name, l)
	if err != nil {
		row.Resolve, row.Probe, row.Detail, row.err = CheckFailed, CheckSkipped, apperr.As(err).Message, err
		return row
	}
	row.Resolve = CheckOK
	if noProbe {
		row.Probe, row.Detail = CheckSkipped, detailNoProbe
		return row
	}
	a.probe(ctx, &row, l, cred)
	return row
}

// probe runs the first available token prober of the login's agents
// (cost Q: spends a few tokens; once, never in a loop).
func (a *App) probe(ctx context.Context, row *LoginCheckRow, l config.Login, cred agentio.TokenCredential) {
	var reasons []string
	for _, id := range l.Agents {
		ag, err := a.loginAgent(id)
		if err != nil {
			reasons = append(reasons, apperr.As(err).Message)
			continue
		}
		tp, ok := agentio.TokenProberOf(ag)
		if !ok {
			reason, _ := agentio.Unsupported(ag, agentio.CapTokenProber)
			if reason == "" {
				reason = "no token probe"
			}
			reasons = append(reasons, string(id)+": "+reason)
			continue
		}
		row.Agent = id
		pctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
		u, err := tp.TokenUsage(pctx, cred)
		cancel()
		a.probeResult(row, u, err)
		return
	}
	row.Probe, row.Detail = CheckSkipped, strings.Join(reasons, "; ")
}

// probeResult maps a probe answer onto the row.
func (a *App) probeResult(row *LoginCheckRow, u agentio.Usage, err error) {
	label := "login " + row.Name
	switch {
	case errors.Is(err, agentio.ErrBilledPerToken):
		row.Probe, row.Detail = CheckOK, detailPerToken
		return
	case errors.Is(err, agentio.ErrUnauthorized):
		row.Probe, row.Detail = CheckFailed, label+": token rejected by the server"
		row.err = apperr.Wrap(err, apperr.CodeSecret, apperr.ClassFailure, row.Detail)
		return
	case err != nil:
		row.Probe, row.Detail = CheckFailed, label+": probe failed: "+apperr.Text(err)
		row.err = apperr.Wrap(err, apperr.CodeNetwork, apperr.ClassFailure, row.Detail)
		return
	}
	row.Probe = CheckOK
	var parts []string
	for _, w := range u.Windows {
		lbl := w.Label
		if lbl == "" {
			lbl = w.VendorKind
		}
		row.Windows = append(row.Windows, CheckWindow{Label: lbl, Percent: w.Percent, ResetsAt: w.ResetsAt})
		parts = append(parts, fmt.Sprintf("%s %.0f%%", lbl, w.Percent))
	}
	row.Detail = strings.Join(parts, ", ")
}

// checkError folds failed rows into the command's error.
func checkError(rows []LoginCheckRow) error {
	var failed []string
	var first error
	for _, r := range rows {
		if r.err != nil {
			failed = append(failed, r.Name)
			if first == nil {
				first = r.err
			}
		}
	}
	switch {
	case len(failed) == 0:
		return nil
	case len(rows) == 1:
		return first
	case len(failed) < len(rows):
		return apperr.New(apperr.CodePartial, apperr.ClassPartial,
			fmt.Sprintf("%d of %d logins failed: %s", len(failed), len(rows), strings.Join(failed, ", ")))
	}
	e := apperr.As(first)
	return apperr.Wrap(first, e.Code, apperr.ClassFailure, fmt.Sprintf("every login failed: %s (first: %s)", strings.Join(failed, ", "), e.Message))
}
