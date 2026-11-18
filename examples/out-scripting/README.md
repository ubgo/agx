# Scripting with agx

The JSON envelope on success and failure, `error.code`, `--field` (one path, several, `*`, table columns, a misspelled path, an empty list), a single-value flag given twice, notes on stderr and `--quiet`, warnings and decisions in the envelope, redacted tokens, the line width from `COLUMNS`, `--color` (and a bad value) and `NO_COLOR`, `--timeout`, `agx help --json` and `agx version --json`. Every command runs in `~/projects/client`, which is bound to the `work` profile.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/out-scripting$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude, Gemini and pi profiles, a plain-value login `carol` on `work`, and the folder binding. |
| [`session.txt`](session.txt) | Regions: `envelope`, `error`, `error-code`, `field-one`, `field-many`, `field-star`, `columns`, `field-missing`, `field-empty`, `twice`, `notes`, `warnings`, `redacted`, `decisions`, `columns-env`, `color`, `timeout`, `help-json`, `version`. |

Guide: [Scripting with agx](../../docs/guides/scripting.md). Reference: [Output, `--field` and exit codes](../../docs/output.md).
