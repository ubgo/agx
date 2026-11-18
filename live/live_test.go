package live

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// TestReadMissingIsNotLive: a run folder without live.jsonc.
func TestReadMissingIsNotLive(t *testing.T) {
	if _, err := Read(t.TempDir()); !errors.Is(err, ErrNotLive) {
		t.Fatalf("err %v", err)
	}
}

// TestUpdateSerializesWriters: concurrent updates (agx live use and the
// agent's agx credential) never lose each other's change, and the file
// stays 0600.
func TestUpdateSerializesWriters(t *testing.T) {
	dir := t.TempDir()
	if err := Create(dir, State{RunID: "r", Agent: "claude", StartLogin: "carol"}); err != nil {
		t.Fatal(err)
	}
	const writers = 20
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Update(context.Background(), dir, func(s *State) error {
				s.Helper += fmt.Sprintf("%d,", i)
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := range writers {
		if !slices.Contains(strings.Split(s.Helper, ","), fmt.Sprint(i)) {
			t.Fatalf("lost update %d: %q", i, s.Helper)
		}
	}
	st, err := os.Stat(Path(dir))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", st.Mode(), err)
	}
}

// TestUpdateRefusalWritesNothing: fn's error is returned and the file is
// unchanged.
func TestUpdateRefusalWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := Create(dir, State{StartLogin: "carol"}); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("no")
	if _, err := Update(context.Background(), dir, func(s *State) error { s.ChosenLogin = "x"; return stop }); !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if s, _ := Read(dir); s.Want() != "carol" {
		t.Fatalf("%+v", s)
	}
}
