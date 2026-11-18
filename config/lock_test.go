package config_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/textdata/atomicfile"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// TestEdge44ConfigLocked: while another writer holds config.jsonc's lock,
// a write waits LockWait and then fails with code locked (exit 1).
func TestEdge44ConfigLocked(t *testing.T) {
	if testing.Short() {
		t.Skip("waits the full lock timeout")
	}
	dir := t.TempDir()
	paths := config.Paths{Dir: dir, File: filepath.Join(dir, "config.jsonc")}
	lk, err := atomicfile.Lock(context.Background(), paths.File)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lk.Close() }()
	start := time.Now()
	err = config.EditConfig(context.Background(), paths, func(d *jsonc.Doc) error { return d.Set("default_agent", "claude") })
	e := apperr.As(err)
	if e.Code != apperr.CodeLocked || !strings.Contains(e.Message, "is being written by another agx") || apperr.Exit(err, false) != 1 {
		t.Fatalf("%v", err)
	}
	if waited := time.Since(start); waited < config.LockWait-time.Second {
		t.Fatalf("gave up after %s, want ~%s", waited, config.LockWait)
	}
}
