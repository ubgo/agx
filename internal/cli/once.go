package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ubgo/agx/internal/apperr"
)

// onceValue wraps the Value of a single-value flag (string, bool, int,
// duration, enum) so a second value in one command line is refused.
//
// Why it exists: pflag keeps the last of two values silently, so
// `agx usage --agent gemini --agent pi` showed only pi. Two values for one
// single-value flag are two choices for one thing, which spec 5.1 rule 9
// refuses with "pick one". Doing it here, once, for every flag of the tree
// (onceFlags) means no command can forget it; repeatable flags (string
// arrays) are never wrapped.
//
// Invariant: one onceValue serves one parse. NewRoot builds a fresh tree
// per invocation, so the remembered first value never leaks between
// command lines.
type onceValue struct {
	pflag.Value
	// name is the flag's long name, used in the refusal whatever form
	// (-p or --profile) the user typed.
	name string
	// first is the value given first; set reports one was given.
	first string
	set   bool
}

// Set refuses a second value, else sets the wrapped Value.
func (v *onceValue) Set(s string) error {
	if v.set {
		return apperr.Conflict(repeatMessage(v.name, v.first, s))
	}
	if err := v.Value.Set(s); err != nil {
		return err
	}
	v.first, v.set = s, true
	return nil
}

// IsBoolFlag forwards pflag's optional bool marker, which pflag reads by
// type assertion on the Value (usage text of a bool flag's default);
// without it a wrapped bool flag would print as a valued one.
func (v *onceValue) IsBoolFlag() bool {
	b, ok := v.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// repeatMessage is the "pick one" refusal: both values when they differ,
// just the flag when the same value was given twice (`--json --json`).
func repeatMessage(name, first, second string) string {
	if first == second {
		return "--" + name + " given twice; pick one"
	}
	return fmt.Sprintf("--%s given twice (%s and %s); pick one", name, first, second)
}

// onceFlags wraps every single-value flag of cmd and its subcommands,
// local and persistent, hidden ones included. A flag already wrapped (a
// persistent flag seen again through a child) is left alone. cobra's own
// --help flag is created lazily at execution, after this walk, and stays
// as cobra made it: repeating it only asks for help twice.
func onceFlags(cmd *cobra.Command) {
	wrap := func(f *pflag.Flag) {
		if _, done := f.Value.(*onceValue); done || flagRepeatable(f) {
			return
		}
		f.Value = &onceValue{Value: f.Value, name: f.Name}
	}
	cmd.PersistentFlags().VisitAll(wrap)
	cmd.Flags().VisitAll(wrap)
	for _, sub := range cmd.Commands() {
		onceFlags(sub)
	}
}
