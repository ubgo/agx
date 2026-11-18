# Start a scratch session, then promote it

`agx new --name "pglite golang"` previews and then creates a timestamped session folder and starts the agent in it; `agx session list` shows the folders; `agx session promote` turns an older session folder into a project under `sessions.promote_root`, with `--name`, `--git-repo`, `--to` and `--dry-run`. Use it for experiments that may or may not become projects.

The fake machine starts with one session folder, `~/.agx/sessions/pglite_golang_20261114_101500`, holding `main.go` (from `home/`).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/session-new-promote$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One Claude profile and `sessions.promote_root`. Region: `sessions`. |
| `home/.agx/sessions/pglite_golang_20261114_101500/main.go` | The earlier session's one file. |
| [`session.txt`](session.txt) | Regions: `new-dry-run`, `new`, `list`, `promote-plan`, `promote` (`--name`, `--git-repo`), `promote-refusals` (promoted already, a path as `--id`, no `--id`). |
| [`to.session.txt`](to.session.txt) | Regions: `to` (`--to` over `promote_root`), `exists` (the destination exists), `no-codex` (`--no-codex` on a folder without Codex history: the same plan). |

Guide: [Session folders](../../docs/guides/session-folders.md). Reference: [`agx session promote`](../../docs/commands/conversation.md#agx-session-promote), [`agx new`](../../docs/commands/launch.md#agx-new).
