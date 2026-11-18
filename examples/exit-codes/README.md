# Exit codes

Refusals: two flags that choose the same thing, an unknown profile, and an unknown flag on a launching and on a plain command. Each prints its reason on stderr and exits non-zero; the session states the code of each with an `[exit N]` line.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/exit-codes$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One Claude profile and one login, `carol`. Region: `profiles`. |
| [`session.txt`](session.txt) | The refusals. Regions: `conflict`, `unknown-profile`, `usage-error` (125: `run` launches), `other-usage-error` (2: `profile list` does not). |

See [exit codes](../../docs/output.md#exit-codes) for what each code means.
