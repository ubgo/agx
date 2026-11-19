// Package agents is the one place agx assembles agentio agents: the
// built-ins (through agentio/builtin) plus config-defined agents from
// agents.<id>.spec. Everything else in agx sees only agentio.Agent and the
// typed accessors, so agx has no "if claude" branch (principle 10); this
// file is the single importer of agent modules the layer rules allow
// (spec 16.1, guarded by internal/archtest).
package agents

import (
	"fmt"
	"slices"
	"sort"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"

	"github.com/ubgo/agx/config"
)

// Registry is every agent agx knows on one Host, in a stable order:
// built-ins first (claude, codex, gemini, pi), then custom agents by id.
type Registry struct {
	host agentio.Host
	all  []agentio.Agent
}

// New builds the registry. custom maps an agent id to its spec (the map
// key wins over any "id" inside the spec); a spec that fails
// agentio.Spec.Validate or reuses a built-in id is an error. Pure.
func New(h agentio.Host, custom map[agentio.ID]agentio.Spec) (*Registry, error) {
	ids := make([]string, 0, len(custom))
	for id := range custom {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	specs := make([]agentio.Spec, 0, len(ids))
	for _, id := range ids {
		s := custom[agentio.ID(id)]
		s.ID = agentio.ID(id)
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("agents.%s.spec: %w", id, err)
		}
		specs = append(specs, s)
	}
	set, err := builtin.New(h, specs...)
	if err != nil {
		return nil, err
	}
	return &Registry{host: h, all: set.All()}, nil
}

// BuiltinIDs lists the built-in agent ids without a real host (config
// checks need them before a Host exists). Pure.
func BuiltinIDs() []agentio.ID {
	set, err := builtin.New(agentio.Host{})
	if err != nil {
		return nil
	}
	out := make([]agentio.ID, 0, len(set.All()))
	for _, a := range set.All() {
		out = append(out, a.ID())
	}
	return out
}

// Host is the host the agents were built on.
func (r *Registry) Host() agentio.Host { return r.host }

// All returns every agent in registry order.
func (r *Registry) All() []agentio.Agent { return slices.Clone(r.all) }

// Get finds an agent by id.
func (r *Registry) Get(id agentio.ID) (agentio.Agent, bool) {
	for _, a := range r.all {
		if a.ID() == id {
			return a, true
		}
	}
	return nil, false
}

// IDs lists every agent id in registry order.
func (r *Registry) IDs() []agentio.ID {
	out := make([]agentio.ID, len(r.all))
	for i, a := range r.all {
		out[i] = a.ID()
	}
	return out
}

// IsCustom reports a config-defined agent.
func (r *Registry) IsCustom(id agentio.ID) bool {
	return !slices.Contains(BuiltinIDs(), id)
}

// BlockSchemas returns the JSON Schema of each built-in agent's typed
// profile block ("claude": {...}), from agentio's launch-config
// capability. Agents without typed launch config have no block.
func BlockSchemas() config.BlockSchemas {
	return blockSchemas()
}

// reasonConfigAgent is why a config-defined agent cannot use a pool: a
// spec models no live credential and no token probe.
const reasonConfigAgent = "a config-defined agent has no live credential and no token probe"

// PoolReason says why agent id cannot use a login pool ("" = it can): a
// pool needs the live-credential capability (the agent re-runs a helper
// for its key while it runs, so each key can come from another member)
// and the token prober (agx reads each member's usage). The reason is
// agentio's typed one for the first capability missing. Pure.
func PoolReason(id agentio.ID) string {
	set, err := builtin.New(agentio.Host{})
	if err != nil {
		return err.Error()
	}
	for _, a := range set.All() {
		if a.ID() != id {
			continue
		}
		have := agentio.Supported(a)
		for _, c := range []agentio.Capability{agentio.CapLiveCredential, agentio.CapTokenProber} {
			if slices.Contains(have, c) {
				continue
			}
			if reason, ok := agentio.Unsupported(a, c); ok && reason != "" {
				return reason
			}
			return "no " + c.String()
		}
		return ""
	}
	return reasonConfigAgent
}
