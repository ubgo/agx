package plan_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/plan"
)

// t0 is a fixed clock for every test.
var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// writePlan writes a valid plan created at t0 and returns its path.
func writePlan(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plan")
	f := plan.File{SchemaVersion: plan.SchemaVersion, CreatedAt: t0, Dir: "/some/dir", Launch: json.RawMessage(`{"k":1}`)}
	if err := plan.Write(p, f); err != nil {
		t.Fatal(err)
	}
	return p
}

// wantRefusal asserts a CodePlanFile refusal with msg ("" = any message).
func wantRefusal(t *testing.T, err error, msg string) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != apperr.CodePlanFile {
		t.Fatalf("want plan-file refusal, got %v", err)
	}
	if msg != "" && e.Message != msg {
		t.Fatalf("message %q, want %q", e.Message, msg)
	}
}

func TestPlanRoundTripIsPrivate(t *testing.T) {
	p := writePlan(t)
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != plan.FileMode {
		t.Errorf("mode %v, want %v", st.Mode().Perm(), plan.FileMode)
	}
	f, err := plan.Read(p, t0.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var launch struct{ K int }
	if err := json.Unmarshal(f.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	if f.Dir != "/some/dir" || launch.K != 1 || !f.CreatedAt.Equal(t0) {
		t.Errorf("read back %+v", f)
	}
}

func TestPlanStaleAndFutureRefused(t *testing.T) {
	p := writePlan(t)
	_, err := plan.Read(p, t0.Add(plan.MaxAge+time.Second))
	wantRefusal(t, err, plan.MsgStaleOrForeign)
	_, err = plan.Read(p, t0.Add(-time.Minute))
	wantRefusal(t, err, plan.MsgStaleOrForeign)
}

func TestPlanWritableByOthersRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix permission bits")
	}
	p := writePlan(t)
	if err := os.Chmod(p, 0o622); err != nil {
		t.Fatal(err)
	}
	_, err := plan.Read(p, t0)
	wantRefusal(t, err, plan.MsgStaleOrForeign)
}

func TestPlanSymlinkRefused(t *testing.T) {
	p := writePlan(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(p, link); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	_, err := plan.Read(link, t0)
	wantRefusal(t, err, plan.MsgStaleOrForeign)
}

func TestPlanWriteReplacesSymlinkNotTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "plan")
	if err := os.Symlink(victim, link); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	if err := plan.Write(link, plan.File{SchemaVersion: plan.SchemaVersion, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("write went through the symlink: %q", b)
	}
}

func TestPlanSchemaMissingAndGarbage(t *testing.T) {
	_, err := plan.Read(filepath.Join(t.TempDir(), "none"), t0)
	wantRefusal(t, err, "")

	p := writePlan(t)
	if err := os.WriteFile(p, []byte(`{"schema_version": 99, "created_at": "2026-11-17T12:00:00Z"}`), plan.FileMode); err != nil {
		t.Fatal(err)
	}
	_, err = plan.Read(p, t0)
	wantRefusal(t, err, "")

	if err := os.WriteFile(p, []byte(`not json`), plan.FileMode); err != nil {
		t.Fatal(err)
	}
	_, err = plan.Read(p, t0)
	wantRefusal(t, err, "")
}

func TestPlanRemoveIsIdempotent(t *testing.T) {
	p := writePlan(t)
	if err := plan.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := plan.Remove(p); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}
