package testkit

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// update rewrites golden files: go test ./... -args -update
var update = flag.Bool("update", false, "rewrite testdata/golden files")

// Volatile values replaced before comparing, so goldens pin shapes and
// stable values, not run ids, times or temp paths.
var (
	runIDRe = regexp.MustCompile(`\b[0-9]{8}-[0-9]{6}-[a-z0-9]{4}\b`)
	timeRe  = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[+-][0-9:]+)\b`)
	stampRe = regexp.MustCompile(`\b[0-9]{8}_[0-9]{6}\b`)
)

// Normalize replaces the fixture root, run ids, timestamps and session
// stamps with placeholders.
func (f *Fixture) Normalize(s string) string {
	s = strings.ReplaceAll(s, f.Root, "<root>")
	s = runIDRe.ReplaceAllString(s, "<run-id>")
	s = timeRe.ReplaceAllString(s, "<time>")
	s = stampRe.ReplaceAllString(s, "<stamp>")
	return s
}

// Golden compares got (JSON, re-indented) with testdata/golden/<name>.json
// in the calling package; -update rewrites it. Read a golden diff as a
// behaviour change: never accept one without checking it is right.
func (f *Fixture) Golden(t testing.TB, name, got string) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(strings.TrimSpace(got)), "", "  "); err != nil {
		t.Fatalf("golden %s: output is not JSON: %v\n%s", name, err, got)
	}
	norm := f.Normalize(buf.String()) + "\n"
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(norm), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -args -update to create it)", name, err)
	}
	if string(want) != norm {
		t.Errorf("golden %s differs:\n--- want\n%s\n--- got\n%s", name, want, norm)
	}
}

// GoldenText compares got (human output, byte for byte, escape sequences
// included) with testdata/golden/<name>.txt in the calling package;
// -update rewrites it. It pins a screen's look: a diff is a visible
// change to what a person reads, so check it against agx v0.3's
// reference before accepting it.
func GoldenText(t testing.TB, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -args -update to create it)", name, err)
	}
	if string(want) != got {
		t.Errorf("golden %s differs:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
