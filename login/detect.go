package login

import (
	"fmt"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// Detected is the kind a token really is, read from its public prefix.
// A login's kind in config is only a label: nothing checks it, so a
// pay-per-use API key filed under kind oauth-token would go unnoticed.
// Detection reads the first characters of the value and nothing else;
// only the kind word is ever shown, never any part of the token.
type Detected string

// Detected kinds.
const (
	// DetectedOAuthToken is a Claude setup-token (sk-ant-oat…): billed
	// to the account's plan.
	DetectedOAuthToken Detected = "oauth-token"
	// DetectedAPIKey is an Anthropic API key (sk-ant-api…): billed per
	// use, on the Console account that owns it.
	DetectedAPIKey Detected = "api-key"
	// DetectedUnknown is any other value (another vendor's key, a
	// gateway token): agx cannot tell how it is billed.
	DetectedUnknown Detected = "unknown"
)

// DetectedValues is the canonical list.
func DetectedValues() []Detected {
	return []Detected{DetectedOAuthToken, DetectedAPIKey, DetectedUnknown}
}

// The public prefixes Anthropic gives its two token kinds. They are the
// documented, non-secret start of the value; agx compares against them
// and never prints them.
const (
	prefixOAuthToken = "sk-ant-oat"
	prefixAPIKey     = "sk-ant-api"
)

// Detect reads the kind of a raw token value (cost: none).
func Detect(raw string) Detected {
	t := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(t, prefixOAuthToken):
		return DetectedOAuthToken
	case strings.HasPrefix(t, prefixAPIKey):
		return DetectedAPIKey
	}
	return DetectedUnknown
}

// DetectToken is Detect for a redacting token. The value is read only to
// compare its prefix and is not kept.
func DetectToken(t agentio.Token) Detected { return Detect(t.Reveal()) }

// Mismatch reports a detected kind that contradicts the login's declared
// kind: oauth-token holding an API key, or api-key holding a setup-token.
// An unknown detection, a helper and a pool never mismatch.
func Mismatch(declared string, d Detected) bool {
	switch declared {
	case config.KindOAuthToken:
		return d == DetectedAPIKey
	case config.KindAPIKey:
		return d == DetectedOAuthToken
	}
	return false
}

// PerUseRefused reports the one mismatch agx refuses to launch: a login
// declared oauth-token whose token is an API key (it would be billed per
// use while the config says plan), unless allow (--allow-api-key) or the
// login's allow_api_key is set.
func PerUseRefused(l config.Login, d Detected, allow bool) bool {
	return l.Kind == config.KindOAuthToken && d == DetectedAPIKey && !allow && !l.AllowAPIKey
}

// CheckPerUse is PerUseRefused as the refusal a launch and agx credential
// return: one line naming the login, with the ways to allow it as the
// hint. runFlag says whether --allow-api-key can apply (a run's own
// login); a pool member can only be allowed in config.
func CheckPerUse(name string, l config.Login, d Detected, allow, runFlag bool) error {
	if !PerUseRefused(l, d, allow) {
		return nil
	}
	hint := fmt.Sprintf("agx config set --key logins.%s.allow_api_key --value true", name)
	if runFlag {
		hint = "--allow-api-key for one run, or " + hint
	}
	return apperr.Failure(apperr.CodePerUseKey, PerUseMessage(name)).WithHint(hint)
}

// PerUseMessage is the refusal's text, also a pool member's skip reason.
func PerUseMessage(name string) string {
	return fmt.Sprintf("login %s holds an API key (billed per use), but its kind is %s", name, config.KindOAuthToken)
}
