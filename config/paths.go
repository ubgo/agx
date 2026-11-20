package config

import (
	"os"
	"path/filepath"
	"strings"
)

// File and folder names inside the config dir (spec 15.1).
const (
	// ConfigFileName is the main config file.
	ConfigFileName = "config.jsonc"
	// TokensFileName holds pasted token values keyed by login name.
	TokensFileName = "tokens.jsonc"
	// RunsDirName holds one folder per launch (spec section 11).
	RunsDirName = "runs"
	// SessionsDirName is the default session-folder root (agx new).
	SessionsDirName = "sessions"
	// DefaultDirName is the config dir under the user home.
	DefaultDirName = ".agx"
	// CacheDirName holds files agx can rebuild at any time.
	CacheDirName = "cache"
	// LoginUsageFileName is the login-usage cache inside CacheDirName:
	// the last probe of each login-pool member (it holds account labels,
	// so it is written 0600).
	LoginUsageFileName = "login-usage.jsonc"
	// LoginUsageHistoryFileName is the bounded history of the probes'
	// readings beside the cache (0600, like it).
	LoginUsageHistoryFileName = "login-usage-history.jsonc"
)

// Environment variables that choose the config dir, first match wins
// (spec 13 global flags, 15.1).
const (
	EnvConfigDir = "AGX_CONFIG_DIR"
	EnvHome      = "AGX_HOME"
)

// File modes agx creates (spec 15.1).
const (
	// ConfigFileMode is config.jsonc: readable, holds no secret by design
	// (plain tokens are allowed, doctor lists them).
	ConfigFileMode os.FileMode = 0o644
	// TokensFileMode is tokens.jsonc: owner only.
	TokensFileMode os.FileMode = 0o600
	// PrivateDirMode is the config dir and every run folder.
	PrivateDirMode os.FileMode = 0o700
)

// Paths are the files of one config dir.
type Paths struct {
	// Dir is the config dir (absolute).
	Dir string
	// File is config.jsonc (or the --config file).
	File string
	// Tokens is tokens.jsonc next to File.
	Tokens string
	// Runs is the run-folder root.
	Runs string
}

// LoginUsage is the login-usage cache file of this config dir.
func (p Paths) LoginUsage() string {
	return filepath.Join(p.Dir, CacheDirName, LoginUsageFileName)
}

// LoginUsageHistory is the login-usage history file of this config dir.
func (p Paths) LoginUsageHistory() string {
	return filepath.Join(p.Dir, CacheDirName, LoginUsageHistoryFileName)
}

// Lookup reads one environment variable; injected so nothing here reads
// os.Environ behind the caller's back (tests pass a fixture env).
type Lookup func(name string) (string, bool)

// ResolvePaths picks the config dir: --config FILE (its folder), else
// AGX_CONFIG_DIR, else AGX_HOME, else <user home>/.agx. "~" and ${VAR} in
// the chosen value are expanded with lookup.
func ResolvePaths(configFlag string, lookup Lookup, userHome string) Paths {
	if configFlag != "" {
		f := ExpandPath(configFlag, lookup, userHome)
		dir := filepath.Dir(f)
		return Paths{Dir: dir, File: f, Tokens: filepath.Join(dir, TokensFileName), Runs: filepath.Join(dir, RunsDirName)}
	}
	dir := filepath.Join(userHome, DefaultDirName)
	for _, name := range []string{EnvConfigDir, EnvHome} {
		if v, ok := lookup(name); ok && v != "" {
			dir = ExpandPath(v, lookup, userHome)
			break
		}
	}
	return Paths{Dir: dir, File: filepath.Join(dir, ConfigFileName), Tokens: filepath.Join(dir, TokensFileName), Runs: filepath.Join(dir, RunsDirName)}
}

// ExpandPath expands a leading "~" and every $VAR / ${VAR} (unset = "")
// and makes the result absolute and clean. It is for paths only (homes,
// files), never for secret references, which go through strict
// expansion in package login.
func ExpandPath(p string, lookup Lookup, userHome string) string {
	if p == "" {
		return ""
	}
	if p == "~" {
		p = userHome
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(userHome, p[2:])
	}
	p = os.Expand(p, func(name string) string {
		v, _ := lookup(name)
		return v
	})
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// NormalizeDir is the stored form of a project folder (spec 13.6):
// absolute, "~" expanded, symlinks resolved when the folder exists, no
// trailing separator, OS separators. The spelling is kept as the file
// system gives it (never lower-cased), so stored keys and every printed
// path stay the user's own; comparisons go through DirKeyFor. A folder that
// does not exist keeps its cleaned absolute form, so a binding to a
// not-yet-created folder still matches once it exists at the same path.
func NormalizeDir(p string, lookup Lookup, userHome string) string {
	p = ExpandPath(p, lookup, userHome)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	return filepath.Clean(p)
}

// DirKeyFor is the comparison form of a folder already normalized with
// NormalizeDir: case-folded where goos's default volumes are
// case-insensitive (macOS APFS, Windows NTFS). Use it to compare or look
// up folders, never to store or print them (that lower-cased every path on
// macOS). goos is a parameter, not runtime.GOOS, so callers can inject it
// and a test can check the macOS and Windows rule on any machine. A
// case-sensitive APFS volume is rare; folding there only merges two
// folders that differ by case.
func DirKeyFor(goos, normalized string) string {
	if goos == goosDarwin || goos == goosWindows {
		return strings.ToLower(normalized)
	}
	return normalized
}

// GOOS names used for platform rules.
const (
	goosDarwin  = "darwin"
	goosWindows = "windows"
)

// TildePath renders an absolute path under userHome as "~/…" for display.
func TildePath(p, userHome string) string {
	if userHome == "" {
		return p
	}
	if p == userHome {
		return "~"
	}
	if rel, ok := strings.CutPrefix(p, userHome+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rel
	}
	return p
}

// Pointer builds a JSON Pointer (RFC 6901) to a value, escaping "~" and
// "/" in each key. Used for every config path agx edits, because project
// keys are folder paths full of dots and slashes that the dotted form
// would split.
func Pointer(keys ...string) string {
	var b strings.Builder
	for _, k := range keys {
		b.WriteByte('/')
		k = strings.ReplaceAll(k, "~", "~0")
		k = strings.ReplaceAll(k, "/", "~1")
		b.WriteString(k)
	}
	return b.String()
}
