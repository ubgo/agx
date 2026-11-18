//go:build agxtestclock

package cli

import (
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// envTestNow fixes the clock of a test build: an RFC 3339 time. Only the
// binary internal/testkit builds (with -tags agxtestclock) reads it; a
// release build has no such variable.
const envTestNow = "AGX_TEST_NOW"

func init() {
	pinClock = func(h *agentio.Host, lookup func(string) (string, bool)) {
		v, ok := lookup(envTestNow)
		if !ok || v == "" {
			return
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return
		}
		h.Now = func() time.Time { return t }
		// The variable is the test harness's, not the user's: a launched
		// agent never sees it, so examples print the same environment.
		h.Env = slices.DeleteFunc(slices.Clone(h.Env), func(kv string) bool { return strings.HasPrefix(kv, envTestNow+"=") })
	}
}
