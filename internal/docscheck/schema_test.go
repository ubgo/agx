package docscheck

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc/schema"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
)

// schemaFile is the published editor schema (config.jsonc "$schema"),
// generated from config.ConfigSchema; -update rewrites it.
const schemaFile = "schema/config.schema.json"

// configReference is the guide that must name every config key.
const configReference = docsDir + "/config.md"

// updateFlag is testkit's -update flag (one flag rewrites every generated
// file of a test run; defining a second one would panic at init).
const updateFlag = "update"

// updating reports whether the run was started with -args -update.
func updating() bool {
	f := flag.Lookup(updateFlag)
	return f != nil && f.Value.String() == "true"
}

// generatedSchema is config.jsonc's schema, as agx validates it.
func generatedSchema(t *testing.T) *schema.Schema {
	t.Helper()
	s, err := config.ConfigSchema(agents.BlockSchemas())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestConfigSchemaFile keeps schema/config.schema.json equal to the schema
// agx validates with; run with -args -update after a config change.
func TestConfigSchemaFile(t *testing.T) {
	root := repoRoot(t)
	b, err := generatedSchema(t).JSON()
	if err != nil {
		t.Fatal(err)
	}
	b = append(bytes.TrimRight(b, "\n"), '\n')
	p := filepath.Join(root, filepath.FromSlash(schemaFile))
	if updating() {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, generatedFileMode); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%s: %v (run: go test ./internal/docscheck/ -run TestConfigSchemaFile -args -update)", schemaFile, err)
	}
	if !bytes.Equal(have, b) {
		t.Errorf("%s is out of date (run: go test ./internal/docscheck/ -run TestConfigSchemaFile -args -update)", schemaFile)
	}
}

// generatedFileMode is the permission of generated files (the usual 0644).
const generatedFileMode = 0o644

// mapPlaceholders name the map keys in documented key paths, by the path
// of the map: profiles.<profile>.home, projects.<folder>.<agent>.login.
var mapPlaceholders = map[string]string{
	"agents":            "<agent>",
	"profiles":          "<profile>",
	"logins":            "<login>",
	"projects":          "<folder>",
	"projects.<folder>": "<agent>",
	"aliases":           "<alias>",
}

// genericPlaceholder names a map key no entry above covers.
const genericPlaceholder = "<key>"

// keyPaths lists every property of s as a documented key path, $refs
// into root's $defs followed (a struct used in several places, like the
// pool's caps blocks, is generated once as a definition).
func keyPaths(root, s *schema.Schema, prefix string, out map[string]bool) {
	if s == nil {
		return
	}
	if s.Ref != "" {
		keyPaths(root, root.Defs[strings.TrimPrefix(s.Ref, "#/$defs/")], prefix, out)
		return
	}
	join := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + "." + name
	}
	for name, p := range s.Properties {
		out[join(name)] = true
		keyPaths(root, p, join(name), out)
	}
	if ap := s.AdditionalProperties; ap != nil && (len(ap.Properties) > 0 || ap.AdditionalProperties != nil || len(ap.AnyOf) > 0) {
		ph, ok := mapPlaceholders[prefix]
		if !ok {
			ph = genericPlaceholder
		}
		keyPaths(root, ap, join(ph), out)
	}
	if s.Items != nil {
		keyPaths(root, s.Items, prefix+"[]", out)
	}
	for _, alt := range s.AnyOf {
		keyPaths(root, alt, prefix, out)
	}
}

// keyCell matches a table row whose first cell is a key path in code.
var keyCell = regexp.MustCompile("^\\|\\s*`([^`]+)`")

// TestConfigReference fails when docs/config.md misses a key of the
// config schema, or its key tables list a key the schema does not have.
func TestConfigReference(t *testing.T) {
	root := repoRoot(t)
	want := map[string]bool{}
	gen := generatedSchema(t)
	keyPaths(gen, gen, "", want)
	lines := readLines(t, root, configReference)
	text := strings.Join(lines, "\n")
	var missing []string
	for k := range want {
		if !strings.Contains(text, "`"+k+"`") {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	for _, k := range missing {
		t.Errorf("%s: config key %q is not documented (name it as `%s`)", configReference, k, k)
	}
	tops := map[string]bool{}
	for k := range want {
		top, _, _ := strings.Cut(k, ".")
		tops[strings.TrimSuffix(top, "[]")] = true
	}
	fenced := fencedLines(lines)
	for i, l := range lines {
		m := keyCell.FindStringSubmatch(l)
		if m == nil || fenced[i] {
			continue
		}
		top, _, _ := strings.Cut(m[1], ".")
		if tops[top] && !want[m[1]] {
			t.Errorf("%s:%d: documents config key %q, which the schema does not have", configReference, i+1, m[1])
		}
	}
}
