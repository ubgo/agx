package login

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// tokensKey is the object in tokens.jsonc that holds values by login name.
const tokensKey = "tokens"

// goosWindows has no POSIX modes; the tokens.jsonc mode check is skipped
// there (spec 17).
const goosWindows = "windows"

// ReadStored returns the pasted value of login name from tokens.jsonc. It
// refuses a file readable by group or others on POSIX (spec 15.1: 0600,
// checked). found is false when the file or the entry is missing. Values
// are never expanded.
func ReadStored(paths config.Paths, name string) (value string, found bool, err error) {
	st, err := os.Stat(paths.Tokens)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read "+paths.Tokens+": "+err.Error())
	}
	if err := CheckMode(paths.Tokens, st.Mode()); err != nil {
		return "", false, err
	}
	d, err := jsonc.Open(paths.Tokens)
	if err != nil {
		return "", false, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, paths.Tokens+": "+err.Error())
	}
	p := config.Pointer(tokensKey, name)
	if !d.Has(p) {
		return "", false, nil
	}
	v, err := d.GetString(p)
	if err != nil {
		return "", false, apperr.Wrap(err, apperr.CodeConfig, apperr.ClassFailure, fmt.Sprintf("tokens.jsonc: %s is not a string", name))
	}
	return v, true, nil
}

// CheckMode refuses a tokens file that group or others can read (POSIX).
func CheckMode(path string, mode os.FileMode) error {
	if runtime.GOOS == goosWindows {
		return nil
	}
	if mode.Perm()&^config.TokensFileMode != 0 {
		return apperr.Failure(apperr.CodeSecret, fmt.Sprintf("%s has mode %04o; it holds tokens and must be 0600", path, mode.Perm())).
			WithHint("chmod 600 " + path)
	}
	return nil
}

// WriteStored sets login name's value in tokens.jsonc (created 0600),
// under the cross-process lock.
func WriteStored(ctx context.Context, paths config.Paths, name, value string) error {
	return config.Edit(ctx, paths.Tokens, config.TokensFileMode, func(d *jsonc.Doc) error {
		return d.Set(config.Pointer(tokensKey, name), value)
	})
}

// DeleteStored removes login name from tokens.jsonc; a missing entry is
// not an error.
func DeleteStored(ctx context.Context, paths config.Paths, name string) error {
	if _, err := os.Stat(paths.Tokens); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return config.Edit(ctx, paths.Tokens, config.TokensFileMode, func(d *jsonc.Doc) error {
		p := config.Pointer(tokensKey, name)
		if !d.Has(p) {
			return nil
		}
		return d.Delete(p)
	})
}

// KindOf maps a config login kind to the agentio token kind.
func KindOf(kind string) (agentio.TokenKind, bool) {
	k, ok := agentio.ParseTokenKind(kind)
	return k, ok && k != agentio.TokenKindUnknown
}

// Credential resolves login name to the credential a launch passes:
// a reference or plain token (expanded strictly, then cleaned), a stored
// value, or an api-key-helper command (never run by agx; the agent runs
// it). This is cost X when the reference runs a command or a plugin.
func Credential(ctx context.Context, e *Expander, paths config.Paths, name string, l config.Login) (agentio.TokenCredential, error) {
	label := "login " + name
	kind, ok := KindOf(l.Kind)
	if !ok {
		return agentio.TokenCredential{}, apperr.Failure(apperr.CodeConfig, fmt.Sprintf("%s: unknown kind %q", label, l.Kind))
	}
	var raw string
	switch {
	case kind == agentio.TokenAPIKeyHelper:
		raw = l.Command
	case l.Stored:
		v, found, err := ReadStored(paths, name)
		if err != nil {
			return agentio.TokenCredential{}, err
		}
		if !found {
			return agentio.TokenCredential{}, apperr.Failure(apperr.CodeSecret, label+": no stored value in tokens.jsonc").
				WithHint("agx login edit --id " + name + " --stdin")
		}
		raw = v
	default:
		v, err := e.Expand(ctx, label, l.Token, true)
		if err != nil {
			return agentio.TokenCredential{}, err
		}
		raw = v
	}
	tok, err := Clean(label, raw)
	if err != nil {
		return agentio.TokenCredential{}, err
	}
	return agentio.TokenCredential{Kind: kind, Token: agentio.NewToken(tok), Provider: l.Provider}, nil
}

// OneOff resolves a --token-ref (a reference or a plain value) of kind.
func OneOff(ctx context.Context, e *Expander, ref, kind, provider string) (agentio.TokenCredential, error) {
	k, ok := KindOf(kind)
	if !ok {
		return agentio.TokenCredential{}, apperr.Usage(fmt.Sprintf("--token-kind %q: use one of %v", kind, config.KindValues()))
	}
	raw := ref
	if k != agentio.TokenAPIKeyHelper {
		v, err := e.Expand(ctx, "--token-ref", ref, true)
		if err != nil {
			return agentio.TokenCredential{}, err
		}
		raw = v
	}
	tok, err := Clean("--token-ref", raw)
	if err != nil {
		return agentio.TokenCredential{}, err
	}
	return agentio.TokenCredential{Kind: k, Token: agentio.NewToken(tok), Provider: provider}, nil
}
