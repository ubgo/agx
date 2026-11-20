package app

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/login"
)

// Reading one row: the usage (live, cached or probed), the status it
// comes to, and the --plan details. Every vendor fact comes through an
// agentio accessor; a missing capability is agentio's own reason.

// Fallback reasons when an agent declares none.
const (
	reasonNoUsage   = "usage not modelled"
	reasonNoProbe   = "token probe not modelled"
	reasonPerToken  = "every profile on this home bills per token"
	reasonNoReading = "no reading on disk yet; run the agent once, or drop --cached"
)

// readJob reads one row. It never returns an error: a failure is the
// row's status and reason, so the other rows still show.
func (a *App) readJob(ctx context.Context, j usageJob, o UsageOptions) UsageRow {
	r := j.row
	r.Source = SourceNone
	switch {
	case j.row.Kind == RowLogin:
		a.probeLogin(ctx, j, &r)
	case j.missing:
		r.Status, r.Reason = StatusHomeMissing, "home "+r.Home+" does not exist"
	case j.perToken:
		r.Status, r.Reason = StatusPerToken, reasonPerToken
	case o.Cached:
		a.readCached(j, &r)
	default:
		a.readLive(ctx, j, &r)
	}
	if r.Kind == RowHome && !j.missing {
		readAccount(j, &r)
	}
	if o.Plan && r.Kind == RowHome && !j.missing {
		r.Plan = a.readPlan(ctx, j, o.Cached)
	}
	return r
}

// readAccount fills a home row's account email and plan label from the
// login on disk (agentio's identity reader, local only). It is display
// only, so a home without one, or an agent that models none, leaves
// them empty and the row is unchanged otherwise.
func readAccount(j usageJob, r *UsageRow) {
	ir, ok := agentio.IdentityOf(j.agent)
	if !ok {
		return
	}
	id, err := ir.Identity(j.home)
	if err != nil {
		return
	}
	// Reveal is deliberate: the owner reads accounts by email, as agx
	// v0.3's usage and profile screens showed them.
	r.Account, r.PlanLabel = id.Email.Reveal(), id.PlanLabel
}

// readLive is one request to the vendor's usage endpoint (cost N).
func (a *App) readLive(ctx context.Context, j usageJob, r *UsageRow) {
	ur, ok := agentio.UsageOf(j.agent)
	if !ok {
		r.Status, r.Reason = StatusUnsupported, usageUnsupportedReason(j.agent, agentio.CapUsage, reasonNoUsage)
		return
	}
	rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
	defer cancel()
	u, err := ur.Usage(rctx, j.home)
	setUsage(r, u, err, SourceLive)
}

// readCached reads the copy the vendor CLI left on disk (cost R).
func (a *App) readCached(j usageJob, r *UsageRow) {
	cr, ok := agentio.CachedUsageOf(j.agent)
	if !ok {
		r.Status, r.Reason = StatusUnsupported, usageUnsupportedReason(j.agent, agentio.CapCachedUsage, reasonNoUsage)
		return
	}
	note := ""
	if n, ok := cr.(agentio.CachedUsageNoter); ok {
		note = n.CachedUsageNote()
	}
	u, found, err := cr.CachedUsage(j.home)
	if err == nil && !found {
		r.Status, r.Reason = StatusNoCache, reasonNoReading
		if note != "" {
			r.Reason = "no reading on disk: " + note
		}
		return
	}
	setUsage(r, u, err, SourceCached)
	if r.Status == StatusOK {
		// Kept for finishRow: an old reading is explained by the agent's
		// note (Claude Code stopped refreshing it).
		r.Reason = note
	}
}

// StaleAfter is the age past which a cached reading is reported stale:
// plan windows are five hours to a week long, so a day-old reading can
// be wrong by a whole window.
const StaleAfter = 24 * time.Hour

// probeLogin resolves the login's token and probes it (cost Q, plus X
// when the reference runs a command). The token is never kept or shown.
func (a *App) probeLogin(ctx context.Context, j usageJob, r *UsageRow) {
	pr, ok := agentio.TokenProberOf(j.agent)
	if !ok {
		r.Status, r.Reason = StatusUnsupported, usageUnsupportedReason(j.agent, agentio.CapTokenProber, reasonNoProbe)
		return
	}
	cred, err := login.Credential(ctx, a.exp, a.paths, r.Login, j.login)
	if err != nil {
		r.Status, r.Reason = StatusSecret, apperr.As(err).Message
		return
	}
	rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
	defer cancel()
	u, err := pr.TokenUsage(rctx, cred)
	setUsage(r, u, err, SourceProbe)
}

// setUsage fills r from a reading or its error.
func setUsage(r *UsageRow, u agentio.Usage, err error, src RowSource) {
	if err != nil {
		r.Status, r.Reason = classifyUsageErr(err)
		return
	}
	r.Source = src
	if !u.ObservedAt.IsZero() {
		t := u.ObservedAt
		r.ObservedAt = &t
	}
	if u.Billing == agentio.BillingPerToken {
		r.Status = StatusPerToken
		return
	}
	r.Windows = make([]UsageWindow, 0, len(u.Windows))
	full := false
	for _, w := range u.Windows {
		r.Windows = append(r.Windows, UsageWindow{Class: w.Class().String(), Label: w.Label, Scope: w.Scope, Percent: w.Percent, ResetsAt: w.ResetsAt,
			ID: w.ID, Status: w.Status, Binding: w.Binding})
		// A full per-model window limits that model only: the account
		// still takes requests for the others.
		full = full || (w.Scope == "" && w.Percent >= percentFull)
	}
	r.Status = StatusOK
	if full {
		r.Status = StatusLimit
	}
}

// percentFull is the percent at which a window is at its cap.
const percentFull = 100.0

// classifyUsageErr maps a read error to a status and a short reason, through
// agentio's neutral sentinels only (no vendor wording is parsed).
func classifyUsageErr(err error) (RowStatus, string) {
	var nerr net.Error
	switch {
	case errors.Is(err, agentio.ErrBilledPerToken):
		return StatusPerToken, ""
	case errors.Is(err, agentio.ErrUnsupported):
		return StatusUnsupported, usageFirstLine(apperr.Text(err))
	case errors.Is(err, agentio.ErrRateLimited):
		return StatusRateLimited, usageFirstLine(apperr.Text(err))
	case errors.Is(err, agentio.ErrExpired):
		return StatusExpired, usageFirstLine(apperr.Text(err))
	case errors.Is(err, agentio.ErrNotLoggedIn):
		return StatusNotLoggedIn, "not logged in; run the agent once in this home"
	case errors.Is(err, agentio.ErrScopeLimited):
		return StatusScopeLimited, usageFirstLine(apperr.Text(err))
	case errors.Is(err, agentio.ErrUnauthorized):
		return StatusRejected, usageFirstLine(apperr.Text(err))
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &nerr):
		return StatusNetwork, usageFirstLine(apperr.Text(err))
	}
	return StatusError, usageFirstLine(apperr.Text(err))
}

// usageUnsupportedReason is the agent's declared reason for lacking c.
func usageUnsupportedReason(ag agentio.Agent, c agentio.Capability, fallback string) string {
	if reason, ok := agentio.Unsupported(ag, c); ok && reason != "" {
		return reason
	}
	return fallback
}

// readPlan is --plan: the subscription from the vendor (cost N), or with
// --cached the plan label of the login on disk (cost R).
func (a *App) readPlan(ctx context.Context, j usageJob, cached bool) *UsagePlan {
	if cached {
		p := &UsagePlan{Source: SourceCached}
		ir, ok := agentio.IdentityOf(j.agent)
		if !ok {
			p.Error = usageUnsupportedReason(j.agent, agentio.CapIdentity, "identity not modelled")
			return p
		}
		id, err := ir.Identity(j.home)
		if err != nil {
			p.Error = apperr.Text(err)
			return p
		}
		p.Plan = id.PlanLabel
		return p
	}
	p := &UsagePlan{Source: SourceLive}
	sr, ok := agentio.SubscriptionOf(j.agent)
	if !ok {
		p.Error = usageUnsupportedReason(j.agent, agentio.CapSubscription, "subscription not modelled")
		return p
	}
	rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
	defer cancel()
	s, err := sr.Subscription(rctx, j.home)
	if err != nil {
		p.Error = apperr.Text(err)
		return p
	}
	p.Plan, p.Status, p.BillingType, p.StartedAt = s.Plan, s.Status, s.BillingType, s.StartedAt
	return p
}

// finishRow derives the display values: what is left, each window's reset
// in loc, and a cached reading's age.
func finishRow(r *UsageRow, loc *time.Location, now time.Time) {
	if r.Windows == nil {
		r.Windows = []UsageWindow{}
	}
	if len(r.Windows) > 0 {
		used := 0.0
		for _, w := range r.Windows {
			used = max(used, w.Percent)
		}
		left := max(0, percentFull-used)
		r.Left = &left
	}
	for i := range r.Windows {
		if t := r.Windows[i].ResetsAt; t != nil {
			r.Windows[i].Reset = datefmt.RoundMinute(t.In(loc)).Format(ResetFormat)
		}
	}
	if r.Source == SourceCached && r.ObservedAt != nil {
		age := now.Sub(*r.ObservedAt)
		r.Age = AgeText(age)
		if r.Status == StatusOK && age > StaleAfter {
			note := r.Reason
			r.Status, r.Reason = StatusStale, "reading is "+r.Age+" old; drop --cached for a live read"
			if note != "" {
				r.Reason += " (" + note + ")"
			}
		}
	}
	if r.Status == StatusOK {
		r.Reason = ""
	}
}

// ResetFormat is how a reset time reads ("Tue 3 Nov 14:00").
const ResetFormat = "Mon 2 Jan 15:04"

// Age units for AgeText.
const (
	usageAgeDay = 24 * time.Hour
)

// AgeText is a reading's age: "<1m", "4m", "2h", "3d".
func AgeText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < usageAgeDay:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/usageAgeDay)) + "d"
}

// usageFirstLine trims a message to its first line (agentio errors may join
// several reasons).
func usageFirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
