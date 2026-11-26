package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/jsonc"

	"github.com/ubgo/agx/config"
	"github.com/ubgo/agx/internal/agents"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/resolve"
	"github.com/ubgo/agx/internal/run"
	"github.com/ubgo/agx/live"
	"github.com/ubgo/agx/login"
)

// statusLineVerb is the command a live run's status line runs: "<agx
// path> statusline".
const statusLineVerb = "statusline"

// StatusLine is agx statusline's data: the account a live run is on now,
// from its live.jsonc and the login-usage cache only (no probe, no
// network), so the agent can run it on every refresh.
type StatusLine struct {
	// RunID is AGX_RUN_ID; "" outside a session agx started.
	RunID string `json:"run_id"`
	// Login is the run's login (the pool, or the chosen login).
	Login string `json:"login"`
	// Pool is true when Login is a pool.
	Pool bool `json:"pool"`
	// InUse is the login whose key the agent holds (live.jsonc in_use);
	// InUseText the same as people read it.
	InUse     string `json:"in_use"`
	InUseText string `json:"in_use_text"`
	// Short and Week are InUse's 5-hour and weekly usage in percent from
	// the login-usage cache; null when unknown.
	Short *float64 `json:"short_percent"`
	Week  *float64 `json:"week_percent"`
	// Line is the one line printed ("" outside a live run).
	Line string `json:"line"`
}

// StatusLine reads the current run's account. Outside a live run it
// returns an empty line and no error: a status line must never print an
// error in the agent's screen.
func (a *App) StatusLine() StatusLine {
	var s StatusLine
	id, _ := a.d.Lookup(run.EnvRunID)
	if id == "" || !run.ValidID(id) {
		return s
	}
	s.RunID = id
	st, err := live.Read(run.Dir(a.paths.Runs, id))
	if err != nil {
		return s
	}
	s.Login, s.InUse, s.InUseText = st.Want(), st.InUse, InUseText(st, a.Now())
	if cfg, err := a.Config(); err == nil {
		lg := cfg.Config.Logins[s.Login]
		s.Pool = lg.Kind == config.KindPool
	}
	if s.InUse != "" {
		if cache, err := login.ReadUsageCache(a.paths); err == nil {
			if e, ok := cache.Logins[s.InUse]; ok {
				r := readingOf(e, nil)
				if r.Short != nil {
					s.Short = &r.Short.Percent
				}
				if r.Week != nil {
					s.Week = &r.Week.Percent
				}
			}
		}
	}
	s.Line = statusLineText(s)
	return s
}

// statusLineText is the line: "pool auto: alice · 5h 40% · week 61%",
// "pool auto: none (all over caps) since 10:52", "login alice".
func statusLineText(s StatusLine) string {
	head := "login " + s.Login
	if s.Pool {
		head = "pool " + s.Login
	}
	if s.InUseText == "" {
		return head + ": waiting for the first key"
	}
	if !s.Pool && s.InUseText == s.Login {
		return statusLineUsage(head, s)
	}
	return statusLineUsage(head+": "+s.InUseText, s)
}

// statusLineSep separates the parts of the status line.
const statusLineSep = " · "

// statusLineUsage adds the cached usage of the login in use.
func statusLineUsage(line string, s StatusLine) string {
	if s.Short != nil {
		line += statusLineSep + fmt.Sprintf("5h %.0f%%", math.Round(*s.Short))
	}
	if s.Week != nil {
		line += statusLineSep + fmt.Sprintf("week %.0f%%", math.Round(*s.Week))
	}
	return line
}

// selfCommand is the shell command that runs this agx binary with verb,
// plus --config when this run used one: what the agent runs for its key
// (agx credential) and its status line (agx statusline).
func (a *App) selfCommand(verb string) string {
	self := a.d.Self
	if self == "" {
		self = selfFallback
	}
	cmd := shellQuote(self) + " " + verb
	if a.d.ConfigFlag != "" {
		cmd += " --config " + shellQuote(a.paths.File)
	}
	return cmd
}

// addStatusLine gives a live run's agent the status line "agx
// statusline" (live.status_line, default on), so the session shows the
// account it is on as it switches. It never replaces a status line the
// user set: one in the run's own settings (profile, --setting, a raw
// settings file) or in the agent's settings files for this home and
// folder is kept, and a decision line says so.
func (a *App) addStatusLine(res resolve.Result, dir string, st *launchState) error {
	cfg, err := a.Config()
	if err != nil {
		return err
	}
	if !cfg.Config.Live.StatusLineOn() {
		return nil
	}
	spec, ok := agents.StatusLine(res.Agent.ID())
	if !ok {
		return nil
	}
	src := "config live.status_line"
	if cfg.Config.Live.StatusLine == nil {
		src = SourceDefault
	}
	note := func(v string) {
		st.notes = append(st.notes, report.Decision{Name: resolve.DecSetting, Value: spec.Key + " " + v, Source: src})
	}
	if m, err := decodeObject(st.settings); err == nil {
		if _, set := m[spec.Key]; set {
			note("from the run's settings kept; agx statusline not added")
			return nil
		}
	}
	if path := a.userSetting(res.Agent, res.Home, dir, spec.Key); path != "" {
		note("of " + a.tilde(path) + " kept; agx statusline not added")
		return nil
	}
	raw, err := spec.Settings(a.selfCommand(statusLineVerb))
	if err != nil {
		return err
	}
	merged, err := agentio.MergeSettings(st.settings, raw, statusLineLabel)
	if err != nil {
		return err
	}
	st.settings = merged
	note("agx statusline (the account in use)")
	return nil
}

// statusLineLabel names agx's status line in a settings merge conflict.
const statusLineLabel = "agx statusline"

// userSetting is the first of the agent's own settings files for home h
// and folder dir (agentio ConfigReader, highest precedence first, the
// command-line layer left out: that is agx's) that sets top-level key;
// "" when none does or the agent lists none. An unreadable file is
// skipped: the agent reports it itself.
func (a *App) userSetting(ag agentio.Agent, h agentio.Home, dir, key string) string {
	cr, ok := agentio.ConfigOf(ag)
	if !ok {
		return ""
	}
	layers, err := cr.ConfigLayers(h, dir)
	if err != nil {
		return ""
	}
	for _, l := range layers {
		if !l.Exists || l.Scope == agentio.ConfigCLI || strings.TrimSpace(l.Path) == "" {
			continue
		}
		if hasTopKey(l.Path, key) {
			return l.Path
		}
	}
	return ""
}

// hasTopKey reports whether the JSON(C) object in path sets key.
func hasTopKey(path, key string) bool {
	doc, err := jsonc.Open(path)
	if err != nil || !doc.Existed() || doc.Err() != nil {
		return false
	}
	body, err := doc.JSON()
	if err != nil {
		return false
	}
	m, err := decodeObject(body)
	if err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
