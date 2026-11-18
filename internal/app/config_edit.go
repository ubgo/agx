package app

import (
	"context"
	"strings"

	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
)

// AliasChecker validates one alias value ("run --profile work") against
// the command tree. Only the CLI knows the commands, so it builds the
// checker (Deps.AliasCheck wraps it with ValidateAlias).
type AliasChecker func(args string) error

// known is what config validation checks against: the built-in agents,
// their typed block schemas and the alias check (as at load).
func (a *App) known() config.Known {
	return config.Known{Agents: agents.BuiltinIDs(), Blocks: agents.BlockSchemas(), Alias: a.checkAlias}
}

// checkAlias validates one alias value: the charset always, the command
// tree through Deps.AliasCheck when the CLI supplied it.
func (a *App) checkAlias(args string) error {
	if a.d.AliasCheck != nil {
		return a.d.AliasCheck(args)
	}
	return ValidateAlias(args, nil)
}

// editConfig changes config.jsonc under the cross-process lock, keeping
// comments and layout (config.EditConfig), and refuses a change that adds
// a config problem: the edited bytes are validated like a load (schema and
// cross-checks) before anything is written, so an agx command never leaves
// a file that the next agx refuses to load. Problems that were already
// there are not blamed on this change (they are compared by path and
// message, since positions move with the edit). The cached config is
// dropped afterwards so a later step of the same invocation sees the
// change.
func (a *App) editConfig(ctx context.Context, fn func(d *jsonc.Doc) error) error {
	err := config.EditConfig(ctx, a.paths, func(d *jsonc.Doc) error {
		before := a.problemKeys(d.Bytes())
		if err := fn(d); err != nil {
			return err
		}
		var added []string
		for _, p := range a.problems(d.Bytes()) {
			if !before[p.Path+"\x00"+p.Message] {
				added = append(added, p.Message)
			}
		}
		if len(added) > 0 {
			return apperr.New(apperr.CodeConfig, apperr.ClassUsage, "refused: the change would make config.jsonc invalid: "+added[0]).
				WithHint(strings.Join(added, "\n"))
		}
		return nil
	})
	a.Reload()
	return err
}

// problems validates src as a config and returns every problem (none for
// a valid file). Empty source (a file being created) has none.
func (a *App) problems(src []byte) []config.Problem {
	if len(strings.TrimSpace(string(src))) == 0 {
		return nil
	}
	if _, err := config.Parse(src, a.paths, a.known()); err != nil {
		return config.ProblemsOf(err)
	}
	return nil
}

// problemKeys indexes problems by path and message.
func (a *App) problemKeys(src []byte) map[string]bool {
	out := map[string]bool{}
	for _, p := range a.problems(src) {
		out[p.Path+"\x00"+p.Message] = true
	}
	return out
}

// loadedConfig returns the loaded config for a command that needs a valid
// file; a missing file is an empty config (Exists false).
func (a *App) loadedConfig() (*config.Loaded, *agents.Registry, error) {
	cfg, err := a.Config()
	if err != nil {
		return nil, nil, err
	}
	reg, err := a.Registry()
	if err != nil {
		return nil, nil, err
	}
	return cfg, reg, nil
}

// expand makes a user-typed path absolute ("~" and ${VAR} expanded).
func (a *App) expand(p string) string { return config.ExpandPath(p, a.d.Lookup, a.d.Host.UserHome) }

// normDir is the project-key form of a folder (spec 13.6).
func (a *App) normDir(p string) string { return config.NormalizeDir(p, a.d.Lookup, a.d.Host.UserHome) }

// dirKey is a folder's comparison form (case-folded where the OS folds):
// only for comparing, never for printing (it is lower-cased and
// symlink-resolved on macOS).
func (a *App) dirKey(p string) string { return config.DirKeyFor(a.foldOS, a.normDir(p)) }
