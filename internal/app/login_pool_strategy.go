package app

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// The strategy flags of login add and edit (docs/guides/login-pool.md
// "Strategies"): the rules that pick, the expiring strategy's knobs, and
// the per-group caps. Like every pool setting flag, an empty value
// removes the key so the documented default applies again.
const (
	flagPoolStrategy          = "--pool-strategy"
	flagPoolWeekRelaxWindow   = "--pool-week-relax-window"
	flagPoolMaxSessions       = "--pool-max-sessions"
	flagPoolSticky            = "--pool-sticky"
	flagPoolStickyLeaveRatio  = "--pool-sticky-leave-ratio"
	flagPoolDefaultWeekRelax  = "--pool-default-week-relax"
	flagPoolCapWeekRelax      = "--pool-cap-week-relax"
	flagPoolGroupCapShort     = "--pool-group-cap-short"
	flagPoolGroupCapWeek      = "--pool-group-cap-week"
	flagPoolGroupCapWeekRelax = "--pool-group-cap-week-relax"
	flagPoolCapacity          = "--pool-capacity"
)

// poolStrategyFlagsText names the strategy flags in "nothing to change".
const poolStrategyFlagsText = flagPoolStrategy + ", " + flagPoolWeekRelaxWindow + ", " + flagPoolMaxSessions + ", " + flagPoolSticky + ", " + flagPoolStickyLeaveRatio + ", " +
	flagPoolDefaultWeekRelax + ", " + flagPoolCapWeekRelax + ", " + flagPoolCapacity + ", " + flagPoolGroupCapShort + ", " + flagPoolGroupCapWeek + ", " + flagPoolGroupCapWeekRelax

// Keys the strategy flags write inside logins.<name>.pool.
const (
	keyPoolStrategy        = "strategy"
	keyPoolWeekRelaxWindow = "week_relax_window"
	keyPoolMaxSessions     = "max_sessions"
	keyPoolSticky          = "sticky"
	keyPoolStickyLeave     = "sticky_leave_ratio"
	keyPoolWeekRelaxMax    = "week_relax_max"
	keyPoolCapacity        = "capacity"
	keyPoolOwnCaps         = "own_caps"
	keyPoolSharedCaps      = "shared_caps"
)

// PoolStrategySettings are the strategy flags; nil = not given.
type PoolStrategySettings struct {
	// Strategy is "deadline" or "expiring".
	Strategy *string
	// WeekRelaxWindow is a Go duration (week_relax_window).
	WeekRelaxWindow *string
	// MaxSessions is a whole number, 0 or more (max_sessions).
	MaxSessions *string
	// Sticky is "true" or "false" (sticky).
	Sticky *string
	// StickyLeaveRatio is a number of 0 or more (sticky_leave_ratio).
	StickyLeaveRatio *string
	// DefaultWeekRelax is defaults.week_relax_max.
	DefaultWeekRelax *string
	// CapWeekRelax sets accounts.NAME.week_relax_max: NAME=N.
	CapWeekRelax []string
	// Capacity sets accounts.NAME.capacity: NAME=N, N above 0.
	Capacity []string
	// GroupCapShort, GroupCapWeek and GroupCapWeekRelax set own_caps or
	// shared_caps: GROUP=N (own or shared); GROUP= removes the key.
	GroupCapShort     []string
	GroupCapWeek      []string
	GroupCapWeekRelax []string
}

// given reports any strategy flag.
func (s PoolStrategySettings) given() bool {
	return s.Strategy != nil || s.WeekRelaxWindow != nil || s.MaxSessions != nil || s.Sticky != nil || s.StickyLeaveRatio != nil ||
		s.DefaultWeekRelax != nil || s.CapWeekRelax != nil || s.Capacity != nil || s.GroupCapShort != nil || s.GroupCapWeek != nil || s.GroupCapWeekRelax != nil
}

// planStrategy plans the strategy, max_sessions, sticky and the group
// caps (week_relax_window is planned with the other durations).
func (p *poolPlan) planStrategy(cur config.Pool, s PoolStrategySettings) error {
	if v := s.Strategy; v != nil {
		if *v != "" && !slices.Contains(pool.StrategyValues(), *v) {
			return apperr.Usage(fmt.Sprintf("%s %q: use %s", flagPoolStrategy, *v, strings.Join(pool.StrategyValues(), " or ")))
		}
		p.change(keyPoolStrategy, orAbsent(cur.Strategy), orAbsent(*v), setOrDeleteEdit(p.at(keyPoolStrategy), *v))
	}
	if v := s.MaxSessions; v != nil {
		old := absent
		if cur.MaxSessions != nil {
			old = strconv.Itoa(*cur.MaxSessions)
		}
		e := deleteIf(p.at(keyPoolMaxSessions), cur.MaxSessions != nil)
		if *v != "" {
			n, err := strconv.Atoi(*v)
			if err != nil || n < 0 {
				return apperr.Usage(fmt.Sprintf("%s %q is not a whole number of 0 or more (0 = no limit)", flagPoolMaxSessions, *v))
			}
			e = edit{keys: p.at(keyPoolMaxSessions), raw: []byte(strconv.Itoa(n))}
		}
		p.change(keyPoolMaxSessions, old, orAbsent(*v), e)
	}
	if v := s.Sticky; v != nil {
		old := absent
		if cur.Sticky != nil {
			old = strconv.FormatBool(*cur.Sticky)
		}
		e := deleteIf(p.at(keyPoolSticky), cur.Sticky != nil)
		if *v != "" {
			b, err := strconv.ParseBool(*v)
			if err != nil {
				return apperr.Usage(fmt.Sprintf("%s %q: use true or false", flagPoolSticky, *v))
			}
			e = setTo(p.at(keyPoolSticky), b)
		}
		p.change(keyPoolSticky, old, orAbsent(*v), e)
	}
	if v := s.StickyLeaveRatio; v != nil {
		old := absent
		if cur.StickyLeaveRatio != nil {
			old = percentText(*cur.StickyLeaveRatio)
		}
		e := deleteIf(p.at(keyPoolStickyLeave), cur.StickyLeaveRatio != nil)
		if *v != "" {
			n, err := strconv.ParseFloat(*v, 64)
			if err != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return apperr.Usage(fmt.Sprintf("%s %q is not a number of 0 or more (0 = never leave)", flagPoolStickyLeaveRatio, *v))
			}
			e = setNumber(p.at(keyPoolStickyLeave), n)
		}
		p.change(keyPoolStickyLeave, old, orAbsent(*v), e)
	}
	return p.planGroupCaps(cur, s)
}

// orAbsent shows an optional string key ("" when unset).
func orAbsent(v string) string {
	if v == "" {
		return absent
	}
	return v
}

// planGroupCaps plans own_caps and shared_caps from the GROUP=N flags.
func (p *poolPlan) planGroupCaps(cur config.Pool, s PoolStrategySettings) error {
	byGroup := map[pool.Group][]capFlag{}
	for _, f := range []struct {
		flag, key string
		in        []string
	}{
		{flagPoolGroupCapShort, keyPoolShort, s.GroupCapShort},
		{flagPoolGroupCapWeek, keyPoolWeek, s.GroupCapWeek},
		{flagPoolGroupCapWeekRelax, keyPoolWeekRelaxMax, s.GroupCapWeekRelax},
	} {
		seen := map[pool.Group]bool{}
		for _, kv := range f.in {
			name, v, ok := strings.Cut(kv, capSep)
			g := pool.Group(name)
			if !ok || (g != pool.GroupOwn && g != pool.GroupShared) {
				return apperr.Usage(fmt.Sprintf("%s %q: use own=N or shared=N (a number from %s to %s; GROUP= removes the group's cap)", f.flag, kv, percentText(pool.MinPercent), percentText(pool.MaxPercent)))
			}
			if seen[g] {
				return apperr.Usage(fmt.Sprintf("%s %s is given twice", f.flag, g))
			}
			seen[g] = true
			byGroup[g] = append(byGroup[g], capFlag{f.flag, f.key, &v})
		}
	}
	for _, g := range []struct {
		group pool.Group
		key   string
		cur   config.PoolCaps
	}{{pool.GroupOwn, keyPoolOwnCaps, cur.OwnCaps}, {pool.GroupShared, keyPoolSharedCaps, cur.SharedCaps}} {
		if err := p.planCapsBlock(g.key, g.cur, byGroup[g.group]); err != nil {
			return err
		}
	}
	return nil
}

// parseCapacities reads repeated NAME=N values of --pool-capacity: a
// member and a number above 0 (NAME= removes the member's capacity).
func parseCapacities(in []string, members []string, poolName string) ([]memberEdit, error) {
	var out []memberEdit
	for _, kv := range in {
		name, v, ok := strings.Cut(kv, capSep)
		if !ok || name == "" {
			return nil, apperr.Usage(fmt.Sprintf("%s %q: use NAME=N (the member's plan size relative to the others, above 0; NAME= removes it)", flagPoolCapacity, kv))
		}
		if err := checkMember(flagPoolCapacity+" "+kv, name, members, poolName); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(out, func(m memberEdit) bool { return m.name == name }) {
			return nil, apperr.Usage(fmt.Sprintf("%s %s is given twice", flagPoolCapacity, name))
		}
		m := memberEdit{name: name}
		if v != "" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, apperr.Usage(fmt.Sprintf("%s %s: %s is not a number above 0", flagPoolCapacity, kv, v))
			}
			m.cap = &n
		}
		out = append(out, m)
	}
	return out, nil
}
