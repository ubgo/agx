# Choose: built-in or config-defined agent

A config-defined agent `mycli` (its fake binary is `pi`) next to the built-in Claude agent: its capabilities and their reasons, how its spec maps `--model` and `--autonomy`, a level it does not describe, what it cannot do (`--setting`, usage), token logins through its `token_env`, and a built-in agent with raw args instead.

Read it with [Built-in or config-defined agent](../../docs/choosing/custom-agent.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-custom-agent$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `show`, `launch`, `limits`, `raw`, `login`. |
| [`config.jsonc`](config.jsonc) | The `mycli` spec, its profile and login, and a Claude profile with `args`. |
| [`home/.mycli/`](home/.mycli/) | The custom agent's home. |

## Related

- Decision guide: [Built-in or config-defined agent](../../docs/choosing/custom-agent.md).
- Guide: [Config reference: a custom agent](../../docs/config.md#a-custom-agent).
- All examples: [examples](../README.md).
