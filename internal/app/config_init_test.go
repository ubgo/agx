package app

import (
	"context"
	"testing"

	"github.com/ubgo/agx/internal/apperr"
)

// TestConfigInitExistingNamesTildePath: config init on an existing file
// refuses (never overwrites) and names the file in the ~ form every other
// message uses, not the absolute path.
func TestConfigInitExistingNamesTildePath(t *testing.T) {
	a, _ := testApp(t, resumeSharedConfig, nil)
	_, err := a.ConfigInit(context.Background(), ConfigInitOptions{})
	const want = "~/.agx/config.jsonc already exists"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
	if got := apperr.Exit(err, false); got != apperr.ExitFailure {
		t.Fatalf("exit %d, want %d", got, apperr.ExitFailure)
	}
}
