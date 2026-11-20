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
			c.Accounts[name] = pool.Account{Short: a.Short, Week: a.Week, Enabled: a.Enabled, WeekRelaxMax: a.WeekRelaxMax, Capacity: a.Capacity}
		}
	}
	if err := p.ruleValues(&c); err != nil {
		return c, err
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
