package config_test

import (
	"strings"
	"testing"
)

// TestAliasNamesCheckedAtLoad: an alias written by hand with a name or a
// word shell-init would skip (a reserved name, a name no shell takes
// unquoted, a word starting with = or @) is refused when the config loads,
// at the alias's file:line:col, with shell-init's own reason. Before, it
// loaded and was then skipped by shell-init with only a warning. A good
// alias still loads.
func TestAliasNamesCheckedAtLoad(t *testing.T) {
	cases := map[string]struct{ alias, want string }{
		"reserved name": {`"agx": "run"`, `config.jsonc:3:5: alias "agx": a name may not be agx, command, builtin, cd, set, function`},
		"shell name":    {`"cd": "run"`, `config.jsonc:3:5: alias "cd": a name may not be`},
		"bad name":      {`"1x": "run"`, `config.jsonc:3:5: alias "1x": a name is letters, digits, _ and -`},
		"= word":        {`"cl": "run =ls"`, `config.jsonc:3:5: alias cl: word "=ls" may not start with = or @`},
		"@ word":        {`"cl": "run @args"`, `config.jsonc:3:5: alias cl: word "@args" may not start with = or @`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(t, "{\n  \"aliases\": {\n    "+c.alias+"\n  }\n}")
			if err == nil {
				t.Fatalf("loaded, want %q", c.want)
			}
			if !strings.Contains(problems(err), c.want) {
				t.Fatalf("err %v\n%s, want %q", err, problems(err), c.want)
			}
		})
	}
	if _, err := parse(t, `{"aliases": {"cl": "run --profile personal", "cl_w-2": "run --model a.b/c:d@e+f"}}`); err != nil {
		t.Fatalf("good aliases refused: %v", err)
	}
}
