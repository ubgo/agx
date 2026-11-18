package config_test

import (
	"slices"
	"testing"

	"github.com/ubgo/agx/config"
)

// TestKeyOrderFollowsFile: listings follow the order the file lists
// profiles and logins in (the usage table's # is a login's position),
// not the alphabetical order of the decoded map.
func TestKeyOrderFollowsFile(t *testing.T) {
	src := `{"version": 1,
  "profiles": {"zed": {"agent": "claude", "home": "~/.z"}, "alpha": {"agent": "claude", "home": "~/.a"}},
  "logins": {"nora": {"agents": ["claude"], "kind": "oauth-token", "token": "${A}"}, "miriam": {"agents": ["claude"], "kind": "oauth-token", "token": "${B}"}}}`
	l, err := config.Parse([]byte(src), config.Paths{File: "config.jsonc"}, known)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.ProfileOrder(); !slices.Equal(got, []string{"zed", "alpha"}) {
		t.Errorf("profiles %v", got)
	}
	if got := l.LoginOrder(); !slices.Equal(got, []string{"nora", "miriam"}) {
		t.Errorf("logins %v", got)
	}
	empty := &config.Loaded{}
	if got := empty.LoginOrder(); len(got) != 0 {
		t.Errorf("no file: %v", got)
	}
}
