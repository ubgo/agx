# launch-options

Typed launch options for Claude Code: the profile's `model` and its `claude` block (`effort`, `extra_args`, `prompt`), `--option key=value` for one run, `--model`, `--add-dir` and `--prompt`, how the dry run names what the command line overrode, and the refusals of one winner per concern. The last region launches the fake agent to show that a JSON value arrives as one argument, with no shell quoting.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-options$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `work` (`model`, `claude.effort`, `claude.extra_args`) and `review` (`claude.prompt`). Regions: `profiles` (with `work` and `review` inside). |
| [`session.txt`](session.txt) | Regions: `profile-block`, `option`, `model-add-dir`, `extra-args-raw` (refused), `block-prompt`, `override`, `unknown-option`, `conflict`, `no-quoting`. |

Explained in [Passing anything to an agent](../../docs/launch-options.md#recipe-7-typed-vendor-launch-flags) (recipes 6 and 7, [One winner per concern](../../docs/launch-options.md#one-winner-per-concern), [Why none of this needs shell quoting](../../docs/launch-options.md#why-none-of-this-needs-shell-quoting)).
