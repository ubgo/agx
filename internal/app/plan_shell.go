package app

import (
	"bytes"
	"sort"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/shellinit"
)

// WarnAlias is the warning code of an alias shell-init skipped.
const WarnAlias = "alias-skipped"

// WarnConfig is the warning code of a config shell-init could not read.
const WarnConfig = "config-unreadable"

// ShellScript is agx shell-init's data.
type ShellScript struct {
	Shell shellinit.Shell `json:"shell"`
	OSC7  bool            `json:"osc7"`
	// Aliases are the alias functions defined, by name.
	Aliases []string `json:"aliases"`
	// Script is the text to evaluate.
	Script string `json:"script"`
	// Warnings are the aliases skipped and a config that did not load.
	Warnings []report.Warning `json:"-"`
}

// ShellInit renders the shell layer for sh (spec 12).
//
// Why it never fails on the config: shell-init runs on every shell start,
// so a broken config.jsonc or one bad alias must not take the wrapper
// down with it. Each is a warning (stderr) instead, and the wrapper and
// the good aliases are still printed.
func (a *App) ShellInit(sh shellinit.Shell, osc7 bool) (ShellScript, error) {
	out := ShellScript{Shell: sh, OSC7: osc7, Aliases: []string{}}
	var aliases []shellinit.Alias
	cfg, err := a.Config()
	switch {
	case err != nil:
		out.Warnings = append(out.Warnings, report.Warning{Code: WarnConfig, Message: "config not loaded, no aliases: " + apperr.Text(err)})
	case cfg.Exists:
		names := make([]string, 0, len(cfg.Config.Aliases))
		for n := range cfg.Config.Aliases {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			al := shellinit.Alias{Name: n, Args: cfg.Config.Aliases[n]}
			if err := shellinit.CheckAlias(al); err != nil {
				out.Warnings = append(out.Warnings, report.Warning{Code: WarnAlias, Message: apperr.Text(err) + "; skipped"})
				continue
			}
			aliases = append(aliases, al)
			out.Aliases = append(out.Aliases, n)
		}
	}
	// The header repeats --config so its load line loads the same file,
	// and the wrapper passes it on every call, so the aliases read from
	// this config run against it. A path the script cannot name is left
	// out with a warning, never a failure (this runs on every shell start).
	configPath := a.d.ConfigFlag
	if err := shellinit.CheckConfigPath(configPath); err != nil {
		out.Warnings = append(out.Warnings, report.Warning{Code: WarnConfig, Message: apperr.Text(err) + "; left out: the layer's commands and aliases run without --config"})
		configPath = ""
	}
	var b bytes.Buffer
	if err := shellinit.Write(&b, shellinit.Params{Shell: sh, OSC7: osc7, Aliases: aliases, ConfigPath: configPath}); err != nil {
		return out, err
	}
	out.Script = b.String()
	return out, nil
}
