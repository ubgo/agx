# Usage: how reset times read

`agx usage --cached` on a Claude home whose reading resets today, on Friday and in seven days, written every way agx can: `relative` (the default), `both` (relative with the clock time after it), `day` and `absolute`, in the bars and the `--table` view; then `--date-format` with the `us` and `iso` presets and a custom format, the same choices kept in config (`usage.time_format`, `date_format`) and overridden by the flags for one run, and the refusals of a format agx cannot read. The machine's clock is fixed at Wed 18 Nov 2026 12:00 UTC, so every time shown is exact.

The guide is [Plan usage](../../docs/guides/usage.md#step-4-change-how-times-read); the flags are in the [usage reference](../../docs/commands/usage.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-usage-times$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One Claude profile, `personal`, on `~/.claude`. |
| [`session.txt`](session.txt) | Regions: `relative`, `both`, `day-absolute`, `duration` (`--duration-format truncate`, `round`, `minutes` and a refusal), `formats`, `config`, `refusals`. |
| [`home/.claude.json`](home/.claude.json) | The home's account file with a reading taken 30 minutes before the fixed clock (fake): the session window resets at 15:10 today, the weekly window on Friday, the Sonnet weekly window in seven days. |

## Related

- Guide: [Plan usage](../../docs/guides/usage.md).
- Reference: [agx usage](../../docs/commands/usage.md), [config `usage` keys](../../docs/config.md#usage).
- All examples: [examples](../README.md).
