# Launching commands at a glance

One dry run per launching command, on a machine with two Claude homes, a Codex home and a stored login `carol`: `agx run` with a profile, a login, a model and `--dir`; `agx new` with `--name`; `agx resume` of a conversation on another login's quota; the refusals when two flags choose the same thing; `agx whoami` outside a session.

When you want it: a first look at what each launching command decides before you read the [command reference](../../docs/commands/launch.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-commands$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | `agents.claude.defaults.args`, Claude profiles `personal` and `work`, a Codex profile, a login `carol` (a gopass reference; the test machine's gopass is a stand-in). |
| [`session.txt`](session.txt) | Regions: `run`, `new`, `resume`, `run-conflicts`, `new-dir` (`agx new` has no `--dir`), `whoami`. |

Reference: [Launching: run, new, resume, whoami](../../docs/commands/launch.md). Guides: [Run folders](../../docs/guides/run-folders.md), [Autonomy](../../docs/guides/autonomy.md), [Headless runs](../../docs/guides/headless.md), [Raw args](../../docs/guides/raw-args.md).
