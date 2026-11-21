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

// The rule flags of login add and edit (docs/guides/pool-rules.md): the
// order, the rule values and the per-group caps. Like every pool setting flag, an empty value
// removes the key so the documented default applies again.
const (
	flagPoolOrder             = "--pool-order"
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
	flagPoolProbeModel        = "--pool-probe-model"
	flagPoolDeadlineWindow    = "--pool-deadline-window"
	flagPoolDeadlineMinRoom   = "--pool-deadline-min-room"
)

// poolRuleFlagsText names the rule flags in "nothing to change".
const poolRuleFlagsText = flagPoolOrder + ", " + flagPoolDeadlineWindow + ", " + flagPoolDeadlineMinRoom + ", " + flagPoolWeekRelaxWindow + ", " + flagPoolMaxSessions + ", " + flagPoolSticky + ", " + flagPoolStickyLeaveRatio + ", " +
	flagPoolDefaultWeekRelax + ", " + flagPoolCapWeekRelax + ", " + flagPoolCapacity + ", " + flagPoolGroupCapShort + ", " + flagPoolGroupCapWeek + ", " + flagPoolGroupCapWeekRelax + ", " + flagPoolProbeModel

// Keys the rule flags write inside logins.<name>.pool.
const (
	keyPoolOrder           = "order"
	keyPoolWeekRelaxWindow = "week_relax_window"
	keyPoolMaxSessions     = "max_sessions"
	keyPoolSticky          = "sticky"
	keyPoolStickyLeave     = "sticky_leave_ratio"
	keyPoolWeekRelaxMax    = "week_relax_max"
	keyPoolCapacity        = "capacity"
	keyPoolOwnCaps         = "own_caps"
	keyPoolSharedCaps      = "shared_caps"
	keyPoolProbeModels     = "probe_models"
	keyPoolDeadlineWindow  = "deadline_window"
	keyPoolDeadlineMinRoom = "deadline_min_room"
)

// PoolRuleSettings are the rule flags; nil = not given.
type PoolRuleSettings struct {
	// Order is "urgency" or "deadline".
	Order *string
	// WeekRelaxWindow is a Go duration (week_relax_window).
	WeekRelaxWindow *string
	// MaxSessions is a whole number, 0 or more (max_sessions).
	MaxSessions *string
	// Sticky is "true" or "false" (sticky).
	Sticky *string
	// StickyLeaveRatio is a number of 0 or more (sticky_leave_ratio).
	StickyLeaveRatio *string
	// DeadlineWindow is a Go duration (deadline_window).
	DeadlineWindow *string
	// DeadlineMinRoom is a percent (deadline_min_room).
	DeadlineMinRoom *string
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
	// ProbeModels replaces probe_models (repeat); one empty value
	// removes the key.
	ProbeModels []string
}

// given reports any rule flag.
func (s PoolRuleSettings) given() bool {
	return s.Order != nil || s.WeekRelaxWindow != nil || s.MaxSessions != nil || s.Sticky != nil || s.StickyLeaveRatio != nil ||
		s.DeadlineWindow != nil || s.DeadlineMinRoom != nil || s.DefaultWeekRelax != nil || s.CapWeekRelax != nil || s.Capacity != nil || s.GroupCapShort != nil || s.GroupCapWeek != nil || s.GroupCapWeekRelax != nil ||
		s.ProbeModels != nil
}

// planRules plans the order, max_sessions, sticky, sticky_leave_ratio,
// deadline_min_room and the group caps (week_relax_window and
// deadline_window are planned with the other durations).
func (p *poolPlan) planRules(cur config.Pool, s PoolRuleSettings) error {
	if v := s.Order; v != nil {
		if *v != "" && !slices.Contains(pool.OrderValues(), *v) {
			return apperr.Usage(fmt.Sprintf("%s %q: use %s", flagPoolOrder, *v, strings.Join(pool.OrderValues(), " or ")))
		}
		p.change(keyPoolOrder, orAbsent(cur.Order), orAbsent(*v), setOrDeleteEdit(p.at(keyPoolOrder), *v))
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
	if v := s.DeadlineMinRoom; v != nil {
		if err := p.planPercent(flagPoolDeadlineMinRoom, keyPoolDeadlineMinRoom, cur.DeadlineMinRoom, *v); err != nil {
			return err
		}
	}
	if s.ProbeModels != nil {
		if err := p.planProbeModels(cur, s.ProbeModels); err != nil {
			return err
		}
	}
	return p.planGroupCaps(cur, s)
}

// planProbeModels plans probe_models: the given models replace the list;
// a single empty value removes it (the default applies again).
func (p *poolPlan) planProbeModels(cur config.Pool, models []string) error {
	old := strings.Join(cur.ProbeModels, " ")
	if len(models) == 1 && models[0] == "" {
		p.change(keyPoolProbeModels, orAbsent(old), absent, deleteIf(p.at(keyPoolProbeModels), cur.ProbeModels != nil))
		return nil
	}
	for _, m := range models {
		if strings.TrimSpace(m) == "" || strings.ContainsAny(m, " \t") {
			return apperr.Usage(fmt.Sprintf("%s %q: give one model name per flag, like claude-fable-5-2 or opus (a single '' removes the list)", flagPoolProbeModel, m))
		}
	}
	p.change(keyPoolProbeModels, orAbsent(old), strings.Join(models, " "), setTo(p.at(keyPoolProbeModels), models))
	return nil
}

// orAbsent shows an optional string key ("" when unset).
func orAbsent(v string) string {
	if v == "" {
		return absent
	}
	return v
}

// planGroupCaps plans own_caps and shared_caps from the GROUP=N flags.
func (p *poolPlan) planGroupCaps(cur config.Pool, s PoolRuleSettings) error {
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
