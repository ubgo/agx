package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ubgo/agx/internal/apperr"
)

// TestUserText: library prefixes go at the start, after ": " and after
// "(", repeatedly; agent names and agx's own labels stay.
func TestUserText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"agentio: login expired at 2026-11-17T09:00:00Z: run claude once", "login expired at 2026-11-17T09:00:00Z: run claude once"},
		{"--option sandbox: agentio: invalid launch option", "--option sandbox: invalid launch option"},
		{"login carol: textdata: jsonc: bad value", "login carol: bad value"},
		{"claude ~/.claude (agentio: not found: conversation)", "claude ~/.claude (not found: conversation)"},
		{"codex: autonomy accept-edits: Codex has no edits-only mode", "codex: autonomy accept-edits: Codex has no edits-only mode"},
		{"skipped claude ~/.claude: already a profile", "skipped claude ~/.claude: already a profile"},
		{"myagentio: x", "myagentio: x"},
	} {
		if got := apperr.UserText(c.in); got != c.want {
			t.Errorf("UserText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if apperr.Text(nil) != "" {
		t.Error("Text(nil)")
	}
	err := fmt.Errorf("%w at 2026-11-17T09:00:00Z", errors.New("agentio: login expired"))
	if got := apperr.As(err).Message; got != "login expired at 2026-11-17T09:00:00Z" {
		t.Errorf("As: %q", got)
	}
	if got := apperr.Failure(apperr.CodeIO, "agentio: not found").WithHint("agentio: try x"); got.Message != "not found" || got.Hint != "try x" {
		t.Errorf("New/WithHint: %q %q", got.Message, got.Hint)
	}
}
