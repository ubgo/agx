# Which way to pass something to an agent

agx has six roads into an agent: the flags every agent shares (`--model`, `--prompt`, `--add-dir`, `--autonomy`, `--headless`), a typed vendor option (`--option` or the profile's agent block), a settings key (`--setting` or the profile's `settings`), an environment variable (`--env`, `--env-file`, `--no-env` or the profile's `env`, `env_refs`, `env_file`), the profile itself for anything you want on every launch, and raw args after `--` (or the profile's `args`). This page helps you pick the road. Every road is shown with its config in [Passing anything to an agent](../launch-options.md).

## Short answer

| You want to pass | Use | Checked by agx |
|---|---|---|
| the model, a first prompt, extra folders, a permission level, an event stream | the shared flags `--model`, `--prompt`, `--add-dir`, `--autonomy`, `--headless` | yes: mapped per agent, refused when an agent cannot honour it |
| a vendor launch flag agx models (Claude `effort`, `mcp_configs`; Codex `sandbox`) | `--option key=value` for one run, the profile's `claude` or `codex` block for every run | yes: key and type from the agent's option registry, and the installed version |
| a Claude settings key (hooks, permissions, `statusLine`, `env`) | `--setting key=value` for one run, `"claude": { "settings": … }` for every run | known keys type-checked; unknown keys pass with a note |
| a Codex config value | `--setting key=value` (becomes `-c key=value`) or the `codex` block's `settings` | known keys type-checked |
| an environment variable | `--env NAME=value`, `--env-file PATH`, `--no-env NAME`; profile `env` (literal), `env_refs` (references), `env_file` | references expanded strictly |
| a credential | a login, not `--env` (see [Which login kind](login-kind.md)) | yes |
| a flag agx has never heard of, or exactly what you typed | after `--`, or profile `args` | no: passed as written, last (a raw `--settings` is read and merged instead) |

## The same value by different roads

The profile below has `"effort": "high"` in its `claude` block, a settings object with one key, and `env` `DEBUG=1`:

<!-- snippet: examples/choose-passing/config.jsonc -->
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
    "gem": { "agent": "gemini", "home": "~/.gemini", "default": true }
  }
}
```

With nothing on the command line, the profile's block and settings reach Claude:

<!-- snippet: examples/choose-passing/session.txt#profile -->
```sh
$ agx run --dry-run --quiet --json --field data.command.line
claude --effort high --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json
```

The model by each road: a shared flag, a typed option, a settings key (inline here so you can read it), and raw after `--`:

<!-- snippet: examples/choose-passing/session.txt#roads -->
```sh
$ agx run --model opus --dry-run --quiet --json --field data.command.line
claude --model opus --effort high --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json

$ agx run --option effort=max --dry-run --quiet --json --field data.command.line
claude --effort max --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json

$ agx run --setting model=opus --settings-delivery inline --dry-run --quiet --json --field data.command.line
claude --effort high --settings "{\"cleanupPeriodDays\":20,\"model\":\"opus\"}"

$ agx run --dry-run --quiet --json --field data.command.line -- --model opus
claude --effort high --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json --model opus
```

An environment variable on the command line beats the profile's:

<!-- snippet: examples/choose-passing/session.txt#env -->
```sh
$ agx run --env DEBUG=2 --dry-run --quiet --json --field data.command.env
["set DEBUG=2","set AGX_RUN_ID=20261114-101500-k3x9","set AGX_RUN_AGENT=claude","set AGX_RUN_PROFILE=work","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.claude-work","set CLAUDE_CONFIG_DIR=~/.claude-work"]
```

## If this, then that

- If the value is the same for every launch of an account, put it in the profile (block, `settings`, `env`, `args`). If it is for one run, use the flag. The command line beats the profile, and `--dry-run` says what it overrode.
- If agx models it, prefer the modelled road (`--model`, `--option`, `--setting`): it is checked, shows its source in `--dry-run`, and refuses a second choice for the same thing. Raw args are for what agx does not model yet.
- If it is a secret, use a login or `env_refs` with a reference, never a plain value on the command line. Values go into the environment, never onto the agent's command line.
- If the agent is Gemini CLI, pi, opencode or a config-defined agent, there is no `--setting` or `--option`: use the shared flags, `--env`, and raw args.
- If two of your roads set the same concern in one command, agx refuses and names both. Pick one.

## Per agent

| Road | Claude Code | Codex | Gemini CLI | pi | opencode | config-defined |
|---|---|---|---|---|---|---|
| `--model` | yes | yes | yes | yes | yes | with `model_args` in the spec |
| `--autonomy` | every level | all but `accept-edits` | every level | `read-only` and `full` | `full` only | levels in `autonomy_args` |
| `--option`, agent block | yes | yes | refused | refused | refused | refused |
| `--setting`, `settings` | settings JSON | `-c key=value` | refused | refused | refused | refused |
| `--env`, `env`, `env_refs`, `env_file` | yes | yes | yes | yes | yes | yes |
| raw after `--`, `args` | yes | yes | yes | yes | yes | yes |

<!-- snippet: examples/choose-passing/session.txt#other-agents -->
```sh
$ agx run --profile codex --setting model_reasoning_effort=high --option sandbox=workspace-write --dry-run --quiet --json --field data.command.line
codex -c "model_reasoning_effort=\"high\"" -s workspace-write

$ agx run --profile gem --setting theme=dark --dry-run --quiet
agx: gemini has no --option or --setting (Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args); pass its flags after --
[exit 125]

$ agx run --profile gem --model gemini-2.5-pro --dry-run --quiet --json --field data.command.line -- --sandbox
gemini --model gemini-2.5-pro --sandbox
```

## Pitfalls

- Two command-line choices for one concern are refused (exit 125). A raw duplicate is not: it is appended last, the agent uses the later one, and `--dry-run` notes it.

<!-- snippet: examples/choose-passing/session.txt#conflict -->
```sh
$ agx run --model opus --setting model=sonnet --dry-run --quiet
agx: --model and --setting model both set the model; pick one
[exit 125]

$ agx run --model opus --dry-run --json --field data.command.line -- --model sonnet
agx: agent claude (default_agent)
claude --model opus --effort high --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json --model sonnet
```

- `--env` cannot set the home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, …): agx refuses with `CLAUDE_CONFIG_DIR is set by the profile's home; use --profile`.
- Profile `env` values are literal; `${…}` in them is never expanded. Put references in `env_refs`.
- A reference that cannot resolve stops the launch instead of passing an empty value: `--env TOKEN: NOT_SET_ANYWHERE is not set`.
- A raw `--settings` after `--` is merged into agx's settings key by key (your keys win; objects merge, anything else replaces) and Claude gets one `--settings`; with `--live-login`, a raw `apiKeyHelper` is refused (see [launch options, recipe 8](../launch-options.md#a-raw---settings-of-your-own)).
- Nothing on this page edits an agent's own settings files: every road applies to one launch.

## Related

- Guide: [Passing anything to an agent](../launch-options.md) (every recipe), [the one-winner rule](../launch-options.md#one-winner-per-concern), [How agx decides](../how-agx-decides.md), [Autonomy](../guides/autonomy.md), [Raw args](../guides/raw-args.md), [Headless runs](../guides/headless.md).
- Config: [agent blocks](../config.md#agent-blocks), [profiles](../config.md#profiles).
- Example: [choose-passing](../../examples/choose-passing/README.md), [launch-options](../../examples/launch-options/README.md), [launch-settings](../../examples/launch-settings/README.md), [launch-env](../../examples/launch-env/README.md), [launch-codex](../../examples/launch-codex/README.md), [launch-gemini-pi](../../examples/launch-gemini-pi/README.md).
- Other decisions: [Settings as a file or inline](settings-delivery.md), [Built-in or config-defined agent](custom-agent.md), [Which login kind](login-kind.md).
