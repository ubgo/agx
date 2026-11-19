package config_test

import (
	"strings"
	"testing"

	"github.com/ubgo/agx/config"
)

// TestPortConfigValidation is the stand-alone selector's describe("config
// validation"), on agx's pool block: each case refused at load with its
// position. Two deliberate differences (pool/testdata/SELECTOR_PORT.md):
// an accounts entry for an unlisted name is an error, not a warning, and
// own/shared may each be omitted as long as the pool has a member.
func TestPortConfigValidation(t *testing.T) {
	pool := func(body string) string {
		return `"auto": { "agents": ["claude"], "kind": "pool", "pool": { ` + body + ` } }`
	}
	cases := []struct {
		name, pool string
		want       []string
	}{
		{"missing own/shared arrays", pool(`"force": null`), []string{"the pool has no members"}},
		{"duplicate names across own and shared", pool(`"own": ["alice"], "shared": ["alice"]`), []string{"listed more than once"}},
		{"old setting names are rejected so nothing is silently ignored", pool(`"own": ["alice"], "burnToFullest": [], "priority": [], "accountLimits": {}, "limits": {}, "sharedBorrowWindowHours": 48`),
			[]string{"burnToFullest", "priority", "accountLimits", "limits", "sharedBorrowWindowHours"}},
		{"force must be a string or null", pool(`"own": ["alice"], "force": true`), []string{"force"}},
		{"bad borrow window", pool(`"own": ["alice"], "shared_borrow_window": "-1h"`), []string{`shared_borrow_window "-1h" is negative`}},
		{"bad borrow window (a number)", pool(`"own": ["alice"], "shared_borrow_window": 48`), []string{"shared_borrow_window"}},
		{"accounts entry for an unlisted name is an error", pool(`"own": ["alice"], "accounts": { "zed": {} }`), []string{"accounts.zed is not listed in own or shared"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePool(t, tc.pool)
			if err == nil {
				t.Fatal("accepted")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) && !strings.Contains(strings.Join(problemMessages(err), "\n"), w) {
					t.Errorf("err %v\nmissing %q", err, w)
				}
			}
		})
	}
}

// problemMessages lists every problem of a config error.
func problemMessages(err error) []string {
	var out []string
	for _, p := range config.ProblemsOf(err) {
		out = append(out, p.String())
	}
	return out
}
