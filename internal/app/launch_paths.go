package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// Tilde forms expanded in path values. Only a leading "~" alone or
// followed by a separator is the user's home: "~user", "$VAR" and globs
// stay as written (no magic, spec 2), as does a "~" anywhere else.
const (
	tildeHome   = "~"
	tildePrefix = "~/"
	// tildePrefixWindows is "~\" on Windows only: elsewhere a backslash is
	// an ordinary file name byte.
	tildePrefixWindows = `~\`
)

// expandTilde expands a leading "~" of p to home; any other p is
// returned unchanged.
func expandTilde(p, home string) string {
	switch {
	case p == tildeHome:
		return home
	case strings.HasPrefix(p, tildePrefix),
		os.PathSeparator == '\\' && strings.HasPrefix(p, tildePrefixWindows):
		return filepath.Join(home, p[len(tildePrefix):])
	}
	return p
}

// pathKeys are the option keys agentio marks as paths (OptionKey.Path):
// the only values agx expands "~" in. Nothing is a path for an agent
// without typed launch options (Gemini CLI, pi, a config-defined agent).
func pathKeys(keys []agentio.OptionKey) []agentio.OptionKey {
	var out []agentio.OptionKey
	for _, k := range keys {
		if k.Path {
			out = append(out, k)
		}
	}
	return out
}

// expandPathOptions is opts with "~" expanded in every value of a path
// option: a string, or each string of a list. Other values (inline JSON
// of a path-or-JSON option, numbers, other keys) are left alone. opts
// itself is not changed, so the decisions keep the value as written.
func expandPathOptions(opts jsonObject, keys []agentio.OptionKey, home string) (jsonObject, error) {
	out := make(jsonObject, len(opts))
	for k, v := range opts {
		out[k] = v
	}
	for _, k := range pathKeys(keys) {
		raw, ok := out[k.Key]
		if !ok {
			continue
		}
		exp, err := expandPathValue(raw, home)
		if err != nil {
			return nil, err
		}
		out[k.Key] = exp
	}
	return out, nil
}

// expandPathValue expands "~" in a JSON string or in each string of a
// JSON list; any other JSON is returned unchanged.
func expandPathValue(raw json.RawMessage, home string) (json.RawMessage, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return json.Marshal(expandTilde(s, home))
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return raw, nil
	}
	for i, el := range list {
		exp, err := expandPathValue(el, home)
		if err != nil {
			return nil, err
		}
		list[i] = exp
	}
	return json.Marshal(list)
}

// expandPathArgs expands "~" in the value of a path option's flag inside
// raw args from the config file (profile args, agent default args): the
// argument after the flag ("--debug-file", "~/d.log") or the part after
// "=" ("--debug-file=~/d.log"). Every other argument stays verbatim. Args
// after "--" never come here: the shell already expanded what was meant
// to be expanded, and a quoted "~" there was quoted on purpose.
func expandPathArgs(args []string, keys []agentio.OptionKey, home string) []string {
	flags := map[string]bool{}
	for _, k := range pathKeys(keys) {
		if k.Flag != "" {
			flags[k.Flag] = true
		}
	}
	if len(flags) == 0 {
		return args
	}
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out); i++ {
		if flags[out[i]] && i+1 < len(out) {
			out[i+1] = expandTilde(out[i+1], home)
			i++
			continue
		}
		if flag, val, ok := strings.Cut(out[i], "="); ok && flags[flag] {
			out[i] = flag + "=" + expandTilde(val, home)
		}
	}
	return out
}

// launchOptionKeys are ag's typed launch options, or none when it has no
// typed launch config or its registry cannot be read (the launch reports
// that error where it builds the options).
func launchOptionKeys(ag agentio.Agent) []agentio.OptionKey {
	lc, ok := agentio.LaunchConfigOf(ag)
	if !ok {
		return nil
	}
	keys, err := lc.OptionKeys()
	if err != nil {
		return nil
	}
	return keys
}
