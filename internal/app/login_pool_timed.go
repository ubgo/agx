package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/pool"
)

// Timed caps on login add and edit (docs/guides/pool-rules.md
// "Schedules and overrides"): --until turns the per-member cap flags
// into an override that ends, --pool-clear-override ends one early,
// --pool-timezone sets the pool's zone, and --pool-schedule /
// --pool-schedule changes the named keys of a schedule (or creates it),
// --pool-schedule-replace writes a whole one and --pool-schedule-remove
// removes one.
const (
	flagPoolUntil          = "--until"
	flagPoolClearOverride  = "--pool-clear-override"
	flagPoolTimezone       = "--pool-timezone"
	flagPoolSchedule       = "--pool-schedule"
	flagPoolScheduleRemove = "--pool-schedule-remove"
	// flagPoolScheduleReplace writes the whole schedule: the keys it does
	// not give are dropped (--pool-schedule keeps them).
	flagPoolScheduleReplace = "--pool-schedule-replace"
)

// poolTimedFlagsText names the timed-cap flags in "nothing to change".
const poolTimedFlagsText = flagPoolUntil + ", " + flagPoolClearOverride + ", " + flagPoolTimezone + ", " + flagPoolSchedule + ", " + flagPoolScheduleReplace + ", " + flagPoolScheduleRemove

// Keys the timed-cap flags write inside logins.<name>.pool.
const (
	keyPoolOverride  = "override"
	keyPoolTimezone  = "timezone"
	keyPoolSchedules = "schedules"
)

// Keys of a --pool-schedule value ("weekend days=sat,sun members=shared
// short=95"): each is the schedule's config key.
const (
	schedKeyDays          = "days"
	schedKeyHours         = "hours"
	schedKeyMembers       = "members"
	schedKeyTimezone      = "timezone"
	schedKeyShort         = "short"
	schedKeyWeek          = "week"
	schedKeyWeekRelaxMax  = "week_relax_max"
	schedKeyBeforeReset   = "week_before_reset"
	schedKeyReleaseBefore = "release_before"
	// schedListSep separates the days and the members.
	schedListSep = ","
)

// scheduleHelp is how a --pool-schedule value is written.
const scheduleHelp = `write it as "NAME key=value …" with keys days=sat,sun, hours=00:00-24:00, members=shared, timezone=Europe/Istanbul, short=95, week=80, week_relax_max=95, week_before_reset=95, release_before=5h; a new schedule needs days, members and a cap, and key= removes a key`

// PoolTimedSettings are the timed-cap flags; nil = not given.
type PoolTimedSettings struct {
	// Until makes this command's --pool-cap-short, --pool-cap-week and
	// --pool-cap-week-relax an override that ends then: a duration
	// ("6h"), a date and time ("2026-11-23T08:00"), "DAY HH:MM" ("sun
	// 23:59") or "HH:MM", in the pool's timezone.
	Until *string
	// ClearOverride removes members' overrides.
	ClearOverride []string
	// Timezone is the pool's IANA zone ("" removes it: local time).
	Timezone *string
	// Schedules change the given keys of schedules by name, or create
	// them ("NAME key=value …"; key= removes a key).
	Schedules []string
	// ScheduleReplace writes whole schedules by name: keys not given are
	// dropped.
	ScheduleReplace []string
	// ScheduleRemove removes schedules by name.
	ScheduleRemove []string
}

// given reports any timed-cap flag.
func (s PoolTimedSettings) given() bool {
	return s.Until != nil || s.ClearOverride != nil || s.Timezone != nil || s.Schedules != nil || s.ScheduleReplace != nil || s.ScheduleRemove != nil
}

// setValue plans setting keys to an override or the schedule list
// (encoding/json cannot fail on these plain structs).
func setValue[T config.PoolOverride | []config.PoolSchedule](keys []string, v T) edit {
	b, _ := json.Marshal(v) // plain structs of strings, string lists and numbers
	return edit{keys: keys, raw: b}
}

// poolZone is the timezone --until reads its time in: the pool's after
// this command.
func poolZone(cur config.Pool, s PoolTimedSettings) (*time.Location, error) {
	name := cur.Timezone
	if s.Timezone != nil {
		name = *s.Timezone
	}
	loc, err := (config.Pool{Timezone: name}).Location()
	if err != nil {
		return nil, apperr.Usage(err.Error())
	}
	if loc == nil {
		loc = time.Local
	}
	return loc, nil
}

// overrideUntil is --until as the RFC 3339 time an override stores, in
// the pool's zone.
func (p *poolPlan) overrideUntil(cur config.Pool, s PoolTimedSettings) (string, error) {
	loc, err := poolZone(cur, s)
	if err != nil {
		return "", err
	}
	t, err := pool.ParseUntil(*s.Until, p.now, loc)
	if err != nil {
		return "", apperr.Usage(flagPoolUntil + " " + err.Error())
	}
	return t.In(loc).Format(time.RFC3339), nil
}

// applyOverride puts this command's cap flags into member m's override
// that ends at until: an override that still applies keeps its other
// caps (and takes the new end), an ended one starts over; NAME= removes
// that cap from it, and an override left with no cap is removed.
func (p *poolPlan) applyOverride(a config.PoolAccount, until string, short, week, relax *memberEdit) config.PoolAccount {
	var o config.PoolOverride
	if a.Override != nil {
		if r, err := a.Override.Rule(""); err == nil && r.Active(p.now) {
			o = *a.Override
		}
	}
	o.Until = until
	for _, f := range []struct {
		m    *memberEdit
		slot **float64
	}{{short, &o.Short}, {week, &o.Week}, {relax, &o.WeekRelaxMax}} {
		if f.m != nil {
			*f.slot = f.m.cap
		}
	}
	a.Override = &o
	if o.Short == nil && o.Week == nil && o.WeekRelaxMax == nil {
		a.Override = nil
	}
	return a
}

// overrideText is an override in a before/after line: "short 95 until
// 2026-11-22T23:59:00+03:00".
func overrideText(o *config.PoolOverride) string {
	if o == nil {
		return absent
	}
	var parts []string
	for _, f := range []struct {
		key string
		v   *float64
	}{{keyPoolShort, o.Short}, {keyPoolWeek, o.Week}, {keyPoolWeekRelaxMax, o.WeekRelaxMax}} {
		if f.v != nil {
			parts = append(parts, f.key+" "+percentText(*f.v))
		}
	}
	return strings.Join(parts, ", ") + " until " + o.Until
}

// planTimezone plans the pool's timezone ("" removes it).
func (p *poolPlan) planTimezone(cur config.Pool, v string) error {
	if _, err := (config.Pool{Timezone: v}).Location(); err != nil {
		return apperr.Usage(flagPoolTimezone + ": " + strings.TrimPrefix(err.Error(), "pool: "))
	}
	p.change(keyPoolTimezone, cur.Timezone, v, setOrDeleteEdit(p.at(keyPoolTimezone), v))
	return nil
}

// scheduleField is one key of a --pool-schedule value: how it shows in a
// before/after line and how it is set or removed on a schedule.
type scheduleField struct {
	// key is the --pool-schedule key ("short"); label is its path inside
	// the schedule's config entry ("caps.short").
	key, label string
	// text shows the field's value ("" = absent).
	text func(*config.PoolSchedule) string
	// set writes v ("" removes the field); what names the flag and key in
	// an error.
	set func(s *config.PoolSchedule, what, v string) error
}

// stringField is a string key of a schedule (hours, timezone,
// release_before): checked with the whole schedule, by config's rules.
func stringField(key string, slot func(*config.PoolSchedule) *string) scheduleField {
	return scheduleField{key: key, label: key,
		text: func(s *config.PoolSchedule) string { return *slot(s) },
		set:  func(s *config.PoolSchedule, _, v string) error { *slot(s) = v; return nil }}
}

// listField is a comma-separated key (days, members); norm cleans each
// value (days are case-free).
func listField(key string, slot func(*config.PoolSchedule) *[]string, norm func(string) string) scheduleField {
	return scheduleField{key: key, label: key,
		text: func(s *config.PoolSchedule) string { return strings.Join(*slot(s), schedListSep) },
		set: func(s *config.PoolSchedule, _, v string) error {
			*slot(s) = nil
			if v != "" {
				*slot(s) = strings.Split(norm(v), schedListSep)
			}
			return nil
		}}
}

// capField is a cap inside the schedule's caps block: a percent.
func capField(key string, slot func(*config.PoolSchedule) **float64) scheduleField {
	return scheduleField{key: key, label: keyPoolCaps + keySep + key,
		text: func(s *config.PoolSchedule) string { return optPercent(*slot(s)) },
		set: func(s *config.PoolSchedule, what, v string) error {
			if v == "" {
				*slot(s) = nil
				return nil
			}
			n, err := parsePercent(what, v)
			if err != nil {
				return err
			}
			*slot(s) = &n
			return nil
		}}
}

// keyPoolCaps is a schedule's caps block.
const keyPoolCaps = "caps"

// noNorm keeps a list value as given (member names are case-sensitive).
func noNorm(v string) string { return v }

// scheduleFields are the keys of a --pool-schedule value, in the order a
// before/after listing shows them.
var scheduleFields = []scheduleField{
	listField(schedKeyDays, func(s *config.PoolSchedule) *[]string { return &s.Days }, strings.ToLower),
	stringField(schedKeyHours, func(s *config.PoolSchedule) *string { return &s.Hours }),
	listField(schedKeyMembers, func(s *config.PoolSchedule) *[]string { return &s.Members }, noNorm),
	stringField(schedKeyTimezone, func(s *config.PoolSchedule) *string { return &s.Timezone }),
	capField(schedKeyShort, func(s *config.PoolSchedule) **float64 { return &s.Caps.Short }),
	capField(schedKeyWeek, func(s *config.PoolSchedule) **float64 { return &s.Caps.Week }),
	capField(schedKeyWeekRelaxMax, func(s *config.PoolSchedule) **float64 { return &s.Caps.WeekRelaxMax }),
	capField(schedKeyBeforeReset, func(s *config.PoolSchedule) **float64 { return &s.Caps.WeekBeforeReset }),
	stringField(schedKeyReleaseBefore, func(s *config.PoolSchedule) *string { return &s.ReleaseBefore }),
}

// scheduleKeys are the keys a --pool-schedule value may give.
func scheduleKeys() []string {
	out := make([]string, 0, len(scheduleFields))
	for _, f := range scheduleFields {
		out = append(out, f.key)
	}
	return out
}

// findScheduleField finds a field by its key.
func findScheduleField(key string) (scheduleField, bool) {
	i := slices.IndexFunc(scheduleFields, func(f scheduleField) bool { return f.key == key })
	if i < 0 {
		return scheduleField{}, false
	}
	return scheduleFields[i], true
}

// scheduleEdit is one --pool-schedule or --pool-schedule-replace value:
// the schedule's name and its key=value pairs in the order given ("" =
// remove the key).
type scheduleEdit struct {
	flag, name string
	keys, vals []string
}

// what names the flag and schedule in a message.
func (e scheduleEdit) what() string { return e.flag + " " + e.name }

// removes reports a key=, which removes that key.
func (e scheduleEdit) removes(key string) bool {
	i := slices.Index(e.keys, key)
	return i >= 0 && e.vals[i] == ""
}

// parseScheduleEdit reads one value: the name first, then key=value pairs
// of known keys, each once; key= (empty) removes the key.
func parseScheduleEdit(flag, spec string) (scheduleEdit, error) {
	fields := strings.Fields(spec)
	if len(fields) == 0 || strings.Contains(fields[0], "=") {
		return scheduleEdit{}, apperr.Usage(fmt.Sprintf("%s %q: the schedule's name comes first; %s", flag, spec, scheduleHelp))
	}
	e := scheduleEdit{flag: flag, name: fields[0]}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		_, known := findScheduleField(k)
		switch {
		case !ok:
			return e, apperr.Usage(fmt.Sprintf("%s: %q is not key=value; %s", e.what(), f, scheduleHelp))
		case !known:
			return e, apperr.Usage(fmt.Sprintf("%s: unknown key %q (keys: %s)", e.what(), k, strings.Join(scheduleKeys(), ", ")))
		case slices.Contains(e.keys, k):
			return e, apperr.Usage(fmt.Sprintf("%s: %s is given twice", e.what(), k))
		}
		e.keys, e.vals = append(e.keys, k), append(e.vals, v)
	}
	return e, nil
}

// apply writes the edit's keys onto sc (a copy of the schedule as it is,
// or a new one): the keys it does not give stay as they are.
func (e scheduleEdit) apply(sc config.PoolSchedule) (config.PoolSchedule, error) {
	sc.Name = e.name
	for i, k := range e.keys {
		f, _ := findScheduleField(k) // every key was checked by parseScheduleEdit
		if err := f.set(&sc, e.what()+" "+k, e.vals[i]); err != nil {
			return sc, err
		}
	}
	return sc, nil
}

// checkSchedule checks a schedule as this command leaves it, as config
// would load it: days, members and a cap present (a key= that removed one
// is named), members of the pool or a group, and config's own checks of
// days, hours, zone and duration.
func (e scheduleEdit) checkSchedule(sc config.PoolSchedule, members []string) error {
	what := e.what()
	removed := func(key, need string) error {
		if e.removes(key) {
			return apperr.Usage(fmt.Sprintf("%s: %s= removes the schedule's %s, but a schedule needs them: give %s=… instead, or remove the schedule with %s %s", what, key, key, key, flagPoolScheduleRemove, e.name))
		}
		return apperr.Usage(fmt.Sprintf("%s: give %s=… (%s)", what, key, need))
	}
	switch {
	case len(sc.Days) == 0:
		return removed(schedKeyDays, strings.Join(pool.DayNames(), schedListSep))
	case len(sc.Members) == 0:
		return removed(schedKeyMembers, "member names, own or shared")
	case sc.Caps == (config.PoolScheduleCaps{}):
		var gone []string
		for _, k := range []string{schedKeyShort, schedKeyWeek, schedKeyWeekRelaxMax, schedKeyBeforeReset} {
			if e.removes(k) {
				gone = append(gone, k+"=")
			}
		}
		if len(gone) > 0 {
			return apperr.Usage(fmt.Sprintf("%s: %s leaves the schedule no cap, but a schedule needs one: give a cap a number instead, or remove the schedule with %s %s", what, strings.Join(gone, " "), flagPoolScheduleRemove, e.name))
		}
		return apperr.Usage(fmt.Sprintf("%s: give at least one cap: short=, week=, week_relax_max= or week_before_reset=", what))
	}
	for _, m := range sc.Members {
		if m != string(pool.GroupOwn) && m != string(pool.GroupShared) && !slices.Contains(members, m) {
			return apperr.Usage(fmt.Sprintf("%s: %s is neither a member of the pool (%s) nor a group (own, shared)", what, m, strings.Join(members, ", ")))
		}
	}
	if _, err := config.ScheduleRules([]config.PoolSchedule{sc}); err != nil {
		msg := strings.TrimPrefix(strings.TrimPrefix(err.Error(), "pool: "), "schedule "+sc.Name+": ")
		return apperr.Usage(what + ": " + strings.TrimPrefix(msg, "schedule "+sc.Name+" "))
	}
	return nil
}

// planSchedules plans --pool-schedule (change the named keys of a
// schedule, or create it), --pool-schedule-replace (the whole schedule)
// and --pool-schedule-remove. A name may come once per command, so no
// value depends on the order of the flags. The list is written whole,
// and removed when left empty; the before/after lines name each changed
// key of each schedule, so a removed value shows.
func (p *poolPlan) planSchedules(cur config.Pool, members []string, s PoolTimedSettings) error {
	var edits []scheduleEdit
	for _, f := range []struct {
		flag  string
		specs []string
	}{{flagPoolSchedule, s.Schedules}, {flagPoolScheduleReplace, s.ScheduleReplace}} {
		for _, spec := range f.specs {
			e, err := parseScheduleEdit(f.flag, spec)
			if err != nil {
				return err
			}
			edits = append(edits, e)
		}
	}
	if err := scheduleNamesOnce(edits, s.ScheduleRemove); err != nil {
		return err
	}
	next := slices.Clone(cur.Schedules)
	for _, name := range s.ScheduleRemove {
		i := slices.IndexFunc(next, func(x config.PoolSchedule) bool { return x.Name == name })
		if i < 0 {
			return apperr.Usage(fmt.Sprintf("%s %s: pool %s has no schedule %s (it has: %s)", flagPoolScheduleRemove, name, p.login, name, scheduleNames(next)))
		}
		next = slices.Delete(next, i, i+1)
	}
	for _, e := range edits {
		i := slices.IndexFunc(next, func(x config.PoolSchedule) bool { return x.Name == e.name })
		var base config.PoolSchedule
		if i >= 0 && e.flag == flagPoolSchedule {
			base = next[i]
		}
		sc, err := e.apply(base)
		if err != nil {
			return err
		}
		if err := e.checkSchedule(sc, members); err != nil {
			return err
		}
		if i >= 0 {
			next[i] = sc
		} else {
			next = append(next, sc)
		}
	}
	changed := p.scheduleChanges(cur.Schedules, next)
	switch {
	case !changed:
	case len(next) == 0:
		p.edits = append(p.edits, deleteIf(p.at(keyPoolSchedules), true))
	default:
		p.edits = append(p.edits, setValue(p.at(keyPoolSchedules), next))
	}
	return nil
}

// scheduleNamesOnce refuses a schedule named twice in one command (in
// any of --pool-schedule, --pool-schedule-replace and
// --pool-schedule-remove): the result would depend on the flags' order.
func scheduleNamesOnce(edits []scheduleEdit, removes []string) error {
	seen := map[string]string{}
	check := func(flag, name string) error {
		if first, ok := seen[name]; ok {
			return apperr.Conflict(fmt.Sprintf("%s %s and %s %s both change schedule %s; give each schedule once (every key in one value)", first, name, flag, name, name))
		}
		seen[name] = flag
		return nil
	}
	for _, n := range removes {
		if err := check(flagPoolScheduleRemove, n); err != nil {
			return err
		}
	}
	for _, e := range edits {
		if err := check(e.flag, e.name); err != nil {
			return err
		}
	}
	return nil
}

// scheduleChanges records a before/after line for each changed key of
// each schedule (pool.schedules.NAME.KEY), the old schedules first in
// file order, then new ones; it reports whether anything changed.
func (p *poolPlan) scheduleChanges(before, after []config.PoolSchedule) bool {
	var names []string
	for _, list := range [][]config.PoolSchedule{before, after} {
		for _, sc := range list {
			if !slices.Contains(names, sc.Name) {
				names = append(names, sc.Name)
			}
		}
	}
	find := func(list []config.PoolSchedule, name string) *config.PoolSchedule {
		if i := slices.IndexFunc(list, func(x config.PoolSchedule) bool { return x.Name == name }); i >= 0 {
			return &list[i]
		}
		return &config.PoolSchedule{}
	}
	n := len(p.changes)
	for _, name := range names {
		old, cur := find(before, name), find(after, name)
		for _, f := range scheduleFields {
			p.change(keyPoolSchedules+keySep+name+keySep+f.label, f.text(old), f.text(cur))
		}
	}
	return len(p.changes) > n
}

// scheduleNames lists schedule names for a message.
func scheduleNames(in []config.PoolSchedule) string {
	if len(in) == 0 {
		return "none"
	}
	names := make([]string, 0, len(in))
	for _, s := range in {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

// planOverrides puts the cap flags of a command with --until into each
// named member's override (applyOverride); --until without a cap flag is
// refused, as is a member both given a cap and cleared.
func (p *poolPlan) planOverrides(cur config.Pool, s PoolTimedSettings, shorts, weeks, relaxes []memberEdit,
	touch func(string) config.PoolAccount, next map[string]config.PoolAccount) error {
	if len(shorts)+len(weeks)+len(relaxes) == 0 {
		return apperr.Usage(fmt.Sprintf("%s ends the caps of this command's %s, %s or %s NAME=N; give one", flagPoolUntil, flagPoolCapShort, flagPoolCapWeek, flagPoolCapWeekRelax))
	}
	until, err := p.overrideUntil(cur, s)
	if err != nil {
		return err
	}
	find := func(list []memberEdit, name string) *memberEdit {
		if i := slices.IndexFunc(list, func(m memberEdit) bool { return m.name == name }); i >= 0 {
			return &list[i]
		}
		return nil
	}
	var names []string
	for _, list := range [][]memberEdit{shorts, weeks, relaxes} {
		for _, m := range list {
			if !slices.Contains(names, m.name) {
				names = append(names, m.name)
			}
		}
	}
	for _, name := range names {
		if slices.Contains(s.ClearOverride, name) {
			return apperr.Conflict(fmt.Sprintf("%s %s and a cap for %s with %s both change its override; pick one", flagPoolClearOverride, name, name, flagPoolUntil))
		}
		next[name] = p.applyOverride(touch(name), until, find(shorts, name), find(weeks, name), find(relaxes, name))
	}
	return nil
}
