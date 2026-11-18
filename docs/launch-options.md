# Passing anything to an agent

Every way to hand a setting, an option, a credential, an environment variable, a script or a raw flag to an agent for one launch. Read this when one account needs stricter settings than the others, when a project needs an API key in the agent's environment, when you want Claude's `--effort` or Codex's sandbox on every launch of a profile, or when you need a flag agx has never heard of. Each recipe shows what you write, what agx does, and what runs: the output is real `--dry-run` output from the tested examples under [`examples/`](../examples/README.md).

## The short version

| You want to pass | Use |
|---|---|
| a Claude settings file | profile `"claude": { "settings": "path" }` |
| some Claude settings keys, always | profile `"claude": { "settings": { … } }` |
| a settings key for one run | `--setting key=value` (Claude: settings JSON; Codex: `-c`) |
| settings visible inline instead of a file | `--settings-delivery inline` or profile `"settings_delivery": "inline"` |
| a Claude settings file or JSON of your own, on top of agx's | raw `-- --settings FILE-or-JSON` (merged key by key, recipe 8) |
| a model, prompt, extra folder, permission level | `--model`, `--prompt`, `--add-dir`, `--autonomy` (plus `--consent full-autonomy` for `full`); profile `model` and `autonomy` |
| a typed vendor launch flag (Claude `effort`, Codex `sandbox`) | the profile's agent block, or `--option key=value` |
| a credential | a login (`--login`, `--home-login`, `--auto-login`, profile `login`, `AGX_LOGIN`) or `--token-ref` |
| any environment variable | `--env NAME=value`, `--no-env NAME`, `--env-file`, profile `env` / `env_refs` / `env_file` |
| a script before the agent | profile `before_run`, or `--before-run PATH` |
| anything at all, including flags agx has never heard of | after `--`, or profile `args` (raw always wins) |

Nothing here edits an agent's own settings files: every option applies to one launch only.

## Before you start

- A config with at least one profile (`agx config init --discovered` writes one; see [Getting started](getting-started.md)).
- Add `--dry-run` to any command on this page: agx prints every decision with its source and starts nothing. Add `--quiet` to drop the warnings, or `--json --field data.command.line` to print only the command it would run.
- Every option below works the same on `agx run`, `agx new` and `agx resume` (see the [command reference](commands/launch.md#agx-run)).

The in-page runnable blocks below use this config (the docs gate runs them against it); the snippets come from the example folders named next to them:

<!-- fixture: recipes -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      "default": true,
      "env": { "DEBUG": "1" },
      "claude": { "effort": "high", "settings": { "cleanupPeriodDays": 20 } }
    },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "pi": { "agent": "pi", "home": "~/.pi/agent", "default": true }
  }
}
```

## Recipe 1: a Claude settings file

Problem: one account needs a stricter settings file (hooks, permissions) than your others, but you do not want to edit that home's `settings.json`.

Step 1: write the settings file. It is JSONC, so comments are fine ([`examples/launch-settings`](../examples/launch-settings/README.md)):

<!-- snippet: examples/launch-settings/home/.config/claude/strict.json -->
```jsonc
// Comments are fine: agx reads this file as JSONC.
{
  "permissions": { "deny": ["Bash(rm -rf:*)"] },
  "cleanupPeriodDays": 10
}
```

Step 2: point the profile at it. A relative path is relative to the config dir (`~/.agx`):

<!-- snippet: examples/launch-settings/config.jsonc#strict -->
```jsonc
    "strict": {
      "agent": "claude",
      "home": "~/.claude-work",
      "claude": { "settings": "~/.config/claude/strict.json" }
    },
```

Try it:

<!-- snippet: examples/launch-settings/session.txt#file -->
```sh
$ agx run --profile strict --dry-run --quiet
agent     claude                                                                                        from profile strict
profile   strict                                                                                        from --profile
home      ~/.claude-work                                                                                from profile strict
login     home login                                                                                    from default (no login named)
binary    /usr/local/bin/claude version unknown                                                         from no install metadata
folder    ~/projects/app                                                                                from current folder
settings  cleanupPeriodDays, permissions.deny                                                           from profile strict (file ~/.config/claude/strict.json)
          delivered as file ~/.agx/runs/20261118-061036-h9dl/settings-20261118-061036-h9dl.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261118-061036-h9dl                                                              from AGX_RUN_ID=20261118-061036-h9dl
command   claude --settings ~/.agx/runs/20261118-061036-h9dl/settings-20261118-061036-h9dl.json         from built by agentio
env       set AGX_RUN_ID=20261118-061036-h9dl                                                           from launch environment
          set AGX_RUN_AGENT=claude                                                                      from launch environment
          set AGX_RUN_PROFILE=strict                                                                    from launch environment
          set AGX_RUN_LOGIN=home                                                                        from launch environment
          set AGX_RUN_HOME=~/.claude-work                                                               from launch environment
          set CLAUDE_CONFIG_DIR=~/.claude-work                                                          from launch environment
note      installed Claude Code version unknown; flags not checked
```

agx reads the file, merges any other settings sources over it (recipes 2 and 3), writes the result into the [run folder](guides/run-folders.md) and passes its path. Claude merges that over the home's own `settings.json` for this run only. A file that does not exist stops the launch, in a dry run too:

<!-- snippet: examples/launch-settings/session.txt#missing-file -->
```sh
$ agx run --profile missing --dry-run --quiet
agx: profile missing: settings file ~/.config/claude/nope.json not found
[exit 125]
```

## Recipe 2: settings keys inline in the profile

Problem: you want a few keys on every launch of a profile without keeping a separate file.

Fix: write them as an object in the profile's `claude` block.

<!-- snippet: examples/launch-settings/config.jsonc#work -->
```jsonc
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      "default": true,
      "claude": { "settings": { "cleanupPeriodDays": 20, "statusLine": { "type": "command", "command": "~/bin/status.sh" } } }
    },
```

<!-- snippet: examples/launch-settings/session.txt#object -->
```sh
$ agx run --profile work --dry-run --quiet
...
settings  cleanupPeriodDays, statusLine.command, statusLine.type                                        from profile work
          delivered as file ~/.agx/runs/20261118-061036-urz6/settings-20261118-061036-urz6.json (0600)  from settings_delivery file
...
command   claude --settings ~/.agx/runs/20261118-061036-urz6/settings-20261118-061036-urz6.json         from built by agentio
...
```

- Every key you write is kept and passed, known or not.
- Keys agx knows are type-checked: a wrong type stops the launch and names the key's file, line and column (in `config.jsonc` for an inline object, in the settings file when the profile names one), the profile, the key and the expected type (never the value).
- An unknown key passes with a note, and a "did you mean" when one is close (recipe 3).

<!-- snippet: examples/launch-settings/session.txt#profile-type -->
```sh
$ agx run --profile typo --dry-run --quiet
agx: config.jsonc:32:86: profile typo settings cleanupPeriodDays: expected int
[exit 125]
```

## Recipe 3: one settings key for one run

Problem: you want to try a settings key once, without touching the config.

Fix: `--setting key=value`, repeated as often as you like. It merges over the profile's settings for this run:

<!-- snippet: examples/launch-settings/session.txt#one-run -->
```sh
$ agx run --profile work --setting cleanupPeriodDays=30 --setting 'permissions.allow=["Read"]' --settings-delivery inline --dry-run --quiet --json --field data.command.line
claude --settings "{\"cleanupPeriodDays\":30,\"permissions\":{\"allow\":[\"Read\"]},\"statusLine\":{\"command\":\"~/bin/status.sh\",\"type\":\"command\"}}"
```

- Dotted keys nest: `permissions.allow` sets `{"permissions": {"allow": …}}`, next to the profile's keys.
- A value that parses as JSON (`30`, `true`, `["a"]`, `{"k":1}`) is used as that JSON; anything else is a string. Quote it as JSON to force a string:

<!-- snippet: examples/launch-settings/session.txt#string-value -->
```sh
$ agx run --profile work --setting 'model="30"' --settings-delivery inline --dry-run --quiet --json --field data.command.line
claude --settings "{\"cleanupPeriodDays\":20,\"model\":\"30\",\"statusLine\":{\"command\":\"~/bin/status.sh\",\"type\":\"command\"}}"
```

- A key agx does not model passes with a note, and a close match is suggested:

<!-- snippet: examples/launch-settings/session.txt#not-modelled -->
```sh
$ agx run --profile work --setting cleanupPeriodDay=30 --dry-run --quiet
...
settings  cleanupPeriodDays, statusLine.command, statusLine.type                                        from profile work
          cleanupPeriodDay                                                                              from --setting
          delivered as file ~/.agx/runs/20261118-061036-7axa/settings-20261118-061036-7axa.json (0600)  from settings_delivery file
...
note      --setting cleanupPeriodDay: not modelled (did you mean cleanupPeriodDays?)
          installed Claude Code version unknown; flags not checked
```

- A known key with a value of the wrong type is refused:

<!-- snippet: examples/launch-settings/session.txt#wrong-type -->
```sh
$ agx run --profile work --setting cleanupPeriodDays=soon --dry-run --quiet
agx: --setting cleanupPeriodDays: expected int
[exit 125]
```

- A settings key that replaces an agx mechanism is allowed, with a note. `apiKeyHelper` supplies the credential, so no agx login is used for the run:

<!-- snippet: examples/launch-settings/session.txt#key-helper -->
```sh
$ agx run --profile work --setting 'apiKeyHelper=gopass show -o work/anthropic-api-key' --dry-run --quiet
...
settings  cleanupPeriodDays, statusLine.command, statusLine.type                                        from profile work
          apiKeyHelper                                                                                  from --setting
note      --setting apiKeyHelper supplies the credential; no agx login is used for this run             from one input per concern; the command line wins
settings  delivered as file ~/.agx/runs/20261118-061054-2vzs/settings-20261118-061054-2vzs.json (0600)  from settings_delivery file
...
note      installed Claude Code version unknown; flags not checked
```

### How settings reach the agent

| `settings_delivery` | How | When |
|---|---|---|
| `file` (default) | a `settings-<run id>.json` file (mode 0600) in the run folder, passed as `--settings <path>` | always safe: nothing on the command line, no length limit |
| `inline` | `--settings '<json>'` | when you want no file; agx warns once that inline settings are visible in `ps` |

Set it per profile (`"settings_delivery": "inline"`) or per run (`--settings-delivery inline`):

<!-- snippet: examples/launch-settings/config.jsonc#visible -->
```jsonc
    "visible": {
      "agent": "claude",
      "home": "~/.claude-work",
      "settings_delivery": "inline",
      "claude": { "settings": { "cleanupPeriodDays": 20 } }
    },
```

<!-- snippet: examples/launch-settings/session.txt#inline-profile -->
```sh
$ agx run --profile visible --dry-run --quiet
...
settings  cleanupPeriodDays                               from profile visible
...
command   claude --settings "{\"cleanupPeriodDays\":20}"  from built by agentio
...
note      settings are visible in ps (inline delivery, from profile visible)
          installed Claude Code version unknown; flags not checked
```

The file stays in the run folder until `agx clean`, because agx has exec'd into the agent and cannot remove it afterwards (see [Run folders](guides/run-folders.md)). Codex settings never use a file: they always go as `-c` (recipe 9).

## Recipe 4: a Claude plan token from gopass

Problem: you have several Claude plan accounts and want to run any home on any of them.

Fix: store each token as a login, then pick it per run.

```sh
agx login add --name carol --agent claude --kind oauth-token --token-ref '${gopass:personal/claude/carol}'
agx run --profile work --login carol
agx run --profile work --token-ref '${gopass:personal/claude/carol}' --token-kind oauth-token
agx run --profile work --token-ref 'plain-token-value' --token-kind oauth-token     # a plain value is allowed too
```

What agx does with the token: it resolves the reference (gopass, `$(…)` or `${VAR}`, with a 30 second limit), trims it, refuses an empty or multi-line value, and puts it in the agent's environment, never on its command line. It is redacted everywhere agx prints. A plain `--dry-run` checks the reference but resolves nothing (`carol (not resolved: --dry-run)`); add `--resolved-secrets` to resolve every reference and see that it works, still without the value ([`examples/launch-env`](../examples/launch-env/README.md)):

<!-- snippet: examples/launch-env/session.txt#resolved -->
```sh
$ agx run --profile work --login carol --dry-run --resolved-secrets
agx: warning: installed Claude Code version unknown; flags not checked
...
login    carol (resolved)                        from --login
...
command  claude                                  from built by agentio
...
```

See [Logins](logins.md) for every login kind.

## Recipe 5: any environment variable

Problem: an account needs an API key, a region or a debug switch in the agent's environment, and you do not want it in your shell.

Step 1: put what every launch of the profile needs in the profile ([`examples/launch-env`](../examples/launch-env/README.md)):

<!-- snippet: examples/launch-env/config.jsonc#profiles -->
```jsonc
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      "default": true,
      "env": { "DEBUG": "1" },                                     // literal, never expanded
      "env_refs": { "API_KEY": "${gopass:work/some-api-key}" },    // references, expanded strictly
      "env_file": "envs/work.env"                                  // relative to the config dir; $(…) off
    },
    "builds": {
      "agent": "claude",
      "home": "~/.claude",
      "env_file": "envs/cmds.env",
      "env_file_commands": true                                    // allow $(…) in env_file
    },
    "nocmds": {
      "agent": "claude",
      "home": "~/.claude",
      "env_file": "envs/cmds.env"
    },
    "service": {
      "agent": "claude",
      "home": "~/.claude",
      "env": { "MY_SERVICE_TOKEN": "literal-abc123", "SERVICE_TIMEOUT_MS": "3000" }   // literals: shown as written
    }
```

The profile's `env_file` is a dotenv file, relative to the config dir:

<!-- snippet: examples/launch-env/home/.agx/envs/work.env -->
```sh
# A dotenv file for the work profile, relative to the config dir (~/.agx).
REGION=eu-west-1
SERVICE_TOKEN=${gopass:work/service-token}
```

Step 2: check what the launch gets. The dry run names every variable and where it came from; values from a reference or a file are redacted:

<!-- snippet: examples/launch-env/session.txt#profile -->
```sh
$ agx run --profile work --dry-run --quiet
agent    claude                                 from profile work
profile  work                                   from --profile
home     ~/.claude-work                         from profile work
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
env      REGION, SERVICE_TOKEN                  from profile work env_file envs/work.env
         DEBUG                                  from profile work env
         API_KEY                                from profile work env_refs
run      ~/.agx/runs/20261118-061119-8qmq       from AGX_RUN_ID=20261118-061119-8qmq
command  claude                                 from built by agentio
env      set REGION=[redacted]                  from launch environment
         set SERVICE_TOKEN=[redacted]           from launch environment
         set DEBUG=1                            from launch environment
         set API_KEY=[redacted]                 from launch environment
         set AGX_RUN_ID=20261118-061119-8qmq    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=work               from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude-work        from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work   from launch environment
note     installed Claude Code version unknown; flags not checked
```

Step 3: add, override or remove variables for one run with `--env-file` (relative to the current folder), `--env NAME=value` and `--no-env NAME`:

```sh
agx run --profile work --env API_KEY='${gopass:work/some-api-key}' --env DEBUG=1
agx run --profile work --env GITHUB_TOKEN='$(gh auth token)' --no-env HTTP_PROXY
agx run --profile work --env API_KEY=plain-value-is-fine --env-file ./local.env
```

<!-- snippet: examples/launch-env/session.txt#one-run -->
```sh
$ agx run --profile work --env-file ./local.env --env GITHUB_TOKEN='$(echo gh-fake)' --env NAME=plain-value --no-env HTTP_PROXY --dry-run --quiet
...
env      REGION, SERVICE_TOKEN                  from profile work env_file envs/work.env
         DEBUG                                  from profile work env
         API_KEY                                from profile work env_refs
         LOG_LEVEL                              from --env-file ./local.env
         GITHUB_TOKEN, NAME                     from --env
         -HTTP_PROXY                            from --no-env
...
env      set REGION=[redacted]                  from launch environment
         set SERVICE_TOKEN=[redacted]           from launch environment
         set DEBUG=1                            from launch environment
         set API_KEY=[redacted]                 from launch environment
         set LOG_LEVEL=[redacted]               from launch environment
         set GITHUB_TOKEN=[redacted]            from launch environment
         set NAME=plain-value                   from launch environment
         set AGX_RUN_ID=20261118-061119-ract    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=work               from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude-work        from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work   from launch environment
...
```

Try it: the fake agent in the example prints its environment, so you see exactly what it received. `HTTP_PROXY` was set in the shell and is gone; the references are resolved:

<!-- snippet: examples/launch-env/session.txt#launch -->
```sh
$ HTTP_PROXY=http://proxy.example:3128 agx run --profile work --env-file ./local.env --no-env HTTP_PROXY
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv
pwd ~/projects/app
env AGX_CONFIG_DIR=~/.agx
env AGX_RUN_AGENT=claude
env AGX_RUN_HOME=~/.claude-work
env AGX_RUN_ID=20261118-061119-ug6d
env AGX_RUN_LOGIN=home
env AGX_RUN_PROFILE=work
env API_KEY=fake-token-some-api-key
env CLAUDE_CONFIG_DIR=~/.claude-work
env COLUMNS=400
env DEBUG=1
env HOME=~
env LANG=C
env LOG_LEVEL=debug
env PATH=/usr/local/bin:/usr/bin:/bin
env PWD=~/projects/app
env REGION=eu-west-1
env SERVICE_TOKEN=fake-token-service-token
env TMPDIR=/tmp/agx-demo/tmp
env TZ=UTC
```

| Source | Meaning |
|---|---|
| profile `env` | literal values, never expanded |
| profile `env_refs` | references, expanded strictly |
| profile `env_file` | a dotenv file relative to the config dir; `$(…)` inside it is an error unless `"env_file_commands": true` |
| `--env NAME=value` | expanded strictly; a plain value is used as written |
| `--env-file PATH` | as `env_file`, relative to the current folder |
| `--no-env NAME` | removes the variable |

Values go into the agent's environment, never its command line. What output redacts goes by source, never by name: every value read from an env file (literal or not), every value from a reference (`env_refs`, an `--env` reference) and any value on an agent's credential variable are `[redacted]`; a literal `env` or `--env NAME=value` on any other variable is shown as written, even when the name looks secret (`--env NAME=plain-value` above). Keep a secret behind a reference so it stays hidden ([what is redacted](output.md#what-is-redacted)). The order, lowest to highest: inherited, credential scrub, profile `env_file`, `env`, `env_refs`, `--env-file`, `--env`, `--no-env`, then the login token and the home variable. Two consequences:

- `--env` cannot override the home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, …); use `--profile`. The same holds for any env source:

<!-- snippet: examples/launch-env/session.txt#home-var -->
```sh
$ agx run --profile work --env CLAUDE_CONFIG_DIR=/tmp/other --dry-run --quiet
agx: CLAUDE_CONFIG_DIR is set by the profile's home; use --profile
[exit 125]
```

- `--env` (or `--env-file`) of the token variable together with a stored login in the same command is refused as two choices for the credential:

<!-- snippet: examples/launch-env/session.txt#credential -->
```sh
$ agx run --profile work --login carol --env CLAUDE_CODE_OAUTH_TOKEN=x --dry-run --quiet
agx: --env CLAUDE_CODE_OAUTH_TOKEN and --login carol both set the credential; pick one
[exit 125]
```

### Strict expansion

Every reference agx expands is strict: the result never holds an unresolved reference, and no error contains a value.

| Case | Result |
|---|---|
| `$VAR` that is not set | an error naming the variable, not an empty string |
| an unknown scheme (`${vault:x}` without a vault plugin) | an error, never guessed |
| `$(…)` where commands are off (an `env_file` without `env_file_commands`) | an error naming the file, line and column |
| `${VAR:-default}`, `${VAR-default}`, `${VAR:+alt}`, `${VAR:?message}`, `$$` | allowed |
| a plain value without `$` | used as written |

<!-- snippet: examples/launch-env/session.txt#unset -->
```sh
$ agx run --profile work --env 'TOKEN=${NOT_SET_ANYWHERE}' --dry-run --quiet
agx: --env TOKEN: NOT_SET_ANYWHERE is not set
[exit 125]
```

<!-- snippet: examples/launch-env/session.txt#scheme -->
```sh
$ agx run --profile work --env 'TOKEN=${vault:x/y}' --dry-run --quiet
agx: --env TOKEN: no plugin for scheme vault
[exit 125]
```

<!-- snippet: examples/launch-env/session.txt#defaults -->
```sh
$ agx run --profile work --env 'TOKEN=${NOT_SET_ANYWHERE:-fallback}' --dry-run --quiet --json --field data.command.line
claude
```

A dotenv file may run commands only when the profile allows it:

<!-- snippet: examples/launch-env/home/.agx/envs/cmds.env -->
```sh
BUILD_SHA=$(echo abc123)
```

<!-- snippet: examples/launch-env/session.txt#commands-off -->
```sh
$ agx run --profile nocmds --dry-run --quiet
agx: envs/cmds.env:2:11: $(echo abc123) is a command, and env_file_commands is off
[exit 125]
```

<!-- snippet: examples/launch-env/session.txt#commands -->
```sh
$ agx run --profile builds --dry-run --quiet --resolved-secrets
...
env      BUILD_SHA                              from profile builds env_file envs/cmds.env
...
env      set BUILD_SHA=[redacted]               from launch environment
         set AGX_RUN_ID=20261118-061119-p8oh    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=builds             from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude             from launch environment
...
```

A plain `--dry-run` checks every reference (unset names, unknown schemes, commands where they are off) but runs nothing, so a command that would fail still passes. `--resolved-secrets` runs them (it may run commands and call gopass) and stops on the first failure. It only works with `--dry-run`: without it agx refuses (`--resolved-secrets only works with --dry-run; add --dry-run`, exit 125) and starts nothing:

<!-- snippet: examples/launch-env/session.txt#resolved-error -->
```sh
$ agx run --profile work --env 'TOKEN=$(exit 3)' --dry-run --quiet --json --field data.command.line
claude
$ agx run --profile work --env 'TOKEN=$(exit 3)' --dry-run --resolved-secrets --quiet
agx: --env TOKEN: command "exit 3" failed: exit 3
[exit 125]
$ agx run --profile work --resolved-secrets
agx: --resolved-secrets only works with --dry-run; add --dry-run
[exit 125]
```

## Recipe 6: model, prompt, folders and permission level

These are the options every agent understands, mapped per agent by agx:

| Flag | Profile key | Meaning |
|---|---|---|
| `--model` | `model` | the model |
| `--prompt` | `claude.prompt`, `codex.prompt` | the first prompt |
| `--add-dir` | `claude.add_dirs`, `codex.add_dirs` | an extra folder the agent may access (repeat) |
| `--autonomy` | `autonomy` | `default`, `read-only`, `ask`, `accept-edits` or `full` |
| `--consent full-autonomy` | | required with `--autonomy full` |
| `--headless` | | a machine-readable event stream instead of the interactive UI |

The command line beats the profile ([`examples/launch-options`](../examples/launch-options/README.md)): here `--model opus` replaces the profile's `"model": "sonnet"`, `--add-dir` adds a folder, and the prompt goes last, after `--`, so a prompt that starts with `-` is never read as a flag:

<!-- snippet: examples/launch-options/session.txt#model-add-dir -->
```sh
$ agx run --profile work --model opus --add-dir ~/projects/client --prompt 'Fix the failing test' --dry-run --quiet --json --field data.command.line
claude --model opus --effort high --add-dir ~/projects/client --verbose -- "Fix the failing test"
```

An agent is never run with more autonomy than you asked for. A level an agent cannot honour exactly is refused, not rounded up: Codex has no edits-only mode, so `--autonomy accept-edits` is refused for Codex. pi has no permission modes at all: it runs with a one-line warning that it never asks, `--autonomy read-only` gives it a read-only tools allowlist, and `ask` or `accept-edits` are refused. Every level per agent is in [Autonomy](guides/autonomy.md).

<!-- runnable: fixture=recipes -->
```sh
$ agx run --profile pi --autonomy read-only --dry-run --quiet --json --field data.command.line
pi --tools read,grep,find,ls --no-mcp --no-extensions
$ agx run --profile work --autonomy full --consent full-autonomy --dry-run --quiet --json --field data.command.line
claude --effort high --dangerously-skip-permissions --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json
```

<!-- runnable: fixture=recipes exit=125 -->
```sh
$ agx run --profile pi --autonomy ask --dry-run --quiet
agx: pi: autonomy ask: pi has no permission modes; it always runs without prompts
```

`--prompt` together with `--headless` runs the agent once without its interactive UI, for scripts and CI: see [Headless runs](guides/headless.md).

## Recipe 7: typed vendor launch flags

Problem: you want Claude's `--effort high` on every work launch, checked against the installed Claude version, and sometimes `max` for one run.

Step 1: put the flags in the profile's agent block (`claude` on a Claude profile, `codex` on a Codex profile):

<!-- snippet: examples/launch-options/config.jsonc#work -->
```jsonc
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      "default": true,
      "model": "sonnet",
      "claude": {
        "effort": "high",
        "extra_args": ["--verbose"]
      }
    },
```

<!-- snippet: examples/launch-options/session.txt#profile-block -->
```sh
$ agx run --profile work --dry-run --quiet
...
options  model=sonnet                                   from profile work
         effort=high                                    from profile work
         extra_args=["--verbose"]                       from profile work
...
command  claude --model sonnet --effort high --verbose  from built by agentio
...
```

Step 2: override one for a run with `--option key=value`. The dry run names what it replaced:

<!-- snippet: examples/launch-options/session.txt#override -->
```sh
$ agx run --profile work --model opus --option effort=max --dry-run --quiet
...
options  model=opus                                  from --model
         effort=max                                  from --option (overrides profile work: high)
         extra_args=["--verbose"]                    from profile work
...
command  claude --model opus --effort max --verbose  from built by agentio
...
```

The keys come from the agent's own option registry (every key is in the [config reference](config.md#agent-blocks)), so an unknown key fails and lists the valid ones (the list is cut short here), and an option the installed version lacks is refused with the version it needs:

```sh
$ agx run --profile work --option efort=max --dry-run --quiet
agx: --option efort: unknown key (did you mean effort?); valid keys: prompt, print, output_format, …
```

`settings` is not among the keys: it is agx's own (recipe 2), so `--option settings=…` is refused with `--option settings: settings is not a launch option; use --setting KEY=VALUE, or "settings" in the profile's claude block (a JSONC file or an object)` (exit 125). A `claude` block on a Codex profile fails the config load. Codex's own `profile` option is named `codex_profile`, so it never reads as an agx profile (recipe 9).

### Block keys with no flag of their own: `prompt` and `extra_args`

Two keys of the `claude` block are not vendor flags:

- `prompt` is a first prompt for every launch of the profile; `--prompt` replaces it for one run.

<!-- snippet: examples/launch-options/config.jsonc#review -->
```jsonc
    "review": {
      "agent": "claude",
      "home": "~/.claude",
      "claude": { "prompt": "Review the open diff" }
    }
```

<!-- snippet: examples/launch-options/session.txt#block-prompt -->
```sh
$ agx run --profile review --dry-run --quiet --json --field data.command.line
claude -- "Review the open diff"
$ agx run --profile review --prompt 'Summarise the README' --dry-run --quiet --json --field data.command.line
claude -- "Summarise the README"
```

- `extra_args` are raw args kept in the profile's block. With other raw args they combine in this order: `extra_args` first, then `agents.claude.defaults.args`, profile `args` and args after `--`, so the later one wins when they repeat a flag (see [Raw args](guides/raw-args.md)):

<!-- snippet: examples/launch-options/session.txt#extra-args-raw -->
```sh
$ agx run --profile work --dry-run --quiet -- --debug
...
args     --debug                                                from after --
options  effort=high                                            from profile work
         extra_args=["--verbose"]                               from profile work
...
command  claude --model sonnet --effort high --verbose --debug  from built by agentio
...
```

Path values in an agent block get `~` expanded: no shell runs between agx and the agent, so agx expands a leading `~` (alone or `~/…`) in every option agentio marks as a path, as a string or in each element of a list. Claude Code: `add_dirs`, `mcp_configs`, `system_prompt_file`, `append_system_prompt_file`, `plugin_dirs`, `debug_file`; Codex: `cd`, `add_dirs`, `images`. The same holds for `--option`. Other values (`system_prompt`, `files`, `resume`, inline JSON) and `~user` or `$VAR` stay as written, and the dry run's `options` line shows the value as you wrote it. (agx reads the `settings` file itself and expands `~` there too.)

## Recipe 8: fully raw (passed as written, a raw `--settings` merged)

<a id="recipe-8-fully-raw-always-works-never-refused"></a>

```sh
agx run --profile work -- --some-brand-new-flag value
```

<!-- runnable: fixture=recipes -->
```sh
$ agx run --profile work --dry-run --quiet --json --field data.command.line -- --effort max --verbose
claude --effort high --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json --effort max --verbose
```

Everything after `--` is appended exactly as given, last. Profile `args` and `agents.<agent>.defaults.args` come just before it, as written, except that `~` is expanded in the value of a path option's flag (`--debug-file ~/d.log`, `--plugin-dir=~/plug`; see [Raw args](guides/raw-args.md#pitfalls)). `--dry-run` notes when a raw arg duplicates a typed choice; the agent uses the later one, which is the raw one. The one raw flag agx does not pass as written is the agent's settings flag (Claude `--settings`): it is merged with agx's own settings, below. A Claude block's `extra_args` go before all raw args. Per-agent and per-profile args, `inherit_defaults` and `agx profile edit --args` are in [Raw args](guides/raw-args.md).

### A raw `--settings` of your own

Problem: a team settings file or a one-off settings value should sit on top of what the profile and `--setting` already set. Claude Code keeps only the last `--settings` it is given, wholesale (an earlier one is dropped, not merged), so passing yours next to agx's would silently drop agx's settings, including a [live-login](guides/live-login.md) helper.

What agx does with every raw `--settings VALUE` or `--settings=VALUE` (after `--`, in profile `args`, in `agents.claude.defaults.args`, or in the block's `extra_args`):

1. It reads the value. A value that is a JSON object (it starts with `{` and ends with `}`) is inline JSON; anything else is a file path, relative to the launch folder (where Claude would have looked), with a leading `~` as your home. Comments and trailing commas are allowed in both.
2. It merges the value over its own settings (the profile's `settings`, then `--setting`) key by key: a key in your value wins; where both hold an object, the two objects merge the same way; an array, a string, a number, `true`/`false` or `null` replaces the whole value (so your `permissions.allow` list is the whole list).
3. Two or more raw values merge in argv order (`extra_args`, agent defaults, profile `args`, after `--`), the later one winning key by key. This differs from Claude's own last-one-wins-wholesale on purpose: nothing you passed is dropped silently.
4. It takes every raw settings flag out of the args and passes exactly one `--settings`, delivered by `settings_delivery` (a file in the run folder, or inline). This also holds when agx has no settings of its own: the value is still read, checked and passed as one file agx wrote, so the rule never depends on what else the launch sets.

The dry run names each raw key `from raw --settings` (with the file, if any) and the agx keys it overrode:

<!-- snippet: examples/launch-raw-settings/session.txt#inline -->
```sh
$ agx run --dry-run --quiet -- --settings '{"cleanupPeriodDays":7,"permissions":{"deny":["Bash"]}}'
agent     claude                                                                                        from default_agent
profile   work                                                                                          from default profile of claude
home      ~/.claude-work                                                                                from profile work
login     home login                                                                                    from default (no login named)
binary    /usr/local/bin/claude version unknown                                                         from no install metadata
folder    ~/projects/app                                                                                from current folder
settings  permissions.allow                                                                             from profile work
          cleanupPeriodDays, permissions.deny                                                           from raw --settings (overrides profile work: cleanupPeriodDays)
          delivered as file ~/.agx/runs/20261118-091339-izjf/settings-20261118-091339-izjf.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261118-091339-izjf                                                              from AGX_RUN_ID=20261118-091339-izjf
command   claude --settings ~/.agx/runs/20261118-091339-izjf/settings-20261118-091339-izjf.json         from built by agentio
env       set AGX_RUN_ID=20261118-091339-izjf                                                           from launch environment
          set AGX_RUN_AGENT=claude                                                                      from launch environment
          set AGX_RUN_PROFILE=work                                                                      from launch environment
          set AGX_RUN_LOGIN=home                                                                        from launch environment
          set AGX_RUN_HOME=~/.claude-work                                                               from launch environment
          set CLAUDE_CONFIG_DIR=~/.claude-work                                                          from launch environment
note      installed Claude Code version unknown; flags not checked
```

What Claude gets (inline delivery shows it): `permissions` keeps the profile's `allow` and gains your `deny`:

<!-- snippet: examples/launch-raw-settings/session.txt#merged -->
```sh
$ agx run --dry-run --quiet --settings-delivery inline --json --field data.command.line -- --settings '{"cleanupPeriodDays":7,"permissions":{"deny":["Bash"]}}'
claude --settings "{\"cleanupPeriodDays\":7,\"permissions\":{\"allow\":[\"Read\"],\"deny\":[\"Bash\"]}}"
```

Raw settings keys still follow [one winner per concern](#one-winner-per-concern) as settings keys from the profile side: `--model` beats a raw `model` with a note, and a raw `apiKeyHelper` replaces a profile login, as a profile's would. With `--live-login`, a raw value is merged with the helper agx adds; a raw `apiKeyHelper` there is two credential helpers in one command and is refused:

<!-- snippet: examples/launch-raw-settings/session.txt#live -->
```sh
$ agx run --profile live --dry-run --quiet -- --settings '{"theme":"dark"}'
...
settings  theme                                                                                         from raw --settings
          apiKeyHelper                                                                                  from --live-login
...
$ agx run --profile live --dry-run --quiet -- --settings '{"apiKeyHelper":"~/bin/key.sh"}'
agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one
[exit 125]
```

A file that is missing, unreadable or not valid JSONC stops the launch and names it (with line:col for a syntax error):

<!-- snippet: examples/launch-raw-settings/session.txt#errors -->
```sh
$ agx run --dry-run --quiet -- --settings missing.json
agx: raw --settings file ~/projects/app/missing.json not found
[exit 125]
$ agx run --dry-run --quiet -- --settings broken.jsonc
agx: raw --settings: ~/projects/app/broken.jsonc:2:12: unexpected ',': expected a value
[exit 125]
$ agx run --dry-run --quiet -- --settings
agx: raw --settings needs a value (inline JSON or a settings file)
[exit 125]
```

More (a file in profile `args`, two values, `--settings=FILE`) in [launch-raw-settings](../examples/launch-raw-settings/README.md).

## Recipe 9: Codex

Problem: you want Codex's reasoning effort, sandbox and its own config profile set per agx profile.

Step 1: a `codex` block. `settings` go as `-c key=value`; `codex_profile` is Codex's own `-p` ([`examples/launch-codex`](../examples/launch-codex/README.md)):

<!-- snippet: examples/launch-codex/config.jsonc#deep -->
```jsonc
    "deep": {
      "agent": "codex",
      "home": "~/.codex",
      "codex": {
        "codex_profile": "deep",                     // Codex's own -p profile, not an agx profile
        "search": true,
        "settings": { "model_reasoning_effort": "high" }
      }
    }
```

<!-- snippet: examples/launch-codex/session.txt#block -->
```sh
$ agx run --profile deep --dry-run --quiet
...
options   codex_profile=deep                                           from profile deep
          search=true                                                  from profile deep
settings  model_reasoning_effort                                       from profile deep
...
command   codex -c "model_reasoning_effort=\"high\"" -p deep --search  from built by agentio
...
note      installed codex version unknown; flags not checked against it
```

Step 2: for one run, `--setting` becomes `-c path=value` with Codex's TOML quoting, `--option` takes the block keys, and `--autonomy ask` maps to Codex's own sandbox and approval flags (`-s workspace-write -a on-request`):

<!-- snippet: examples/launch-codex/session.txt#setting -->
```sh
$ agx run --profile codex --model gpt-5 --autonomy ask --setting model_reasoning_effort=high --dry-run --quiet --json --field data.command.line
codex -c "model_reasoning_effort=\"high\"" -m gpt-5 -s workspace-write -a on-request
```

<!-- snippet: examples/launch-codex/session.txt#option -->
```sh
$ agx run --profile codex --option sandbox=workspace-write --dry-run --quiet --json --field data.command.line -- --search
codex -s workspace-write --search
```

- Known settings keys are type-checked and option values are checked against their allowed values; both are refused with the reason:

<!-- snippet: examples/launch-codex/session.txt#bad-value -->
```sh
$ agx run --profile codex --option sandbox=everything --dry-run --quiet
agx: --option sandbox: invalid launch option: sandbox="everything": one of read-only, workspace-write, danger-full-access
[exit 125]
$ agx run --profile codex --setting model_reasoning_effort=12 --dry-run --quiet
agx: --setting model_reasoning_effort: expected string, found integer
[exit 125]
```

- An unknown settings key passes with a note:

<!-- snippet: examples/launch-codex/session.txt#unknown-setting -->
```sh
$ agx run --profile codex --setting my_new_key=on --dry-run --quiet
...
settings  my_new_key                            from --setting
...
command   codex -c "my_new_key=\"on\""          from built by agentio
...
note      --setting my_new_key: not modelled
          installed codex version unknown; flags not checked against it
```

- Codex has no token login, so a stored login or `--token-ref` is refused and a Codex run always uses its home's own login:

<!-- snippet: examples/launch-codex/session.txt#no-token-login -->
```sh
$ agx run --profile codex --login carol --dry-run --quiet
agx: login carol serves claude; this run is codex
[exit 125]
$ agx run --profile codex --token-ref plain-value --dry-run --quiet
agx: codex has no token login (Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)); log in inside the home instead
[exit 125]
```

## Recipe 10: Gemini, pi and custom agents

| Pass | Gemini | pi | custom |
|---|---|---|---|
| `--model`, `--prompt`, `--add-dir`, `--headless` | yes | `--model`, `--headless` | per the agent's spec (`model_args`, `prompt_args`, `add_dir_args`, `headless_args`) |
| `--autonomy` | every level (approval modes) | `read-only` and `full` only | per `autonomy_args`; a level not listed is refused |
| a login or `--token-ref` | yes (`GEMINI_API_KEY`) | yes (the provider's variable) | with `token_env`: the variable it names for the login's kind (and provider) |
| `--setting`, `--option` | no settings flag: refused | refused | refused |
| `--env`, `--no-env`, `--env-file` | yes | yes | yes |
| raw after `--` | yes | yes | yes |

Gemini maps the common options to its own flags, and an API-key login goes into `GEMINI_API_KEY` ([`examples/launch-gemini-pi`](../examples/launch-gemini-pi/README.md)):

<!-- snippet: examples/launch-gemini-pi/session.txt#gemini -->
```sh
$ agx run --profile gemini --model gemini-2.5-pro --add-dir ~/projects/client --prompt 'Explain the build' --autonomy accept-edits --dry-run --quiet --json --field data.command.line
...
$ agx run --profile gemini --login gem --dry-run --quiet
...
login    gem (not resolved: --dry-run)          from --login
...
env      set AGX_RUN_ID=20261118-061250-ipx5    from launch environment
         set AGX_RUN_AGENT=gemini               from launch environment
         set AGX_RUN_PROFILE=gemini             from launch environment
         set AGX_RUN_LOGIN=gem                  from launch environment
         set AGX_RUN_HOME=~/.gemini             from launch environment
         set GEMINI_API_KEY=[redacted]          from launch environment
```

pi gets `--model`, a read-only tools allowlist for `--autonomy read-only`, and a warning on every launch that it never asks:

<!-- snippet: examples/launch-gemini-pi/session.txt#pi -->
```sh
$ agx run --profile pi --model sonnet --autonomy read-only --dry-run --quiet
...
options  model=sonnet                                                          from --model
         autonomy=read-only                                                    from --autonomy
...
command  pi --model sonnet --tools read,grep,find,ls --no-mcp --no-extensions  from built by agentio
...
$ agx run --profile pi
agx: warning: pi runs with full autonomy and never asks
...
$ agx run --profile pi --autonomy ask --dry-run --quiet
agx: pi: autonomy ask: pi has no permission modes; it always runs without prompts
[exit 125]
```

Neither has a settings flag or typed options, so `--setting` and `--option` are refused; pass their flags after `--`:

<!-- snippet: examples/launch-gemini-pi/session.txt#no-setting -->
```sh
$ agx run --profile gemini --setting theme=dark --dry-run --quiet
agx: gemini has no --option or --setting (Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args); pass its flags after --
[exit 125]
$ agx run --profile pi --setting theme=dark --dry-run --quiet
agx: pi has no --option or --setting (pi has no settings flag and no typed launch options in agentio; pass flags as raw args); pass its flags after --
[exit 125]
```

<!-- snippet: examples/launch-gemini-pi/session.txt#raw -->
```sh
$ agx run --profile gemini --dry-run --quiet --json --field data.command.line -- --sandbox
gemini --sandbox
```

A custom agent gets exactly what its `spec` templates say; define one in the config (see [A custom agent](config.md#a-custom-agent)).

## A script before the agent: `before_run`

Problem: one account needs a VPN check or a token refresh before every launch.

Step 1: write the script. Its output is discarded; a non-zero exit stops the launch ([`examples/launch-before-run`](../examples/launch-before-run/README.md)):

<!-- snippet: examples/launch-before-run/home/.agx/scripts/work-env.sh#work-env -->
```sh
# Runs before every launch of the work profile. Its output is discarded;
# a non-zero exit stops the launch.
test -d "$HOME/.claude-work" || { echo "work home missing" >&2; exit 1; }
```

Step 2: give the profile the script. The path is relative to the config dir; `"before_run_from": "folder"` finds it relative to the launch folder instead (an opt-in, so a repository's scripts only run when you allow it):

<!-- snippet: examples/launch-before-run/config.jsonc#profiles -->
```jsonc
    "work": {
      "agent": "claude",
      "home": "~/.claude-work",
      "default": true,
      "before_run": "scripts/work-env.sh"            // relative to the config dir (~/.agx)
    },
    "repo": {
      "agent": "claude",
      "home": "~/.claude",
      "before_run": "scripts/setup.sh",
      "before_run_from": "folder"                    // relative to the launch folder instead
    },
```

Try it: `--dry-run` shows the resolved path and never runs the script; a real launch runs it, then the agent:

<!-- snippet: examples/launch-before-run/session.txt#dry-run -->
```sh
$ agx run --profile work --dry-run --quiet
...
before_run  ~/.agx/scripts/work-env.sh             from profile work (before_run_from config)
...
command     claude                                 from built by agentio
...
```

<!-- snippet: examples/launch-before-run/session.txt#launch -->
```sh
$ agx run --profile work --prompt hello
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [--] [hello]
pwd ~/projects/app
...
```

For one run, `--before-run PATH` (relative to the current folder) replaces the profile's script. A failure stops the launch with exit 125, its exit code and the first line it wrote to stderr:

<!-- snippet: examples/launch-before-run/session.txt#one-run -->
```sh
$ agx run --profile work --before-run ~/.agx/scripts/vpn-check.sh --dry-run --quiet
...
before_run  ~/.agx/scripts/vpn-check.sh            from --before-run (overrides profile work: scripts/work-env.sh)
...
$ agx run --profile work --before-run ~/.agx/scripts/vpn-check.sh
agx: warning: installed Claude Code version unknown; flags not checked
agx: before_run failed (exit 2): VPN is down; connect first
[exit 125]
```

- It runs in the launch folder, with a 60 second limit and stdin closed.
- A script that is not found refuses the launch, in a dry run too. With `before_run_from: folder` it is looked up in each launch folder, so a folder without it refuses:

<!-- snippet: examples/launch-before-run/session.txt#from-folder -->
```sh
$ agx run --profile repo
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv
pwd ~/projects/app
...
$ agx run --profile repo --dir ~/projects/client --dry-run --quiet
agx: before_run scripts/setup.sh not found in ~/projects/client
[exit 125]
```

- It sees the launch environment without the credential variables (the login token and the agent's credential variables) unless `"before_run_env": "full"`:

<!-- snippet: examples/launch-before-run/config.jsonc#env -->
```jsonc
    "hidden": {
      "agent": "claude",
      "home": "~/.claude",
      "login": "carol",
      "before_run": "scripts/show-token.sh"          // credential variables removed (the default)
    },
    "full": {
      "agent": "claude",
      "home": "~/.claude",
      "login": "carol",
      "before_run": "scripts/show-token.sh",
      "before_run_env": "full"                       // the script sees the token too
    }
```

<!-- snippet: examples/launch-before-run/home/.agx/scripts/show-token.sh#show-token -->
```sh
# Says whether the login token reached the script, then stops the launch.
if [ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]; then seen=visible; else seen=hidden; fi
echo "token $seen" >&2
exit 1
```

<!-- snippet: examples/launch-before-run/session.txt#env -->
```sh
$ agx run --profile hidden
agx: warning: installed Claude Code version unknown; flags not checked
agx: before_run failed (exit 1): token hidden
[exit 125]
$ agx run --profile full
agx: warning: installed Claude Code version unknown; flags not checked
agx: before_run failed (exit 1): token visible
[exit 125]
```

## One winner per concern

| Concern | Command line | Profile | Raw |
|---|---|---|---|
| model | `--model`, `--setting model`, `--option model` | `model`, `claude.settings.model` | `-- --model x` |
| permission level | `--autonomy`, `--option permission_mode`, `--setting permissions.defaultMode` | `autonomy`, block keys, settings keys | `-- --dangerously-skip-permissions` |
| credential | `--login`, `--home-login`, `--auto-login`, `--token-ref`, `--setting apiKeyHelper` | `login`, agent default login, `AGX_LOGIN` | `-- --settings '{"apiKeyHelper":…}'` |
| settings | `--setting` | `claude.settings` | `-- --settings …` (merged key by key, recipe 8) |
| headless output | `--headless`, `--option print`, `--option output_format` | `claude.print`, `claude.output_format` | `-- -p` |
| environment | `--env`, `--no-env`, `--env-file` | `env`, `env_refs`, `env_file` | none |

1. Raw always wins; a Claude block's `extra_args` combine with it, placed before the other raw args (recipe 7). A raw `--settings` is merged instead of passed (recipe 8): its keys win over agx's settings key by key and then count as settings keys from the profile side, so a command-line `--model` or `--login` still beats a raw `model` or `apiKeyHelper` (with a note); the one refusal is a raw `apiKeyHelper` with `--live-login`.
2. The command line beats the profile; `--dry-run` shows the source of each choice and, for `--option`, the profile value it overrode (recipe 7).
3. The only refusal is two command-line choices for one concern in one command. The message names both:

<!-- snippet: examples/launch-options/session.txt#conflict -->
```sh
$ agx run --profile work --model opus --setting model=sonnet --dry-run --quiet
agx: --model and --setting model both set the model; pick one
[exit 125]
$ agx run --profile work --model opus --option model=sonnet --dry-run --quiet
agx: --model and --option model both set the model; pick one
[exit 125]
```

4. Keys of one concern from one input are one choice: kept or dropped together, named together in one note, and checked together by the agent's rules. A profile block `{ "print": true, "output_format": "json" }` launches `claude -p --output-format json`, and so does `--option print=true --option output_format=json`; a contradiction inside one input (`resume` and `continue` in one block) is refused with the agent's reason (exit 125).
5. A settings key that replaces an agx mechanism is allowed, with a note. For example `apiKeyHelper` replaces the login token (recipe 3).

## Why none of this needs shell quoting

agx builds the agent's command as a list of arguments and starts it directly, with no shell in between. A JSON string, a path with spaces or a value with quotes arrives as exactly one argument. The command line `--dry-run` prints is quoted for display only. The fake agent of the example prints each argument it received in brackets:

<!-- snippet: examples/launch-options/session.txt#no-quoting -->
```sh
$ agx run --profile review --setting 'permissions.allow=["Bash(git status)","Read"]' --settings-delivery inline
agx: warning: settings are visible in ps (inline delivery, from --settings-delivery)
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [--settings] [{"permissions":{"allow":["Bash(git status)","Read"]}}] [--] [Review the open diff]
pwd ~/projects/app
...
```

The flip side: no shell expands `~`, `$VAR` or globs in values agx passes through. agx itself expands a leading `~` only in path options (the block, `--option`, and after a path option's flag in config args, recipe 7); everything else, `$VAR` and globs included, arrives as written. Your own shell still expands what you type on the command line.

## Pitfalls

| Symptom | Cause and fix |
|---|---|
| `agx: --model and --setting model both set the model; pick one` (exit 125) | two command-line choices for one concern; drop one |
| `agx: CLAUDE_CONFIG_DIR is set by the profile's home; use --profile` (exit 125) | an env source sets the home variable; choose the home with a profile |
| `agx: --env CLAUDE_CODE_OAUTH_TOKEN and --login carol both set the credential; pick one` (exit 125) | a credential variable on the command line next to a stored login |
| `agx: --env TOKEN: NOT_SET_ANYWHERE is not set` (exit 125) | strict expansion; set the variable or write `${NAME:-default}` |
| `agx: envs/cmds.env:2:11: $(echo abc123) is a command, and env_file_commands is off` (exit 125) | add `"env_file_commands": true` to the profile, or remove the command |
| `agx: profile missing: settings file ~/.config/claude/nope.json not found` (exit 125) | the profile's `settings` path; it is relative to the config dir |
| `agx: --setting cleanupPeriodDays: expected int` (exit 125) | a known settings key with the wrong type |
| `agx: config.jsonc:32:86: profile typo settings cleanupPeriodDays: expected int` (exit 125) | the same in a profile's `settings`; the position is the key's in `config.jsonc`, or in the settings file the profile names |
| `agx: --option settings: settings is not a launch option; use --setting KEY=VALUE, …` (exit 125) | `settings` is agx's own key: use `--setting`, the block's `settings`, or a raw `-- --settings FILE` (merged with agx's) |
| `agx: raw --settings file ~/projects/app/missing.json not found` (exit 125) | a raw `--settings` path is relative to the launch folder; agx reads it itself (recipe 8) |
| `agx: raw --settings: ~/projects/app/broken.jsonc:2:12: unexpected ','…` (exit 125) | the raw settings file is not valid JSONC; fix it at that line and column |
| `agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one` (exit 125) | live login brings its own helper; drop `apiKeyHelper` from your settings, or drop `--live-login` |
| `agx: --option efort: unknown key (did you mean effort?); valid keys: …` (exit 125) | a typo in an option key |
| `agx: pi has no --option or --setting (…); pass its flags after --` (exit 125) | the agent has no settings flag; use raw args |
| `agx: codex has no token login (…); log in inside the home instead` (exit 125) | Codex always runs on its home's login |
| `agx: before_run failed (exit 2): VPN is down; connect first` (exit 125) | the script failed; its first stderr line is shown |
| `agx: before_run scripts/setup.sh not found in ~/projects/client` (exit 125) | the script path does not resolve (config dir, or launch folder with `before_run_from: folder`) |
| a path with `~` is not found by the agent | agx expands `~` only in options agentio marks as paths (recipe 7) and after their flags in config args; elsewhere (another option, args after `--` in quotes, Gemini CLI, pi, a custom agent) write an absolute path |
| `--dry-run` passes but the launch fails on a reference | a plain dry run checks references without running them; add `--resolved-secrets` |

## Related

- [`agx run`](commands/launch.md#agx-run): every flag of the launching commands.
- [How agx decides](how-agx-decides.md): where the agent, profile, home and login come from.
- [Autonomy](guides/autonomy.md), [Raw args](guides/raw-args.md), [Headless runs](guides/headless.md), [Run folders](guides/run-folders.md).
- [Logins](logins.md): every credential kind.
- [Config reference](config.md#agent-blocks): every key of the `claude` and `codex` blocks.
- Examples: [launch-settings](../examples/launch-settings/README.md), [launch-raw-settings](../examples/launch-raw-settings/README.md), [launch-env](../examples/launch-env/README.md), [launch-before-run](../examples/launch-before-run/README.md), [launch-options](../examples/launch-options/README.md), [launch-codex](../examples/launch-codex/README.md), [launch-gemini-pi](../examples/launch-gemini-pi/README.md).
