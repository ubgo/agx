package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/pool"
)

// --pool-only (docs/guides/login-pool.md "Pin one account but keep its
// limits"): one member for every key of a run or one call, with the
// pool's rules still applied to it, so it hands out no key rather than
// another member's or one over its caps. --pool-force is the opposite
// pin: one member, every rule ignored. The two are a conflict.

// The --pool-only flags, as messages and sources name them.
const (
	flagPoolOnly      = "--pool-only"
	flagPoolOnlyShort = "--pool-only-short"
	flagPoolOnlyWeek  = "--pool-only-week"
)

// onlyEnforced is how a pin reads in whoami, live show and the dry run.
const onlyEnforced = "(caps enforced)"

// PoolOnly is --pool-only NAME and its tighter caps for one run or call.
type PoolOnly struct {
	// Member is the one member keys come from; "" = no pin.
	Member string
	// Caps are --pool-only-short / --pool-only-week: lower the member's
	// configured caps for this run only (never raise them).
	Caps pool.Tighter
}

// stateOnly is a run's pin as live.jsonc keeps it.
func stateOnly(st live.State) PoolOnly {
	return PoolOnly{Member: st.PoolOnly, Caps: pool.Tighter{Short: st.PoolOnlyShort, Week: st.PoolOnlyWeek}}
}

// given reports whether any --pool-only flag was given.
func (o PoolOnly) given() bool { return o.Member != "" || o.Caps.Given() }

// checkPoolOnly refuses a pin that cannot be kept, before anything is
// read or written: a tighter cap without --pool-only, --pool-only with
// --pool-force, a name that is not a member of the pool, a cap outside
// 0..100 or above the member's configured cap, and at launch (launch
// true) a disabled member, which would never get a key.
func checkPoolOnly(name string, p *config.Pool, rules pool.Config, o PoolOnly, force string, launch bool) error {
	if o.Member == "" {
		if o.Caps.Given() {
			return apperr.Usage(flagPoolOnlyShort + " and " + flagPoolOnlyWeek + " lower the " + flagPoolOnly + " member's caps; add " + flagPoolOnly + " NAME")
		}
		return nil
	}
	if force != "" {
		return apperr.Conflict(flagPoolOnly + " and " + flagPoolForce + " both choose the member: " + flagPoolOnly + " keeps its caps, " + flagPoolForce + " ignores them; pick one")
	}
	if !slices.Contains(p.Members(), o.Member) {
		return apperr.Usage(fmt.Sprintf("%s %s: %s is not a member of pool %s (members: %s)", flagPoolOnly, o.Member, o.Member, name, strings.Join(p.Members(), ", ")))
	}
	if launch && !rules.Enabled(o.Member) {
		return apperr.Usage(fmt.Sprintf("%s %s: %s is disabled in pool %s, so it would never get a key", flagPoolOnly, o.Member, o.Member, name)).
			WithHint("agx login edit --id " + name + " --pool-enable " + o.Member)
	}
	configured := rules.CapsFor(o.Member)
	for _, c := range []struct {
		flag, label string
		v           *float64
		cap         float64
	}{{flagPoolOnlyShort, "5h", o.Caps.Short, configured.Short}, {flagPoolOnlyWeek, "week", o.Caps.Week, configured.Week}} {
		switch {
		case c.v == nil:
		case !validPercent(*c.v):
			return apperr.Usage(fmt.Sprintf("%s %s is not a number from %s to %s", c.flag, percentText(*c.v), percentText(pool.MinPercent), percentText(pool.MaxPercent)))
		case *c.v > c.cap:
			return apperr.Usage(fmt.Sprintf("%s %s is above %s's %s cap %s; it may only lower it", c.flag, percentText(*c.v), o.Member, c.label, percentText(c.cap)))
		}
	}
	return nil
}

// validPercent is a finite value from pool.MinPercent to pool.MaxPercent.
func validPercent(v float64) bool { return v >= pool.MinPercent && v <= pool.MaxPercent }

// onlyFailure is a pinned pick that found its member blocked: one line,
// the member and every reason ("alice: week is 70% (cap 70%)"), with the
// code of every "none usable" pick, so agx credential prints no key and
// the agent's request fails instead of switching.
func onlyFailure(member string, res pool.Result) error {
	return apperr.Failure(apperr.CodeAutoPick, member+": "+strings.Join(onlyReasons(res), "; "))
}

// onlyReasons is the pinned member's block reasons (the one candidate).
func onlyReasons(res pool.Result) []string {
	if len(res.Candidates) == 0 {
		return []string{}
	}
	return res.Candidates[0].Reasons
}

// onlyNoKey is what live.jsonc keeps when a pinned run's key request got
// nothing: "alice over cap: week is 70% (cap 70%)" when a cap stopped it,
// "alice not usable: …" for any other reason (status, disabled, no
// reading), so whoami's "in use: none (…)" says which.
func onlyNoKey(member string, reasons []string) string {
	return member + " " + onlyWhat(reasons) + ": " + strings.Join(reasons, "; ")
}

// onlyWhat is "over cap" when a cap blocks the member, else "not usable".
func onlyWhat(reasons []string) string {
	if slices.ContainsFunc(reasons, isCapReason) {
		return briefOverCap
	}
	return briefNotUsable
}

// isCapReason reports a pool reason that names a cap.
func isCapReason(r string) bool { return strings.Contains(r, capMarker) }

// capMarker is how the pool's reasons name a cap ("week is 70% (cap 70%)").
const capMarker = "(cap "

// OnlyText is a pin as whoami, live show and the dry run print it:
// "alice (caps enforced)".
func OnlyText(member string) string { return member + " " + onlyEnforced }

// InUseText is the "in use" value whoami, agx live list, agx live show
// and agx statusline print for a live run at now: the login in use;
// "none (all over caps) since 10:52" when the last key request was
// refused and the agent was handed no key (when_none_usable "stop");
// "alice (no key since 10:52: all over caps; the agent keeps its key)"
// when it was refused under "keep"; "" when the agent has not asked yet.
func InUseText(st live.State, now time.Time) string {
	if st.NoKey == "" {
		return st.InUse
	}
	brief := cmp.Or(st.NoKeyBrief, st.NoKey)
	since := ""
	if t := cmp.Or(st.NoKeySince, st.InUseAt); !t.IsZero() {
		since = " since " + clockText(t, now)
	}
	if st.InUse == "" {
		return "none (" + brief + ")" + since
	}
	return st.InUse + " (no key" + since + ": " + brief + "; the agent keeps its key)"
}

// clockText is a time of a live run as a person reads it next to now:
// "10:52" today, "25 Nov 10:52" on another day (local time).
func clockText(t, now time.Time) string {
	t, now = t.In(time.Local), now.In(time.Local)
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format(clockLayout)
	}
	return t.Format(dayClockLayout)
}

// clockLayout and dayClockLayout are clockText's layouts.
const (
	clockLayout    = "15:04"
	dayClockLayout = "2 Jan 15:04"
)

// onlyDecisions are a pinned launch's dry-run lines: the pin and the
// member's effective caps, each with its source (the run's flag, the
// member's own entry, its group's block, the pool's defaults, or agx's
// default).
func onlyDecisions(key string, pl *config.Pool, rules pool.Config, o PoolOnly) []report.Decision {
	caps, _ := rules.Tightened(o.Member, o.Caps) // checkPoolOnly passed
	g := groupOf(pl, o.Member)
	src := func(flag string, tight *float64, which string) string {
		if tight != nil {
			return flag
		}
		if _, _, cfgKey := memberValue(*pl, o.Member, g, which); cfgKey != "" {
			return key + "." + cfgKey
		}
		return SourceDefault
	}
	return []report.Decision{
		{Name: decPool, Value: "pick: only " + OnlyText(o.Member) + " for every key of this run", Source: flagPoolOnly},
		{Name: decPool, Value: fmt.Sprintf("%s 5h cap %g%%", o.Member, caps.Short), Source: src(flagPoolOnlyShort, o.Caps.Short, memberShort)},
		{Name: decPool, Value: fmt.Sprintf("%s week cap %g%%", o.Member, caps.Week), Source: src(flagPoolOnlyWeek, o.Caps.Week, memberWeek)},
	}
}

// errNotPoolOnly is --pool-only on a login that is not a pool.
func errNotPoolOnly(what string) error {
	return apperr.Usage(fmt.Sprintf("%s picks a member of a pool login; %s", flagPoolOnly, what)).
		WithHint("agx run --login POOL " + flagPoolOnly + " MEMBER")
}
