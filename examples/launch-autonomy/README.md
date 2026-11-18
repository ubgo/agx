# Autonomy levels per agent

Every `--autonomy` level on Claude Code, Codex, Gemini CLI, pi and a config-defined agent, shown as the command agx would run, plus the refusals: a level an agent cannot honour exactly, `full` without `--consent full-autonomy`, an unknown level. A profile `autonomy` key sets the level for every launch of that profile, and `--autonomy` overrides it.

When you want it: you want an agent to plan only, ask before each change, edit files without asking, or run without any prompt, and you want to know exactly which vendor flag that becomes.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-autonomy$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | One profile per built-in agent, a Claude profile `reviewer` with `"autonomy": "accept-edits"`, and a custom agent `mycli` with `autonomy_args`. The test machine has no `mycli` binary, so its spec borrows the fake `pi` binary; the command line therefore reads `pi`. Regions: `profiles`, `custom`. |
| [`session.txt`](session.txt) | Regions: `claude`, `codex`, `gemini`, `pi`, `default`, `profile`, `consent`, `custom`, `field-error` (a script reading the refusal with `--field error.message`), `concern` (one winner: `--autonomy` against `--option permission_mode` and raw args), `pi-full`. |
| `home/.mycli/` | The custom agent's home. |

Guide: [Autonomy](../../docs/guides/autonomy.md). Reference: [`agx run`](../../docs/commands/launch.md#agx-run).
