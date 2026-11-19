package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/ubgo/agx/internal/app"
)

// optPercent is a percent flag that stays nil until it is given, so a
// tighter cap that was not asked for keeps the member's configured cap
// (0 is a real cap: it would block the member).
type optPercent struct{ p **float64 }

// String is the flag's value for help; "" when not given.
func (o optPercent) String() string {
	if o.p == nil || *o.p == nil {
		return ""
	}
	return strconv.FormatFloat(**o.p, 'f', -1, 64)
}

// Set parses the value; the range and the "only lower" rule are the
// app's, so every command words them the same way.
func (o optPercent) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number", s)
	}
	*o.p = &v
	return nil
}

// Type names the value in help.
func (optPercent) Type() string { return "percent" }

// addPoolOnlyFlags declares --pool-only (help onlyHelp) and its tighter
// caps, which apply for scope ("this run", "this pick").
func addPoolOnlyFlags(f *pflag.FlagSet, o *app.PoolOnly, onlyHelp, scope string) {
	f.StringVar(&o.Member, "pool-only", "", onlyHelp)
	f.Var(optPercent{&o.Caps.Short}, "pool-only-short", "with --pool-only: a lower 5-hour cap for that member, for "+scope+" (never above its own)")
	f.Var(optPercent{&o.Caps.Week}, "pool-only-week", "with --pool-only: a lower weekly cap for that member, for "+scope+" (never above its own)")
}
