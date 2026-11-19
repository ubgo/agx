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

// Where a pool setting's value comes from (PoolSetting.Source).
const (
	// SourceConfig is a value the pool's block in config sets.
	SourceConfig = "config"
	// SourceDefault is agx's documented default for an omitted key.
	SourceDefault = "default"
	// SourcePoolDefaults is a member cap taken from the pool's defaults
	// key, which config sets (the member has no entry of its own for it).
	SourcePoolDefaults = "defaults"
)

// Pool setting keys, as the pool block in config names them, so a key in
// agx pool show is the one agx config set and the docs use.
const (
	poolKeyForce        = "force"
	poolKeyDefaultShort = "defaults.short"
	poolKeyDefaultWeek  = "defaults.week"
	poolKeyBorrow       = "shared_borrow_window"
	poolKeyCutoff       = "short_unavailable_at_percent"
	poolKeyMaxAge       = "usage_max_age"
	poolKeyKeyTTL       = "key_ttl"
	poolKeyPicker       = "picker.command"
	poolKeyPickerTime   = "picker.timeout"
	poolKeyLog          = "log"
)

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
// writes it, and where the value comes from (config or default).
type PoolSetting struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// PoolMember is one member with the caps and enabled state it gets: its
// own accounts entry (config), else the pool's defaults (defaults), else
// agx's defaults (default).
type PoolMember struct {
	Name          string     `json:"name"`
	Group         pool.Group `json:"group"`
	Enabled       bool       `json:"enabled"`
	EnabledSource string     `json:"enabled_source"`
	Short         float64    `json:"short"`
	ShortSource   string     `json:"short_source"`
	Week          float64    `json:"week"`
	WeekSource    string     `json:"week_source"`
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
	info := PoolInfo{PoolSummary: poolSummary(name, lg), Members: []PoolMember{}}
	defShort, defShortSrc := capOr(p.Defaults.Short, pool.DefaultShortCap)
	defWeek, defWeekSrc := capOr(p.Defaults.Week, pool.DefaultWeekCap)
	for _, m := range p.Own {
		info.Members = append(info.Members, poolMember(p, m, pool.GroupOwn, defShort, defShortSrc, defWeek, defWeekSrc))
	}
	for _, m := range p.Shared {
		info.Members = append(info.Members, poolMember(p, m, pool.GroupShared, defShort, defShortSrc, defWeek, defWeekSrc))
	}
	add := func(key, value, src string) {
		info.Settings = append(info.Settings, PoolSetting{Key: key, Value: value, Source: src})
	}
	addDuration := func(key, v string, def time.Duration) {
		value, src := durationOr(v, def)
		add(key, value, src)
	}
	force, forceSrc := poolOffForce, SourceDefault
	if p.Force != nil {
		force, forceSrc = *p.Force, SourceConfig
	}
	add(poolKeyForce, force, forceSrc)
	add(poolKeyDefaultShort, percentText(defShort), defShortSrc)
	add(poolKeyDefaultWeek, percentText(defWeek), defWeekSrc)
	addDuration(poolKeyBorrow, p.SharedBorrowWindow, pool.DefaultBorrowWindow)
	cutoff, cutoffSrc := capOr(p.ShortUnavailableAtPercent, pool.DefaultShortUnavailableAt)
	add(poolKeyCutoff, percentText(cutoff), cutoffSrc)
	addDuration(poolKeyMaxAge, p.UsageMaxAge, config.DefaultUsageMaxAge)
	addDuration(poolKeyKeyTTL, p.KeyTTL, config.DefaultKeyTTL)
	picker, pickerSrc, timeout, timeoutSrc := poolOffPicker, SourceDefault, durationText(config.DefaultPickerTimeout), SourceDefault
	if p.Picker != nil {
		picker, pickerSrc = p.Picker.Command, SourceConfig
		timeout, timeoutSrc = durationOr(p.Picker.Timeout, config.DefaultPickerTimeout)
	}
	add(poolKeyPicker, picker, pickerSrc)
	add(poolKeyPickerTime, timeout, timeoutSrc)
	logPath, logSrc := poolOffLog, SourceDefault
	if p.Log != "" {
		logPath, logSrc = p.Log, SourceConfig
	}
	add(poolKeyLog, logPath, logSrc)
	return info
}

// poolMember is member m's caps and enabled state, each with its source.
func poolMember(p config.Pool, m string, g pool.Group, defShort float64, defShortSrc string, defWeek float64, defWeekSrc string) PoolMember {
	pm := PoolMember{Name: m, Group: g, Enabled: true, EnabledSource: SourceDefault,
		Short: defShort, ShortSource: memberCapSource(defShortSrc), Week: defWeek, WeekSource: memberCapSource(defWeekSrc)}
	acc, ok := p.Accounts[m]
	if !ok {
		return pm
	}
	if acc.Enabled != nil {
		pm.Enabled, pm.EnabledSource = *acc.Enabled, SourceConfig
	}
	if acc.Short != nil {
		pm.Short, pm.ShortSource = *acc.Short, SourceConfig
	}
	if acc.Week != nil {
		pm.Week, pm.WeekSource = *acc.Week, SourceConfig
	}
	return pm
}

// memberCapSource is where a member without its own cap gets it: the
// pool's defaults key when config sets it, else agx's default.
func memberCapSource(defaultsSrc string) string {
	if defaultsSrc == SourceConfig {
		return SourcePoolDefaults
	}
	return SourceDefault
}

// capOr is a percent the config sets, or the default.
func capOr(v *float64, def float64) (float64, string) {
	if v != nil {
		return *v, SourceConfig
	}
	return def, SourceDefault
}

// durationOr is a duration as config writes it, or the default written
// the same way ("48h", "60s").
func durationOr(v string, def time.Duration) (string, string) {
	if v != "" {
		return v, SourceConfig
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
