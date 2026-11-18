# Aliases

Add, replace, list and remove aliases with `agx alias`, see the functions the shell layer makes of them, preview what an alias runs with `--dry-run`, and every refusal: shell characters, a positional word, a group or internal command, an unknown flag, an unknown alias. Two more sessions show a broken alias in a config (the layer still loads, without aliases) and a reserved name: `alias add` refuses it, and the layer skips one written by hand.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/shell-aliases$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Two Claude profiles, no aliases yet. |
| `home/broken.jsonc` | A config with an alias holding `;`. Region: `bad-alias`. |
| `home/reserved.jsonc` | A config with an alias named `agx`, written by hand. |
| [`session.txt`](session.txt) | Regions: `add`, `functions`, `dry-run`, `remove`, `refusals`. |
| [`broken.session.txt`](broken.session.txt) | Region `broken`: shell-init warns and prints the wrapper; `profile list` refuses the config (exit 2). |
| [`reserved.session.txt`](reserved.session.txt) | Region `reserved`: `alias add --name agx` is refused (exit 2); shell-init with `--config ~/reserved.jsonc`, where the name was written by hand, warns that the config was refused at load (line:col) and prints the wrapper without aliases. |

Guide: [Aliases](../../docs/shell.md#aliases).
