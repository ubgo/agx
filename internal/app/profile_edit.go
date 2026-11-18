package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// envSep separates NAME and value in --env NAME=value.
const envSep = "="

// ProfileEditOptions are agx profile edit's flags. A field left at its
// zero value (or a nil pointer / nil list) is not changed.
type ProfileEditOptions struct {
	ID        string
	Login     string
	HomeLogin bool
	Home      string
	// Args replaces the profile's args when non-nil (repeat --args); only
	// empty values (--args "") remove them.
	Args []string
	// InheritDefaults sets inherit_defaults when non-nil.
	InheritDefaults *bool
	// Env sets NAME=value; "NAME=" (empty value) removes NAME.
	Env     []string
	Billing string
	DryRun  bool
}

// Change is one key a write changes: its old and new value (secret
// values shown as "(set)").
type Change struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// ProfileEditResult is agx profile edit.
type ProfileEditResult struct {
	Name    string   `json:"name"`
	Changes []Change `json:"changes"`
	// Written is false for --dry-run.
	Written bool `json:"written"`
}

// absent is the From or To of a key that is not set.
const absent = ""

// edit is one planned document change: set raw (JSON) at keys, or delete
// keys when delete is true; neither = nothing to do.
type edit struct {
	keys   []string
	raw    json.RawMessage
	delete bool
}

// setTo plans setting keys to v (a string, bool or list of strings: values
// encoding/json cannot fail on).
func setTo[T string | bool | []string](keys []string, v T) edit {
	b, _ := json.Marshal(v) // encoding/json cannot fail on a string, bool or []string
	return edit{keys: keys, raw: b}
}

// deleteIf plans deleting keys when cond holds.
func deleteIf(keys []string, cond bool) edit { return edit{keys: keys, delete: cond} }

// ProfileEdit is agx profile edit --id: changes login, home, args, env or
// billing of one profile, keeping the rest of the file as it is.
func (a *App) ProfileEdit(ctx context.Context, o ProfileEditOptions) (ProfileEditResult, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return ProfileEditResult{}, err
	}
	if o.ID == "" {
		return ProfileEditResult{}, apperr.Usage("--id is required (a profile name)")
	}
	p, ok := cfg.Config.Profiles[o.ID]
	if !ok {
		return ProfileEditResult{}, unknownProfile(o.ID)
	}
	if err := checkLoginChoice(cfg.Config, p.Agent, o.Login, o.HomeLogin); err != nil {
		return ProfileEditResult{}, err
	}
	if err := checkBilling(o.Billing); err != nil {
		return ProfileEditResult{}, err
	}
	res := ProfileEditResult{Name: o.ID, Changes: []Change{}}
	var edits []edit
	at := func(k ...string) []string { return append([]string{"profiles", o.ID}, k...) }
	change := func(key, from, to string, e ...edit) {
		res.Changes = append(res.Changes, Change{Key: key, From: from, To: to})
		edits = append(edits, e...)
	}
	switch {
	case o.Login != "":
		change(pkLogin, profileLogin(p), o.Login, setTo(at(pkLogin), o.Login), deleteIf(at(pkHomeLogin), p.HomeLogin))
	case o.HomeLogin:
		change(pkLogin, profileLogin(p), HomeLoginText, setTo(at(pkHomeLogin), true), deleteIf(at(pkLogin), p.Login != ""))
	}
	if o.Home != "" {
		dir := a.expand(o.Home)
		if !statDir(dir) {
			return ProfileEditResult{}, apperr.Failure(apperr.CodeHomeMissing, fmt.Sprintf("home %s does not exist", a.tilde(dir))).WithHint("mkdir -p " + dir)
		}
		change(pkHome, p.Home, a.tilde(dir), setTo(at(pkHome), a.tilde(dir)))
	}
	if o.Args != nil {
		args := slices.DeleteFunc(slices.Clone(o.Args), func(s string) bool { return s == "" })
		if len(args) == 0 {
			change(pkArgs, strings.Join(p.Args, " "), absent, deleteIf(at(pkArgs), len(p.Args) > 0))
		} else {
			change(pkArgs, strings.Join(p.Args, " "), strings.Join(args, " "), setTo(at(pkArgs), args))
		}
	}
	if o.InheritDefaults != nil {
		from := fmt.Sprint(p.InheritsDefaults())
		if *o.InheritDefaults {
			// true is the default, so the key is removed rather than written.
			change(pkInherit, from, "true", deleteIf(at(pkInherit), p.InheritDefaults != nil))
		} else {
			change(pkInherit, from, "false", setTo(at(pkInherit), false))
		}
	}
	for _, kv := range o.Env {
		name, val, ok := strings.Cut(kv, envSep)
		if !ok || name == "" {
			return ProfileEditResult{}, apperr.Usage(fmt.Sprintf("--env %q: use NAME=value (NAME= removes it)", kv))
		}
		_, had := p.Env[name]
		from := absent
		if had {
			from = setText
		}
		if val == "" {
			if !had {
				return ProfileEditResult{}, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("--env %s=: profile %s has no env %s", name, o.ID, name))
			}
			change(pkEnv+"."+name, from, absent, deleteIf(at(pkEnv, name), true))
			continue
		}
		change(pkEnv+"."+name, from, setText, setTo(at(pkEnv, name), val))
	}
	if o.Billing != "" {
		change(pkBilling, billingOf(p), o.Billing, setTo(at(pkBilling), o.Billing))
	}
	if len(res.Changes) == 0 {
		return ProfileEditResult{}, apperr.Usage("nothing to change: give --login, --home-login, --home, --args, --inherit-defaults, --no-inherit-defaults, --env or --billing")
	}
	if o.DryRun {
		return res, nil
	}
	err = a.editConfig(ctx, func(d *jsonc.Doc) error { return applyEdits(d, edits) })
	res.Written = err == nil
	return res, err
}

// applyEdits runs planned edits on d. A delete of a key that is not there
// is skipped (an edit plans deletes only for keys it saw, but the file may
// have changed under the lock).
func applyEdits(d *jsonc.Doc, edits []edit) error {
	for _, e := range edits {
		p := config.Pointer(e.keys...)
		switch {
		case e.delete:
			if d.Has(p) {
				if err := d.Delete(p); err != nil {
					return err
				}
			}
		case e.raw != nil:
			if err := d.Set(p, jsonc.Raw(e.raw)); err != nil {
				return err
			}
		}
	}
	return nil
}
