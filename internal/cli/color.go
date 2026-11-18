package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agx/internal/apperr"
)

// Color values (--color). What gets coloured is render's choice
// (internal/render/color.go: the human output on stdout in the v0.3
// look, and agx's own error and warning lines on stderr); this file only
// decides whether colour is on, once per stream.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// ColorValues lists the --color values in the order help and errors show
// them.
func ColorValues() []string { return []string{ColorAuto, ColorAlways, ColorNever} }

// envNoColor is the no-color.org variable: present and not empty turns
// colour off unless --color always is given.
const envNoColor = "NO_COLOR"

// colorFlag is the --color Value: it refuses anything but ColorValues at
// parse time, so `--color purple` is a usage error (exit 2) instead of a
// value nobody reads.
//
// Type() is "string" on purpose: help --json and the flag tables show
// --color as the string flag it always was.
type colorFlag struct {
	// mode is the globals field the parsed value lands in.
	mode *string
}

// String is the current value (pflag shows it as the default).
func (f *colorFlag) String() string { return *f.mode }

// Type is pflag's type name for help.
func (f *colorFlag) Type() string { return "string" }

// Set checks v against ColorValues.
func (f *colorFlag) Set(v string) error {
	if !slices.Contains(ColorValues(), v) {
		return apperr.Usage(fmt.Sprintf("--color %q: use one of %s", v, strings.Join(ColorValues(), ", ")))
	}
	*f.mode = v
	return nil
}

// colorOn decides colour for one stream (spec 13 global flags,
// no-color.org): "always" colours even with NO_COLOR set, because an
// explicit command-line choice wins over the environment; "never" never
// does; "auto" (the default) colours only when that stream is a terminal
// and NO_COLOR is absent or empty. lookup reads agx's environment.
func colorOn(mode string, lookup func(string) (string, bool), streamTTY bool) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}
	if lookup != nil {
		if v, ok := lookup(envNoColor); ok && v != "" {
			return false
		}
	}
	return streamTTY
}
