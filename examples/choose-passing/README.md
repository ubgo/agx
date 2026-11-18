# Choose: which way to pass something

One Claude profile with `env`, a typed option and a settings object, then the model passed by each road (`--model`, `--option`, `--setting`, raw after `--`), an environment override, the refusal of two choices for one concern, and what Codex and Gemini CLI accept.

Read it with [Which way to pass something](../../docs/choosing/passing-options.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-passing$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `profile`, `roads`, `env`, `conflict`, `other-agents`. |
| [`config.jsonc`](config.jsonc) | A Claude, a Codex and a Gemini CLI profile. |

## Related

- Decision guide: [Which way to pass something](../../docs/choosing/passing-options.md).
- Guide: [Passing anything to an agent](../../docs/launch-options.md).
- All examples: [examples](../README.md).
