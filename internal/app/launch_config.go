package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
)

// Sources named in decisions and messages.
const (
	srcOption    = "--option"
	srcSetting   = "--setting"
	srcLiveLogin = "--live-login"
	srcTokenRef  = "--token-ref"
	srcModel     = "--model"
	srcAutonomy  = "--autonomy"
	srcPrompt    = "--prompt"
	srcAddDir    = "--add-dir"
	srcHeadless  = "--headless"
	srcResume    = "resume --id"
	// srcWinnerRule is the source of the notes that name a dropped input:
	// user-facing, so it says the rule in words rather than citing the spec.
	srcWinnerRule  = "one input per concern; the command line wins"
	jsonStringByte = '"'
	// groupBlock and groupProfileSettings name the profile's agent block
	// and its settings as inputs (configChoice.group); the command-line
	// inputs use their flag (srcOption, srcSetting).
	// cfgKeyProfiles is the config's top-level "profiles" member, where
	// inline profile settings are placed for file:line:col.
	cfgKeyProfiles       = "profiles"
	groupBlock           = "profile block"
	groupProfileSettings = "profile settings"
)

// configChoice is one input that sets a launch concern (spec 7.1): a
// request field, an option key or a settings key, with how to take it
// back when another input wins.
//
// group names the input a key came from (the profile block, the --option
// flags, the profile settings, the --setting flags; "" for a request
// field, which is an input of its own). Keys of one concern from one
// group are one choice: a profile block with print and output_format
// asks for one headless output, so the keys are kept or dropped together
// and the agent's own rules (agentio's build) judge whether they agree.
// Treating them as rivals dropped one and broke the launch.
type configChoice struct {
	label   string
	group   string
	prefix  string
	part    string
	cmdLine bool
	drop    func() error
}

// mergeGroups joins the choices of one concern that came from the same
// group into one choice, at the place of its first key, labelled with
// the group's prefix and every key ("--option print, output_format").
func mergeGroups(choices []configChoice) []configChoice {
	out := make([]configChoice, 0, len(choices))
	at := map[string]int{}
	for _, ch := range choices {
		i, seen := at[ch.group]
		if ch.group == "" || !seen {
			if ch.group != "" {
				at[ch.group] = len(out)
			}
			out = append(out, ch)
			continue
		}
		prev := out[i]
		prev.part += ", " + ch.part
		prev.label = prev.prefix + " " + prev.part
		first, next := prev.drop, ch.drop
		prev.drop = func() error {
			if err := first(); err != nil {
				return err
			}
			return next()
		}
		out[i] = prev
	}
	return out
}

// launchState is the options and settings of one launch while they are
// merged, with where each came from.
type launchState struct {
	profile string
	// options is the merged options object; optSrc maps each key to its
	// label; optOverride notes a profile value an --option replaced.
	options     jsonObject
	optSrc      map[string]string
	optCLI      map[string]bool
	optOverride map[string]string
	// settings is the merged settings object; profSettings the profile's
	// part (its source label in profLabel); cliKeys the --setting paths.
	settings     agentio.Raw
	profSettings agentio.Raw
	profLabel    string
	// profWhere gives "file:line:col" of a profile settings key (dotted)
	// in the file it was written in, "" when it cannot be placed.
	profWhere func(key string) string
	// agent is the launched agent: the profile block's key in the config.
	agent        agentio.ID
	cliKeys      []string
	cliOverrides []string
	// raws are the raw settings values in argv order, merged over the
	// profile and --setting keys (rawLeaves[i] the paths raws[i] sets).
	raws      []rawSettings
	rawLeaves [][]string
	// notes are decision lines added after the merge.
	notes []report.Decision
}

// launchConfig applies layer 2 (--option, profile block) and settings
// (profile settings, --setting, live login) to the plan (spec 7.2-7.4,
// 7.8): it merges each by precedence, checks them through agentio's
// launch-config capability, applies one winner per concern across the
// request, options and settings (two command-line choices are refused,
// naming both), and leaves the result in p.Options, p.Settings and
// p.SettingsDelivery for launchBuild. An agent without the capability
// refuses --option and --setting with agentio's typed reason.
func (a *App) launchConfig(ctx context.Context, o LaunchOptions, res resolve.Result, t *agentio.Target, req *agentio.LaunchRequest, p *LaunchPlan) error {
	ag := res.Agent
	st := &launchState{profile: res.ProfileName, agent: ag.ID(), options: jsonObject{}, optSrc: map[string]string{}, optCLI: map[string]bool{}, optOverride: map[string]string{}}
	block, err := decodeObject(res.Profile.Blocks[ag.ID()])
	if err != nil {
		return apperr.Usage(fmt.Sprintf("profile %s: %s block: %s", res.ProfileName, ag.ID(), err.Error()))
	}
	lc, has := agentio.LaunchConfigOf(ag)
	if !has && (len(o.Options) > 0 || len(o.Settings) > 0 || len(block) > 0) {
		reason, _ := agentio.Unsupported(ag, agentio.CapLaunchConfig)
		if reason == "" {
			reason = "no typed launch config"
		}
		return apperr.Unsupported(fmt.Sprintf("%s has no --option or --setting (%s); pass its flags after --", ag.ID(), reason))
	}

	live, err := a.livePlan(ctx, o, res, p)
	if err != nil {
		return err
	}
	if !has {
		if live != nil && !isEmptyRaw(live.Settings) {
			return apperr.Unsupported(fmt.Sprintf("%s takes no settings for --live-login", ag.ID()))
		}
		applyLive(live, t, req, p)
		return nil
	}

	if err := a.mergeOptions(lc, o, block, st); err != nil {
		return err
	}
	if err := a.mergeSettings(o, block, st); err != nil {
		return err
	}
	optKeys, err := lc.OptionKeys()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, fmt.Sprintf("%s option keys: %s", ag.ID(), err.Error()))
	}
	inOptions, err := takeOptionRawSettings(st, optKeys, ag)
	if err != nil {
		return err
	}
	raws, err := a.readRawSettings(o.Dir, append(inOptions, p.rawSettings...))
	if err != nil {
		return err
	}
	if live != nil {
		for _, r := range raws {
			if shared := sharedTopKeys(r.obj, live.Settings); len(shared) > 0 {
				return apperr.Conflict(fmt.Sprintf("%s %s and %s both set the %s; pick one", r.flagLabel, strings.Join(shared, ", "), cmp.Or(p.liveSrc, srcLiveLogin), agentio.ConcernCredential))
			}
		}
	}
	if err := st.mergeRaw(raws); err != nil {
		return err
	}

	check, err := lc.CheckSettings(orEmptyObject(st.settings))
	if err != nil {
		return apperr.Usage("settings: " + err.Error())
	}
	if len(check.TypeErrors) > 0 {
		te := check.TypeErrors[0]
		return apperr.Usage(st.typeErrorText(te.Key, te.Detail))
	}
	for _, u := range check.Unknown {
		msg := fmt.Sprintf("%s %s: not modelled", st.keyLabel(u.Key), u.Key)
		if u.Suggestion != "" {
			msg += " (did you mean " + u.Suggestion + "?)"
		}
		p.Warnings = append(p.Warnings, report.Warning{Code: WarnUnmodelled, Message: msg})
	}

	keys := optKeys
	if err := a.oneWinner(o, res, req, p, st, keys, check); err != nil {
		return err
	}
	// The agent gets path options with "~" expanded (it never expands
	// one itself: no shell runs in between); st.options keeps the values
	// as written for the decisions.
	expanded, err := expandPathOptions(st.options, keys, a.d.Host.UserHome)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "options: "+err.Error())
	}
	opts, err := encodeObject(expanded)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "options: "+err.Error())
	}
	if !isEmptyRaw(opts) {
		if err := lc.CheckOptions(opts); err != nil {
			return apperr.Usage(fmt.Sprintf("profile %s %s block: %s", res.ProfileName, ag.ID(), err.Error()))
		}
	}

	if live != nil {
		merged, err := agentio.MergeSettings(st.settings, live.Settings, "--live-login")
		if err != nil {
			return apperr.Conflict(fmt.Sprintf("--live-login and the run's settings both set %s; pick one", strings.Join(sharedTopKeys(st.settings, live.Settings), ", ")))
		}
		if !isEmptyRaw(live.Settings) {
			st.notes = append(st.notes, report.Decision{Name: resolve.DecSetting, Value: strings.Join(leafPaths(live.Settings), ", "), Source: cmp.Or(p.liveSrc, srcLiveLogin)})
		}
		st.settings = merged
		applyLive(live, t, req, p)
	}

	if err := a.delivery(o, res, st, p); err != nil {
		return err
	}
	p.Options = opts
	a.configDecisions(st, p)
	rawDuplicates(req, st, keys, p)
	return nil
}

// orEmptyObject gives CheckSettings an object even when there are no
// settings (it refuses a body that is not an object).
func orEmptyObject(raw agentio.Raw) agentio.Raw {
	if isEmptyRaw(raw) {
		return agentio.Raw("{}")
	}
	return raw
}

// sharedTopKeys are the top-level keys two objects both set.
func sharedTopKeys(a, b agentio.Raw) []string {
	bk := topKeys(b)
	var out []string
	for _, k := range topKeys(a) {
		if slices.Contains(bk, k) {
			out = append(out, k)
		}
	}
	return out
}

// applyLive adds a live-login plan's env and scrub to the target and
// drops the request's token: the agent fetches its credential from the
// helper command, and a token variable would outrank it (spec 10).
func applyLive(live *agentio.LiveCredentialPlan, t *agentio.Target, req *agentio.LaunchRequest, p *LaunchPlan) {
	if live == nil {
		return
	}
	t.ExtraEnv = append(t.ExtraEnv, live.Env...)
	t.ExtraEnv = append(t.ExtraEnv, agentio.ScrubEnv(live.Scrub)...)
	req.Credential = nil
	if p.Live == nil {
		p.Live = &LiveStart{StartLogin: p.Login}
	}
}

// mergeOptions builds the options object: the profile block (minus
// settings), then each --option key=value through agentio's registry
// (unknown key: the valid keys and a "did you mean").
func (a *App) mergeOptions(lc agentio.LaunchConfigurer, o LaunchOptions, block jsonObject, st *launchState) error {
	for k, v := range block {
		if k == agents.SettingsKey {
			continue
		}
		st.options[k] = v
		st.optSrc[k] = fmt.Sprintf("profile %s", st.profile)
	}
	if len(o.Options) == 0 {
		return nil
	}
	keys, err := lc.OptionKeys()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "option keys: "+err.Error())
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.Key
	}
	for _, kv := range o.Options {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			return apperr.Usage("--option " + kv + ": use key=value")
		}
		if name == agents.SettingsKey {
			return apperr.Usage(fmt.Sprintf("--option %s: settings is not a launch option; use --setting KEY=VALUE, or %q in the profile's %s block (a JSONC file or an object)", name, agents.SettingsKey, st.agent))
		}
		k, err := agentio.FindOptionKey(keys, name)
		if err != nil {
			msg := fmt.Sprintf("--option %s: unknown key", name)
			if s := agentio.ClosestKey(name, names); s != "" {
				msg += " (did you mean " + s + "?)"
			}
			return apperr.Usage(msg + "; valid keys: " + strings.Join(names, ", "))
		}
		raw, err := agentio.OptionValueJSON(k, val)
		if err != nil {
			return apperr.Usage(fmt.Sprintf("--option %s: %s", name, err.Error()))
		}
		if old, ok := st.options[name]; ok && !st.optCLI[name] {
			st.optOverride[name] = fmt.Sprintf("overrides profile %s: %s", st.profile, compactJSON(old))
		}
		st.options[name] = raw
		st.optSrc[name] = srcOption
		st.optCLI[name] = true
	}
	return nil
}

// mergeSettings builds the settings object (spec 7.4): the profile
// block's settings (a JSONC file path, relative to the config dir, or an
// object), then each --setting with dotted keys nesting over it.
func (a *App) mergeSettings(o LaunchOptions, block jsonObject, st *launchState) error {
	if raw, ok := block[agents.SettingsKey]; ok {
		prof, label, where, err := a.profileSettings(raw, st)
		if err != nil {
			return err
		}
		st.settings, st.profSettings, st.profLabel, st.profWhere = prof, prof, label, where
	}
	profLeaves := leafPaths(st.profSettings)
	for _, kv := range o.Settings {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || key == "" || strings.Contains(key, pathSep+pathSep) || strings.HasPrefix(key, pathSep) || strings.HasSuffix(key, pathSep) {
			return apperr.Usage("--setting " + kv + ": use key=value (dotted keys nest)")
		}
		merged, err := setPath(st.settings, strings.Split(key, pathSep), settingValue(val))
		if err != nil {
			return apperr.Usage("--setting " + key + ": " + err.Error())
		}
		st.settings = merged
		st.cliKeys = append(st.cliKeys, key)
		for _, l := range profLeaves {
			if pathsOverlap(l, key) && !slices.Contains(st.cliOverrides, l) {
				st.cliOverrides = append(st.cliOverrides, l)
			}
		}
	}
	return nil
}

// profileSettings reads a block's "settings": a path (read as JSONC) or
// an object. label is the decision source; where places a settings key
// in the file it was written in (config.jsonc for an object, the
// settings file for a path), so a type error names file:line:col.
func (a *App) profileSettings(raw json.RawMessage, st *launchState) (agentio.Raw, string, func(string) string, error) {
	profile := st.profile
	label := "profile " + profile
	t := strings.TrimSpace(string(raw))
	if t == "" || t[0] != jsonStringByte {
		obj, err := decodeObject(raw)
		if err != nil {
			return nil, "", nil, apperr.Usage(fmt.Sprintf("profile %s: settings: %s", profile, err.Error()))
		}
		out, err := encodeObject(obj)
		return out, label, a.inlineSettingsWhere(profile, st.agent), err
	}
	var rel string
	if err := json.Unmarshal(raw, &rel); err != nil {
		return nil, "", nil, apperr.Usage(fmt.Sprintf("profile %s: settings: %s", profile, err.Error()))
	}
	path := resolvePath(rel, a.paths.Dir, a)
	label += " (file " + a.tilde(path) + ")"
	doc, err := jsonc.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !doc.Existed()):
		return nil, "", nil, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("profile %s: settings file %s not found", profile, a.tilde(path)))
	case err != nil:
		return nil, "", nil, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, fmt.Sprintf("profile %s: settings file %s: %s", profile, a.tilde(path), err.Error()))
	}
	if derr := doc.Err(); derr != nil {
		return nil, "", nil, apperr.Wrap(derr, apperr.CodeConfig, apperr.ClassFailure, derr.Error())
	}
	body, err := doc.JSON()
	if err != nil {
		return nil, "", nil, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, fmt.Sprintf("settings file %s: %s", a.tilde(path), err.Error()))
	}
	if _, err := decodeObject(body); err != nil {
		return nil, "", nil, apperr.Usage(fmt.Sprintf("settings file %s: %s", a.tilde(path), err.Error()))
	}
	where := func(key string) string {
		return docWhere(doc, a.tilde(path), strings.Split(key, pathSep))
	}
	return body, label, where, nil
}

// inlineSettingsWhere places a settings key written inline in the
// profile: profiles.<profile>.<agent>.settings.<key> in config.jsonc.
func (a *App) inlineSettingsWhere(profile string, agent agentio.ID) func(string) string {
	return func(key string) string {
		if a.cfg == nil || a.cfg.Doc == nil {
			return ""
		}
		keys := append([]string{cfgKeyProfiles, profile, agent.String(), agents.SettingsKey}, strings.Split(key, pathSep)...)
		return docWhere(a.cfg.Doc, filepath.Base(a.cfg.Paths.File), keys)
	}
}

// docWhere is "name:line:col" of the member name at keys in doc, or ""
// when the key is not in it (a key that only a --setting added).
func docWhere(doc *jsonc.Doc, name string, keys []string) string {
	p, err := doc.KeyPos(config.Pointer(keys...))
	if err != nil || !p.IsValid() {
		return ""
	}
	return name + ":" + p.String()
}

// typeErrorText words a settings type error: a --setting is named by its
// flag; a profile or raw-file key by where it was written (file:line:col,
// the same form as a config error) when it can be placed, else by its
// source.
func (st *launchState) typeErrorText(key, detail string) string {
	o := st.origin(key)
	if o.where != nil {
		if where := o.where(key); where != "" {
			return fmt.Sprintf("%s: %s %s: %s", where, o.short, key, detail)
		}
	}
	return fmt.Sprintf("%s %s: %s", o.label, key, detail)
}

// keyLabel names the source of a settings key for messages and
// decisions (settingsOrigin.label).
func (st *launchState) keyLabel(key string) string { return st.origin(key).label }

// settingsKind is where a settings key came from, in merge order: the
// profile, a --setting flag, a raw settings flag (the last one wins).
type settingsKind int

// The settings sources, lowest first.
const (
	kindProfile settingsKind = iota
	kindSetting
	kindRaw
)

// settingsOrigin is the input that set a settings key.
type settingsOrigin struct {
	kind settingsKind
	// label names it in decisions and messages ("profile work (file
	// ~/s.jsonc)", "--setting", "raw --settings (file ./team.json)");
	// short is the form after a file:line:col ("profile work settings",
	// "raw --settings").
	label, short string
	// where places the key in its file; nil when it has no file.
	where func(key string) string
}

// origin finds the input that set key: the last raw settings value with
// an overlapping path, else a --setting, else the profile (merge order,
// so the input found is the one whose value is used).
func (st *launchState) origin(key string) settingsOrigin {
	for i := len(st.raws) - 1; i >= 0; i-- {
		if coveredBy(key, st.rawLeaves[i]) {
			r := st.raws[i]
			return settingsOrigin{kind: kindRaw, label: r.label, short: r.flagLabel, where: r.where}
		}
	}
	for _, k := range st.cliKeys {
		if pathsOverlap(k, key) {
			return settingsOrigin{kind: kindSetting, label: srcSetting, short: srcSetting}
		}
	}
	return settingsOrigin{kind: kindProfile, label: st.profLabel, short: "profile " + st.profile + " settings", where: st.profWhere}
}

// mergeRaw lays each raw settings value over the merged settings, in
// argv order (deepMerge: objects merge, other values replace), and type
// checks nothing: CheckSettings runs on the result.
func (st *launchState) mergeRaw(raws []rawSettings) error {
	for _, r := range raws {
		merged, err := deepMerge(st.settings, r.obj)
		if err != nil {
			return apperr.Usage(r.label + ": " + err.Error())
		}
		st.settings = merged
		st.raws = append(st.raws, r)
		st.rawLeaves = append(st.rawLeaves, setLeaves(r.obj))
	}
	return nil
}

// oneWinner applies spec 7.1 across the request, the options and the
// settings: per concern, two command-line choices are a conflict naming
// both (edge 37); one command-line choice beats every profile choice;
// otherwise the later layer wins (request field, then option, then
// settings: a settings apiKeyHelper replaces the login, spec 7.8). Each
// override is noted.
func (a *App) oneWinner(o LaunchOptions, res resolve.Result, req *agentio.LaunchRequest, p *LaunchPlan, st *launchState, keys []agentio.OptionKey, check agentio.SettingsCheck) error {
	byConcern := map[agentio.Concern][]configChoice{}
	add := func(c agentio.Concern, ch configChoice) { byConcern[c] = append(byConcern[c], ch) }
	prof := "profile " + res.ProfileName

	if req.Model != "" {
		label := prof + " model"
		if o.Model != "" {
			label = srcModel
		}
		add(agentio.ConcernModel, configChoice{label: label, cmdLine: o.Model != "", drop: func() error { req.Model = ""; notUsed(p, "model="); return nil }})
	}
	if req.Autonomy != agentio.AutonomyDefault {
		label := prof + " autonomy"
		if o.Autonomy != "" {
			label = srcAutonomy
		}
		add(agentio.ConcernAutonomy, configChoice{label: label, cmdLine: o.Autonomy != "", drop: func() error { req.Autonomy = agentio.AutonomyDefault; notUsed(p, "autonomy="); return nil }})
	}
	if req.Prompt != "" {
		add(agentio.ConcernPrompt, configChoice{label: srcPrompt, cmdLine: true, drop: func() error { req.Prompt = ""; return nil }})
	}
	if len(req.AddDirs) > 0 {
		add(agentio.ConcernAddDir, configChoice{label: srcAddDir, cmdLine: true, drop: func() error { req.AddDirs = nil; return nil }})
	}
	if req.Headless {
		add(agentio.ConcernHeadless, configChoice{label: srcHeadless, cmdLine: true, drop: func() error { req.Headless = false; return nil }})
	}
	if req.ResumeID != "" {
		add(agentio.ConcernResume, configChoice{label: srcResume, cmdLine: true, drop: func() error { req.ResumeID = ""; return nil }})
	}
	if l := res.Login; l.Kind != resolve.LoginHome || strings.HasPrefix(l.Source, "--") {
		label := loginChoice(l)
		if l.Kind == resolve.LoginHome {
			label = flagOf(l.Source)
		}
		add(agentio.ConcernCredential, configChoice{label: label, cmdLine: strings.HasPrefix(l.Source, "--"), drop: func() error { req.Credential = nil; return nil }})
	}

	for _, k := range keys {
		if _, ok := st.options[k.Key]; !ok || k.Concern == agentio.ConcernNone {
			continue
		}
		name := k.Key
		group, prefix, part := groupBlock, prof, res.Agent.ID().String()+"."+name
		if st.optCLI[name] {
			group, prefix, part = srcOption, srcOption, name
		}
		add(k.Concern, configChoice{label: prefix + " " + part, group: group, prefix: prefix, part: part, cmdLine: st.optCLI[name], drop: func() error { delete(st.options, name); return nil }})
	}

	ckeys := make([]string, 0, len(check.Concerns))
	for k := range check.Concerns {
		ckeys = append(ckeys, k)
	}
	slices.Sort(ckeys)
	for _, key := range ckeys {
		o := st.origin(key)
		cli := o.kind == kindSetting
		group, prefix := srcSetting, srcSetting
		switch o.kind {
		case kindRaw:
			// Each raw value is an input of its own; never a command-line
			// choice: raw args are passed, not refused (edge 38).
			group, prefix = o.label, o.label
		case kindProfile:
			group, prefix = groupProfileSettings, prof+" settings"
		}
		path := strings.Split(key, pathSep)
		add(check.Concerns[key], configChoice{label: prefix + " " + key, group: group, prefix: prefix, part: key, cmdLine: cli, drop: func() error {
			s, err := deletePath(st.settings, path)
			st.settings = s
			return err
		}})
	}
	// The settings object as a whole competes only with an option that
	// delivers settings of its own (ConcernSettings).
	if len(byConcern[agentio.ConcernSettings]) > 0 && !isEmptyRaw(st.settings) {
		label, cli := st.profLabel, len(st.cliKeys) > 0
		if cli {
			label = srcSetting
		}
		add(agentio.ConcernSettings, configChoice{label: label, cmdLine: cli, drop: func() error { st.settings = nil; return nil }})
	}

	for _, c := range agentio.ConcernValues() {
		choices := mergeGroups(byConcern[c])
		if len(choices) == 0 {
			continue
		}
		var cmd []configChoice
		for _, ch := range choices {
			if ch.cmdLine {
				cmd = append(cmd, ch)
			}
		}
		if len(cmd) > 1 {
			return apperr.Conflict(fmt.Sprintf("%s and %s both set the %s; pick one", cmd[0].label, cmd[1].label, c))
		}
		winner := choices[len(choices)-1]
		if len(cmd) == 1 {
			winner = cmd[0]
		}
		for _, ch := range choices {
			if ch.label == winner.label {
				continue
			}
			if err := ch.drop(); err != nil {
				return apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, err.Error())
			}
			st.notes = append(st.notes, report.Decision{Name: resolve.DecNote, Value: fmt.Sprintf("%s wins the %s; %s is not used", winner.label, c, ch.label), Source: srcWinnerRule})
		}
		if c == agentio.ConcernCredential && len(choices) == 1 && res.Login.Kind == resolve.LoginHome && !strings.HasPrefix(res.Login.Source, "--") {
			st.notes = append(st.notes, report.Decision{Name: resolve.DecNote, Value: winner.label + " supplies the credential; no agx login is used for this run", Source: srcWinnerRule})
		}
	}
	return nil
}

// delivery picks settings_delivery: --settings-delivery, else the
// profile's, else file (spec 7.4); inline adds the edge-40 note.
func (a *App) delivery(o LaunchOptions, res resolve.Result, st *launchState, p *LaunchPlan) error {
	val, src := o.SettingsDelivery, "--settings-delivery"
	if val == "" {
		val, src = res.Profile.SettingsDelivery, "profile "+res.ProfileName
	}
	d := agentio.SettingsFile
	if val != "" {
		parsed, ok := agentio.ParseSettingsDelivery(val)
		if !ok {
			names := make([]string, 0, len(agentio.SettingsDeliveryValues()))
			for _, v := range agentio.SettingsDeliveryValues() {
				names = append(names, v.String())
			}
			return apperr.Usage(fmt.Sprintf("--settings-delivery %q: use one of %s", val, strings.Join(names, ", ")))
		}
		d = parsed
	}
	if isEmptyRaw(st.settings) {
		return nil
	}
	p.Settings, p.SettingsDelivery = st.settings, d.String()
	if d == agentio.SettingsFile {
		prev := p.finish
		p.finish = func(p *LaunchPlan) error {
			a.fileDecision(p)
			if prev != nil {
				return prev(p)
			}
			return nil
		}
	}
	if d == agentio.SettingsInline {
		p.Warnings = append(p.Warnings, report.Warning{Code: WarnInline, Message: "settings are visible in ps (inline delivery, from " + src + ")"})
	}
	return nil
}

// configDecisions records the options (key=value and source) and the
// settings (keys only, never values) after one winner per concern.
func (a *App) configDecisions(st *launchState, p *LaunchPlan) {
	names := make([]string, 0, len(st.options))
	for k := range st.options {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		src := st.optSrc[k]
		if ov, ok := st.optOverride[k]; ok {
			src += " (" + ov + ")"
		}
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecOptions, Value: k + "=" + compactJSON(st.options[k]), Source: src})
	}
	if !isEmptyRaw(p.Settings) {
		var prof []string
		for _, l := range leafPaths(st.profSettings) {
			if hasLeaf(p.Settings, l) && !st.rawCovers(l, len(st.raws)) {
				prof = append(prof, l)
			}
		}
		if len(prof) > 0 {
			p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecSetting, Value: strings.Join(prof, ", "), Source: st.profLabel})
		}
		var cli []string
		for _, k := range st.cliKeys {
			if hasLeaf(p.Settings, k) && !slices.Contains(cli, k) && !st.rawCovers(k, len(st.raws)) {
				cli = append(cli, k)
			}
		}
		if len(cli) > 0 {
			src := srcSetting
			if len(st.cliOverrides) > 0 {
				src += " (overrides " + st.profLabel + ": " + strings.Join(st.cliOverrides, ", ") + ")"
			}
			p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecSetting, Value: strings.Join(cli, ", "), Source: src})
		}
		st.rawDecisions(p)
	}
	p.Decisions = append(p.Decisions, st.notes...)
}

// hasLeaf reports whether obj has a value at the dotted path (or under it).
func hasLeaf(obj agentio.Raw, path string) bool {
	for _, l := range leafPaths(obj) {
		if pathsOverlap(l, path) {
			return true
		}
	}
	return false
}

// compactJSON renders a raw value on one line for a decision.
func compactJSON(raw json.RawMessage) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	var buf json.RawMessage
	if err := json.Unmarshal(raw, &buf); err != nil {
		return string(raw)
	}
	if err := enc.Encode(buf); err != nil {
		return string(raw)
	}
	s := strings.TrimSpace(b.String())
	if len(s) > 1 && s[0] == jsonStringByte {
		var str string
		if json.Unmarshal([]byte(s), &str) == nil {
			return str
		}
	}
	return s
}

// rawDuplicates notes raw args that repeat a typed concern this launch
// already sets (edge 38): both are passed, the raw one last, so the agent
// uses the raw one. Generic over agentio's option keys (OptionKey.Flag).
func rawDuplicates(req *agentio.LaunchRequest, st *launchState, keys []agentio.OptionKey, p *LaunchPlan) {
	set := map[agentio.Concern]bool{
		agentio.ConcernModel:    req.Model != "",
		agentio.ConcernAutonomy: req.Autonomy != agentio.AutonomyDefault,
		agentio.ConcernPrompt:   req.Prompt != "",
		agentio.ConcernAddDir:   len(req.AddDirs) > 0,
		agentio.ConcernHeadless: req.Headless,
		agentio.ConcernResume:   req.ResumeID != "",
	}
	var seen []string
	for _, k := range keys {
		if k.Flag == "" || slices.Contains(seen, k.Flag) {
			continue
		}
		_, inOptions := st.options[k.Key]
		if !inOptions && !set[k.Concern] {
			continue
		}
		for _, arg := range req.Args {
			if arg == k.Flag || strings.HasPrefix(arg, k.Flag+"=") {
				seen = append(seen, k.Flag)
				p.Warnings = append(p.Warnings, report.Warning{Code: WarnRawDup,
					Message: fmt.Sprintf("raw %s repeats a typed %s; both are passed and the agent uses the raw one (last)", k.Flag, k.Key)})
				break
			}
		}
	}
}

// launchBuild builds the command through agentio's launch-config
// capability when the agent has it (options, settings delivered into
// runDir as settings-<run id>.json, or inline); else the plain Launch.
// Unknown-settings warnings were reported with their source already, so
// agentio's copies are dropped here.
func (a *App) launchBuild(ag agentio.Agent, t agentio.Target, req agentio.LaunchRequest, p *LaunchPlan, runDir string) (agentio.Command, error) {
	lc, ok := agentio.LaunchConfigOf(ag)
	if !ok {
		return ag.Launch(t, req)
	}
	d := agentio.SettingsFile
	if p.SettingsDelivery != "" {
		if parsed, ok := agentio.ParseSettingsDelivery(p.SettingsDelivery); ok {
			d = parsed
		}
	}
	cmd, err := lc.LaunchConfigured(t, req, agentio.LaunchConfig{Options: p.Options, Settings: p.Settings, Delivery: d, RunDir: runDir, RunID: p.RunID})
	if err != nil {
		return cmd, err
	}
	cmd.Warnings = slices.DeleteFunc(cmd.Warnings, func(w agentio.Warning) bool { return w.Kind == agentio.WarnUnknownKey })
	return cmd, nil
}

// settingsFilePrefix and settingsFileExt name agentio's per-run settings
// file, settings-<run id>.json (agentio.LaunchConfig.RunID).
const (
	settingsFilePrefix = "settings-"
	settingsFileExt    = ".json"
)

// fileDecision states where file-delivered settings went (spec 7.0
// recipe 1), once the command shows the agent took the file; an agent
// that turns settings into flags (Codex -c) gets no file line.
func (a *App) fileDecision(p *LaunchPlan) {
	path := filepath.Join(p.RunDir, settingsFilePrefix+p.RunID+settingsFileExt)
	for _, arg := range p.Command.Args {
		if strings.HasSuffix(arg, string(filepath.Separator)+settingsFilePrefix+p.RunID+settingsFileExt) {
			p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecSetting, Value: "delivered as file " + a.tilde(path) + " (0600)", Source: "settings_delivery file"})
			return
		}
	}
}

// helperDecision names the settings keys an api-key-helper credential
// added (Claude: apiKeyHelper) and where they went, the same rows a
// --live-login run shows. agentio merges the helper into the settings
// file itself, so the keys are read back from the file it wrote in
// runDir (the temporary one of a dry run) and compared with the run's
// own settings: no agent's key name is written here. Keys only, never
// the helper command. A helper delivered any other way gets no row.
func (a *App) helperDecision(req agentio.LaunchRequest, p *LaunchPlan, runDir string) {
	if req.Credential == nil || req.Credential.Kind != agentio.TokenAPIKeyHelper {
		return
	}
	name := settingsFilePrefix + p.RunID + settingsFileExt
	delivered := false
	for _, arg := range p.Command.Args {
		if strings.HasSuffix(arg, string(filepath.Separator)+name) {
			delivered = true
			break
		}
	}
	if !delivered {
		return
	}
	body, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		return // the command still names the file; the row is only a note
	}
	var added []string
	for _, l := range leafPaths(body) {
		if !hasLeaf(p.Settings, l) {
			added = append(added, l)
		}
	}
	if len(added) == 0 {
		return
	}
	src := "login " + p.Login
	if p.LoginKind == LoginKindTokenRef {
		src = srcTokenRef
	}
	p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecSetting, Value: strings.Join(added, ", "), Source: src})
	if isEmptyRaw(p.Settings) {
		// With settings of its own the run already gets the file row
		// (delivery's finish); without, this is the only settings file.
		a.fileDecision(p)
	}
}

// notUsed marks the request decision whose value starts with prefix as
// replaced by another input (one winner per concern).
func notUsed(p *LaunchPlan, prefix string) {
	for i := range p.Decisions {
		if p.Decisions[i].Name == resolve.DecOptions && strings.HasPrefix(p.Decisions[i].Value, prefix) {
			p.Decisions[i].Source += " (not used: see note)"
		}
	}
}

// rawCovers reports a settings path that one of the first n raw values
// replaces (so its earlier source is no longer the one used).
func (st *launchState) rawCovers(path string, n int) bool {
	for i := 0; i < n && i < len(st.raws); i++ {
		if coveredBy(path, st.rawLeaves[i]) {
			return true
		}
	}
	return false
}

// rawDecisions adds one settings decision per raw value: the keys it
// supplies that are still used (not replaced by a later raw value), from
// its label, with the earlier keys it replaced ("overrides profile work:
// cleanupPeriodDays; --setting: model").
func (st *launchState) rawDecisions(p *LaunchPlan) {
	for i, r := range st.raws {
		var keys []string
		for _, l := range st.rawLeaves[i] {
			if hasLeaf(p.Settings, l) && !st.laterRawCovers(l, i) {
				keys = append(keys, l)
			}
		}
		if len(keys) == 0 {
			continue
		}
		var over []string
		add := func(label string, paths []string) {
			var hit []string
			for _, path := range paths {
				if coveredBy(path, st.rawLeaves[i]) && !slices.Contains(hit, path) {
					hit = append(hit, path)
				}
			}
			if len(hit) > 0 {
				over = append(over, label+": "+strings.Join(hit, ", "))
			}
		}
		add(st.profLabel, leafPaths(st.profSettings))
		add(srcSetting, st.cliKeys)
		for j := 0; j < i; j++ {
			add(st.raws[j].label, st.rawLeaves[j])
		}
		src := r.label
		if len(over) > 0 {
			src += " (overrides " + strings.Join(over, "; ") + ")"
		}
		p.Decisions = append(p.Decisions, report.Decision{Name: resolve.DecSetting, Value: strings.Join(keys, ", "), Source: src})
	}
}

// laterRawCovers reports a path a raw value after the i-th replaces.
func (st *launchState) laterRawCovers(path string, i int) bool {
	for j := i + 1; j < len(st.raws); j++ {
		if coveredBy(path, st.rawLeaves[j]) {
			return true
		}
	}
	return false
}
