package agents

import (
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/config"
)

// SettingsKey is the one key of a profile's agent block that is not a
// launch option: the agent's settings, as a path to a JSONC file
// (relative to the config dir) or an inline object (spec 7.3, 7.4).
const SettingsKey = "settings"

// JSON Schema type names used in the settings property.
const (
	schemaTypeString = "string"
	schemaTypeObject = "object"
)

// blockSchemas builds the profile block schema of every built-in agent
// with typed launch config: agentio's closed options schema (so an
// unknown key fails the config load at its line:col and a wrong type is
// caught early) plus a "settings" property that is a path or an object.
// An agent without the capability, or whose schema cannot be built, has
// no block, so a block for it is an unknown profile key. Pure.
func blockSchemas() config.BlockSchemas {
	set, err := builtin.New(agentio.Host{})
	if err != nil {
		return nil
	}
	out := config.BlockSchemas{}
	for _, a := range set.All() {
		s, ok := blockSchema(a)
		if ok {
			out[a.ID()] = s
		}
	}
	return out
}

// blockSchema is one agent's block schema; ok is false without the
// launch-config capability.
func blockSchema(a agentio.Agent) (*schema.Schema, bool) {
	lc, ok := agentio.LaunchConfigOf(a)
	if !ok {
		return nil, false
	}
	node, err := lc.OptionsSchema()
	if err != nil {
		return nil, false
	}
	raw, err := agentio.MarshalSchema(node)
	if err != nil {
		return nil, false
	}
	s, err := schema.Parse(raw)
	if err != nil {
		return nil, false
	}
	if s.Properties == nil {
		s.Properties = map[string]*schema.Schema{}
	}
	s.Properties[SettingsKey] = &schema.Schema{
		Description: "agent settings: a JSONC file path (relative to the config dir) or an object",
		AnyOf:       []*schema.Schema{{Types: []string{schemaTypeString}}, {Types: []string{schemaTypeObject}}},
	}
	// agentio's neutral options leave settings to LaunchConfig.Settings
	// (one settings channel), so agx's key is never a duplicate here.
	s.PropertyOrder = append(s.PropertyOrder, SettingsKey)
	return s, true
}
