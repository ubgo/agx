# Choose: continue on another account

The ways to carry a Claude conversation over to another account: resume it on another login (`agx resume --login`), start a session switchable with `--live-login`, and move the folder's conversations to another home with `agx conversation move --dry-run`. Also the live-login refusals.

Read it with [Continue on another account](../../docs/choosing/continue-on-another-account.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-continue-elsewhere$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Runs in `~/projects/client`, where the machine's two fake conversations ran. Regions: `list`, `resume-login`, `live`, `live-refused`, `move`. |
| [`config.jsonc`](config.jsonc) | Two Claude profiles and two plan-token logins. |

## Related

- Decision guide: [Continue on another account](../../docs/choosing/continue-on-another-account.md).
- Guide: [Logins: live login](../../docs/logins.md#live-login-switch-a-running-sessions-account).
- All examples: [examples](../README.md).
