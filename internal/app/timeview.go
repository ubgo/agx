package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
)

// TimeOptions are the flags that decide how reset times read, shared by
// agx usage and agx pool pick so both print a reset the same way.
type TimeOptions struct {
	// TZ is --tz: the IANA zone of days and clock times; "" = local.
	TZ string
	// TimeFormat is --time-format (config.TimeFormatValues).
	TimeFormat string
	// DateFormat is the global --date-format (datefmt tokens or preset).
	DateFormat string
	// DurationFormat is --duration-format (config.DurationFormatValues).
	DurationFormat string
}

// TimeView is how a command writes reset times: each value from its flag,
// else its config key, else the default.
type TimeView struct {
	// Loc is the --tz zone.
	Loc *time.Location
	// TimeFormat: --time-format, else usage.time_format, else relative.
	TimeFormat string
	// DateFormat: --date-format, else usage.date_format, else
	// date_format, else the long preset.
	DateFormat datefmt.Format
	// DurationFormat: --duration-format, else usage.duration_format,
	// else truncate.
	DurationFormat string
	// Now is the clock relative times are measured from.
	Now time.Time
}

// checkTimeOptions refuses a bad flag value before anything is read
// (exit 2), naming the values.
func checkTimeOptions(o TimeOptions) error {
	if o.TimeFormat != "" && !slices.Contains(config.TimeFormatValues(), o.TimeFormat) {
		return apperr.Usage(fmt.Sprintf("--time-format %q: use one of %s", o.TimeFormat, strings.Join(config.TimeFormatValues(), ", ")))
	}
	if o.DurationFormat != "" && !slices.Contains(config.DurationFormatValues(), o.DurationFormat) {
		return apperr.Usage(fmt.Sprintf("--duration-format %q: use one of %s", o.DurationFormat, strings.Join(config.DurationFormatValues(), ", ")))
	}
	if o.DateFormat != "" {
		if _, err := datefmt.Parse(o.DateFormat); err != nil {
			return apperr.Usage(fmt.Sprintf("--date-format %q: %s", o.DateFormat, err))
		}
	}
	return nil
}

// timeView resolves o against config c (flag beats config beats default).
func (a *App) timeView(c config.Config, o TimeOptions) (TimeView, error) {
	v := TimeView{Now: a.Now()}
	var err error
	if v.Loc, err = ParseTZ(o.TZ); err != nil {
		return v, err
	}
	if err := checkTimeOptions(o); err != nil {
		return v, err
	}
	v.TimeFormat = cmp.Or(o.TimeFormat, c.Usage.TimeFormat, config.TimeRelative)
	v.DurationFormat = cmp.Or(o.DurationFormat, c.Usage.DurationFormat, config.DurationTruncate)
	// Every configured value was checked when the config loaded
	// (config.CrossCheck); the error stays handled anyway.
	spec := cmp.Or(o.DateFormat, c.Usage.DateFormat, c.DateFormat, datefmt.PresetLong)
	if v.DateFormat, err = datefmt.Parse(spec); err != nil {
		return v, apperr.Usage(fmt.Sprintf("date format %q: %s", spec, err))
	}
	return v, nil
}
