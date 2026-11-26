# Troubleshooting

Each entry is a message you may see, why agx stopped, and what to do. agx stops instead of guessing, and its messages name the command that resolves the problem. Add `--dry-run` to any launch to see every decision and its source, `--json` to get the stable `error.code`, and run [`agx doctor`](guides/doctor.md) to check the whole setup at once.

Exit codes: a launching command (`run`, `new`, `resume`) exits 125 when agx stops before the agent starts, 127 when the binary is missing; any other command exits 1 (failure), 2 (usage error, conflict) or 4 (the agent cannot). See [exit codes](output.md#exit-codes).

## Setup

### "no profiles configured"

agx found no config file, or one without profiles. Launches stop (exit 125); read commands show nothing.

<!-- runnable: fixture=empty exit=125 -->
```sh
$ agx run
agx: no profiles configured
  agx config init
  agx profile import --discovered --yes
```

Run `agx config init --discovered`, or `agx profile import --discovered --yes` to make a profile of each home already on your machine. If you keep the config elsewhere, set `AGX_CONFIG_DIR` or pass `--config`; `agx config path` shows which file agx reads ([see the config agx uses](guides/doctor.md#step-6-see-the-config-agx-uses)).

### "no profile, binding or agent given, and default_agent is not set"

Nothing named an agent: no `--profile` or `--agent`, no folder binding, no `default_agent`. At a terminal agx asks; in a script it stops. Set a default once:

```sh
agx config set --key default_agent --value claude
```

See [when nothing names an agent](how-agx-decides.md#when-nothing-names-an-agent).

### "no profile chosen for gemini and none is its default"

The agent is known (from `--agent`, a binding or `default_agent`) but none of its profiles is marked `"default": true`, so agx will not pick one (exit 125). The message names the commands for each of its profiles, such as `agx run --profile gem` and `agx profile use --id gem`. For an agent with no profile at all, it names `agx profile add --name NAME --agent AGENT --home DIR`. See [step 2: the profile](how-agx-decides.md#step-2-the-profile).

### "--agent codex and --profile work (a claude profile) disagree; pick one"

`--profile` already names the agent, so `--agent` with a profile of another agent is a conflict (exit 125). Drop one of the two.

### "unknown profile"

<!-- runnable: exit=125 -->
```sh
$ agx run --profile wrok --quiet
agx: unknown profile wrok
  agx profile list
```

### "unknown agent cursor; known: claude, codex, gemini, pi, opencode"

agx has no agent with that id (exit 1). A custom agent joins the list once it is in config under `agents.<id>.spec`; see [A custom agent](guides/custom-agent.md).

### "home … does not exist"

A profile's `home` must exist. Create it with `agx profile add`, which also links the shared files from the agent's default home, or fix the path in config.

### "claude not found on PATH; install: …" (exit 127)

```text
agx: claude not found on PATH; install: npm i -g @anthropic-ai/claude-code
```

The agent is not installed, or not on the `PATH` agx sees. Run the install command the message names, then check with `agx agent list`. Exit 126 means the file was found but cannot run (permissions, wrong architecture). A custom agent's missing binary stops even a `--dry-run` the same way:

<!-- snippet: examples/cfg-custom-agent/session.txt#missing -->
```sh
$ agx run --agent nocli --dry-run
agx: nocli not found on PATH
[exit 127]
```

See [Agents and what they support](guides/agents.md#pitfalls).

### "claude has two default profiles (personal, work); keep one"

Each agent may have one profile with `"default": true`. The message names the file position and both profiles; keep one, or run `agx profile use --id NAME`.

### A config error with a line and column

agx checks the whole file on every load; `agx config check` lists every problem at once:

<!-- snippet: examples/cfg-doctor/session.txt#check-typo -->
```sh
$ agx config check --config ~/typo.jsonc
agx: typo.jsonc has 2 problems
  typo.jsonc:4:76: profiles.personal.modle: unknown key "modle"
  typo.jsonc:5:71: profiles.work.default: must be boolean, got string
[exit 1]
```

<!-- snippet: examples/cfg-doctor/session.txt#check-syntax -->
```sh
$ agx config check --config ~/syntax.jsonc
agx: syntax.jsonc:4:58: missing comma before '"'
[exit 1]
```

A file that does not read as config exits 1; a cross-check problem (an unknown login, two default profiles, an alias to an internal command) exits 2. Adding the `$schema` line to the file gives your editor the same checks as you type. See [fix what fails](guides/doctor.md#step-4-fix-what-fails).

### "config version 2 is newer than this agx"

The file was written by a newer agx. Update agx (`go install github.com/ubgo/agx@latest`).

### "config is being written by another agx"

Two agx processes tried to write the same file. The second waits 5 seconds for the lock, then stops without writing. Run the command again.

### "config.jsonc already exists"

`agx config init` never overwrites a config (exit 1); the message names it in the `~` form (`agx: ~/.agx/config.jsonc already exists`), while `--json` keeps the absolute path in `data.file`. Edit the file, or change one key with `agx config set`.

### `config set`: "takes true or false", "takes a number from 0 to 100", "is an object, not a scalar", "unknown key", "the change would make config.jsonc invalid"

`agx config set` changes one scalar key, types the value by the config schema at that key (a string, a number, an integer, `true`/`false`, or `null` where the key takes it) and validates the whole file before writing. Each refusal exits 2, names what the key takes, and writes nothing. A pool cap takes a number from 0 to 100, also on a member entry that is not in the file yet (`agx config set --key logins.auto.pool.accounts.bob.short --value 95`); agx before 0.4.10 refused every number with `is a float64, not a scalar`. For several pool numbers at once, use the pool flags of `agx login edit` (`--pool-cap-short NAME=N`, `--pool-cap-week NAME=N`, …):

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

Lists and objects have their own commands (`profile edit`, `project use`, `alias add`). See [change one key](guides/doctor.md#step-5-change-one-key).

### "3 checks failed" from `agx doctor`

At least one doctor row is `fail` (exit 1, `error.code` `checks-failed`). Each failing row says what is wrong, and many carry a `fix:` line, such as `chmod 600 ~/.agx/tokens.jsonc` for a tokens file other users can read. See [Check your setup](guides/doctor.md#step-4-fix-what-fails).

## Logins and secrets

### "login carol serves claude; this run is codex"

A login, from any source (a flag, `AGX_LOGIN`, a binding, a profile), must serve the run's agent. It is refused, never skipped, so you never run on an account you did not mean. Check `echo $AGX_LOGIN`, `agx project show` and `agx login show --id carol`.

### "login carol serves claude, not codex"

The same check when you bind a login: `agx project use --agent codex --login carol` refuses a login that does not serve that agent (exit 2). See [folder bindings](guides/folder-bindings.md#pitfalls).

### "--login and --home-login both choose the login; pick one"

Two choices for the same thing in one command are refused. The same applies to `--model` with `--setting model=…`, `--login` with `--setting apiKeyHelper=…`, and `--env` of a token variable with `--login`.

### "gopass timed out after 30s"

Your secret store is locked or slow. Unlock it (`gopass show -o some/entry` once) and run again. Every secret command has a 30 second limit, so a locked store never hangs agx.

### "token is empty", "token has 3 lines"

The reference resolved, but not to one token. Check the entry: `gopass show -o personal/claude/carol | wc -l` should print 1.

### "tokens.jsonc has mode 0644", "stored (tokens.jsonc unreadable; agx login check shows why)"

`tokens.jsonc` holds pasted tokens, so agx refuses it when group or others can read it, and `agx login list` marks every stored login unreadable instead of using it. agx always writes the file 0600; a wider mode comes from copying the file, restoring it from a backup, or checking it out with git (git keeps no file modes). Fix it with `chmod 600 ~/.agx/tokens.jsonc`. `agx login check` and `agx doctor` name the file and its mode.

### "gopass:personal/claude/carol is not in the gopass store"

The reference names a gopass entry that does not exist (often a typo in the path, or a store that is not mounted). `gopass ls` lists the entries; fix the reference in the login. A store that exists but cannot be read (locked, no key) gives gopass's own reason, except inside a live session, where `agx credential` says only `could not be read by gopass` and `agx login check --id NAME --no-probe` shows the reason.

### "no plugin for scheme gopas", "AWS_REGION is not set"

Expansion is strict: an unknown scheme (often a typo) and an unset variable are errors, never an empty value. Use `${VAR:-default}` when a variable is optional. `agx doctor` checks every reference in config without resolving it, and `agx doctor --resolved-secrets` resolves each one ([ask for more checks](guides/doctor.md#step-3-ask-for-more-checks)).

### "$(…) is a command, and env_file_commands is off"

An `env_file` may not run commands unless the profile sets `"env_file_commands": true`. This keeps a dotenv file from running code you did not expect.

### "--provider "nope": pi has no api-key provider by that name"

pi reads a different variable per provider, so a pi token login names its provider, and the name must be one pi knows for that kind (exit 2). The message lists them. Without `--provider` the message is `pi reads a different variable per provider; pass --provider: one of …`.

### "--resolved-secrets only works with --dry-run; add --dry-run"

`--resolved-secrets` resolves references for a dry run; on its own it would mean nothing, so a launch with it and no `--dry-run` is refused and starts nothing (exit 125).

### "--live-login needs a stored login; this run starts on the home login"

Live login switches the token a running Claude session fetches through `agx credential`, so the run must start on a stored token login: name one with `--login` (exit 125 otherwise). A one-off `--token-ref` and an `api-key-helper` login are refused for the same reason, and so is a profile whose environment sets a variable that outranks the helper. See [live login pitfalls](guides/live-login.md#pitfalls).

### "stdout is a terminal; pipe it (| pbcopy) or add --reveal"

`agx login token` prints a token only to a pipe or a file, so it does not land in your scrollback or a screen share by accident (exit 2). Pipe it (`agx login token --id NAME | pbcopy`) or add `--reveal` to print it on the terminal anyway. Like every `agx login token` error it is one line on stderr, without hint lines, so a script or an `apiKeyHelper` reads just the reason. `--json` and `--field` are refused too: the output is the bare value. See [print a login's token](logins.md#print-a-logins-token).

### "login token prints the bare value for scripts and apiKeyHelper; it takes no --json or --field"

`agx login token` prints the token alone, so a script or an `apiKeyHelper` reads it as is; wrapping a secret in a JSON envelope would put it where tools log harmless data. Drop `--json` and `--field` (exit 2).

### "--id is required: the login name (agx login list)" from `agx login token`

Name the login with `--id` (exit 2); `agx login list` shows the names.

### "unknown login NAME" from `agx login token`

No login has that name (exit 1). Check the spelling with `agx login list`; an `apiKeyHelper` that runs `agx login token --id NAME` fails the same way after the login is renamed or removed.

### "login NAME: VAR is not set" from `agx login token`

The login's reference did not resolve, so there is no token to print (exit 1); the line names the reference, never a value. Set the variable, or fix the gopass entry or command, and check with `agx login check --id NAME --no-probe`.

### "login vault is an api-key-helper: it has no token to print"

An `api-key-helper` login holds a command the agent runs, not a token, and `agx login token` does not run it for you (exit 4). Run the helper command yourself, or point `agx login token` at the token login the helper would read.

### Claude exits with "apiKeyHelper failed: did not return a value" (or "exited 1")

A live-login session asks `agx credential` for its key at every refresh. Since v0.4.23 a login that does not resolve to one token makes `agx credential` hand out the placeholder `agx-no-account-usable` and exit 0 (`when_none_usable: "stop"`, the default), so the session waits instead (see [the next entry](#claude-shows-an-authentication-error-401-and-agx-no-account-usable)). With `when_none_usable: "keep"`, or an older agx, it prints nothing and exits non-zero: a session that has no key yet then exits after three tries, with no fallback to the home's own login, and one that already has a key goes on with it. Claude's `/status` shows agx's one-line reason, for example `agx: login alice: token is empty (try: agx login check --id alice --no-probe)`. Run that check, fix the login, and resume the conversation (`agx resume --last --login NAME --live-login`); an ended run cannot be switched back. Check a login before you switch to it. See [when the helper has no key](guides/live-login.md#when-the-helper-has-no-key). A helper of your own that runs `agx login token --id NAME` fails the same way when that login does not resolve: `agx login check --id NAME --no-probe` shows why.

### Claude shows an authentication error (401) and "agx-no-account-usable"

The session's key helper had no key to hand out, so it gave Claude `agx-no-account-usable`, which is not a key: every member of the pool is over a cap (or disabled, or unreadable), a `--pool-only` member is over its cap, or the run's login does not resolve. Nothing more is spent; Claude retries the request about ten times over four to five minutes (`401 invalid x-api-key · retrying in 3s · attempt 3/10`), asking for a key before each try, and goes on by itself if one becomes usable in that time. `agx live list` shows the run as `IN USE  none (all over caps) since 10:52`, the status line as `pool auto: none (…) since …`, and `! agx whoami` adds the full reason under `no key:`. To go on sooner: wait for a reset, raise a cap (`agx login edit --id auto --pool-cap-week NAME=N`), take a disabled member back, or put this run on one member (`agx live use --id RUN --pool-force NAME`, which ignores her caps). After the last try Claude stops the turn with `Invalid API key · Fix external API key` and `Once fixed, resend your message`: type `continue` once an account is usable. See [when no account is usable](guides/pool-rules.md#when-no-account-is-usable).

### Sessions keep running on an account over its cap

`agx pool pick` says every member is over its caps, yet sessions on the pool keep working and a member's usage goes past 100% of its 5-hour window. Claude Code keeps using its previous key when its key helper fails, so a helper that only refuses does not stop it. agx v0.4.23 and later hand Claude a placeholder instead ([when no account is usable](guides/pool-rules.md#when-no-account-is-usable)); this happens when:

- the pool (or the `live` block) sets `when_none_usable: "keep"`: `agx pool show --id POOL` lists `when_none_usable` with its source; `agx config set --key logins.POOL.pool.when_none_usable --value stop` changes it (the next key request uses it);
- the sessions were started by an agx older than 0.4.23, whose helper exits 1: `agx version` shows yours. A running session runs the agx binary its launch named, so after an upgrade the next key request uses the new one.

`agx live list` shows which runs are on which member (`IN USE`; under `keep` it reads `alice (no key since 10:52: …; the agent keeps its key)`). To stop one now, end that session, or move it with `agx live use --id RUN --login OTHER`.

### A 429 right after a pool or live switch

`API Error: Request rejected (429)` mid-task means the account ran out before the next key refresh. Claude does not retry it on another key: type `continue`. The first `continue` can fail once more (the request that fetches the new key still uses the old one); the next runs on the new account. `/status` shows only `apiKeyHelper` throughout; `! agx whoami` shows the login in use. See [when an account runs out mid-task](guides/login-pool.md#when-an-account-runs-out-mid-task).

### "pool pick is for pool logins; login NAME is …"

`agx pool pick` and `agx pool show` read a pool login; NAME is a token login (exit 2). `agx pool list` shows the pools. The message says which agents can have a pool today (from their capabilities) and, for another agent's login, why it cannot. `agx login list` shows the kinds.

### Which account is my session on?

Look at the bottom of Claude's screen: a live or pool session shows [`agx statusline`](commands/live.md#agx-statusline) as its status line (`pool auto: alice · 5h 40% · week 61%`) unless you set your own, in which case run `agx statusline` from your status line script. Or run `! agx whoami` inside Claude (the `!` runs it in the session's shell, which has `AGX_RUN_ID`). In a pool or `--live-login` session it shows `login:  auto (pool)`, `in use:` (the login or member whose key Claude got last) and `next:` when a switch is pending, plus `only:  carol (caps enforced)` on a run pinned with `--pool-only` (and `in use:  none (carol over cap: …)` while carol is blocked); `--json` has `login_kind`, `in_use`, `only` and `next`. /status only says apiKeyHelper, and Claude's own guess reads the keychain login, not the key it uses.

### "pool auto: no member is usable: …"

A run pinned with `--pool-only` fails with its member's line instead (`carol: week is 70% (cap 70%)`); see [My pinned session stopped](#my-pinned-session-stopped-the-member-hit-its-cap). Every member of the login pool is skipped, and the line lists each with its reasons (`status is limit-reached`, `week is 70% (cap 70%)`, `disabled in config`). `agx pool pick --id auto` shows the same as a report; `agx login token` exits 1 with it; the key helper of a running session hands Claude `agx-no-account-usable` with it on stderr, so the session waits ([when no account is usable](guides/pool-rules.md#when-no-account-is-usable)). Wait for a reset, raise a cap (`agx login edit --id auto --pool-cap-week NAME=N`), take a disabled member back (`--pool-enable NAME`), add a member, or force one: `agx login edit --id auto --pool-force NAME` (clear it later with `--pool-force ''`). See [the rules](guides/login-pool.md#the-rules).

### My pinned session stopped: the member hit its cap

A run started with `--pool-only carol` (or pinned with `agx live use --id RUN --pool-only carol`) gets no key once carol is at a cap: the helper hands Claude `agx-no-account-usable` with one line such as `carol: week is 70% (cap 70%)` on stderr, Claude's requests fail authentication, and `! agx whoami` shows `in use:  none (carol over cap) since 10:52`. That is the pin working: it keeps carol's caps, so it hands out no key rather than going over one or switching to another member. It starts again by itself once her usage is under the cap. To go on now, hand the run back to the pool's rules (`agx live use --id RUN --login auto`), pin another member, or, if going over the cap is fine, pin carol without caps (`agx live use --id RUN --login carol`). `agx pool pick --id auto --pool-only carol` shows her reasons. See [Pin one account but keep its limits](guides/login-pool.md#pin-one-account-but-keep-its-limits).

### "codex cannot use a login pool (…)"

A pool needs an agent that asks for its key again while it runs and whose logins agx can probe; today that is Claude Code. The reason is the agent's own (exit 4 from `agx login add`; a config error at the line from `agx config check`).

### "member NAME is an api-key-helper; agx cannot fetch its key", "unknown login NAME" in a pool

Every pool member must be a token login (`oauth-token` or `api-key`) of the pool's agent, listed once across `own` and `shared`. Add the member as a login first (`agx login add`), or take it out of the pool.

### "--pool-cap-short NAME=N: N is not a number from 0 to 100", "NAME is not a member of pool auto"

The pool flags of `agx login add` and `agx login edit` check each value before anything is written (exit 2): a cap or cutoff is a number from 0 to 100, a member flag takes `NAME=N` and must name a login in the pool's `own` or `shared` (after this command's `--pool-own` and `--pool-shared`), and a duration is a Go duration such as `48h` (`--pool-key-ttl` at least `1s`). `agx login show --id auto` lists the members. Add `--dry-run` to see the changes before writing them.

### "login NAME is used by pool auto"

A pool lists the login as a member, so removing it would leave the pool naming a login that does not exist. `agx login remove --id NAME --cascade` also drops it from the pool's lists, its `accounts` entry and its `force`.

### "warning: probe of NAME failed (…); using its cached usage from 2m0s ago"

A pool member's usage probe failed for a passing reason (no network, a 429 without usage, an unexpected answer), so agx used its last cached reading; the pick may be stale. The next key request probes again. `agx login check --id NAME` shows the probe's own error.

## Launch options

### "--autonomy full needs --consent full-autonomy"

<!-- runnable: exit=125 -->
```sh
$ agx run --profile gem --autonomy full --dry-run --quiet
agx: --autonomy full needs --consent full-autonomy
```

Full autonomy lets the agent run any command without asking, so it needs an explicit second flag. See [autonomy](guides/autonomy.md#pitfalls).

### "Codex has no edits-only mode"

agx never runs an agent with more autonomy than you asked for, so a level the agent cannot honour exactly is refused rather than rounded up:

<!-- runnable: exit=125 -->
```sh
$ agx run --profile codex --autonomy accept-edits --dry-run --quiet
agx: codex: autonomy accept-edits: Codex has no edits-only mode; never mapped to a more permissive one
```

Use `ask` or `full` for Codex. pi has no permission modes: only `read-only` and `full` are accepted, and with no `--autonomy` it runs with a notice that it never asks. See [autonomy](guides/autonomy.md#pitfalls).

### "config.jsonc:32:86: profile typo settings cleanupPeriodDays: expected int"

A profile's `settings` (inline in the claude block, or the JSONC file it names) has a value of the wrong type for a key Claude Code models. The message starts with the file, line and column of that key (the settings file's own position when the settings live in a file), then the profile and the key (exit 125). Fix the value there. A `--setting KEY=VALUE` with a wrong type reads `--setting KEY: …`.

### "codex has no token login"

Codex authenticates only through its home, so `--login`, `--token-ref` and `--login-setup` are refused for it. Use a Codex profile per account: `agx profile add --name codex-work --agent codex --home ~/.codex-work`, then log that home in with `agx run --profile codex-work -- login` (Codex's own `codex login`, run with that profile's `CODEX_HOME`).

### "mycli has no token login (spec has no token_env)", "mycli has no oauth-token login; it takes api-key"

A custom agent takes token logins only when its spec has `token_env`, and only for the kinds it names (exit 125 on a launch, 4 from `agx login add`). Add a `token_env` entry for the kind with the variable the CLI reads, or use a kind it names. See [A custom agent: a token login](guides/custom-agent.md#step-6-a-token-login).

### "mycli: autonomy ask: not described by the agent spec"

A custom agent's spec has no template for what you asked: an `--autonomy` level missing from `autonomy_args`, `--headless` without `headless_args`, `--model` without `model_args`, and so on (exit 125). The message names the agent and what was asked. Add the template, or drop the flag.

### "mycli has no --option or --setting"

The agent has no typed options or settings flag (a custom agent, Gemini CLI, pi, opencode). Pass its flags as raw args after `--`, in profile `args` or in `agents.<agent>.defaults.args`.

### "--option settings: settings is not a launch option"

`settings` means agx's settings only: the `settings` key in the profile's claude block (a JSONC file or an object) and `--setting KEY=VALUE`. `--option settings=…` is refused (exit 125). To hand Claude Code a settings file of your own, pass it raw: `-- --settings ./my.json` (agx reads it and merges it over its own settings, your keys winning).

### "effort needs claude 2.1.320+, installed 2.1.300"

The option exists only in a newer version of the agent. Update the agent, or pass the flag raw after `--` if you know better.

### "not modelled (did you mean apiKeyHelper?)"

A settings key agx does not know passes with a warning. If it is a typo, fix it; if it is a new key, it still reaches the agent.

### "raw --settings apiKeyHelper and --live-login both set the credential"

A raw `--settings` (after `--`, in the profile's `args`, the agent's `defaults.args` or a Claude block's `extra_args`) is merged with agx's own settings, and `--live-login` adds an `apiKeyHelper` that runs `agx credential`. A raw `apiKeyHelper` next to it is two helpers in one command, so the launch is refused (exit 125); any other raw key is merged with the helper:

<!-- snippet: examples/launch-raw-settings/session.txt#live -->
```sh
$ agx run --profile live --dry-run --quiet -- --settings '{"theme":"dark"}'
...
settings  theme                                                                                         from raw --settings
          apiKeyHelper                                                                                  from --live-login
...
$ agx run --profile live --dry-run --quiet -- --settings '{"apiKeyHelper":"~/bin/key.sh"}'
agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one
[exit 125]
```

Drop `apiKeyHelper` from your settings to switch logins live, or drop `--live-login` to use your own helper. Older agx versions printed "raw --settings wins: … not applied" and dropped agx's settings instead; a raw `--settings` is now always merged (see [launch options, recipe 8](launch-options.md#a-raw---settings-of-your-own)).

### "raw --settings file … not found"

agx reads a raw `--settings FILE` itself: a relative path is taken from the launch folder (where Claude would look), a leading `~` is your home. A file with a syntax error names its line and column; a value that starts with `{` and ends with `}` is read as inline JSON instead of a path.

### A path with `~` is not found by the agent

No shell runs between agx and the agent, so agx itself expands a leading `~`, but only in options agentio marks as paths: Claude Code's `add_dirs`, `mcp_configs`, `system_prompt_file`, `append_system_prompt_file`, `plugin_dirs`, `debug_file` and Codex's `cd`, `add_dirs`, `images`, in the profile block, `--option`, and after those options' flags in profile `args` and agent default args. Anywhere else (another option, a quoted `'~/x'` after `--`, Gemini CLI, pi, opencode, a custom agent) the agent gets a literal `~`: write an absolute path there. See [launch options, recipe 7](launch-options.md#block-keys-with-no-flag-of-their-own-prompt-and-extra_args).

### "before_run failed (exit 3)"

The profile's `before_run` script failed, so the agent did not start (exit 125). Run the script by hand in the launch folder to see why. With `"before_run_from": "folder"` a missing script is refused with its path.

## Resume

### "no terminal for the picker; pass --id or --last"

`agx resume` without `--id` or `--last` shows a picker, which needs a terminal. In scripts, pass `--id` or `--last`. See [conversations pitfalls](guides/conversations.md#pitfalls).

### "3f2a matches 2 conversations"

The id prefix is stored in two homes. Give a longer id, or `--profile` to say which home.

### "conversation 3f2a was in /old/path, which no longer exists"

The agent resumes a conversation in its own folder. Restore or recreate the folder, or move the conversation with `agx conversation move`.

### "~/.claude has 2 profiles (kimi, personal) and no default; add --profile"

Several profiles share the conversation's home and none is the default, so agx will not pick one (exit 125). Mark one with `agx profile use --id NAME`, or pass `--profile`. See [let a resume find its profile](guides/shared-homes.md#step-3-let-a-resume-find-its-profile).

### "mycli cannot resume conversations: config-only agent: …"

A custom agent defined in config has no conversation reader, so agx cannot find or resume its conversations. `agx resume` refuses up front when the command chose that agent (`--agent mycli`, or a profile of it) or when no other home is left to search (exit 125). Start the agent with `agx run --profile …` and use the agent's own resume flags as raw args after `--`.

### "--limit -1: must be 0 (no cap) or more"

`--limit` on `agx resume` and `agx conversation list` is how many conversations are considered, newest first; `0` means all of them (exit 2 for a negative value).

## Sessions and the shell layer

### "reserved.jsonc:8:5: alias "agx": a name may not be agx, command, builtin, cd, set, function"

An alias written into the config by hand has a name the shell layer cannot define (`agx`, which would replace its own wrapper, or a shell builtin), or a word starting with `=` or `@`. The config is refused at load with the line and column, for every command (exit 2); `agx shell-init` still prints the wrapper, without aliases, and warns. Rename or remove the alias. `agx alias add` refuses the same names up front.

### "plan file is older than 60s or not yours"

The shell layer's launch plan expired between `agx new`/`resume` and the launch (for example, the shell was suspended). Run the command again. See [how the `cd` works](shell.md#how-the-cd-works-the-plan-file).

### "plan file schema_version N, this agx reads M; reload the shell layer (agx shell-init)"

The shell layer loaded in this terminal came from another agx version than the one now on `PATH`. Open a new terminal, or run the `eval "$(agx shell-init --shell …)"` line again. See [shell layer pitfalls](shell.md#pitfalls).

### "this folder has N Codex conversations; add --no-codex"

`agx session promote` would move a folder whose Codex conversations Codex indexes by absolute path, so it stops before touching a file. Add `--no-codex` to promote anyway and leave those conversations behind. See [promote a session folder](guides/session-folders.md#step-5-promote-a-session-folder-into-a-project).

## Usage

### Some rows failed, exit 3

Some homes answered and some did not (no network, a rate limit, an expired login). The rows that answered are shown. When every row fails the exit is 1. `agx usage --cached` shows the last readings without the network.

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

### `usage --cached` shows `no-cache` or `stale` for a Claude home

Claude Code 2.1.323 and later no longer store a usage reading on disk, and do not refresh one an older version left. A Claude home therefore shows `no-cache` (no reading) or `stale` (a reading more than a day old, kept with its age; not a failure, the exit code is unchanged). Use live usage (`agx usage`) for Claude; `--cached` still works for Codex. See [cached readings and Claude Code 2.1.323](guides/usage.md#cached-readings-and-claude-code-21323) and [usage: live, cached or probed](choosing/usage-live-or-cached.md).

### "auto profile: none eligible"

`--auto-profile` found no profile with headroom; the message lists each candidate and why (rate-limited until a time, login expired, bills per token). Agents without plan usage (Gemini, pi) cannot be auto-picked: choose a profile. See [auto-pick pitfalls](guides/auto-pick.md#pitfalls).

## Output and flags

### "unexpected argument "work"; every input is a named flag"

Every input is a named flag: `agx run --profile work`, not `agx run work`. Exit 2, or 125 on a launching command.

### "--field data.homee: no such path (did you mean data.home?)"

The `--field` path is not in the envelope (exit 2, or 125 on a launching command). `agx help --json` lists each command's paths; with `*`, every item must have the key. On an empty list the path is checked against those declared paths, so a misspelled column fails there too. See [Scripting with agx](guides/scripting.md#pitfalls).

### "--agent given twice (gemini and pi); pick one"

A flag that takes one value was given twice (exit 2, or 125 on a launching command), even with the same value twice (`--quiet given twice; pick one`). A common cause: an alias that already sets the flag (`cl` = `run --profile work`) and the same flag typed after it (`cl --profile other`). Keep one. Repeatable flags are marked `"repeatable": true` in `agx help --json`.

### "--color "purple": use one of auto, always, never"

`--color` takes `auto` (the default), `always` or `never` (exit 2). `auto` colours tables, usage bars and agx's error and warning lines only on a terminal, so piped output is plain; `--json` is never coloured. If escape codes reach a file, something passed `--color always`; use `--color never` or set `NO_COLOR=1`.

### "invalid argument "soon" for "--timeout" flag"

`--timeout` takes a duration such as `5s` or `1m` (exit 2).

## Runs and cleanup

### Run folders pile up

Every launch gets a folder under `~/.agx/runs/` that holds its settings file and state. agx execs into the agent, so it cannot remove the folder when the agent ends. `agx doctor` reports ended runs; `agx clean` deletes them. A run whose process cannot be checked is kept and listed. See [clean up ended runs](guides/run-folders.md#step-5-clean-up-ended-runs).

## Windows

Windows builds are a preview until they are verified on a real Windows machine. Windows has no exec, so agx stays as the parent process: it forwards Ctrl-C and exits with the agent's exit code. `--dry-run` says so. Process checks for `agx clean` use only the start time there. See [exit codes, exec and Windows](guides/run-folders.md#step-6-exit-codes-exec-and-windows).
