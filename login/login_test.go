package login_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/login"
)

// TestDescribeRef classifies each source without its value.
func TestDescribeRef(t *testing.T) {
	cases := []struct {
		in   string
		want login.Source
	}{
		{"fake-token-ALPHA", login.Source{Type: login.SourcePlain}},
		{"${gopass:personal/claude-token}", login.Source{Type: login.SourceReference, Detail: "gopass"}},
		{"$(op read op://vault/item)", login.Source{Type: login.SourceCommand, Detail: "op"}},
		{"${TOKVAR}", login.Source{Type: login.SourceEnv, Detail: "TOKVAR"}},
		{"${TOKVAR:-x}", login.Source{Type: login.SourceEnv, Detail: "TOKVAR"}},
		{"$TOKVAR", login.Source{Type: login.SourceEnv, Detail: "TOKVAR"}},
		{"trailing$", login.Source{Type: login.SourcePlain}},
	}
	for _, c := range cases {
		if got := login.DescribeRef(c.in); got != c.want {
			t.Errorf("DescribeRef(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
	if s := login.Describe(config.Login{Kind: config.KindAPIKeyHelper, Command: "gopass show -o x"}, login.StoredMissing); s.String() != "helper gopass" {
		t.Errorf("helper: %s", s)
	}
	if s := login.Describe(config.Login{Kind: config.KindOAuthToken, Stored: true}, login.StoredMissing); s.String() != "stored (missing from tokens.jsonc)" {
		t.Errorf("stored missing: %s", s)
	}
	// A refused file (wide mode, bad JSONC) is never reported as missing.
	stored := []struct {
		found bool
		err   error
		want  string
	}{
		{true, nil, "stored (tokens.jsonc)"},
		{false, nil, "stored (missing from tokens.jsonc)"},
		{false, errors.New("mode 0644"), "stored (tokens.jsonc unreadable; agx login check shows why)"},
	}
	for _, c := range stored {
		if s := login.Describe(config.Login{Kind: config.KindOAuthToken, Stored: true}, login.StoredStateOf(c.found, c.err)); s.String() != c.want {
			t.Errorf("found=%v err=%v: %s, want %s", c.found, c.err, s, c.want)
		}
	}
}

// TestValidName keeps names usable in the export line.
func TestValidName(t *testing.T) {
	for _, ok := range []string{"personal", "a.b-c_d", "9x"} {
		if !login.ValidName(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "-x", "a b", "a;b", "a$b"} {
		if login.ValidName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestSchemes lists the plugin schemes (doctor uses it).
func TestSchemes(t *testing.T) {
	e := login.NewExpander(func(string) (string, bool) { return "", false }, "")
	if got := e.Schemes(); !slices.Equal(got, []string{"gopass"}) {
		t.Errorf("schemes %v", got)
	}
}

// fakeGopass writes a gopass stand-in running body.
func fakeGopass(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake")
	}
	p := filepath.Join(t.TempDir(), "gopass")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestEdge16GopassTimeout: a slow or locked gopass stops at the runner
// timeout with the login named, never hangs.
func TestEdge16GopassTimeout(t *testing.T) {
	bin := fakeGopass(t, "exec sleep 5")
	e := login.NewExpander(func(string) (string, bool) { return "", false }, bin).WithTimeout(200 * time.Millisecond)
	start := time.Now()
	_, err := e.Expand(context.Background(), "login personal", "${gopass:personal/claude-token}", true)
	if err == nil {
		t.Fatal("no error")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("waited %s", time.Since(start))
	}
	ae := apperr.As(err)
	if ae.Code != apperr.CodeSecret || ae.Message != "login personal: gopass timed out after 200ms" {
		t.Errorf("error %q (%s)", ae.Message, ae.Code)
	}
}

// TestEdge17MultiLine: a value of several lines is refused by count.
func TestEdge17MultiLine(t *testing.T) {
	bin := fakeGopass(t, `printf 'fake-a\nfake-b\nfake-c\n'`)
	e := login.NewExpander(func(string) (string, bool) { return "", false }, bin)
	v, err := e.Expand(context.Background(), "login personal", "${gopass:x}", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = login.Clean("login personal", v)
	if err == nil || err.Error() != "login personal: token has 3 lines" {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "fake-a") {
		t.Error("value in message")
	}
}
