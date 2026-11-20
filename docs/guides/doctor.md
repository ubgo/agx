# Check your setup

`agx doctor` checks everything agx depends on, locally: the config file, every secret reference in it, agx's own files, run folders left behind, the shell layer, each agent binary and its version, and each configured home. `agx config check`, `agx config show`, `agx config path` and `agx config set` are the config file's own tools.

Run doctor after you install agx or an agent, after you edit `config.jsonc` by hand, and first whenever a launch fails in a way you do not understand. By default it costs nothing: no network, no secret resolved, no agent binary run. Two flags ask for more, and say what they cost.

## Before you start

- agx installed, and a config. Without one, doctor still runs and reports `no config file at ~/.agx/config.jsonc` as information, with the fix `agx config init`.
- Nothing else: doctor reads the disk only. The examples on this page run on [`examples/cfg-doctor/config.jsonc`](../../examples/cfg-doctor/config.jsonc): two Claude homes, a Gemini CLI home, a login whose token is in gopass, and one whose token is a plain value.

## Step 1: run doctor

<!-- snippet: examples/cfg-doctor/session.txt#doctor -->
```sh
$ agx doctor
✓ config                      ~/.agx/config.jsonc is valid
✓ reference                   logins.carol.token: parses
✓ reference                   profiles.work.env_refs.API_KEY: parses
· plain-token                 logins.gem-work.token holds a plain token value (allowed; redacted in all output)
✓ runs                        no run folders
· shell                       shell layer not loaded: new and resume leave this shell where it is
                              fix: eval "$(agx shell-init --shell zsh)"
✓ binary claude               /usr/local/bin/claude (unknown)
! binary claude               claude version unknown (no install metadata); tested 2.1.323..2.1.327
✓ home claude ~/.claude       ~/.claude
! home claude ~/.claude       no login found in the credentials file or keychain
· home claude ~/.claude       transcripts kept 30 days
✓ home claude ~/.claude-work  ~/.claude-work
! home claude ~/.claude-work  no login found in the credentials file or keychain
· home claude ~/.claude-work  transcripts kept 30 days
✓ binary codex                /usr/local/bin/codex (unknown)
! binary codex                codex version unknown (no install metadata); tested 0.162.1..0.162.1
✓ binary gemini               /usr/local/bin/gemini (unknown)
! binary gemini               gemini version unknown (no install metadata); tested 0.68.0..0.68.0
! home gemini ~/.gemini       ~/.gemini is not a home (missing or no marker)
! home gemini ~/.gemini       no auth method chosen (security.auth.selectedType unset); the first interactive run asks
· home gemini ~/.gemini       general.enableAutoUpdate is on: Gemini CLI updates its own npm-family install
✓ binary pi                   /usr/local/bin/pi (unknown)
! binary pi                   pi version unknown (no install metadata); tested 1.5.2..1.5.2
· binary opencode             opencode not found on PATH; install: npm i -g opencode-ai
```

Each row is one check. `STATUS` is `ok`, `info` (worth knowing, nothing to fix), `warn` (works, but check it), `fail` (something will break) or `skip` (a check that could not run, with the reason). A row with a fix shows it on an indented `fix:` line. The message column wraps to the terminal width.

| Area | What it checks |
|---|---|
| `config` | the file parses, matches the schema and passes the cross-checks (a missing login, two default profiles, …) |
| `reference` | each secret reference parses, its scheme has a plugin (`gopass`), a `$(…)` command's program is on `PATH`, a `${VAR}` is set, an `env_file` exists. Nothing is resolved |
| `plain-token` | a plain token value in config, listed by key as information: a login's plain token, or a literal profile `env` value on an agent's credential variable (allowed; redacted everywhere). A literal profile `env` value whose name only looks secret (`MY_SERVICE_TOKEN`) gets its own line: `looks like a secret written as a literal: shown as written in output; put it in env_refs to keep it redacted`, since output redacts by source, never by name |
| `files` | `tokens.jsonc` has mode 0600, and each `stored` login has a value in it |
| `runs` | run folders whose agent has ended, reported, never deleted; `agx clean` deletes them |
| `shell` | whether the [shell layer](../shell.md) is loaded in this shell |
| `binary` | each agent binary on `PATH`, its version from install metadata against the tested range, and shadowed copies |
| `home` | each configured home: it exists and looks like a home, whether it is logged in, the agent's own notes (transcript retention, auto-update) |
| `secret` | with `--resolved-secrets` only: each reference resolves |
| `vendor` | with `--vendor-checks` only: the binary's own `--version` |

Exit 0 when nothing failed, 1 when a check failed (`error.code` `checks-failed`). Warnings never fail the run.

## Step 2: narrow to one agent

`--agent` keeps the binary and home rows of one agent; the config, reference, files, runs and shell rows always show, because every agent depends on them.

<!-- snippet: examples/cfg-doctor/session.txt#agent -->
```sh
$ agx doctor --agent gemini
✓ config                 ~/.agx/config.jsonc is valid
✓ reference              logins.carol.token: parses
✓ reference              profiles.work.env_refs.API_KEY: parses
· plain-token            logins.gem-work.token holds a plain token value (allowed; redacted in all output)
✓ runs                   no run folders
· shell                  shell layer not loaded: new and resume leave this shell where it is
                         fix: eval "$(agx shell-init --shell zsh)"
✓ binary gemini          /usr/local/bin/gemini (unknown)
! binary gemini          gemini version unknown (no install metadata); tested 0.68.0..0.68.0
! home gemini ~/.gemini  ~/.gemini is not a home (missing or no marker)
! home gemini ~/.gemini  no auth method chosen (security.auth.selectedType unset); the first interactive run asks
· home gemini ~/.gemini  general.enableAutoUpdate is on: Gemini CLI updates its own npm-family install
```

The shell layer sets `AGX_SHELL` when it loads, so in a shell that has it the row turns `ok`:

<!-- snippet: examples/cfg-doctor/session.txt#shell -->
```sh
$ AGX_SHELL=zsh agx doctor --field data.*.status --field data.*.id --field data.*.message
STATUS  ID               MESSAGE
ok      config-valid     ~/.agx/config.jsonc is valid
ok      reference-parse  logins.carol.token: parses
ok      reference-parse  profiles.work.env_refs.API_KEY: parses
info    plain-token      logins.gem-work.token holds a plain token value (allowed; redacted in all output)
ok      run-folder       no run folders
ok      shell-layer      shell layer loaded (zsh)
ok      binary-found     /usr/local/bin/claude (unknown)
warn    version-tested   claude version unknown (no install metadata); tested 2.1.323..2.1.327
ok      home             ~/.claude
warn    login            no login found in the credentials file or keychain
info    retention        transcripts kept 30 days
ok      home             ~/.claude-work
warn    login            no login found in the credentials file or keychain
info    retention        transcripts kept 30 days
ok      binary-found     /usr/local/bin/codex (unknown)
warn    version-tested   codex version unknown (no install metadata); tested 0.162.1..0.162.1
ok      binary-found     /usr/local/bin/gemini (unknown)
warn    version-tested   gemini version unknown (no install metadata); tested 0.68.0..0.68.0
warn    home             ~/.gemini is not a home (missing or no marker)
warn    login            no auth method chosen (security.auth.selectedType unset); the first interactive run asks
info    auto-update      general.enableAutoUpdate is on: Gemini CLI updates its own npm-family install
ok      binary-found     /usr/local/bin/pi (unknown)
warn    version-tested   pi version unknown (no install metadata); tested 1.5.2..1.5.2
info    binary-found     opencode not found on PATH; install: npm i -g opencode-ai
```

## Step 3: ask for more checks

`--resolved-secrets` resolves every reference (cost X: it runs `gopass` and any `$(…)` command, with the same 30 second limit as a launch). The values are never printed, only whether each resolved:

<!-- snippet: examples/cfg-doctor/session.txt#secrets -->
```sh
$ agx doctor --agent claude --resolved-secrets --field data.*.status --field data.*.area --field data.*.message
STATUS  AREA         MESSAGE
ok      config       ~/.agx/config.jsonc is valid
ok      reference    logins.carol.token: parses
ok      reference    profiles.work.env_refs.API_KEY: parses
info    plain-token  logins.gem-work.token holds a plain token value (allowed; redacted in all output)
ok      runs         no run folders
info    shell        shell layer not loaded: new and resume leave this shell where it is
ok      binary       /usr/local/bin/claude (unknown)
warn    binary       claude version unknown (no install metadata); tested 2.1.323..2.1.327
ok      home         ~/.claude
warn    home         no login found in the credentials file or keychain
info    home         transcripts kept 30 days
ok      home         ~/.claude-work
warn    home         no login found in the credentials file or keychain
info    home         transcripts kept 30 days
ok      secret       login carol: resolves
ok      secret       profiles.work.env_refs.API_KEY: resolves
```

`--vendor-checks` runs each agent binary's `--version` (cost X: a process that may touch the login) and places the answer against the tested range:

<!-- snippet: examples/cfg-doctor/session.txt#vendor -->
```sh
$ agx doctor --agent gemini --vendor-checks --field data.*.status --field data.*.area --field data.*.message
STATUS  AREA         MESSAGE
ok      config       ~/.agx/config.jsonc is valid
ok      reference    logins.carol.token: parses
ok      reference    profiles.work.env_refs.API_KEY: parses
info    plain-token  logins.gem-work.token holds a plain token value (allowed; redacted in all output)
ok      runs         no run folders
info    shell        shell layer not loaded: new and resume leave this shell where it is
ok      binary       /usr/local/bin/gemini (unknown)
warn    binary       gemini version unknown (no install metadata); tested 0.68.0..0.68.0
warn    home         ~/.gemini is not a home (missing or no marker)
warn    home         no auth method chosen (security.auth.selectedType unset); the first interactive run asks
info    home         general.enableAutoUpdate is on: Gemini CLI updates its own npm-family install
warn    vendor       gemini --version: gemini 1.0.0 is newer than tested (0.68.0)
```

The fake `gemini` in the example prints its arguments, so the version read here is not a real one. A real binary prints its own; inside the tested range the row is `ok` and reads `<binary> --version: <agent> <version> is tested (<range>)`, outside it `warn` with `is newer than tested` or `is older than tested`, and a binary that prints no version fails the row.

## Step 4: fix what fails

The same checks on a config with problems (the example's `broken.jsonc`, read with `--config`):

<!-- snippet: examples/cfg-doctor/session.txt#broken -->
```sh
$ agx doctor --config ~/broken.jsonc --agent claude
agx: 3 checks failed
✓ config                      ~/broken.jsonc is valid
✗ reference                   logins.carol.token: program op not on PATH
✓ reference                   logins.carol.token: parses
✗ reference                   profiles.work.env_refs.API_KEY: no plugin for scheme gopas (known: gopass)
✗ files                       ~/tokens.jsonc has mode 0644; it holds tokens and must be 0600
                              fix: chmod 600 ~/tokens.jsonc
✓ runs                        no run folders
· shell                       shell layer not loaded: new and resume leave this shell where it is
                              fix: eval "$(agx shell-init --shell zsh)"
✓ binary claude               /usr/local/bin/claude (unknown)
! binary claude               claude version unknown (no install metadata); tested 2.1.323..2.1.327
✓ home claude ~/.claude       ~/.claude
! home claude ~/.claude       no login found in the credentials file or keychain
· home claude ~/.claude       transcripts kept 30 days
✓ home claude ~/.claude-work  ~/.claude-work
! home claude ~/.claude-work  no login found in the credentials file or keychain
· home claude ~/.claude-work  transcripts kept 30 days
[exit 1]
```

- `reference-program`: a `$(…)` reference runs a program that is not on `PATH`. Install it, or fix the command.
- `reference-scheme`: `${gopas:…}` is a misspelled scheme. Expansion is strict: a typo is an error, never an empty value.
- `tokens-mode`: `tokens.jsonc` holds pasted tokens and must be private. Run the `fix:` line.

`agx config check` lists every problem of the file itself at once, each with its line and column:

<!-- snippet: examples/cfg-doctor/session.txt#check-typo -->
```sh
$ agx config check --config ~/typo.jsonc
agx: typo.jsonc has 2 problems
  typo.jsonc:4:76: profiles.personal.modle: unknown key "modle"
  typo.jsonc:5:71: profiles.work.default: must be boolean, got string
[exit 1]
```

<!-- snippet: examples/cfg-doctor/session.txt#check-cross -->
```sh
$ agx config check --config ~/cross.jsonc
agx: cross.jsonc has 3 problems
  cross.jsonc:5:77: profiles.work.login: unknown login dave
  cross.jsonc:5:60: profiles.work.default: claude has two default profiles (personal, work); keep one
  cross.jsonc:7:16: aliases.x: agx launch is internal; an alias cannot run it
[exit 2]
```

<!-- snippet: examples/cfg-doctor/session.txt#check-syntax -->
```sh
$ agx config check --config ~/syntax.jsonc
agx: syntax.jsonc:4:58: missing comma before '"'
[exit 1]
```

A file that cannot be read as config (a syntax error, an unknown key, a wrong type) exits 1; a file that reads but fails a cross-check exits 2. Both have `error.code` `config`, and with `--json` every problem is in `error.hint`, one per line. On a valid file:

<!-- snippet: examples/cfg-doctor/session.txt#check -->
```sh
$ agx config check
~/.agx/config.jsonc: ok (0 agents, 3 profiles, 2 logins, 0 projects, 0 aliases)
```

## Step 5: change one key

`agx config set` changes one scalar key and keeps your comments, key order and layout. The value is checked against the schema and the whole file is validated before anything is written.

<!-- snippet: examples/cfg-config-set/session.txt#set -->
```sh
$ agx config set --key default_agent --value codex
default_agent = codex

$ agx config set --key profiles.work.model --value opus
profiles.work.model = opus

$ agx config set --key profiles.work.live_login --value true
profiles.work.live_login = true

$ agx config set --key sessions.root --value ~/sessions
sessions.root = ~/sessions
```

Switches take `true` or `false`, numbers take digits, and `~` in a path value is kept as written. Lists and objects have their own commands:

<!-- snippet: examples/cfg-config-set/session.txt#set-refused -->
```sh
$ agx config set --key profiles.work.live_login --value yes
agx: --key profiles.work.live_login takes true or false, not "yes"
[exit 2]

$ agx config set --key profiles --value work
agx: --key profiles is an object, not a scalar; use its own command (profile edit, project use, alias add, login edit) or edit config.jsonc
[exit 2]

$ agx config set --key profiles.work.modle --value opus
agx: --key profiles.work.modle: unknown key profiles.work.modle
  keys: agent, home, default, billing, login, home_login, args, inherit_defaults, model, autonomy, env, env_refs, env_file, env_file_commands, before_run, before_run_from, before_run_env, settings_delivery, live_login, claude, codex
[exit 2]

$ agx config set --key profiles.work.home --value ''
agx: refused: the change would make config.jsonc invalid: home is empty
  home is empty
[exit 2]
```

## Step 6: see the config agx uses

`agx config path` prints the file in use; with `--json`, `data.source` says which rule chose it. The rules, in order: `--config FILE`, then `$AGX_CONFIG_DIR`, then `$AGX_HOME`, then `~/.agx`. Both variables name a folder, and agx expands a leading `~` and `$VAR` in them itself.

<!-- snippet: examples/cfg-config-set/session.txt#where -->
```sh
$ agx config path --json --field data.file --field data.source
{
  "data.file": "~/.agx/config.jsonc",
  "data.source": "AGX_CONFIG_DIR"
}

$ AGX_CONFIG_DIR= AGX_HOME=~/agx-alt agx config path --json --field data.file --field data.source
{
  "data.file": "~/agx-alt/config.jsonc",
  "data.source": "AGX_HOME"
}

$ AGX_CONFIG_DIR= agx config path --json --field data.file --field data.source
{
  "data.file": "~/.agx/config.jsonc",
  "data.source": "default (~/.agx)"
}

$ agx config path --config ~/other/agx.jsonc --json --field data.file --field data.exists
{
  "data.file": "~/other/agx.jsonc",
  "data.exists": false
}
```

`agx config show` prints every value with the line and column it comes from, plus the defaults that apply where the file says nothing. Tokens show only whether they are a reference or a plain value:

<!-- snippet: examples/cfg-doctor/session.txt#show -->
```sh
$ agx config show
KEY                                 VALUE                      SOURCE
version                             1                          config.jsonc:2:3
default_agent                       claude                     config.jsonc:3:3
profiles.gem.agent                  gemini                     config.jsonc:7:14
profiles.gem.home                   ~/.gemini                  config.jsonc:7:33
profiles.gem.default                true                       config.jsonc:7:54
profiles.gem.login                  gem-work                   config.jsonc:7:71
profiles.personal.agent             claude                     config.jsonc:5:19
profiles.personal.home              ~/.claude                  config.jsonc:5:38
profiles.personal.default           true                       config.jsonc:5:59
profiles.work.agent                 claude                     config.jsonc:6:15
profiles.work.home                  ~/.claude-work             config.jsonc:6:34
profiles.work.login                 carol                      config.jsonc:6:60
profiles.work.env_refs.API_KEY      ${gopass:work/api-key}     config.jsonc:6:92
logins.carol.agents                 claude                     config.jsonc:10:16
logins.carol.kind                   oauth-token                config.jsonc:10:38
logins.carol.token                  (reference)                config.jsonc:10:61
logins.gem-work.agents              gemini                     config.jsonc:12:19
logins.gem-work.kind                api-key                    config.jsonc:12:41
logins.gem-work.token               (plain value, [redacted])  config.jsonc:12:60
sessions.root                       ~/.agx/sessions            default
profiles.gem.billing                plan                       default
profiles.gem.inherit_defaults       true                       default
profiles.personal.billing           plan                       default
profiles.personal.inherit_defaults  true                       default
profiles.work.billing               plan                       default
profiles.work.inherit_defaults      true                       default
```

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-doctor$'
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-config-set$'
```

The sessions [`examples/cfg-doctor/session.txt`](../../examples/cfg-doctor/session.txt) and [`examples/cfg-config-set/session.txt`](../../examples/cfg-config-set/session.txt) hold every command on this page with its real output.

## Pitfalls

- **Doctor exits 1 on any `fail`.** A script that runs it as a gate should read the rows (`--json --field data`) rather than treat every non-zero exit as broken agx.
- **`warn … not a home (missing or no marker)`** means the folder exists but has none of the agent's marker files yet: run the agent once in that home, or point the profile at the right folder.
- **`warn … no login found`** is the home's own login. A profile that always runs with a token login (`login` in config) does not need one.
- **`--resolved-secrets` waits on a locked store.** Each reference has the same 30 second limit as a launch, so a locked gopass store fails the row after 30 seconds; unlock it first (`gopass show -o some/entry` once).
- **`--vendor-checks` starts every agent binary** (or the one `--agent` names). Keep it out of loops and status bars.
- **`config set` refuses rather than half-writes.** `--key profiles.work.home --value ''` is refused with `the change would make config.jsonc invalid: home is empty`, and a key the schema does not have lists the valid ones. Every refusal exits 2.
- **`--config` names a file, the variables name a folder.** `--config ~/other/agx.jsonc` reads that file and keeps `tokens.jsonc` and `runs/` next to it; `AGX_HOME=~/agx-alt` reads `~/agx-alt/config.jsonc`.
- **Two agx processes writing at once.** The second waits 5 seconds for the lock and stops with `config is being written by another agx`; run it again.

## Related

- [Agents and what they support](agents.md): what the binary and version rows mean.
- [Troubleshooting](../troubleshooting.md): messages and what to do.
- Reference: [`agx doctor`](../commands/setup.md#agx-doctor), [`agx config check`](../commands/setup.md#agx-config-check), [`agx config set`](../commands/setup.md#agx-config-set), [`agx config path`](../commands/setup.md#agx-config-path), [config files](../config.md#files).
- Examples: [`examples/cfg-doctor`](../../examples/cfg-doctor/README.md), [`examples/cfg-config-set`](../../examples/cfg-config-set/README.md).
