# Choose: usage live, cached or probed

`agx usage --cached` on four homes: a Claude home with no reading (Claude Code 2.1.323 and later no longer write one), a Claude home with an old reading (reported `stale`), a Codex home with no session yet, and Gemini CLI (no usage). Also live usage for an agent without usage, and the refusal of `--cached` with login probes. Live usage itself needs the network, so it is not run here.

Read it with [Usage: live, cached or probed](../../docs/choosing/usage-live-or-cached.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-usage$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `cached`, `stale`, `unsupported`, `conflict`. |
| [`config.jsonc`](config.jsonc) | Two Claude profiles, a Codex and a Gemini CLI profile. |
| [`home/.claude-work/.claude.json`](home/.claude-work/.claude.json) | An old cached reading of the `work` home (fake). |

## Related

- Decision guide: [Usage: live, cached or probed](../../docs/choosing/usage-live-or-cached.md).
- Guide: [Usage reference](../../docs/commands/usage.md).
- All examples: [examples](../README.md).
