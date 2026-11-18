# Headless runs and a first prompt

`--prompt` alone opens the agent's interactive UI with a first prompt. `--headless` with `--prompt` runs one task and streams machine-readable JSON events, for scripts and CI. The session shows the command per agent (Claude Code, Codex, Gemini CLI, pi), a headless resume, the refusal when a typed option sets the same thing, and one real headless launch on the fake `claude`.

When you want it: a CI job, a git hook or a script runs an agent on a task and reads its output.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-headless$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One profile per built-in agent, each its agent's default, a Claude profile `batch` with the typed option `"print": true`, and a Claude profile `report` with `"print": true` and `"output_format": "json"` together. |
| [`session.txt`](session.txt) | Regions: `prompt`, `claude`, `codex`, `gemini`, `pi`, `ci`, `resume`, `conflict`, `launch`, `profile-override` (`--headless` wins over the profile's `print`), `print-format` (`print` and `output_format` from one input launch together; `--headless` plus `--option output_format` is refused). |

Guide: [Headless runs](../../docs/guides/headless.md). Reference: [`agx run`](../../docs/commands/launch.md#agx-run).
