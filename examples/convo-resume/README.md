# Resume a conversation

Continue a conversation on the home that stores it, in its own folder: the newest with `--last`, one by id prefix with `--id`, narrowed with `--query`, across folders with `--scope all`, capped with `--limit`, and on another login's quota with `--login carol`. The `start` region runs the fake `claude` and shows the `--resume` flag and the folder it got. Use it to pick up yesterday's chat, or to continue a chat while its own account is out of quota.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-resume$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `personal` and `work` (Claude) and a stored login `carol`. Regions: `profiles`, `logins`. |
| [`session.txt`](session.txt) | Run in `~/projects/client`. Regions: `last`, `id`, `query`, `login` (`--login carol`), `start` (no `--dry-run`). |
| [`other-folder.session.txt`](other-folder.session.txt) | Run in `~/projects/app`. Regions: `here` (nothing in this folder, exit 125), `all` (`--scope all`). |
| [`refusals.session.txt`](refusals.session.txt) | Regions: `refusals` (no terminal for the picker, two choices of the chat, unknown id, unknown profile; all exit 125), `profile-narrows` (`--profile` limits the homes searched), `limit`. |

Guide: [Conversations, steps 4 and 5](../../docs/guides/conversations.md#step-4-resume-a-conversation). Reference: [`agx resume`](../../docs/commands/launch.md#agx-resume).
