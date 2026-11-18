# Output, `--field` and exit codes

Every agx command prints a human table or text by default, one JSON shape with `--json`, and exit codes a script can rely on. This page is the reference; [Scripting with agx](guides/scripting.md) walks through using it step by step, and [`examples/out-scripting`](../examples/out-scripting/README.md) runs every case.

## The JSON envelope

Every command prints the same envelope with `--json`:

```json
{
  "schema_version": 1,
  "command": "profile list",
  "ok": true,
  "data": [],
  "decisions": [],
  "warnings": [],
  "error": null
}
```

| Key | Meaning |
|---|---|
| `schema_version` | bumped only on a breaking change to the envelope or to any command's `data` |
| `command` | the command path, such as `profile list` |
| `ok` | false on failure |
| `data` | the result; null on failure. Each command's shape is pinned by a test |
| `decisions` | on launching commands and `project show`: each decision's name, value and source |
| `warnings` | each warning's code and message |
| `error` | on failure: code, message and hint |

A failure has the same shape, with `ok` false and `error` set:

<!-- runnable: exit=125 -->
```sh
$ agx run --login carol --home-login --json
{
  "schema_version": 1,
  "command": "run",
  "ok": false,
  "data": null,
  "decisions": [],
  "warnings": [],
  "error": {
    "code": "conflict",
    "message": "--login and --home-login both choose the login; pick one"
  }
}
```

`error.code` is stable: match on it, not on the message.

## `--field`: keep only what you need

Problem: you want one value in a script without installing jq.

Fix: `--field PATH` selects from the whole envelope. Dots go down a level, numbers index a list, and `*` is every item: `data`, `data.0.name`, `data.*.home`, `error.code`.

- One `--field` prints the value bare: a string as text, anything else as compact JSON.
- Several print one object keyed by path, in the order you gave them.
- On a human table, `--field data.*.home --field data.*.name` picks and orders the columns.
- A missing path is an error that names it, with a "did you mean".
- Exit codes still signal failure.

<!-- runnable -->
```sh
$ agx run --profile work --dry-run --quiet --json --field data.agent
claude
$ agx run --profile work --dry-run --quiet --json --field data.command.args
["--chrome"]
```

<!-- runnable: exit=125 -->
```sh
$ agx run --dry-run --quiet --json --field data.homee
agx: --field data.homee: no such path (did you mean data.home?)
```

There is no query language beyond this; pipe `--json` to jq for more. The `--field` paths of each command's data are listed by `agx help --json`.

## Notes and warnings

When something other than a flag decided (a folder binding, the shared-home rule, `AGX_LOGIN`, `default_agent`), agx prints one note on stderr, such as `agx: profile work (folder binding ~/projects/client)`; [notes on stderr](how-agx-decides.md#notes-on-stderr) lists them all. Warnings (an unknown settings key, a version newer than tested) go to stderr as `agx: warning: …` lines in text mode, and to `warnings` in the envelope with `--json` (not to stderr then). `--quiet` turns off the notes and, in text mode, the warning lines; the envelope keeps its `warnings`. stdout is never mixed with either.

<!-- snippet: examples/out-scripting/session.txt#notes -->
```sh
$ agx run --dry-run --json --field data.profile
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
work

$ agx run --dry-run --quiet --json --field data.profile
work
```

Secrets are never printed: a token value is `[redacted]` in text, JSON, errors and logs, plain or resolved. The one exception is [`agx login token`](commands/login.md#agx-login-token), which prints a login's token when you ask for it.

<!-- snippet: examples/out-scripting/session.txt#redacted -->
```sh
$ agx run --dry-run --quiet --resolved-secrets --json --field data.command.env
["set AGX_RUN_ID=20261118-061527-ppg6","set AGX_RUN_AGENT=claude","set AGX_RUN_PROFILE=work","set AGX_RUN_LOGIN=carol","set AGX_RUN_HOME=~/.claude-work","set CLAUDE_CODE_OAUTH_TOKEN=[redacted]","set CLAUDE_CONFIG_DIR=~/.claude-work"]

$ agx login show --id carol
name     carol
agents   claude
kind     oauth-token
source   plain
account  —
users    profile work
```

### What is redacted

agx decides what is secret by where a value came from and by the value itself, never by a variable's name. In a launch's output (the `--dry-run` view, its decisions and warnings, `--json`), `[redacted]` replaces:

- a login's token, plain or a reference, from `config.jsonc` or `tokens.jsonc`, and a one-off `--token-ref`;
- every value from a reference (`env_refs`, an `--env` whose value is a reference) and every value from an env file (`env_file`, `--env-file`), literal or not;
- any value on a variable an agent reads a credential from (`CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, pi's provider variables, a custom agent's `token_env`), whatever its source;
- that same value anywhere else: in another variable, an argument or a decision (inside longer text from 8 characters on).

A literal profile `env` value or `--env NAME=value` on any other variable is shown as written, even when its name looks secret: `MY_SERVICE_TOKEN=abc123` prints as is, and so does a number such as `CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000`. Keep a real secret in `env_refs` (or a login) and it stays redacted; `agx doctor` flags a literal profile `env` value whose name looks secret: `profiles.work.env.MY_SERVICE_TOKEN looks like a secret written as a literal: shown as written in output; put it in env_refs to keep it redacted`. Config views are stricter: `agx config show` and `agx config set` never print a login token or any profile `env` value.

<!-- snippet: examples/launch-env/session.txt#redaction -->
```sh
$ agx run --profile service --env 'EXTRA=${gopass:work/extra}' --env ANTHROPIC_API_KEY=plain-key-value --dry-run --quiet --json --field data.command.env
["set MY_SERVICE_TOKEN=literal-abc123","set SERVICE_TIMEOUT_MS=3000","set EXTRA=[redacted]","set ANTHROPIC_API_KEY=[redacted]","set AGX_RUN_ID=20261118-074332-qmd1","set AGX_RUN_AGENT=claude","set AGX_RUN_PROFILE=service","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.claude"]
$ agx doctor --agent claude --field data.*.status --field data.*.area --field data.*.message
STATUS  AREA         MESSAGE
...
info    plain-token  profiles.service.env.MY_SERVICE_TOKEN looks like a secret written as a literal: shown as written in output; put it in env_refs to keep it redacted
...
```

## Line width, colour and timeouts

- **Width.** Human tables wrap their last column (doctor's `MESSAGE`, `config show`'s value) to the line width: the terminal's when stdout is one, else `COLUMNS`, else 120. Below 60 agx uses 60. `--json` never wraps.
- **Colour.** The human output looks as agx v0.3's did: table headers and section titles bold, `#` indices, placeholders (`—`) and sources dim, and statuses coloured by what they mean: blue for fine (`ok`, a passed check, plenty left), amber for worth a look (`expired`, `stale`, under 20% left, a usage bar from 80%), red for blocked or failed (`limit reached`, `rejected`, a failed check, 0% left). On stderr the error line `agx: …` is red and each `agx: warning: …` line yellow. The decision is made per stream: `--color auto` (the default) colours a stream only when it is a terminal and `NO_COLOR` is unset or empty, so a pipe, a file or `$(agx …)` never sees an escape sequence; `--color always` colours even with `NO_COLOR` set (the flag you typed wins); `--color never` never does. The `--json` envelope is never coloured. Any other value is refused: `agx: --color "purple": use one of auto, always, never` (exit 2).
- **Timeout.** `--timeout` bounds each network request: live `agx usage`, login probes (`agx login check`, `--login-probes`, `--login`, `--all-logins`, `--login-picker`) and `--auto-profile`/`--auto-login`. It takes a duration such as `5s` or `1m`; the default is `15s`, and `0` means the default. Secret commands keep their own 30 second limit, `before_run` 60 seconds.

<!-- snippet: examples/out-scripting/session.txt#columns-env -->
```sh
$ COLUMNS=60 agx config show --field data.*.key --field data.*.value
KEY                                        VALUE
$schema                                    https://raw.githubuserco
                                           ntent.com/ubgo/agx/main/
                                           schema/config.schema.jso
                                           n
version                                    1
default_agent                              claude
profiles.gem.agent                         gemini
profiles.gem.home                          ~/.gemini
profiles.gem.default                       true
profiles.personal.agent                    claude
profiles.personal.home                     ~/.claude
profiles.personal.default                  true
profiles.pi.agent                          pi
profiles.pi.home                           ~/.pi/agent
profiles.pi.default                        true
profiles.work.agent                        claude
profiles.work.home                         ~/.claude-work
profiles.work.login                        carol
logins.carol.agents                        claude
logins.carol.kind                          oauth-token
logins.carol.token                         (plain value,
                                           [redacted])
projects.~/projects/client.claude.profile  work
sessions.root                              ~/.agx/sessions
profiles.gem.billing                       plan
profiles.gem.inherit_defaults              true
profiles.personal.billing                  plan
profiles.personal.inherit_defaults         true
profiles.pi.billing                        plan
profiles.pi.inherit_defaults               true
profiles.work.billing                      plan
profiles.work.inherit_defaults             true
```

## Exit codes

| Code | Launching commands (`run`, `new`, `resume`) | Other commands |
|---|---|---|
| 0 | the agent exited 0 | success |
| the agent's code | the agent's own exit code | not used |
| 125 | agx failed before the agent started: a usage error, an ambiguity, an unsupported option, a secret that did not resolve, a failed `before_run`, a picker without a terminal. `error.code` says which | not used |
| 126 | the agent's binary was found but cannot run | not used |
| 127 | the agent's binary was not found | not used |
| 130 | Ctrl-C before the agent started | Ctrl-C |
| 1 | a picker dismissed with Esc | failure: I/O, network, nothing found, every row failed, a doctor check failed, a picker dismissed |
| 2 | not used | usage error: a bare word, a conflict, an ambiguity, a picker without a terminal, a missing `--field` path, a config cross-check problem |
| 3 | not used | partial: some homes answered and some failed (`usage`) |
| 4 | not used | unsupported by this agent |

Launching commands keep the 125 to 127 range for agx's own failures, the same convention as `env`, `nohup` and container runtimes, so any other code is the agent's. `error.code` names the failure within its class; [Scripting with agx](guides/scripting.md#step-5-match-on-errorcode) lists every code. [`examples/out-exit-codes`](../examples/out-exit-codes/README.md) makes each exit code happen:

<!-- snippet: examples/out-exit-codes/session.txt#other-3 -->
```sh
$ agx usage
agx: warning: claude home ~/.claude-work is not in config; agx profile import --discovered
agx: 2 of 4 rows failed: personal (not-logged-in); codex (not-logged-in)
personal  personal · claude  ~/.claude
  not logged in; run the agent once in this home

codex  codex · codex  ~/.codex
  not logged in; run the agent once in this home

gem  gem · gemini  ~/.gemini
  unsupported: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet

no  no · nocli  ~/.nocli
  unsupported: config-only agent: a spec describes L1 only; this needs code
[exit 3]
```

## Cost classes

Every command and costly flag has a cost class, shown in `agx help --json`:

| Class | Means | Examples | Needs |
|---|---|---|---|
| R | reads disk | `profile list`, `conversation list`, `usage --cached`, `agent list`, `doctor` | nothing |
| N | network | live `usage` (one request per home) | the command is `usage` |
| Q | spends quota | token probes | `--login-probes`, `--login`, `--all-logins` or `--login-picker` on usage, `login check`, `--auto-login` |
| X | runs the agent binary or a secret command | `run`, `new`, `resume`, `--login-setup`, `--resolved-secrets`, `--vendor-checks` | the command or the flag |
| W | writes files | `profile add`, `profile import`, `conversation move`, `session clean`, `clean`, `config init`, `config set` | a plan, `--dry-run`, or `--yes` where it deletes |

Reading disk is the default; nothing spends quota or calls the network unless the command or a flag says so.

## `agx help --json`

The whole command tree with every flag: its type, whether it takes a value, whether it repeats, its cost class, its help line, and the `--field` paths of each command's data. It is generated from the same declarations that build the parser, so it cannot drift from the real flags.

<!-- runnable -->
```sh
$ agx help --json --field data.flags.*.name
["color","config","date-format","field","json","quiet","timeout"]
```

Under `data.commands`, each command has `name`, `path`, `short`, `cost` (leaf commands), `flags`, `fields` (its `--field` paths), `hidden` (for `credential`, `launch` and `plan`) and, for a group, `commands`.

## Pitfalls

- **A launch never exits 2.** On `run`, `new` and `resume` a usage error is 125, like every agx failure before the agent starts.
- **`--field` with `*` needs the key on every item**: `data.commands.*.cost` fails because group commands have no cost.
- **An empty list is checked too.** `--field data.*.nosuch` is `no such path` (exit 2) in text and `--json` alike; on an empty list the path is checked against the fields the command declares (the paths `agx help --json` lists for it), so a misspelled column fails even with no rows, and a declared one prints an empty result.
- **A single-value flag given twice is refused**, even with the same value: `agx: --agent given twice (gemini and pi); pick one` (exit 2; 125 on `run`, `new` and `resume`). The long name is used even when you typed the short one. An alias that already sets a flag (`cl` = `run --profile work`) plus the same flag typed after it (`cl --profile other`) is this refusal too. `agx help --json` marks the repeatable flags (`"repeatable": true`).

## Related

- [Scripting with agx](guides/scripting.md): the same features as a walkthrough.
- [Global flags](commands/README.md#global-flags).
- Examples: [`examples/out-scripting`](../examples/out-scripting/README.md), [`examples/out-exit-codes`](../examples/out-exit-codes/README.md), [`examples/exit-codes`](../examples/exit-codes/README.md).
