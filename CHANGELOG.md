# Changelog

All notable changes to agx are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and agx uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.17] - 2026-11-21

### Changed

- Claude Code 2.1.330 is tested: built on agentio `claude` v0.1.4 and `builtin` v0.1.3, whose tested range for Claude Code is now 2.1.323..2.1.330. `agx run`, `agx doctor` and `agx agent list` no longer warn that an installed 2.1.330 is outside the tested range.

### Fixed

- A conversation whose transcript holds a record with a missing or malformed `timestamp` (Claude Code wrote such records; 2.1.330 fixed `--resume` failing on them) is read in full: agentio used to skip that record, so a title or last prompt on it could go missing.

## [0.4.16] - 2026-11-20

### Changed

- Login pools have no strategy mode: every rule is a pool key with a default, and the defaults are the pool simulator's recommendation across solo users, small and large teams: `order` `urgency` (the member with the most room to lose per hour first, room weighed by `capacity`; `deadline` = the soonest weekly reset first), `week_relax_window` `24h` and `week_relax_max` 95 (a member's weekly cap rises to 95 over the last day before its reset; the 5-hour cap never rises), `max_sessions` 4 (each session gets its own pick, a member takes four before the next one is used), `sticky` `true` with `sticky_leave_ratio` 3, `shared_borrow_window` `48h`. The `strategy` key and `--pool-strategy` are gone (an unknown key now); `--pool-order` sets the order.
- Own members without `own_caps` or a member entry default to 5h 95% and week 98%; `defaults` is the caps of shared members (50/70).
- `agx pool show` lists every value in effect with its source (`default`, `pool`, `group` or `member`), members with their `week_relax_max` and `capacity`; `agx pool pick` prints the order, a `Rules:` line, each member's caps with their source and its urgency, and a run's dry run lists the rule values.
- A probe's per-model window at 100% no longer marks the login `limit-reached`: only the all-models windows do.

### Added

- `agx pool preset --name recommended|classic --id POOL [--dry-run]` writes a named set of rule values into the pool's block through the comment-keeping editor: `recommended` is the defaults, `classic` the rules before this release (soonest reset first, no relax, every session on one pick). Each value is an ordinary key afterwards.
- Per-model weekly windows. agx reads every `anthropic-ratelimit-unified-<window>` header of a probe (agentio v0.1.2, `claude` v0.1.3), so Fable 5.2's own weekly window (`7d_oi`) and any future one are seen with their status and `representative-claim`. A pool probes with the session's model: the run's `--model`, profile `model` or Claude's own setting (recorded in `live.jsonc` at launch), else the new `probe_models` key (`--pool-probe-model`), else the default profile's model; `agx pool pick --model` stands in for a run. Each per-model window is an extra weekly cap for that model's sessions with the same cap and relax rules, a rejected one blocks the member for that model until its reset, and the room is the least over the windows; `agx pool pick` shows them (`fable week 80%`) and names the probe model. A model the API refuses falls back to the default probe with a warning. The usage cache keeps each model's windows.
- Usage history: every answered probe adds a point (time, probe model, each window's percent and reset) to `~/.agx/cache/login-usage-history.jsonc`, at most one per member every 10 minutes, 8 days and 1200 points kept, with no extra API call. The pool log line adds the picked member's usage (`usage="5h=12 week=40 fable_week=61"`) and the probe models.
- `agx usage --json` windows carry `id`, `status` and `binding` when the source gives them.
- The pool simulator models per-model weeks (scenario key `model`, scenarios `fable-heavy-solo`, in the gate, and `fable-heavy-team`; `REC-blind` shows the 429s reading them saves), and its reference `A` is the `classic` preset written as config. Guide [How the pool chooses](docs/guides/pool-rules.md): the rules in order, every key and default, the presets, per-model windows, the usage history and the 60% shared weekly cap for heavy-use teams.

## [0.4.15] - 2026-11-20

### Added

- agx detects the kind a Claude token really is from its public prefix, since a login's `kind` is only a label in config: `oauth-token` (a setup-token, billed to the plan), `api-key` (an API key, billed per use) or `unknown`. Only the kind word is ever shown, never any part of the token, the prefix included.
- `agx login list` has a DETECTED column (`detected`, `detected_mismatch` and `detected_view` in `--json`), filled for pasted, plain and `${VAR}` tokens and for kinds a pool probe recorded; `--verify` resolves every token login (runs gopass and `$(…)` references, cost X). A kind that contradicts the login's reads `api-key (mismatch)`. `agx login show` and `agx login check` show it too.
- A login declared `oauth-token` whose token is an API key is refused (code `per-use-key`) by `agx run`, `agx new`, `agx resume` and `agx credential`, with one line naming the login. Allow it for one run with `--allow-api-key` (kept for a live run's key requests) or for the login with `logins.NAME.allow_api_key: true`. A login declared `api-key` is unchanged, and `agx login check` fails such a login without probing it.
- Login pools never pick a member whose token is a per-use API key unless that member's `allow_api_key` is set: `agx pool pick` lists it as `skip: token is an API key (billed per use); allow_api_key is off`, it is never probed, a forced or pinned one is refused, and a member found to be one while its key is handed out is recorded in the login-usage cache (`detected`) and the pool picks again.
- `agx whoami` adds the detected kind to `in use:` (`alice (detected oauth-token)`; `in_use_detected` in `--json`), recorded by `agx credential` in `live.jsonc`.
- Guide section [Is a login billed per use?](docs/logins.md#is-a-login-billed-per-use), a README FAQ entry, the example `examples/acct-login-detect`, and the agx skill text.

### Changed

- `agx run --dry-run --resolved-secrets` on a named login says `(resolved, detected KIND)`.

## [0.4.14] - 2026-11-20

### Changed

- Claude Code 2.1.328 and 2.1.329 are tested: built on agentio `claude` v0.1.2 and `builtin` v0.1.2, whose tested range for Claude Code is now 2.1.323..2.1.329. `agx run`, `agx doctor` and `agx agent list` no longer warn that an installed 2.1.328 or 2.1.329 is outside the tested range. agentio's drift check on real 2.1.328 and 2.1.329 homes found no new transcript, settings or session shape.

## [0.4.13] - 2026-11-20

### Added

- opencode is a built-in agent (agentio `opencode` v0.1.0, tested with opencode 1.18.30). It shows in `agx agent list`, `agx agent show --id opencode`, `agx doctor` (binary on PATH, tested range) and in the known-agents list of an unknown-id error. A profile can use it (`"agent": "opencode"`, home `~/.local/share/opencode` or another data dir); a non-default home is selected with `XDG_DATA_HOME` set to the folder that contains the `opencode` data dir. `--model`, `--prompt`, `--headless` (`opencode run --format json`) and resume (`--session ID`) work; `--autonomy full` becomes `--auto`, `default` adds nothing, and `read-only`, `ask` and `accept-edits` are refused (exit 125) because opencode expresses them only as permission rules in `opencode.json`. `agx conversation list` reads the sessions opencode kept before 1.2.0 (newer ones live in its `opencode.db`, not read yet). opencode has no plan usage, token login or live login, each reported with its reason.
- Config-defined agents can match an npm install run through an interpreter: `agents.<agent>.spec.process.script_basenames`, `script_contains` and `interpreters` (absent: node, nodejs, bun, deno), in the config schema and the config reference.
- `agx agent show` and `agx agent list` list agentio's new capabilities: `process-sessions`, `stop-planner`, `turn-files` and `hooks`, each with its cost and, where missing, the agent's reason.

### Changed

- Built on agentio v0.1.1 (root, `claude`, `codex`, `gemini`, `pi`, `builtin`) and `opencode` v0.1.0. Process matching now also tests the raw command line and ignores a Windows `.exe`/`.cmd`/`.bat` ending, and an `npm install` of an agent package is no longer taken for the agent.

## [0.4.12] - 2026-11-19

### Added

- `--pool-only NAME` pins one pool member with its caps enforced, on `agx run`, `agx new`, `agx resume`, `agx live use`, `agx pool pick` and `agx login token`: every key comes from that member while the pool's rules allow it (its caps, the 5-hour cutoff, `enabled`, its status), and no key while they do not, so Claude's request fails with one line (`carol: week is 70% (cap 70%)`, exit 1, `auto-pick`) instead of switching to another member or going over the cap; it starts working again by itself once usage is under the cap. The pool's `force` and its picker do not apply to a pinned run. `--pool-only-short N` and `--pool-only-week N` lower that member's caps for the run or call only (a value above its own cap is refused). `--pool-only` with `--pool-force` is refused (exit 2; 125 from a launch), as are a name that is not a member and, at launch, a disabled member. `agx live use --id RUN --pool-only NAME` pins a running session, `--login POOL` clears the pin, and the run keeps it in `live.jsonc` (`pool_only`, `pool_only_short`, `pool_only_week`, `no_key`). `agx whoami` and `agx live show` add `only: NAME (caps enforced)`, and `in use: none (NAME over cap: …)` while it is blocked (`--json`: `only`, and `no_key` on live runs). The dry run shows the pin and the member's effective caps, each with its source. A pinned pick's `picked_by` is `only`.

## [0.4.11] - 2026-11-19

### Changed

- `agx login pick` is now `agx pool pick`, in a new `agx pool` group: `login` names a login (a noun), and picking applies to pools only. Same flags, output and exit codes. `agx login pick` is an unknown command (exit 2) whose hint names `agx pool pick`; there is no alias. A pool command given a login that is not a pool says `pool pick is for pool logins; login NAME is KIND` (or `pool show …`) with the hint `agx pool list`. Pools are still created and changed with `agx login add --kind pool` and `agx login edit --pool-…`, and `agx run --login auto` is unchanged.

### Added

- `agx pool list`: every pool login with its agents, own and shared members, force and picker, as a table or `--json`.
- `agx pool show --id NAME`: one pool's full settings (members with their caps and enabled state, force, defaults, borrow window, cutoff, `usage_max_age`, `key_ttl`, picker, log), each with its source: `config`, `defaults` (a member cap from the pool's defaults) or `default`; `--json` too.

## [0.4.10] - 2026-11-19

### Fixed

- `agx config set` accepts every scalar the config schema allows at the key: a number (`--key logins.auto.pool.accounts.alice.short --value 95` was refused with `is a float64, not a scalar`), an integer, `true`/`false`, a string, and `null` only where the key takes it (a pool's `force` does; a cap does not). The value is typed by the schema at the key, never by the Go type or the value already in the file, so a key that is not in the file yet works too, such as a cap on a member entry that does not exist. Digits on a string key stay a string; a quoted value (`'"null"'`) is a string. A wrong value is refused naming what the key takes (`--key logins.auto.pool.accounts.alice.short takes a number from 0 to 100, not "101"`, `takes one of: plan, api`, `takes an integer`), never a Go type name, and nothing is written. A typed agent block key (`profiles.work.claude.effort`) can be set too.
- `agx config show` lists a pool's caps and cutoff (number values were left out).

### Added

- Pool settings as flags on `agx login add --kind pool` and `agx login edit`, so no pool number needs a hand edit: `--pool-cap-short NAME=N` and `--pool-cap-week NAME=N` (a member's own caps; `NAME=` removes one), `--pool-disable NAME` and `--pool-enable NAME`, `--pool-default-short N`, `--pool-default-week N`, `--pool-short-cutoff N`, `--pool-borrow-window DUR`, `--pool-usage-max-age DUR`, `--pool-key-ttl DUR` and `--pool-picker CMD`; an empty value removes the key so its default applies, and a member entry, `accounts`, `defaults` or `picker` block left empty is removed. Each changed key prints its value before and after, as `agx profile edit` does; `--dry-run` prints the changes and writes nothing; flags that match the file print `no change` and write nothing. A cap outside 0 to 100 (`NaN` included), a member the pool does not list, `--pool-disable` and `--pool-enable` for one member, a bad or negative duration and a `--pool-key-ttl` under `1s` are refused before anything is written. `--json` adds `changes` and `written`.
- The published config schema states the pool's ranges: every cap and the cutoff is a number with `minimum` 0 and `maximum` 100, and caps, the cutoff and `enabled` are never `null`, so an editor flags them as you type and `agx config check` reports them at their line.

## [0.4.9] - 2026-11-19

### Added

- `agx skill` prints a Claude Code skill (`SKILL.md`) that teaches an agent session how to drive agx: when to use it, how agx works (profiles, logins, login pools, the resolution chain, `--dry-run`, `--json`/`--field`, exit codes), then every visible command with its descriptions, examples and flags, generated from the binary's own command tree and stamped with its version. `agx skill --install` writes it to `~/.claude/skills/agx/SKILL.md` (folder 0755, file 0644), backing up an existing file to `SKILL.md.bak-<timestamp>`; `--dir` chooses another skills folder. `agx help --json` now also carries each command's `long` and `example`.

## [0.4.8] - 2026-11-19

### Added

- Login pools can be overridden at every level: `--pool-force MEMBER` on `agx run`, `new` and `resume` forces one member for every key of that run (kept in the run's `live.jsonc`; `agx live use` ends it), and on `agx login pick` and `agx login token` for one call; `agx live use --id RUN --login MEMBER` pins a running pool session and `--login POOL` hands it back to the pool's rules; `--login MEMBER` and `AGX_LOGIN` bypass or choose the pool like any login. Who decides a key, first match wins: a run's `--pool-force`, the pool's `force`, its picker, the built-in rules.
- A pool's own picker: `logins.<name>.pool.picker.command` (and `picker.timeout`, default `10s`) runs at every key request with the members and their usage as JSON on stdin (version 1, never a token) and prints one member's name. A failure (a non-zero exit, a timeout, no line, extra lines, a name that is not a member) falls back to the built-in rules with a warning.
- A dry run on a pool shows each pool decision with its source: the members, who picks (`--pool-force`, the pool's `force`, its `picker` or the built-in rules), `key_ttl`, `usage_max_age`, the borrow window, the cutoff, the default caps (each `config logins.NAME.pool.KEY` or `default`) and the pick from the cache.
- The pool rules are proven three ways: example tests for every rule, edge and config mistake; randomised tests against a second, independent implementation of the rules with invariants (seeded, replayable); and `task mutation` (opt-in, not in `task check`): breaks the pool's rules and validation on purpose 46 ways on a temporary copy and fails unless the tests catch every break.
- `agx login pick` takes `--time-format`, `--duration-format`, `--tz` and the global `--date-format`, as `agx usage` does, and a hidden `--now` to read an old cache; `--max-age 0` probes every member.
- `agx usage --duration-format truncate|round|minutes` and the config key `usage.duration_format`: how a reset a day or more away reads. `truncate` is the default and unchanged ("1 d 2 hr"); `round` rounds to the nearest hour ("1 d 3 hr" for 1 d 2 hr 52 min); `minutes` keeps the minutes ("1 d 2 hr 52 min"). Another value is refused (exit 2) with the list.

- `agx whoami` inside a pool or `--live-login` session shows `login:  auto (pool)`, `in use:` (the login or member whose key the agent got last) and `next:` when a switch is pending; `--json` adds `login_kind`, `in_use` and `next`. Inside Claude, `! agx whoami` answers "which account is this session on?", which /status (it says apiKeyHelper) and Claude's own guess (the keychain login) cannot.

### Changed

- `agx whoami` in a live session prints `in use:` and `next:` lines instead of the single `live: started on …` line.
- `agx login pick` prints `Pool: NAME (AGENT)`, then the stand-alone selector's report line for line: `Use: NAME (GROUP)` (or `(FORCED, ignoring rules)` with an indented `warning:` line per ignored rule), the reason, `warning:` lines, `Source: live agx result` or `cache (25s old)`, then `Candidates (ready accounts in pick order):` with one `- NAME [GROUP] …` line per member (`reset passed` for stale data, `reset unknown` without one). Its `--json` carries the selector's fields in snake_case (`room_percent`, `hours_to_reset`, `rank` null unless ready, `picked_by`). `--force` is renamed `--pool-force`. On a login that is not a pool it says pick is for pool logins and which agents can have one today, from their capabilities. The dry run's first pool row names the agent (`auto (claude): own …`).
- Pools are strict: `"force": ""` and an explicit `null` for a cap, the cutoff or `enabled` are config errors; a reading whose usage is not a finite number blocks the member (`week usage is unreadable`) instead of passing its cap; an unreadable weekly reset is noted and sorts as unknown; `pool.Select` refuses a selection without a clock (`ErrBadNow`).

### Fixed

- A pool member that is disabled and has no cached reading lists both reasons (`disabled in config`, `no usage reading`), as the selector did; a reading exactly `usage_max_age` old is probed again; a failed probe is no longer cached, so the next key request asks again. The pool log has one line for every pick, a failed one included (`picked=null`, `error=…`), with `by=` (rules, force or picker).

## [0.4.7] - 2026-11-19

0.4.7 was released before its tests were complete; 0.4.8 completes it (the full test port, the overrides and picker, the strict config and the selector's report) and is the version to use.

### Added

- Login pools: a login of kind `pool` (Claude Code only) lists your own logins (`own`, used up almost fully) and teammates' shared ones (`shared`, borrowed up to their caps), with per-member caps, a borrow window, a 5-hour safety cutoff and an optional `force`, as `logins.<name>.pool` in `config.jsonc`. Each time Claude asks for a key, agx picks one member and hands out its token: usable members only (every reason a member is skipped is listed), your own first plus teammates' accounts whose weekly reset is within `shared_borrow_window` (48h), soonest weekly reset first. `agx run --login auto` (or a profile, agent or folder bound to the pool) is always a live run, with `apiKeyHelper` running `agx credential` and `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` from the pool's `key_ttl` (60s); `agx live use --login MEMBER` pins one member for a run. Guide: [Share a pool of Claude accounts](docs/guides/login-pool.md).
- `agx login pick --id POOL` prints the pick, the rule that chose it and every member (`#N READY`, `WAIT`, `skip:` with reasons, use, reset, room), with `--json`, `--cached`, `--max-age` and a one-run `--force`; it never prints a token. Exit 0 when a member is picked, 1 with error code `auto-pick` when none is usable.
- `agx login token --id POOL` prints the picked member's token; `agx login add --kind pool --pool-own NAME --pool-shared NAME` and `agx login edit --pool-own`, `--pool-shared`, `--pool-force`.
- The members' usage comes from one token probe each, kept in `~/.agx/cache/login-usage.jsonc` (0600, written atomically, shared by every session) and reused while younger than the pool's `usage_max_age` (60s); a probe that fails for a passing reason falls back to the cached reading with a warning on stderr. An optional `log` file gets one line per key request (time, pick, source; never a token).
- The public package `github.com/ubgo/agx/pool` holds the selection rules as a pure function.

### Changed

- `agx login remove` refuses a login a pool lists as a member; `--cascade` also drops it from the pool. `agx usage --all-logins`, `--auto-login` and `agx login check` leave a pool out (it has no token to probe; its members are logins of their own). `--auto-login` keeps its own rule (most headroom, once at launch); [Let agx pick by headroom](docs/guides/auto-pick.md) compares the two.

## [0.4.6] - 2026-11-19

### Changed

- `agx login token` prints every error as exactly one line on stderr, without hint lines, and leaves stdout empty: an unknown login, an `api-key-helper` login, a missing `--id`, a reference that does not resolve, and the refusal of a terminal, which now reads `stdout is a terminal; pipe it (| pbcopy) or add --reveal`. `--json` and `--field` are refused the same way instead of with a JSON envelope on stdout. Exit codes are unchanged.

## [0.4.5] - 2026-11-19

### Fixed

- A dry run on an `api-key-helper` login (or a `--token-ref` of that kind) lists the settings key the helper adds, `settings apiKeyHelper … from login NAME`, and the 0600 settings file it goes in, as a `--live-login` run does. It showed only the `--settings` file on the command line before. Keys only: the helper command is never shown.

## [0.4.4] - 2026-11-19

### Added

- `agx login token --id NAME` prints one login's token (the value and a newline on stdout, nothing else) for a pipe or a Claude Code `apiKeyHelper`: `--helper-command 'agx login token --id work-key'` makes any agx login the helper. It resolves the stored value or reference and never refreshes a token; stdout on a terminal is refused unless `--reveal`, `--json` and `--field` are refused, and an `api-key-helper` login is refused (agx does not run its command). Every other output still shows `[redacted]`.

## [0.4.3] - 2026-11-18

### Changed

- First public release of github.com/ubgo/agx. Test fixtures use placeholder login names.

## [0.4.2] - 2026-11-18

### Changed

- The human output is agx v0.3's again (the owner's reference look): `agx usage` prints the claude.ai-style bars (bold account header, `█`/`░` bars, a `Weekly limits` heading, `resets in 2 hr 50 min`), and `--table` has v0.3's columns `#`, `LOGIN` (`PROFILE` for homes), `PROVIDER`, `ACCOUNT`, `5H USED`, `5H RESETS`, `WEEKLY USED`, `WEEKLY RESETS`, `RESET DAY`, `RESETS ON`, a pair per model window, `LEFT`, `STATUS`, with the `Weekly resets:` summary and failed rows' full reasons under it; `#` is the login's (or home's) place in the config file and stays under `--sort`. `--plan` adds v0.3's plan table.
- `agx profile list` lists profiles in the file's order with v0.3's columns (`*` marks each agent's default, `ACCOUNT` and `STATE` from the login on disk); `agx login list` and `login check` follow the file's order; `agx doctor` prints one line per check (`✓`, `·`, `!`, `✗`) with its fix below; `agx session list` shows `LAST ACTIVITY` as "3 min ago" and the root on stderr; `agx conversation list` and the resume picker print v0.3's one-line rows (when, agent, profile, title, folder, id prefix); `agx whoami` prints `label:  value` lines; empty `login list` and `project list` say how to add one.
- `--color` now also colours the human output on stdout, decided per stream: bold headers, dim indices and sources, blue/amber/red statuses, coloured usage bars; `auto` only on a terminal and without `NO_COLOR`, `always` forces, `never` and `--json` stay plain.
- `agx usage --json` rows carry `index`, `plan_label` and, for homes, the `account` email read from disk.
- The usage table's `STATUS` cell holds only the status words (`expired`, `limit reached`, `rejected`), as v0.3's did; every row's reason is a `  name: reason` line under the table.
- Messages no longer carry library prefixes: `agentio: login expired at …` reads `login expired at …`, `--option sandbox: agentio: invalid launch option` reads `--option sandbox: invalid launch option`, in errors, warnings, hints, usage reasons, doctor findings and `--json` alike. Agent names (`codex: autonomy accept-edits: …`) stay.

### Fixed

- Every printed clock time rounds to the nearest minute, the same in every column, mode and `--date-format` (a reset at 22:19:50 reads `22:20` everywhere, as in v0.3); durations keep v0.3's rule.
- The shell layer survives your own aliases and functions: bash and zsh parse its definitions with alias expansion off (restored right after), so `alias do=…`, `alias then=…` or `alias fi=…` no longer break the whole layer at load, and its loops are gone; every shell calls the commands it runs as `builtin cd`, `builtin printf`, `command rm`, `command mktemp`, `command hostname` (PowerShell: module-qualified cmdlets), so an alias or function named `cd`, `printf` or `rm` is never run instead.

### Added

- `agx usage --time-format relative|day|absolute|both` and the config key `usage.time_format` (default `relative`). `both` keeps the duration and adds the clock time: `3 hr 10 min (22:20)`, with the weekday when the reset is not today (`1 d 4 hr (Thu 23:30)`) and the date when it is more than six days away, in the bars, the table and the per-model columns.
- A date and time format of your own: the global flag `--date-format` and the config keys `date_format` (every screen that prints a date) and `usage.date_format` (`agx usage` only, first). Presets `iso`, `short`, `long` (usage's default), `us`, or tokens `YYYY MM MMM DD D ddd dddd HH H hh h mm ss a A` with words in `[brackets]`; it writes the clock and date parts of `day`, `absolute`, `both` and `RESETS ON`, table timestamps and `conversation show`. An unknown token or an empty value is refused (exit 2) with the token list.

## [0.4.1] - 2026-11-18

agx v0.4.1 is the first release of a complete rewrite of `github.com/khanakia/agx` v0.3 under the new module `github.com/ubgo/agx`, built on [agentio](https://github.com/ubgo/agentio) v0.1.0 and [textdata](https://github.com/ubgo/textdata), with no backward compatibility. See [Coming from v0.3](docs/coming-from-v0.3.md).

### Added

- Built-in support for Claude Code, OpenAI Codex, Gemini CLI and pi, plus any agent described in config under `agents.<agent>.spec`.
- A typed JSONC config (`~/.agx/config.jsonc`) checked on every load, with unknown keys and wrong types reported at their line and column; writes keep comments and layout under a cross-process lock; a published schema for editor completion.
- One resolution chain for agent, profile, home, login, binary, folder and args, with `--dry-run` printing every decision and its source and a stderr note whenever something other than a flag decided.
- Launching commands `run`, `new` and `resume` that exec into the agent on macOS and Linux, with one run folder per launch and `AGX_RUN_*` variables in the agent's environment.
- Per-launch options for every agent: `--model`, `--prompt`, `--add-dir`, `--autonomy` with `--consent`, `--headless`, typed `--option`, `--setting` with file or inline delivery, `--env`, `--no-env`, `--env-file`, `before_run` scripts, and raw args after `--`.
- Named logins (`oauth-token`, `api-key`, `api-key-helper`) from references (`${gopass:…}`, `$(…)`, `${VAR}`), plain values or a 0600 token store; `AGX_LOGIN` and `AGX_LOGIN_<AGENT>`; strict expansion that never yields an empty or unresolved secret.
- Per-agent folder bindings (`agx project`), profiles with discovery on request (`agx profile list --discovered`, `agx profile import`), session folders (`agx session`), conversations (`agx conversation`), plan usage across homes and logins (`agx usage`), live login switching for Claude (`agx live`), `agx doctor` and `agx clean`.
- A shell layer for zsh, bash, fish and PowerShell: `cd` after `new` and `resume`, per-terminal logins, and config aliases as shell functions.
- One JSON envelope for every command with `--field` path selection, stable `error.code` values, documented exit codes, and `agx help --json` generated from the parser's declarations.

### Changed

- `--color` is validated and honoured: agx colours its own error line red and warning lines yellow on stderr (`auto` when stderr is a terminal and `NO_COLOR` is unset or empty, `always`, `never`); stdout and `--json` are never coloured, and any other value is refused (exit 2).
- A flag that takes one value, given twice, is refused with `--agent given twice (gemini and pi); pick one` (exit 2; 125 on a launching command), even when both values are equal.
- Launch output redacts by a value's source (a login token, a reference, an env file, `tokens.jsonc`, an agent's credential variable) and by the value wherever else it appears, never by a variable's name: a literal such as `CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000` or `MY_SERVICE_TOKEN=abc` is shown as written, and `agx doctor` flags a literal profile `env` value whose name looks secret.
- Keys of one concern from one input launch together: a profile with Claude's `print` and `output_format`, or `--option print=true --option output_format=json`, gives `claude -p --output-format json`; a contradiction inside one input is refused with the agent's reason.
- `settings` means agx's settings only: `--option settings=…` is refused and points at `--setting`; the valid-keys list no longer offers it.
- A raw `--settings` (after `--`, in profile `args`, agent default args or `extra_args`) is merged into agx's settings instead of replacing them: agx reads the value (inline JSON or a JSONC file, a relative path from the launch folder), deep-merges it over the profile and `--setting` keys (your keys win; objects merge, anything else replaces; several values merge in order), takes it out of the args and passes exactly one `--settings`, even when it has no settings of its own. Claude keeps only the last `--settings` wholesale, so the old "raw --settings wins: … not applied" behaviour silently disabled a live-login helper. The dry run sources each raw key `from raw --settings`; a missing or invalid file is an error naming it (line:col for bad JSONC); a raw `apiKeyHelper` with `--live-login` is refused (exit 125).
- `agx resume --limit 0` means no cap, as in `agx conversation list`; a negative value is refused.
- A shell layer made with `--config X` passes it on every call it makes, aliases included; a `--config` the user types wins.
- Alias names and words are checked on every config load: a name or word the shell layer cannot define is refused at its line and column.
- A config-defined agent whose spec has `token_env` takes token logins (`agx login add --agent mycli`, `--login`, `--token-ref`) for the kinds it names; the token goes into the variable `token_env` names, redacted in output.
- A Claude block's `extra_args` combine with other raw args, placed before them (later wins), instead of being refused.
- A leading `~` is expanded in the options agentio marks as paths (profile block, `--option`, and after their flags in profile and agent default args); everything else, and args after `--`, stays as written.
- A searched folder is a home only when it holds a file distinctive of the agent: `settings.json` alone no longer makes a folder a Claude, Gemini CLI and pi home at once.
- An agent's refusal of its own options (`--resume and --continue are exclusive`) is a usage error, not `io`.
- `agx credential` (the live-login key helper Claude Code runs) fails with exactly one stderr line, the line Claude shows in `/status`: a hint is folded into it (`(try: agx login check --id NAME --no-probe)`), and it names the login and the reference but never what a secret store or a command printed on stderr. It still never prints an empty value: an empty, whitespace-only, multi-line or unresolvable token exits non-zero with nothing on stdout, because Claude Code stops the session after three failed tries with no fallback to the home's own login.
- A missing gopass entry reads `gopass:REF is not in the gopass store` instead of the plugin's nested error text.
- The live-login guide says when a switch applies (the next key refresh, or at once after a 401; the first request after a timed refresh may still use the old key), where agx sets the refresh interval and why, and that a failing helper stops the session.

### Fixed

- `--resolved-secrets` without `--dry-run` was silently ignored; it is now refused (exit 125) and nothing starts.
- `--field data.*.KEY` on an empty list accepted any key; the path is now checked against the fields the command declares.
- A profile settings type error now names the key's file, line and column (`config.jsonc:32:86: profile typo settings cleanupPeriodDays: expected int`).
- Agent errors no longer carry a doubled agent prefix (`claude: claude: …`).
- `agx resume` on a config-defined agent searched nothing and said "not found"; it now refuses up front with the agent's reason (`mycli cannot resume conversations: …`, exit 125).
- `agx config init` on an existing file names it in the `~` form.
- `agx login add --provider nope` answered as if no provider was given; it now says the provider is unknown and lists the known ones.
- The attribution gate no longer trips on pi's real `github-copilot` provider name.
- A Codex dry run with `codex_profile` no longer carries the note `--profile conflict with a legacy profile not checked (no config given)`: the home's `config.toml` is checked.

### Documentation

- A learning path and a "start from your goal" table at the top of `docs/README.md`, and a feature index (`docs/reference/features.md`) that points every command, flag, config key, environment variable and exit code at the guide section and the examples that show it.
- Sixteen task guides under `docs/guides/` (a second account, auto-pick, plan usage, live login, folder bindings, shared homes, run folders, autonomy, raw args, headless runs, conversations, session folders, agents, a custom agent, doctor, scripting), and the existing guides and command reference rewritten to one shape: what and when, before you start, steps, real output, pitfalls, related links.
- Nine decision guides under `docs/choosing/`: which login kind, a second account, where to set the profile and login, which way to pass something to an agent, settings as a file or inline, usage live or cached (Claude Code 2.1.323 and later no longer write a cached reading), continuing on another account, built-in or config-defined agent, and the shell layer.
- 52 runnable example folders under `examples/`, each a scripted session run by the test suite on a fake machine; docs show their output through snippet markers that a gate keeps equal to the files, so every output in the docs is what agx prints.
- An architecture diagram (`docs/assets/architecture.svg`).
- What is redacted, colour, the repeated-flag refusal, `--resolved-secrets` without `--dry-run`, `--field` on an empty list, settings errors with line and column, headless keys together, `--option settings`, resume of a config-defined agent, `--limit 0`, a shell layer made with `--config`, alias checks at load, pi's `--provider` refusals, token logins for a config-defined agent, `extra_args` with raw args, `~` in path options and home markers for searched folders, each with an example session or a troubleshooting entry.

### Removed

- Everything specific to v0.3: positional arguments, `-p auto` and `--login home`, profile discovery at run time, `config.yaml` and `tokens.yaml`, and the v0.3 JSON envelope.
