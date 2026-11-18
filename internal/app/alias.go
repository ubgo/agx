package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/shellinit"
)

// aliasArgsRe is the charset of an alias value. shell-init writes the
// value into a shell alias for four shells; keeping it to these bytes
// means no quoting, expansion or injection is possible in any of them.
var aliasArgsRe = regexp.MustCompile(`^[A-Za-z0-9 _./:=@+-]+$`)

// aliasesKey is the config object holding aliases.
const aliasesKey = "aliases"

// AliasRow is one alias.
type AliasRow struct {
	Name string `json:"name"`
	// Args are the agx arguments the alias runs ("run --profile work").
	Args string `json:"args"`
}

// AliasList is agx alias list.
func (a *App) AliasList() ([]AliasRow, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return nil, err
	}
	rows := []AliasRow{}
	for _, n := range sortedNames(cfg.Config.Aliases) {
		rows = append(rows, AliasRow{Name: n, Args: cfg.Config.Aliases[n]})
	}
	return rows, nil
}

// AliasAddResult is agx alias add.
type AliasAddResult struct {
	Name string `json:"name"`
	Args string `json:"args"`
	// Replaced is true when the alias existed (its old value is Previous).
	Replaced bool   `json:"replaced"`
	Previous string `json:"previous,omitempty"`
}

// AliasAdd is agx alias add: add or replace an alias after checking its
// name, its charset and that it is a valid agx command with named flags
// (the CLI's check, Deps.AliasCheck).
func (a *App) AliasAdd(ctx context.Context, name, args string) (AliasAddResult, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return AliasAddResult{}, err
	}
	if name == "" {
		return AliasAddResult{}, apperr.Usage("--name is required")
	}
	// The name and words are checked by shell-init's own rules, so an alias
	// that is stored is one shell-init writes (never stored, then skipped).
	if err := shellinit.CheckAliasName(name); err != nil {
		return AliasAddResult{}, apperr.Usage(fmt.Sprintf("--name %q: %s", name, err))
	}
	if err := a.checkAlias(args); err != nil {
		return AliasAddResult{}, apperr.Usage("--args: " + err.Error())
	}
	if err := shellinit.CheckAliasArgs(args); err != nil {
		return AliasAddResult{}, apperr.Usage("--args: " + err.Error())
	}
	args = strings.Join(strings.Fields(args), " ")
	prev, had := cfg.Config.Aliases[name]
	res := AliasAddResult{Name: name, Args: args, Replaced: had}
	if had {
		res.Previous = prev
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		return d.Set(config.Pointer(aliasesKey, name), args)
	})
	return res, err
}

// ValidateAlias checks one alias value: not empty, only the safe charset,
// then check (the command tree; nil = charset only).
func ValidateAlias(args string, check AliasChecker) error {
	if strings.TrimSpace(args) == "" {
		return fmt.Errorf("an alias needs agx arguments, such as \"run --profile work\"")
	}
	if !aliasArgsRe.MatchString(args) {
		return fmt.Errorf("%q has a character outside [A-Za-z0-9 _./:=@+-]", args)
	}
	if check != nil {
		return check(args)
	}
	return nil
}

// AliasRemove is agx alias remove --id.
func (a *App) AliasRemove(ctx context.Context, id string) (AliasRow, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return AliasRow{}, err
	}
	if id == "" {
		return AliasRow{}, apperr.Usage("--id is required (an alias name)")
	}
	args, ok := cfg.Config.Aliases[id]
	if !ok {
		return AliasRow{}, apperr.Failure(apperr.CodeNotFound, "unknown alias "+id).WithHint("agx alias list")
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		return d.Delete(config.Pointer(aliasesKey, id))
	})
	return AliasRow{Name: id, Args: args}, err
}
