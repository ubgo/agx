package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/pick"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/login"
)

// --auto-profile and --auto-login (spec 6.1 steps 2 and 4): read each
// candidate's usage, then let agentio/pick choose the one with the most
// headroom. The returned reason is pick's one-line explanation, which the
// resolver records as the decision's source, so --dry-run shows why.

// autoProfile picks the profile with the most headroom among names (all
// of ag's profiles). Cost N: one live read per home (a home shared by two
// profiles is read once); a home that answers 429 falls back to its
// cached reading, whose age the explanation shows. Edges 13 to 15.
func (a *App) autoProfile(ctx context.Context, ag agentio.Agent, names []string) (string, string, error) {
	live, hasLive := agentio.UsageOf(ag)
	cached, hasCached := agentio.CachedUsageOf(ag)
	if !hasLive && !hasCached {
		return "", "", apperr.Unsupported(fmt.Sprintf("%s has no usage; choose a profile", ag.ID())).
			WithHint("agx run --profile " + names[0])
	}
	cfg, err := a.Config()
	if err != nil {
		return "", "", err
	}
	type reading struct {
		u   agentio.Usage
		err error
	}
	byDir := map[string]reading{}
	cands := make([]pick.Candidate, 0, len(names))
	for _, name := range names {
		if err := ctxErr(ctx); err != nil {
			return "", "", err
		}
		p := cfg.Config.Profiles[name]
		dir := config.ExpandPath(p.Home, a.d.Lookup, a.d.Host.UserHome)
		home := agentio.NewHome(ag.ID(), dir, dir == ag.DefaultHome().Dir(), name)
		c := pick.Candidate{Home: home}
		if p.Billing == config.BillingAPI {
			// Per-token billing has no windows; nothing is requested.
			c.Billing = agentio.BillingPerToken
			cands = append(cands, c)
			continue
		}
		rd, seen := byDir[dir]
		if !seen {
			switch {
			case !statDir(dir):
				rd.err = errors.New("home " + a.tilde(dir) + " does not exist")
			case hasLive:
				rd.u, rd.err = a.liveOnce(ctx, live, home)
				if errors.Is(rd.err, agentio.ErrRateLimited) && hasCached {
					if u, found, cerr := cached.CachedUsage(home); cerr == nil && found {
						rd.u, rd.err = u, nil
					}
				}
			default:
				u, found, cerr := cached.CachedUsage(home)
				rd.u, rd.err = u, cerr
				if cerr == nil && !found {
					rd.err = errors.New(reasonNoReading)
				}
			}
			byDir[dir] = rd
		}
		c.Usage, c.Err = rd.u, rd.err
		perTokenAnswer(&c)
		cands = append(cands, c)
	}
	return a.pickBest("auto profile", cands)
}

// autoLogin picks the stored login with the most headroom among names
// (every login serving ag) by probing each (cost Q, plus X for a
// reference that runs a command). A login whose token does not resolve is
// a skipped candidate with its reason, never a silent drop.
func (a *App) autoLogin(ctx context.Context, ag agentio.Agent, names []string) (string, string, error) {
	pr, ok := agentio.TokenProberOf(ag)
	if !ok {
		return "", "", apperr.Unsupported(fmt.Sprintf("%s cannot probe a login's usage (%s); choose a login",
			ag.ID(), usageUnsupportedReason(ag, agentio.CapTokenProber, reasonNoProbe))).WithHint("agx run --login " + names[0])
	}
	cfg, err := a.Config()
	if err != nil {
		return "", "", err
	}
	cands := make([]pick.Candidate, 0, len(names))
	for _, name := range names {
		if err := ctxErr(ctx); err != nil {
			return "", "", err
		}
		// A login has no folder; the home only carries its name for pick's
		// explanation and is never used to read or launch.
		c := pick.Candidate{Home: agentio.NewHome(ag.ID(), "", false, name)}
		cred, err := login.Credential(ctx, a.exp, a.paths, name, cfg.Config.Logins[name])
		if err != nil {
			c.Err = errors.New(apperr.As(err).Message)
			cands = append(cands, c)
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
		c.Usage, c.Err = pr.TokenUsage(rctx, cred)
		cancel()
		perTokenAnswer(&c)
		cands = append(cands, c)
	}
	return a.pickBest("auto login", cands)
}

// liveOnce is one live usage read bounded by --timeout.
func (a *App) liveOnce(ctx context.Context, r agentio.UsageReader, h agentio.Home) (agentio.Usage, error) {
	rctx, cancel := context.WithTimeout(ctx, a.d.Timeout)
	defer cancel()
	return r.Usage(rctx, h)
}

// perTokenAnswer turns agentio's "billed per token" answer into per-token
// billing, so pick reports it as such and not as a read failure.
func perTokenAnswer(c *pick.Candidate) {
	if errors.Is(c.Err, agentio.ErrBilledPerToken) {
		c.Err, c.Billing = nil, agentio.BillingPerToken
	}
}

// pickBest runs pick over cands and words a refusal as edges 13 and 14:
// every candidate's reason in order, or "every candidate bills per token".
func (a *App) pickBest(what string, cands []pick.Candidate) (string, string, error) {
	res, err := pick.Best(cands, pick.WithPolicy(headroom{}), pick.WithNow(a.Now()))
	if err == nil {
		return res.Chosen.Home.Name(), res.Explain(), nil
	}
	allPerToken := len(res.Skipped) > 0
	parts := make([]string, 0, len(res.Skipped))
	for _, s := range res.Skipped {
		allPerToken = allPerToken && s.Reason == pick.SkipPerToken
		parts = append(parts, s.Home.Name()+" ("+skipText(s)+")")
	}
	if allPerToken {
		return "", "", apperr.Failure(apperr.CodeAutoPick, what+": every candidate bills per token")
	}
	return "", "", apperr.Failure(apperr.CodeAutoPick, what+": none eligible: "+strings.Join(parts, "; "))
}

// skipText is a skipped candidate's short reason: the policy's own words,
// "bills per token", or the read error's status and reason.
func skipText(s pick.Skip) string {
	switch s.Reason {
	case pick.SkipPerToken:
		return "bills per token"
	case pick.SkipPolicy:
		// pick wraps the policy's reason as "<ErrPolicySkip>: <reason>".
		return apperr.UserText(strings.TrimPrefix(s.Err.Error(), pick.ErrPolicySkip.Error()+": "))
	}
	status, reason := classifyUsageErr(s.Err)
	switch status {
	case StatusRateLimited, StatusExpired, StatusNotLoggedIn, StatusRejected, StatusNetwork:
		return string(status)
	}
	if reason == "" {
		return string(status)
	}
	return reason
}

// headroom is MaxPercent (the highest window percent decides, lowest
// wins) plus one rule pick's default lacks: a candidate with a window at
// its cap has no headroom at all and is skipped with "rate-limited,
// resets <time>" (edge 13), instead of being chosen when every candidate
// is full. Its name is MaxPercent's, since the score is the same, so the
// explanation reads "auto → work (max 5%)".
type headroom struct{}

// Name is MaxPercent's name.
func (headroom) Name() string { return pick.MaxPercent{}.Name() }

// Score is MaxPercent's score, refusing a candidate at its cap.
func (headroom) Score(c pick.Candidate) (float64, bool, string) {
	score, _, _ := pick.MaxPercent{}.Score(c)
	if score < percentFull {
		return score, true, ""
	}
	reason := "rate-limited"
	for _, w := range c.Usage.Windows {
		if w.Percent >= percentFull && w.ResetsAt != nil {
			reason += ", resets " + datefmt.RoundMinute(w.ResetsAt.In(time.Local)).Format(resetClock)
			break
		}
	}
	return score, false, reason
}

// resetClock is the reset time in a refusal ("resets 14:00").
const resetClock = "15:04"
