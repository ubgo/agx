package app

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
)

// ConfigPathResult is agx config path.
type ConfigPathResult struct {
	// File is config.jsonc (or the --config file).
	File string `json:"file"`
	// Dir is the config dir.
	Dir string `json:"dir"`
	// Exists is false before config init.
	Exists bool `json:"exists"`
	// Source says which rule chose the dir (--config, AGX_CONFIG_DIR,
	// AGX_HOME or the default).
	Source string `json:"source"`
}

// ConfigPath is agx config path. It works without a valid config.
func (a *App) ConfigPath() ConfigPathResult {
	src := "default (~/" + config.DefaultDirName + ")"
	switch {
	case a.d.ConfigFlag != "":
		src = "--config"
	default:
		for _, name := range []string{config.EnvConfigDir, config.EnvHome} {
			if v, ok := a.d.Lookup(name); ok && v != "" {
				src = name
				break
			}
		}
	}
	return ConfigPathResult{File: a.paths.File, Dir: a.paths.Dir, Exists: fileExists(a.paths.File), Source: src}
}

// fileExists reports an existing regular file.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ConfigCheckResult is agx config check on a valid file.
type ConfigCheckResult struct {
	File     string `json:"file"`
	Exists   bool   `json:"exists"`
	Agents   int    `json:"agents"`
	Profiles int    `json:"profiles"`
	Logins   int    `json:"logins"`
	Projects int    `json:"projects"`
	Aliases  int    `json:"aliases"`
}

// ConfigCheck is agx config check: load, validate and cross-check the
// file (spec 15.3), with aliases checked against the command tree. Every
// problem is returned (edge 45): the error's message is the only problem
// or a count, and its hint lists each one with its file:line:col.
func (a *App) ConfigCheck() (ConfigCheckResult, error) {
	res := ConfigCheckResult{File: a.paths.File}
	l, err := config.Load(a.paths, a.known())
	if err == nil && l.Exists {
		custom := map[agentio.ID]agentio.Spec{}
		for id, ac := range l.Config.Agents {
			if ac.Spec != nil {
				custom[id] = *ac.Spec
			}
		}
		if _, rerr := agents.New(a.d.Host, custom); rerr != nil {
			err = apperr.Wrap(rerr, apperr.CodeConfig, apperr.ClassFailure, rerr.Error())
		}
	}
	if err != nil {
		ae := apperr.As(err)
		ps := config.ProblemsOf(err)
		lines := make([]string, len(ps))
		for i, p := range ps {
			// Cross-check messages leave the key out; name it, so each line
			// says what to fix without opening the file.
			if p.Path != "" && !strings.Contains(p.Message, p.Path) {
				p.Message = p.Path + ": " + p.Message
			}
			lines[i] = p.String()
		}
		msg := lines[0]
		if len(ps) > 1 {
			msg = fmt.Sprintf("%s has %d problems", filepath.Base(a.paths.File), len(ps))
		}
		out := apperr.Wrap(err, ae.Code, ae.Class, msg)
		if len(ps) > 1 {
			out = out.WithHint(strings.Join(lines, "\n"))
		} else if ae.Hint != "" {
			out = out.WithHint(ae.Hint)
		}
		return res, out
	}
	c := l.Config
	res.Exists = l.Exists
	res.Agents, res.Profiles, res.Logins, res.Projects, res.Aliases = len(c.Agents), len(c.Profiles), len(c.Logins), len(c.Projects), len(c.Aliases)
	return res, nil
}

// Sources of config show for values the file does not set.
const (
	sourceNotSet = "not set"
)

// ConfigShow is agx config show: every value of the config with the
// file:line:col it comes from, plus the defaults that apply where the file
// sets nothing. Secret-looking values are never printed: login tokens
// show only whether they are a reference, literal env values show (set).
func (a *App) ConfigShow() ([]ValueSource, error) {
	cfg, _, err := a.loadedConfig()
	if err != nil {
		return nil, err
	}
	w := &showWalker{l: cfg, rows: []ValueSource{}}
	if !cfg.Exists {
		w.rows = append(w.rows, ValueSource{Key: "file", Value: a.paths.File, Source: "does not exist (agx config init)"})
	}
	w.walk(reflect.ValueOf(cfg.Config), nil)
	c := cfg.Config
	if c.DefaultAgent == "" {
		w.rows = append(w.rows, ValueSource{Key: "default_agent", Value: "", Source: sourceNotSet + " (asked at a terminal, refused without one)"})
	}
	if c.Sessions.Root == "" {
		w.rows = append(w.rows, ValueSource{Key: "sessions.root", Value: a.tilde(filepath.Join(a.paths.Dir, config.SessionsDirName)), Source: sourceDefault})
	}
	for _, n := range sortedNames(c.Profiles) {
		p := c.Profiles[n]
		if p.Billing == "" {
			w.rows = append(w.rows, ValueSource{Key: "profiles." + n + ".billing", Value: config.BillingPlan, Source: sourceDefault})
		}
		if p.InheritDefaults == nil {
			w.rows = append(w.rows, ValueSource{Key: "profiles." + n + ".inherit_defaults", Value: "true", Source: sourceDefault})
		}
		for _, id := range slices.Sorted(maps.Keys(p.Blocks)) {
			w.rows = append(w.rows, ValueSource{Key: "profiles." + n + "." + string(id), Value: blockText, Source: cfg.Where("profiles", n, string(id))})
		}
	}
	return w.rows, nil
}

// Display values of config show.
const (
	tokenRefText   = "(reference)"
	tokenPlainText = "(plain value, " + redactedText + ")"
	specText       = "(custom agent spec)"
	blockText      = "(typed launch block)"
)

// showWalker turns the typed config into rows, using json names for keys
// so a row's key is the path a user edits.
type showWalker struct {
	l    *config.Loaded
	rows []ValueSource
}

// walk adds one row per set scalar or list under v at keys.
func (w *showWalker) walk(v reflect.Value, keys []string) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if len(keys) > 0 && keys[len(keys)-1] == "spec" {
			w.add(keys, specText)
			return
		}
		if v.Elem().Kind() == reflect.Bool {
			// An optional switch (inherit_defaults) is shown when set,
			// false included: false is the value that matters there.
			w.add(keys, strconv.FormatBool(v.Elem().Bool()))
			return
		}
		w.walk(v.Elem(), keys)
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" || !t.Field(i).IsExported() {
				continue
			}
			w.walk(v.Field(i), append(slices.Clone(keys), name))
		}
	case reflect.Map:
		names := make([]string, 0, v.Len())
		for _, k := range v.MapKeys() {
			names = append(names, k.String())
		}
		slices.Sort(names)
		for _, n := range names {
			mv := v.MapIndex(reflect.ValueOf(n).Convert(v.Type().Key()))
			w.walk(mv, append(slices.Clone(keys), n))
		}
	case reflect.Slice:
		if v.Len() == 0 {
			return
		}
		parts := make([]string, v.Len())
		for i := range v.Len() {
			parts[i] = v.Index(i).String() // config lists are strings or string-kind ids
		}
		w.add(keys, strings.Join(parts, " "))
	case reflect.String:
		if v.String() == "" {
			return
		}
		w.add(keys, w.scalar(keys, v.String()))
	case reflect.Bool:
		if v.Bool() {
			w.add(keys, "true")
		}
	case reflect.Int, reflect.Int64:
		w.add(keys, strconv.FormatInt(v.Int(), 10))
	}
}

// scalar redacts a string value that may be a secret.
func (w *showWalker) scalar(keys []string, s string) string {
	if len(keys) >= 2 && keys[0] == "logins" && keys[len(keys)-1] == "token" {
		if strings.HasPrefix(s, "$") {
			return tokenRefText
		}
		return tokenPlainText
	}
	if len(keys) >= 3 && keys[0] == "profiles" && keys[2] == "env" {
		return setText
	}
	return s
}

// add appends one row with the value's position as its source; a value
// the file does not hold (version on a missing file) is a default.
func (w *showWalker) add(keys []string, value string) {
	src := sourceDefault
	if w.l.Doc != nil && w.l.Doc.Has(config.Pointer(keys...)) {
		src = w.l.Where(keys...)
	}
	w.rows = append(w.rows, ValueSource{Key: strings.Join(keys, keySep), Value: value, Source: src})
}
