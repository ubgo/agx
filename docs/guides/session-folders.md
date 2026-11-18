# Session folders: scratch work with `agx new`

Not every question to an agent belongs to a project. `agx new` creates a fresh, timestamped folder and starts an agent in it, so a quick experiment never lands in your home folder or in the last project you had open. Later, `agx session list` shows those folders, `agx session clean` removes the ones you never used, and `agx session promote` turns the one that grew into something into a real project, with its conversation history.

You want this when you start "just a quick test" several times a day, when you try a library before deciding to use it, or when an agent should work somewhere it cannot touch anything else.

## Before you start

- A config with at least one profile (`agx config init --discovered` writes one). `agx new` resolves the agent, profile and login like `agx run`.
- Session folders live in `<config dir>/sessions` (`~/.agx/sessions`, or under `AGX_CONFIG_DIR` when set) unless `sessions.root` says otherwise. `sessions.promote_root` is the default parent folder for `agx session promote`. The first example sets only the latter ([`examples/session-new-promote/config.jsonc`](../../examples/session-new-promote/config.jsonc)):

<!-- snippet: examples/session-new-promote/config.jsonc#sessions -->
```jsonc
  "sessions": {
    "promote_root": "~/projects"
  }
```

- A folder's name is the slug of `--name` (lowercase, every run of other characters becomes `_`) plus the local time: `--name "pglite golang"` makes `pglite_golang_20261114_101500`. Without `--name` it is the time alone, `20261114_101500`.
- With the [shell layer](../shell.md) loaded, your shell also `cd`s into the new folder before the agent starts, and is still there when the agent exits.

## Step 1: preview the folder with `--dry-run`

`--dry-run` shows every decision, the folder included, and creates nothing:

<!-- snippet: examples/session-new-promote/session.txt#new-dry-run -->
```sh
$ agx new --name "pglite golang" --dry-run --quiet
agent    claude                                         from default_agent
profile  personal                                       from default profile of claude
home     ~/.claude                                      from profile personal
login    home login                                     from default (no login named)
binary   /usr/local/bin/claude version unknown          from no install metadata
folder   ~/.agx/sessions/pglite_golang_20261118_061109  from new session folder
run      ~/.agx/runs/20261118-061109-uw4u               from AGX_RUN_ID=20261118-061109-uw4u
command  claude                                         from built by agentio
env      set AGX_RUN_ID=20261118-061109-uw4u            from launch environment
         set AGX_RUN_AGENT=claude                       from launch environment
         set AGX_RUN_PROFILE=personal                   from launch environment
         set AGX_RUN_LOGIN=home                         from launch environment
         set AGX_RUN_HOME=~/.claude                     from launch environment
note     installed Claude Code version unknown; flags not checked
```

`folder … from new session folder` is the folder `agx new` would create. Every `agx run` flag works the same way (`--profile`, `--login`, `--model`, `--prompt`, `--autonomy`, `--env`, `--` raw args, …) except `--dir`: the folder is always the new one. See [`agx new`](../commands/launch.md#agx-new) for the list.

## Step 2: start an agent in a new folder

Without `--dry-run`, agx creates the folder, then replaces itself with the agent started in it. The example's fake `claude` prints the folder it started in:

<!-- snippet: examples/session-new-promote/session.txt#new -->
```sh
$ agx new --name "pglite golang"
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv
pwd ~/.agx/sessions/pglite_golang_20261118_061109
...
```

## Step 3: list the session folders

`agx session list` prints every folder under the root, newest first, with its file count, the agents that hold conversation history for it (in any home, configured or not), when it last changed and its path. The example picks the columns that do not change between runs with `--field`:

<!-- snippet: examples/session-new-promote/session.txt#list -->
```sh
$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                           FILES  HISTORY  PATH
pglite_golang_20261118_061241  0      —        ~/.agx/sessions/pglite_golang_20261118_061241
pglite_golang_20261114_101500  1      —        ~/.agx/sessions/pglite_golang_20261114_101500
```

`HISTORY` is `—` here because the fake agent writes no transcript; after a real session it names the agent, for example `claude`.

## Step 4: remove the folders you never used

`agx session clean` removes folders that are empty and have no history in any home, and only when they are older than `--min-age` (default `24h`, so a folder made today, which may be about to get its first file, is kept). Without `--yes` it only prints the plan. The second example keeps its sessions in `~/scratch` ([`examples/session-clean/config.jsonc`](../../examples/session-clean/config.jsonc)):

<!-- snippet: examples/session-clean/config.jsonc#sessions -->
```jsonc
  "sessions": {
    "root": "~/scratch"
  }
```

It starts with one folder that has a file, then makes an empty one (with the plan-file step of the shell layer, which creates the folder without starting an agent; a session you open and close without writing anything leaves the same):

<!-- snippet: examples/session-clean/session.txt#make-empty -->
```sh
$ agx new --name "quick test" --plan-file ~/plan.json
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked

$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                        FILES  HISTORY  PATH
quick_test_20261118_061241  0      —        ~/scratch/quick_test_20261118_061241
notes_20261102_090000       1      —        ~/scratch/notes_20261102_090000
```

The default plan keeps both: one has files, the other is too new:

<!-- snippet: examples/session-clean/session.txt#plan -->
```sh
$ agx session clean
keep  quick_test_20261118_061119  newer than --min-age
keep  notes_20261102_090000       has files
plan only: 0 folder(s) to remove in ~/scratch; add --yes to remove them
```

`--min-age 0s` makes every empty folder eligible, and `--yes` removes it:

<!-- snippet: examples/session-clean/session.txt#min-age -->
```sh
$ agx session clean --min-age 0s
remove  quick_test_20261118_061119  empty, no history
keep    notes_20261102_090000       has files
plan only: 1 folder(s) to remove in ~/scratch; add --yes to remove them

$ agx session clean --min-age 0s --yes
removed  quick_test_20261118_061119  empty, no history
keep     notes_20261102_090000       has files

$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                   FILES  HISTORY  PATH
notes_20261102_090000  1      —        ~/scratch/notes_20261102_090000
```

Each row says why: `empty, no history` (removed with `--yes`), `has files`, `has conversation history`, `history unknown` (a home could not be read, so agx keeps the folder rather than guess) or `newer than --min-age`. A `.DS_Store` does not count as a file. The removal itself only deletes empty folders, so a file added between the plan and `--yes` keeps that folder: its row reads `failed` and the command exits 3.

## Step 5: promote a session folder into a project

When a scratch folder turns into real work, `agx session promote` moves it under a projects folder with one rename and re-keys its conversation history in every home whose agent can move history (Claude Code), so `agx resume` and `claude --resume` find the chats in the new place. Preview it first:

<!-- snippet: examples/session-new-promote/session.txt#promote-plan -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --dry-run
would promote ~/.agx/sessions/pglite_golang_20261114_101500 to ~/projects/pglite_golang
dry run: nothing changed
```

The destination is `<--to or sessions.promote_root>/<--name or the session's slug>`. `--name` picks another folder name and `--git-repo` runs `git init` in the promoted folder:

<!-- snippet: examples/session-new-promote/session.txt#promote -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --name pglite-go --git-repo
promoted ~/.agx/sessions/pglite_golang_20261114_101500 to ~/projects/pglite-go
  git init in ~/projects/pglite-go

$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                           FILES  HISTORY  PATH
pglite_golang_20261118_061241  0      —        ~/.agx/sessions/pglite_golang_20261118_061241
```

`--to` overrides `sessions.promote_root` for one promote:

<!-- snippet: examples/session-new-promote/to.session.txt#to -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --to ~/work --dry-run
would promote ~/.agx/sessions/pglite_golang_20261114_101500 to ~/work/pglite_golang
dry run: nothing changed
```

When the folder has Claude history, the plan adds one line per home, `claude history in ~/.claude moves with it`, and the real promote adds the backup path agentio wrote before it changed anything (`backup: …`). Codex keeps absolute folder paths in its own index, so its history cannot follow the folder: a folder with Codex chats is refused with exit 4, `this folder has 2 Codex conversations; add --no-codex`. `--no-codex` promotes anyway and warns that those chats stay keyed to the old path, where only a folder at that path can resume them. (The examples cannot show history moving: a transcript records the absolute folder path, which differs on every test machine; `internal/cli/session_test.go` covers both cases.)

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/session-new-promote$'
go test -count=1 ./internal/docscheck/ -run 'TestExamples/session-clean$'
```

On your own machine: `agx new --name "try it" --dry-run`, then `agx new --name "try it"`, exit the agent, and `agx session list`.

## Pitfalls

Refusals of `promote` and `clean`:

<!-- snippet: examples/session-new-promote/session.txt#promote-refusals -->
```sh
$ agx session promote --id pglite_golang_20261114_101500
agx: no session folder pglite_golang_20261114_101500 in ~/.agx/sessions
  agx session list
[exit 1]

$ agx session promote --id ~/projects/app
agx: --id "~/projects/app": give a session folder's name, not a path
[exit 2]

$ agx session promote
agx: --id is required: the session folder's name (agx session list)
[exit 2]
```

<!-- snippet: examples/session-new-promote/to.session.txt#exists -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --name app
agx: promote: destination already exists: ~/projects/app
[exit 1]
```

<!-- snippet: examples/session-clean/session.txt#refusals -->
```sh
$ agx session clean --min-age -1h
agx: --min-age must not be negative
[exit 2]

$ agx session clean --min-age soon
agx: invalid argument "soon" for "--min-age" flag: must be a duration like "30s" or "5m"
[exit 2]

$ agx session promote --id notes_20261102_090000
agx: --to is required: the folder to move the session into (or set sessions.promote_root)
  agx config set --key sessions.promote_root --value ~/projects
[exit 2]
```

- `--id` is a folder name from `agx session list`, never a path; `--name` is a folder name too.
- A session made without `--name` has no slug, so promote needs `--name` (`<id> has no name before its timestamp; add --name`, exit 2).
- Without `--to` and without `sessions.promote_root`, promote is refused with the `agx config set` line that fixes it.
- Promote never overwrites: an existing destination is refused (exit 1) and nothing moves. It also refuses while an agent runs in the folder (`… is running in … (pid N); exit it first, nothing changed`), and when the destination already has history in a home. A failure after the folder moved puts the folder and every moved history back.
- Promote moves within one disk only: a destination on another disk is refused (`destination is on another disk; promote only moves within one disk`).
- `clean` checks history in every home agx can find, including homes that no profile uses, so a folder whose only chats live in an unconfigured home is never removed.
- `agx new --dry-run` creates nothing, so the folder it shows does not exist yet; the real run gets a new timestamp.

## Related

- [`agx session` reference](../commands/conversation.md#agx-session-list) and [`agx new`](../commands/launch.md#agx-new)
- [Conversations](conversations.md): list, resume and move chats between accounts
- [Shell layer](../shell.md): `cd` into the new folder, and the plan-file step used above
- [`sessions` config keys](../config.md)
- Examples: [session-new-promote](../../examples/session-new-promote/README.md), [session-clean](../../examples/session-clean/README.md)
