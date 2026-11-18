# Getting started

The first session on a new machine, step by step as [Getting started](../../docs/getting-started.md) walks it: write an empty starter config, import the agent homes agx finds, import a home that has no markers yet by its path, read usage from disk, preview a launch, then launch for real. The fake `claude` prints what it received: its arguments, its folder and its environment, so you can see that the raw `--verbose` arrived untouched and that the `work` profile's home reached Claude as `CLAUDE_CONFIG_DIR`.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/getting-started$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | `agx config init`, `agx profile import --discovered --yes`, `agx profile import --id`, `agx profile list`, `agx usage --cached`, `agx run --dry-run`, `agx run`, and a refusal. Regions: `init`, `import`, `import-id`, `usage`, `dry-run`, `run`, `refused`. |

There is no `config.jsonc`: the machine starts without one, which is what `config init` is for.

## Related

- Guide: [Getting started](../../docs/getting-started.md).
- The same first steps in one command (`config init --discovered`): [quickstart](../quickstart/README.md).
- All examples: [examples](../README.md).
