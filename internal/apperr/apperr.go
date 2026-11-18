// Package apperr is agx's one error type: a stable machine code, a
// human message, an optional hint and the exit class that decides the
// process exit code (spec section 14.2).
//
// Why one type: every command prints the same JSON envelope, whose
// "error" object is {code, message, hint}; the exit code differs between
// launching commands (run, new, resume, launch: 125 for anything agx
// refused before the agent started) and every other command (1, 2, 3 or
// 4). Carrying the class instead of a number lets one error value be
// raised from shared code (resolution, config, secrets) and still get the
// right number in either kind of command.
//
// Invariant: a message never holds a secret value. Callers pass names,
// paths and reasons only; textdata's strict expansion errors already
// carry no value.
package apperr

import "errors"

// Code is the stable machine name of a failure (error.code in --json).
// Open set for readers: new codes may appear; a script should branch on
// the ones it knows.
type Code string

// Codes. Every refusal agx makes has exactly one; tests assert on them.
const (
	// CodeUsage is a malformed command line: a bare word, a bad value, a
	// missing required flag.
	CodeUsage Code = "usage"
	// CodeConflict is two choices for one concern in one command
	// ("--login carol --home-login").
	CodeConflict Code = "conflict"
	// CodeAmbiguous is a choice agx cannot make alone (two matching
	// conversations, a shared home without a default).
	CodeAmbiguous Code = "ambiguous"
	// CodeNoTTY is a picker or question without a terminal.
	CodeNoTTY Code = "no-tty"
	// CodeNotFound is a named thing that does not exist.
	CodeNotFound Code = "not-found"
	// CodeUnsupported is a capability the agent does not have (agentio's
	// typed unsupported reason).
	CodeUnsupported Code = "unsupported"
	// CodeConfig is an invalid or unreadable config file.
	CodeConfig Code = "config"
	// CodeConfigVersion is a config written by a newer agx.
	CodeConfigVersion Code = "config-version"
	// CodeLocked is a config file held by another agx process.
	CodeLocked Code = "locked"
	// CodeSecret is a reference or token that did not resolve.
	CodeSecret Code = "secret"
	// CodeNoProfiles is a launch with no profile configured at all.
	CodeNoProfiles Code = "no-profiles"
	// CodeNoAgent is a launch where nothing names an agent.
	CodeNoAgent Code = "no-agent"
	// CodeHomeMissing is a profile whose home folder is gone.
	CodeHomeMissing Code = "home-missing"
	// CodeBinaryMissing is an agent binary that is not on PATH.
	CodeBinaryMissing Code = "binary-missing"
	// CodeBinaryNotExecutable is a binary that was found but cannot run.
	CodeBinaryNotExecutable Code = "binary-not-executable"
	// CodeFeatureUnavailable is an option the installed version lacks.
	CodeFeatureUnavailable Code = "feature-unavailable"
	// CodeWrongAgent is a login that does not serve the run's agent.
	CodeWrongAgent Code = "wrong-agent"
	// CodeAutoPick is --auto-profile or --auto-login with no eligible
	// candidate.
	CodeAutoPick Code = "auto-pick"
	// CodeBeforeRunFailed is a before_run script that failed or is missing.
	CodeBeforeRunFailed Code = "before-run-failed"
	// CodeFolderGone is a conversation whose folder no longer exists.
	CodeFolderGone Code = "folder-gone"
	// CodePlanFile is a stale, foreign or unreadable shell plan file.
	CodePlanFile Code = "plan-file"
	// CodeFieldPath is a --field path missing from the envelope.
	CodeFieldPath Code = "field-path"
	// CodeInterrupted is Ctrl-C before anything started.
	CodeInterrupted Code = "interrupted"
	// CodeDismissed is a picker closed with Esc.
	CodeDismissed Code = "dismissed"
	// CodePartial is a listing where some homes answered and some failed.
	CodePartial Code = "partial"
	// CodeNetwork is a failed network call.
	CodeNetwork Code = "network"
	// CodeChecksFailed is a doctor run where at least one check failed
	// (the report is still printed; exit 1).
	CodeChecksFailed Code = "checks-failed"
	// CodeInUse is a remove refused because other config entries still
	// name the object (login remove without --cascade); exit 2.
	CodeInUse Code = "in-use"
	// CodeMoveBlocked is a move or promote stopped before touching a file:
	// a running session in the folder, an id already in the target, or
	// history already keyed to the destination.
	CodeMoveBlocked Code = "move-blocked"
	// CodeExists is a destination that already exists (session promote).
	CodeExists Code = "exists"
	// CodeIO is any other failure (disk, process).
	CodeIO Code = "io"
)

// Class decides the exit code; see Exit. It is separate from Code because
// several codes share a class (every usage-like refusal exits 2).
type Class uint8

// Classes, from spec 14.2.
const (
	// ClassFailure is a plain failure: exit 1 (125 when launching).
	ClassFailure Class = iota
	// ClassUsage is a usage error or ambiguity: exit 2 (125 when launching).
	ClassUsage
	// ClassPartial is "some answered, some failed": exit 3.
	ClassPartial
	// ClassUnsupported is "this agent cannot": exit 4 (125 when launching).
	ClassUnsupported
	// ClassNotFoundBinary is a missing agent binary: 127 when launching.
	ClassNotFoundBinary
	// ClassNotExecutable is a binary that cannot run: 126 when launching.
	ClassNotExecutable
	// ClassInterrupted is Ctrl-C: exit 130 everywhere.
	ClassInterrupted
	// ClassDismissed is a picker closed with Esc: exit 1 everywhere.
	ClassDismissed
)

// Exit codes (spec 14.2), named so no number is written twice.
const (
	ExitOK            = 0
	ExitFailure       = 1
	ExitUsage         = 2
	ExitPartial       = 3
	ExitUnsupported   = 4
	ExitLaunchFailed  = 125
	ExitNotExecutable = 126
	ExitNotFound      = 127
	ExitInterrupted   = 130
)

// Error is one agx failure.
type Error struct {
	// Code is the stable machine name.
	Code Code
	// Class picks the exit code.
	Class Class
	// Message is the human sentence, without the "agx: " prefix.
	Message string
	// Hint is an optional next step ("agx config init"), may be multi-line.
	Hint string
	// Err is the cause, kept for errors.Is; never printed separately.
	Err error
}

// Error renders the message (the hint is printed separately by the CLI).
func (e *Error) Error() string { return e.Message }

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Err }

// New builds an Error. Callers format the message themselves
// (fmt.Sprintf): the package takes a finished sentence so no untyped
// variadic crosses its API.
//
// Library prefixes ("agentio: ") are stripped from the message (UserText).
func New(code Code, class Class, msg string) *Error {
	return &Error{Code: code, Class: class, Message: UserText(msg)}
}

// Wrap builds an Error around a cause; the message is msg only (the cause
// is reachable through errors.Is but never appended, so a library error
// that might quote a value is not echoed).
func Wrap(err error, code Code, class Class, msg string) *Error {
	e := New(code, class, msg)
	e.Err = err
	return e
}

// WithHint returns e with its hint set.
func (e *Error) WithHint(hint string) *Error {
	e.Hint = UserText(hint)
	return e
}

// Usage is a usage error (exit 2, or 125 when launching).
func Usage(msg string) *Error { return New(CodeUsage, ClassUsage, msg) }

// Conflict is two choices for one concern (spec 7.1 rule 3).
func Conflict(msg string) *Error { return New(CodeConflict, ClassUsage, msg) }

// Unsupported is a capability the agent lacks.
func Unsupported(msg string) *Error { return New(CodeUnsupported, ClassUnsupported, msg) }

// Failure is a plain failure with a code.
func Failure(code Code, msg string) *Error { return New(code, ClassFailure, msg) }

// As returns the *Error inside err, or a generic failure wrapping it.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeIO, Class: ClassFailure, Message: Text(err), Err: err}
}

// Exit maps err to the process exit code. launching is true for run, new,
// resume and launch, where every refusal before the agent starts is 125
// (spec 14.2), except a missing binary (127), a binary that cannot run
// (126) and Ctrl-C (130).
func Exit(err error, launching bool) int {
	if err == nil {
		return ExitOK
	}
	e := As(err)
	switch e.Class {
	case ClassInterrupted:
		return ExitInterrupted
	case ClassDismissed:
		return ExitFailure
	case ClassNotFoundBinary:
		if launching {
			return ExitNotFound
		}
		return ExitFailure
	case ClassNotExecutable:
		if launching {
			return ExitNotExecutable
		}
		return ExitFailure
	}
	if launching {
		return ExitLaunchFailed
	}
	switch e.Class {
	case ClassUsage:
		return ExitUsage
	case ClassPartial:
		return ExitPartial
	case ClassUnsupported:
		return ExitUnsupported
	}
	return ExitFailure
}
