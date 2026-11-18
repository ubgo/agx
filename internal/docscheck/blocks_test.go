package docscheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"
)

// The agent-block keys of docs/config.md are generated from the schema
// (they come from each agent's option registry in agentio and follow its
// releases): the lines between these markers are rewritten by -update and
// compared otherwise.
const (
	blocksStart = "<!-- generated: agent-blocks (go test ./internal/docscheck/ -run TestAgentBlockTable -args -update) -->"
	blocksEnd   = "<!-- end generated: agent-blocks -->"
)

// profileDef is the profile node of the config schema.
func profileDef(s *schema.Schema) *schema.Schema {
	ps, ok := s.Properties["profiles"]
	if !ok {
		return nil
	}
	return ps.AdditionalProperties
}

// typeText renders a property's type for a reference table: "string",
// "list of strings", "path or object", with enum values listed.
func typeText(p *schema.Schema) string {
	if len(p.AnyOf) > 0 {
		var alts []string
		for _, a := range p.AnyOf {
			alts = append(alts, typeText(a))
		}
		return strings.Join(alts, " or ")
	}
	var names []string
	for _, ty := range p.Types {
		switch ty {
		case "null":
			continue
		case "array":
			item := "value"
			if p.Items != nil && len(p.Items.Types) > 0 {
				item = p.Items.Types[0]
			}
			names = append(names, "list of "+item+"s")
		case "boolean":
			names = append(names, "bool")
		default:
			names = append(names, ty)
		}
	}
	t := strings.Join(names, " or ")
	if t == "" {
		t = "any JSON"
	}
	if len(p.Enum) > 0 {
		var vals []string
		for _, e := range p.Enum {
			var v string
			if json.Unmarshal(e, &v) == nil {
				vals = append(vals, "`"+v+"`")
			}
		}
		t += ": " + strings.Join(vals, ", ")
	}
	return t
}

// noFlagKeysLink is where the guide explains the block keys that pass no
// flag of their own (prompt, extra_args): the only keys whose schema has
// no description. TestLinks fails if the heading is renamed.
const noFlagKeysLink = "launch-options.md#block-keys-with-no-flag-of-their-own-prompt-and-extra_args"

// blockKeyOrder is the order of a block's keys in its table: the schema's
// property order, then any property it does not list (sorted), each key
// once. The schema's order can name a key twice (claude's settings is
// listed by its option registry and again as the common settings key),
// which printed a duplicate row.
func blockKeyOrder(b *schema.Schema) []string {
	var order []string
	seen := map[string]bool{}
	add := func(k string) {
		if _, ok := b.Properties[k]; ok && !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	for _, k := range b.PropertyOrder {
		add(k)
	}
	rest := make([]string, 0, len(b.Properties))
	for k := range b.Properties {
		rest = append(rest, k)
	}
	slices.Sort(rest)
	for _, k := range rest {
		add(k)
	}
	return order
}

// blockTable renders the generated part: one table per agent with a typed
// block, rows in the schema's property order.
func blockTable(s *schema.Schema) ([]string, error) {
	prof := profileDef(s)
	if prof == nil {
		return nil, fmt.Errorf("profile definition missing from the schema")
	}
	var agents []string
	for name, p := range prof.Properties {
		if len(p.Properties) > 0 {
			agents = append(agents, name)
		}
	}
	slices.Sort(agents)
	var out []string
	for _, a := range agents {
		b := prof.Properties[a]
		out = append(out, "", "#### `profiles.<profile>."+a+"`", "", "| Key | Type | Passes |", "|---|---|---|")
		for _, k := range blockKeyOrder(b) {
			p := b.Properties[k]
			desc := p.Description
			if strings.HasPrefix(desc, "-") {
				desc = "`" + desc + "`"
			}
			if desc == "" {
				desc = "see [launch options](" + noFlagKeysLink + ")"
			}
			out = append(out, fmt.Sprintf("| `profiles.<profile>.%s.%s` | %s | %s |", a, k, typeText(p), desc))
		}
	}
	return append(out, ""), nil
}

// TestAgentBlockTable keeps the generated agent-block tables of
// docs/config.md equal to the schema; -update rewrites them.
func TestAgentBlockTable(t *testing.T) {
	root := repoRoot(t)
	want, err := blockTable(generatedSchema(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, root, configReference)
	start, end := slices.Index(lines, blocksStart), slices.Index(lines, blocksEnd)
	if start < 0 || end < start {
		t.Fatalf("%s: the markers %q and %q are missing", configReference, blocksStart, blocksEnd)
	}
	got := lines[start+1 : end]
	if slices.Equal(got, want) {
		return
	}
	if !updating() {
		t.Fatalf("%s:%d: the agent-block tables are out of date (run: go test ./internal/docscheck/ -run TestAgentBlockTable -args -update)", configReference, start+1)
	}
	next := append(append(append([]string(nil), lines[:start+1]...), want...), lines[end:]...)
	p := filepath.Join(root, filepath.FromSlash(configReference))
	if err := os.WriteFile(p, []byte(strings.Join(next, "\n")+"\n"), generatedFileMode); err != nil {
		t.Fatal(err)
	}
}
