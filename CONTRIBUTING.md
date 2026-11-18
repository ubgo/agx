# Contributing to agx

Thanks for helping improve agx. This page covers the toolchain, the build, the tests and docs gates every change must pass, and the conventions the code and docs follow.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Security problems go through the [security policy](SECURITY.md), never a public issue.

## Prerequisites

- Go 1.26.4 or newer (the version in `go.mod`).
- [Task](https://taskfile.dev) to run the tasks in `Taskfile.yml`.
- For `task check`: [staticcheck](https://staticcheck.dev) and [golangci-lint](https://golangci-lint.run) v2 on your `PATH`.

None of the agents agx drives (Claude Code, Codex, Gemini CLI, pi) needs to be installed: the tests use fakes.

## Build

```sh
git clone https://github.com/ubgo/agx.git
cd agx
task build          # go build -o dist/agx2 .
./dist/agx2 --help
```

The build goes to `dist/agx2` on purpose, so it never replaces an `agx` already on your `PATH`. Try it against a throwaway config with `AGX_CONFIG_DIR=$(mktemp -d) ./dist/agx2 …`.

## Tests

```sh
task test           # go test -count=1 ./...
task check          # everything that must be green before a commit
```

`task check` runs, in order:

| Task | What it runs |
|---|---|
| `fmt:check` | fails when a Go file is not `gofmt`-clean |
| `vet` | `go vet ./...` |
| `staticcheck` | `staticcheck ./...` |
| `lint` | `golangci-lint run ./...` with `.golangci.yml` (no exclusions: findings are fixed, never silenced) |
| `test:race` | `go test -race -count=1 ./...` |
| `windows` | builds and vets every package for `windows/amd64` |
| `cross` | builds the binary for darwin, linux and windows on amd64 and arm64 |
| `docs:check` | the docs gates below |

Nothing runs on push or pull request. The local `task check` is the gate: run it before you open a pull request and say in the pull request that it passed. The maintainers can also run the same gate on Linux and macOS by hand with the `test` workflow (`.github/workflows/test.yml`, Actions tab → Run workflow).

### Test isolation

Tests never use a real account, a real vendor binary, the network or quota. Use instead:

- **Fake agents**: `internal/testkit` builds the real agx binary once and runs it on a fixture machine (a temporary home and config dir, fake `claude`, `codex`, `gemini` and `pi` scripts on `PATH` that print what they received, a clean environment).
- **Fixture homes**: agent homes built in a temporary folder, and agentio's `agenttest` fakes for code that talks to an agent.
- **`httptest`** servers and recorded fixtures for usage endpoints. Never loop live calls in a test: the real endpoints rate-limit.

`internal/archtest` enforces the layer rules by reading the source (which package may import cobra, start processes, write files or import an agent module), so a change that breaks one fails a test, not a review.

## Docs gates

`task docs:check` (`go test -count=1 ./internal/docscheck/`) checks every public markdown file (`README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`, everything under `docs/` and `.github/`, and the READMEs under `examples/`):

- **Command reference**: every command and flag agx declares has a heading and a mention under `docs/commands/`; a documented flag or command that does not exist fails too.
- **Runnable blocks**: a ```` ```sh ```` block marked `<!-- runnable -->` runs on a fake machine and must print exactly the output shown.
- **Examples**: every folder under `examples/` is a scripted session that runs with the real binary (`task examples`).
- **Snippet sync**: a block marked `<!-- snippet: examples/NAME/FILE#REGION -->` must equal that region of the example file.
- **Config reference**: `schema/config.schema.json` and `docs/config.md` must match the config types (`task schema` regenerates the schema).
- **Links**: relative links and `#anchors` must resolve inside the repository.
- **No hard wraps**: one physical line per paragraph, bullet or table cell. Never wrap markdown prose at a fixed width.
- **Content**: no local machine paths (write `~`), no authoring attribution.

`internal/docscheck/doc.go` describes each gate in full.

### Add an example

1. Create `examples/<kebab-name>/` with a `README.md` (what it shows, how to run it, its files) and a `session.txt`; add `config.jsonc` or a `home/` folder only if the session needs them. [examples/README.md](examples/README.md) describes the format.
2. Write the commands as `$ agx …` lines, with a placeholder line under each.
3. Fill in the real output: `go test -count=1 ./internal/docscheck/ -run 'TestExamples/<kebab-name>$' -args -update`. Read the result: it is what agx prints today.
4. Run the test again without `-update`, and add the example to the index in `examples/README.md`.

### Show part of an example in a doc

Wrap the part in `#region <name>` and `#endregion` lines in the session (or behind the file's own comment token, such as `// #region x` in JSONC). In the doc, put the marker `<!-- snippet: examples/<kebab-name>/session.txt#<name> -->` on the line directly above a fenced block (of any language) that holds a copy of the region; the [README](README.md) quick start does this with `examples/readme-tour`. The block must equal the region, marker lines excluded and trailing whitespace ignored. Run `go test -count=1 ./internal/docscheck/ -run TestSnippets`; a failure names the first differing line. After an example's output changes, `go test -count=1 ./internal/docscheck/ -run TestSnippets -args -update` copies every region into its doc block (read the diff before you commit it). [examples/README.md](examples/README.md#showing-an-example-in-a-doc) has a full example.

## Code conventions

- **No vendor logic in agx.** Everything about an agent comes from [agentio](https://github.com/ubgo/agentio); a missing capability is agentio's typed unsupported reason, never an `if claude` branch here.
- **Named flags only.** The action is the command, a flag names a thing, a mode is one flag with a value, negatives are `--no-x`. A bare `agx` prints help.
- **No magic.** Every decision has one documented source that `--dry-run` shows. Plain values are allowed anywhere; only a true conflict inside one command is refused.
- **Never refresh, write or print an OAuth token** (the one exception: `agx login token` prints a login's token on request), never put one on argv, redact every secret in every output (see [SECURITY.md](SECURITY.md)).
- No lint suppressions, no `any` in signatures, named constants for closed sets, and comments that say why.

## Commits

- One logical change per commit, in the style of `git log`: `type: what changed`, or `type(scope): …`, in the imperative and lower case. Types in use: `feat`, `fix`, `docs`, `test`, `refactor`, `style`, `chore`.
- Run `task check` before committing; a red gate is never committed.
- User-facing changes get a line under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), following [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Pull requests

- Open an issue first for anything non-trivial, so the approach is agreed before the code is written.
- Keep the change scoped, with tests for new behaviour and docs for new commands, flags or config keys (the gates will tell you which are missing).
- Fill in the pull request template, including the `task check` result.
