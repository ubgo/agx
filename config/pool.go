package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/ubgo/agx/pool"
)

// Pool defaults that are not selection rules (package pool owns those).
const (
	// DefaultUsageMaxAge is how old the login-usage cache may be before a
	// pool probes its members again.
	DefaultUsageMaxAge = 60 * time.Second
	// DefaultKeyTTL is how long the agent keeps a pool key before it asks
	// agx again.
	DefaultKeyTTL = 60 * time.Second
	// DefaultPickerTimeout bounds one run of a pool's picker command: it
	// runs inside the agent's key request, which must not hang.
	DefaultPickerTimeout = 10 * time.Second
	// MinKeyTTL keeps a typo ("1ms") from making the agent ask for a key
	// on every request; Claude Code refuses a shorter helper interval.
	MinKeyTTL = time.Second
)

// Rules turns the pool block into the selection rules, with every
// omitted value at its default. An error names the key.
func (p Pool) Rules() (pool.Config, error) {
	c := pool.New(p.Own, p.Shared)
	if p.Force != nil {
		c.Force = *p.Force
	}
	if p.SharedBorrowWindow != "" {
		d, err := parseDuration("shared_borrow_window", p.SharedBorrowWindow)
		if err != nil {
			return c, err
		}
		c.BorrowWindow = d
	}
	if p.ShortUnavailableAtPercent != nil {
		c.ShortUnavailableAt = *p.ShortUnavailableAtPercent
	}
	if p.Defaults.Short != nil {
		c.Defaults.Short = *p.Defaults.Short
	}
	if p.Defaults.Week != nil {
		c.Defaults.Week = *p.Defaults.Week
	}
	if p.Defaults.WeekRelaxMax != nil {
		c.WeekRelaxMax = *p.Defaults.WeekRelaxMax
	}
	for g, gc := range map[pool.Group]PoolCaps{pool.GroupOwn: p.OwnCaps, pool.GroupShared: p.SharedCaps} {
		if gc != (PoolCaps{}) {
			if c.Groups == nil {
				c.Groups = map[pool.Group]pool.GroupCaps{}
			}
			c.Groups[g] = pool.GroupCaps{Short: gc.Short, Week: gc.Week, WeekRelaxMax: gc.WeekRelaxMax}
		}
	}
	if len(p.Accounts) > 0 {
		c.Accounts = make(map[string]pool.Account, len(p.Accounts))
		for name, a := range p.Accounts {
			acct := pool.Account{Short: a.Short, Week: a.Week, Enabled: a.Enabled, WeekRelaxMax: a.WeekRelaxMax, Capacity: a.Capacity}
			if a.Override != nil {
				o, err := a.Override.Rule(name)
				if err != nil {
					return c, err
				}
				acct.Override = &o
			}
			c.Accounts[name] = acct
		}
	}
	loc, err := p.Location()
	if err != nil {
		return c, err
	}
	c.Location = loc
	if c.Schedules, err = ScheduleRules(p.Schedules); err != nil {
		return c, err
	}
	if err := p.ruleValues(&c); err != nil {
		return c, err
	}
	for _, m := range p.ProbeModels {
		if strings.TrimSpace(m) == "" {
			return c, fmt.Errorf("pool: probe_models has an empty model name")
		}
	}
	if _, err := p.MaxAge(); err != nil {
		return c, err
	}
	if _, err := p.TTL(); err != nil {
		return c, err
	}
	if _, err := p.PickerTimeout(); err != nil {
		return c, err
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// MaxAge is usage_max_age, or DefaultUsageMaxAge when omitted.
func (p Pool) MaxAge() (time.Duration, error) {
	if p.UsageMaxAge == "" {
		return DefaultUsageMaxAge, nil
	}
	return parseDuration("usage_max_age", p.UsageMaxAge)
}

// TTL is key_ttl, or DefaultKeyTTL when omitted; at least MinKeyTTL.
func (p Pool) TTL() (time.Duration, error) {
	if p.KeyTTL == "" {
		return DefaultKeyTTL, nil
	}
	d, err := parseDuration("key_ttl", p.KeyTTL)
	if err != nil {
		return 0, err
	}
	if d < MinKeyTTL {
		return 0, fmt.Errorf("pool: key_ttl %s is below %s", p.KeyTTL, MinKeyTTL)
	}
	return d, nil
}

// PickerTimeout is picker.timeout, or DefaultPickerTimeout when omitted
// (or when there is no picker); a picker needs a command.
func (p Pool) PickerTimeout() (time.Duration, error) {
	if p.Picker == nil {
		return DefaultPickerTimeout, nil
	}
	if strings.TrimSpace(p.Picker.Command) == "" {
		return 0, fmt.Errorf("pool: picker.command is empty; remove picker to use the built-in rules")
	}
	if p.Picker.Timeout == "" {
		return DefaultPickerTimeout, nil
	}
	d, err := parseDuration("picker.timeout", p.Picker.Timeout)
	if err == nil && d == 0 {
		return 0, fmt.Errorf("pool: picker.timeout must be more than 0s")
	}
	return d, err
}

// parseDuration reads a Go duration string that is not negative.
func parseDuration(key, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("pool: %s %q is not a duration (write it like \"48h\", \"60s\")", key, v)
	}
	if d < 0 {
		return 0, fmt.Errorf("pool: %s %q is negative", key, v)
	}
	return d, nil
}

// ruleValues copies the order and the rule values into c, each omitted
// one at its default.
func (p Pool) ruleValues(c *pool.Config) error {
	if p.Order != "" {
		c.Order = pool.Order(p.Order)
	}
	if p.WeekRelaxWindow != "" {
		d, err := parseDuration("week_relax_window", p.WeekRelaxWindow)
		if err != nil {
			return err
		}
		c.WeekRelaxWindow = d
	}
	if p.MaxSessions != nil {
		c.MaxSessions = *p.MaxSessions
	}
	if p.Sticky != nil {
		c.Sticky = *p.Sticky
	}
	if p.StickyLeaveRatio != nil {
		c.StickyLeaveRatio = *p.StickyLeaveRatio
	}
	return nil
}

// Location is the pool's timezone (nil = this machine's local time).
func (p Pool) Location() (*time.Location, error) {
	return loadZone("timezone", p.Timezone)
}

// loadZone reads an IANA time zone name ("" = nil, local time).
func loadZone(key, name string) (*time.Location, error) {
	if name == "" {
		return nil, nil
	}
	if strings.EqualFold(name, "local") {
		return nil, fmt.Errorf("pool: %s %q: leave it out for this machine's local time, or give an IANA name like \"Europe/Istanbul\"", key, name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("pool: %s %q is not an IANA time zone (like \"Europe/Istanbul\", \"America/New_York\", \"UTC\")", key, name)
	}
	return loc, nil
}

// ScheduleRules turns schedule entries into the pool's schedules; an
// error names the schedule and the key. Members are checked by
// pool.Config.Validate. The simulator reads its scenario files' schedules
// through it, so they mean what they mean in config.
func ScheduleRules(in []PoolSchedule) ([]pool.Schedule, error) {
	out := make([]pool.Schedule, 0, len(in))
	for i, s := range in {
		label := fmt.Sprintf("schedules[%d]", i)
		if s.Name != "" {
			label = "schedule " + s.Name
		}
		days, err := pool.ParseDays(s.Days)
		if err != nil {
			return nil, fmt.Errorf("pool: %s: %w", label, err)
		}
		start, end, err := pool.ParseHours(s.Hours)
		if err != nil {
			return nil, fmt.Errorf("pool: %s: %w", label, err)
		}
		loc, err := loadZone(label+" timezone", s.Timezone)
		if err != nil {
			return nil, err
		}
		release := time.Duration(0)
		if s.Caps.Short != nil {
			release = pool.DefaultScheduleReleaseBefore
		}
		if s.ReleaseBefore != "" {
			if release, err = parseDuration(label+" release_before", s.ReleaseBefore); err != nil {
				return nil, err
			}
		}
		out = append(out, pool.Schedule{
			Name: s.Name, Days: days, Start: start, End: end, Location: loc, Members: append([]string{}, s.Members...),
			Caps: pool.ScheduleCaps{Short: s.Caps.Short, Week: s.Caps.Week, WeekRelaxMax: s.Caps.WeekRelaxMax}, ReleaseBefore: release,
		})
	}
	return out, nil
}

// Rule is the override of member name; until must be an RFC 3339 time.
func (o PoolOverride) Rule(name string) (pool.Override, error) {
	t, err := time.Parse(time.RFC3339, o.Until)
	if err != nil {
		return pool.Override{}, fmt.Errorf("pool: accounts.%s.override.until %q is not an RFC 3339 time (like \"2026-11-22T23:59:00+03:00\")", name, o.Until)
	}
	return pool.Override{Caps: pool.ScheduleCaps{Short: o.Short, Week: o.Week, WeekRelaxMax: o.WeekRelaxMax}, Until: t}, nil
}
