# Autonomy: how much an agent may do without asking

`--autonomy` sets how much an agent may do on its own, with one word that means the same on every agent: `default`, `read-only`, `ask`, `accept-edits` or `full`. agx maps it to each agent's own permission flags (Claude Code's permission modes, Codex's sandbox and approval policy, Gemini CLI's approval modes, pi's tool allowlist, a custom agent's `autonomy_args`). An agent is never run with more autonomy than you asked for: a level an agent cannot honour exactly is refused, never rounded up.

You want this when you switch between agents and do not want to remember four permission vocabularies, when a profile should always plan only or always edit without asking, or when a script must be sure it never runs an agent unattended by accident.

## Before you start

- A profile per agent you use (see [Getting started](../getting-started.md)).
- Know the five levels:

| Level | Meaning |
|---|---|
| `default` | add nothing: the agent's own default (its settings file decides) |
| `read-only` | look and plan, change nothing |
| `ask` | may change things, asks before each action that needs approval |
| `accept-edits` | edits files without asking, asks for everything else |
| `full` | runs anything without asking; needs `--consent full-autonomy` |

The examples run against this config (from [examples/launch-autonomy](../../examples/launch-autonomy/README.md)):

<!-- snippet: examples/launch-autonomy/config.jsonc#profiles -->
```jsonc
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "reviewer": { "agent": "claude", "home": "~/.claude-work", "autonomy": "accept-edits" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "gemini": { "agent": "gemini", "home": "~/.gemini", "default": true },
    "pi": { "agent": "pi", "home": "~/.pi/agent", "default": true },
```

## Step 1: pick a level for one run

Why: the same word works on every agent, and `--dry-run` shows the exact vendor flag it becomes.

<!-- snippet: examples/launch-autonomy/session.txt#claude -->
```sh
$ agx run --agent claude --autonomy read-only --dry-run --quiet --json --field data.command.line
claude --permission-mode plan
$ agx run --agent claude --autonomy ask --dry-run --quiet --json --field data.command.line
claude --permission-mode manual
$ agx run --agent claude --autonomy accept-edits --dry-run --quiet --json --field data.command.line
claude --permission-mode acceptEdits
$ agx run --agent claude --autonomy full --consent full-autonomy --dry-run --quiet --json --field data.command.line
claude --dangerously-skip-permissions
```

`ask` becomes Claude Code's `manual` permission mode (the spelling Claude Code 2.1.323 lists in its `--help`; settings files may also spell it `default`). `full` needs the consent flag on every agent, because it lets the agent run any command without asking.

## Step 2: the same levels on Codex, Gemini CLI, pi and opencode

Codex maps levels to its sandbox (`-s`) and approval policy (`-a`). It has no edits-only mode, so `accept-edits` is refused rather than mapped to something more permissive:

<!-- snippet: examples/launch-autonomy/session.txt#codex -->
```sh
$ agx run --agent codex --autonomy read-only --dry-run --quiet --json --field data.command.line
codex -s read-only -a on-request
$ agx run --agent codex --autonomy ask --dry-run --quiet --json --field data.command.line
codex -s workspace-write -a on-request
$ agx run --agent codex --autonomy full --consent full-autonomy --dry-run --quiet --json --field data.command.line
codex --dangerously-bypass-approvals-and-sandbox
$ agx run --agent codex --autonomy accept-edits --dry-run --quiet
agx: codex: autonomy accept-edits: Codex has no edits-only mode; never mapped to a more permissive one
[exit 125]
```

Gemini CLI maps every level to an approval mode:

<!-- snippet: examples/launch-autonomy/session.txt#gemini -->
```sh
$ agx run --agent gemini --autonomy read-only --dry-run --quiet --json --field data.command.line
gemini --approval-mode plan
$ agx run --agent gemini --autonomy ask --dry-run --quiet --json --field data.command.line
gemini --approval-mode default
$ agx run --agent gemini --autonomy accept-edits --dry-run --quiet --json --field data.command.line
gemini --approval-mode auto_edit
$ agx run --agent gemini --autonomy full --consent full-autonomy --dry-run --quiet --json --field data.command.line
gemini --approval-mode yolo
```

pi has no permission modes: it always runs without asking. A plain launch says so in a note, `read-only` gives it a read-only tool allowlist and turns off MCP and extensions, and `ask` or `accept-edits` are refused:

<!-- snippet: examples/launch-autonomy/session.txt#pi -->
```sh
$ agx run --agent pi --dry-run --quiet
agent    pi                                   from --agent
profile  pi                                   from default profile of pi
home     ~/.pi/agent                          from profile pi
login    home login                           from default (no login named)
binary   /usr/local/bin/pi version unknown    from no install metadata
folder   ~/projects/app                       from current folder
run      ~/.agx/runs/20261118-060944-wzua     from AGX_RUN_ID=20261118-060944-wzua
command  pi                                   from built by agentio
env      set AGX_RUN_ID=20261118-060944-wzua  from launch environment
         set AGX_RUN_AGENT=pi                 from launch environment
         set AGX_RUN_PROFILE=pi               from launch environment
         set AGX_RUN_LOGIN=home               from launch environment
         set AGX_RUN_HOME=~/.pi/agent         from launch environment
note     pi runs with full autonomy and never asks
$ agx run --agent pi --autonomy read-only --dry-run --quiet --json --field data.command.line
pi --tools read,grep,find,ls --no-mcp --no-extensions
$ agx run --agent pi --autonomy ask --dry-run --quiet
agx: pi: autonomy ask: pi has no permission modes; it always runs without prompts
[exit 125]
```

`full` on pi adds nothing, because that is how pi always runs (the consent flag is still required):

<!-- snippet: examples/launch-autonomy/session.txt#pi-full -->
```sh
$ agx run --agent pi --autonomy full --consent full-autonomy --dry-run --quiet --json --field data.command.line
pi
```

opencode has one flag for this: `full` becomes `opencode --auto` (approve every permission that is not explicitly denied). `read-only`, `ask` and `accept-edits` are permission rules in its `opencode.json`, not flags, so they are refused (exit 125) rather than guessed; `default` leaves its configured permissions in charge.

`default` adds nothing on any agent:

<!-- snippet: examples/launch-autonomy/session.txt#default -->
```sh
$ agx run --agent claude --autonomy default --dry-run --quiet --json --field data.command.line
claude
```

### The whole map

| Level | Claude Code | Codex | Gemini CLI | pi | custom agent |
|---|---|---|---|---|---|
| `default` | nothing | nothing | nothing | nothing (with a note that pi never asks) | `autonomy_args.default`, or nothing when not listed |
| `read-only` | `--permission-mode plan` | `-s read-only -a on-request` | `--approval-mode plan` | `--tools read,grep,find,ls --no-mcp --no-extensions` | `autonomy_args.read-only` |
| `ask` | `--permission-mode manual` | `-s workspace-write -a on-request` | `--approval-mode default` | refused | `autonomy_args.ask` |
| `accept-edits` | `--permission-mode acceptEdits` | refused | `--approval-mode auto_edit` | refused | `autonomy_args.accept-edits` |
| `full` | `--dangerously-skip-permissions` | `--dangerously-bypass-approvals-and-sandbox` | `--approval-mode yolo` | nothing | `autonomy_args.full` |

A custom agent supports exactly the levels its `autonomy_args` lists (and `default`, which adds nothing unless listed); any other level is refused (exit 125) with a message that names the agent and the level: `mycli: autonomy ask: not described by the agent spec`. (`agx agent show` may list fewer levels for Claude Code and Codex than this table; the launch behaviour above is what counts, and `--dry-run` always shows it.)

## Step 3: set a level for every launch of a profile

Why: a review account that should always edit without asking, or a shared machine profile that should always plan only.

```jsonc
"reviewer": { "agent": "claude", "home": "~/.claude-work", "autonomy": "accept-edits" }
```

The dry run names the profile as the source, and `--autonomy` on the command line overrides it for one run:

<!-- snippet: examples/launch-autonomy/session.txt#profile -->
```sh
$ agx run --profile reviewer --dry-run --quiet
agent    claude                                 from profile reviewer
profile  reviewer                               from --profile
home     ~/.claude-work                         from profile reviewer
login    home login                             from default (no login named)
binary   /usr/local/bin/claude version unknown  from no install metadata
folder   ~/projects/app                         from current folder
options  autonomy=accept-edits                  from profile reviewer
run      ~/.agx/runs/20261118-060944-nph4       from AGX_RUN_ID=20261118-060944-nph4
command  claude --permission-mode acceptEdits   from built by agentio
env      set AGX_RUN_ID=20261118-060944-nph4    from launch environment
         set AGX_RUN_AGENT=claude               from launch environment
         set AGX_RUN_PROFILE=reviewer           from launch environment
         set AGX_RUN_LOGIN=home                 from launch environment
         set AGX_RUN_HOME=~/.claude-work        from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work   from launch environment
note     installed Claude Code version unknown; flags not checked
$ agx run --profile reviewer --autonomy read-only --dry-run --quiet --json --field data.command.line
claude --permission-mode plan
```

`profiles.<profile>.autonomy` takes the same five words. `full` in a profile still needs `--consent full-autonomy` on each command line: consent is never stored.

## Step 4: a custom agent's levels

Why: agx cannot know a config-defined agent's flags, so its spec lists them per level. A level not listed is refused.

<!-- snippet: examples/launch-autonomy/config.jsonc#custom -->
```jsonc
  "agents": {
    "mycli": {
      "spec": {
        "name": "My CLI",
        "binary": "pi",
        "default_home": ".mycli",
        "home_env": "MYCLI_HOME",
        "new_args": ["{args}"],
        "autonomy_args": { "default": [], "read-only": ["--plan"], "full": ["--yolo"] }
      }
    }
  }
```

<!-- snippet: examples/launch-autonomy/session.txt#custom -->
```sh
$ agx run --agent mycli --autonomy read-only --dry-run --quiet --json --field data.command.line
pi --plan
$ agx run --agent mycli --autonomy ask --dry-run --quiet
agx: mycli: autonomy ask: not described by the agent spec
[exit 125]
```

The config also has a profile `"mine": { "agent": "mycli", "home": "~/.mycli", "default": true }`. (The example machine has no `mycli`, so its spec borrows the stand-in `pi` binary; on your machine `binary` names the real executable.) See [a custom agent](../config.md#a-custom-agent) for the whole spec.

## Try it

From the repository:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-autonomy$'
```

On your own machine, without starting anything:

```sh
agx run --agent codex --autonomy ask --dry-run --quiet --json --field data.command.line
```

## Pitfalls

Every refusal below exits 125 (a launching command) and starts nothing.

<!-- snippet: examples/launch-autonomy/session.txt#consent -->
```sh
$ agx run --agent claude --autonomy full --dry-run --quiet
agx: --autonomy full needs --consent full-autonomy
[exit 125]
$ agx run --agent claude --autonomy everything --dry-run --quiet
agx: --autonomy "everything": use one of default, read-only, ask, accept-edits, full
[exit 125]
$ agx run --agent claude --autonomy full --consent yes --dry-run --quiet
agx: --consent "yes": the only consent is full-autonomy
[exit 125]
```

- `full` without `--consent full-autonomy`, an unknown level and any other consent word are refused, as above.
- A level the agent cannot honour exactly is refused with the agent's reason: `agx: Codex has no edits-only mode; never mapped to a more permissive one`, `agx: pi has no permission modes; it always runs without prompts`. Pick the next stricter level (`ask`) or `full` with consent.
- One concern, one winner. `--autonomy` and a typed option for the same thing in one command are refused; a typed option on the command line beats the profile's `autonomy`; a raw arg after `--` is passed as written, last, so the agent uses it, even when it contradicts the level:

<!-- snippet: examples/launch-autonomy/session.txt#concern -->
```sh
$ agx run --agent claude --autonomy ask --option permission_mode=plan --dry-run --quiet
agx: --autonomy and --option permission_mode both set the autonomy; pick one
[exit 125]
$ agx run --profile reviewer --option permission_mode=plan --dry-run --quiet --json --field data.command.line
claude --permission-mode plan
$ agx run --agent claude --autonomy read-only --dry-run --quiet --json --field data.command.line -- --dangerously-skip-permissions
claude --permission-mode plan --dangerously-skip-permissions
```

- A script that reads `--field data.command.line` gets nothing useful from a refusal; read `error.message` (or `error.code`) instead:

<!-- snippet: examples/launch-autonomy/session.txt#field-error -->
```sh
$ agx run --agent codex --autonomy accept-edits --dry-run --quiet --json --field error.message
codex: autonomy accept-edits: Codex has no edits-only mode; never mapped to a more permissive one
[exit 125]
```

- `default` is not "safe": it is whatever the agent's own settings say. Use `read-only` or `ask` when you need a known level.

## Related

- Reference: [`agx run`](../commands/launch.md#agx-run) (`--autonomy`, `--consent`), [config: profiles](../config.md#profiles) (`autonomy`), [agent blocks](../config.md#agent-blocks) (`permission_mode`, `sandbox`, `approval`)
- Example: [examples/launch-autonomy](../../examples/launch-autonomy/README.md)
- [Passing anything to an agent](../launch-options.md#one-winner-per-concern): one winner per concern
- [Headless runs](headless.md): autonomy in scripts and CI
- [Raw args](raw-args.md): flags agx does not model
