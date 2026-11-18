# Choose: a second account

Three ways to add a second Claude account, each tried with `--dry-run` or as a dry launch: a new profile with its own home (shared files linked, or `--bare`), a second profile on the same home billed per token, and a named login used on the existing home. The last region shows that the default home runs with `CLAUDE_CONFIG_DIR` absent.

Read it with [A second account](../../docs/choosing/second-account.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-second-account$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `profile`, `bare`, `shared`, `login`, `default-home`. |
| [`config.jsonc`](config.jsonc) | One profile on the default home. |
| [`home/.claude/`](home/.claude/) | Fake shared files (`CLAUDE.md`, `settings.json`, `skills/`) that `profile add` links into a new home. |

## Related

- Decision guide: [A second account](../../docs/choosing/second-account.md).
- Guide: [Logins](../../docs/logins.md), [acct-second-account](../acct-second-account/README.md) (the full setup).
- All examples: [examples](../README.md).
