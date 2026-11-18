# Conversations and session folders

Conversations are the chats an agent stores in its home. Session folders are the scratch folders `agx new` creates. Choosing a conversation to continue happens in [`agx resume`](launch.md#agx-resume); these commands list, inspect and move. The guides walk through each task: [conversations](../guides/conversations.md) and [session folders](../guides/session-folders.md).

## agx conversation list

Print conversations, newest first, from every configured home. Cost R. Each is one line, as agx v0.3's `resume --list` printed it: how long ago it changed, the agent, the profile, the title, the folder and the first 8 characters of the id (a prefix `--id` takes); `--by-folder` prints when, how many chats, the profiles and the folder. `--json` and `--field data.*.KEY` keep every full value.

| Flag | Meaning |
|---|---|
| `--query` | keep conversations whose title, folder or id holds this text (any case) |
| `--scope` | `folder` (default: this folder's) or `all` |
| `--by-folder` | one row per folder |
| `--limit` | conversations considered, newest first; default 50, `0` for all |
| `--agent` | only this agent's homes |
| `--profile` | only these profiles' homes (repeat) |

Try it ([example](../../examples/convo-list/README.md), run in `~/projects/client`):

<!-- snippet: examples/convo-list/session.txt#list -->
```sh
$ agx conversation list
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
8 d ago      claude personal   Fix the parser                   ~/projects/client             3f2a9c1e
```

From any folder, `--scope all` lists every folder's conversations, and `--by-folder` groups them:

<!-- snippet: examples/convo-list/all.session.txt#all -->
```sh
$ agx conversation list --scope all
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
8 d ago      claude personal   Fix the parser                   ~/projects/client             3f2a9c1e

$ agx conversation list --scope all --by-folder
5 d ago      2 chats   work,personal        ~/projects/client
```

```sh
agx conversation list --scope all --query parser
agx conversation list --profile work --profile personal
agx conversation list --json --field data.*.id
```

All four built-in agents list their conversations. A warning names each home on disk that no profile uses (`claude home ~/.claude-old is not in config; agx profile import --discovered`); it is never searched.

Pitfalls: `--scope` other than `folder` or `all` is exit 2. `--agent` or `--profile` of an agent that cannot list conversations is exit 4 with its reason. A home that cannot be read is a warning; the rows of the others still print and the command exits 3.

## agx conversation show

One conversation: home, folder, title, turns, size, last activity, and whether an agent is running it now. Cost R.

| Flag | Meaning |
|---|---|
| `--id` | the conversation id or a unique prefix |

Try it:

<!-- snippet: examples/convo-list/session.txt#show -->
```sh
$ agx conversation show --id 3f2a
id        3f2a9c1e-0000-4000-8000-000000000001
title     Fix the parser
agent     claude
profiles  personal
home      ~/.claude
folder    ~/projects/client
turns     2
...
created   2026-11-10 10:00
updated   2026-11-10 10:00
live      no
path      ~/.claude/projects/-Users-you-projects-client/3f2a9c1e-0000-4000-8000-000000000001.jsonl
```

`turns` and `live` read `unknown (…)` when the agent cannot report them. Pitfall: an id that matches nothing is exit 1 with the hint `agx conversation list --scope all`.

## agx conversation move

Move a folder's conversations to another home of the same agent, for example from the personal account to the work account. Claude only; Codex keeps an index of absolute paths, so its conversations are never moved. Cost W.

| Flag | Meaning |
|---|---|
| `--dir` | the folder whose conversations move; default the current folder |
| `--to-profile` | the profile whose home receives them (required) |
| `--from-profile` | the profile whose home holds them; default the one other home with history for the folder |
| `--id` | only this conversation, exact or a unique prefix (repeat) |
| `--picker` | `chat` (tick conversations) or `folder` (choose a folder, then tick); needs a terminal |
| `--scope` | `folder` (default: the current folder) or `all` (every folder of the source home) |
| `--no-backup` | discard the backup after a successful move (by default a backup is kept) |
| `--dry-run` | print the plan; change nothing |

Try it ([example](../../examples/convo-move-to-work/README.md)):

<!-- snippet: examples/convo-move-to-work/session.txt#plan -->
```sh
$ agx conversation move --to-profile work --dry-run
would move 1 claude conversation(s) of ~/projects/client from ~/.claude to ~/.claude-work
  3f2a9c1e-0000-4000-8000-000000000001
dry run: nothing changed
```

<!-- snippet: examples/convo-move-to-work/session.txt#move -->
```sh
$ agx conversation move --to-profile work
moved 1 claude conversation(s) of ~/projects/client from ~/.claude to ~/.claude-work
  3f2a9c1e-0000-4000-8000-000000000001
...

$ agx conversation list
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
8 d ago      claude work       Fix the parser                   ~/projects/client             3f2a9c1e
```

```sh
agx conversation move --dir ~/projects/client --to-profile work --from-profile personal
agx conversation move --to-profile work --picker chat
```

Pitfalls:

<!-- snippet: examples/convo-move-to-work/refusals.session.txt -->
```sh
#options dir=~/projects/client
$ agx conversation move --dry-run
agx: --to-profile is required: the profile whose home receives the conversations
[exit 2]

$ agx conversation move --to-profile codex
agx: codex cannot move conversations between homes: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
[exit 4]

$ agx conversation move --to-profile personal --from-profile personal
agx: --from-profile personal and --to-profile personal share the home ~/.claude; nothing to move
[exit 2]

$ agx conversation move --to-profile work --picker chat
agx: no terminal for the picker; pass --id (repeat) to name the conversations
[exit 2]

$ agx conversation move --to-profile work --id 0000
agx: not found: conversation "0000" for ~/projects/client
[exit 1]
```

Also refused: `--id` with `--picker`, `--dir` with `--picker folder` or `--scope all` (exit 2); two other homes with history for the folder (`add --from-profile`); a running conversation or an id already in the target (`move blocked, nothing changed: …`).

## agx session list

Session folders made by `agx new`, newest first, with their files and conversation history (from every home agx finds, configured or not). Cost R. No flags of its own; `--field` picks columns. The root and the folder count go to stderr; the table shows `FOLDER`, `LAST ACTIVITY` ("3 min ago"), `FILES` and `HISTORY`, and `--json` keeps the full paths and times.

Try it ([example](../../examples/session-new-promote/README.md)):

<!-- snippet: examples/session-new-promote/session.txt#list -->
```sh
$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                           FILES  HISTORY  PATH
pglite_golang_20261118_061241  0      —        ~/.agx/sessions/pglite_golang_20261118_061241
pglite_golang_20261114_101500  1      —        ~/.agx/sessions/pglite_golang_20261114_101500
```

The folders live in `sessions.root`, default `~/.agx/sessions` (the config folder).

## agx session clean

Remove session folders that are empty and have no conversation history. Shows a plan unless `--yes`. Cost W.

| Flag | Meaning |
|---|---|
| `--min-age` | keep folders younger than this; default `24h` |
| `--yes` | remove the planned folders; without it agx only shows the plan |

Try it ([example](../../examples/session-clean/README.md), sessions in `~/scratch`):

<!-- snippet: examples/session-clean/session.txt#plan -->
```sh
$ agx session clean
keep  quick_test_20261118_061119  newer than --min-age
keep  notes_20261102_090000       has files
plan only: 0 folder(s) to remove in ~/scratch; add --yes to remove them
```

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

Pitfalls: a negative or malformed `--min-age` is exit 2. A folder with a file (other than `.DS_Store`), with history, or whose history cannot be read is kept. Removal only deletes empty folders, so a file added after the plan makes that row `failed` and the command exit 3.

## agx session promote

Turn a session folder into a project: move it, with its Claude history, under a projects folder. Cost W.

| Flag | Meaning |
|---|---|
| `--id` | the session folder's name (see `agx session list`) |
| `--name` | the project folder's name; default the session's slug |
| `--to` | the parent folder; default `sessions.promote_root` |
| `--git-repo` | run `git init` in the promoted folder |
| `--no-codex` | promote even though some history (Codex) cannot move with the folder; it stays behind |
| `--dry-run` | print the plan; change nothing |

Try it ([example](../../examples/session-new-promote/README.md), with `sessions.promote_root` set to `~/projects`):

<!-- snippet: examples/session-new-promote/session.txt#promote-plan -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --dry-run
would promote ~/.agx/sessions/pglite_golang_20261114_101500 to ~/projects/pglite_golang
dry run: nothing changed
```

<!-- snippet: examples/session-new-promote/session.txt#promote -->
```sh
$ agx session promote --id pglite_golang_20261114_101500 --name pglite-go --git-repo
promoted ~/.agx/sessions/pglite_golang_20261114_101500 to ~/projects/pglite-go
  git init in ~/projects/pglite-go

$ agx session list --field data.*.name --field data.*.files --field data.*.history --field data.*.path
NAME                           FILES  HISTORY  PATH
pglite_golang_20261118_061241  0      —        ~/.agx/sessions/pglite_golang_20261118_061241
```

A folder with Codex conversations is refused (exit 4) unless `--no-codex`, because Codex conversations cannot follow the folder: `this folder has 2 Codex conversations; add --no-codex`.

Pitfalls:

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

Also refused: no `--to` and no `sessions.promote_root` (exit 2, with the `agx config set` line), an existing destination (exit 1), a destination on another disk, and an agent running in the folder.
