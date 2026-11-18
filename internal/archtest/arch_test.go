// Package archtest enforces agx's layer rules (spec 16.1) by reading the
// source: which package may import cobra, start processes, write files,
// or import an agent module. A rule broken by a new file fails here, not
// in review.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// module is agx's module path.
const module = "github.com/ubgo/agx"

// pkgFile is one parsed non-test Go file.
type pkgFile struct {
	pkg  string // module-relative package dir ("internal/cli")
	path string
	file *ast.File
}

// repoRoot walks up from this package to go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// testSupport packages are test helpers (they import testing, build the
// binary and write fixtures); they never ship in the agx binary.
var testSupport = []string{"internal/testkit"}

// files parses every non-test Go file of the module, except test support.
func files(t *testing.T) []pkgFile {
	t.Helper()
	root := repoRoot(t)
	var out []pkgFile
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "dist" || strings.HasPrefix(d.Name(), ".")) && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		full, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		if slices.Contains(testSupport, filepath.ToSlash(rel)) {
			return nil
		}
		out = append(out, pkgFile{pkg: filepath.ToSlash(rel), path: p, file: full})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no Go files found")
	}
	return out
}

// imports lists a file's import paths.
func imports(f *ast.File) []string {
	var out []string
	for _, is := range f.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err == nil {
			out = append(out, p)
		}
	}
	return out
}

// calls lists "pkg.Func" selector calls in a file, with the import name
// resolved to the import path's last element.
func calls(f *ast.File) []string {
	alias := map[string]string{}
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		name := filepath.Base(p)
		if is.Name != nil {
			name = is.Name.Name
		}
		alias[name] = p
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if p, ok := alias[id.Name]; ok {
			out = append(out, p+"."+sel.Sel.Name)
		}
		return true
	})
	return out
}

// TestOnlyCLIImportsCobra: cobra and pflag stay in internal/cli.
func TestOnlyCLIImportsCobra(t *testing.T) {
	for _, f := range files(t) {
		for _, imp := range imports(f.file) {
			if (strings.HasPrefix(imp, "github.com/spf13/cobra") || strings.HasPrefix(imp, "github.com/spf13/pflag")) && f.pkg != "internal/cli" {
				t.Errorf("%s imports %s; only internal/cli may", f.path, imp)
			}
		}
	}
}

// TestAppStaysPure: internal/app never imports cobra, render or os/exec.
func TestAppStaysPure(t *testing.T) {
	banned := []string{"github.com/spf13/cobra", module + "/internal/render", "os/exec"}
	for _, f := range files(t) {
		if f.pkg != "internal/app" {
			continue
		}
		for _, imp := range imports(f.file) {
			if slices.Contains(banned, imp) {
				t.Errorf("%s imports %s; internal/app must not", f.path, imp)
			}
		}
	}
}

// processStarters are calls that start a process.
var processStarters = []string{"os/exec.Command", "os/exec.CommandContext", "syscall.Exec", "os.StartProcess", "syscall.ForkExec"}

// TestOnlyRunStartsProcesses: process starts live in internal/run (the
// textdata shell runner is a library and outside this scan).
func TestOnlyRunStartsProcesses(t *testing.T) {
	for _, f := range files(t) {
		if f.pkg == "internal/run" {
			continue
		}
		for _, c := range calls(f.file) {
			if slices.Contains(processStarters, c) {
				t.Errorf("%s calls %s; only internal/run starts processes", f.path, c)
			}
		}
	}
}

// fileWriters are calls that write or remove files.
var fileWriters = []string{
	"os.WriteFile", "os.Create", "os.OpenFile", "os.Mkdir", "os.MkdirAll", "os.MkdirTemp", "os.CreateTemp",
	"os.Remove", "os.RemoveAll", "os.Rename", "os.Chmod", "os.Symlink", "os.Link", "os.Truncate",
	"github.com/ubgo/textdata/atomicfile.Write", "github.com/ubgo/textdata/jsonc.Update",
	"github.com/ubgo/textdata/dotenv.Update",
}

// writerPackages may write files (spec 16.1).
var writerPackages = []string{"config", "login", "live", "sessions", "internal/run", "internal/plan"}

// TestOnlyOwnersWriteFiles: file writes stay in the owning packages.
func TestOnlyOwnersWriteFiles(t *testing.T) {
	for _, f := range files(t) {
		if slices.Contains(writerPackages, f.pkg) {
			continue
		}
		for _, c := range calls(f.file) {
			if slices.Contains(fileWriters, c) {
				t.Errorf("%s calls %s; only %v write files", f.path, c, writerPackages)
			}
		}
	}
}

// agentModules are agentio's vendor modules.
var agentModules = []string{"github.com/ubgo/agentio/builtin", "github.com/ubgo/agentio/claude", "github.com/ubgo/agentio/codex", "github.com/ubgo/agentio/gemini", "github.com/ubgo/agentio/pi"}

// TestAgentModulesOnlyInRegistry: vendor modules are imported only by the
// registry package, so agx stays agent-neutral (principle 10).
func TestAgentModulesOnlyInRegistry(t *testing.T) {
	for _, f := range files(t) {
		if f.pkg == "internal/agents" {
			continue
		}
		for _, imp := range imports(f.file) {
			for _, m := range agentModules {
				if imp == m || strings.HasPrefix(imp, m+"/") {
					t.Errorf("%s imports %s; only internal/agents may import agent modules", f.path, imp)
				}
			}
		}
	}
}
