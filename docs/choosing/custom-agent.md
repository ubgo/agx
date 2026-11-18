# Built-in or config-defined agent

agx knows four agents in code: Claude Code, Codex, Gemini CLI and pi. Any other coding-agent CLI can be described in config with `agents.<id>.spec`: its binary, how it selects a home, and the argument templates for a model, extra folders, a prompt, each autonomy level and headless mode. A built-in agent can also take anything raw after `--` or in a profile's `args`. This page helps you choose between a config-defined agent and a built-in one with raw args. The steps are in [A custom agent](../guides/custom-agent.md), the spec keys in the [config reference](../config.md#a-custom-agent).

## Short answer

| Your situation | Choose |
|---|---|
| the CLI is Claude Code, Codex, Gemini CLI or pi | the built-in agent; pass what agx does not model as raw args |
| a built-in agent gained a flag agx does not know yet | the built-in agent with the flag after `--` or in the profile's `args` |
| another CLI, and you want profiles, homes, `agx run` and `agx new`, `--model`, `--autonomy` mapped, run folders and `--dry-run` | a config-defined agent |
| another CLI, and you only want to start it | a config-defined agent with just `binary`, `new_args` and a home; or start it yourself |
| usage, conversations (`agx resume`), settings or typed options for another CLI | not possible from config: these need code in agentio |
| token logins for another CLI | a config-defined agent with `token_env` in its spec |

## What a config-defined agent gets

| Feature | Built-in | Config-defined |
|---|---|---|
| profiles, homes, folder bindings, `--dry-run`, run folders, `AGX_RUN_*` | yes | yes |
| several homes | yes | with `home_env` (the variable that selects a home) |
| `--model`, `--add-dir`, `--prompt`, `--headless` | mapped per agent | with `model_args`, `add_dir_args`, `prompt_args`, `headless_args` |
| `--autonomy` | mapped per agent | only the levels in `autonomy_args`; any other level is refused |
| token logins (`--login`, `--token-ref`, `AGX_LOGIN`) | Claude, Gemini CLI, pi | with `token_env`: the kinds it names; no probe, no setup command |
| `--setting`, `--option`, agent blocks | Claude, Codex | no |
| `agx usage`, `--auto-profile` | Claude, Codex | no |
| `agx resume`, `agx conversation` | yes | no |
| `--env`, `--env-file`, `env`, `env_refs`, `before_run`, raw args | yes | yes |

`agx agent show` lists every capability with its reason:

<!-- snippet: examples/choose-custom-agent/session.txt#show -->
```sh
$ agx agent show --id mycli
agent: mycli (My CLI, custom)
binary: pi
path: /usr/local/bin/pi
version: unknown
method: unknown
tested: —
compat: unknown
autonomy: default, read-only, full
token kinds: api-key

CAPABILITY          SUPPORTED  COST     REASON
installer           yes        disk     —
binary-version      no         process  spec has no version args
identity            no         process  config-only agent: a spec describes L1 only; this needs code
usage               no         network  config-only agent: a spec describes L1 only; this needs code
...
token-login         yes        pure     —
...
conversations       no         disk     config-only agent: a spec describes L1 only; this needs code
...
live-credential     no         pure     config-only agent: a spec describes L1 only; this needs code
launch-config       no         disk     config-only agent: a spec has no typed launch options and no settings flag; pass vendor flags as raw args
```

## See it

A spec maps the shared flags to the CLI's own:

<!-- snippet: examples/choose-custom-agent/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true, "args": ["--verbose"] },
    "mine": { "agent": "mycli", "home": "~/.mycli", "default": true }
  },
  "logins": {
    "my-key": { "agents": ["mycli"], "kind": "api-key", "token": "${gopass:mycli/key}" }
  },
  "agents": {
    "mycli": {
      "spec": {
        "name": "My CLI",
        "binary": "pi",
        "default_home": ".mycli",
        "home_env": "MYCLI_HOME",
        "new_args": ["{args}"],
        "model_args": ["--model", "{model}"],
        "autonomy_args": { "read-only": ["--plan"], "full": ["--yolo"] },
        "token_env": [{ "kind": "api-key", "env": "MYCLI_API_KEY" }]
      }
    }
  }
}
```

<!-- snippet: examples/choose-custom-agent/session.txt#launch -->
```sh
$ agx run --profile mine --model big --autonomy read-only --dry-run --quiet --json --field data.command.line
pi --plan --model big

$ agx run --profile mine --autonomy ask --dry-run --quiet
agx: mycli: autonomy ask: not described by the agent spec
[exit 125]
```

A built-in agent with raw args: profile `args` first, then what follows `--`, exactly as given:

<!-- snippet: examples/choose-custom-agent/session.txt#raw -->
```sh
$ agx run --profile personal --dry-run --quiet --json --field data.command.line -- --brand-new-flag
claude --verbose --brand-new-flag
```

## If this, then that

- If you only need one extra flag on a built-in agent, do not define an agent: use `--` or `args`, and switch to `--option` when agx models the flag.
- If the other CLI keeps per-account state in a folder chosen by a variable, set `home_env`; profiles then give you one home per account, as for the built-ins.
- If the CLI needs an API key, describe its variable in `token_env` and add a login per key (`agx login add --agent mycli --kind api-key --token-ref …`); for one fixed key a profile's `env_refs` is enough.
- If you want usage, conversations or a login probe for it, the agent needs an agentio package; a spec cannot describe them.

## Pitfalls

- A level missing from `autonomy_args` is refused, never rounded to another level: `mycli: autonomy ask: not described by the agent spec`. `default` needs no entry, and `full` still needs `--consent full-autonomy`.
- Token logins work only with `token_env` in the spec, and only for the kinds it names; a dry run shows the variable the token fills, redacted:

<!-- snippet: examples/choose-custom-agent/session.txt#login -->
```sh
$ agx run --profile mine --login my-key --dry-run --quiet
...
login    my-key (not resolved: --dry-run)     from --login
...
         set MYCLI_API_KEY=[redacted]         from launch environment

$ agx run --profile mine --token-ref '${gopass:mycli/key}' --token-kind api-key --dry-run --quiet
...
login    one-off token (not resolved: --dry-run)  from --token-ref
...
         set MYCLI_API_KEY=[redacted]             from launch environment

$ agx login add --name my-key2 --agent mycli --kind api-key --token-ref '${gopass:mycli/key}'
added login my-key2 (api-key, reference gopass)
```

- `--setting` and `--option` are refused for a config-defined agent, and its usage row says why:

<!-- snippet: examples/choose-custom-agent/session.txt#limits -->
```sh
$ agx run --profile mine --setting theme=dark --dry-run --quiet
agx: mycli has no --option or --setting (config-only agent: a spec has no typed launch options and no settings flag; pass vendor flags as raw args); pass its flags after --
[exit 125]

$ agx usage --agent mycli
mine  mine · mycli  ~/.mycli
  unsupported: config-only agent: a spec describes L1 only; this needs code
```

- Raw args are never checked: a typo reaches the agent as written, and a raw flag that duplicates a typed choice wins because it comes last.

## Related

- Guide: [Config reference: a custom agent](../config.md#a-custom-agent), [Passing anything to an agent](../launch-options.md#recipe-10-gemini-pi-and-custom-agents), [A custom agent](../guides/custom-agent.md), [Agents and what they support](../guides/agents.md), [Raw args](../guides/raw-args.md).
- Commands: [`agx agent show`](../commands/setup.md#agx-agent-show).
- Example: [choose-custom-agent](../../examples/choose-custom-agent/README.md), [cfg-custom-agent](../../examples/cfg-custom-agent/README.md), [agent-capabilities](../../examples/agent-capabilities/README.md), [launch-raw-args](../../examples/launch-raw-args/README.md).
- Other decisions: [Which way to pass something](passing-options.md), [Which login kind](login-kind.md).
