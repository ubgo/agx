package login

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
)

// The login-usage cache (config.Paths.LoginUsage): the last probe of each
// login-pool member, shared by every agx process. A pool's key helper
// runs once a minute in every session, and a probe spends quota, so a
// probe younger than the pool's usage_max_age is reused instead. The file
// is rebuilt at any time; it holds account labels (emails), so it is
// owner-only (0600) in an owner-only folder, and it never holds a token.

// UsageCacheVersion is the cache file's format version; a file with
// another version is ignored and rewritten.
const UsageCacheVersion = 1

// UsageCache is the cache file.
type UsageCache struct {
	Version int `json:"version"`
	// Logins maps a login name to its last probe.
	Logins map[string]UsageEntry `json:"logins"`
}

// UsageEntry is one login's last probe: the windows when it answered,
// else the status and reason of the failure.
type UsageEntry struct {
	// Agent is the agent whose prober read it.
	Agent string `json:"agent"`
	// Account is the login's display label at probe time.
	Account string `json:"account,omitempty"`
	// ProbedAt is when the probe answered.
	ProbedAt time.Time `json:"probed_at"`
	// Status is the usage row status ("ok", "limit-reached", "rejected").
	Status string `json:"status"`
	// Reason is the failure's short text; "" when it answered.
	Reason string `json:"reason,omitempty"`
	// Windows are the probed windows: the all-models ones and the
	// per-model ones of Model.
	Windows []UsageCacheWindow `json:"windows"`
	// Model is the model the probe asked with ("" = the agent's default
	// probe model): a per-model window shows only for its own model.
	Model string `json:"model,omitempty"`
	// Models are the per-model windows by the model whose probe returned
	// them, each with its probe time, kept across probes with other
	// models (an empty list records that the model has none). Key "" is
	// the default probe model.
	Models map[string]ModelWindows `json:"models,omitempty"`
	// Detected is the kind read from the probed token's prefix
	// (oauth-token, api-key, unknown); "" when the entry predates it.
	// Never any part of the token.
	Detected string `json:"detected,omitempty"`
}

// UsageCacheWindow is one probed window.
type UsageCacheWindow struct {
	// Class is the agentio window class ("short", "week").
	Class string `json:"class"`
	// Scope is the model the window is limited to; "" = all usage.
	Scope    string     `json:"scope,omitempty"`
	Percent  float64    `json:"percent"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// ID is the vendor's name of the window ("5h", "7d", "7d_oi").
	ID string `json:"id,omitempty"`
	// Status is the vendor's status of the window ("allowed",
	// "rejected"); "" when it gave none.
	Status string `json:"status,omitempty"`
	// Binding is true when the vendor named it the window that limits
	// the account now.
	Binding bool `json:"binding,omitempty"`
}

// ModelWindows are the per-model windows one model's probe returned.
type ModelWindows struct {
	ProbedAt time.Time          `json:"probed_at"`
	Windows  []UsageCacheWindow `json:"windows"`
}

// ScopedOnly are the windows of ws limited to one model.
func ScopedOnly(ws []UsageCacheWindow) []UsageCacheWindow {
	out := []UsageCacheWindow{}
	for _, w := range ws {
		if w.Scope != "" {
			out = append(out, w)
		}
	}
	return out
}

// WithModel is e after a probe with model e.Model: its per-model windows
// are recorded under that model, beside the ones old kept for other
// models (same agent only).
func (e UsageEntry) WithModel(old UsageEntry, hasOld bool) UsageEntry {
	models := map[string]ModelWindows{}
	if hasOld && old.Agent == e.Agent {
		for k, v := range old.Models {
			models[k] = v
		}
	}
	for k, v := range e.Models {
		models[k] = v
	}
	if len(e.Windows) > 0 {
		models[e.Model] = ModelWindows{ProbedAt: e.ProbedAt, Windows: ScopedOnly(e.Windows)}
	}
	e.Models = models
	if len(e.Models) == 0 {
		e.Models = nil
	}
	return e
}

// ReadUsageCache reads the cache. A missing file is an empty cache; an
// unreadable or foreign one is an empty cache plus the error, so the
// caller can warn and go on (the cache is only an optimisation).
func ReadUsageCache(paths config.Paths) (UsageCache, error) {
	empty := UsageCache{Version: UsageCacheVersion, Logins: map[string]UsageEntry{}}
	b, err := os.ReadFile(paths.LoginUsage())
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	var c UsageCache
	if err := json.Unmarshal(b, &c); err != nil {
		return empty, fmt.Errorf("%s: %w", config.LoginUsageFileName, err)
	}
	if c.Version != UsageCacheVersion {
		return empty, fmt.Errorf("%s: version %d, want %d; it is rebuilt", config.LoginUsageFileName, c.Version, UsageCacheVersion)
	}
	if c.Logins == nil {
		c.Logins = map[string]UsageEntry{}
	}
	return c, nil
}

// UpdateUsageCache merges entries into the cache under its cross-process
// lock and replaces the file atomically (0600), so sessions refreshing at
// the same moment never read half a file or drop each other's entries.
// It returns the cache as written.
func UpdateUsageCache(ctx context.Context, paths config.Paths, entries map[string]UsageEntry) (UsageCache, error) {
	file := paths.LoginUsage()
	if err := os.MkdirAll(filepath.Dir(file), config.PrivateDirMode); err != nil {
		return UsageCache{}, err
	}
	lctx, cancel := context.WithTimeout(ctx, config.LockWait)
	defer cancel()
	lk, err := atomicfile.Lock(lctx, file)
	switch {
	case errors.Is(err, atomicfile.ErrLockTimeout):
		return UsageCache{}, apperr.Wrap(err, apperr.CodeLocked, apperr.ClassFailure,
			fmt.Sprintf("%s is being written by another agx (waited %s)", config.LoginUsageFileName, config.LockWait))
	case err != nil:
		return UsageCache{}, err
	}
	defer func() { _ = lk.Close() }() // the OS drops the lock with the descriptor anyway
	c, _ := ReadUsageCache(paths)     // a broken cache is replaced, not fatal
	for name, e := range entries {
		old, hasOld := c.Logins[name]
		c.Logins[name] = e.WithModel(old, hasOld)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return UsageCache{}, err
	}
	if err := atomicfile.Write(file, append(b, '\n'), atomicfile.Options{Mode: config.TokensFileMode}); err != nil {
		return UsageCache{}, err
	}
	return c, nil
}

// AppendPoolLog appends one line to a pool's log file (0600, created on
// first use). The caller builds the line; it never holds a token.
func AppendPoolLog(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), config.PrivateDirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, config.TokensFileMode)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(line + "\n")
	cerr := f.Close()
	return errors.Join(werr, cerr)
}
