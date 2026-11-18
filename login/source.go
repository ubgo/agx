package login

import (
	"regexp"
	"strings"

	"github.com/ubgo/textdata/expand"

	"github.com/ubgo/agx/config"
)

// SourceType is where a login's value comes from, as list and show print
// it. It never carries the value itself (principle 12): a reference names
// its scheme, a command its program, an env reference its variable.
type SourceType string

// Source types. A login has exactly one (the config cross-check enforces
// one of token, stored or command).
const (
	// SourceReference is a ${scheme:…} plugin reference (gopass, vault…).
	SourceReference SourceType = "reference"
	// SourceCommand is a $(…) command reference; agx runs it (cost X).
	SourceCommand SourceType = "command"
	// SourceEnv is a ${VAR} / $VAR reference to agx's environment.
	SourceEnv SourceType = "env"
	// SourcePlain is a literal token in config.jsonc (allowed, principle 3;
	// edge 23).
	SourcePlain SourceType = "plain"
	// SourceStored is a pasted value in tokens.jsonc.
	SourceStored SourceType = "stored"
	// SourceHelper is an api-key-helper command the agent runs itself; the
	// key never passes through agx.
	SourceHelper SourceType = "helper"
)

// SourceTypeValues is the canonical list.
func SourceTypeValues() []SourceType {
	return []SourceType{SourceReference, SourceCommand, SourceEnv, SourcePlain, SourceStored, SourceHelper}
}

// Source describes a login's value without revealing it.
type Source struct {
	// Type is the kind of source.
	Type SourceType `json:"type"`
	// Detail names the scheme, program or variable ("gopass", "op",
	// "TOKVAR"); for stored, whether tokens.jsonc holds the value. Never a
	// secret: references name where a secret lives, not the secret.
	Detail string `json:"detail,omitempty"`
}

// String renders "reference gopass", "plain", "stored (tokens.jsonc)".
func (s Source) String() string {
	if s.Detail == "" {
		return string(s.Type)
	}
	if s.Type == SourceStored {
		return string(s.Type) + " (" + s.Detail + ")"
	}
	return string(s.Type) + " " + s.Detail
}

// StoredState is what reading tokens.jsonc found for a stored login.
type StoredState int

// Stored states. StoredUnreadable is a tokens.jsonc that exists but was
// refused (a mode wider than 0600, bad JSONC, an I/O error): calling that
// "missing" sent people to paste a token that was already there.
const (
	StoredMissing StoredState = iota
	StoredPresent
	StoredUnreadable
)

// StoredStateOf maps ReadStored's (found, err) to a StoredState.
func StoredStateOf(found bool, err error) StoredState {
	switch {
	case err != nil:
		return StoredUnreadable
	case found:
		return StoredPresent
	}
	return StoredMissing
}

// Stored-value details for Source.Detail, per StoredState.
const (
	storedPresent    = "tokens.jsonc"
	storedMissing    = "missing from tokens.jsonc"
	storedUnreadable = "tokens.jsonc unreadable; agx login check shows why"
)

// Describe classifies login l without resolving anything (cost R).
// stored is what tokens.jsonc holds for a stored login (callers read it
// with ReadStored and StoredStateOf; ignored for other sources).
func Describe(l config.Login, stored StoredState) Source {
	switch {
	case l.Kind == config.KindAPIKeyHelper:
		return Source{Type: SourceHelper, Detail: program(l.Command)}
	case l.Stored:
		switch stored {
		case StoredPresent:
			return Source{Type: SourceStored, Detail: storedPresent}
		case StoredUnreadable:
			return Source{Type: SourceStored, Detail: storedUnreadable}
		}
		return Source{Type: SourceStored, Detail: storedMissing}
	}
	return DescribeRef(l.Token)
}

// DescribeRef classifies a token string: plain, or its first reference.
// A value mixing text and references is described by its first
// reference, which is what decides whether resolving runs anything.
func DescribeRef(tok string) Source {
	i := strings.IndexByte(tok, '$')
	if i < 0 || i == len(tok)-1 {
		return Source{Type: SourcePlain}
	}
	rest := tok[i+1:]
	switch rest[0] {
	case '(':
		if end := expand.MatchParen(rest); end > 0 {
			return Source{Type: SourceCommand, Detail: program(rest[1:end])}
		}
	case '{':
		if end := expand.MatchBrace(rest); end > 0 {
			inner := rest[1:end]
			if scheme, _, ok := expand.SplitScheme(inner); ok {
				return Source{Type: SourceReference, Detail: scheme}
			}
			return Source{Type: SourceEnv, Detail: expand.ParseRef(inner).Name}
		}
	default:
		if expand.IsNameStart(rest[0]) {
			n := 1
			for n < len(rest) && expand.IsNameChar(rest[n]) {
				n++
			}
			return Source{Type: SourceEnv, Detail: rest[:n]}
		}
	}
	return Source{Type: SourcePlain}
}

// program is the first word of a command line: enough to say what runs
// (gopass, op) without echoing arguments, which may name secret paths.
func program(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// namePattern is a valid login name: it is a config key, an AGX_LOGIN value
// and a word in the printed `export AGX_LOGIN=…` line, so it must need no
// shell quoting.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName reports a usable login name.
func ValidName(name string) bool { return namePattern.MatchString(name) }
