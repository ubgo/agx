package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/picker"
)

// pickerApp is an App over a fixture home with two agents' profiles, no
// default_agent, a fake claude on PATH, and a line picker fed answer.
func pickerApp(t *testing.T, answer string) (*app.App, string) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	for _, d := range []string{".claude", ".codex", "bin", "work"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, b), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfgDir := filepath.Join(home, ".agx")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "{\n  // keep me\n  \"version\": 1,\n  \"profiles\": {\n    \"personal\": { \"agent\": \"claude\", \"home\": \"~/.claude\", \"default\": true },\n    \"codex\": { \"agent\": \"codex\", \"home\": \"~/.codex\", \"default\": true }\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.jsonc"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": home, "PATH": bin, "AGX_CONFIG_DIR": cfgDir}
	var environ []string
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	a := app.New(app.Deps{
		Host:   agentio.Host{UserHome: home, Env: environ},
		Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		Cwd:    filepath.Join(home, "work"),
		Picker: picker.NewLine(strings.NewReader(answer), &bytes.Buffer{}, -1),
	})
	return a, filepath.Join(cfgDir, "config.jsonc")
}

// TestEdge03AgentPickerRemembers: nothing names an agent, at a terminal:
// the picker offers each agent, and the "remember" row writes
// default_agent to config.jsonc, keeping its comments.
func TestEdge03AgentPickerRemembers(t *testing.T) {
	a, path := pickerApp(t, "4\n") // rows: claude, codex, claude+remember, codex+remember
	p, err := a.Prepare(t.Context(), app.LaunchOptions{Mode: app.ModeRun, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Agent != "codex" {
		t.Fatalf("agent %s", p.Agent)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"default_agent": "codex"`) || !strings.Contains(string(b), "// keep me") {
		t.Fatalf("config after remember:\n%s", b)
	}
}

// TestEdge03AgentPickerThisRunOnly: a plain row picks for this run only.
func TestEdge03AgentPickerThisRunOnly(t *testing.T) {
	a, path := pickerApp(t, "1\n")
	p, err := a.Prepare(t.Context(), app.LaunchOptions{Mode: app.ModeRun, DryRun: true})
	if err != nil || p.Agent != "claude" {
		t.Fatalf("%v %v", err, p)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "default_agent") {
		t.Fatal("default_agent written without remember")
	}
}

// TestEdge58PickerDismissed: an empty answer (or Esc) starts nothing and
// exits 1 even for a launching command.
func TestEdge58PickerDismissed(t *testing.T) {
	for _, answer := range []string{"\n", "\x1b\n"} {
		a, _ := pickerApp(t, answer)
		_, err := a.Prepare(t.Context(), app.LaunchOptions{Mode: app.ModeRun, DryRun: true})
		if e := apperr.As(err); e.Code != apperr.CodeDismissed || apperr.Exit(err, true) != apperr.ExitFailure {
			t.Fatalf("answer %q: %v (exit %d)", answer, err, apperr.Exit(err, true))
		}
	}
}
