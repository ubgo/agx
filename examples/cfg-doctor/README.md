# Doctor and config checks

`agx doctor` on a healthy machine and on a broken config, `--agent`, `--resolved-secrets` and `--vendor-checks`, the shell-layer check, and `agx config path`, `agx config check` (unknown keys, wrong types, cross-check problems, a syntax error) and `agx config show`.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/cfg-doctor$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Claude and Gemini profiles, a gopass login, a plain-value login (reported as information). |
| [`session.txt`](session.txt) | Regions: `doctor`, `agent` (`--agent gemini`), `shell` (`AGX_SHELL` set, as the shell layer does), `secrets` (`--resolved-secrets`), `vendor` (`--vendor-checks`), `json` (one check as JSON), `broken` (`--config ~/broken.jsonc`: three failures, exit 1), `config-path`, `check`, `check-typo`, `check-cross`, `check-syntax`, `show`. |
| [`home/broken.jsonc`](home/broken.jsonc) | A config with a misspelled scheme, a command reference whose program is missing, and a stored login. |
| [`home/tokens.jsonc`](home/tokens.jsonc) | A tokens file with mode 0644 next to `broken.jsonc`, which doctor fails. |
| [`home/typo.jsonc`](home/typo.jsonc) | An unknown key and a wrong type (exit 1). |
| [`home/cross.jsonc`](home/cross.jsonc) | A missing login, two default profiles and an alias to a hidden command (exit 2). |
| [`home/syntax.jsonc`](home/syntax.jsonc) | A missing comma (exit 1). |

The fake `gemini` prints its arguments and environment instead of a version, so `--vendor-checks` reads a number out of that text; a real binary prints its own version.

Guide: [Check your setup](../../docs/guides/doctor.md). Reference: [`agx doctor`](../../docs/commands/setup.md#agx-doctor), [`agx config check`](../../docs/commands/setup.md#agx-config-check).
