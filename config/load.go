package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/internal/apperr"
)

// Known is what the config is checked against beyond its own shape: the
// agents that exist (built-ins plus custom specs, from the registry), each
// agent's typed launch block schema, and an alias validator from the CLI.
type Known struct {
	// Agents are the built-in agent ids; custom ids come from the file.
	Agents []agentio.ID
	// Blocks are the typed profile block schemas per agent.
	Blocks BlockSchemas
	// Alias checks one alias value ("run --profile work"); nil = unchecked.
	Alias func(args string) error
}

// BlockSchemas gives, per agent id, the JSON Schema of that agent's typed
// launch block in a profile (settings plus launch option keys). Built by
// the agent registry from agentio; an agent without one has no block key
// at all, so "gemini": {...} inside a profile is an unknown key.
type BlockSchemas map[agentio.ID]*schema.Schema

// Problem is one config error with its position (file:line:col).
type Problem struct {
	// Where is "config.jsonc:12:5" (file base name, line, column); "" when
	// the problem has no single position.
	Where string `json:"where,omitempty"`
	// Path is the JSON path of the value ("profiles.work.home").
	Path string `json:"path,omitempty"`
	// Message is the problem, without position.
	Message string `json:"message"`
}

// String renders "where: message".
func (p Problem) String() string {
	if p.Where == "" {
		return p.Message
	}
	return p.Where + ": " + p.Message
}

// Loaded is a config read from disk with its document, kept for positions
// in later messages and for the source map (config show).
type Loaded struct {
	// Config is the decoded file; zero value (no profiles) when missing.
	Config Config
	// Paths are the files it came from.
	Paths Paths
	// Exists is false when config.jsonc does not exist (edge 2).
	Exists bool
	// Doc is the parsed document; nil when the file does not exist.
	Doc *jsonc.Doc
}

// Where returns "config.jsonc:L:C" for the key at keys (a JSON Pointer
// built from them), or "config.jsonc" when the key is absent.
func (l *Loaded) Where(keys ...string) string {
	base := baseName(l.Paths.File)
	if l.Doc == nil {
		return base
	}
	p, err := l.Doc.KeyPos(Pointer(keys...))
	if err != nil {
		return base
	}
	return fmt.Sprintf("%s:%d:%d", base, p.Line, p.Col)
}

// ProfileOrder is the profile names in the order the file lists them.
// The decoded Profiles map has no order; listings follow the file (as
// agx v0.3's list did) so a person sees their own arrangement. Names the
// document does not hold (none, for a loaded file) are appended sorted.
func (l *Loaded) ProfileOrder() []string { return l.keyOrder("profiles", mapKeys(l.Config.Profiles)) }

// LoginOrder is the login names in the order the file lists them; the
// usage table's # column is a login's 1-based position here.
func (l *Loaded) LoginOrder() []string { return l.keyOrder("logins", mapKeys(l.Config.Logins)) }

// keyOrder is the keys of the object at top-level key in document order,
// limited to have, plus any of have the document lacks, sorted.
func (l *Loaded) keyOrder(key string, have []string) []string {
	sort.Strings(have)
	var out []string
	seen := map[string]bool{}
	if l.Doc != nil {
		if keys, err := l.Doc.Keys(Pointer(key)); err == nil {
			for _, k := range keys {
				if slices.Contains(have, k) && !seen[k] {
					out, seen[k] = append(out, k), true
				}
			}
		}
	}
	for _, k := range have {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// mapKeys is the keys of m, unordered.
func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// baseName is the last element of a path, for messages.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ConfigError is the error Load returns for an invalid file: every
// problem found (config check lists them all, edge 45), and the first one
// as the message.
type ConfigError struct {
	Problems []Problem
}

// Error renders the first problem plus a count of the rest.
func (e *ConfigError) Error() string {
	if len(e.Problems) == 0 {
		return "invalid config"
	}
	msg := e.Problems[0].String()
	if n := len(e.Problems) - 1; n > 0 {
		msg += fmt.Sprintf(" (and %d more; agx config check lists them)", n)
	}
	return msg
}

// Load reads and validates the config at paths.File. A missing file is not
// an error: it returns an empty config with Exists false (read commands
// show nothing, launches refuse with the init hint). Every other problem
// is an *apperr.Error with Code config wrapping a *ConfigError.
func Load(paths Paths, known Known) (*Loaded, error) {
	l := &Loaded{Paths: paths}
	src, err := os.ReadFile(paths.File)
	if errors.Is(err, fs.ErrNotExist) {
		l.Config.Version = CurrentVersion
		return l, nil
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, "read "+paths.File+": "+err.Error())
	}
	l.Exists = true
	return l, l.parse(src, known)
}

// Parse validates src as a config (for tests and config check on bytes).
func Parse(src []byte, paths Paths, known Known) (*Loaded, error) {
	l := &Loaded{Paths: paths, Exists: true}
	return l, l.parse(src, known)
}

// parse runs every stage and collects problems: syntax, schema (unknown
// keys and types, all at once), version, decode, cross-checks.
func (l *Loaded) parse(src []byte, known Known) error {
	name := baseName(l.Paths.File)
	doc, err := jsonc.Parse(src, jsonc.WithFileName(name))
	if err != nil {
		return configErr([]Problem{{Message: err.Error()}})
	}
	if derr := doc.Err(); derr != nil {
		return configErr([]Problem{{Message: derr.Error()}})
	}
	l.Doc = doc
	if v, verr := doc.GetInt("version"); verr == nil && v > CurrentVersion {
		return apperr.New(apperr.CodeConfigVersion, apperr.ClassFailure,
			fmt.Sprintf("config version %d is newer than this agx (%d)", v, CurrentVersion)).
			WithHint("upgrade agx: go install github.com/ubgo/agx@latest")
	}
	s, err := ConfigSchema(known.Blocks)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, "config schema: "+err.Error())
	}
	res := schema.Validate(doc, s, schema.StrictKeys(), schema.WithFileName(name))
	var problems []Problem
	for _, is := range res.Errors {
		problems = append(problems, Problem{Where: fmt.Sprintf("%s:%d:%d", name, is.Pos.Line, is.Pos.Col), Path: is.Path, Message: issueMessage(is)})
	}
	if len(problems) > 0 {
		return configErr(problems)
	}
	if err := doc.Decode(&l.Config); err != nil {
		return configErr([]Problem{{Message: err.Error()}})
	}
	if err := l.fillBlocks(); err != nil {
		return configErr([]Problem{{Message: err.Error()}})
	}
	if l.Config.Version == 0 {
		l.Config.Version = CurrentVersion
	}
	if problems := l.CrossCheck(known); len(problems) > 0 {
		// A cross-check problem is a usage-class error (edge 5: exit 2),
		// unlike a file that does not parse (edge 45: exit 1).
		ce := &ConfigError{Problems: problems}
		return apperr.Wrap(ce, apperr.CodeConfig, apperr.ClassUsage, ce.Error())
	}
	return nil
}

// issueMessage renders a schema issue without its position (the position
// goes in Problem.Where), keeping the path for context.
func issueMessage(is schema.Issue) string {
	if is.Path == "" {
		return is.Message
	}
	return is.Path + ": " + is.Message
}

// configErr wraps problems in the apperr shape.
func configErr(problems []Problem) error {
	ce := &ConfigError{Problems: problems}
	return apperr.Wrap(ce, apperr.CodeConfig, apperr.ClassFailure, ce.Error())
}

// ProblemsOf returns every problem inside err (one for a non-config error).
func ProblemsOf(err error) []Problem {
	var ce *ConfigError
	if errors.As(err, &ce) {
		return ce.Problems
	}
	return []Problem{{Message: err.Error()}}
}

// profileKeys are the JSON keys of Profile's own fields; any other key of
// a profile object is an agent block.
func profileKeys() []string {
	t := reflect.TypeFor[Profile]()
	var out []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	return out
}

// fillBlocks copies each profile's agent-block keys (validated by the
// schema already) into Profile.Blocks as raw JSON.
func (l *Loaded) fillBlocks() error {
	known := profileKeys()
	for name, p := range l.Config.Profiles {
		keys, err := l.Doc.Keys(Pointer("profiles", name))
		if err != nil {
			return err
		}
		for _, k := range keys {
			if slices.Contains(known, k) {
				continue
			}
			v, err := l.Doc.Get(Pointer("profiles", name, k))
			if err != nil {
				return err
			}
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			if p.Blocks == nil {
				p.Blocks = map[agentio.ID]json.RawMessage{}
			}
			p.Blocks[agentio.ID(k)] = raw
		}
		l.Config.Profiles[name] = p
	}
	return nil
}

// ConfigSchema is the JSON Schema of config.jsonc: generated from Config,
// with each agent's typed block added to the profile definition. The same
// schema is published for editors (schema/config.schema.json) and used by
// Load with StrictKeys.
func ConfigSchema(blocks BlockSchemas) (*schema.Schema, error) {
	s, err := schema.FromStruct(Config{}, schema.WithTitle("agx config"))
	if err != nil {
		return nil, err
	}
	prof := profileSchema(s)
	if prof == nil {
		return nil, errors.New("profile definition missing from the generated schema")
	}
	ids := make([]string, 0, len(blocks))
	for id := range blocks {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		if prof.Properties == nil {
			prof.Properties = map[string]*schema.Schema{}
		}
		prof.Properties[id] = blocks[agentio.ID(id)]
		prof.PropertyOrder = append(prof.PropertyOrder, id)
	}
	return s, nil
}

// profileSchema finds the Profile node: a $defs entry when FromStruct
// shared it, else the inline additionalProperties of "profiles".
func profileSchema(s *schema.Schema) *schema.Schema {
	if d, ok := s.Defs["Profile"]; ok {
		return d
	}
	ps, ok := s.Properties["profiles"]
	if !ok || ps.AdditionalProperties == nil {
		return nil
	}
	ap := ps.AdditionalProperties
	if ap.Ref != "" {
		return s.Defs[strings.TrimPrefix(ap.Ref, "#/$defs/")]
	}
	if len(ap.AnyOf) > 0 {
		for _, alt := range ap.AnyOf {
			if alt.Properties != nil {
				return alt
			}
		}
	}
	return ap
}
