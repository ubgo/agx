package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// TestSlugOf: the default promote name is the part before the stamp.
func TestSlugOf(t *testing.T) {
	cases := map[string]string{
		"pglite_golang_20261114_101500":   "pglite_golang",
		"pglite_golang_20261114_101500_2": "pglite_golang",
		"20261114_101500":                 "",
		"hand-made":                       "hand-made",
	}
	for in, want := range cases {
		if got := SlugOf(in); got != want {
			t.Errorf("SlugOf(%q) = %q, want %q", in, got, want)
		}
	}
	now := time.Date(2026, 11, 14, 10, 15, 0, 0, time.Local)
	if got := SlugOf(FolderName("PGlite Golang!", now)); got != "pglite_golang" {
		t.Errorf("round trip %q", got)
	}
}

// TestPlanCleanReasons covers every reason in check order.
func TestPlanCleanReasons(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)
	old := now.Add(-48 * time.Hour)
	in := []Folder{
		{Name: "files", Files: 1, Created: old},
		{Name: "unknown", HistoryUnknown: true, Created: old},
		{Name: "history", History: []string{"claude"}, Created: old},
		{Name: "new", Created: now.Add(-time.Hour)},
		{Name: "hand", Modified: old},
		{Name: "gone", Created: old},
	}
	want := []Reason{ReasonHasFiles, ReasonHistoryUnknown, ReasonHasHistory, ReasonTooNew, ReasonRemove, ReasonRemove}
	for i, it := range PlanClean(in, now, 24*time.Hour) {
		if it.Reason != want[i] || it.Remove != (want[i] == ReasonRemove) {
			t.Errorf("%s: %s remove=%v, want %s", it.Folder.Name, it.Reason, it.Remove, want[i])
		}
	}
	if len(ReasonValues()) != len(want)-1 {
		t.Errorf("ReasonValues out of date")
	}
}

// TestRemoveEmptyRefusesContent: a file appearing after the plan stops
// the removal, and nothing is deleted.
func TestRemoveEmptyRefusesContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), FolderMode); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "sub", "work.txt")
	if err := os.WriteFile(work, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmpty(dir); err == nil {
		t.Fatal("removed a folder with content")
	}
	if _, err := os.Stat(work); err != nil {
		t.Fatal("work file deleted")
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmpty(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("empty folder kept")
	}
}

// TestPromoteRefusesExistingAndCreatesParents.
func TestPromoteRefusesExistingAndCreatesParents(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "s")
	if err := os.Mkdir(src, FolderMode); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(root, "taken")
	if err := os.Mkdir(taken, FolderMode); err != nil {
		t.Fatal(err)
	}
	if err := Promote(src, taken); !errors.Is(err, ErrExists) {
		t.Fatalf("err %v, want ErrExists", err)
	}
	dst := filepath.Join(root, "a", "b", "proj")
	if err := Promote(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}
	if err := Unpromote(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("undo did not move the folder back")
	}
}

// TestDiscardBackupOnlyTouchesBackups removes agentio backup copies and
// their emptied folders, and refuses a path outside a backup folder.
func TestDiscardBackupOnlyTouchesBackups(t *testing.T) {
	root := t.TempDir()
	bak := filepath.Join(root, ".claude"+backupMarker+"20261117T120000Z")
	cp := filepath.Join(bak, "projects", "-p", "a.jsonl")
	if err := os.MkdirAll(filepath.Dir(cp), FolderMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cp, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := DiscardBackup(agentio.Backup{Entries: []agentio.BackupEntry{{Original: "o", Copy: cp}, {Original: "o", Copy: outside}}})
	if err == nil {
		t.Fatal("a copy outside a backup folder was accepted")
	}
	if _, err := os.Stat(bak); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup folder kept")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("a file outside the backup was deleted")
	}
}

// TestListOrderAndHistory: newest stamp first, history from the callback.
func TestListOrderAndHistory(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a_20200101_101500", "b_20210101_101500", "plain"} {
		if err := os.Mkdir(filepath.Join(root, n), FolderMode); err != nil {
			t.Fatal(err)
		}
	}
	got, err := List(root, func(dir string) ([]string, bool) {
		if filepath.Base(dir) == "plain" {
			return nil, true
		}
		return []string{"claude"}, false
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "plain" || got[1].Name != "b_20210101_101500" || !got[0].HistoryUnknown || got[0].History == nil || got[1].History[0] != "claude" {
		t.Fatalf("list %+v", got)
	}
	missing, err := List(filepath.Join(root, "none"), nil)
	if err != nil || missing != nil {
		t.Fatalf("missing root: %v %v", missing, err)
	}
}
