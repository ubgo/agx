# Move a folder's conversations to another account

Move the Claude conversations of `~/projects/client` from the personal home (`~/.claude`) to the work home (`~/.claude-work`): preview with `--dry-run`, move (agentio keeps a backup), and check with `conversation list`. A second session names everything a script would (`--dir`, `--from-profile`, `--id`, `--no-backup`) and previews `--scope all`; a third shows the refusals. Use it when a project changes accounts for good.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-move-to-work$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `personal` and `work` (Claude) and `codex`. Region: `profiles`. |
| [`session.txt`](session.txt) | Run in `~/projects/client`. Regions: `plan` (`--dry-run`), `move` (the move, then the list). The backup line holds the time, so the session shows it as `...`. |
| [`options.session.txt`](options.session.txt) | Regions: `explicit` (`--dir`, `--from-profile`, `--id`, `--no-backup`), `all` (`--scope all --dry-run`). |
| [`refusals.session.txt`](refusals.session.txt) | Region `refusals`: no `--to-profile` (2), Codex (4), same home (2), `--picker` without a terminal (2), unknown id (1). |

Guide: [Conversations, step 6](../../docs/guides/conversations.md#step-6-move-a-folders-conversations-to-another-account). Reference: [`agx conversation move`](../../docs/commands/conversation.md#agx-conversation-move).
