package login_test

import (
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/login"
)

// Made-up values that only start with the public prefixes; assembled so
// no scanner reads a key shape in the source.
const (
	fakeOAT = "sk-" + "ant-" + "oat01-FAKEVALUE"
	fakeAPI = "sk-" + "ant-" + "api03-FAKEVALUE"
)

// TestDetectPrefixes: the three kinds, with surrounding space, and the
// near misses that must stay unknown.
func TestDetectPrefixes(t *testing.T) {
	for in, want := range map[string]login.Detected{
		fakeOAT:               login.DetectedOAuthToken,
		"  " + fakeAPI + "\n": login.DetectedAPIKey,
		"fake-token-OTHER":    login.DetectedUnknown,
		"":                    login.DetectedUnknown,
		"sk-" + "ant-":        login.DetectedUnknown,
		"xsk-" + "ant-oat01":  login.DetectedUnknown,
	} {
		if got := login.Detect(in); got != want {
			t.Errorf("Detect(%d chars) = %s, want %s", len(in), got, want)
		}
	}
	if got := login.DetectToken(agentio.NewToken(fakeAPI)); got != login.DetectedAPIKey {
		t.Errorf("DetectToken = %s", got)
	}
	if len(login.DetectedValues()) != 3 {
		t.Errorf("DetectedValues = %v", login.DetectedValues())
	}
}

// TestMismatchAndRefusal: only oauth-token holding an API key is refused,
// unless the run or the login allows it; the error names the login and
// the kind, carries the per-use-key code and no token bytes.
func TestMismatchAndRefusal(t *testing.T) {
	oauth := config.Login{Kind: config.KindOAuthToken}
	key := config.Login{Kind: config.KindAPIKey}
	if !login.Mismatch(oauth.Kind, login.DetectedAPIKey) || !login.Mismatch(key.Kind, login.DetectedOAuthToken) {
		t.Error("contradictions not reported")
	}
	if login.Mismatch(oauth.Kind, login.DetectedUnknown) || login.Mismatch(config.KindAPIKeyHelper, login.DetectedAPIKey) || login.Mismatch(config.KindPool, login.DetectedAPIKey) {
		t.Error("unknown, helper or pool reported as a mismatch")
	}
	if login.PerUseRefused(key, login.DetectedAPIKey, false) || login.PerUseRefused(oauth, login.DetectedOAuthToken, false) {
		t.Error("a matching kind refused")
	}
	if login.PerUseRefused(oauth, login.DetectedAPIKey, true) || login.PerUseRefused(config.Login{Kind: config.KindOAuthToken, AllowAPIKey: true}, login.DetectedAPIKey, false) {
		t.Error("the overrides do not allow it")
	}
	err := login.CheckPerUse("work", oauth, login.DetectedAPIKey, false, true)
	e := apperr.As(err)
	if err == nil || e.Code != apperr.CodePerUseKey || e.Message != "login work holds an API key (billed per use), but its kind is oauth-token" ||
		!strings.Contains(e.Hint, "--allow-api-key") || strings.Contains(e.Message+e.Hint, "sk-") {
		t.Fatalf("refusal = %+v", e)
	}
	if h := apperr.As(login.CheckPerUse("m", oauth, login.DetectedAPIKey, false, false)).Hint; strings.Contains(h, "--allow-api-key") {
		t.Errorf("a pool member's hint offers the run flag: %q", h)
	}
	if login.CheckPerUse("work", oauth, login.DetectedAPIKey, true, true) != nil {
		t.Error("allowed run refused")
	}
}
