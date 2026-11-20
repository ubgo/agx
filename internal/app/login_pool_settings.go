package app

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// Pool setting flags of login add and edit, named in messages. They
// complete the --pool-own / --pool-shared / --pool-force family: one flag
// per key of logins.<name>.pool, so no pool setting needs a hand edit.
const (
	flagPoolCapShort     = "--pool-cap-short"
	flagPoolCapWeek      = "--pool-cap-week"
	flagPoolDisable      = "--pool-disable"
	flagPoolEnable       = "--pool-enable"
	flagPoolDefaultShort = "--pool-default-short"
	flagPoolDefaultWeek  = "--pool-default-week"
	flagPoolShortCutoff  = "--pool-short-cutoff"
	flagPoolBorrowWindow = "--pool-borrow-window"
	flagPoolUsageMaxAge  = "--pool-usage-max-age"
	flagPoolKeyTTL       = "--pool-key-ttl"
	flagPoolPicker       = "--pool-picker"
	flagDryRun           = "--dry-run"
)

// Keys inside logins.<name>.pool that the setting flags write.
const (
	keyPoolDefaults     = "defaults"
	keyPoolShort        = "short"
	keyPoolWeek         = "week"
	keyPoolEnabled      = "enabled"
	keyPoolCutoff       = "short_unavailable_at_percent"
	keyPoolBorrowWindow = "shared_borrow_window"
	keyPoolUsageMaxAge  = "usage_max_age"
	keyPoolKeyTTL       = "key_ttl"
	keyPoolPicker       = "picker"
	keyPoolCommand      = "command"
)

// capSep separates the member and the value of --pool-cap-short NAME=N.
const capSep = "="

// PoolSettings are the pool flags beyond the member lists and the force,
// shared by login add --kind pool and login edit on a pool. A nil pointer
// or list leaves that key as it is; an empty value removes the key, so
// the documented default applies again.
type PoolSettings struct {
	// CapShort and CapWeek set a member's own caps: NAME=N (a percent);
	// NAME= removes the member's own cap (the pool default applies).
	CapShort []string
	CapWeek  []string
	// Disable leaves members out (enabled false); Enable takes them back.
	Disable []string
	Enable  []string
	// DefaultShort and DefaultWeek are the caps of members without their
	// own (defaults.short, defaults.week).
	DefaultShort *string
	DefaultWeek  *string
	// ShortCutoff is short_unavailable_at_percent.
	ShortCutoff *string
	// BorrowWindow, UsageMaxAge and KeyTTL are Go durations
	// (shared_borrow_window, usage_max_age, key_ttl).
	BorrowWindow *string
	UsageMaxAge  *string
	KeyTTL       *string
	// Picker is picker.command; "" removes the picker (the built-in rules
	// decide again).
	Picker *string
	// The strategy settings (login_pool_strategy.go).
	Strategy PoolStrategySettings
}

// given reports any setting flag.
func (s PoolSettings) given() bool {
	return s.CapShort != nil || s.CapWeek != nil || s.Disable != nil || s.Enable != nil ||
		s.DefaultShort != nil || s.DefaultWeek != nil || s.ShortCutoff != nil ||
		s.BorrowWindow != nil || s.UsageMaxAge != nil || s.KeyTTL != nil || s.Picker != nil || s.Strategy.given()
}

// poolPlan is the set of document edits one pool command makes, with the
// before/after line of each changed key (keys relative to the login).
type poolPlan struct {
	login   string
	edits   []edit
	changes []Change
}

// at is the document path of keys inside logins.<login>.pool.
func (p *poolPlan) at(keys ...string) []string {
	return append([]string{"logins", p.login, keyPool}, keys...)
}

// change records one changed key and its edits; a value that stays the
// same records nothing.
func (p *poolPlan) change(key, from, to string, e ...edit) {
	if from == to {
		return
	}
	p.changes = append(p.changes, Change{Key: keyPool + keySep + key, From: from, To: to})
	p.edits = append(p.edits, e...)
}

// setNumber plans writing a number at keys (encoding/json cannot fail on
// a finite float, and every number here was parsed as finite).
func setNumber(keys []string, v float64) edit {
	return edit{keys: keys, raw: []byte(percentText(v))}
}

// percentText writes a percent the shortest exact way (95, 97.5).
func percentText(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// optPercent shows an optional percent ("" when unset).
func optPercent(v *float64) string {
	if v == nil {
		return absent
	}
	return percentText(*v)
}

// planMembers plans the member lists and the force (edit only; add writes
// them with the new login).
func (p *poolPlan) planMembers(cur config.Pool, own, shared []string, force *string) {
	for _, f := range []struct {
		key       string
		old, want []string
	}{{keyPoolOwn, cur.Own, own}, {keyPoolShared, cur.Shared, shared}} {
		if f.want == nil || slices.Equal(f.old, f.want) {
			continue
		}
		p.change(f.key, strings.Join(f.old, " "), strings.Join(f.want, " "), setTo(p.at(f.key), f.want))
	}
	if force == nil {
		return
	}
	old := absent
	if cur.Force != nil {
		old = *cur.Force
	}
	e := edit{keys: p.at(keyPoolForce), raw: []byte("null")}
	if *force != "" {
		e = setTo(p.at(keyPoolForce), *force)
	}
	p.change(keyPoolForce, old, *force, e)
}

// planSettings plans every setting flag against the pool as it is (cur,
// zero for a new pool); members are the pool's members after this
// command, which a per-member flag must name.
func (p *poolPlan) planSettings(cur config.Pool, members []string, s PoolSettings) error {
	if err := p.planAccounts(cur, members, s); err != nil {
		return err
	}
	if err := p.planCapsBlock(keyPoolDefaults, cur.Defaults, []capFlag{
		{flagPoolDefaultShort, keyPoolShort, s.DefaultShort},
		{flagPoolDefaultWeek, keyPoolWeek, s.DefaultWeek},
		{flagPoolDefaultWeekRelax, keyPoolWeekRelaxMax, s.Strategy.DefaultWeekRelax},
	}); err != nil {
		return err
	}
	if err := p.planStrategy(cur, s.Strategy); err != nil {
		return err
	}
	if s.ShortCutoff != nil {
		if err := p.planPercent(flagPoolShortCutoff, keyPoolCutoff, cur.ShortUnavailableAtPercent, *s.ShortCutoff); err != nil {
			return err
		}
	}
	for _, d := range []struct {
		flag, key, old string
		v              *string
		min            time.Duration
	}{
		{flagPoolBorrowWindow, keyPoolBorrowWindow, cur.SharedBorrowWindow, s.BorrowWindow, 0},
		{flagPoolUsageMaxAge, keyPoolUsageMaxAge, cur.UsageMaxAge, s.UsageMaxAge, 0},
		{flagPoolKeyTTL, keyPoolKeyTTL, cur.KeyTTL, s.KeyTTL, config.MinKeyTTL},
		{flagPoolWeekRelaxWindow, keyPoolWeekRelaxWindow, cur.WeekRelaxWindow, s.Strategy.WeekRelaxWindow, 0},
	} {
		if d.v == nil {
			continue
		}
		if err := checkDuration(d.flag, *d.v, d.min); err != nil {
			return err
		}
		p.change(d.key, d.old, *d.v, setOrDeleteEdit(p.at(d.key), *d.v))
	}
	if s.Picker != nil {
		return p.planPicker(cur, *s.Picker)
	}
	return nil
}

// planPercent plans one top-level percent key ("" removes it).
func (p *poolPlan) planPercent(flag, key string, old *float64, v string) error {
	if v == "" {
		p.change(key, optPercent(old), absent, deleteIf(p.at(key), old != nil))
		return nil
	}
	n, err := parsePercent(flag+" "+v, v)
	if err != nil {
		return err
	}
	p.change(key, optPercent(old), percentText(n), setNumber(p.at(key), n))
	return nil
}

// capFlag is one percent flag of a caps block (defaults, own_caps,
// shared_caps): the flag, the key inside the block and its value (nil =
// not given, "" = remove).
type capFlag struct {
	flag, key string
	v         *string
}

// planCapsBlock plans the given keys of one caps block; a block left
// empty is removed.
func (p *poolPlan) planCapsBlock(block string, cur config.PoolCaps, flags []capFlag) error {
	next := cur
	slots := map[string]**float64{keyPoolShort: &next.Short, keyPoolWeek: &next.Week, keyPoolWeekRelaxMax: &next.WeekRelaxMax}
	for _, f := range flags {
		if f.v == nil {
			continue
		}
		slot := slots[f.key]
		old := *slot
		key := block + keySep + f.key
		if *f.v == "" {
			*slot = nil
			p.change(key, optPercent(old), absent, deleteIf(p.at(block, f.key), old != nil))
			continue
		}
		n, err := parsePercent(f.flag+" "+*f.v, *f.v)
		if err != nil {
			return err
		}
		*slot = &n
		p.change(key, optPercent(old), percentText(n), setNumber(p.at(block, f.key), n))
	}
	if next == (config.PoolCaps{}) && cur != (config.PoolCaps{}) {
		p.edits = append(p.edits, deleteIf(p.at(block), true))
	}
	return nil
}

// memberEdit is one per-member flag value: the member and its new value
// (nil = remove the member's own value).
type memberEdit struct {
	name string
	cap  *float64
}

// parseCaps reads repeated NAME=N values of one cap flag.
func parseCaps(flag string, in []string, members []string, poolName string) ([]memberEdit, error) {
	var out []memberEdit
	for _, kv := range in {
		name, v, ok := strings.Cut(kv, capSep)
		if !ok || name == "" {
			return nil, apperr.Usage(fmt.Sprintf("%s %q: use NAME=N (a number from %s to %s; NAME= removes the member's own cap)", flag, kv, percentText(pool.MinPercent), percentText(pool.MaxPercent)))
		}
		if err := checkMember(flag+" "+kv, name, members, poolName); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(out, func(m memberEdit) bool { return m.name == name }) {
			return nil, apperr.Usage(fmt.Sprintf("%s %s is given twice", flag, name))
		}
		m := memberEdit{name: name}
		if v != "" {
			n, err := parsePercent(flag+" "+kv, v)
			if err != nil {
				return nil, err
			}
			m.cap = &n
		}
		out = append(out, m)
	}
	return out, nil
}

// checkMember refuses a per-member flag for a login that is not one of
// the pool's members (after this command's --pool-own / --pool-shared).
func checkMember(what, name string, members []string, poolName string) error {
	if slices.Contains(members, name) {
		return nil
	}
	return apperr.Usage(fmt.Sprintf("%s: %s is not a member of pool %s (members: %s)", what, name, poolName, strings.Join(members, ", ")))
}

// planAccounts plans the per-member caps and enabled. A member entry left
// empty is removed, and so is an accounts block left empty.
func (p *poolPlan) planAccounts(cur config.Pool, members []string, s PoolSettings) error {
	shorts, err := parseCaps(flagPoolCapShort, s.CapShort, members, p.login)
	if err != nil {
		return err
	}
	weeks, err := parseCaps(flagPoolCapWeek, s.CapWeek, members, p.login)
	if err != nil {
		return err
	}
	relaxes, err := parseCaps(flagPoolCapWeekRelax, s.Strategy.CapWeekRelax, members, p.login)
	if err != nil {
		return err
	}
	for _, f := range []struct {
		flag  string
		names []string
	}{{flagPoolDisable, s.Disable}, {flagPoolEnable, s.Enable}} {
		for _, n := range f.names {
			if err := checkMember(f.flag+" "+n, n, members, p.login); err != nil {
				return err
			}
		}
	}
	for _, n := range s.Disable {
		if slices.Contains(s.Enable, n) {
			return apperr.Conflict(fmt.Sprintf("%s %s and %s %s both set %s's enabled; pick one", flagPoolDisable, n, flagPoolEnable, n, n))
		}
	}
	next := make(map[string]config.PoolAccount, len(cur.Accounts))
	for k, v := range cur.Accounts {
		next[k] = v
	}
	var touched []string
	touch := func(name string) config.PoolAccount {
		if !slices.Contains(touched, name) {
			touched = append(touched, name)
		}
		return next[name]
	}
	for _, m := range shorts {
		a := touch(m.name)
		a.Short = m.cap
		next[m.name] = a
	}
	for _, m := range weeks {
		a := touch(m.name)
		a.Week = m.cap
		next[m.name] = a
	}
	for _, m := range relaxes {
		a := touch(m.name)
		a.WeekRelaxMax = m.cap
		next[m.name] = a
	}
	off := false
	for _, n := range s.Disable {
		a := touch(n)
		a.Enabled = &off
		next[n] = a
	}
	for _, n := range s.Enable {
		a := touch(n)
		// enabled omitted = true, so taking a member back removes the key.
		a.Enabled = nil
		next[n] = a
	}
	for _, name := range touched {
		p.planAccount(name, cur.Accounts[name], next[name])
		if accountEmpty(next[name]) {
			delete(next, name)
		}
	}
	if len(next) == 0 && len(cur.Accounts) > 0 {
		p.edits = append(p.edits, deleteIf(p.at(keyPoolAccounts), true))
	}
	return nil
}

// planAccount plans one member entry: the whole entry goes when nothing
// is left in it, else each changed key is set or removed.
func (p *poolPlan) planAccount(name string, old, next config.PoolAccount) {
	entry := func(keys ...string) []string { return p.at(append([]string{keyPoolAccounts, name}, keys...)...) }
	label := func(k string) string { return keyPoolAccounts + keySep + name + keySep + k }
	empty := accountEmpty(next)
	numberEdit := func(k string, v *float64) edit {
		switch {
		case empty:
			return deleteIf(entry(), true)
		case v == nil:
			return deleteIf(entry(k), true)
		}
		return setNumber(entry(k), *v)
	}
	p.change(label(keyPoolShort), optPercent(old.Short), optPercent(next.Short), numberEdit(keyPoolShort, next.Short))
	p.change(label(keyPoolWeek), optPercent(old.Week), optPercent(next.Week), numberEdit(keyPoolWeek, next.Week))
	p.change(label(keyPoolWeekRelaxMax), optPercent(old.WeekRelaxMax), optPercent(next.WeekRelaxMax), numberEdit(keyPoolWeekRelaxMax, next.WeekRelaxMax))
	enabledEdit := setTo(entry(keyPoolEnabled), false)
	switch {
	case empty:
		enabledEdit = deleteIf(entry(), true)
	case next.Enabled == nil:
		enabledEdit = deleteIf(entry(keyPoolEnabled), true)
	}
	p.change(label(keyPoolEnabled), enabledText(old.Enabled), enabledText(next.Enabled), enabledEdit)
}

// accountEmpty reports a member entry with nothing in it.
func accountEmpty(a config.PoolAccount) bool {
	return a.Short == nil && a.Week == nil && a.Enabled == nil && a.WeekRelaxMax == nil
}

// enabledText is a member's effective enabled (omitted = true).
func enabledText(v *bool) string { return strconv.FormatBool(v == nil || *v) }

// planPicker plans picker.command ("" removes the whole picker block,
// timeout included: a timeout without a command is refused at load).
func (p *poolPlan) planPicker(cur config.Pool, cmd string) error {
	old := absent
	if cur.Picker != nil {
		old = cur.Picker.Command
	}
	switch {
	case cmd == "":
		p.change(keyPoolPicker+keySep+keyPoolCommand, old, absent, deleteIf(p.at(keyPoolPicker), cur.Picker != nil))
	case strings.TrimSpace(cmd) == "":
		return apperr.Usage(flagPoolPicker + ": the command is blank; give '' to remove the picker")
	default:
		p.change(keyPoolPicker+keySep+keyPoolCommand, old, cmd, setTo(p.at(keyPoolPicker, keyPoolCommand), cmd))
	}
	return nil
}

// setOrDeleteEdit plans a string key: "" deletes it.
func setOrDeleteEdit(keys []string, v string) edit {
	if v == "" {
		return deleteIf(keys, true)
	}
	return setTo(keys, v)
}

// parsePercent reads a cap or the cutoff: a finite number from
// pool.MinPercent to pool.MaxPercent. what names the flag and value.
func parsePercent(what, v string) (float64, error) {
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < pool.MinPercent || n > pool.MaxPercent {
		return 0, apperr.Usage(fmt.Sprintf("%s: %s is not a number from %s to %s", what, v, percentText(pool.MinPercent), percentText(pool.MaxPercent)))
	}
	return n, nil
}

// checkDuration refuses a value that is not a Go duration, a negative
// one, or one below min ("" removes the key and is always fine).
func checkDuration(flag, v string, min time.Duration) error {
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	switch {
	case err != nil:
		return apperr.Usage(fmt.Sprintf("%s %q is not a duration (write it like \"48h\", \"60s\")", flag, v))
	case d < 0:
		return apperr.Usage(fmt.Sprintf("%s %q is negative", flag, v))
	case d < min:
		return apperr.Usage(fmt.Sprintf("%s %s is below %s", flag, v, min))
	}
	return nil
}
