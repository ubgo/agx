# Quickstart

The first steps on a new machine: list the agent homes agx finds, write a starter config from them, and ask what a launch would do without starting anything. A second session stores a plan token read from stdin.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/quickstart$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | `agx profile list --discovered`, `agx config init --discovered`, `agx profile list`, `agx run --dry-run`, on a machine with no config. Regions: `discover`, `init`, `dry-run`. |
| [`login.session.txt`](login.session.txt) | On its own fresh machine, run from `~/projects/client`: `agx login add --stdin < ~/token.txt`, then `agx login list`. Region: `add`. |
| [`home/token.txt`](home/token.txt) | The fake token the second session reads, copied to `~/token.txt`. |

There is no `config.jsonc`: the machine starts without one, which is what `config init` is for.
