# Choose: with or without the shell layer

What changes with the shell layer: `agx login use --terminal` prints the line to copy, `agx doctor` reports the layer (`AGX_SHELL=zsh` stands in for a loaded one), a config alias is not a binary command, and `agx new` makes its folder either way.

Read it with [With or without the shell layer](../../docs/choosing/shell-layer.md).

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/choose-shell-layer$'
```

## Files

| File | What it is |
|---|---|
| [`session.txt`](session.txt) | Regions: `terminal`, `doctor`, `alias`, `new`. |
| [`config.jsonc`](config.jsonc) | One Claude profile, one login and one alias. |

## Related

- Decision guide: [With or without the shell layer](../../docs/choosing/shell-layer.md).
- Guide: [Shell layer](../../docs/shell.md).
- All examples: [examples](../README.md).
