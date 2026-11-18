# Usage: live, cached or probed

`agx usage` can read plan usage three ways: live from the vendor's usage endpoint (the default), from the reading the agent left on disk (`--cached`), or by probing each stored login's token (`--login-probes`, `--login`, `--all-logins`, `--login-picker`). They differ in cost, freshness and what they can see. This page helps you pick one. The steps are in [Plan usage](../guides/usage.md), every flag in the [usage reference](../commands/usage.md).

## Short answer

| You want | Use | Cost class | Network |
|---|---|---|---|
| the real numbers for every home, now | `agx usage` | N: one request per home | yes |
| something for a status bar or a prompt that runs every few seconds | `agx usage --cached`, and accept that Claude rows may be empty or old | R: reads files only | no |
| the quota of plan tokens you stored as logins (not of homes) | `agx usage --login-probes` (homes plus logins), `--login NAME`, `--all-logins` | Q: each probe spends a few tokens | yes |
| a launch on whatever has the most headroom | `agx run --auto-profile` (homes) or `--auto-login` (stored logins) | N or Q, once per launch | yes |
| plan name, subscription status and start date | add `--plan` | as the rows | as the rows |

The cost classes are in [Output and exit codes](../output.md); `agx help --json` gives each command's class.

## What each agent can do

| Agent | live (`usage`) | cached (`--cached`) | login probes |
|---|---|---|---|
| Claude Code | yes | only a reading an older Claude Code left; Claude Code 2.1.323 and later no longer store one | yes, for its stored token logins |
| Codex | yes | yes: the last rate-limit snapshot in its newest session files | no (Codex has no token login) |
| Gemini CLI | no | no | no |
| pi | no | no | no |
| a config-defined agent | no | no | no |

A row agx cannot read says why, so one unsupported home never hides the others.

## Claude Code no longer writes a cached reading

Claude Code 2.1.323 and later do not store or refresh the usage reading that `--cached` reads, and nothing on disk replaces it. On a current install `--cached` therefore shows `no-cache` for a Claude home that never had a reading, and `stale` for one whose old reading is more than a day old (its windows are kept, with the age, so you can judge them; it is not a failure and does not change the exit code). Codex still writes its snapshot, so `--cached` stays useful for Codex.

<!-- snippet: examples/choose-usage/session.txt#cached -->
```sh
$ agx usage --cached
personal  personal · claude  ~/.claude
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

work  work · claude  ~/.claude-work  cached 381 d ago
  Current session    █████████████░░░░░░░░░░░░░░░░░   42% used  resets now
  Weekly limits
  All models         █████░░░░░░░░░░░░░░░░░░░░░░░░░   17% used  resets now
  stale: reading is 381d old; drop --cached for a live read (Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage)

codex  codex · codex  ~/.codex
  no cache: no reading on disk yet; run the agent once, or drop --cached

gem  gem · gemini  ~/.gemini
  unsupported: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
```

The elided lines are the `work` home's old reading: its windows, its age and the same note. As data:

<!-- snippet: examples/choose-usage/session.txt#stale -->
```sh
$ agx usage --cached --profile work --json --field data.0.status
stale
```

So for Claude, use live usage. Keep `--cached` for Codex, or for a status bar that must never make a request and can live with "no reading".

## If this, then that

- If you check quota by hand a few times a day, run plain `agx usage`.
- If a status bar or shell prompt polls, use `--cached` on Codex homes, and poll live usage for Claude at most every few minutes: usage endpoints rate-limit. A home that answers 429 shows `rate-limited` and the other rows still show.
- If you keep several Claude plan tokens as logins (one home, many accounts), homes alone do not tell you their quota: add `--login-probes`, or name them with `--login`. Each probe spends a few tokens of that account.
- If you want the launch itself to choose, use `--auto-profile` or `--auto-login` instead of reading the table yourself.

## Pitfalls

- `--cached` and any probe flag are refused together, because one promises no network and the other spends quota:

<!-- snippet: examples/choose-usage/session.txt#conflict -->
```sh
$ agx usage --cached --login-probes
agx: --cached reads disk only and --login-probes probes logins (spends quota); pick one
[exit 2]
```

- Live usage for an agent that has none is a row with its reason, not a request:

<!-- snippet: examples/choose-usage/session.txt#unsupported -->
```sh
$ agx usage --agent gemini
gem  gem · gemini  ~/.gemini
  unsupported: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
```

- A home whose every profile has `"billing": "api"` bills per token: it shows `per-token` without a request, and `--auto-profile` never picks it for headroom.
- When some rows fail and others answer, agx prints what it has and exits 3 (`error.code` `partial` in `--json`). Scripts should treat 3 as "read the rows".

## Related

- Guide: [Logins](../logins.md) (the logins probes read), [Output and exit codes](../output.md) (cost classes, exit 3), [Plan usage](../guides/usage.md), [Let agx pick by headroom](../guides/auto-pick.md).
- Commands: [`agx usage`](../commands/usage.md).
- Example: [choose-usage](../../examples/choose-usage/README.md), [acct-usage-cached](../../examples/acct-usage-cached/README.md), [acct-auto-pick](../../examples/acct-auto-pick/README.md).
- Other decisions: [Where to set the profile and login](where-to-set.md) (`--auto-profile`, `--auto-login`), [Which login kind](login-kind.md).
