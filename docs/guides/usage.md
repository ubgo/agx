# Plan usage

`agx usage` shows how much of each plan is used: for every configured home, the five-hour session window and the weekly windows, how full each is and when it resets. With the login flags it also probes stored logins, so you see the quota of a plan token you keep in gopass, not only of the homes. One table answers "which account still has room".

You want this before a long session, when choosing which account to run on, and in a status bar or prompt.

## Before you start

- Profiles for your homes ([A second account](second-account.md)). Only configured homes are read; a home on disk that no profile uses is named in a warning.
- Each home logged in once with the agent's own login (`claude /login`), since live usage is read with that login.
- Which agents have plan usage:

| Agent | Live usage | Usage on disk (`--cached`) | Login probes |
|---|---|---|---|
| Claude Code | yes | only an old reading, see below | yes (plan tokens) |
| Codex | yes | yes | no: no token login |
| Gemini CLI | no | no | no |
| pi, custom agents | no: no plan | no | no |

- The cost of each mode, shown in every command's help: live usage is one request per home (cost N); `--cached` reads files only (cost R); the login flags send a minimal request with each login's token (cost Q: a few tokens of each plan). Usage endpoints rate-limit, so agx makes one request per home per call and never retries in a loop.

## Step 1: read live usage

```sh
agx usage
agx usage --profile work --profile personal
agx usage --agent claude
```

Each home is one block: its name, agent, home, source (`live`, or `cached` with the reading's age) and status, then one line per window with its percent and reset time. A home shared by several profiles is read once and lists them all (`profiles kimi, personal`). This is the shape, from the tested example's old Claude reading (its age grows every day, so this block is not checked):

```text
personal  claude           ~/.claude  cached 20d ago  stale: reading is 20d old; drop --cached for a live read (Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage)  profiles kimi, personal
          Current session  6%         resets Wed 28 Oct 13:00
          All models       42%        resets Wed 4 Nov 06:00
```

The `status` of a row is one of: `ok`, `limit-reached` (a window at 100%), `per-token` (every profile on the home has `"billing": "api"`, so there are no windows), `unsupported` (the agent has no usage; the reason says why), `no-cache` and `stale` (only with `--cached`), or a failure: `rate-limited`, `expired`, `not-logged-in`, `rejected`, `scope-limited`, `home-missing`, `secret`, `network`, `error`.

## Step 2: add the plan

`--plan` adds the plan name, subscription status and start date per home. Live, that is one more request per home (cost N); with `--cached` it is the plan label stored in the home (cost R):

<!-- snippet: examples/acct-usage-cached/session.txt#stale -->
```sh
$ agx usage --cached --profile personal --json --field data.*.status
["stale"]

$ agx usage --cached --profile personal --json --field data.0.windows --tz UTC
[{"class":"short","label":"Current session","percent":6,"resets_at":"2026-10-28T12:59:59.902135Z","reset":"Wed 28 Oct 13:00"},{"class":"week","label":"All models","percent":42,"resets_at":"2026-11-04T05:59:59.902151Z","reset":"Wed 4 Nov 06:00"}]

$ agx usage --cached --plan --profile personal --json --field data.0.plan
{"plan":"Max","source":"cached"}
```

## Step 3: a compact table, sorted

`--table` prints one row per home or login, with `LEFT` (what is left before the first cap), the five-hour and weekly percents and resets, the source and the status. `--sort` orders the rows by `login` (the name), `left`, `5h`, `5h-reset`, `weekly`, `weekly-reset`, `day` (the weekday the weekly window resets, so accounts group by reset day) or `status`; prefix `-` for high to low, repeat for tie-breaks. `--tz` reads reset times in another IANA zone (the default is this machine's).

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

```sh
agx usage --table --sort -left
agx usage --table --sort day --sort 5h --tz Europe/Berlin
```

## Step 4: change how times read

Reset times read as a duration by default ("resets in 3 hr 10 min"), as agx v0.3 and claude.ai write them. `--time-format` picks another way for one run, in the bars and the table alike, for the five-hour, weekly and per-model windows; `--date-format` picks how the clock and date parts are written; `--tz` picks the zone. The output below comes from a reading whose windows reset today at 15:10, on Friday and in seven days, on a clock fixed at Wed 18 Nov 2026 12:00 UTC:

<!-- snippet: examples/acct-usage-times/session.txt#relative -->
```sh
$ agx usage --cached --profile personal
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets in 3 hr 10 min
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets in 2 d 4 hr
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 7 d 6 hr
```

`both` keeps the duration and adds when that is, in brackets: the clock time for a reset today, the weekday and clock for one in the next six days, and the whole date further out (a weekday a week away would read as today):

<!-- snippet: examples/acct-usage-times/session.txt#both -->
```sh
$ agx usage --cached --profile personal --time-format both
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets in 3 hr 10 min (15:10)
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets in 2 d 4 hr (Fri 16:30)
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 7 d 6 hr (Wed 25 Nov 18:00)

$ agx usage --cached --profile personal --time-format both --table
#  PROFILE   PROVIDER  ACCOUNT          5H USED  5H RESETS            WEEKLY USED  WEEKLY RESETS         RESET DAY  RESETS ON         SONNET WEEKLY USED  SONNET WEEKLY RESETS         LEFT  STATUS
1  personal  claude    you@example.com  13%      3 hr 10 min (15:10)  49%          2 d 4 hr (Fri 16:30)  Fri        Fri 20 Nov 16:30  3%                  7 d 6 hr (Wed 25 Nov 18:00)  51%   ok
Weekly resets: Fri 1 (+49%)
```

`day` writes the weekday and clock, `absolute` the weekday, date and clock (`RESETS ON` in the table always uses the date format):

<!-- snippet: examples/acct-usage-times/session.txt#day-absolute -->
```sh
$ agx usage --cached --profile personal --time-format day
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets Wed 15:10
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets Fri 16:30
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets Wed 18:00

$ agx usage --cached --profile personal --time-format absolute
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets Wed 18 Nov 15:10
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets Fri 20 Nov 16:30
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets Wed 25 Nov 18:00
```

`--date-format` takes a preset (`iso`, `short`, `long` (the default for usage), `us`) or tokens: `YYYY`, `MM`, `MMM` (Nov), `DD`, `D`, `ddd` (Thu), `dddd` (Thursday), `HH`, `H`, `hh`, `h`, `mm`, `ss`, `a` (am/pm), `A` (AM/PM), with any punctuation between them and words in `[brackets]`. The clock part of a format (from its first to its last time token) is what `day` and `both` show, so `us` turns them to the 12-hour clock; durations never change:

<!-- snippet: examples/acct-usage-times/session.txt#formats -->
```sh
$ agx usage --cached --profile personal --time-format both --date-format us
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets in 3 hr 10 min (3:10 PM)
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets in 2 d 4 hr (Fri 4:30 PM)
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets in 7 d 6 hr (Nov 25 6:00 PM)

$ agx usage --cached --profile personal --time-format absolute --date-format iso
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets 2026-11-18 15:10
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets 2026-11-20 16:30
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets 2026-11-25 18:00

$ agx usage --cached --profile personal --time-format absolute --date-format "dddd D MMM [at] h:mm a"
you@example.com  Max  personal · claude  ~/.claude  cached 30 min ago
  Current session    ████░░░░░░░░░░░░░░░░░░░░░░░░░░   13% used  resets Wednesday 18 Nov at 3:10 pm
  Weekly limits
  All models         ███████████████░░░░░░░░░░░░░░░   49% used  resets Friday 20 Nov at 4:30 pm
  Sonnet             █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░    3% used  resets Wednesday 25 Nov at 6:00 pm
```

To keep a choice, set it in config: `usage.time_format` and `usage.date_format` for `agx usage`, or the top-level `date_format` for every screen that prints a date ([Config: Usage](../config.md#usage)). The flags still win for one run:

<!-- snippet: examples/acct-usage-times/session.txt#config -->
```sh
$ agx config set --key usage.time_format --value both
usage.time_format = both

$ agx config set --key date_format --value us
date_format = us

$ agx usage --cached --profile personal --table
#  PROFILE   PROVIDER  ACCOUNT          5H USED  5H RESETS              WEEKLY USED  WEEKLY RESETS           RESET DAY  RESETS ON       SONNET WEEKLY USED  SONNET WEEKLY RESETS       LEFT  STATUS
1  personal  claude    you@example.com  13%      3 hr 10 min (3:10 PM)  49%          2 d 4 hr (Fri 4:30 PM)  Fri        Nov 20 4:30 PM  3%                  7 d 6 hr (Nov 25 6:00 PM)  51%   ok
Weekly resets: Fri 1 (+49%)

$ agx usage --cached --profile personal --table --time-format relative --date-format long
#  PROFILE   PROVIDER  ACCOUNT          5H USED  5H RESETS    WEEKLY USED  WEEKLY RESETS  RESET DAY  RESETS ON         SONNET WEEKLY USED  SONNET WEEKLY RESETS  LEFT  STATUS
1  personal  claude    you@example.com  13%      3 hr 10 min  49%          2 d 4 hr       Fri        Fri 20 Nov 16:30  3%                  7 d 6 hr              51%   ok
Weekly resets: Fri 1 (+49%)
```

A format agx cannot read is refused with the list of tokens and presets (exit 2). `MM` is the month and `mm` the minutes, so `HH:MM:SS` stops at `SS`:

<!-- snippet: examples/acct-usage-times/session.txt#refusals -->
```sh
$ agx usage --cached --profile personal --date-format "YYYY-MM-DD HH:MM:SS"
agx: --date-format "YYYY-MM-DD HH:MM:SS": unknown token "SS"; tokens: YYYY MMM MM DD D dddd ddd HH H hh h mm ss a A, literal text in [brackets]; presets: iso, short, long, us
[exit 2]

$ agx usage --cached --profile personal --date-format ""
agx: --date-format "": empty format; tokens: YYYY MMM MM DD D dddd ddd HH H hh h mm ss a A, literal text in [brackets]; presets: iso, short, long, us
[exit 2]

$ agx usage --cached --profile personal --time-format clock
agx: --time-format "clock": use one of relative, day, absolute, both
[exit 2]
```

## Step 5: probe stored logins

A stored plan token ([Logins](../logins.md)) is not tied to a home, so live usage cannot read it. The login flags probe it instead: one minimal request with its token, whose rate-limit headers carry the windows.

| Flag | Rows |
|---|---|
| `--login-probes` | every home, plus one probed row per stored login |
| `--login NAME` | only these logins (repeat), no homes |
| `--all-logins` | every stored login, no homes |
| `--login-picker` | tick logins in a checklist first (needs a terminal) |

```sh
agx usage --login carol --login derek --plan
agx usage --all-logins --table --sort -left
```

Each probe spends a few tokens of that plan and resolves the login's reference first (running gopass or a command). An API key or a key helper bills per token and is shown as `per-token`, never probed.

## Cached readings and Claude Code 2.1.323

`--cached` reads what the agent left on disk: no network, no quota, any number of times. Older Claude Code versions kept a copy of their last usage reading in the home's account file. Claude Code 2.1.323 and later no longer write that copy, and never refresh an old one. So for Claude:

- a home that only ran 2.1.323 or later has no reading: `no-cache`, with that reason;
- a home with a reading from an older version shows it as `stale` once it is more than a day old: the windows are kept (they may be far from the truth, and their reset times may be in the past), the status says how old it is, and it does not count as a failure.

Codex reads its own files (`no-cache` until Codex has run in that home); Gemini CLI keeps nothing on disk:

<!-- snippet: examples/acct-usage-cached/session.txt#notes -->
```sh
$ agx usage --cached --profile work --profile codex --profile gem
work  work · claude  ~/.claude-work
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

codex  codex · codex  ~/.codex
  no cache: no reading on disk yet; run the agent once, or drop --cached

gem  gem · gemini  ~/.gemini
  unsupported: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
```

For current Claude numbers, read live usage. In a status bar or shell prompt, call it for one profile at a modest interval (every few minutes, not every prompt), and pick the fields you need:

```sh
agx usage --profile work --json --field data.0.left
agx usage --profile work --json --field data.0.windows
```

## Try it

Live usage needs a real account and the network, so the tested example reads from disk ([`examples/acct-usage-cached`](../../examples/acct-usage-cached/README.md)):

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-usage-cached$'
```

A Claude home on disk that no profile uses is named in a warning (on stderr, and under `warnings` in JSON), so a forgotten account is visible. It is shown only when you did not narrow the rows with `--profile`:

<!-- snippet: examples/acct-usage-cached/session.txt#footer -->
```sh
$ agx usage --cached --agent claude --json --field warnings
[{"code":"unconfigured-home","message":"claude home ~/.claude-client is not in config; agx profile import --discovered"}]
[exit 3]
```

## Pitfalls

- When some rows answer and some fail, the answering rows are shown and the exit code is 3; when none answer, it is 1 (the message names each failed row and its status):

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

- `--cached` with a login flag is refused (exit 2): it reads disk only, and a probe spends quota. `--login-picker` needs a terminal (exit 2). An unknown login or profile exits 1:

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

- `not-logged-in` on a live read means the home has no stored login of its own; run the agent there once. `scope-limited` means the home is logged in with a setup token that cannot read usage: add that token as a stored login and probe it with `--login`.
- A home the vendor answers with 429 shows `rate-limited`. Wait; do not retry in a loop.
- A row's numbers are the home's own login. A run that used `--login carol` on that home spent carol's plan, which only a login probe shows.

## Related

- [Usage reference](../commands/usage.md): every flag.
- [Let agx pick by headroom](auto-pick.md): `--auto-profile` and `--auto-login` choose from these numbers.
- [Logins](../logins.md): the stored logins the probes read.
- [Output](../output.md): `--json`, `--field` and exit codes.
- [`examples/acct-usage-cached`](../../examples/acct-usage-cached/README.md): the session this page shows.
- [Usage: live, cached or probed](../choosing/usage-live-or-cached.md): the decision guide.
