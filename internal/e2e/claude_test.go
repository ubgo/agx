//go:build !windows

// Package e2e_test holds end-to-end checks against a real agent. They
// are gated: they run the real claude binary with a real login and spend
// a few requests of a small model, so a plain go test ./... skips them.
//
//	AGX_E2E_CLAUDE=1 AGX_E2E_CLAUDE_LOGIN=NAME go test -count=1 -run TestClaudeStopsOnRefusal -v ./internal/e2e/
//
// AGX_E2E_CLAUDE_LOGIN names an agx login (or pool) of your own config
// whose key the test may use: it is read once with agx login token, kept
// in a 0600 file under the test's temp folder and never printed. Or set
// AGX_E2E_CLAUDE_TOKEN to the key itself (from your secret store, never
// on a command line). AGX_E2E_CLAUDE_KEEP=1 also runs the control with
// when_none_usable keep, which must finish: it proves the test still
// sees the behaviour the fix is for. Re-run after every Claude Code
// update.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Gate and inputs.
const (
	envGate  = "AGX_E2E_CLAUDE"
	envLogin = "AGX_E2E_CLAUDE_LOGIN"
	envToken = "AGX_E2E_CLAUDE_TOKEN"
	envKeep  = "AGX_E2E_CLAUDE_KEEP"
	envModel = "AGX_E2E_CLAUDE_MODEL"
)

// defaultModel is the cheapest model that runs a tool.
const defaultModel = "haiku"

// prompt takes three tool calls 12 s apart: longer than the run's key
// refresh (10 s), so Claude asks for its key again between requests.
const prompt = "Run: sleep 12 . Then: sleep 12 . Then: sleep 12 . Then reply DONE"

// doneWord is what the model replies when it got through every step.
const doneWord = "DONE"

// keyedCalls is how many token reads get the real key: the launch's own
// read and Claude's first key request. Every later read gets nothing,
// which agx credential refuses ("token is empty"), as it refuses a pool
// with no member usable.
const keyedCalls = 2

// Time limits: a stopped run is watched this long, then killed; the
// keep control must finish within keepLimit.
const (
	stopWatch = 150 * time.Second
	keepLimit = 4 * time.Minute
)

// keyScript hands out the key for the first keyedCalls reads, then
// nothing; each read is counted in count.
const keyScript = `#!/bin/sh
d=%q
n=$(cat "$d/count" 2>/dev/null || echo 0)
n=$((n+1))
echo "$n" > "$d/count"
if [ "$n" -le %d ]; then cat "$d/key"; fi
`

// TestClaudeStopsOnRefusal: once agx credential has no key to hand out,
// the real Claude Code stops (when_none_usable stop): it never finishes
// the task, keeps asking for a key, and live.jsonc records the refusal.
// With keep (the old behaviour) Claude goes on with its old key and
// finishes, which is the bug stop fixes.
func TestClaudeStopsOnRefusal(t *testing.T) {
	if os.Getenv(envGate) != "1" {
		t.Skipf("set %s=1 and %s or %s: it runs the real claude and spends a few requests", envGate, envLogin, envToken)
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("claude is not on PATH: %v", err)
	}
	bin := buildAgx(t)
	key := realKey(t, bin)
	model := os.Getenv(envModel)
	if model == "" {
		model = defaultModel
	}
	t.Run("stop", func(t *testing.T) {
		r := runClaude(t, bin, key, model, "stop", stopWatch)
		if strings.Contains(r.result, doneWord) {
			t.Fatalf("Claude finished the task after agx refused its key: requests did not stop\n%s", r.tail)
		}
		if r.reads < keyedCalls+2 {
			t.Errorf("Claude asked for a key %d times after the refusal; it should keep asking", r.reads-keyedCalls)
		}
		if r.state.WhenNoneUsable != "stop" || r.state.NoKeySince == "" || !strings.Contains(r.state.NoKey, "token is empty") || r.state.InUse != "" {
			t.Errorf("live.jsonc does not record the refusal: %+v", r.state)
		}
		t.Logf("stop: %d key reads, %s, killed after %s: %v", r.reads, r.state.NoKeyBrief, stopWatch, r.killed)
	})
	if os.Getenv(envKeep) != "1" {
		return
	}
	t.Run("keep control", func(t *testing.T) {
		r := runClaude(t, bin, key, model, "keep", keepLimit)
		if !strings.Contains(r.result, doneWord) {
			t.Fatalf("with keep Claude should go on with its old key and finish; it did not, so this test no longer sees the behaviour stop fixes\n%s", r.tail)
		}
		if r.state.WhenNoneUsable != "keep" {
			t.Errorf("live.jsonc %+v", r.state)
		}
		t.Logf("keep: finished with %d key reads", r.reads)
	})
}

// buildAgx builds this checkout's agx into the test's temp folder.
func buildAgx(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agx")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agx: %v\n%s", err, out)
	}
	return bin
}

// realKey reads the key the test may use, never printing it.
func realKey(t *testing.T, bin string) []byte {
	t.Helper()
	if tok := strings.TrimSpace(os.Getenv(envToken)); tok != "" {
		return []byte(tok)
	}
	name := os.Getenv(envLogin)
	if name == "" {
		t.Fatalf("set %s (an agx login of yours) or %s", envLogin, envToken)
	}
	var out, errb bytes.Buffer
	cmd := exec.Command(bin, "login", "token", "--id", name, "--reveal")
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("agx login token --id %s: %v: %s", name, err, strings.TrimSpace(errb.String()))
	}
	tok := bytes.TrimSpace(out.Bytes())
	if len(tok) == 0 || bytes.ContainsAny(tok, "\n\r") {
		t.Fatalf("agx login token --id %s did not print one line", name)
	}
	return tok
}

// liveFile is the part of live.jsonc the test reads.
type liveFile struct {
	InUse          string `json:"in_use"`
	NoKey          string `json:"no_key"`
	NoKeyBrief     string `json:"no_key_brief"`
	NoKeySince     string `json:"no_key_since"`
	WhenNoneUsable string `json:"when_none_usable"`
}

// runResult is what one run showed.
type runResult struct {
	// result is the text of the stream's result events.
	result string
	// tail is the end of the output, for a failure message.
	tail string
	// reads is how many times the token was read.
	reads  int
	killed bool
	state  liveFile
}

// tailBytes bounds the output kept for a failure message.
const tailBytes = 4000

// runClaude starts claude through agx run --live-login on a temp config
// whose login hands out key keyedCalls times, with when_none_usable mode,
// and waits until it exits or limit passes (then kills its process
// group).
func runClaude(t *testing.T, bin string, key []byte, model, mode string, limit time.Duration) runResult {
	t.Helper()
	dir := t.TempDir()
	cfgDir, home, work := filepath.Join(dir, "cfg"), filepath.Join(dir, "home"), filepath.Join(dir, "work")
	for _, d := range []string{cfgDir, home, work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.sh"), fmt.Appendf(nil, keyScript, dir, keyedCalls), 0o700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"version":1,"default_agent":"claude",
 "profiles":{"e2e":{"agent":"claude","home":%q,"default":true}},
 "live":{"when_none_usable":%q,"status_line":false},
 "logins":{"e2e":{"agents":["claude"],"kind":"oauth-token","token":"$(sh %s)"}}}`, home, mode, filepath.Join(dir, "key.sh"))
	if err := os.WriteFile(filepath.Join(cfgDir, "config.jsonc"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.Command(bin, "run", "--login", "e2e", "--live-login", "--headless", "--model", model, "--prompt", prompt, "--", "--allowedTools", "Bash(sleep:*)")
	cmd.Dir, cmd.Stdout, cmd.Stderr = work, &out, &out
	cmd.Env = append(os.Environ(), "AGX_CONFIG_DIR="+cfgDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	r := runResult{}
	select {
	case <-done:
	case <-ctx.Done():
		// The whole group: claude (agx exec-replaced into it) and its tools.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		r.killed = true
	}
	text := out.String()
	if strings.Contains(text, string(key)) {
		t.Fatal("the key appeared in the output")
	}
	r.tail = text[max(0, len(text)-tailBytes):]
	r.result = resultText(text)
	b, _ := os.ReadFile(filepath.Join(dir, "count"))
	r.reads, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	runs, _ := filepath.Glob(filepath.Join(cfgDir, "runs", "*", "live.jsonc"))
	if len(runs) != 1 {
		t.Fatalf("want one run folder, got %d\n%s", len(runs), r.tail)
	}
	raw, err := os.ReadFile(runs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &r.state); err != nil {
		t.Fatal(err)
	}
	return r
}

// resultText joins the "result" field of the stream's result events.
func resultText(stream string) string {
	var parts []string
	for line := range strings.SplitSeq(stream, "\n") {
		var ev struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Type == "result" {
			parts = append(parts, ev.Result)
		}
	}
	return strings.Join(parts, "\n")
}
