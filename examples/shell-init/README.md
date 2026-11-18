# The shell layer for every shell

The full script `agx shell-init` prints for zsh, bash, fish and PowerShell with two aliases from config, the `--osc7` variant, the JSON form, the refusals, and the first lines of each `agx completion` script. Read these before you put `eval "$(agx shell-init --shell zsh)"` in a startup file.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/shell-init$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Two Claude profiles and the aliases `cl` and `clw`. Region: `aliases`. |
| [`session.txt`](session.txt) | Regions: `zsh`, `bash`, `fish`, `pwsh` (the whole scripts); inside them `zsh-aliases`, `fish-aliases`, `pwsh-aliases` and `pwsh-raw-args`. |
| [`osc7.session.txt`](osc7.session.txt) | Regions: `osc7` (the zsh script with `--osc7`; inside it `osc7-function`), `json` (`--json --field data.aliases`), `refusals` (no `--shell`, an unknown shell). |
| [`config.session.txt`](config.session.txt) | Region `config` (the bash script made with `--config`; inside it `config-helper`, the `__agx_bin` helper every call goes through). |
| [`completion.session.txt`](completion.session.txt) | Regions: `completion` (the four scripts' first lines), `completion-refusal`. |

Guide: [Shell layer](../../docs/shell.md#load-it).
