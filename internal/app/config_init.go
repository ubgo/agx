package app

import (
	"context"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/generate"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/picker"
)

// ConfigInitOptions are agx config init's flags.
type ConfigInitOptions struct {
	// DefaultAgent is written as default_agent ("" = ask at a terminal,
	// else left out).
	DefaultAgent agentio.ID
	// Discovered adds every discovered home as a profile (at a terminal
	// without it, a checklist offers them).
	Discovered bool
	// DryRun returns the file without writing it.
	DryRun bool
}

// ConfigInitResult is agx config init.
type ConfigInitResult struct {
	File         string      `json:"file"`
	DefaultAgent agentio.ID  `json:"default_agent"`
	Profiles     []ImportRow `json:"profiles"`
	// Content is the file written (or that would be, with --dry-run).
	Content string `json:"content"`
	Written bool   `json:"written"`
}

// starterDocs are the comments above each top-level key of the starter
// file; the same text is written by generate (for --dry-run) and set on
// the saved file, so both show the same comments. Empty sections are left
// out on purpose: an empty {} keeps a one-line layout, so the first entry
// an agx command adds to it would be written on one line too, while a
// missing section is created in the file's own multi-line layout.
var starterDocs = []struct{ key, doc string }{
	{"version", "agx config: default_agent, agents, profiles, logins, projects, aliases.\n" +
		"Edit it by hand or with agx profile / login / project / alias / config set; comments and layout\n" +
		"are kept on every write. agx config check validates it; agx config show prints each value's source."},
	{"default_agent", "The agent used when no --profile, --agent or folder binding names one."},
	{"profiles", "A profile is a name for one agent home plus launch defaults (agx profile add / import)."},
}

// initDefaultNone is the picker row that leaves default_agent unset.
const initDefaultNone = "none (choose at each run)"

// ConfigInit is agx config init: a commented starter config.jsonc built
// with jsonc/generate. At a terminal it asks for default_agent (unless
// --default-agent) and offers the discovered homes (unless --discovered,
// which takes them all). An existing file is never overwritten.
func (a *App) ConfigInit(ctx context.Context, o ConfigInitOptions) (ConfigInitResult, error) {
	res := ConfigInitResult{File: a.paths.File, Profiles: []ImportRow{}}
	if fileExists(a.paths.File) {
		return res, a.configExists().
			WithHint("agx config check\nagx config set --key KEY --value VALUE")
	}
	_, reg, err := a.loadedConfig()
	if err != nil {
		return res, err
	}
	if o.DefaultAgent != "" {
		if _, ok := reg.Get(o.DefaultAgent); !ok {
			return res, unknownAgent(reg, string(o.DefaultAgent))
		}
	}
	plan, err := a.ProfileImport(ctx, ProfileImportOptions{Discovered: true})
	if err != nil {
		return res, err
	}
	pk := a.d.Picker
	switch {
	case o.Discovered:
		res.Profiles = plan.Profiles
	case pk != nil && len(plan.Profiles) > 0:
		items := make([]picker.Item, len(plan.Profiles))
		for i, r := range plan.Profiles {
			items[i] = picker.Item{Label: r.Name, Detail: string(r.Agent) + "  " + r.Home}
		}
		picked, err := pk.Checklist(ctx, "Add these homes as profiles?", items)
		if err != nil {
			return res, err
		}
		res.Profiles = defaultsAfterPick(plan.Profiles, picked)
	}
	res.DefaultAgent = o.DefaultAgent
	if res.DefaultAgent == "" && pk != nil {
		ids := reg.IDs()
		items := make([]picker.Item, 0, len(ids)+1)
		for _, id := range ids {
			items = append(items, picker.Item{Label: string(id)})
		}
		items = append(items, picker.Item{Label: initDefaultNone})
		i, err := pk.Choose(ctx, "Default agent (default_agent)?", items)
		if err != nil {
			return res, err
		}
		if i < len(ids) {
			res.DefaultAgent = ids[i]
		}
	}
	f := starter(res.DefaultAgent, res.Profiles)
	b, err := f.Bytes()
	if err != nil {
		return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "generate config: "+err.Error())
	}
	res.Content = string(b)
	if o.DryRun {
		return res, nil
	}
	gd, err := f.Doc(jsonc.WithFileName(config.ConfigFileName))
	if err != nil {
		return res, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "generate config: "+err.Error())
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error {
		if d.Existed() {
			return a.configExists()
		}
		// EditConfig wrote "version" first; drop it so the merged starter
		// keeps its own order ($schema first).
		if err := d.Delete(config.Pointer("version")); err != nil {
			return err
		}
		if err := d.Merge(gd); err != nil {
			return err
		}
		for _, sd := range starterDocs {
			if d.Has(config.Pointer(sd.key)) {
				if err := d.SetComment(config.Pointer(sd.key), sd.doc); err != nil {
					return err
				}
			}
		}
		return nil
	})
	res.Written = err == nil
	return res, err
}

// defaultsAfterPick keeps the picked rows and moves "default" to the first
// picked row of each agent, so an agent whose planned default was not
// picked still gets one.
func defaultsAfterPick(rows []ImportRow, picked []int) []ImportRow {
	out := []ImportRow{}
	seen := map[agentio.ID]bool{}
	for _, i := range picked {
		r := rows[i]
		r.Default = !seen[r.Agent]
		seen[r.Agent] = true
		out = append(out, r)
	}
	return out
}

// starter builds the starter file: every top-level key with its comment.
func starter(defaultAgent agentio.ID, profiles []ImportRow) *generate.File {
	f := generate.New(generate.SchemaURL(config.SchemaURL))
	root := f.Root()
	doc := map[string]string{}
	for _, sd := range starterDocs {
		doc[sd.key] = sd.doc
	}
	root.Set("version", config.CurrentVersion, generate.Doc(doc["version"]))
	if defaultAgent != "" {
		root.Set("default_agent", string(defaultAgent), generate.Doc(doc["default_agent"]), generate.BlankBefore())
	}
	if len(profiles) > 0 {
		po := root.Object("profiles", generate.Doc(doc["profiles"]), generate.BlankBefore())
		for _, r := range profiles {
			p := po.Object(r.Name).Set("agent", string(r.Agent)).Set("home", r.Home)
			if r.Default {
				p.Set("default", true)
			}
		}
	}
	return f
}

// configExists is config init's refusal for a file that is already there
// (checked up front, and again under the lock in case another agx wrote it
// meanwhile). The file is named in the ~ form, as in every other message;
// the absolute path stays in the result's File for scripts.
func (a *App) configExists() *apperr.Error {
	return apperr.Failure(apperr.CodeConfig, a.tilde(a.paths.File)+" already exists")
}
