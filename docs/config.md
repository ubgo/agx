# Config reference

agx keeps its own settings in one JSONC file (JSON with comments and trailing commas). This page names every key. The docs gate checks it against the schema agx validates with, so a key missing here fails the build.

## Files

| File | Holds | Mode |
|---|---|---|
| `config.jsonc` | profiles, logins, projects, aliases, agent defaults and custom agents | 0644 |
| `tokens.jsonc` | pasted token values keyed by login name; never expanded | 0600, checked |
| `runs/<run id>/` | one folder per launch: `run.jsonc`, the delivered settings file, live-login state | 0700 |
| `cache/login-usage.jsonc` | the last probe of each [login pool](guides/login-pool.md) member (windows, status, account label; never a token), shared by every agx process; rebuilt at any time | 0600, folder 0700 |
| `cache/login-usage-history.jsonc` | a point per member for every answered probe (at most one every 10 minutes, 8 days kept; never a token), the [usage history](guides/pool-rules.md#usage-history); safe to delete | 0600, folder 0700 |
| `<file>.lock` | the lock file agx keeps next to each file it writes, so two agx processes never write at once; never removed, by design | 0600 |

The folder is `$AGX_CONFIG_DIR`, else `$AGX_HOME`, else `~/.agx`; an empty variable counts as unset, and agx expands a leading `~` and `$VAR` in either. `--config FILE` on any command reads that file instead, with `tokens.jsonc` and `runs/` next to it. `agx config path` prints the file in use, and with `--json` the rule that chose it:

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

## How agx reads and writes it

- **Checked on every load.** An unknown key or a wrong type fails with its line and column (`config.jsonc:12:5: unknown key "profles"`), and `agx config check` lists every problem at once (exit 1 for a file that does not read as config, 2 for a cross-check problem; see [fix what fails](guides/doctor.md#step-4-fix-what-fails)). After the shape, agx cross-checks: a missing login, agent or profile, two default profiles for one agent, a login that does not serve a profile's agent, a binding naming another agent's profile, both `login` and `home_login` in one place, an alias that is not a valid command, and an `env_refs` value that is not a reference.
- **Your layout is kept.** Commands that write (`profile add`, `login use`, `project use`, `config set`, …) keep your comments, key order and formatting, and write only when something changed. A change that would make the file invalid is refused before anything is written.
- **Editor completion.** Set `"$schema"` to the published schema, `"https://raw.githubusercontent.com/ubgo/agx/main/schema/config.schema.json"` (`schema/config.schema.json` in this repository), and your editor completes and checks keys as you type. `agx config init` writes that line for you.
- **Version.** A file with a `version` newer than this agx understands is refused rather than half-read (`error.code` `config-version`).

## Top level

| Key | Type | Meaning |
|---|---|---|
| `$schema` | string | the editor schema URL; agx ignores it |
| `version` | integer | the file format version; `1` |
| `default_agent` | agent id | the agent used when no flag or folder binding names one; unset means ask at a terminal and refuse without one |
| `sessions` | object | where session folders live (below) |
| `agents` | map of agent id to object | per-agent defaults, and custom agent definitions |
| `profiles` | map of name to object | each profile: one home plus launch defaults |
| `logins` | map of name to object | named logins |
| `projects` | map of folder to map of agent id to object | folder bindings |
| `aliases` | map of name to string | shell aliases: named-flag agx arguments, such as `"run --profile work"` |
| `date_format` | string | how every human date and time reads: a preset (`iso`, `short`, `long`, `us`) or tokens ([below](#date-formats)); unset keeps each screen's own (`long` in `agx usage`, `iso` in table timestamps and `conversation show`); `--date-format` overrides it for one run |
| `usage` | object | how `agx usage` shows its readings (below) |

## Sessions

| Key | Type | Meaning |
|---|---|---|
| `sessions.root` | path | where `agx new` creates timestamped folders; default `<config dir>/sessions` (see [session folders](guides/session-folders.md)) |
| `sessions.promote_root` | path | the default parent folder for `agx session promote` (see [promote](guides/session-folders.md#step-5-promote-a-session-folder-into-a-project)) |

## Agents

`agents.<agent>` holds defaults for an agent (built in or custom), and, for a custom agent, its definition.

| Key | Type | Meaning |
|---|---|---|
| `agents.<agent>.defaults` | object | launch defaults for every profile of this agent |
| `agents.<agent>.defaults.args` | list of strings | raw args every launch gets first, unless a profile sets `inherit_defaults` to false (see [raw args](guides/raw-args.md#step-1-args-for-every-launch-of-an-agent)) |
| `agents.<agent>.defaults.login` | login name | the agent's default login; unset means the home's own login |
| `agents.<agent>.defaults.home_login` | bool | use the home's own login (accepted for symmetry with profiles and bindings) |
| `agents.<agent>.spec` | object | defines a custom agent (below); not allowed for a built-in id |

Defaults apply to every profile of the agent; a profile replaces them with its own `login` or `home_login`, drops the args with `"inherit_defaults": false`, and a flag (`--login`, `--home-login`, args after `--`) wins over both. `login` and `home_login` together in one place are refused.

<!-- snippet: examples/cfg-agent-defaults/config.jsonc#agents -->
```jsonc
  "agents": {
    "claude": {
      // every Claude profile starts with these, unless it says otherwise
      "defaults": { "args": ["--chrome"], "login": "carol" }
    }
  },
```

<!-- snippet: examples/cfg-agent-defaults/session.txt#personal -->
```sh
$ agx run --profile personal --dry-run --quiet
agent    claude                                  from profile personal
profile  personal                                from --profile
home     ~/.claude                               from profile personal
login    carol (not resolved: --dry-run)         from agents.claude.defaults
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/app                          from current folder
args     --chrome                                from agents.claude.defaults.args
run      ~/.agx/runs/20261118-062216-66fo        from AGX_RUN_ID=20261118-062216-66fo
command  claude --chrome                         from built by agentio
env      set AGX_RUN_ID=20261118-062216-66fo     from launch environment
         set AGX_RUN_AGENT=claude                from launch environment
         set AGX_RUN_PROFILE=personal            from launch environment
         set AGX_RUN_LOGIN=carol                 from launch environment
         set AGX_RUN_HOME=~/.claude              from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]  from launch environment
note     installed Claude Code version unknown; flags not checked
```

[`examples/cfg-agent-defaults`](../examples/cfg-agent-defaults/README.md) shows a profile that keeps the defaults, one that replaces them, and one on the home login.

### A custom agent

Problem: you use an agent CLI agx does not ship with.

Fix: describe it. agx then launches it with profiles and homes, `--model`, `--prompt`, `--add-dir`, `--autonomy` and `--headless` through the templates below, raw args, env and `before_run`. Token logins work through `token_env`. Usage, conversations (so `agx resume` refuses it up front: `mycli cannot resume conversations: config-only agent: …`), settings and typed options need code, so they are reported as not supported. The [custom agent guide](guides/custom-agent.md) builds one step by step, and [`examples/cfg-custom-agent`](../examples/cfg-custom-agent/README.md) runs it.

```jsonc
"agents": {
  "mycli": {
    "spec": {
      "name": "My CLI",
      "binary": "mycli",
      "default_home": ".mycli",
      "home_env": "MYCLI_HOME",
      "home_markers": ["config.json"],
      "new_args": ["{args}"],
      "resume_args": ["--resume", "{id}", "{args}"],
      "model_args": ["--model", "{model}"],
      "prompt_args": ["--prompt", "{prompt}"],
      "add_dir_args": ["--add-dir", "{dir}"],
      "version_args": ["--version"],
      "autonomy_args": { "ask": [], "full": ["--yolo"] },
      "token_env": [{ "kind": "api-key", "env": "MYCLI_API_KEY" }],
      "process": { "basenames": ["mycli"] }
    },
    "defaults": { "args": ["--color=always"] }
  }
}
```

| Key | Type | Meaning |
|---|---|---|
| `agents.<agent>.spec.id` | string | optional; the map key is the id |
| `agents.<agent>.spec.name` | string | display name |
| `agents.<agent>.spec.binary` | string | the executable looked up on `PATH` |
| `agents.<agent>.spec.default_home` | path | the home folder, relative to your user home |
| `agents.<agent>.spec.extra_homes` | pattern | a glob for more homes, used by `profile list --discovered` |
| `agents.<agent>.spec.home_markers` | list of strings | files that show a folder is a home |
| `agents.<agent>.spec.generic_home_markers` | list of strings | the `home_markers` other tools write too (`settings.json`, `sessions`): alone they make a home only in the agent's own folders (`default_home`, an `extra_homes` match, the `home_env` folder), never in a folder found by `--search`. Each must be in `home_markers`, and one marker must stay distinctive |
| `agents.<agent>.spec.home_env` | string | the variable that points the agent at a home; left unset for the default home (see [home variables](guides/run-folders.md#step-3-know-which-home-variable-the-agent-gets)) |
| `agents.<agent>.spec.home_env_names_parent` | bool | the variable names the parent of the home folder, not the folder itself |
| `agents.<agent>.spec.new_args` | list of strings | argv template for a new session; `{args}` is the raw args |
| `agents.<agent>.spec.resume_args` | list of strings | argv template for a resume; `{id}` is the conversation. Unused today: `agx resume` needs a conversation reader, which a spec has none of, so it refuses the agent up front (`mycli cannot resume conversations: config-only agent: a spec describes L1 only; this needs code`, exit 125) |
| `agents.<agent>.spec.model_args` | list of strings | template for `--model`; `{model}` |
| `agents.<agent>.spec.prompt_args` | list of strings | template for `--prompt`; `{prompt}` |
| `agents.<agent>.spec.add_dir_args` | list of strings | template for each `--add-dir`; `{dir}` |
| `agents.<agent>.spec.autonomy_args` | map of level to list | args per `--autonomy` level; a level not listed is refused (see [a custom agent's levels](guides/autonomy.md#step-4-a-custom-agents-levels)) |
| `agents.<agent>.spec.headless_args` | list of strings | args for `--headless` |
| `agents.<agent>.spec.token_env` | list of objects | how a token login reaches the agent: the kinds it names are the agent's token login kinds; without it the agent has no token login |
| `agents.<agent>.spec.token_env[].kind` | login kind | the login kind this entry serves |
| `agents.<agent>.spec.token_env[].provider` | string | the provider, for agents with one variable per provider |
| `agents.<agent>.spec.token_env[].env` | string | the variable the token goes into |
| `agents.<agent>.spec.token_env[].scrub` | list of strings | variables removed when this login is used, because they would outrank it |
| `agents.<agent>.spec.credential_env` | list of strings | variables that carry a credential (removed for a home login) |
| `agents.<agent>.spec.api_billing_env` | list of strings | variables that switch the agent to pay-per-token billing |
| `agents.<agent>.spec.version_args` | list of strings | args that print the version (only run with `--vendor-checks`) |
| `agents.<agent>.spec.packages` | list of strings | package names whose install metadata gives the version without running the binary |
| `agents.<agent>.spec.login_args` | list of strings | args of the agent's own login command |
| `agents.<agent>.spec.logout_args` | list of strings | args of the agent's own logout command |
| `agents.<agent>.spec.shared_home_files` | list of strings | files `profile add` may link from the default home into a new one |
| `agents.<agent>.spec.process` | object | how to recognise the agent's running processes |
| `agents.<agent>.spec.process.basenames` | list of strings | executable names |
| `agents.<agent>.spec.process.argv_contains` | list of strings | an argument that must appear (for agents run through an interpreter) |
| `agents.<agent>.spec.process.exclude` | list of strings | arguments that mark a process as not the agent |
| `agents.<agent>.spec.process.script_basenames` | list of strings | names of the script an interpreter runs (`node /opt/bin/mycli` matches `mycli`), for an npm install started through node |
| `agents.<agent>.spec.process.script_contains` | list of strings | a substring of that script's path, such as `@acme/mycli/`; narrower than `argv_contains`, which also matches an `npm install` of the package |
| `agents.<agent>.spec.process.interpreters` | list of strings | the programs that run such a script; absent means node, nodejs, bun and deno (with `.exe` on Windows) |

## Profiles

A profile names one home plus launch defaults. Several profiles may share a home (see [shared homes](how-agx-decides.md#shared-homes)). `agent` and `home` are required; `default` marks the agent's default profile (one per agent); `billing` says how the account pays.

```jsonc
"profiles": {
  "personal": { "agent": "claude", "home": "~/.claude", "default": true },
  "work": { "agent": "claude", "home": "~/.claude-work", "login": "carol", "model": "opus" },
  // Moonshot through the Anthropic API shape; shares ~/.claude with personal
  "kimi": {
    "agent": "claude",
    "home": "~/.claude",
    "billing": "api",
    "inherit_defaults": false,
    "args": ["--dangerously-skip-permissions"],
    "env": { "ANTHROPIC_BASE_URL": "https://api.moonshot.ai/anthropic" },
    "env_refs": { "ANTHROPIC_AUTH_TOKEN": "${gopass:moonshot/kimi-token}" }
  }
}
```

| Key | Type | Meaning |
|---|---|---|
| `profiles.<profile>.agent` | agent id | required; the agent this profile runs |
| `profiles.<profile>.home` | path | required; the agent's config folder (`~` and `${VAR}` expanded). It must exist |
| `profiles.<profile>.default` | bool | the agent's default profile, and the profile a shared home resolves to; one per agent |
| `profiles.<profile>.billing` | `plan` or `api` | `plan` (the default) or `api` (pays per token); `--auto-profile` skips `api` profiles, and on a shared home without a default the `plan` profile is the one chosen (see [shared homes](guides/shared-homes.md)) |
| `profiles.<profile>.login` | login name | the profile's default login; unset inherits the agent default, then the home login |
| `profiles.<profile>.home_login` | bool | use the home's own login even when an agent default names a login |
| `profiles.<profile>.args` | list of strings | raw args after the agent defaults (see [raw args](guides/raw-args.md#step-2-args-for-one-profile)) |
| `profiles.<profile>.inherit_defaults` | bool | false drops `agents.<agent>.defaults.args` for this profile; default true (see [raw args](guides/raw-args.md#step-4-a-profile-without-the-agent-defaults)) |
| `profiles.<profile>.model` | string | the default model |
| `profiles.<profile>.autonomy` | `default`, `read-only`, `ask`, `accept-edits`, `full` | the default autonomy level (see [autonomy](guides/autonomy.md#step-3-set-a-level-for-every-launch-of-a-profile)) |
| `profiles.<profile>.env` | map | literal environment values, never expanded |
| `profiles.<profile>.env_refs` | map | environment values that are references (`${gopass:…}`, `$(…)`, `${VAR}`), expanded strictly at launch |
| `profiles.<profile>.env_file` | path | a dotenv file, relative to the config dir |
| `profiles.<profile>.env_file_commands` | bool | allow `$(…)` inside `env_file`; default false |
| `profiles.<profile>.before_run` | path | a script run before every launch of this profile |
| `profiles.<profile>.before_run_from` | `config` or `folder` | where `before_run` is found: the config dir (default) or the launch folder |
| `profiles.<profile>.before_run_env` | `safe` or `full` | `safe` (default) hides credential variables from the script |
| `profiles.<profile>.settings_delivery` | `file` or `inline` | how settings reach the agent; default `file` |
| `profiles.<profile>.live_login` | bool | start every launch switchable (see [live login](guides/live-login.md)) |

How defaults, a profile's own `login`, `home_login`, `args` and `billing` combine:

<!-- snippet: examples/cfg-agent-defaults/config.jsonc#profiles -->
```jsonc
  "profiles": {
    // inherits both defaults: --chrome and the login carol
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    // its own login, and its own args instead of the defaults
    "work": { "agent": "claude", "home": "~/.claude-work", "login": "dave", "inherit_defaults": false, "args": ["--verbose"] },
    // the home's own login, even though the agent default names one
    "own": { "agent": "claude", "home": "~/.claude-work", "home_login": true, "billing": "api" }
  },
```

<!-- snippet: examples/cfg-agent-defaults/session.txt#list -->
```sh
$ agx profile list
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   carol       plan
work       claude    ~/.claude-work           none   dave        plan
own        claude    ~/.claude-work           none   home login  api
```

The `LOGIN` column shows the login a plain run of the profile uses: its own `login` or `home_login`, else the agent default (`personal` runs on `carol` from `agents.claude.defaults`), else the home login. A folder binding, `AGX_LOGIN` or `--login` can still change it at launch, as the dry run shows.

### Agent blocks

A profile may hold one block named after its agent with that agent's typed launch options and settings: `"claude": { "effort": "high", "settings": "~/strict.json" }` on a Claude profile, `"codex": { "codex_profile": "deep" }` on a Codex profile. The keys come from the agent's own option registry and are checked against it: an unknown key fails and lists the valid ones, an option the installed version lacks is refused, and a block for another agent fails the load. `--option key=value` takes the same keys for one run. See [launch options](launch-options.md#recipe-7-typed-vendor-launch-flags) for examples.

Every block has `settings`: a JSONC file path (relative to the config dir) or an object, merged and delivered as described in [settings delivery](launch-options.md#how-settings-reach-the-agent). `settings` is agx's own key, never also the agent's raw option: `--option settings=…` is refused (use `--setting KEY=VALUE`), and a settings file of your own goes raw, `-- --settings FILE`, which agx merges over its own settings, your keys winning ([recipe 8](launch-options.md#a-raw---settings-of-your-own)). The other keys map one to one to the agent's own flag, shown in the last column; read the agent's documentation for what each flag does.

<!-- generated: agent-blocks (go test ./internal/docscheck/ -run TestAgentBlockTable -args -update) -->

#### `profiles.<profile>.claude`

| Key | Type | Passes |
|---|---|---|
| `profiles.<profile>.claude.add_dirs` | list of strings | `--add-dir` |
| `profiles.<profile>.claude.agent` | string | `--agent` |
| `profiles.<profile>.claude.agents` | object | `--agents` |
| `profiles.<profile>.claude.allow_dangerously_skip_permissions` | bool | `--allow-dangerously-skip-permissions` |
| `profiles.<profile>.claude.allowed_tools` | list of strings | `--allowedTools` |
| `profiles.<profile>.claude.append_system_prompt` | string | `--append-system-prompt` |
| `profiles.<profile>.claude.append_system_prompt_file` | string | `--append-system-prompt-file` |
| `profiles.<profile>.claude.autocompact` | string | `--autocompact` |
| `profiles.<profile>.claude.background` | bool | `--bg` |
| `profiles.<profile>.claude.bare` | bool | `--bare` |
| `profiles.<profile>.claude.betas` | list of strings | `--betas` |
| `profiles.<profile>.claude.brief` | bool | `--brief` |
| `profiles.<profile>.claude.chrome` | bool | `--chrome` |
| `profiles.<profile>.claude.continue` | bool | `--continue` |
| `profiles.<profile>.claude.dangerously_skip_permissions` | bool | `--dangerously-skip-permissions` |
| `profiles.<profile>.claude.debug` | string | `--debug` |
| `profiles.<profile>.claude.debug_file` | string | `--debug-file` |
| `profiles.<profile>.claude.disable_slash_commands` | bool | `--disable-slash-commands` |
| `profiles.<profile>.claude.disallowed_tools` | list of strings | `--disallowedTools` |
| `profiles.<profile>.claude.effort` | string: `low`, `medium`, `high`, `xhigh`, `max` | `--effort` |
| `profiles.<profile>.claude.exclude_dynamic_system_prompt_sections` | bool | `--exclude-dynamic-system-prompt-sections` |
| `profiles.<profile>.claude.extra_args` | list of strings | see [launch options](launch-options.md#block-keys-with-no-flag-of-their-own-prompt-and-extra_args) |
| `profiles.<profile>.claude.fallback_models` | list of strings | `--fallback-model` |
| `profiles.<profile>.claude.files` | list of strings | `--file` |
| `profiles.<profile>.claude.fork_session` | bool | `--fork-session` |
| `profiles.<profile>.claude.forward_subagent_text` | bool | `--forward-subagent-text` |
| `profiles.<profile>.claude.from_pr` | string | `--from-pr` |
| `profiles.<profile>.claude.ide` | bool | `--ide` |
| `profiles.<profile>.claude.include_hook_events` | bool | `--include-hook-events` |
| `profiles.<profile>.claude.include_partial_messages` | bool | `--include-partial-messages` |
| `profiles.<profile>.claude.input_format` | string: `text`, `stream-json` | `--input-format` |
| `profiles.<profile>.claude.json_schema` | any JSON | `--json-schema` |
| `profiles.<profile>.claude.max_budget_usd` | number | `--max-budget-usd` |
| `profiles.<profile>.claude.mcp_configs` | list of strings | `--mcp-config` |
| `profiles.<profile>.claude.model` | string | `--model` |
| `profiles.<profile>.claude.name` | string | `--name` |
| `profiles.<profile>.claude.no_chrome` | bool | `--no-chrome` |
| `profiles.<profile>.claude.no_session_persistence` | bool | `--no-session-persistence` |
| `profiles.<profile>.claude.output_format` | string: `text`, `json`, `stream-json` | `--output-format` |
| `profiles.<profile>.claude.permission_mode` | string: `acceptEdits`, `auto`, `bypassPermissions`, `manual`, `dontAsk`, `plan` | `--permission-mode` |
| `profiles.<profile>.claude.permission_prompts` | string | `--permission-prompts` |
| `profiles.<profile>.claude.plugin_dirs` | list of strings | `--plugin-dir` |
| `profiles.<profile>.claude.plugin_urls` | list of strings | `--plugin-url` |
| `profiles.<profile>.claude.print` | bool | `-p` |
| `profiles.<profile>.claude.prompt` | string | see [launch options](launch-options.md#block-keys-with-no-flag-of-their-own-prompt-and-extra_args) |
| `profiles.<profile>.claude.remote_control` | string | `--remote-control` |
| `profiles.<profile>.claude.replay_user_messages` | bool | `--replay-user-messages` |
| `profiles.<profile>.claude.restricted` | bool | `--restricted` |
| `profiles.<profile>.claude.resume` | string | `--resume` |
| `profiles.<profile>.claude.safe_mode` | bool | `--safe-mode` |
| `profiles.<profile>.claude.session_id` | string | `--session-id` |
| `profiles.<profile>.claude.setting_sources` | list of strings | `--setting-sources` |
| `profiles.<profile>.claude.strict_mcp_config` | bool | `--strict-mcp-config` |
| `profiles.<profile>.claude.system_prompt` | string | `--system-prompt` |
| `profiles.<profile>.claude.system_prompt_file` | string | `--system-prompt-file` |
| `profiles.<profile>.claude.system_prompt_snapshot` | string | `--system-prompt-snapshot` |
| `profiles.<profile>.claude.tmux` | string | `--tmux` |
| `profiles.<profile>.claude.tools` | list of strings | `--tools` |
| `profiles.<profile>.claude.verbose` | bool | `--verbose` |
| `profiles.<profile>.claude.worktree` | string | `--worktree` |
| `profiles.<profile>.claude.settings` | string or object | agent settings: a JSONC file path (relative to the config dir) or an object |

#### `profiles.<profile>.codex`

| Key | Type | Passes |
|---|---|---|
| `profiles.<profile>.codex.add_dirs` | list of strings | `--add-dir` |
| `profiles.<profile>.codex.approval` | string: `on-request`, `never`, `untrusted`, `on-failure` | `-a` |
| `profiles.<profile>.codex.approve_for_me` | bool | `--approve-for-me` |
| `profiles.<profile>.codex.bypass_all` | bool | `--dangerously-bypass-approvals-and-sandbox` |
| `profiles.<profile>.codex.cd` | string | `-C` |
| `profiles.<profile>.codex.codex_profile` | string | `-p` |
| `profiles.<profile>.codex.disable` | list of strings | `--disable` |
| `profiles.<profile>.codex.enable` | list of strings | `--enable` |
| `profiles.<profile>.codex.images` | list of strings | `-i` |
| `profiles.<profile>.codex.local_provider` | string | `--local-provider` |
| `profiles.<profile>.codex.model` | string | `-m` |
| `profiles.<profile>.codex.no_alt_screen` | bool | `--no-alt-screen` |
| `profiles.<profile>.codex.no_daemon` | bool | `--no-daemon` |
| `profiles.<profile>.codex.oss` | bool | `--oss` |
| `profiles.<profile>.codex.prompt` | string | see [launch options](launch-options.md#block-keys-with-no-flag-of-their-own-prompt-and-extra_args) |
| `profiles.<profile>.codex.remote` | string | `--remote` |
| `profiles.<profile>.codex.remote_auth_token_env` | string | `--remote-auth-token-env` |
| `profiles.<profile>.codex.sandbox` | string: `read-only`, `workspace-write`, `danger-full-access` | `-s` |
| `profiles.<profile>.codex.search` | bool | `--search` |
| `profiles.<profile>.codex.strict_config` | bool | `--strict-config` |
| `profiles.<profile>.codex.worktree` | bool | `--worktree` |
| `profiles.<profile>.codex.settings` | string or object | agent settings: a JSONC file path (relative to the config dir) or an object |

<!-- end generated: agent-blocks -->

## Logins

Each entry is one named login; [adding a login](logins.md#adding-a-login) shows every way to write one, and [which login kind](choosing/login-kind.md) helps you choose.

| Key | Type | Meaning |
|---|---|---|
| `logins.<login>.agents` | list of agent ids | required; the agents this login serves |
| `logins.<login>.kind` | `oauth-token`, `api-key`, `api-key-helper`, `pool` | required; see [kinds](logins.md#kinds) |
| `logins.<login>.token` | string | a reference (`${gopass:…}`, `$(…)`, `${VAR}`) or a plain value |
| `logins.<login>.stored` | bool | the value lives in `tokens.jsonc` under the login name |
| `logins.<login>.command` | string | for `api-key-helper`: the command the agent runs |
| `logins.<login>.account` | string | a display label, such as an email; never verified |
| `logins.<login>.provider` | string | the model provider, for agents with one variable per provider (pi: `anthropic`, `openai`) |
| `logins.<login>.allow_api_key` | bool | let an `oauth-token` login whose token is an API key (billed per use) launch, be handed out by `agx credential` and be picked by a pool; default `false` refuses it. See [Is a login billed per use?](logins.md#is-a-login-billed-per-use) |
| `logins.<login>.pool` | object | for `pool`: the member logins and the selection rules' numbers (below) |

Exactly one of `token`, `stored` and `command` is set, except on a `pool`, which sets none of them and has `pool` instead.

### Login pools

A `pool` login serves exactly one agent that can re-ask for its key while it runs and whose logins can be probed (today Claude Code), and holds no value: each time the agent asks for a key, agx picks one member by the rules in [Share a pool of Claude accounts](guides/login-pool.md#the-rules) and hands out its token. Every member must be another token login of that agent (not a pool, not an `api-key-helper`), named once across `own` and `shared`.

<!-- snippet: examples/acct-login-pool/config.jsonc#pool -->
```jsonc
    "auto": {
      "agents": ["claude"],
      "kind": "pool",
      "pool": {
        "own": ["alice", "bob"],              // yours: used up almost fully
        "shared": ["carol", "derek", "erin"], // teammates': borrowed up to their caps
        "force": null,                        // a member name overrides every rule
        "shared_borrow_window": "48h",
        "short_unavailable_at_percent": 99,
        "defaults": { "short": 50, "week": 70 },
        "accounts": {
          "alice": { "short": 95, "week": 98 },
          "bob": { "short": 95, "week": 98 }
        },
        "usage_max_age": "60s",
        "key_ttl": "60s"
      }
    },
```

This pool sets its members, caps and cache keys; the rule values, schedules and overrides below are written with their flags in [acct-pool-rules](../examples/acct-pool-rules/README.md) and [acct-pool-schedules](../examples/acct-pool-schedules/README.md).

| Key | Type | Meaning |
|---|---|---|
| `logins.<login>.pool.own` | list of login names | your accounts, in order; used up to their caps (95 and 98 unless `own_caps` or a member entry says otherwise) |
| `logins.<login>.pool.shared` | list of login names | teammates' accounts, in order; borrowed up to their caps |
| `logins.<login>.pool.force` | login name or `null` | always pick this login, ignoring every rule; `null` (the default) is off |
| `logins.<login>.pool.shared_borrow_window` | duration | a shared login goes before a usable own login only when its weekly reset is within this; default `48h`, `0s` never |
| `logins.<login>.pool.short_unavailable_at_percent` | number | the 5-hour safety cutoff for every member, even one whose status is ok; default 99 |
| `logins.<login>.pool.defaults` | object | caps of shared members without a `shared_caps` or member entry, and the pool's `week_relax_max` |
| `logins.<login>.pool.defaults.short` | number | 5-hour cap of shared members in percent, exclusive; default 50 |
| `logins.<login>.pool.defaults.week` | number | weekly cap of shared members in percent, exclusive; default 70 |
| `logins.<login>.pool.defaults.week_relax_max` | number | the weekly cap members reach at their weekly reset; default 95. A member whose weekly cap is already higher keeps it |
| `logins.<login>.pool.own_caps` | object | caps of your own members without their own entry (own members never take `defaults`) |
| `logins.<login>.pool.own_caps.short` | number | 5-hour cap of own members; default 95 |
| `logins.<login>.pool.own_caps.week` | number | weekly cap of own members; default 98 |
| `logins.<login>.pool.own_caps.week_relax_max` | number | weekly cap own members reach at their reset; default `defaults.week_relax_max` |
| `logins.<login>.pool.shared_caps` | object | caps of shared members without their own entry; over `defaults` |
| `logins.<login>.pool.shared_caps.short` | number | 5-hour cap of shared members |
| `logins.<login>.pool.shared_caps.week` | number | weekly cap of shared members |
| `logins.<login>.pool.shared_caps.week_relax_max` | number | weekly cap shared members reach at their reset; default `defaults.week_relax_max` |
| `logins.<login>.pool.accounts` | map of member to object | per-member entries; a name in neither list is refused |
| `logins.<login>.pool.accounts.<key>.short` | number | this member's 5-hour cap |
| `logins.<login>.pool.accounts.<key>.week` | number | this member's weekly cap |
| `logins.<login>.pool.accounts.<key>.enabled` | bool | `false` leaves the member out (it is not probed) |
| `logins.<login>.pool.accounts.<key>.week_relax_max` | number | this member's weekly cap at its reset; its weekly cap or lower = never relaxed |
| `logins.<login>.pool.accounts.<key>.capacity` | number | this member's plan size relative to the others' (only ratios matter, such as Pro 1, Max 5x 5, Max 20x 20; omitted = 1): its room is weighed by it, so a bigger plan's percent counts for more work. Without it every member counts the same |
| `logins.<login>.pool.accounts.<key>.override` | object | this member's caps until a time, then ignored (`agx login edit --pool-cap-short NAME=N --until TIME`; `agx pool prune` removes ended ones). It beats every schedule for the caps it sets. See [schedules and overrides](guides/pool-rules.md#schedules-and-overrides) |
| `logins.<login>.pool.accounts.<key>.override.short` | number | the 5-hour cap until `until` |
| `logins.<login>.pool.accounts.<key>.override.week` | number | the weekly cap until `until` |
| `logins.<login>.pool.accounts.<key>.override.week_relax_max` | number | the weekly cap at the reset until `until` |
| `logins.<login>.pool.accounts.<key>.override.until` | RFC 3339 time | when it ends, with its zone (`2026-11-22T23:59:00+03:00`); required |
| `logins.<login>.pool.order` | `urgency` or `deadline` | how the pool ranks its usable members: `urgency` (the default) puts first the members whose reset is within `deadline_window` with at least `deadline_min_room` of room, soonest reset first, then the most room to lose per hour (the room left before the reset ÷ hours to it, times `capacity`; inside `week_relax_window` the room counts up to the cap reached at the reset); `deadline` the soonest weekly reset. See [how the pool chooses](guides/pool-rules.md) |
| `logins.<login>.pool.deadline_window` | duration | under the `urgency` order, members whose weekly reset is within this, with at least `deadline_min_room` of room, go before the rest, soonest reset first: the room that expires first is used first; default `24h`, `0s` never (urgency alone) |
| `logins.<login>.pool.deadline_min_room` | number | the least room, in percent up to the weekly cap reached at the reset, a member inside `deadline_window` needs to go first, so a scrap of room does not take the top; default 2 |
| `logins.<login>.pool.week_relax_window` | duration | over this long before a member's weekly reset its weekly cap rises in a straight line to its `week_relax_max`; default `24h`, `0s` never. The 5-hour cap never rises |
| `logins.<login>.pool.max_sessions` | whole number | how many sessions one member takes before the next member is used; default 4, `0` no limit |
| `logins.<login>.pool.sticky` | bool | a session stays on its member while that member is still in the pool (keeps its prompt cache); default `true` |
| `logins.<login>.pool.sticky_leave_ratio` | number | a sticky session leaves its member when the member it would go to (the first in order that can take another session) is at least this many times as urgent as its own; default 3, `0` never. Leaving keeps a small pool that runs short of quota following the account that resets first |
| `logins.<login>.pool.probe_models` | list of model names | the models to probe the members with when a session's own model is unknown (no `--model`, profile `model` or Claude setting): a per-model weekly window (Fable 5.2's own week) shows only when the probe asks with that model. Omitted = the default profile's model, else the agent's default probe model. See [per-model weekly windows](guides/pool-rules.md#per-model-weekly-windows) |
| `logins.<login>.pool.usage_max_age` | duration | reuse the login-usage cache while it is younger than this; default `60s` |
| `logins.<login>.pool.key_ttl` | duration | how long the agent keeps a key before asking again (Claude Code's `CLAUDE_CODE_API_KEY_HELPER_TTL_MS`); default `60s`, at least `1s` |
| `logins.<login>.pool.picker` | object | your own picker instead of the built-in rules; unset (the default) uses the rules. See [your own picker](guides/login-pool.md#your-own-picker) |
| `logins.<login>.pool.picker.command` | string | a shell command line; it reads the members and their usage as JSON on stdin (never a token) and prints one member's login name. A failure falls back to the built-in rules with a warning |
| `logins.<login>.pool.picker.timeout` | duration | how long one run may take; default `10s` |
| `logins.<login>.pool.log` | path | a file that gets one line per key request (time, pick, group, forced, source, the picked member's usage in percent and the probe models; never a token); `~` and `${VAR}` expand |
| `logins.<login>.pool.timezone` | IANA time zone | the zone of schedules without their own and of `--until` times, like `Europe/Istanbul`; omitted = this machine's local time |
| `logins.<login>.pool.schedules` | list of objects | recurring windows of different caps: on their days and hours their caps replace the named members' caps. `agx login edit --pool-schedule 'NAME key=value …'` changes the keys it gives of one schedule and keeps the rest (`key=` removes one); `--pool-schedule-replace` writes a whole one. See [schedules and overrides](guides/pool-rules.md#schedules-and-overrides) |
| `logins.<login>.pool.schedules[].name` | string | its name, shown by `agx pool pick` and `agx pool show`; unique |
| `logins.<login>.pool.schedules[].days` | list of `mon` … `sun` | the days a window starts on |
| `logins.<login>.pool.schedules[].hours` | `HH:MM-HH:MM` | the window each day; omitted = `00:00-24:00`. An end at or before the start ends the next day (`22:00-06:00`); back-to-back windows join into one |
| `logins.<login>.pool.schedules[].members` | list | member names and group names (`own`, `shared`). A schedule naming a member beats one naming its group; at the same level the highest value wins |
| `logins.<login>.pool.schedules[].timezone` | IANA time zone | its own zone; omitted = the pool's `timezone` |
| `logins.<login>.pool.schedules[].caps` | object | the caps while it applies; at least one. A cap it leaves out stays as it is |
| `logins.<login>.pool.schedules[].caps.short` | number | the 5-hour cap |
| `logins.<login>.pool.schedules[].caps.week` | number | the weekly cap |
| `logins.<login>.pool.schedules[].caps.week_relax_max` | number | the weekly cap at the reset |
| `logins.<login>.pool.schedules[].caps.week_before_reset` | number | the weekly cap, at once, of a member whose weekly reset (or a per-model window's own reset) comes at or before the schedule's end, `release_before` counted: its room is lost at that reset before its teammate is back. Members that reset later keep their weekly cap; a higher cap (the relax, your own 98) is kept; an override that sets `week` wins ([why](guides/pool-rules.md#use-teammates-accounts-more-on-their-days-off)) |
| `logins.<login>.pool.schedules[].release_before` | duration | stop applying this long before the window ends; omitted = `5h` when it sets `short` (a teammate starting right after finds a fresh 5-hour window), else `0s` |

Durations are Go duration strings (`48h`, `90m`, `60s`). A member's caps come from its override while it applies, else the schedules that apply now (one naming the member before one naming its group), else its own entry, else its group's (`own_caps`, `shared_caps`), else for a shared member `defaults`, else agx's default (own 95/98, shared 50/70), each cap on its own ([which cap wins](guides/pool-rules.md#which-cap-wins)); `agx pool show` prints each value with its source (`member`, `group`, `pool`, `default`). `agx pool preset --name recommended|classic --id POOL` writes a named set of the rule values ([presets](guides/pool-rules.md#presets)). Caps and the cutoff are 0 to 100, and the published schema says so (`minimum`, `maximum`), so your editor flags a wrong one as you type. Every key here has a flag on `agx login add` and `agx login edit` (`--pool-cap-short NAME=N`, `--pool-cap-week NAME=N`, `--pool-disable`, `--pool-enable`, `--pool-default-short`, `--pool-default-week`, `--pool-short-cutoff`, `--pool-borrow-window`, `--pool-usage-max-age`, `--pool-key-ttl`, `--pool-picker`, `--pool-order`, `--pool-deadline-window`, `--pool-deadline-min-room`, `--pool-week-relax-window`, `--pool-max-sessions`, `--pool-sticky`, `--pool-sticky-leave-ratio`, `--pool-default-week-relax`, `--pool-cap-week-relax NAME=N`, `--pool-capacity NAME=N`, `--pool-group-cap-short GROUP=N`, `--pool-group-cap-week GROUP=N`, `--pool-group-cap-week-relax GROUP=N`, `--pool-probe-model MODEL`, `--pool-timezone`, `--pool-schedule`, `--pool-schedule-replace`, `--pool-schedule-remove`, `--until` (an override that ends) and `--pool-clear-override`; see the [login reference](commands/login.md#agx-login-edit)), and `agx config set` sets any single value (a list such as `probe_models` goes through the flag). Nothing in a pool is ignored silently: an unknown key anywhere, a cap that is not a number from 0 to 100, `null` for anything but `force`, an empty `force`, an `enabled` that is not `true`/`false` and a bad duration are each an error at their line. The old setting names of the stand-alone selector (`burnToFullest`, `priority`, `accountLimits`, `limits`, `sharedBorrowWindowHours`) are unknown keys and refused at their line.

## Projects

`projects` binds folders, per agent; [Bind a folder](guides/folder-bindings.md) walks through it. The key is the folder (stored absolute and normalized; `~` is fine when you write it by hand), and inside it each agent id maps to a binding.

```jsonc
"projects": {
  "~/projects/client": {
    "claude": { "profile": "work" },
    "codex": { "profile": "codex" },
    "gemini": { "login": "gem-work" }
  }
}
```

| Key | Type | Meaning |
|---|---|---|
| `projects.<folder>.<agent>.profile` | profile name | the profile for this agent in the folder; it must be a profile of that agent |
| `projects.<folder>.<agent>.login` | login name | the login for this agent in the folder |
| `projects.<folder>.<agent>.home_login` | bool | use the home's own login in the folder |

## Usage

| Key | Type | Meaning |
|---|---|---|
| `usage.time_format` | `relative`, `day`, `absolute` or `both` | how `agx usage` writes reset times: `relative` (the default, "2 hr 50 min", as agx v0.3 and claude.ai write them), `day` ("Tue 05:30"), `absolute` ("Tue 24 Nov 05:30") or `both` ("3 hr 10 min (22:20)", with the weekday when the reset is not today and the date when it is more than six days away), in the `--tz` zone; `--time-format` overrides it for one run |
| `usage.duration_format` | `truncate`, `round` or `minutes` | how a relative time a day or more away reads, in `agx usage` and `agx pool pick`: `truncate` (the default, "1 d 2 hr"), `round` (nearest hour, "1 d 3 hr") or `minutes` ("1 d 2 hr 52 min"); `--duration-format` overrides it for one run |
| `usage.date_format` | string | how `agx usage` writes the clock and date parts of `day`, `absolute`, `both` and `RESETS ON`: a preset or tokens ([below](#date-formats)); unset uses the top-level `date_format`, else `long`; `--date-format` overrides it for one run |

```jsonc
"date_format": "iso",
"usage": { "time_format": "both", "date_format": "us" }
```

### Date formats

A date format is a preset or tokens. Letters are tokens, so words go in `[brackets]`; any other character prints as itself. An unknown token, an unclosed bracket or a format without a token fails the config load at its line (exit 2), as `--date-format` does on the command line.

| Preset | Tokens | Reads |
|---|---|---|
| `iso` | `YYYY-MM-DD HH:mm` | 2026-11-19 23:30 |
| `short` | `ddd HH:mm` | Thu 23:30 |
| `long` | `ddd D MMM HH:mm` | Thu 19 Nov 23:30 |
| `us` | `MMM D h:mm A` | Nov 19 11:30 PM |

| Token | Reads | Token | Reads |
|---|---|---|---|
| `YYYY` | 2026 | `HH` | 23, 09 (24-hour, padded) |
| `MM` | 11 (month, padded) | `H` | 23, 9 (24-hour) |
| `MMM` | Nov | `hh` | 11, 09 (12-hour, padded) |
| `DD` | 09 (day, padded) | `h` | 11, 9 (12-hour) |
| `D` | 9 | `mm` | 30 (minutes) |
| `ddd` | Thu | `ss` | 07 (seconds) |
| `dddd` | Thursday | `a` / `A` | pm / PM |

The clock part of a format runs from its first time token to its last (`h:mm A` in `us`); `day` and `both` use it, and a format without a time token uses `HH:mm` there. Relative times ("2 hr 50 min", "3 min ago") and `--json` values never change.

## Aliases

`aliases` maps a name to agx arguments, defined as shell functions by the [shell layer](shell.md#aliases); `agx alias add` checks each against the command tree, and every load checks the name and words, so a hand-written alias the layer could not define fails with its line and column:

- **Name:** letters, digits, `_` and `-`, not starting with a digit or `-`; not `agx`, `command`, `builtin`, `cd`, `set` or `function`.
- **Words:** letters, digits, spaces and `_./:=@+-`; no word starts with `=` (zsh expands `=word` to a path) or `@` (PowerShell splats `@word`).

```jsonc
"aliases": { "cl": "run --profile personal", "clw": "run --profile work", "cla": "run --auto-profile" }
```

## References and plain values

Anywhere agx takes a secret (`logins.<login>.token`, `profiles.<profile>.env_refs`, `--env`, `--token-ref`, env files) you may write:

| Form | Example | Resolved by |
|---|---|---|
| gopass | `${gopass:personal/claude/carol}` | `gopass show -o personal/claude/carol` |
| a command | `$(op read op://work/claude/token)` | `sh -c` (PowerShell on Windows), 30 second limit, stdin closed |
| a variable | `${CLAUDE_TOKEN}` | your environment; unset is an error |
| a plain value | `plain-token-value` | used as written |

Resolution is strict: an unset variable, an unknown scheme or a failing command is an error that names the reference and never shows a value. A plain value is allowed anywhere except `env_refs`, which holds only references (plain values go in `env`). A login token, an `env_refs` value and anything on an agent's credential variable are redacted in all output, plain or not; a literal profile `env` value on any other variable is shown as written in a launch's output, whatever its name ([what is redacted](output.md#what-is-redacted)).

## Related

- [Check your setup](guides/doctor.md): `agx config check`, `config show`, `config set`, `config path` and `agx doctor`.
- [A custom agent](guides/custom-agent.md): `agents.<agent>.spec` step by step.
- [Agents and what they support](guides/agents.md): which agent block and login kinds each agent takes.
- Examples: [`examples/cfg-agent-defaults`](../examples/cfg-agent-defaults/README.md), [`examples/cfg-custom-agent`](../examples/cfg-custom-agent/README.md), [`examples/cfg-config-set`](../examples/cfg-config-set/README.md), [`examples/cfg-doctor`](../examples/cfg-doctor/README.md).
