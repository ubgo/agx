// Package aliasrule holds the one rule for a config alias that the shell
// layer can write: its name and its words. It is a leaf package (no agx
// imports) so that every place that meets an alias applies the same rule:
//
//   - config load (config.Loaded.CrossCheck) refuses a hand-written alias
//     that breaks it, at the alias's file:line:col;
//   - agx alias add refuses it before storing it;
//   - shell-init (internal/shellinit) checks it again before writing a
//     function, as a last guard.
//
// Why a separate package: internal/shellinit imports internal/resolve,
// which imports config, so config cannot import shellinit itself.
package aliasrule

import (
	"fmt"
	"regexp"
	"strings"
)

// BinaryName is agx's own command name: the wrapper function the shell
// layer defines, so no alias may take it.
const BinaryName = "agx"

// namePattern is a function name every supported shell (bash, zsh, fish,
// pwsh) accepts without quoting.
var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// argsPattern is the alias value charset of spec 12: nothing a shell
// treats specially, so the words are emitted verbatim.
var argsPattern = regexp.MustCompile(`^[A-Za-z0-9 _./:=@+-]*$`)

// Word prefixes a shell would expand in an unquoted word: zsh turns =word
// into a path, pwsh splats @word.
const (
	zshPathPrefix   = "="
	pwshSplatPrefix = "@"
)

// reservedOrder are the names an alias must not take, in the fixed order
// messages list them: the wrapper itself, and the names a shell resolves
// before functions or relies on. reserved is the same set for lookups.
var (
	reservedOrder = []string{BinaryName, "command", "builtin", "cd", "set", "function"}
	reserved      = func() map[string]bool {
		m := make(map[string]bool, len(reservedOrder))
		for _, n := range reservedOrder {
			m[n] = true
		}
		return m
	}()
)

// Check validates one alias: CheckName, then CheckArgs. Its errors name the
// alias (`alias "agx": …` for a name, `alias cl: …` for its words), so
// callers print them as they are.
func Check(name, args string) error {
	if err := CheckName(name); err != nil {
		return fmt.Errorf("alias %q: %w", name, err)
	}
	if err := CheckArgs(args); err != nil {
		return fmt.Errorf("alias %s: %w", name, err)
	}
	return nil
}

// CheckName checks an alias name: a function name every shell accepts
// without quoting, and not a reserved name. The error does not repeat the
// name; Check and callers add it.
func CheckName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("a name is letters, digits, _ and -, not starting with a digit or -")
	}
	if reserved[name] {
		return fmt.Errorf("a name may not be %s", strings.Join(reservedOrder, ", "))
	}
	return nil
}

// CheckArgs checks an alias value for the scripts. Beyond the charset it
// refuses a word starting with "=" or "@" (see the prefixes above) and an
// empty value.
func CheckArgs(args string) error {
	if strings.TrimSpace(args) == "" {
		return fmt.Errorf("empty arguments")
	}
	if !argsPattern.MatchString(args) {
		return fmt.Errorf("arguments may hold only letters, digits, space and _ . / : = @ + -")
	}
	for _, w := range strings.Fields(args) {
		if strings.HasPrefix(w, zshPathPrefix) || strings.HasPrefix(w, pwshSplatPrefix) {
			return fmt.Errorf("word %q may not start with %s or %s", w, zshPathPrefix, pwshSplatPrefix)
		}
	}
	return nil
}
