package app

import (
	"strconv"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// agx pool list and agx pool show: a pool's settings as config states
// them, read without a probe, a cache read or a pick. Creating and
// editing a pool stay on agx login add --kind pool and agx login edit
// --pool-…: a pool is a login kind.

// Where a pool value comes from (PoolSetting.Source, PoolMember's
// *_source): agx's documented default, a pool-wide key of the pool's
// block (defaults.* included), a group's block (own_caps, shared_caps)
// or the member's own entry (accounts.NAME).
const (
	SourceDefault = "default"
	SourcePool    = "pool"
	SourceGroup   = "group"
	SourceMember  = "member"
)

// Pool setting keys, as the pool block in config names them, so a key in
// agx pool show is the one agx config set and the docs use.
const (
	poolKeyForce        = "force"
	poolKeyOrder        = "order"
	poolKeyRelaxWindow  = "week_relax_window"
	poolKeyRelaxMax     = "defaults.week_relax_max"
	poolKeyMaxSessions  = "max_sessions"
	poolKeySticky       = "sticky"
	poolKeyStickyLeave  = "sticky_leave_ratio"
	poolKeyDefaultShort = "defaults.short"
	poolKeyDefaultWeek  = "defaults.week"
	poolKeyOwnShort     = "own_caps.short"
	poolKeyOwnWeek      = "own_caps.week"
	poolKeyBorrow       = "shared_borrow_window"
	poolKeyCutoff       = "short_unavailable_at_percent"
	poolKeyMaxAge       = "usage_max_age"
	poolKeyKeyTTL       = "key_ttl"
	poolKeyPicker       = "picker.command"
	poolKeyPickerTime   = "picker.timeout"
	poolKeyLog          = "log"
)

// poolRuleKeys are the settings that decide the pick (agx pool pick and a
// launch's dry run show them): the rule values and the group defaults.
var poolRuleKeys = []string{poolKeyOrder, poolKeyRelaxWindow, poolKeyRelaxMax, poolKeyMaxSessions, poolKeySticky, poolKeyStickyLeave,
	poolKeyBorrow, poolKeyCutoff, poolKeyOwnShort, poolKeyOwnWeek, poolKeyDefaultShort, poolKeyDefaultWeek}

// Words of an off setting: no force, no picker, no log.
const (
	poolOffForce  = "off"
	poolOffPicker = "built-in rules"
	poolOffLog    = "none"
)

// PoolSummary is one pool in agx pool list.
type PoolSummary struct {
	Name   string   `json:"name"`
	Agents []string `json:"agents"`
	Own    []string `json:"own"`
	Shared []string `json:"shared"`
	// Force is the member the pool always picks; "" = off.
	Force string `json:"force"`
	// Picker is the picker command; "" = the built-in rules decide.
	Picker string `json:"picker"`
}

// PoolSetting is one pool setting: its config key, its value as config
// writes it, and where the value comes from (default or pool).
type PoolSetting struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// PoolMember is one member with the values it gets, each with its
// source: its own accounts entry (member), its group's block (group),
// the pool's defaults key (pool), else agx's default (default).
type PoolMember struct {
	Name               string     `json:"name"`
	Group              pool.Group `json:"group"`
	Enabled            bool       `json:"enabled"`
	EnabledSource      string     `json:"enabled_source"`
	Short              float64    `json:"short"`
	ShortSource        string     `json:"short_source"`
	Week               float64    `json:"week"`
	WeekSource         string     `json:"week_source"`
	WeekRelaxMax       float64    `json:"week_relax_max"`
	WeekRelaxMaxSource string     `json:"week_relax_max_source"`
	Capacity           float64    `json:"capacity"`
	CapacitySource     string     `json:"capacity_source"`
}

// PoolInfo is agx pool show's result.
type PoolInfo struct {
	PoolSummary
	Members  []PoolMember  `json:"members"`
	Settings []PoolSetting `json:"settings"`
}

// PoolList is every pool login in the file's order (agx pool list).
// Cost R: config only.
func (a *App) PoolList() ([]PoolSummary, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, err
	}
	out := []PoolSummary{}
	for _, name := range cfg.LoginOrder() {
		if lg := cfg.Config.Logins[name]; lg.Kind == config.KindPool {
			out = append(out, poolSummary(name, lg))
		}
	}
	return out, nil
}

// PoolShowOptions are agx pool show's flags.
type PoolShowOptions struct {
	// ID is the pool login; required.
	ID string
}

// PoolShow is one pool's full settings, each with its source (agx pool
// show). Cost R: config only. A login of another kind is refused with
// the pool commands' wording.
func (a *App) PoolShow(o PoolShowOptions) (PoolInfo, error) {
	_, lg, err := a.namedLogin(o.ID)
	if err != nil {
		return PoolInfo{}, err
	}
	if lg.Kind != config.KindPool {
		return PoolInfo{}, apperr.Usage(a.notAPool(poolCmdShow, o.ID, lg)).WithHint(poolListHint)
	}
	return poolInfo(o.ID, lg), nil
}

// poolSummary is a pool's list row. A pool login without its pool block
// (config check reports it) lists with empty members.
func poolSummary(name string, lg config.Login) PoolSummary {
	p := poolOf(lg)
	s := PoolSummary{Name: name, Agents: []string{}, Own: nonNil(p.Own), Shared: nonNil(p.Shared)}
	for _, id := range lg.Agents {
		s.Agents = append(s.Agents, string(id))
	}
	if p.Force != nil {
		s.Force = *p.Force
	}
	if p.Picker != nil {
		s.Picker = p.Picker.Command
	}
	return s
}

// poolOf is a login's pool block, or an empty one.
func poolOf(lg config.Login) config.Pool {
	if lg.Pool == nil {
		return config.Pool{}
	}
	return *lg.Pool
}

// nonNil keeps an empty list a list in JSON ([] rather than null).
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// poolInfo is a pool's settings with their sources. Values read as the
// config file would write them, defaults included, so a value can be
// copied into agx config set.
func poolInfo(name string, lg config.Login) PoolInfo {
	p := poolOf(lg)
	info := PoolInfo{PoolSummary: poolSummary(name, lg), Members: []PoolMember{}, Settings: poolSettings(p)}
	for _, m := range p.Own {
		info.Members = append(info.Members, poolMember(p, m, pool.GroupOwn))
	}
	for _, m := range p.Shared {
		info.Members = append(info.Members, poolMember(p, m, pool.GroupShared))
	}
	return info
}

// poolSettings is every pool-wide setting with its effective value and
// source, in the order agx pool show prints them.
func poolSettings(p config.Pool) []PoolSetting {
	var out []PoolSetting
	add := func(key, value, src string) {
		out = append(out, PoolSetting{Key: key, Value: value, Source: src})
	}
	addDuration := func(key, v string, def time.Duration) {
		value, src := durationOr(v, def)
		add(key, value, src)
	}
	addPercent := func(key string, v *float64, def float64) {
		value, src := capOr(v, def)
		add(key, percentText(value), src)
	}
	force, forceSrc := poolOffForce, SourceDefault
	if p.Force != nil {
		force, forceSrc = *p.Force, SourcePool
	}
	add(poolKeyForce, force, forceSrc)
	order, orderSrc := string(pool.DefaultOrder), SourceDefault
	if p.Order != "" {
		order, orderSrc = p.Order, SourcePool
	}
	add(poolKeyOrder, order, orderSrc)
	addDuration(poolKeyRelaxWindow, p.WeekRelaxWindow, pool.DefaultWeekRelaxWindow)
	addPercent(poolKeyRelaxMax, p.Defaults.WeekRelaxMax, pool.DefaultWeekRelaxMax)
	sessions, sessionsSrc := strconv.Itoa(pool.DefaultMaxSessions), SourceDefault
	if p.MaxSessions != nil {
		sessions, sessionsSrc = strconv.Itoa(*p.MaxSessions), SourcePool
	}
	add(poolKeyMaxSessions, sessions, sessionsSrc)
	sticky, stickySrc := strconv.FormatBool(pool.DefaultSticky), SourceDefault
	if p.Sticky != nil {
		sticky, stickySrc = strconv.FormatBool(*p.Sticky), SourcePool
	}
	add(poolKeySticky, sticky, stickySrc)
	addPercent(poolKeyStickyLeave, p.StickyLeaveRatio, pool.DefaultStickyLeaveRatio)
	addDuration(poolKeyBorrow, p.SharedBorrowWindow, pool.DefaultBorrowWindow)
	addPercent(poolKeyCutoff, p.ShortUnavailableAtPercent, pool.DefaultShortUnavailableAt)
	ownShort, ownShortSrc := groupCapOr(p.OwnCaps.Short, pool.DefaultOwnShortCap)
	add(poolKeyOwnShort, percentText(ownShort), ownShortSrc)
	ownWeek, ownWeekSrc := groupCapOr(p.OwnCaps.Week, pool.DefaultOwnWeekCap)
	add(poolKeyOwnWeek, percentText(ownWeek), ownWeekSrc)
	addPercent(poolKeyDefaultShort, p.Defaults.Short, pool.DefaultShortCap)
	addPercent(poolKeyDefaultWeek, p.Defaults.Week, pool.DefaultWeekCap)
	addDuration(poolKeyMaxAge, p.UsageMaxAge, config.DefaultUsageMaxAge)
	addDuration(poolKeyKeyTTL, p.KeyTTL, config.DefaultKeyTTL)
	picker, pickerSrc, timeout, timeoutSrc := poolOffPicker, SourceDefault, durationText(config.DefaultPickerTimeout), SourceDefault
	if p.Picker != nil {
		picker, pickerSrc = p.Picker.Command, SourcePool
		timeout, timeoutSrc = durationOr(p.Picker.Timeout, config.DefaultPickerTimeout)
	}
	add(poolKeyPicker, picker, pickerSrc)
	add(poolKeyPickerTime, timeout, timeoutSrc)
	logPath, logSrc := poolOffLog, SourceDefault
	if p.Log != "" {
		logPath, logSrc = p.Log, SourcePool
	}
	add(poolKeyLog, logPath, logSrc)
	return out
}

// poolRuleSettings are the settings of poolRuleKeys, in that order.
func poolRuleSettings(p config.Pool) []PoolSetting {
	all := poolSettings(p)
	out := make([]PoolSetting, 0, len(poolRuleKeys))
	for _, k := range poolRuleKeys {
		for _, s := range all {
			if s.Key == k {
				out = append(out, s)
			}
		}
	}
	return out
}

// groupCapOr is an own_caps value, or the own default.
func groupCapOr(v *float64, def float64) (float64, string) {
	if v != nil {
		return *v, SourceGroup
	}
	return def, SourceDefault
}

// Member value keys (memberValue).
const (
	memberShort    = "short"
	memberWeek     = "week"
	memberRelaxMax = "week_relax_max"
	memberCapacity = "capacity"
)

// memberValue is one of member m's values (memberShort, memberWeek,
// memberRelaxMax, memberCapacity) with its source and the config key
// that sets it ("" for agx's default): accounts.m, then the group's
// block, then the pool's defaults, then agx's default (own members
// default to 95/98, which defaults does not change).
func memberValue(p config.Pool, m string, g pool.Group, key string) (float64, string, string) {
	acct := p.Accounts[m]
	groupKey, group := "shared_caps", p.SharedCaps
	if g == pool.GroupOwn {
		groupKey, group = "own_caps", p.OwnCaps
	}
	type level struct {
		v        *float64
		src, cfg string
	}
	var levels []level
	var def float64
	switch key {
	case memberShort:
		levels = []level{{acct.Short, SourceMember, "accounts." + m + ".short"}, {group.Short, SourceGroup, groupKey + ".short"}}
		def = pool.DefaultShortCap
		if g == pool.GroupOwn {
			def = pool.DefaultOwnShortCap
		} else {
			levels = append(levels, level{p.Defaults.Short, SourcePool, "defaults.short"})
		}
	case memberWeek:
		levels = []level{{acct.Week, SourceMember, "accounts." + m + ".week"}, {group.Week, SourceGroup, groupKey + ".week"}}
		def = pool.DefaultWeekCap
		if g == pool.GroupOwn {
			def = pool.DefaultOwnWeekCap
		} else {
			levels = append(levels, level{p.Defaults.Week, SourcePool, "defaults.week"})
		}
	case memberRelaxMax:
		levels = []level{{acct.WeekRelaxMax, SourceMember, "accounts." + m + ".week_relax_max"}, {group.WeekRelaxMax, SourceGroup, groupKey + ".week_relax_max"},
			{p.Defaults.WeekRelaxMax, SourcePool, "defaults.week_relax_max"}}
		def = pool.DefaultWeekRelaxMax
	default:
		levels = []level{{acct.Capacity, SourceMember, "accounts." + m + ".capacity"}}
		def = 1
	}
	for _, l := range levels {
		if l.v != nil {
			return *l.v, l.src, l.cfg
		}
	}
	return def, SourceDefault, ""
}

// poolMember is member m's values and enabled state, each with its source.
func poolMember(p config.Pool, m string, g pool.Group) PoolMember {
	pm := PoolMember{Name: m, Group: g, Enabled: true, EnabledSource: SourceDefault}
	pm.Short, pm.ShortSource, _ = memberValue(p, m, g, memberShort)
	pm.Week, pm.WeekSource, _ = memberValue(p, m, g, memberWeek)
	pm.WeekRelaxMax, pm.WeekRelaxMaxSource, _ = memberValue(p, m, g, memberRelaxMax)
	pm.Capacity, pm.CapacitySource, _ = memberValue(p, m, g, memberCapacity)
	if acc, ok := p.Accounts[m]; ok && acc.Enabled != nil {
		pm.Enabled, pm.EnabledSource = *acc.Enabled, SourceMember
	}
	return pm
}

// capOr is a number the pool's block sets, or the default.
func capOr(v *float64, def float64) (float64, string) {
	if v != nil {
		return *v, SourcePool
	}
	return def, SourceDefault
}

// durationOr is a duration as config writes it, or the default written
// the same way ("48h", "60s").
func durationOr(v string, def time.Duration) (string, string) {
	if v != "" {
		return v, SourcePool
	}
	return durationText(def), SourceDefault
}

// durationText writes a default duration as the docs and a config file
// do ("48h", "60s"), rather than time's "48h0m0s" and "1m0s".
func durationText(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d%time.Second == 0:
		return strconv.FormatInt(int64(d/time.Second), 10) + "s"
	}
	return d.String()
}
