package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/shellinit/aliasrule"
)

// aliasesKey is the config object holding the shell aliases.
const aliasesKey = "aliases"

// refMarker is the byte every reference starts with (${…}, $(…), $VAR):
// an env_refs value without it is a plain value in the wrong map.
const refMarker = "$"

// CrossCheck runs the checks the schema cannot express (spec 15.3) and
// returns every problem, sorted by position. It never stops at the first.
func (l *Loaded) CrossCheck(known Known) []Problem {
	c := l.Config
	var out []Problem
	add := func(where, path, msg string) { out = append(out, Problem{Where: where, Path: path, Message: msg}) }

	agentKnown := func(id agentio.ID) bool {
		if slices.Contains(known.Agents, id) {
			return true
		}
		a, ok := c.Agents[id]
		return ok && a.Spec != nil
	}
	if c.DefaultAgent != "" && !agentKnown(c.DefaultAgent) {
		add(l.Where("default_agent"), "default_agent", fmt.Sprintf("unknown agent %s", c.DefaultAgent))
	}
	for id, a := range c.Agents {
		if a.Spec != nil && slices.Contains(known.Agents, id) {
			add(l.Where("agents", string(id), "spec"), "agents."+string(id)+".spec", fmt.Sprintf("%s is a built-in agent; a spec defines a new agent id", id))
		}
		if a.Defaults.Login != "" {
			out = append(out, l.checkLoginRef(a.Defaults.Login, id, []string{"agents", string(id), "defaults", "login"})...)
		}
		if a.Defaults.Login != "" && a.Defaults.HomeLogin {
			add(l.Where("agents", string(id), "defaults"), "agents."+string(id)+".defaults", "login and home_login are both set; keep one")
		}
	}

	// A date format is checked here, where the file names the line, not
	// on first use: a typo in it would otherwise surface only in the
	// screens that print a date.
	for _, df := range []struct {
		value string
		keys  []string
	}{
		{c.DateFormat, []string{"date_format"}},
		{c.Usage.DateFormat, []string{"usage", "date_format"}},
	} {
		if df.value == "" {
			continue
		}
		if _, err := datefmt.Parse(df.value); err != nil {
			path := strings.Join(df.keys, ".")
			add(l.Where(df.keys...), path, fmt.Sprintf("%s %q: %s", path, df.value, err))
		}
	}

	defaults := map[agentio.ID][]string{}
	for _, name := range sortedKeys(c.Profiles) {
		p := c.Profiles[name]
		path := "profiles." + name
		if !agentKnown(p.Agent) {
			add(l.Where("profiles", name, "agent"), path+".agent", fmt.Sprintf("unknown agent %s", p.Agent))
		}
		if strings.TrimSpace(p.Home) == "" {
			add(l.Where("profiles", name), path, "home is empty")
		}
		if p.Default {
			defaults[p.Agent] = append(defaults[p.Agent], name)
		}
		if p.Login != "" && p.HomeLogin {
			add(l.Where("profiles", name, "home_login"), path, "login and home_login are both set; keep one")
		}
		if p.Login != "" {
			out = append(out, l.checkLoginRef(p.Login, p.Agent, []string{"profiles", name, "login"})...)
		}
		for k, v := range p.EnvRefs {
			if !strings.Contains(v, refMarker) {
				add(l.Where("profiles", name, "env_refs", k), path+".env_refs."+k, "not a reference (no $); put plain values in env")
			}
		}
		for id := range p.Blocks {
			if id != p.Agent {
				add(l.Where("profiles", name, string(id)), path+"."+string(id), fmt.Sprintf("a %s block on a %s profile", id, p.Agent))
			}
		}
	}
	for _, id := range sortedKeys(defaults) {
		names := defaults[id]
		if len(names) > 1 {
			sort.Strings(names)
			add(l.Where("profiles", names[1], "default"), "profiles."+names[1]+".default",
				fmt.Sprintf("%s has two default profiles (%s); keep one", id, strings.Join(names, ", ")))
		}
	}

	for _, name := range sortedKeys(c.Logins) {
		lg := c.Logins[name]
		path := "logins." + name
		if len(lg.Agents) == 0 {
			add(l.Where("logins", name), path, "agents is empty; name the agents this login serves")
		}
		for _, id := range lg.Agents {
			if !agentKnown(id) {
				add(l.Where("logins", name, "agents"), path+".agents", fmt.Sprintf("unknown agent %s", id))
			}
		}
		sources := 0
		for _, set := range []bool{lg.Token != "", lg.Stored, lg.Command != ""} {
			if set {
				sources++
			}
		}
		if lg.Kind == KindPool || lg.Pool != nil {
			out = append(out, l.checkPool(name, lg, sources, known)...)
			continue
		}
		switch {
		case lg.Kind == KindAPIKeyHelper && lg.Command == "":
			add(l.Where("logins", name), path, "an api-key-helper login needs command")
		case lg.Kind != KindAPIKeyHelper && lg.Command != "":
			add(l.Where("logins", name, "command"), path+".command", "command is only for api-key-helper logins")
		case sources != 1:
			add(l.Where("logins", name), path, "set exactly one of token, stored or command")
		}
	}

	for _, dir := range sortedKeys(c.Projects) {
		for _, id := range sortedKeys(c.Projects[dir]) {
			b := c.Projects[dir][id]
			keys := []string{"projects", dir, string(id)}
			path := "projects." + dir + "." + string(id)
			if !agentKnown(id) {
				add(l.Where(keys...), path, fmt.Sprintf("unknown agent %s", id))
			}
			if b.Login != "" && b.HomeLogin {
				add(l.Where(keys...), path, "login and home_login are both set; keep one")
			}
			if b.Profile != "" {
				p, ok := c.Profiles[b.Profile]
				switch {
				case !ok:
					add(l.Where(append(keys, "profile")...), path+".profile", fmt.Sprintf("unknown profile %s", b.Profile))
				case p.Agent != id:
					add(l.Where(append(keys, "profile")...), path+".profile", fmt.Sprintf("profile %s is a %s profile, bound for %s", b.Profile, p.Agent, id))
				}
			}
			if b.Login != "" {
				out = append(out, l.checkLoginRef(b.Login, id, append(keys, "login"))...)
			}
		}
	}

	// Aliases: the caller's value check (charset and command tree, when
	// given), then the shell layer's own rule for the name and the words
	// (aliasrule), so an alias that loads is one shell-init writes, never
	// one it skips. A hand-written reserved name fails the load at its
	// position like any other config problem (spec 15, decision 9: a bad
	// alias fails the config load for every command). One problem per
	// alias: the value check's message wins when both would fire.
	for _, name := range sortedKeys(c.Aliases) {
		args := c.Aliases[name]
		var err error
		if known.Alias != nil {
			err = known.Alias(args)
		}
		if err == nil {
			err = aliasrule.Check(name, args)
		}
		if err != nil {
			add(l.Where(aliasesKey, name), aliasesKey+"."+name, err.Error())
		}
	}
	return out
}

// checkPool checks a kind "pool" login (and a pool block on any other
// kind): it holds no value, serves exactly one agent that can run a pool,
// and its members are other token logins of that agent, each named once;
// the numbers parse and are in range. Every problem is reported.
func (l *Loaded) checkPool(name string, lg Login, sources int, known Known) []Problem {
	var out []Problem
	path := "logins." + name
	add := func(keys []string, msg string) {
		out = append(out, Problem{Where: l.Where(append([]string{"logins", name}, keys...)...), Path: strings.Join(append([]string{path}, keys...), "."), Message: msg})
	}
	switch {
	case lg.Kind != KindPool:
		add([]string{"pool"}, "pool is only for kind pool logins")
		return out
	case lg.Pool == nil:
		add(nil, "a pool login needs pool (own and shared member logins)")
		return out
	}
	if sources > 0 {
		add(nil, "a pool login holds no token, stored value or command; its members do")
	}
	if lg.Provider != "" {
		add([]string{"provider"}, "a pool login takes no provider; its members do")
	}
	if len(lg.Agents) != 1 {
		add([]string{"agents"}, "a pool login serves exactly one agent")
	}
	for _, id := range lg.Agents {
		if known.PoolAgent == nil {
			continue
		}
		if reason := known.PoolAgent(id); reason != "" {
			add([]string{"agents"}, fmt.Sprintf("%s cannot use a login pool (%s)", id, reason))
		}
	}
	p := lg.Pool
	if len(p.Members()) == 0 {
		add([]string{"pool"}, "the pool has no members; list logins in own or shared")
	}
	seen := map[string]bool{}
	for _, g := range []struct {
		key   string
		names []string
	}{{"own", p.Own}, {"shared", p.Shared}} {
		for _, m := range g.names {
			keys := []string{"pool", g.key}
			if seen[m] {
				add(keys, fmt.Sprintf("member %s is listed more than once in own/shared", m))
				continue
			}
			seen[m] = true
			if msg := l.memberProblem(m, lg.Agents); msg != "" {
				add(keys, msg)
			}
		}
	}
	if p.Force != nil && *p.Force == "" {
		// An empty name would read as "off" while looking set: refused,
		// like every other value that could be silently ignored.
		add([]string{"pool", "force"}, `force is empty; name a login or write null`)
	}
	if p.Force != nil && *p.Force != "" {
		if msg := l.memberProblem(*p.Force, lg.Agents); msg != "" {
			add([]string{"pool", "force"}, "force: "+msg)
		}
	}
	if _, err := p.Rules(); err != nil {
		add([]string{"pool"}, err.Error())
	}
	for _, keys := range l.poolNulls(name, p) {
		add(keys, strings.Join(keys, ".")+" is null; give a value or leave the key out")
	}
	return out
}

// poolNulls finds an explicit null where a pool needs a value (a cap, the
// cutoff, enabled): decoded it would read as "use the default", a value
// silently ignored. Only force takes null (off).
func (l *Loaded) poolNulls(name string, p *Pool) [][]string {
	var out [][]string
	isNull := func(keys ...string) bool {
		ptr := Pointer(append([]string{"logins", name, "pool"}, keys...)...)
		if l.Doc == nil || !l.Doc.Has(ptr) {
			return false
		}
		v, err := l.Doc.Get(ptr)
		return err == nil && v == nil
	}
	check := func(keys ...string) {
		if isNull(keys...) {
			out = append(out, append([]string{"pool"}, keys...))
		}
	}
	check("short_unavailable_at_percent")
	check("defaults", "short")
	check("defaults", "week")
	for _, m := range sortedKeys(p.Accounts) {
		check("accounts", m, "short")
		check("accounts", m, "week")
		check("accounts", m, "enabled")
	}
	return out
}

// memberProblem says why login m cannot be a pool member for agents ("" =
// it can): it must exist, serve the pool's agent and carry a token (not
// a pool, not an api-key-helper, whose key agx cannot fetch).
func (l *Loaded) memberProblem(m string, agents []agentio.ID) string {
	ml, ok := l.Config.Logins[m]
	switch {
	case !ok:
		return "unknown login " + m
	case ml.Kind == KindPool:
		return fmt.Sprintf("member %s is a pool; a member must be a token login", m)
	case ml.Kind == KindAPIKeyHelper:
		return fmt.Sprintf("member %s is an api-key-helper; agx cannot fetch its key", m)
	}
	for _, id := range agents {
		if !ml.Serves(id) {
			return fmt.Sprintf("member %s serves %s, not %s", m, joinIDs(ml.Agents), id)
		}
	}
	return ""
}

// checkLoginRef checks that a login named at keys exists and serves id.
func (l *Loaded) checkLoginRef(login string, id agentio.ID, keys []string) []Problem {
	lg, ok := l.Config.Logins[login]
	path := strings.Join(keys, ".")
	if !ok {
		return []Problem{{Where: l.Where(keys...), Path: path, Message: fmt.Sprintf("unknown login %s", login)}}
	}
	if !lg.Serves(id) {
		return []Problem{{Where: l.Where(keys...), Path: path, Message: fmt.Sprintf("login %s serves %s, not %s", login, joinIDs(lg.Agents), id)}}
	}
	return nil
}

// joinIDs renders ids comma-separated.
func joinIDs(ids []agentio.ID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = string(id)
	}
	return strings.Join(s, ", ")
}

// sortedKeys returns a map's keys sorted, so checks and listings are
// deterministic.
func sortedKeys[K ~string, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
