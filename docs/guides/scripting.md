# Scripting with agx

Every agx command can feed a script: one JSON shape with `--json`, single values with `--field`, stable error codes, exit codes that separate agx's own failures from the agent's, and a cost class on every command so a status bar never spends quota by accident. No jq needed for the common cases.

Use this page when you write a shell prompt segment, a status bar, a CI step, or a wrapper that launches an agent and reacts to how it ended.

## Before you start

- agx installed and a config. The examples run on [`examples/out-scripting/config.jsonc`](../../examples/out-scripting/config.jsonc), in `~/projects/client`, a folder bound to the `work` profile.
- Run `agx help --json` once to see every command's flags and the `--field` paths of its data (step 6).

## Step 1: ask for JSON

`--json` prints the same envelope on every command, on success and on failure:

<!-- snippet: examples/out-scripting/session.txt#envelope -->
```sh
$ agx profile list --profile gem --json
{
  "schema_version": 1,
  "command": "profile list",
  "ok": true,
  "data": [
    {
      "name": "gem",
      "agent": "gemini",
      "home": "~/.gemini",
      "default": true,
      "billing": "plan",
      "login": "home login",
      "home_exists": true,
      "state": "none"
    }
  ],
  "decisions": [],
  "warnings": [],
  "error": null
}
```

<!-- snippet: examples/out-scripting/session.txt#error -->
```sh
$ agx run --profile nosuch --json
{
  "schema_version": 1,
  "command": "run",
  "ok": false,
  "data": null,
  "decisions": [],
  "warnings": [],
  "error": {
    "code": "not-found",
    "message": "unknown profile nosuch",
    "hint": "agx profile list"
  }
}
[exit 125]
```

`ok` is false and `data` null on failure; `error` has a stable `code`, the `message` a person reads, and an optional `hint` (the command that helps). The [envelope reference](../output.md#the-json-envelope) lists every key.

## Step 2: pick values with `--field`

`--field PATH` selects from the whole envelope: dots go down a level, a number indexes a list, `*` is every item. One path prints the value bare (a string as plain text):

<!-- snippet: examples/out-scripting/session.txt#field-one -->
```sh
$ agx run --dry-run --quiet --json --field data.profile
work
```

Several paths print one object keyed by path, in your order:

<!-- snippet: examples/out-scripting/session.txt#field-many -->
```sh
$ agx run --dry-run --quiet --json --field data.profile --field data.home --field data.login
{
  "data.profile": "work",
  "data.home": "~/.claude-work",
  "data.login": "carol"
}
```

`*` collects one key from every item:

<!-- snippet: examples/out-scripting/session.txt#field-star -->
```sh
$ agx profile list --json --field data.*.name
["personal","work","gem","pi"]
```

Without `--json`, `--field data.*.KEY` picks and orders the columns of a table:

<!-- snippet: examples/out-scripting/session.txt#columns -->
```sh
$ agx profile list --field data.*.name --field data.*.agent --field data.*.home
NAME      AGENT   HOME
personal  claude  ~/.claude
work      claude  ~/.claude-work
gem       gemini  ~/.gemini
pi        pi      ~/.pi/agent
```

A misspelled path is an error that names it:

<!-- snippet: examples/out-scripting/session.txt#field-missing -->
```sh
$ agx profile list --json --field data.*.hom
agx: --field data.*.hom: no such path (did you mean data.*.home?)
[exit 2]
```

An empty list is no loophole: with no rows, the path is checked against the fields the command declares, so a typo still fails and a real column prints an empty result:

<!-- snippet: examples/out-scripting/session.txt#field-empty -->
```sh
$ agx alias list --field data.*.nosuch
agx: --field data.*.nosuch: no such path
[exit 2]

$ agx alias list --json --field data.*.name
[]
```

A flag that takes one value is refused when given twice, so a script that adds a flag an alias or variable already sets fails loudly instead of keeping one of them:

<!-- snippet: examples/out-scripting/session.txt#twice -->
```sh
$ agx usage --agent gemini --agent pi
agx: --agent given twice (gemini and pi); pick one
[exit 2]

$ agx profile list --quiet --quiet
agx: --quiet given twice; pick one
[exit 2]
```

## Step 3: keep stdout clean

When something other than a flag decided (here, the folder binding), agx prints a note on stderr. stdout holds only the result, so a script that reads stdout is never confused; `--quiet` turns the notes off for a clean terminal too:

<!-- snippet: examples/out-scripting/session.txt#notes -->
```sh
$ agx run --dry-run --json --field data.profile
agx: agent claude (folder binding ~/projects/client)
agx: profile work (folder binding ~/projects/client)
work

$ agx run --dry-run --quiet --json --field data.profile
work
```

Warnings are `agx: warning: …` lines on stderr in text mode (dropped by `--quiet`); with `--json` they are in `warnings`, each with a code, and not on stderr:

<!-- snippet: examples/out-scripting/session.txt#warnings -->
```sh
$ agx run --dry-run --quiet --json --field warnings
[{"code":"version","message":"installed Claude Code version unknown; flags not checked"}]
```

On a launching command, `decisions` holds each decision with its value and source, the same rows `--dry-run` prints:

<!-- snippet: examples/out-scripting/session.txt#decisions -->
```sh
$ agx run --dry-run --quiet --json --field decisions.1
{"name":"profile","value":"work","source":"folder binding ~/projects/client"}
```

A token never reaches any output, plain or resolved: it is `[redacted]` in text, JSON, errors and logs. The one exception is a script that asks for it with `agx login token`; see [use a login's token in a script](#use-a-logins-token-in-a-script). Redaction goes by a value's source (a login, a reference, an env file, an agent's credential variable) and by the value, never by a name, so a literal `--env MY_SERVICE_TOKEN=abc` is shown as written; [what is redacted](../output.md#what-is-redacted) has the full rule.

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

## Step 4: branch on the exit code

Launching commands (`run`, `new`, `resume`) exit with the agent's own code, so agx keeps 125 to 127 for its own failures, the convention of `env`, `nohup` and container runtimes. Every other command uses small codes.

| Code | Launching commands | Other commands |
|---|---|---|
| 0 | the agent exited 0 | success |
| 1 | a picker dismissed with Esc | failure: I/O, network, nothing found, every row failed, a doctor check failed |
| 2 | not used | usage error, conflict, ambiguity, a picker without a terminal, a missing `--field` path, a config cross-check |
| 3 | not used | partial: some rows answered and some failed |
| 4 | not used | unsupported by this agent |
| 125 | agx failed before the agent started (any of the above) | not used |
| 126 | the binary was found but cannot run | not used |
| 127 | the binary was not found | not used |
| 130 | Ctrl-C before the agent started | Ctrl-C |
| any other | the agent's own exit code | not used |

The agent's code passes through unchanged (the fake agent here exits 3):

<!-- snippet: examples/out-exit-codes/session.txt#launch-agent-code -->
```sh
$ FAKE_AGENT_EXIT=3 agx run --quiet
agent claude
argv
pwd ~/projects/app
...
[exit 3]
```

agx's own failures on a launch:

<!-- snippet: examples/out-exit-codes/session.txt#launch-125 -->
```sh
$ agx run --profile personal --login carol --home-login
agx: --login and --home-login both choose the login; pick one
[exit 125]
```

<!-- snippet: examples/out-exit-codes/session.txt#launch-127 -->
```sh
$ agx run --profile no
agx: nocli not found on PATH
[exit 127]
```

A listing where some rows answered exits 3 and still prints the rows that did:

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

## Step 5: match on `error.code`

The exit code says what kind of failure; `error.code` says which. Match on the code, never on the message, which may be reworded.

<!-- snippet: examples/out-scripting/session.txt#error-code -->
```sh
$ agx run --profile nosuch --json --field error.code
not-found
[exit 125]
```

<!-- snippet: examples/out-exit-codes/session.txt#codes -->
```sh
$ agx usage --agent claude --json --field error.code
io
[exit 1]

$ agx conversation move --dir ~/projects/client --to-profile codex --dry-run --json --field error.code
unsupported
[exit 4]
```

| `error.code` | Means |
|---|---|
| `usage` | a malformed command line: a bare word, a bad value, an unknown flag |
| `conflict` | two choices for one thing in one command |
| `ambiguous` | a choice agx will not make alone (an id prefix in two homes, a shared home with no default) |
| `no-tty` | a picker or question without a terminal |
| `not-found` | a named profile, login, agent or conversation that does not exist |
| `unsupported` | a capability the agent does not have |
| `config` | an invalid or unreadable config file |
| `config-version` | a config written by a newer agx |
| `locked` | the config is being written by another agx |
| `secret` | a reference or token that did not resolve |
| `no-profiles` | a launch with no profile configured |
| `no-agent` | a launch where nothing names an agent |
| `home-missing` | a profile whose home folder is gone |
| `binary-missing` | the agent binary is not on `PATH` (exit 127 on a launch) |
| `binary-not-executable` | the binary was found but cannot run (exit 126 on a launch) |
| `feature-unavailable` | an option the installed agent version lacks |
| `wrong-agent` | a login that does not serve the run's agent |
| `auto-pick` | `--auto-profile` or `--auto-login` found nothing eligible, or no member of a login pool is usable (`agx login pick`, `agx login token`) |
| `before-run-failed` | the profile's `before_run` script failed or is missing |
| `folder-gone` | a conversation whose folder no longer exists |
| `plan-file` | a stale, foreign or unreadable shell plan file |
| `field-path` | a `--field` path missing from the envelope |
| `interrupted` | Ctrl-C before anything started |
| `dismissed` | a picker closed with Esc |
| `partial` | some rows answered and some failed |
| `network` | a failed network call |
| `checks-failed` | `agx doctor` with at least one failed check |
| `in-use` | a remove refused because config entries still use the thing |
| `move-blocked` | a move or promote stopped before touching a file |
| `exists` | a destination that already exists |
| `io` | any other failure (disk, process) |

## Step 6: know the cost before you call

Every command and costly flag has a cost class: **R** reads disk, **N** network, **Q** spends quota, **X** runs the agent binary or a secret command, **W** writes files. Nothing calls the network or spends quota unless the command or a flag says so, so a status bar can call any R command as often as it likes. The classes are in `agx help --json`, which is generated from the parser's own declarations and lists every command, flag and `--field` path:

<!-- snippet: examples/out-scripting/session.txt#help-json -->
```sh
$ agx help --json --field data.flags.*.name
["color","config","date-format","field","json","quiet","timeout"]

$ agx help --json --field data.commands.*.name
["agent","alias","clean","completion","config","conversation","credential","doctor","help","launch","live","login","new","plan","profile","project","resume","run","session","shell-init","usage","version","whoami"]

$ agx help --json --field data.commands.20.name --field data.commands.20.cost --field data.commands.20.fields
{
  "data.commands.20.name": "usage",
  "data.commands.20.cost": "N",
  "data.commands.20.fields": [
    "data.*.kind",
    "data.*.name",
    "data.*.agent",
    "data.*.home",
    "data.*.profile",
    "data.*.profiles",
    "data.*.login",
    "data.*.account",
    "data.*.source",
    "data.*.observed_at",
    "data.*.age",
    "data.*.status",
    "data.*.reason",
    "data.*.left",
    "data.*.windows",
    "data.*.plan",
    "data.*.index",
    "data.*.plan_label"
  ]
}
```

The third command reads one entry by its index (20 is `usage` in this version; match on `name` in a script, since indexes move when commands are added). Under `data.commands`, each command has `name`, `path`, `short`, `cost` (on leaf commands), `flags` (each with `name`, `type`, `takes_value`, `repeatable`, `required`, `default`, `help`), `fields` (its `--field` paths) and, for groups, `commands`. The [cost class table](../output.md#cost-classes) gives an example of each class.

`agx version --json` gives the binary, version, build source, commit, build time, Go version and platform:

<!-- snippet: examples/out-scripting/session.txt#version -->
```sh
$ agx version --json --field data.binary
agx
```

## Step 7: width, colour and timeouts

Human tables wrap their last column to the line width: the terminal's when stdout is one, else `COLUMNS`, else 120, and never below 60. A script that parses human output should set `COLUMNS` wide, or better, use `--json`:

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

Colour never reaches stdout: agx colours only its own error and warning lines on stderr (`auto`: when stderr is a terminal and `NO_COLOR` is unset or empty). Output is the same with `--color never` or `NO_COLOR` set, and a value other than `auto`, `always` or `never` is refused:

<!-- snippet: examples/out-scripting/session.txt#color -->
```sh
$ agx profile list --color never --field data.*.name
NAME
personal
work
gem
pi

$ NO_COLOR=1 agx profile list --field data.*.name
NAME
personal
work
gem
pi

$ agx profile list --color purple
agx: --color "purple": use one of auto, always, never
[exit 2]
```

`--timeout` bounds each network request (live `usage`, login probes, `--auto-profile` and `--auto-login`); the default is `15s`, and `0` means the default. It takes a Go duration:

<!-- snippet: examples/out-scripting/session.txt#timeout -->
```sh
$ agx usage --agent gemini --timeout 5s --json --field data.*.status
["unsupported"]

$ agx usage --timeout soon
agx: invalid argument "soon" for "--timeout" flag: must be a duration like "30s" or "5m"
[exit 2]
```

## Use a login's token in a script

`agx login token --id NAME` prints one login's token, and only the value and a newline, on stdout, so a script can read it from whatever source the login uses (gopass, a `$(…)` command, a variable, a pasted token) without knowing which:

```sh
export ANTHROPIC_API_KEY="$(agx login token --id ci)"
agx login token --id ci | pbcopy
```

From [`examples/acct-login-token`](../../examples/acct-login-token/README.md), where the test runner gives stdout a pipe as `$(…)` and `|` do:

<!-- snippet: examples/acct-login-token/session.txt#sources -->
```sh
$ agx login token --id ci
fake-ci-token

$ agx login token --id carol
fake-token-carol

$ agx login token --id derek
fake-token-derek

$ agx login token --id pasted
fake-pasted-token
```

It is the one command that prints a token, so it guards the value: stdout on a terminal is refused (exit 2) unless you add `--reveal`, and `--json` and `--field` are refused (exit 2) because an envelope around a secret ends up in logs. A key helper login has no token of its own and is refused (exit 4); a reference that does not resolve exits 1 with the reason and no value:

<!-- snippet: examples/acct-login-token/session.txt#refusals -->
```sh
$ agx login token --id vault
agx: login vault is an api-key-helper: it has no token to print; the agent runs its helper command itself
[exit 4]

$ agx login token --id ci --json
agx: login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field
[exit 2]

$ agx login token --id spare
agx: login spare: SPARE_CLAUDE_TOKEN is not set
[exit 1]

$ agx login token
agx: --id is required: the login name (agx login list)
[exit 2]

$ agx login token --id nosuch
agx: unknown login nosuch
[exit 1]
```

## Read a login pool in a script

`agx login pick --id auto --json` carries the pool's decision: `--field data.selected.name` is the member a key request would get now, `data.source` says whether that came from a probe or the cache, and `data.candidates` lists every member with `state`, `reasons` and `room_percent`. Use `--cached` in a loop: without it, a cache older than the pool's `usage_max_age` is probed. Inside a session, `agx whoami --json --field data.in_use` is the member the agent got last and `data.next` the one it gets next (empty when no switch is pending). `agx login token --id auto` prints the picked member's token for a pipe.

```sh
member=$(agx login pick --id auto --cached --json --field data.selected.name) || echo "no member usable (exit $?)"
```


```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/(out-|acct-login-token$)'
```

[`examples/out-scripting/session.txt`](../../examples/out-scripting/session.txt), [`examples/out-exit-codes/session.txt`](../../examples/out-exit-codes/session.txt) and [`examples/acct-login-token/session.txt`](../../examples/acct-login-token/session.txt) hold every command on this page with its real output.

## Pitfalls

- **A launch never exits 2.** On `run`, `new` and `resume` every agx failure, usage errors included, is 125, so 1 to 124 are always the agent's. Check `error.code` for which.
- **`--field` with `*` needs the key on every item.** `agx help --json --field data.commands.*.cost` fails with `no such path (did you mean data.commands.*.short?)` because group commands have no cost; select the leaf commands' entries instead.
- **An empty list still checks `--field`.** On an empty list a path is checked against the fields the command declares (`agx help --json` lists them), so `agx alias list --field data.*.nosuch` with no aliases fails with `--field data.*.nosuch: no such path` (exit 2), and a declared path prints an empty result.
- **A single-value flag given twice is refused.** `agx usage --agent gemini --agent pi` fails with `--agent given twice (gemini and pi); pick one` (exit 2; 125 on a launch), even when both values are the same, so a script that appends a flag to an alias or a variable that already sets it fails instead of silently keeping one. Repeatable flags (`--profile` on `usage`, `--env`, `--field`) are marked `"repeatable": true` in `agx help --json`.
- **`--timeout` does not shorten secret commands or `before_run`.** A reference has its own 30 second limit and a `before_run` script 60 seconds.
- **Colour goes to stderr only.** `--color` takes `auto`, `always` or `never` (anything else: `--color "X": use one of auto, always, never`, exit 2), and colours only agx's own error and warning lines; stdout and `--json` never carry escape codes.
- **Reading stderr for results breaks.** Notes and warnings may be reworded or turned off by `--quiet`; use `warnings` and `decisions` in the envelope instead.

## Related

- Reference: [Output, `--field` and exit codes](../output.md), [global flags](../commands/README.md#global-flags).
- [Check your setup](doctor.md): `agx doctor --json` in a CI gate.
- [Headless runs](headless.md): an agent's own machine-readable output.
- Examples: [`examples/out-scripting`](../../examples/out-scripting/README.md), [`examples/out-exit-codes`](../../examples/out-exit-codes/README.md), [`examples/exit-codes`](../../examples/exit-codes/README.md).
