# With or without the shell layer

The shell layer is one `agx()` function your shell loads from `agx shell-init`. It exists because a program cannot change its parent shell: it cannot `cd` your shell into the folder `agx new` or `agx resume` starts in, and it cannot set `AGX_LOGIN` in your terminal. Everything else in agx works the same with or without it. This page helps you decide whether to load it. Loading it is in [Shell layer](../shell.md).

## Short answer

| You | Choose |
|---|---|
| use `agx new` or `agx resume` and want your shell to be in that folder when the agent exits | load the layer |
| switch logins per terminal (`agx login use --id NAME --terminal`) | load the layer, or copy the printed `export` line yourself |
| keep short commands in config (`aliases`) | load the layer: aliases exist only as shell functions |
| want new terminal tabs to open in the current folder | load it with `--osc7` |
| run agx from scripts, CI, an IDE task or a status bar | no layer: scripts call the binary, and every command works |
| use a shell other than zsh, bash, fish or PowerShell | no layer; use `export AGX_LOGIN=…` by hand |

## What changes

| Feature | Without the layer | With it |
|---|---|---|
| `agx new`, `agx resume` | the agent starts in the right folder; your shell stays where it was | your shell `cd`s there first, then the agent starts |
| `agx login use --id X --terminal` | prints `export AGX_LOGIN=X` for you to copy | sets `AGX_LOGIN` in this shell and says `agx: AGX_LOGIN=X in this shell` |
| `agx login use --home-login --terminal` | prints `unset AGX_LOGIN` | unsets it |
| `aliases` from config | not defined: `agx cl` is an unknown command | one shell function per alias |
| `agx doctor` | reports the layer as not loaded (information, not a failure) | reports it loaded, with the shell |
| everything else | the binary | the same binary (`command agx`) |

Without the layer, `--terminal` prints the line instead of running it:

<!-- snippet: examples/choose-shell-layer/session.txt#terminal -->
```sh
$ agx login use --id carol --terminal
export AGX_LOGIN=carol

$ agx login use --home-login --terminal
unset AGX_LOGIN
```

`agx doctor` reads `AGX_SHELL`, which the layer exports:

<!-- snippet: examples/choose-shell-layer/session.txt#doctor -->
```sh
$ agx doctor --json --field data.3.message
shell layer not loaded: new and resume leave this shell where it is

$ AGX_SHELL=zsh agx doctor --json --field data.3.message
shell layer loaded (zsh)
```

An alias is a shell function the layer defines, so without it the binary does not know the name; the alias's own words always work:

<!-- snippet: examples/choose-shell-layer/session.txt#alias -->
```sh
$ agx cl
agx: unknown command "cl"; see agx --help
[exit 2]

$ agx run --profile personal --dry-run --quiet --json --field data.profile
personal
```

`agx new` makes the session folder either way; only the `cd` of your shell needs the layer:

<!-- snippet: examples/choose-shell-layer/session.txt#new -->
```sh
$ agx new --name demo --dry-run --quiet --json --field data.folder
~/.agx/sessions/demo_20261114_101500
```

## If this, then that

- If you work in a terminal every day, load it: one line in your shell's startup file, and agx stays a plain binary for everything else.
- If you only script agx, skip it. Nothing a script needs depends on it, and no launch decision ever reads `AGX_SHELL`.
- If you use `AGX_LOGIN` but do not want the layer, `export AGX_LOGIN=NAME` yourself; agx reads the variable the same way.
- If your terminal supports OSC 7 (most do), add `--osc7` so new tabs and splits open where your shell is.

## Pitfalls

- The layer only routes: `new` and `resume` write a launch plan to a private temporary file, the function `cd`s to its folder, then runs the hidden `agx launch --plan-file F`. A plan older than 60 seconds, or owned by another user, is refused.
- The function matches only the exact `export AGX_LOGIN=…` and `unset AGX_LOGIN` lines and never `eval`s agx output, so a login name can never run as code. Login names are limited to letters, digits and `._-` for the same reason.
- Alias values may only contain letters, digits, spaces and `_./:=@+-`, so an alias cannot smuggle shell syntax into your shell.
- A layer made with `--config X` (`eval "$(agx shell-init --shell zsh --config ~/work.jsonc)"`) runs every command and alias against that file; a `--config` you type wins over it.
- After changing `aliases` in config, reload the layer (open a new terminal or re-run the `eval` line): the functions are generated when it loads.

## Related

- Guide: [Shell layer](../shell.md), [Logins: per-terminal login](../logins.md#per-terminal-login-agx_login).
- Commands: [`agx shell-init`](../commands/setup.md#agx-shell-init), [`agx alias`](../commands/setup.md#agx-alias-add), [`agx new`](../commands/launch.md#agx-new).
- Example: [choose-shell-layer](../../examples/choose-shell-layer/README.md), [shell-init](../../examples/shell-init/README.md), [shell-plan-file](../../examples/shell-plan-file/README.md), [shell-aliases](../../examples/shell-aliases/README.md).
- Other decisions: [Where to set the profile and login](where-to-set.md).
