# Coming from agx v0.3

agx v0.4 is a complete rewrite under a new module, `github.com/ubgo/agx`. It continues the version line of `github.com/khanakia/agx` v0.3, which is archived. Commands, flags and the config file all changed, and nothing is carried over automatically: v0.4 starts fresh.

## Why it changed

v0.3 grew Claude-first. Codex support was a reduced copy, and adding Gemini CLI, pi and other agents would have meant more vendor-specific code. v0.4 is built on agentio, a library that knows each agent's files, logins, usage and launch rules, so every agent works the same way. It also brings named flags only, a typed JSONC config, `--dry-run` showing where every decision came from, and no hidden defaults.

## Install

```sh
go install github.com/ubgo/agx@latest
agx version
```

Remove the old binary from your `PATH` first if it is installed somewhere else, so the right `agx` runs (`which -a agx`).

## Recreate your setup

```sh
agx config init --default-agent claude --discovered
agx profile add --name kimi --agent claude --home ~/.claude --billing api
agx login add --name derek --agent claude --kind oauth-token --token-ref '$(gopass show -o personal/claude/derek)'
agx alias add --name cl --args "run --profile personal"
```

`config init --discovered` offers every home it finds as a profile. Add logins and aliases with their commands, or edit `~/.agx/config.jsonc` by hand (the [config reference](config.md) has every key). Your agent homes and their conversations are untouched: agx never edits or deletes them.

Then check the result. `agx config check` validates the file and every cross-reference, and `agx doctor` checks the references, the binaries and their versions, and each home, without the network ([Check your setup](guides/doctor.md)):

```sh
agx config check
agx doctor
```

## Where each v0.3 feature went

| v0.3 | v0.4 |
|---|---|
| bare `agx` (usage) | `agx` prints help; usage is `agx usage` |
| `agx plan` | `agx usage --plan` |
| `agx profiles` | `agx profile list` |
| `agx run`, `agx new` | the same commands with named flags (`--name` for the session slug) |
| `agx resume`, `agx ws` | `agx resume` (`--picker folder` for the folder picker); listing is `agx conversation list` |
| `agx sessions ls`, `gc`, `promote` | `agx session list`, `clean`, `promote` |
| `agx sessions move` | `agx conversation move` |
| `agx login add`, `ls`, `rm`, `use`, `check` | `agx login add`, `list`, `remove`, `use`, `check`; `--replace` is `agx login edit` |
| `agx project use`, `ls`, `rm`, `show` | `agx project use`, `list`, `remove`, `show`, per agent |
| `-p auto`, `--login auto`, `--login home` | `--auto-profile`, `--auto-login`, `--home-login` |
| `token_cmd`, `token_env`, `secrets` | one reference syntax in `token` and `env_refs`; plain values are allowed too |
| `providers.<id>.args` and `.login` | `agents.<agent>.defaults.args` and `.login` |
| `args_replace` | `"inherit_defaults": false` |
| `AGX_LOGIN` and `aglogin` | `AGX_LOGIN` (plus `AGX_LOGIN_<AGENT>`) and `agx login use --terminal` |
| `AGX_PLAN_FILE` and `agx exec --plan` | the shell layer's hidden `--plan-file` and `agx launch` |
| profile discovery at run time | dropped; `agx profile list --discovered` and `agx profile import` |
| `config.yaml` and `tokens.yaml` | `config.jsonc` and `tokens.jsonc` |
| JSON envelope with `kind` and `count` | a new envelope with `--field` and stable `error.code` values (see [output](output.md) and [Scripting with agx](guides/scripting.md)) |
| hard-coded Claude and Codex support | five built-in agents plus config-defined ones; `agx agent show --id` lists what each supports ([Agents](guides/agents.md)) |

## Habits to change

- **Name every input.** `agx run work` is now `agx run --profile work` (or `agx run -p work`). A bare word is an error that prints the named form.
- **No reserved words.** `--profile auto` and `--login home` are now their own flags, so a profile can be called anything.
- **Bindings are per agent.** `agx project use --profile work` binds Claude only; bind Codex separately.
- **Load the shell layer** for `cd` after `new` and `resume` and for per-terminal logins: `eval "$(agx shell-init --shell zsh)"`. See [Shell layer](shell.md).
- **Scripts read `error.code` and exit codes.** A launch exits with the agent's code, and 125 to 127 when agx itself stopped; see [exit codes](output.md#exit-codes).

## Related

- [Getting started](getting-started.md), [config reference](config.md), [command reference](commands/README.md).
- [Check your setup](guides/doctor.md), [Agents and what they support](guides/agents.md), [Scripting with agx](guides/scripting.md).
