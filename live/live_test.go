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
	"time"

	"github.com/ubgo/agx/config"
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

// TestStateRefusal: a refused key request records why and since when;
// under stop nothing is in use, under keep the agent's login stays; a
// second refusal keeps the first one's time; a key ends the refusal and
// a switch of login moves InUseSince.
func TestStateRefusal(t *testing.T) {
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	var s State
	s.SetInUse("alice", t0)
	if s.InUse != "alice" || !s.InUseSince.Equal(t0) || !s.KeyAt.Equal(t0) || !s.InUseAt.Equal(t0) {
		t.Fatalf("first key %+v", s)
	}
	t1 := t0.Add(time.Minute)
	s.SetInUse("alice", t1)
	if !s.InUseSince.Equal(t0) || !s.KeyAt.Equal(t1) {
		t.Errorf("same login keeps its since: %+v", s)
	}
	t2 := t1.Add(time.Minute)
	s.SetNoKey("pool auto: no member is usable: …", "all over caps", config.WhenNoneUsableStop, t2)
	if s.InUse != "" || !s.InUseSince.Equal(t2) || !s.NoKeySince.Equal(t2) || !s.KeyAt.Equal(t1) || s.WhenNoneUsable != "stop" || s.NoKeyBrief != "all over caps" {
		t.Errorf("stop %+v", s)
	}
	t3 := t2.Add(time.Minute)
	s.SetNoKey("again", "all over caps", config.WhenNoneUsableStop, t3)
	if !s.NoKeySince.Equal(t2) || !s.InUseAt.Equal(t3) || !s.InUseSince.Equal(t2) || s.NoKey != "again" {
		t.Errorf("second refusal %+v", s)
	}
	t4 := t3.Add(time.Minute)
	s.SetInUse("bob", t4)
	if s.InUse != "bob" || s.NoKey != "" || s.NoKeyBrief != "" || !s.NoKeySince.IsZero() || s.WhenNoneUsable != "" || !s.InUseSince.Equal(t4) || !s.KeyAt.Equal(t4) {
		t.Errorf("recovered %+v", s)
	}
	s.SetNoKey("bob over cap", "bob over cap", config.WhenNoneUsableKeep, t4.Add(time.Minute))
	if s.InUse != "bob" || !s.InUseSince.Equal(t4) || s.WhenNoneUsable != "keep" {
		t.Errorf("keep %+v", s)
	}
}
