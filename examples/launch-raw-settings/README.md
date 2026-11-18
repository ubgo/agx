# launch-raw-settings

Pass Claude Code a settings value of your own with a raw `--settings` (after `--`, or in a profile's `args`) and keep agx's settings too. agx reads the value (inline JSON or a JSONC file), merges it over its own settings key by key, takes it out of the args and passes exactly one `--settings`. Claude itself keeps only the last `--settings` it is given, so without the merge the profile's settings (and a `--live-login` helper) would be dropped. Use it when a team settings file or a one-off key should sit on top of what the profile already sets.

Run it:

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/launch-raw-settings$'
```

## Files

| File | What it is |
|---|---|
| [`config.jsonc`](config.jsonc) | Profiles `work` (settings `cleanupPeriodDays` and `permissions.allow`, the default), `team` (a raw `--settings ~/.config/claude/team.jsonc` in its `args`) and `live` (`"live_login": true`, login `carol`). Region: `profiles`. |
| [`home/.config/claude/team.jsonc`](home/.config/claude/team.jsonc) | The team's settings file; comments and trailing commas are allowed. |
| [`home/projects/app/local.json`](home/projects/app/local.json) | A settings file in the launch folder, named by a relative path. |
| [`home/projects/app/broken.jsonc`](home/projects/app/broken.jsonc) | A file with a syntax error, for the error region. |
| [`session.txt`](session.txt) | Regions: `inline` (the merge in the dry run: each raw key `from raw --settings`, with the profile keys it overrides), `merged` (the one inline value Claude gets: `permissions` keeps `allow` and gains `deny`), `file` (`--settings=FILE`, relative to the launch folder), `profile-args` (a raw `--settings` in profile `args`, with no agx settings of its own), `two` (two raw values merge in order), `live` (merged with the live-login helper; a raw `apiKeyHelper` is refused), `errors` (a missing file, bad JSONC with line:col, no value). |

Explained in [Launch options: Recipe 8: a raw --settings of your own](../../docs/launch-options.md#a-raw---settings-of-your-own) and [Raw args](../../docs/guides/raw-args.md).
