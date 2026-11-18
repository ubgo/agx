package app_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/picker"
)

// pastePicker answers Secret with a fixed paste; other questions fail.
type pastePicker struct{ paste string }

func (p pastePicker) Choose(context.Context, string, []picker.Item) (int, error) {
	return 0, picker.Dismissed()
}

func (p pastePicker) Checklist(context.Context, string, []picker.Item) ([]int, error) {
	return nil, picker.Dismissed()
}

func (p pastePicker) Confirm(context.Context, string, bool) (bool, error) {
	return false, picker.Dismissed()
}

func (p pastePicker) Secret(context.Context, string) (string, error) { return p.paste, nil }

// setupApp is an App with a fake claude on the host PATH and a terminal.
func setupApp(t *testing.T, paste string) (*app.App, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Never run: RunSetup is faked; LookPath only needs it present.
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, ".agx", "config.jsonc")
	host := agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithEnv("PATH="+bin), agenttest.WithHTTP(agenttest.FailingHTTP(t)))
	a := app.New(app.Deps{Host: host, Lookup: func(string) (string, bool) { return "", false }, ConfigFlag: cfgPath, Cwd: home, Picker: pastePicker{paste: paste}})
	return a, filepath.Dir(cfgPath)
}

// TestLoginAddLoginSetup runs the agent's token setup command (faked),
// then stores the pasted token in tokens.jsonc.
func TestLoginAddLoginSetup(t *testing.T) {
	a, dir := setupApp(t, "fake-token-SETUP\n")
	var ran agentio.Command
	src := app.LoginSource{LoginSetup: true, RunSetup: func(_ context.Context, path string, c agentio.Command) (int, error) {
		if filepath.Base(path) != "claude" {
			t.Errorf("path %s", path)
		}
		ran = c
		return 0, nil
	}}
	ch, err := a.LoginAdd(context.Background(), app.LoginAddOptions{Name: "carol", Agents: []agentio.ID{"claude"}, Kind: "oauth-token", Source: src})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(ran.Args, " "), "setup-token") {
		t.Errorf("setup command %v", ran.Args)
	}
	if ch.Info.Source != "stored (tokens.jsonc)" {
		t.Errorf("source %s", ch.Info.Source)
	}
	b, err := os.ReadFile(filepath.Join(dir, "tokens.jsonc"))
	if err != nil || !strings.Contains(string(b), "fake-token-SETUP") {
		t.Fatalf("tokens.jsonc: %v %s", err, b)
	}
}

// TestLoginAddLoginSetupFailedWritesNothing: a failed setup saves nothing.
func TestLoginAddLoginSetupFailedWritesNothing(t *testing.T) {
	a, dir := setupApp(t, "fake-token-SETUP")
	src := app.LoginSource{LoginSetup: true, RunSetup: func(context.Context, string, agentio.Command) (int, error) { return 1, nil }}
	_, err := a.LoginAdd(context.Background(), app.LoginAddOptions{Name: "carol", Agents: []agentio.ID{"claude"}, Kind: "oauth-token", Source: src})
	if err == nil || !strings.Contains(err.Error(), "exited 1; nothing saved") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.jsonc")); err == nil {
		t.Error("config written")
	}
}

// TestLoginAddPrompt: with no source at a terminal the value is asked
// hidden and stored.
func TestLoginAddPrompt(t *testing.T) {
	a, dir := setupApp(t, "fake-token-PROMPTED")
	ch, err := a.LoginAdd(context.Background(), app.LoginAddOptions{Name: "p", Agents: []agentio.ID{"claude"}, Kind: "oauth-token"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "tokens.jsonc"))
	if err != nil || !strings.Contains(string(b), "fake-token-PROMPTED") || ch.Info.SourceType != "stored" {
		t.Fatalf("tokens.jsonc: %v %s %+v", err, b, ch.Info)
	}
}
