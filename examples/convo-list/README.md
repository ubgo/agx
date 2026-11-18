# List and inspect conversations

List the conversations of every configured home, newest first: this folder's, every folder's (`--scope all`), grouped by folder (`--by-folder`), narrowed by `--profile`, `--agent`, `--query` and `--limit`, as JSON with `--field`; then show one with `conversation show --id` and a prefix. Use it when you forgot which account a chat ran on.

The fake machine holds two Claude conversations in `~/projects/client`: `3f2a9c1e…` in `~/.claude` (profile `personal`) and `7b1d4e20…` in `~/.claude-work` (profile `work`).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/convo-list$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Two Claude profiles on two homes and one Codex profile. Region: `profiles`. |
| [`session.txt`](session.txt) | Run in `~/projects/client`. Regions: `list`, `show`, `narrow` (`--profile`, `--query`, `--agent`, `--limit`), `json`. |
| [`all.session.txt`](all.session.txt) | Run in `~/projects/app` (no chats). Regions: `here` (empty), `all` (`--scope all`, `--by-folder`), `errors` (unknown id, bad `--scope`). |

Guide: [Conversations, steps 1 to 3](../../docs/guides/conversations.md#step-1-list-this-folders-conversations). Reference: [`agx conversation list`](../../docs/commands/conversation.md#agx-conversation-list).
