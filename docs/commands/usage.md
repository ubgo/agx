# Usage

[Plan usage](../guides/usage.md) is the guide: live and cached readings, the login probes, the table, and use in a status bar. The examples below come from [`examples/acct-usage-cached`](../../examples/acct-usage-cached/README.md), which runs in the test suite with no network: live usage and the probes need a real account, so they are not shown with output here.

## agx usage

Plan usage, one row per configured home: the 5-hour window, the weekly window and when each resets. Cost N (one request per home); R with `--cached`; Q with the probe flags.

| Flag | Meaning |
|---|---|
| `--profile`, `-p` | only these profiles' homes (repeat) |
| `--agent` | only this agent |
| `--plan` | add the plan, subscription status and start date (live: one more request per home; with `--cached`: the plan label on disk) |
| `--cached` | read the last readings from disk, no network; each row shows its age |
| `--login-probes` | add one probed row per stored login (Q) |
| `--login` | show these logins instead of homes (repeat; Q) |
| `--all-logins` | every stored login (Q) |
| `--login-picker` | pick logins in a checklist first (Q; needs a terminal) |
| `--table` | one compact row per home or login |
| `--sort` | `login`, `left`, `5h`, `5h-reset`, `weekly`, `weekly-reset`, `day` or `status`; a `-` prefix sorts descending; repeat for tie-breaks |
| `--tz` | an IANA time zone for reset days and dates; default this machine's |
| `--time-format` | how reset times read: `relative` ("2 hr 50 min", the default), `day` ("Tue 05:30"), `absolute` ("Tue 24 Nov 05:30") or `both` ("3 hr 10 min (22:20)"; the weekday is added when the reset is not today, "1 d 4 hr (Thu 23:30)", and the date when it is more than six days away); default config `usage.time_format`, else `relative`. The clock and date parts, and `RESETS ON`, follow the global `--date-format` (default config `usage.date_format`, else `date_format`, else `long`); see [change how times read](../guides/usage.md#step-4-change-how-times-read) |

```sh
agx usage
agx usage --cached
agx usage --profile work --profile personal --plan
agx usage --all-logins --table --sort -left
agx usage --json --field data.*.profile --field data.*.left
```

The default view is one block per home or login: the account in bold, its plan, profile, agent and home, then one bar per window (`█` used, `░` left; blue, amber from 80%, red at the cap) with its percent and reset. `--table` prints one row per home or login in these columns: `#` (the login's or home's place in the config file, kept under `--sort`), `LOGIN` (`PROFILE` for homes), `PROVIDER`, `ACCOUNT`, `5H USED`, `5H RESETS`, `WEEKLY USED`, `WEEKLY RESETS`, `RESET DAY`, `RESETS ON`, a `USED`/`RESETS` pair per model window, `LEFT` and `STATUS` (the status words alone, such as `expired` or `limit reached`); under it a `Weekly resets:` line sums, per weekday, the weekly use that comes back then, and each row's reason follows as `  name: reason`. Colours (bold header, dim `#`, coloured `LEFT` and `STATUS`) appear on a terminal; `--color never`, `NO_COLOR` or a pipe turn them off, `--color always` forces them.

Each row has a `status`: `ok`, `limit-reached`, `per-token`, `unsupported`, `no-cache` or `stale` are answers; `rate-limited`, `expired`, `not-logged-in`, `rejected`, `scope-limited`, `home-missing`, `secret`, `network` and `error` are failures.

### Cached readings

With `--cached` on a machine where the agents have not run yet there is no reading (`no-cache`), and agents without plan usage say why. Claude Code 2.1.323 and later no longer store a usage reading on disk and never refresh an old one, so for Claude `--cached` shows `no-cache`, or `stale` for a reading older than a day, with that reason; use live `agx usage` for current numbers:

<!-- runnable -->
```sh
$ agx usage --cached --json --field data.*.status
["no-cache","no-cache","no-cache","unsupported"]
$ agx usage --cached --json --field data.3.reason
Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
```

A `stale` reading keeps its windows (their reset times may be in the past) and is not a failure:

<!-- snippet: examples/acct-usage-cached/session.txt#stale -->
```sh
$ agx usage --cached --profile personal --json --field data.*.status
["stale"]

$ agx usage --cached --profile personal --json --field data.0.windows --tz UTC
[{"class":"short","label":"Current session","percent":6,"resets_at":"2026-10-28T12:59:59.902135Z","reset":"Wed 28 Oct 13:00"},{"class":"week","label":"All models","percent":42,"resets_at":"2026-11-04T05:59:59.902151Z","reset":"Wed 4 Nov 06:00"}]

$ agx usage --cached --plan --profile personal --json --field data.0.plan
{"plan":"Max","source":"cached"}
```

### Table, sort and time zone

<!-- snippet: examples/acct-usage-cached/session.txt#table -->
```sh
$ agx usage --cached --table --profile work --profile codex --profile gem --sort login
#  PROFILE  PROVIDER  ACCOUNT  5H USED  5H RESETS  WEEKLY USED  WEEKLY RESETS  RESET DAY  RESETS ON  LEFT  STATUS
5  codex    codex     —        —        —          —            —              —          —          —     no cache
6  gem      gemini    —        —        —          —            —              —          —          —     unsupported
3  work     claude    —        —        —          —            —              —          —          —     no cache
  codex: no reading on disk yet; run the agent once, or drop --cached
  gem: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
  work: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage
```

### Rows and exit codes

- A home shared by several profiles is shown once, with all of its profiles. When every profile on it has `"billing": "api"`, the row is `per-token` and nothing is requested.
- Agents without plan usage (Gemini, pi, custom agents) are listed as `unsupported` with their reason.
- A warning names homes on disk that are not profiles yet, so a forgotten account is visible (not with `--profile`):

<!-- snippet: examples/acct-usage-cached/session.txt#footer -->
```sh
$ agx usage --cached --agent claude --json --field warnings
[{"code":"unconfigured-home","message":"claude home ~/.claude-client is not in config; agx profile import --discovered"}]
[exit 3]
```

- When some homes answer and some fail (no network, a rate limit, a missing home), the answering rows are shown and the exit code is 3. When none answer, it is 1:

<!-- snippet: examples/acct-usage-cached/session.txt#partial -->
```sh
$ agx usage --cached --profile work --profile old
agx: 1 of 2 rows failed: old (home-missing)
work  work · claude  ~/.claude-work
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

old  old · claude  ~/.claude-old
  home missing: home ~/.claude-old does not exist
[exit 3]

$ agx usage --cached --profile old
agx: no row answered: old (home-missing)
[exit 1]
```

### Pitfalls

- Usage endpoints rate-limit: agx makes one request per home per call and never retries in a loop. In a status bar or prompt, read one profile live every few minutes; `--cached` costs nothing but, for Claude, holds no current reading.
- The probe flags spend a few tokens of each plan. They cannot be combined with `--cached` (exit 2), `--login-picker` needs a terminal (exit 2), and an unknown login or profile exits 1:

<!-- snippet: examples/acct-usage-cached/session.txt#refusals -->
```sh
$ agx usage --cached --login-probes
agx: --cached reads disk only and --login-probes probes logins (spends quota); pick one
[exit 2]

$ agx usage --cached --all-logins
agx: --cached reads disk only and --all-logins probes logins (spends quota); pick one
[exit 2]

$ agx usage --login-picker
agx: no terminal for the picker; pass --login NAME (repeat) or --all-logins
[exit 2]

$ agx usage --login nosuch
agx: unknown login nosuch
  agx login list
[exit 1]
```
