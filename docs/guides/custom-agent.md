# A custom agent

agx ships with Claude Code, Codex, Gemini CLI, pi and opencode. Any other agent CLI can be described in `config.jsonc` under `agents.<agent>.spec`: its binary, its config folder, and how its flags are spelled. agx then launches it like a built-in agent: profiles and homes, the folder, `--model`, `--prompt`, `--add-dir`, `--autonomy`, `--headless`, token logins (with `token_env`), raw args, `--env`, `before_run` and `--dry-run` all work.

Use it when you want one launcher for every agent you run, or a profile per account for an agent agx does not know yet. What a description cannot give you is anything that needs code that reads the agent's files: plan usage, conversation lists and resume, settings and typed options, and a login probe. Those are reported as not supported, with the reason, rather than half-working.

## Before you start

- The agent's binary on your `PATH`, and its config folder (its home) on disk.
- The agent's own flags for a model, a first prompt, an extra folder, resuming, and each permission mode you want to map. Its `--help` lists them.
- The variable that points it at another config folder, if it has one (such as `MYCLI_HOME`), so you can keep several homes.

The example on this page, [`examples/cfg-custom-agent`](../../examples/cfg-custom-agent/README.md), describes an agent called `mycli` whose binary is `echo`, so every launch prints the arguments agx built. Put your agent's real binary name in `binary`.

## Step 1: describe the agent

Add the agent under `agents`. The map key (`mycli`) is the id you pass to `--agent` and write in profiles; it may not be a built-in id.

<!-- snippet: examples/cfg-custom-agent/config.jsonc#agents -->
```jsonc
  "agents": {
    "mycli": {
      // The stand-in binary is echo, so a launch prints the arguments agx built.
      // For a real agent, binary is its executable name, such as "aider".
      "spec": {
        "name": "My CLI",
        "binary": "echo",
        "default_home": ".mycli",
        "home_env": "MYCLI_HOME",
        "home_markers": ["config.json"],
        "new_args": ["{args}"],
        "resume_args": ["--resume", "{id}", "{args}"],
        "model_args": ["--model", "{model}"],
        "prompt_args": ["--prompt", "{prompt}"],
        "add_dir_args": ["--add-dir", "{dir}"],
        "autonomy_args": { "ask": [], "full": ["--yolo"] },
        "token_env": [{ "kind": "api-key", "env": "MYCLI_API_KEY" }],
        "process": { "basenames": ["mycli"] }
      },
      "defaults": { "args": ["--color=always"] }
    },
    // An agent whose binary is not installed.
    "nocli": {
      "spec": { "name": "Not installed", "binary": "nocli", "default_home": ".nocli", "new_args": ["{args}"] }
    }
  },
```

The templates turn agx's neutral options into the agent's flags:

| Key | Placeholder | Used for |
|---|---|---|
| `new_args` | `{args}` | every new session; `{args}` is replaced by everything below, in this order |
| `autonomy_args` | none | the args for each `--autonomy` level; a level not listed is refused |
| `headless_args` | none | the args for `--headless`; without it `--headless` is refused |
| `model_args` | `{model}` | `--model` and profile `model` |
| `add_dir_args` | `{dir}` | each `--add-dir` |
| (raw args) | | `agents.<agent>.defaults.args`, profile `args`, then everything after `--` |
| `prompt_args` | `{prompt}` | `--prompt` |
| `resume_args` | `{id}`, `{args}` | a resume; see the pitfalls |

A request with no template (a `--model` without `model_args`, say) is refused, never dropped. Every key is in the [config reference](../config.md#a-custom-agent).

## Step 2: add a profile for each home

A profile names a home of the agent. The home must exist; `home_markers` are the files that make a folder look like a home to `agx profile list --discovered` and `agx doctor`.

<!-- snippet: examples/cfg-custom-agent/config.jsonc#profiles -->
```jsonc
  "profiles": {
    "my": { "agent": "mycli", "home": "~/.mycli", "default": true },
    "my-work": { "agent": "mycli", "home": "~/.mycli-work" },
    "no": { "agent": "nocli", "home": "~/.nocli", "default": true }
  },
```

`my` is on the agent's default home (`default_home`, relative to your user home), so agx leaves `MYCLI_HOME` unset for it, the same rule as the built-in agents. `my-work` is another home, so agx sets `MYCLI_HOME` to it.

## Step 3: check it

`agx config check` validates the spec with the rest of the file, and `agx agent show` lists what the description gives you:

<!-- snippet: examples/cfg-custom-agent/session.txt#check -->
```sh
$ agx config check
~/.agx/config.jsonc: ok (2 agents, 3 profiles, 1 login, 0 projects, 0 aliases)
```

<!-- snippet: examples/cfg-custom-agent/session.txt#list -->
```sh
$ agx agent list --field data.*.id --field data.*.kind --field data.*.binary --field data.*.tested
ID        KIND      BINARY    TESTED
claude    built-in  claude    2.1.323..2.1.329
codex     built-in  codex     0.162.1..0.162.1
gemini    built-in  gemini    0.68.0..0.68.0
pi        built-in  pi        1.5.2..1.5.2
opencode  built-in  opencode  1.18.30..1.18.30
mycli     custom    echo      —
nocli     custom    nocli     —
```

<!-- snippet: examples/cfg-custom-agent/session.txt#show -->
```sh
$ agx agent show --id mycli --json --field data.kind --field data.autonomy --field data.token_kinds
{
  "data.kind": "custom",
  "data.autonomy": [
    "default",
    "ask",
    "full"
  ],
  "data.token_kinds": [
    "api-key"
  ]
}
```

The autonomy levels are `default` plus the keys of `autonomy_args`. The token kinds are the kinds `token_env` names (step 6). `agx agent show --id mycli` without `--json` also lists every capability with the reason it is missing, such as `config-only agent: a spec describes L1 only; this needs code`.

## Step 4: preview a launch

`--dry-run` shows every decision and the command, and starts nothing. The `binary` line (elided here as `...`) shows where on `PATH` the binary was found:

<!-- snippet: examples/cfg-custom-agent/session.txt#dry-run -->
```sh
$ agx run --agent mycli --dry-run --quiet
agent    mycli                                from --agent
profile  my                                   from default profile of mycli
home     ~/.mycli                             from profile my
login    home login                           from default (no login named)
...
folder   ~/projects/app                       from current folder
args     --color=always                       from agents.mycli.defaults.args
run      ~/.agx/runs/20261118-061258-ju3b     from AGX_RUN_ID=20261118-061258-ju3b
command  echo --color=always                  from built by agentio
env      set AGX_RUN_ID=20261118-061258-ju3b  from launch environment
         set AGX_RUN_AGENT=mycli              from launch environment
         set AGX_RUN_PROFILE=my               from launch environment
         set AGX_RUN_LOGIN=home               from launch environment
         set AGX_RUN_HOME=~/.mycli            from launch environment
```

On the non-default home the home variable is set:

<!-- snippet: examples/cfg-custom-agent/session.txt#dry-run-work -->
```sh
$ agx run --profile my-work --dry-run --quiet --json --field data.command
{"line":"echo --color=always","path":"echo","args":["--color=always"],"dir":"~/projects/app","env":["set AGX_RUN_ID=20261118-061258-sbgk","set AGX_RUN_AGENT=mycli","set AGX_RUN_PROFILE=my-work","set AGX_RUN_LOGIN=home","set AGX_RUN_HOME=~/.mycli-work","set MYCLI_HOME=~/.mycli-work"]}
```

## Step 5: launch it

agx replaces itself with the agent. Here the agent is `echo`, so the output is the argument list:

<!-- snippet: examples/cfg-custom-agent/session.txt#run -->
```sh
$ agx run --profile my --model big --prompt "fix the tests" --add-dir ~/projects/client
--model big --add-dir ~/projects/client --color=always --prompt fix the tests
```

Autonomy goes through `autonomy_args`; `full` still needs `--consent full-autonomy`, and a level you did not map is refused:

<!-- snippet: examples/cfg-custom-agent/session.txt#autonomy -->
```sh
$ agx run --profile my --autonomy ask
--color=always

$ agx run --profile my --autonomy full --consent full-autonomy
--yolo --color=always

$ agx run --profile my --autonomy read-only
agx: mycli: autonomy read-only: not described by the agent spec
[exit 125]
```

## Step 6: a token login

Why: run the agent on a stored key per account, chosen per run, without putting the key in a profile.

`token_env` in the spec says which variable a login fills, per kind (and per provider, when the agent reads one variable per provider). Add a login for the agent and use it like any other ([Logins](../logins.md)):

<!-- snippet: examples/cfg-custom-agent/session.txt#login-add -->
```sh
$ agx login add --name my-key-2 --agent mycli --kind api-key --token-ref '${MYCLI_KEY}'
added login my-key-2 (api-key, env MYCLI_KEY)

$ agx login add --name plan --agent mycli --kind oauth-token --token-ref '${MYCLI_KEY}'
agx: mycli has no oauth-token login; it takes api-key
[exit 4]

$ agx login check --id my-key
NAME    KIND     SOURCE            RESOLVE  DETECTED  PROBE    DETAIL
my-key  api-key  reference gopass  ok       unknown   skipped  mycli: config-only agent: a spec describes L1 only; this needs code
```

A dry run shows the variable the token goes into, redacted, and resolves nothing:

<!-- snippet: examples/cfg-custom-agent/session.txt#login -->
```sh
$ agx run --profile my --login my-key --dry-run
...
login    my-key (not resolved: --dry-run)     from --login
...
command  echo --color=always                  from built by agentio
env      set AGX_RUN_ID=20261114-101500-k3x9  from launch environment
         set AGX_RUN_AGENT=mycli              from launch environment
         set AGX_RUN_PROFILE=my               from launch environment
         set AGX_RUN_LOGIN=my-key             from launch environment
         set AGX_RUN_HOME=~/.mycli            from launch environment
         set MYCLI_API_KEY=[redacted]         from launch environment
```

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-custom-agent$'
```

[`examples/cfg-custom-agent/session.txt`](../../examples/cfg-custom-agent/session.txt) holds every command above with its real output. To try your own description without starting the agent, set `binary` to `echo` as the example does: each launch prints the arguments agx built.

## Pitfalls

- **Token logins need `token_env`.** Without it the agent has no token login (`spec has no token_env`), and `--login`, `--token-ref` and `agx login add --agent mycli` are refused. With it, only the kinds it names are accepted (`mycli has no oauth-token login; it takes api-key`, exit 4); the first is the default kind of `agx login add`.
- **One variable per provider needs `--provider`.** When `token_env` entries name a `provider`, a login must name one of them, as for pi; with no provider in the entries, `--provider` is refused.
- **`scrub` lists what would outrank the token.** agx removes those variables from the agent's environment when the login is used, so a key exported in your shell cannot silently win; list every variable the CLI reads before yours.
- **No probe and no setup command.** `agx login check` resolves the reference but skips the probe (`config-only agent: …`), `--auto-login` has no headroom to compare, and `--login-setup` has no command to run: add the key with `--token-ref` or `--stdin`. `--live-login` is Claude Code only.
- **No resume.** `agx resume` finds a conversation through the agent's conversation reader, which a description does not have, so it refuses the agent up front, before any search, when you chose it (`--agent mycli` or a profile of it) or when no other home is left to search (exit 125). Next to agents that do list conversations, an unchosen custom agent is named in the not-found hint instead. `resume_args` is kept for when a reader exists; start the agent's own resume with raw args: `agx run --profile my -- --resume ID`.

  <!-- snippet: examples/cfg-custom-agent/session.txt#resume -->
  ```sh
  $ agx resume --agent mycli --id abc123 --dry-run
  agx: mycli cannot resume conversations: config-only agent: a spec describes L1 only; this needs code
  [exit 125]
  ```
- **A missing template is refused.** `--autonomy read-only` without a `read-only` entry, or `--headless` without `headless_args`, stops before the agent starts (exit 125). The message names the agent and what it lacks, `agx: mycli: autonomy read-only: not described by the agent spec`; add that entry to the agent's `spec` or drop the flag.

  <!-- snippet: examples/cfg-custom-agent/session.txt#headless -->
  ```sh
  $ agx run --profile my --headless
  agx: mycli: headless: not described by the agent spec
  [exit 125]
  ```

- **A binary that is not installed stops even a dry run** with exit 127:

  <!-- snippet: examples/cfg-custom-agent/session.txt#missing -->
  ```sh
  $ agx run --agent nocli --dry-run
  agx: nocli not found on PATH
  [exit 127]
  ```

- **No usage, settings or typed options.** `agx usage` shows the agent's homes as `unsupported`; `--setting`, `--option` and a profile agent block are refused; `--auto-profile` cannot pick it. Pass any vendor flag as a raw arg: in `agents.<agent>.defaults.args`, profile `args`, or after `--`.
- **A built-in id cannot be redefined.** `agents.claude.spec` fails the load; give your description its own id.
- **`binary` is a name looked up on `PATH`**, or an absolute path. A leading `~` is not expanded.
- **`--vendor-checks` runs `version_args`.** Leave `version_args` out if the agent's version command has side effects; `agx agent show` then says `spec has no version args`.

## Related

- [Agents and what they support](agents.md): the built-in agents' capabilities, and how to read a refusal.
- [Built-in or config-defined agent](../choosing/custom-agent.md): when a description is worth it.
- [Autonomy](autonomy.md), [raw args](raw-args.md), [headless runs](headless.md).
- Reference: [`agents.<agent>.spec`](../config.md#a-custom-agent), [`agents.<agent>.defaults`](../config.md#agents), [`agx agent show`](../commands/setup.md#agx-agent-show).
- Example: [`examples/cfg-custom-agent`](../../examples/cfg-custom-agent/README.md).
