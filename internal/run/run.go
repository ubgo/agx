// Package run owns run folders and starting the agent (spec section 11).
//
// One launch = one folder <agx dir>/runs/<YYYYMMDD-HHMMSS>-<random>/
// (0700) holding run.jsonc (who was started, where, with which pid), the
// delivered settings file, live-login state and the shell plan. The agent
// gets AGX_RUN_ID and the AGX_RUN_* outputs in its env.
//
// "agx disappears": on macOS and Linux Start replaces agx with the agent
// (exec), so the agent keeps agx's pid, which run.jsonc records before the
// exec. Windows has no exec: agx stays as the parent, forwards Ctrl-C and
// returns the agent's exit code (exec_windows.go).
//
// This is the only package (with textdata's shell runner) that starts
// processes; internal/archtest enforces it.
package run

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/ubgo/textdata/atomicfile"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/apperr"
)

// Env variables a launch sets in the agent's environment. Outputs only:
// agx never reads them to choose anything (a nested agx makes its own
// run, edge 50); whoami and the hidden credential command read them.
const (
	EnvRunID      = "AGX_RUN_ID"
	EnvRunAgent   = "AGX_RUN_AGENT"
	EnvRunProfile = "AGX_RUN_PROFILE"
	EnvRunLogin   = "AGX_RUN_LOGIN"
	EnvRunHome    = "AGX_RUN_HOME"
)

// File names inside a run folder.
const (
	RecordFile = "run.jsonc"
	LiveFile   = "live.jsonc"
	PlanFile   = "plan.jsonc"
)

// idTimeLayout is the timestamp part of a run id.
const idTimeLayout = "20060102-150405"

// idRandomLen is the length of the random suffix; with 36 symbols it makes
// two launches in the same second collide with probability ~1e-6.
const idRandomLen = 4

// idAlphabet is the random suffix's symbols (lowercase, digits).
const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// idPattern matches a run id (also a valid claude.LaunchSettings run id).
var idPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-z0-9]{4}$`)

// ValidID reports a well-formed run id (refuses path tricks in --id).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// NewID makes a run id for now.
func NewID(now time.Time) (string, error) {
	b := make([]byte, idRandomLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(idAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = idAlphabet[n.Int64()]
	}
	return now.Format(idTimeLayout) + "-" + string(b), nil
}

// Record is run.jsonc: what was started. Never holds a token.
type Record struct {
	ID string `json:"id"`
	// PID is agx's own pid, which is the agent's after exec (Unix), or the
	// child's pid on Windows.
	PID int `json:"pid"`
	// StartedAt is the process start time (from the process table when
	// readable, else the clock at launch); clean compares it.
	StartedAt time.Time `json:"started_at"`
	Agent     string    `json:"agent"`
	Binary    string    `json:"binary"`
	Profile   string    `json:"profile"`
	Home      string    `json:"home"`
	// Login is the login name, "home" (the home's own login) or
	// "token-ref" (a one-off --token-ref); one word, as AGX_RUN_LOGIN.
	Login  string `json:"login"`
	Folder string `json:"folder"`
	// LiveLogin is true when the run was started with --live-login.
	LiveLogin bool `json:"live_login,omitempty"`
}

// Dir is a run folder path for id under runs.
func Dir(runs, id string) string { return filepath.Join(runs, id) }

// Create makes the run folder (0700, refusing an existing one).
func Create(runs, id string) (string, error) {
	if err := os.MkdirAll(runs, config.PrivateDirMode); err != nil {
		return "", apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "create "+runs+": "+err.Error())
	}
	dir := Dir(runs, id)
	if err := os.Mkdir(dir, config.PrivateDirMode); err != nil {
		return "", apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "create run folder: "+err.Error())
	}
	return dir, nil
}

// WriteRecord writes run.jsonc atomically (0600).
func WriteRecord(dir string, r Record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dir, RecordFile), append(b, '\n'), atomicfile.Options{Mode: config.TokensFileMode})
}

// ReadRecord reads run.jsonc of a run folder.
func ReadRecord(dir string) (Record, error) {
	b, err := os.ReadFile(filepath.Join(dir, RecordFile))
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, fmt.Errorf("%s: %w", RecordFile, err)
	}
	return r, nil
}

// Folder is one run folder found on disk.
type Folder struct {
	ID  string
	Dir string
	// Record is run.jsonc; zero when unreadable (RecordErr says why).
	Record    Record
	RecordErr error
}

// List returns every run folder under runs, oldest first.
func List(runs string) ([]Folder, error) {
	ents, err := os.ReadDir(runs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Folder
	for _, e := range ents {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		f := Folder{ID: e.Name(), Dir: filepath.Join(runs, e.Name())}
		f.Record, f.RecordErr = ReadRecord(f.Dir)
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Remove deletes one run folder. It refuses anything that is not a run
// folder under runs.
func Remove(runs, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("not a run id: %q", id)
	}
	return os.RemoveAll(Dir(runs, id))
}

// dryRunPrefix names the temporary folder a dry run builds in.
const dryRunPrefix = "agx-dry-run-"

// DryRunDir makes a temporary stand-in for run folder id, so a dry run can
// call builders that write a settings file without touching the config
// dir. cleanup removes it.
func DryRunDir(id string) (dir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", dryRunPrefix)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) } // a leftover temp folder is harmless
	dir = filepath.Join(tmp, id)
	if err := os.Mkdir(dir, config.PrivateDirMode); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}
