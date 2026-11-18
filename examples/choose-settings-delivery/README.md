# Choose: settings as a file or inline

The same Claude settings delivered as a file in the run folder and inline on the command line (per run and per profile), the warning inline adds, Codex turning settings into `-c` flags either way, an invalid value, and a raw `--settings` merged into agx's settings and delivered the same way.

Read it with [Settings as a file or inline](../../docs/choosing/settings-delivery.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-settings-delivery$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `file`, `inline`, `codex`, `refused`, `raw`. |
| [`config.jsonc`](config.jsonc) | A Claude profile with one settings key, the same with `"settings_delivery": "inline"`, and a Codex profile. |

## Related

- Decision guide: [Settings as a file or inline](../../docs/choosing/settings-delivery.md).
- Guide: [Passing anything to an agent](../../docs/launch-options.md#how-settings-reach-the-agent).
- All examples: [examples](../README.md).
