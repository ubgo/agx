package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/internal/apperr"
)

// Raw settings (spec decision 27, edge 39). An agent that takes its
// settings as one flag (Claude --settings) keeps only the LAST such flag,
// wholesale: earlier ones are dropped, not merged (live check V7). A raw
// settings flag in the raw args would therefore silently drop every
// setting agx passes, including the live-login helper. So agx takes each
// raw settings flag out of the args, reads its value (inline JSON or a
// JSONC file), deep-merges it over its own settings and passes exactly
// one settings flag. Which args are settings flags is the agent's answer
// (agentio's RawSettingsArg), never a flag name known here.

// Raw settings wording and forms.
const (
	// rawPrefix starts the label of a raw settings flag: "raw --settings".
	rawPrefix = "raw "
	// flagValueSep separates a flag from an attached value ("--x=v").
	flagValueSep = "="
	// inlineOpen and inlineClose mark an inline value: one that is a JSON
	// object (trimmed) is inline, anything else a file path. It is the
	// rule the agent itself applies to a path-or-JSON flag (Claude Code
	// 2.1.327 reads --settings this way), so a value means the same thing
	// to agx and to the agent.
	inlineOpen  = "{"
	inlineClose = "}"
)

// rawSettingsArg is one raw settings flag taken out of the raw args.
type rawSettingsArg struct {
	// flag is the flag as written, without a value ("--settings").
	flag string
	// value is its value as passed: inline JSON or a file path (with "~"
	// already expanded for args from the config file).
	value string
}

// rawSettings is one raw settings value, read.
type rawSettings struct {
	// flagLabel is "raw <flag>"; label adds the file for a file value
	// ("raw --settings (file ~/team.json)"), the decision source.
	flagLabel, label string
	// obj is the value as a JSON object (JSONC comments removed).
	obj agentio.Raw
	// where places a key in the file as "file:line:col"; nil for an inline
	// value (a position inside an argument helps no one).
	where func(key string) string
}

// argGroup is the raw args of one source, in order.
type argGroup struct {
	// src names the source in the args decision ("profile work",
	// "after --").
	src  string
	args []string
}

// joinArgGroups flattens groups into the args and their decision
// source, skipping groups left empty (a source whose only args were a
// raw settings flag is not named).
func joinArgGroups(groups []argGroup) ([]string, string) {
	var args, src []string
	for _, g := range groups {
		if len(g.args) == 0 {
			continue
		}
		args = append(args, g.args...)
		src = append(src, g.src)
	}
	return args, strings.Join(src, ", ")
}

// takeRawSettings removes every raw settings flag (both "--flag value"
// and "--flag=value") from groups and returns them in argv order. An
// agent without typed launch config keeps its args as they are. A flag
// with no value after it is a usage error.
func takeRawSettings(groups []argGroup, ag agentio.Agent) ([]argGroup, []rawSettingsArg, error) {
	lc, ok := agentio.LaunchConfigOf(ag)
	if !ok {
		return groups, nil, nil
	}
	var raws []rawSettingsArg
	out := make([]argGroup, len(groups))
	for gi, g := range groups {
		out[gi].src = g.src
		for i := 0; i < len(g.args); i++ {
			arg := g.args[i]
			if !lc.RawSettingsArg([]string{arg}) {
				out[gi].args = append(out[gi].args, arg)
				continue
			}
			if flag, val, attached := strings.Cut(arg, flagValueSep); attached {
				raws = append(raws, rawSettingsArg{flag: flag, value: val})
				continue
			}
			if i+1 >= len(g.args) {
				return nil, nil, apperr.Usage(rawPrefix + arg + " needs a value (inline JSON or a settings file)")
			}
			i++
			raws = append(raws, rawSettingsArg{flag: arg, value: g.args[i]})
		}
	}
	return out, raws, nil
}

// readRawSettings reads each raw settings value: inline JSON (JSONC
// tolerated) or a JSONC file, a relative path against the launch folder
// dir (where the agent would have looked: it runs in dir) and a leading
// "~" as the user's home. Every value must be a JSON object.
func (a *App) readRawSettings(dir string, raws []rawSettingsArg) ([]rawSettings, error) {
	out := make([]rawSettings, 0, len(raws))
	for _, r := range raws {
		flagLabel := rawPrefix + r.flag
		v := strings.TrimSpace(r.value)
		if v == "" {
			return nil, apperr.Usage(flagLabel + " needs a value (inline JSON or a settings file)")
		}
		if strings.HasPrefix(v, inlineOpen) && strings.HasSuffix(v, inlineClose) {
			doc, err := jsonc.Parse([]byte(v), jsonc.WithFileName(flagLabel))
			if err != nil {
				return nil, apperr.Usage(err.Error())
			}
			body, err := doc.JSON()
			if err != nil {
				return nil, apperr.Usage(flagLabel + ": " + err.Error())
			}
			out = append(out, rawSettings{flagLabel: flagLabel, label: flagLabel, obj: body})
			continue
		}
		rs, err := a.readRawSettingsFile(dir, flagLabel, r.value)
		if err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, nil
}

// readRawSettingsFile reads one raw settings file (see readRawSettings):
// missing is not-found, unreadable or bad JSONC a config error naming the
// file (line:col for JSONC), not an object a usage error.
func (a *App) readRawSettingsFile(dir, flagLabel, value string) (rawSettings, error) {
	path := expandTilde(value, a.d.Host.UserHome)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	shown := a.tilde(path)
	doc, err := jsonc.Open(path, jsonc.WithFileName(shown))
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !doc.Existed()):
		return rawSettings{}, apperr.Failure(apperr.CodeNotFound, fmt.Sprintf("%s file %s not found", flagLabel, shown))
	case err != nil && doc == nil:
		return rawSettings{}, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, fmt.Sprintf("%s file %s: %s", flagLabel, shown, err.Error()))
	case err != nil:
		// A syntax error: its text is "file:line:col: what".
		return rawSettings{}, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, flagLabel+": "+err.Error())
	}
	body, err := doc.JSON()
	if err != nil {
		return rawSettings{}, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, fmt.Sprintf("%s file %s: %s", flagLabel, shown, err.Error()))
	}
	if _, err := decodeObject(body); err != nil || isNull(body) {
		return rawSettings{}, apperr.Usage(fmt.Sprintf("%s file %s: %s", flagLabel, shown, errNotObject.Error()))
	}
	where := func(key string) string { return docWhere(doc, shown, strings.Split(key, pathSep)) }
	return rawSettings{flagLabel: flagLabel, label: flagLabel + " (file " + shown + ")", obj: body, where: where}, nil
}

// isNull reports a JSON null (decodeObject reads it as an empty object,
// but a settings file holding null is not a settings object).
func isNull(raw agentio.Raw) bool {
	return string(bytes.TrimSpace(raw)) == jsonNullText
}

// jsonNullText is JSON's null literal.
const jsonNullText = "null"

// deepMerge lays over on top of base: keys of over win; where both hold
// an object the two merge the same way, recursively; any other value (an
// array, a string, a number, true/false, null) replaces the base value
// whole, so an allow list in over is the whole list. Both are JSON
// objects (empty base = over).
func deepMerge(base, over agentio.Raw) (agentio.Raw, error) {
	b, err := decodeObject(base)
	if err != nil {
		return over, nil
	}
	o, err := decodeObject(over)
	if err != nil {
		return nil, err
	}
	for k, v := range o {
		if bv, ok := b[k]; ok && isJSONObject(bv) && isJSONObject(v) {
			m, err := deepMerge(bv, v)
			if err != nil {
				return nil, err
			}
			b[k] = json.RawMessage(m)
			continue
		}
		b[k] = v
	}
	return json.Marshal(b)
}

// isJSONObject reports a raw value that is a JSON object.
func isJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == inlineOpen[0]
}

// setLeaves are the dotted paths a settings object sets when merged over
// another (deepMerge): every leaf except an empty object, which merges
// into the base value and so sets nothing.
func setLeaves(obj agentio.Raw) []string {
	m, err := decodeObject(obj)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range leafPaths(obj) {
		if v, ok := valueAt(m, strings.Split(l, pathSep)); ok && isJSONObject(v) && isEmptyRaw(agentio.Raw(v)) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// valueAt is the raw value at path inside m.
func valueAt(m jsonObject, path []string) (json.RawMessage, bool) {
	v, ok := m[path[0]]
	if !ok || len(path) == 1 {
		return v, ok
	}
	sub, err := decodeObject(agentio.Raw(v))
	if err != nil {
		return nil, false
	}
	return valueAt(sub, path[1:])
}

// coveredBy reports a path that one of leaves replaces (or sits under).
func coveredBy(path string, leaves []string) bool {
	for _, l := range leaves {
		if pathsOverlap(l, path) {
			return true
		}
	}
	return false
}

// takeOptionRawSettings removes raw settings flags from the options that
// carry verbatim args (a list option with no flag of its own: Claude's
// extra_args), which the agent places before the other raw args. They are
// raw args too, so they are merged the same way rather than reaching the
// agent as a second settings flag that would drop agx's. A list left
// empty is removed. The flags come back in order.
func takeOptionRawSettings(st *launchState, keys []agentio.OptionKey, ag agentio.Agent) ([]rawSettingsArg, error) {
	var all []rawSettingsArg
	for _, k := range keys {
		raw, ok := st.options[k.Key]
		if !ok || k.Flag != "" || k.Kind != agentio.OptionList {
			continue
		}
		var list []string
		if json.Unmarshal(raw, &list) != nil {
			// Not a list of strings: CheckOptions refuses it with the key.
			continue
		}
		groups, raws, err := takeRawSettings([]argGroup{{src: k.Key, args: list}}, ag)
		if err != nil {
			return nil, err
		}
		if len(raws) == 0 {
			continue
		}
		all = append(all, raws...)
		if len(groups[0].args) == 0 {
			delete(st.options, k.Key)
			continue
		}
		kept, err := json.Marshal(groups[0].args)
		if err != nil {
			return nil, err
		}
		st.options[k.Key] = kept
	}
	return all, nil
}
