// Package live owns live.jsonc, the live-login state of one run (spec
// section 10): the login a run started with, the login the user chose
// since (agx live use) and the login the agent last fetched (agx
// credential). It never holds a token: the hidden credential command
// resolves the named login each time the agent asks.
//
// Why a package of its own: two processes write the file while the agent
// runs (agx live use from the user's terminal, agx credential from the
// agent's helper every refresh), so every change is a locked
// read-modify-write with an atomic replace. Only this package (with
// config, login, sessions, internal/run and internal/plan) writes files
// (internal/archtest).
package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/textdata/atomicfile"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/run"
)

// ErrNotLive is returned by Read for a run folder without live.jsonc: the
// run was not started with --live-login.
var ErrNotLive = errors.New("live: run is not a live-login run")

// State is live.jsonc. Login fields are login names from config.jsonc,
// never values.
type State struct {
	// RunID is the run folder id (also AGX_RUN_ID in the agent's env).
	RunID string `json:"run_id"`
	// Agent is the run's agent id; a chosen login must serve it.
	Agent string `json:"agent"`
	// StartLogin is the login the run started with (agx live use
	// --start-login goes back to it).
	StartLogin string `json:"start_login"`
	// ChosenLogin is the login picked by agx live use; "" = the start
	// login.
	ChosenLogin string `json:"chosen_login,omitempty"`
	// ChosenAt is when ChosenLogin was last set (zero = never).
	ChosenAt time.Time `json:"chosen_at,omitzero"`
	// InUse is the login whose key the agent holds now: the login agx
	// credential last handed out. "" = the agent has not asked yet, or
	// its last request was refused under when_none_usable "stop" (it was
	// handed no key; NoKey says why). Under "keep" a refusal leaves InUse
	// as it was, since the agent goes on with that key.
	InUse string `json:"in_use,omitempty"`
	// InUseAt is when the agent last asked (zero = never).
	InUseAt time.Time `json:"in_use_at,omitzero"`
	// InUseSince is when InUse last changed to its current value (a
	// switch of member, the first key, or a refusal that cleared it);
	// zero = never.
	InUseSince time.Time `json:"in_use_since,omitzero"`
	// KeyAt is when agx credential last handed out a real key (zero =
	// never); a refused request does not change it.
	KeyAt time.Time `json:"key_at,omitzero"`
	// Helper is the command the agent runs to fetch its credential.
	Helper string `json:"helper"`
	// Model is the model the run's session uses (--model, its profile's
	// model or the agent's own setting at launch); "" = unknown. A pool
	// probes its members with it so per-model windows show.
	Model string `json:"model,omitempty"`
	// PoolForce is the member a pool run's --pool-force named: every key
	// request gets it over the pool's rules. agx live use clears it.
	PoolForce string `json:"pool_force,omitempty"`
	// PoolOnly is the member a pool run is pinned to with its caps
	// enforced (--pool-only): every key request gets it while the pool's
	// rules allow, and no key otherwise. agx live use --login clears it.
	PoolOnly string `json:"pool_only,omitempty"`
	// PoolOnlyShort and PoolOnlyWeek are the pin's tighter caps for this
	// run (--pool-only-short, --pool-only-week); nil = the configured cap.
	PoolOnlyShort *float64 `json:"pool_only_short,omitempty"`
	PoolOnlyWeek  *float64 `json:"pool_only_week,omitempty"`
	// NoKey says why the last key request handed out no key, in one line
	// ("pool auto: no member is usable: …", "alice over cap: week is 70%
	// (cap 70%)"); "" once one did.
	NoKey string `json:"no_key,omitempty"`
	// NoKeyBrief is NoKey in a few words for a table or a status line
	// ("all over caps", "alice over cap", "api key refused").
	NoKeyBrief string `json:"no_key_brief,omitempty"`
	// NoKeySince is the first refused request of the current run of
	// refusals: since when the run has had no key (zero = it has one).
	NoKeySince time.Time `json:"no_key_since,omitzero"`
	// WhenNoneUsable is how the last refusal was handled: "stop" (the
	// agent was handed a placeholder, so it stops) or "keep" (the
	// request failed and the agent kept its old key); "" with no
	// refusal.
	WhenNoneUsable string `json:"when_none_usable,omitempty"`
	// AllowAPIKey is the run's --allow-api-key: a token login (not a pool
	// member) declared oauth-token whose token is an API key is handed out
	// anyway.
	AllowAPIKey bool `json:"allow_api_key,omitempty"`
	// InUseDetected is the kind agx credential detected in the key it
	// last handed out (oauth-token, api-key or unknown); "" = not asked
	// yet. Never any part of the token.
	InUseDetected string `json:"in_use_detected,omitempty"`
}

// Want is the login the next credential request gets: the chosen login,
// else the start login.
func (s State) Want() string {
	if s.ChosenLogin != "" {
		return s.ChosenLogin
	}
	return s.StartLogin
}

// SetInUse records a key handed out at now: login is in use, the
// refusal (if any) is over.
func (s *State) SetInUse(login string, now time.Time) {
	if s.InUse != login || s.InUseSince.IsZero() {
		s.InUseSince = now
	}
	s.InUse, s.InUseAt, s.KeyAt = login, now, now
	s.NoKey, s.NoKeyBrief, s.NoKeySince, s.WhenNoneUsable = "", "", time.Time{}, ""
}

// SetNoKey records a refused key request at now: why in one line, brief
// in a few words, and how it was handled (mode). Under "stop" the agent
// holds no usable key, so nothing is in use; under "keep" it goes on
// with the key it has, so InUse stays. NoKeySince keeps the first
// refusal of a run of them.
func (s *State) SetNoKey(why, brief, mode string, now time.Time) {
	if s.NoKeySince.IsZero() {
		s.NoKeySince = now
	}
	if mode != config.WhenNoneUsableKeep && s.InUse != "" {
		s.InUse, s.InUseSince = "", now
	}
	s.InUseAt, s.NoKey, s.NoKeyBrief, s.WhenNoneUsable = now, why, brief, mode
}

// Path is live.jsonc of a run folder.
func Path(runDir string) string { return filepath.Join(runDir, run.LiveFile) }

// Create writes the first live.jsonc of a new run (0600, atomic).
func Create(runDir string, s State) error {
	return write(runDir, s)
}

// Read returns the state of a run folder; ErrNotLive when the run has no
// live.jsonc.
func Read(runDir string) (State, error) {
	b, err := os.ReadFile(Path(runDir))
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, ErrNotLive
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, fmt.Errorf("%s: %w", run.LiveFile, err)
	}
	return s, nil
}

// Update changes the state under the file's cross-process lock and
// returns the state written. fn may refuse (its error is returned and
// nothing is written). Waiting for another writer is bounded by
// config.LockWait, the same bound as every config write (edge 44).
func Update(ctx context.Context, runDir string, fn func(s *State) error) (State, error) {
	lctx, cancel := context.WithTimeout(ctx, config.LockWait)
	defer cancel()
	lk, err := atomicfile.Lock(lctx, Path(runDir))
	switch {
	case errors.Is(err, atomicfile.ErrLockTimeout):
		return State{}, apperr.Wrap(err, apperr.CodeLocked, apperr.ClassFailure,
			fmt.Sprintf("%s is being written by another agx (waited %s)", run.LiveFile, config.LockWait))
	case err != nil:
		return State{}, err
	}
	defer func() { _ = lk.Close() }() // the OS drops the lock with the descriptor anyway
	s, err := Read(runDir)
	if err != nil {
		return State{}, err
	}
	if err := fn(&s); err != nil {
		return State{}, err
	}
	if err := write(runDir, s); err != nil {
		return State{}, err
	}
	return s, nil
}

// write replaces live.jsonc atomically (0600: it names logins, which is
// private even though no value is in it).
func write(runDir string, s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(Path(runDir), append(b, '\n'), atomicfile.Options{Mode: config.TokensFileMode})
}
