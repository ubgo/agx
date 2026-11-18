# launch-settings

Hand Claude Code settings to one launch without editing the home's own `settings.json`: a settings file named by the profile, settings keys written inline in the profile, one key for one run with `--setting`, and the two ways the result reaches Claude (`settings_delivery` `file` or `inline`). Use it when one account needs stricter hooks or permissions than your others, or when you want to try a key for one run.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-settings$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Three Claude profiles: `strict` (a settings file), `work` (a settings object), `visible` (`"settings_delivery": "inline"`), plus two broken ones for the refusals. Regions: `profiles` (with `strict`, `work`, `visible` inside), `broken`. |
| [`home/.config/claude/strict.json`](home/.config/claude/strict.json) | The settings file of `strict`; comments are allowed. |
| [`session.txt`](session.txt) | Regions: `file`, `object`, `one-run` (`--setting` and `--settings-delivery inline`), `string-value` (a JSON-quoted string), `not-modelled` (a typo passes with a note), `wrong-type` (refused), `inline-profile`, `file-merge` (`--setting` over a settings file), `missing-file`, `profile-type`, `key-helper` (`apiKeyHelper` replaces the agx login). |

Explained in [Passing anything to an agent](../../docs/launch-options.md#recipe-1-a-claude-settings-file) (recipes 1 to 3 and [How settings reach the agent](../../docs/launch-options.md#how-settings-reach-the-agent)).
