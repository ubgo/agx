# Decide: agent and profile

Where the agent and the profile of a run come from: the profile named by `--profile`, `--agent` with its default profile, a folder binding, `default_agent`, and what happens when nothing names one. Read it with the [agent](../../docs/how-agx-decides.md#step-1-the-agent) and [profile](../../docs/how-agx-decides.md#step-2-the-profile) steps of how agx decides.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-agent-profile$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude profiles `personal` (default) and `work`, Codex profile `codex` (default), Gemini profile `gem` with no default, no `default_agent`, and `~/projects/client` bound to Codex. Regions: `profiles`, `projects`. |
| [`session.txt`](session.txt) | `agx run --dry-run` refused without `default_agent` (`no-agent`); `project show` with `--profile` (`agent-from-profile`), `--agent` (`agent-flag`), both disagreeing (`agent-conflict`), in the bound folder (`agent-binding`), for an agent without a default profile (`no-default-profile`); then `agx config set --key default_agent` and the run it fixes (`default-agent`); `--profile` with `--auto-profile` refused, exit 125 (`auto-conflict`). |

`agx project show` runs the same resolver as a launch for the agent, profile, home and login, so its lines are what `agx run --dry-run` prints first.
