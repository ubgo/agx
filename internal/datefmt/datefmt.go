// Package datefmt is the date/time format a person writes for agx's human
// output (--date-format, config date_format and usage.date_format): a
// small token language ("ddd D MMM HH:mm") and four presets, instead of
// Go's reference-time layouts, which few people can write from memory.
//
// Why a parsed list of parts rather than one translated Go layout: Go has
// no unpadded 24-hour hour (the H token), and literal text copied into a
// Go layout is reinterpreted when it holds a layout word ("[Mon]" would
// print the weekday). Each token still formats through its Go layout
// where one exists, so the spelling of months, weekdays and am/pm is Go's.
//
// The package is a leaf: config validates a configured format with it,
// internal/app resolves agx usage's format, and the CLI formats cells.
package datefmt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Preset names: a whole format in one word.
const (
	// PresetISO is "2026-11-19 23:30", the default of table timestamp
	// cells and conversation show.
	PresetISO = "iso"
	// PresetShort is "Thu 23:30".
	PresetShort = "short"
	// PresetLong is "Thu 19 Nov 23:30", the default of agx usage's
	// absolute times and RESETS ON column.
	PresetLong = "long"
	// PresetUS is "Nov 19 11:30 PM".
	PresetUS = "us"
)

// presetSpecs is each preset's format, in token form.
var presetSpecs = map[string]string{
	PresetISO:   "YYYY-MM-DD HH:mm",
	PresetShort: "ddd HH:mm",
	PresetLong:  "ddd D MMM HH:mm",
	PresetUS:    "MMM D h:mm A",
}

// Presets is the canonical list of preset names, for help and errors.
func Presets() []string { return []string{PresetISO, PresetShort, PresetLong, PresetUS} }

// tokenOrder lists every token for help and errors, date parts first.
var tokenOrder = []string{"YYYY", "MMM", "MM", "DD", "D", "dddd", "ddd", "HH", "H", "hh", "h", "mm", "ss", "a", "A"}

// Tokens is the canonical list of tokens, for help and errors.
func Tokens() []string { return append([]string(nil), tokenOrder...) }

// layout formats a time with one Go layout element.
func layout(l string) func(time.Time) string {
	return func(t time.Time) string { return t.Format(l) }
}

// tokens maps each token to its formatter: a Go layout element where Go
// has one ("MMM" is "Jan"), else code (H: Go pads every 24-hour hour).
var tokens = map[string]func(time.Time) string{
	"YYYY": layout("2006"),
	"MMM":  layout("Jan"),
	"MM":   layout("01"),
	"DD":   layout("02"),
	"D":    layout("2"),
	"dddd": layout("Monday"),
	"ddd":  layout("Mon"),
	"HH":   layout("15"),
	"H":    func(t time.Time) string { return strconv.Itoa(t.Hour()) },
	"hh":   layout("03"),
	"h":    layout("3"),
	"mm":   layout("04"),
	"ss":   layout("05"),
	"a":    layout("pm"),
	"A":    layout("PM"),
}

// clockTokens are the tokens of the time of day: the clock part of a
// format runs from its first to its last clock token.
var clockTokens = map[string]bool{"HH": true, "H": true, "hh": true, "h": true, "mm": true, "ss": true, "a": true, "A": true}

// defaultClock is the clock part of a format with no clock token
// ("YYYY-MM-DD"), so a view that needs a time of day still has one.
const defaultClock = "HH:mm"

// Literal text delimiters: "[at] HH:mm" prints "at 23:30".
const (
	literalOpen  = '['
	literalClose = ']'
)

// part is one piece of a format: a token, or literal text.
type part struct {
	token string
	lit   string
}

// Format is a parsed date format. The zero value is not usable; get one
// from Parse or Preset.
type Format struct {
	spec  string
	parts []part
	clock []part
}

// Parse reads spec: a preset name, or tokens with any punctuation and
// spaces between them and words in [brackets]. A letter outside brackets
// that is not a token, an unclosed bracket, a format without a single
// token, or an empty spec is an error naming the tokens and presets
// (callers put the flag or key and the value in front of it).
func Parse(spec string) (Format, error) {
	if p, ok := presetSpecs[spec]; ok {
		f, err := parseTokens(p)
		f.spec = spec
		return f, err
	}
	f, err := parseTokens(spec)
	if err != nil {
		return Format{}, fmt.Errorf("%w; %s", err, Help())
	}
	return f, nil
}

// Preset is the named preset's Format; an unknown name panics, since the
// names are this package's constants and every one is tested to parse.
func Preset(name string) Format {
	p, ok := presetSpecs[name]
	if !ok {
		panic("datefmt: unknown preset " + name)
	}
	f, err := parseTokens(p)
	if err != nil {
		panic("datefmt: preset " + name + ": " + err.Error())
	}
	f.spec = name
	return f
}

// Help is the one-line list of tokens and presets shown with every error.
func Help() string {
	return "tokens: " + strings.Join(tokenOrder, " ") + ", literal text in [brackets]; presets: " + strings.Join(Presets(), ", ")
}

// parseTokens splits spec into parts: a run of one repeated letter is a
// token ("MMM"), "[…]" is literal, any other rune is literal.
func parseTokens(spec string) (Format, error) {
	if strings.TrimSpace(spec) == "" {
		return Format{}, errors.New("empty format")
	}
	var parts []part
	lit := func(s string) {
		if n := len(parts); n > 0 && parts[n-1].token == "" {
			parts[n-1].lit += s
			return
		}
		parts = append(parts, part{lit: s})
	}
	hasToken := false
	for i := 0; i < len(spec); {
		r, size := utf8.DecodeRuneInString(spec[i:])
		switch {
		case r == literalOpen:
			end := strings.IndexRune(spec[i+size:], literalClose)
			if end < 0 {
				return Format{}, fmt.Errorf("unclosed %c", literalOpen)
			}
			lit(spec[i+size : i+size+end])
			i += size + end + utf8.RuneLen(literalClose)
		case unicode.IsLetter(r):
			j := i + size
			for j < len(spec) {
				next, n := utf8.DecodeRuneInString(spec[j:])
				if next != r {
					break
				}
				j += n
			}
			run := spec[i:j]
			if _, ok := tokens[run]; !ok {
				return Format{}, fmt.Errorf("unknown token %q", run)
			}
			parts = append(parts, part{token: run})
			hasToken = true
			i = j
		default:
			lit(string(r))
			i += size
		}
	}
	if !hasToken {
		return Format{}, errors.New("no token")
	}
	f := Format{spec: spec, parts: parts, clock: clockSpan(parts)}
	if f.clock == nil {
		def, err := parseTokens(defaultClock)
		if err != nil {
			return Format{}, err
		}
		f.clock = def.parts
	}
	return f, nil
}

// clockSpan is the parts from the first clock token to the last, with the
// literals between them ("h:mm A" of "MMM D h:mm A"); nil without one.
func clockSpan(parts []part) []part {
	first, last := -1, -1
	for i, p := range parts {
		if clockTokens[p.token] {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return nil
	}
	return parts[first : last+1]
}

// IsZero reports a Format that was never parsed (a report built by hand).
func (f Format) IsZero() bool { return f.parts == nil }

// String is the format as written (a preset name stays its name).
func (f Format) String() string { return f.spec }

// RoundMinute is the one rounding rule of every clock time agx prints:
// the nearest minute, as agx v0.3 printed resets (a reset at 22:19:31
// reads 22:20). Go's Format truncates the seconds instead, so without
// this one path printed 22:19 and another 22:20 for the same reset.
// Callers that print a part of the time outside a Format (the weekday of
// RESET DAY, a fixed Go layout) round with this first, so a reset at
// 23:59:40 is the next day's 00:00 in every column. Durations keep their
// own rule (human.Until truncates, never claiming more time than is left).
func RoundMinute(t time.Time) time.Time { return t.Round(time.Minute) }

// Round is t rounded for this format: to the nearest minute (RoundMinute),
// or to the nearest second when the format prints seconds (the ss token),
// since a person who asked for seconds should not see a constant :00.
// Date and Clock round with it; a caller printing extra parts of the same
// time (a weekday beside Clock) rounds with it first so every part agrees.
func (f Format) Round(t time.Time) time.Time {
	if f.hasSeconds() {
		return t.Round(time.Second)
	}
	return RoundMinute(t)
}

// hasSeconds reports a format with the ss token.
func (f Format) hasSeconds() bool {
	for _, p := range f.parts {
		if p.token == secondsToken {
			return true
		}
	}
	return false
}

// secondsToken is the token of the seconds, which changes Round's unit.
const secondsToken = "ss"

// Date is t in the whole format, in t's own zone (callers pick the zone),
// rounded by Round.
func (f Format) Date(t time.Time) string { return render(f.parts, f.Round(t)) }

// Clock is t's time of day in the format's clock part ("23:30" of the
// long preset, "11:30 PM" of us), rounded by Round.
func (f Format) Clock(t time.Time) string { return render(f.clock, f.Round(t)) }

// render joins parts for t.
func render(parts []part, t time.Time) string {
	var b strings.Builder
	for _, p := range parts {
		if p.token == "" {
			b.WriteString(p.lit)
			continue
		}
		b.WriteString(tokens[p.token](t))
	}
	return b.String()
}
