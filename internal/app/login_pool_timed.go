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
// --pool-schedule-remove add, replace and remove schedules.
const (
	flagPoolUntil          = "--until"
	flagPoolClearOverride  = "--pool-clear-override"
	flagPoolTimezone       = "--pool-timezone"
	flagPoolSchedule       = "--pool-schedule"
	flagPoolScheduleRemove = "--pool-schedule-remove"
)

// poolTimedFlagsText names the timed-cap flags in "nothing to change".
const poolTimedFlagsText = flagPoolUntil + ", " + flagPoolClearOverride + ", " + flagPoolTimezone + ", " + flagPoolSchedule + ", " + flagPoolScheduleRemove

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

// scheduleKeys are the keys a --pool-schedule value may give.
var scheduleKeys = []string{schedKeyDays, schedKeyHours, schedKeyMembers, schedKeyTimezone, schedKeyShort, schedKeyWeek, schedKeyWeekRelaxMax, schedKeyBeforeReset, schedKeyReleaseBefore}

// scheduleHelp is how a --pool-schedule value is written.
const scheduleHelp = `write it as "NAME days=sat,sun [hours=00:00-24:00] members=shared [timezone=Europe/Istanbul] [short=95] [week=80] [week_relax_max=95] [week_before_reset=95] [release_before=5h]"`

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
	// Schedules add or replace schedules by name ("NAME key=value …").
	Schedules []string
	// ScheduleRemove removes schedules by name.
	ScheduleRemove []string
}

// given reports any timed-cap flag.
func (s PoolTimedSettings) given() bool {
	return s.Until != nil || s.ClearOverride != nil || s.Timezone != nil || s.Schedules != nil || s.ScheduleRemove != nil
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

// planSchedules plans --pool-schedule (add or replace by name) and
// --pool-schedule-remove; the list is written whole, and removed when
// left empty. Each schedule is checked as config would load it.
func (p *poolPlan) planSchedules(cur config.Pool, members []string, s PoolTimedSettings) error {
	next := slices.Clone(cur.Schedules)
	for _, name := range s.ScheduleRemove {
		i := slices.IndexFunc(next, func(x config.PoolSchedule) bool { return x.Name == name })
		if i < 0 {
			return apperr.Usage(fmt.Sprintf("%s %s: pool %s has no schedule %s (it has: %s)", flagPoolScheduleRemove, name, p.login, name, scheduleNames(next)))
		}
		next = slices.Delete(next, i, i+1)
	}
	for _, spec := range s.Schedules {
		sc, err := parseSchedule(spec, members)
		if err != nil {
			return err
		}
		if i := slices.IndexFunc(next, func(x config.PoolSchedule) bool { return x.Name == sc.Name }); i >= 0 {
			next[i] = sc
		} else {
			next = append(next, sc)
		}
	}
	before, after := scheduleList(cur.Schedules), scheduleList(next)
	e := setValue(p.at(keyPoolSchedules), next)
	if len(next) == 0 {
		e = deleteIf(p.at(keyPoolSchedules), len(cur.Schedules) > 0)
	}
	p.change(keyPoolSchedules, before, after, e)
	return nil
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

// scheduleList is the schedules as one before/after value: each as its
// --pool-schedule value, "; " between them.
func scheduleList(in []config.PoolSchedule) string {
	parts := make([]string, 0, len(in))
	for _, s := range in {
		parts = append(parts, scheduleSpec(s))
	}
	return strings.Join(parts, "; ")
}

// scheduleSpec writes a schedule as a --pool-schedule value.
func scheduleSpec(s config.PoolSchedule) string {
	parts := []string{s.Name, schedKeyDays + "=" + strings.Join(s.Days, schedListSep)}
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add(schedKeyHours, s.Hours)
	add(schedKeyMembers, strings.Join(s.Members, schedListSep))
	add(schedKeyTimezone, s.Timezone)
	add(schedKeyShort, optPercent(s.Caps.Short))
	add(schedKeyWeek, optPercent(s.Caps.Week))
	add(schedKeyWeekRelaxMax, optPercent(s.Caps.WeekRelaxMax))
	add(schedKeyBeforeReset, optPercent(s.Caps.WeekBeforeReset))
	add(schedKeyReleaseBefore, s.ReleaseBefore)
	return strings.Join(parts, " ")
}

// parseSchedule reads one --pool-schedule value and checks it: known
// keys, days, hours, zone and duration, caps from 0 to 100, members of
// the pool or a group name.
func parseSchedule(spec string, members []string) (config.PoolSchedule, error) {
	fields := strings.Fields(spec)
	if len(fields) == 0 || strings.Contains(fields[0], "=") {
		return config.PoolSchedule{}, apperr.Usage(fmt.Sprintf("%s %q: the schedule's name comes first; %s", flagPoolSchedule, spec, scheduleHelp))
	}
	sc := config.PoolSchedule{Name: fields[0]}
	what := flagPoolSchedule + " " + sc.Name
	seen := map[string]bool{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		switch {
		case !ok || v == "":
			return sc, apperr.Usage(fmt.Sprintf("%s: %q is not key=value; %s", what, f, scheduleHelp))
		case !slices.Contains(scheduleKeys, k):
			return sc, apperr.Usage(fmt.Sprintf("%s: unknown key %q (keys: %s)", what, k, strings.Join(scheduleKeys, ", ")))
		case seen[k]:
			return sc, apperr.Usage(fmt.Sprintf("%s: %s is given twice", what, k))
		}
		seen[k] = true
		switch k {
		case schedKeyDays:
			sc.Days = strings.Split(strings.ToLower(v), schedListSep)
		case schedKeyHours:
			sc.Hours = v
		case schedKeyMembers:
			sc.Members = strings.Split(v, schedListSep)
		case schedKeyTimezone:
			sc.Timezone = v
		case schedKeyReleaseBefore:
			sc.ReleaseBefore = v
		default:
			n, err := parsePercent(what+" "+k, v)
			if err != nil {
				return sc, err
			}
			slot := map[string]**float64{schedKeyShort: &sc.Caps.Short, schedKeyWeek: &sc.Caps.Week, schedKeyWeekRelaxMax: &sc.Caps.WeekRelaxMax,
				schedKeyBeforeReset: &sc.Caps.WeekBeforeReset}[k]
			*slot = &n
		}
	}
	switch {
	case !seen[schedKeyDays]:
		return sc, apperr.Usage(fmt.Sprintf("%s: give days=… (%s)", what, strings.Join(pool.DayNames(), schedListSep)))
	case !seen[schedKeyMembers]:
		return sc, apperr.Usage(fmt.Sprintf("%s: give members=… (member names, own or shared)", what))
	case sc.Caps == (config.PoolScheduleCaps{}):
		return sc, apperr.Usage(fmt.Sprintf("%s: give at least one cap: short=, week=, week_relax_max= or week_before_reset=", what))
	}
	for _, m := range sc.Members {
		if m != string(pool.GroupOwn) && m != string(pool.GroupShared) && !slices.Contains(members, m) {
			return sc, apperr.Usage(fmt.Sprintf("%s: %s is neither a member of the pool (%s) nor a group (own, shared)", what, m, strings.Join(members, ", ")))
		}
	}
	if _, err := config.ScheduleRules([]config.PoolSchedule{sc}); err != nil {
		msg := strings.TrimPrefix(strings.TrimPrefix(err.Error(), "pool: "), "schedule "+sc.Name+": ")
		return sc, apperr.Usage(what + ": " + strings.TrimPrefix(msg, "schedule "+sc.Name+" "))
	}
	return sc, nil
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
