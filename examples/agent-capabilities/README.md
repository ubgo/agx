# Agent capabilities

What each built-in agent can do with agx, and the agent's own reason for each thing it cannot: `agx agent list` for the overview, `agx agent show --id` for one agent's capabilities, autonomy levels and login kinds, `--vendor-checks` to also run the binary's `--version`, and the refusals a missing capability produces.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/agent-capabilities$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `list` (`agent list`), `ids` (`agent list --json --field data.*.id`), `show-claude`, `show-codex`, `show-gemini`, `show-pi` (`agent show --id`), `levels` (autonomy levels and token kinds as JSON), `vendor` (`--vendor-checks`), `refused` (an autonomy level the agent cannot honour, `usage` on an agent without plan usage), `unknown` (an agent agx does not know, exit 1). |
| [`config.jsonc`](config.jsonc) | One default profile per built-in agent. |

The fake binaries print their arguments and environment instead of a version, so the `vendor` region reads a number out of that text; a real binary prints its own version.

Guide: [Agents and what they support](../../docs/guides/agents.md). Reference: [`agx agent list`](../../docs/commands/setup.md#agx-agent-list), [`agx agent show`](../../docs/commands/setup.md#agx-agent-show).
