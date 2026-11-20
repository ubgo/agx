# Conversations: find, resume and move

Every agent keeps its conversations (chats) in its home: Claude Code under `~/.claude/projects`, Codex under `~/.codex/sessions`, and so on. With two accounts you have two homes, so a folder's history is split between them. agx reads every configured home at once: `agx conversation list` shows what exists, `agx conversation show` shows one, `agx resume` continues one on the home that stores it, and `agx conversation move` moves a folder's chats from one account's home to another's.

You want this when you forgot which account a chat ran on, when you want to continue a chat while the account it ran on is out of quota, or when a project changes accounts for good (a personal side project becomes client work).

## Before you start

- A config with a profile per home. Every example below uses this one ([`examples/convo-list/config.jsonc`](../../examples/convo-list/config.jsonc)):

<!-- snippet: examples/convo-list/config.jsonc#profiles -->
```jsonc
  "profiles": {
    "personal": { "agent": "claude", "home": "~/.claude", "default": true },
    "work": { "agent": "claude", "home": "~/.claude-work" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  }
```

- Only configured homes are read. A home on disk that no profile uses is named in a warning (`claude home ~/.claude-old is not in config; agx profile import --discovered`), never searched.
- Every command here except `move` and `resume` only reads (cost R). Transcripts are read at their head and tail, never whole.
- The examples run in `~/projects/client`, which has one chat in each Claude home: `3f2a9c1e…` "Fix the parser" in `~/.claude` (profile `personal`) and `7b1d4e20…` "Review the API" in `~/.claude-work` (profile `work`).

## Step 1: list this folder's conversations

`agx conversation list` prints the current folder's conversations from every configured home, newest first. The `PROFILES` column names every profile on the home that stores the chat, so you see which account it belongs to.

<!-- snippet: examples/convo-list/session.txt#list -->
```sh
$ agx conversation list
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
8 d ago      claude personal   Fix the parser                   ~/projects/client             3f2a9c1e
```

All five built-in agents (Claude Code, Codex, Gemini CLI, pi, opencode) list their conversations; for opencode that is the sessions from before opencode 1.2.0 (newer ones live in its `opencode.db`, which agx does not read yet), and `agx resume --id` works for any session id. An agent that cannot is skipped quietly, or refused with its reason (exit 4) when you asked for it with `--agent` or `--profile`.

## Step 2: look across folders, narrow and script

From a folder with no chats the list is empty. `--scope all` lists every folder's chats, and `--by-folder` turns that into one row per folder, which answers "where did I work last week":

<!-- snippet: examples/convo-list/all.session.txt#all -->
```sh
$ agx conversation list --scope all
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
8 d ago      claude personal   Fix the parser                   ~/projects/client             3f2a9c1e

$ agx conversation list --scope all --by-folder
5 d ago      2 chats   work,personal        ~/projects/client
```

Narrow with `--profile` (repeat it for several), `--agent`, `--query` (a case-insensitive piece of the title, folder or id) and `--limit` (how many conversations, newest first; default 50, `0` for all). With `--query`, agx reads every conversation and applies `--limit` after the filter, so an old match is still found:

<!-- snippet: examples/convo-list/session.txt#narrow -->
```sh
$ agx conversation list --profile work
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20

$ agx conversation list --query parser
8 d ago      claude personal   Fix the parser                   ~/projects/client             3f2a9c1e

$ agx conversation list --agent codex

$ agx conversation list --limit 1
5 d ago      claude work       Review the API                   ~/projects/client             7b1d4e20
```

For scripts, `--json` with `--field` prints just the values:

<!-- snippet: examples/convo-list/session.txt#json -->
```sh
$ agx conversation list --json --field data.*.id
["7b1d4e20-0000-4000-8000-000000000002","3f2a9c1e-0000-4000-8000-000000000001"]
```

## Step 3: inspect one conversation

`agx conversation show --id` takes the full id or a unique prefix. It prints the home and profiles, the folder, the number of turns, the transcript size, the times, whether an agent is running it right now (`live`), and the transcript path:

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

`turns` and `live` say `unknown (…)` for an agent that cannot report them.

## Step 4: resume a conversation

`agx resume` continues a chat on the home that stores it, in the folder it ran in, whatever folder you are in now. You never name the profile or the home: the conversation decides them, and `--dry-run` shows that as `from conversation <id>`.

Choose the chat one of three ways:

| You type | agx picks |
|---|---|
| `agx resume` | a picker of this folder's chats, newest first (needs a terminal) |
| `agx resume --last` | the newest chat in this folder, no picker |
| `agx resume --id 3f2a` | that chat, by full id or unique prefix, from any folder |

<!-- snippet: examples/convo-resume/session.txt#last -->
```sh
$ agx resume --last --dry-run
agx: profile work from home ~/.claude-work
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 7b1d4e20-0000-4000-8000-000000000002
profile  work                                                  from home ~/.claude-work
home     ~/.claude-work                                        from conversation 7b1d4e20-0000-4000-8000-000000000002 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 7b1d4e20-0000-4000-8000-000000000002
run      ~/.agx/runs/20261118-061044-tvqg                      from AGX_RUN_ID=20261118-061044-tvqg
command  claude --resume 7b1d4e20-0000-4000-8000-000000000002  from built by agentio
env      set AGX_RUN_ID=20261118-061044-tvqg                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=work                              from launch environment
         set AGX_RUN_LOGIN=home                                from launch environment
         set AGX_RUN_HOME=~/.claude-work                       from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work                  from launch environment
note     installed Claude Code version unknown; flags not checked
```

<!-- snippet: examples/convo-resume/session.txt#id -->
```sh
$ agx resume --id 3f2a --dry-run
agx: profile personal from home ~/.claude (default)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (default)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
run      ~/.agx/runs/20261118-061045-2qrs                      from AGX_RUN_ID=20261118-061045-2qrs
command  claude --resume 3f2a9c1e-0000-4000-8000-000000000001  from built by agentio
env      set AGX_RUN_ID=20261118-061045-2qrs                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=personal                          from launch environment
         set AGX_RUN_LOGIN=home                                from launch environment
         set AGX_RUN_HOME=~/.claude                            from launch environment
note     installed Claude Code version unknown; flags not checked
```

`--query` narrows the picker, or `--last`, to chats whose title, folder or id holds the text:

<!-- snippet: examples/convo-resume/session.txt#query -->
```sh
$ agx resume --last --query parser --dry-run --quiet
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (default)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
run      ~/.agx/runs/20261118-061045-t11p                      from AGX_RUN_ID=20261118-061045-t11p
command  claude --resume 3f2a9c1e-0000-4000-8000-000000000001  from built by agentio
env      set AGX_RUN_ID=20261118-061045-t11p                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=personal                          from launch environment
         set AGX_RUN_LOGIN=home                                from launch environment
         set AGX_RUN_HOME=~/.claude                            from launch environment
note     installed Claude Code version unknown; flags not checked
```

`--scope all` widens the picker and `--last` from this folder to every folder; `--picker folder` asks for a folder first, then a chat in it (it always offers every folder); `--limit` caps how many chats the picker considers, newest first (default 50, `0` for all, as in `conversation list`). From `~/projects/app`, which has no chats:

<!-- snippet: examples/convo-resume/other-folder.session.txt -->
```sh
$ agx resume --last --dry-run
agx: no conversations in ~/projects/app
  agx resume --scope all
[exit 125]

$ agx resume --last --scope all --dry-run --quiet
agent    claude                                                from conversation 7b1d4e20-0000-4000-8000-000000000002
profile  work                                                  from home ~/.claude-work
home     ~/.claude-work                                        from conversation 7b1d4e20-0000-4000-8000-000000000002 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 7b1d4e20-0000-4000-8000-000000000002
run      ~/.agx/runs/20261118-061045-eehv                      from AGX_RUN_ID=20261118-061045-eehv
command  claude --resume 7b1d4e20-0000-4000-8000-000000000002  from built by agentio
env      set AGX_RUN_ID=20261118-061045-eehv                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=work                              from launch environment
         set AGX_RUN_LOGIN=home                                from launch environment
         set AGX_RUN_HOME=~/.claude-work                       from launch environment
         set CLAUDE_CONFIG_DIR=~/.claude-work                  from launch environment
note     installed Claude Code version unknown; flags not checked
```

Without `--dry-run` agx replaces itself with the agent. The fake `claude` of the example prints what it got: the resume flag and id, started in the chat's own folder:

<!-- snippet: examples/convo-resume/session.txt#start -->
```sh
$ agx resume --id 3f2a
agx: profile personal from home ~/.claude (default)
agx: warning: installed Claude Code version unknown; flags not checked
agent claude
argv [--resume] [3f2a9c1e-0000-4000-8000-000000000001]
pwd ~/projects/client
...
```

`--profile` and `--agent` narrow which homes are searched; they do not move a chat to another home (use `conversation move` for that). With the [shell layer](../shell.md) loaded, your shell also `cd`s into the chat's folder before the agent starts.

## Step 5: resume on another account's quota

History and quota are separate. The history stays in the home that stores the chat; the login, and so the quota, is resolved as for any launch: `--login`, `AGX_LOGIN`, the folder binding of the chat's folder, the profile's `login`, and so on (see [how agx decides](../how-agx-decides.md)). So when the account a chat ran on is out of quota, resume it on another stored login:

<!-- snippet: examples/convo-resume/session.txt#login -->
```sh
$ agx resume --id 3f2a --login carol --dry-run
agx: profile personal from home ~/.claude (default)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (default)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    carol (not resolved: --dry-run)                       from --login
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
run      ~/.agx/runs/20261118-061045-tmpk                      from AGX_RUN_ID=20261118-061045-tmpk
command  claude --resume 3f2a9c1e-0000-4000-8000-000000000001  from built by agentio
env      set AGX_RUN_ID=20261118-061045-tmpk                   from launch environment
         set AGX_RUN_AGENT=claude                              from launch environment
         set AGX_RUN_PROFILE=personal                          from launch environment
         set AGX_RUN_LOGIN=carol                               from launch environment
         set AGX_RUN_HOME=~/.claude                            from launch environment
         set CLAUDE_CODE_OAUTH_TOKEN=[redacted]                from launch environment
note     installed Claude Code version unknown; flags not checked
```

The chat keeps its home (`~/.claude`) and history; the token of `carol` is injected for this run only. Every login flag of `agx run` works: `--home-login`, `--auto-login` (the stored login with the most headroom), `--token-ref`. See [logins](../logins.md) for adding `carol`.

## Step 6: move a folder's conversations to another account

When a project moves to another account for good, move its history too, so `agx resume` and the agent's own `--resume` find it on the new home. Claude Code only: Codex keeps an index of absolute paths in its own database, so its chats are refused (exit 4) and stay where they are.

Preview first. `--to-profile` names the receiving profile; the source is the one other home of the same agent with history for this folder, or `--from-profile`:

<!-- snippet: examples/convo-move-to-work/session.txt#plan -->
```sh
$ agx conversation move --to-profile work --dry-run
would move 1 claude conversation(s) of ~/projects/client from ~/.claude to ~/.claude-work
  3f2a9c1e-0000-4000-8000-000000000001
dry run: nothing changed
```

Then move. agentio backs up every file it touches before it changes anything and prints where the copy is: a line `backup: ~/.claude.agentio-bak-<UTC time>/projects/<folder key>/<id>.jsonl`, next to the source home (shown as `...` below, because it holds the time). The list afterwards shows both chats on `work`:

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

Name everything when you script it: `--dir` (another folder than the current one), `--from-profile`, `--id` (repeat for several; full id or unique prefix) and `--no-backup` to discard the backup once the move succeeded. `--scope all` moves every folder's chats of the source home:

<!-- snippet: examples/convo-move-to-work/options.session.txt -->
```sh
$ agx conversation move --dir ~/projects/client --from-profile personal --to-profile work --id 3f2a --no-backup
moved 1 claude conversation(s) of ~/projects/client from ~/.claude to ~/.claude-work
  3f2a9c1e-0000-4000-8000-000000000001

$ agx conversation move --scope all --from-profile work --to-profile personal --dry-run
would move 2 claude conversation(s) of ~/projects/client from ~/.claude-work to ~/.claude
  3f2a9c1e-0000-4000-8000-000000000001
  7b1d4e20-0000-4000-8000-000000000002
dry run: nothing changed
```

In a terminal, `--picker chat` lets you tick the chats of this folder, and `--picker folder` asks for a folder first, then the chats.

## Try it

Every command above runs in the test suite on a fake machine, from these example folders:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-list$'
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-resume$'
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-move-to-work$'
```

On your own machine, start read-only: `agx conversation list --scope all --by-folder`, then `agx resume --last --dry-run` in a project folder.

## Pitfalls

Refusals of `conversation` (exit 2 for a usage mistake, 1 for not found, 4 for an agent that cannot):

<!-- snippet: examples/convo-list/all.session.txt#errors -->
```sh
$ agx conversation show --id 0000
agx: no conversation 0000 in any configured home
  agx conversation list --scope all
[exit 1]

$ agx conversation list --scope everywhere
agx: --scope "everywhere": use folder or all
[exit 2]
```

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

Refusals of `resume` exit 125, like every launching command, so a script can tell "agx refused" from "the agent exited non-zero":

<!-- snippet: examples/convo-resume/refusals.session.txt#refusals -->
```sh
$ agx resume
agx: no terminal for the picker; pass --id or --last
[exit 125]

$ agx resume --id 3f2a --last
agx: --id and --last both choose the conversation; pick one
[exit 125]

$ agx resume --last --picker folder
agx: --last and --picker folder both choose the conversation; pick one
[exit 125]

$ agx resume --id 0000
agx: conversation 0000 not found in 2 configured home(s)
[exit 125]

$ agx resume --id 3f2a --profile codex
agx: unknown profile codex
  agx profile list
[exit 125]
```

- No terminal, no picker: pass `--id` or `--last` in scripts, CI and editors.
- Two choices of the chat in one command are refused: `--id` with `--last`, `--query` or `--picker folder`; `--last` with `--picker folder`. In `move`, `--id` with `--picker`, `--dir` with `--picker folder`, and `--dir` with `--scope all`.
- An id prefix that matches chats in two homes is refused (`… matches 2 conversations: …; add --profile`); within one home, give more of the id.
- A chat whose folder was deleted is refused with the hint `mkdir -p <folder>   (recreate it to resume there)`.
- `--profile` narrows the search to that profile's home. Asking for a chat that lives elsewhere is "not found"; drop `--profile` (or name the profile whose home holds it) to find it:

<!-- snippet: examples/convo-resume/refusals.session.txt#profile-narrows -->
```sh
$ agx resume --id 3f2a --profile work --dry-run
agx: conversation 3f2a not found in 1 configured home(s)
[exit 125]
```

- `move` stops before any file changes when one of the chats is running (`move blocked, nothing changed: …`) or when an id already exists in the target home. Exit the agent first.
- A config-defined agent has no conversation reader: `agx resume --agent mycli` is refused up front (`mycli cannot resume conversations: config-only agent: …`, exit 125), and `conversation list` names it as not searched. Pass the agent's own resume flags raw: `agx run --profile my -- --resume ID`.

## Related

- [`agx conversation` reference](../commands/conversation.md) and [`agx resume`](../commands/launch.md#agx-resume)
- [Session folders](session-folders.md): `agx new`, and moving a scratch folder with its history into a project
- [Shell layer](../shell.md): `cd` into the chat's folder on resume
- [Logins](../logins.md) and [how agx decides](../how-agx-decides.md) for the login chain of Step 5
- Examples: [convo-list](../../examples/convo-list/README.md), [convo-resume](../../examples/convo-resume/README.md), [convo-move-to-work](../../examples/convo-move-to-work/README.md)
