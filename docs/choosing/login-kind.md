# Which login kind

A login is what one launch authenticates with. agx offers six choices: the home's own login, a plan token (`oauth-token`) kept as a reference, a plan token pasted into `tokens.jsonc`, a pay-per-token key (`api-key`), a key helper the agent runs itself (`api-key-helper`), and live login, a switchable mode on top of a stored token login. This page helps you pick one. How to set each up is in [Logins](../logins.md).

## Short answer

| Your situation | Choose | Agents |
|---|---|---|
| you logged in inside the agent (`claude /login`, `codex login`, `gemini`, `pi`) and each home is one account | the home login: no login in config at all | all |
| you have several Claude plan accounts and want any home on any of them | an `oauth-token` login per account (the token comes from `claude setup-token`) | Claude |
| that token already lives in gopass, a password manager or an environment variable | `oauth-token` with `token` set to a reference: `${gopass:path}`, `$(command)` or `${VAR}` | Claude |
| you have no secret store | `oauth-token` with `--stdin` or `--login-setup`: the value is stored in `tokens.jsonc` (mode 0600) | Claude |
| you pay per token (an Anthropic Console key, a Gemini key, a pi provider key) | `api-key` | Claude, Gemini CLI, pi |
| the key must never pass through agx, or a command hands out short-lived keys | `api-key-helper`: Claude runs the command itself | Claude |
| a long session may hit its plan limit and you want to switch it without a restart | live login: `--live-login` on a stored `oauth-token` or `api-key` login | Claude |
| one run on a token you do not want to save | `--token-ref REF --token-kind KIND` | Claude, Gemini CLI, pi |
| Codex | the home login only: Codex has no token login | Codex |

## What each agent takes

| Agent | home login | `oauth-token` | `api-key` | `api-key-helper` | live login |
|---|---|---|---|---|---|
| Claude Code | yes | yes (`CLAUDE_CODE_OAUTH_TOKEN`) | yes (`ANTHROPIC_API_KEY`) | yes (`apiKeyHelper` in the launch settings) | yes |
| Codex | yes | no | no | no | no |
| Gemini CLI | yes | no | yes (`GEMINI_API_KEY`) | no | no |
| pi | yes | no | yes (the provider's variable, e.g. `ANTHROPIC_API_KEY` for `"provider": "anthropic"`) | no | no |
| a config-defined agent | yes | no | no | no | no |

A kind an agent does not take is refused when you add the login (exit 4), with the kind it does take:

<!-- snippet: examples/choose-login-kind/session.txt#agents -->
```sh
$ agx login add --name gem-plan --agent gemini --kind oauth-token --token-ref '${gopass:google/plan}'
agx: gemini has no oauth-token login; it takes api-key
[exit 4]

$ agx login add --name pi-helper --agent pi --kind api-key-helper --helper-command 'gopass show -o x'
agx: pi has no api-key-helper login; it takes api-key
[exit 4]

$ agx login add --name codex-key --agent codex --kind api-key --token-ref '${gopass:openai/key}'
agx: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
[exit 4]
```

## Where the token lives

A token login holds exactly one of `token`, `stored` or `command`. Pick the source by where the secret already is:

| Source | Config | Resolved | Shown as (`agx login list`) |
|---|---|---|---|
| a plugin reference | `"token": "${gopass:personal/claude/carol}"` | at launch, by the plugin | `reference gopass` |
| a command | `"token": "$(op read op://vault/claude/token)"` | at launch: agx runs it (30 second limit) | `command op` |
| an environment variable | `"token": "${CLAUDE_PLAN_TOKEN}"` | at launch, from agx's environment | `env CLAUDE_PLAN_TOKEN` |
| a plain value | `"token": "the-token"` | as written; allowed, redacted in every output | `plain` |
| pasted | `"stored": true` (written by `--stdin`, `--login-setup` or the hidden prompt) | read from `tokens.jsonc`, which must be mode 0600 | `stored (tokens.jsonc)` |
| a key helper | `"command": "gopass show -o work/anthropic-api-key"` (kind `api-key-helper` only) | never by agx: the agent runs it | `helper gopass` |

`list` and `check --no-probe` name the source, never the value:

<!-- snippet: examples/choose-login-kind/session.txt#list -->
```sh
$ agx login list
NAME           AGENTS     KIND            SOURCE                 ACCOUNT
carol          claude     oauth-token     reference gopass       —
pasted         claude     oauth-token     stored (tokens.jsonc)  —
console        claude     api-key-helper  helper gopass          —
anthropic-key  claude,pi  api-key         reference gopass       —
gem-key        gemini     api-key         reference gopass       —

$ agx login check --no-probe
NAME           KIND            SOURCE                 RESOLVE  PROBE    DETAIL
carol          oauth-token     reference gopass       ok       skipped  --no-probe
pasted         oauth-token     stored (tokens.jsonc)  ok       skipped  --no-probe
console        api-key-helper  helper gopass          skipped  skipped  the agent runs the helper itself; agx never does
anthropic-key  api-key         reference gopass       ok       skipped  --no-probe
gem-key        api-key         reference gopass       ok       skipped  --no-probe
```

A `--dry-run` resolves nothing; add `--resolved-secrets` to resolve each reference and print `resolved` or the error, never the value:

<!-- snippet: examples/choose-login-kind/session.txt#resolved -->
```sh
$ agx run --profile work --login carol --resolved-secrets --dry-run --quiet --json --field decisions.3
{"name":"login","value":"carol (resolved)","source":"--login"}
```

## How each kind reaches the agent

A plan token goes into the agent's environment, never its command line:

<!-- snippet: examples/choose-login-kind/session.txt#plan-token -->
```sh
$ agx run --profile work --login carol --dry-run --quiet
agent    claude                                  from profile work
profile  work                                    from --profile
home     ~/.claude-work                          from profile work
login    carol (not resolved: --dry-run)         from --login
binary   /usr/local/bin/claude version unknown   from no install metadata
folder   ~/projects/app                          from current folder
run      ~/.agx/runs/20261114-101500-k3x9        from AGX_RUN_ID=20261114-101500-k3x9
command  claude                                  from built by agentio
env      set AGX_RUN_ID=20261114-101500-k3x9     from launch environment
         set AGX_RUN_AGENT=claude                from launch environment
         set AGX_RUN_PROFILE=work                from launch environment
         set AGX_RUN_LOGIN=carol                 from launch environment
         set AGX_RUN_HOME=~/.claude-work         from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]  from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work    from launch environment
note     installed Claude Code version unknown; flags not checked
```

An `api-key` login sets the agent's own key variable. One login can serve several agents (`"agents": ["claude", "pi"]`); for pi, `provider` picks the variable:

<!-- snippet: examples/choose-login-kind/session.txt#api-key -->
```sh
$ agx run --profile pi --login anthropic-key --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261114-101500-k3x9","set AGX_RUN_AGENT=pi","set AGX_RUN_PROFILE=pi","set AGX_RUN_LOGIN=anthropic-key","set AGX_RUN_HOME=~/.pi/agent","set ANTHROPIC_API_KEY=[redacted]"]

$ agx run --profile gem --login gem-key --dry-run --quiet --json --field data.command.env
["set AGX_RUN_ID=20261114-101500-k3x9","set AGX_RUN_AGENT=gemini","set AGX_RUN_PROFILE=gem","set AGX_RUN_LOGIN=gem-key","set AGX_RUN_HOME=~/.gemini","set GEMINI_API_KEY=[redacted]"]
```

An `api-key-helper` login sets no variable: agx writes `apiKeyHelper` into the run's settings file and Claude runs the command when it needs a key, so the key never passes through agx:

<!-- snippet: examples/choose-login-kind/session.txt#helper -->
```sh
$ agx run --profile work --login console --dry-run --quiet --json --field data.command.line
claude --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json
```

## If this, then that

- If every account has its own home and you never mix them, stay on home logins. Add named logins later; nothing else changes.
- If you want one home (its settings, history and MCP servers) on several plan accounts, add an `oauth-token` login per account and pick it per run, per folder or per terminal (see [Where to set the profile and login](where-to-set.md)).
- If a session must outlive one account's quota, use a stored token login and `--live-login` (see [Continue on another account](continue-on-another-account.md)).
- If your organisation hands out keys through a command, use `api-key-helper`. It cannot be live: agx never runs a credential command it only passes on.
- If you only need a token once, `--token-ref` with `--token-kind` saves nothing; it also cannot be live.

## Pitfalls

- A login must serve the run's agent. A Claude login on a Codex run is refused, never skipped, from any source (`--login`, `AGX_LOGIN`, a binding):

<!-- snippet: examples/choose-login-kind/session.txt#wrong-agent -->
```sh
$ agx run --profile codex --login carol --dry-run --quiet
agx: login carol serves claude; this run is codex
[exit 125]
```

- Live login needs a stored token login. The home login, a one-off `--token-ref` and an `api-key-helper` login are refused:

<!-- snippet: examples/choose-login-kind/session.txt#live -->
```sh
$ agx run --profile work --live-login --dry-run --quiet
agx: --live-login needs a stored login; this run starts on the home login (default (no login named))
  agx run --login NAME --live-login
[exit 125]

$ agx run --profile work --login console --live-login --dry-run --quiet
agx: login console is an api-key-helper; --live-login needs a token login
[exit 125]

$ agx run --profile work --token-ref '${CLAUDE_PLAN_TOKEN}' --token-kind oauth-token --live-login --dry-run --quiet
agx: --live-login needs a stored login; a one-off --token-ref cannot be fetched again by agx credential
  agx login add --name NAME --agent claude --kind oauth-token --token-ref REF
[exit 125]
```

- `tokens.jsonc` readable by group or others is refused: `~/.agx/tokens.jsonc has mode 0664; it holds tokens and must be 0600`. agx creates it 0600; a copy from elsewhere may not be.
- When agx sets a token it also removes the variables that would outrank it for that agent (for Claude, `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY` outrank `CLAUDE_CODE_OAUTH_TOKEN`), so an exported key in your shell cannot silently win.
- A config-defined agent takes the kinds its spec's `token_env` names, and only those (`mycli has no oauth-token login; it takes api-key`, exit 4); without `token_env` it has no token login. It has no probe and no setup command, so add its key with `--token-ref` or `--stdin`. See [A custom agent](../guides/custom-agent.md#step-6-a-token-login).
- agx never refreshes, writes or prints an agent's own stored OAuth credential. Rotate a token login with `agx login edit`.

## Related

- Guide: [Logins](../logins.md) (kinds, adding, using, live login), [Passing anything to an agent](../launch-options.md#recipe-4-a-claude-plan-token-from-gopass), [Live login](../guides/live-login.md), [A second account](../guides/second-account.md).
- Commands: [`agx login`](../commands/login.md), [`agx live`](../commands/live.md).
- Example: [choose-login-kind](../../examples/choose-login-kind/README.md) (every kind side by side), [quickstart](../../examples/quickstart/README.md) (a token from stdin), [acct-login-kinds](../../examples/acct-login-kinds/README.md), [acct-use-login](../../examples/acct-use-login/README.md), [acct-live-login](../../examples/acct-live-login/README.md).
- Other decisions: [Where to set the profile and login](where-to-set.md), [Continue on another account](continue-on-another-account.md), [A second account](second-account.md).
