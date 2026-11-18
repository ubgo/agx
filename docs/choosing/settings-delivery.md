# Settings as a file or inline

When a Claude launch has settings (from the profile's `claude.settings`, `--setting`, or a login or live login that needs `apiKeyHelper`), agx hands them to Claude with `--settings` in one of two ways: a path to a file it writes into the run folder (`file`, the default), or the JSON itself on the command line (`inline`). You choose with `--settings-delivery` for one run or `"settings_delivery"` on the profile. This page helps you pick. The settings themselves are in [Passing anything to an agent](../launch-options.md#how-settings-reach-the-agent).

## Short answer

| You want | Choose |
|---|---|
| the safe default: nothing on the command line, no length limit | `file` (do nothing) |
| to read the exact settings in `--dry-run`, `ps` or a process list | `inline` |
| no file left in the run folder | `inline` |
| settings that hold anything private (hooks with tokens, `env` values, an `apiKeyHelper` command) | `file` |
| Codex | either: Codex settings always become `-c key=value` flags, so the choice changes nothing |

## Compare

| | `file` | `inline` |
|---|---|---|
| Claude gets | `--settings ~/.agx/runs/<run id>/settings-<run id>.json` | `--settings '<json>'` |
| file | written with mode 0600 in the run folder; stays until `agx clean` (agx has exec'd into Claude and cannot remove it) | none |
| visible to other users in `ps` | the path only | the whole JSON; agx warns once per launch |
| `--dry-run` shows | the keys and the file path | the keys and the JSON in the command |
| length | no limit | the system's argument length limit |

## See it

<!-- snippet: examples/choose-settings-delivery/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "work": { "agent": "claude", "home": "~/.claude-work", "default": true, "claude": { "settings": { "cleanupPeriodDays": 20 } } },
    "visible": { "agent": "claude", "home": "~/.claude-work", "settings_delivery": "inline", "claude": { "settings": { "cleanupPeriodDays": 20 } } },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true }
  }
}
```

By default the settings go into a file:

<!-- snippet: examples/choose-settings-delivery/session.txt#file -->
```sh
$ agx run --dry-run --quiet --json --field data.command.line
claude --settings ~/.agx/runs/20261114-101500-k3x9/settings-20261114-101500-k3x9.json
```

Inline puts the JSON on the command line and adds a warning, for one run or for every run of a profile:

<!-- snippet: examples/choose-settings-delivery/session.txt#inline -->
```sh
$ agx run --settings-delivery inline --dry-run --quiet --json --field data.command.line --field data.warnings
{
  "data.command.line": "claude --settings \"{\\\"cleanupPeriodDays\\\":20}\"",
  "data.warnings": [
    {
      "code": "inline-settings",
      "message": "settings are visible in ps (inline delivery, from --settings-delivery)"
    },
    {
      "code": "version",
      "message": "installed Claude Code version unknown; flags not checked"
    }
  ]
}

$ agx run --profile visible --dry-run --quiet --json --field data.command.line
claude --settings "{\"cleanupPeriodDays\":20}"
```

Codex turns settings into `-c` flags whichever you choose:

<!-- snippet: examples/choose-settings-delivery/session.txt#codex -->
```sh
$ agx run --profile codex --setting model_reasoning_effort=high --settings-delivery inline --dry-run --quiet --json --field data.command.line
codex -c "model_reasoning_effort=\"high\""
```

## If this, then that

- If you are unsure, keep `file`. It is never wrong, only less visible.
- If you debug a settings merge (profile object plus `--setting` keys), run once with `--settings-delivery inline --dry-run` to read the merged JSON, then go back to `file`.
- If run folders pile up, that is what `agx clean` is for; switching to `inline` only to avoid files trades a file only you can read for a command line everyone can.
- If you pass your own `--settings` after `--`, delivery no longer matters: agx passes none of its own (next section).

## Pitfalls

- Only `file` and `inline` exist; anything else is refused before launch:

<!-- snippet: examples/choose-settings-delivery/session.txt#refused -->
```sh
$ agx run --settings-delivery stdin --dry-run --quiet
agx: --settings-delivery "stdin": use one of file, inline
[exit 125]
```

- A raw `--settings` after `--` (or in profile `args`) is merged into agx's settings key by key and delivered the same way, as one `--settings`: here the profile's `inline` delivery carries both its `cleanupPeriodDays` (overridden) and the raw `theme` (see [launch options, recipe 8](../launch-options.md#a-raw---settings-of-your-own)):

<!-- snippet: examples/choose-settings-delivery/session.txt#raw -->
```sh
$ agx run --profile visible --dry-run --quiet --json --field data.command.line -- --settings '{"cleanupPeriodDays":5,"theme":"dark"}'
claude --settings "{\"cleanupPeriodDays\":5,\"theme\":\"dark\"}"
```

- Gemini CLI, pi and config-defined agents have no settings flag: `--setting` is refused for them, so `settings_delivery` never applies.
- The `apiKeyHelper` of an `api-key-helper` login or of live login is a settings key too: with `inline` the helper command is visible in `ps`.

## Related

- Guide: [Passing anything to an agent](../launch-options.md#how-settings-reach-the-agent).
- Config: [profiles](../config.md#profiles) (`settings_delivery`).
- Example: [choose-settings-delivery](../../examples/choose-settings-delivery/README.md), [launch-settings](../../examples/launch-settings/README.md).
- Other decisions: [Which way to pass something](passing-options.md).
