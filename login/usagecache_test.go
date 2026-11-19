package login_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/login"
)

// TestUsageCacheRoundTrip: a missing cache is empty; updates merge by
// login, write 0600 in a 0700 folder, and read back.
func TestUsageCacheRoundTrip(t *testing.T) {
	paths := config.Paths{Dir: t.TempDir()}
	c, err := login.ReadUsageCache(paths)
	if err != nil || len(c.Logins) != 0 {
		t.Fatalf("empty: %+v %v", c, err)
	}
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if _, err := login.UpdateUsageCache(ctx, paths, map[string]login.UsageEntry{"alice": {Agent: "claude", ProbedAt: at, Status: "ok"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := login.UpdateUsageCache(ctx, paths, map[string]login.UsageEntry{"bob": {Agent: "claude", ProbedAt: at, Status: "rejected", Reason: "token rejected"}}); err != nil {
		t.Fatal(err)
	}
	c, err = login.ReadUsageCache(paths)
	if err != nil || c.Logins["alice"].Status != "ok" || c.Logins["bob"].Reason != "token rejected" {
		t.Fatalf("read back %+v %v", c, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	st, _ := os.Stat(paths.LoginUsage())
	dir, _ := os.Stat(filepath.Dir(paths.LoginUsage()))
	if st.Mode().Perm() != 0o600 || dir.Mode().Perm() != 0o700 {
		t.Errorf("modes %v %v", st.Mode(), dir.Mode())
	}
}

// TestUsageCacheBrokenOrForeign: a broken or other-version cache reads
// as empty with an error (the caller warns) and is replaced on update.
func TestUsageCacheBrokenOrForeign(t *testing.T) {
	paths := config.Paths{Dir: t.TempDir()}
	if err := os.MkdirAll(filepath.Dir(paths.LoginUsage()), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"{broken", `{"version": 9, "logins": {}}`} {
		if err := os.WriteFile(paths.LoginUsage(), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if c, err := login.ReadUsageCache(paths); err == nil || len(c.Logins) != 0 {
			t.Errorf("%q: %+v %v", body, c, err)
		}
	}
	if _, err := login.UpdateUsageCache(context.Background(), paths, map[string]login.UsageEntry{"alice": {Status: "ok"}}); err != nil {
		t.Fatal(err)
	}
	if c, err := login.ReadUsageCache(paths); err != nil || len(c.Logins) != 1 {
		t.Errorf("after replace %+v %v", c, err)
	}
}

// TestAppendPoolLog appends lines to a new 0600 file.
func TestAppendPoolLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logs", "pool.log")
	for _, l := range []string{"one", "two"} {
		if err := login.AppendPoolLog(p, l); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	if strings.TrimSpace(string(b)) != "one\ntwo" {
		t.Errorf("log %q", b)
	}
}
