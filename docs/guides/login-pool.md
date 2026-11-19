# Share a pool of Claude accounts

A login pool lets one Claude Code session move between accounts on its own as quotas fill up or expire, with no proxy and no restart. You list your own accounts and your teammates' shared ones in a login of kind `pool`; each time Claude asks for a key, agx reads the members' usage, applies the rules below, and hands Claude the token of the member to use now.

The goal is to waste as little plan quota as possible. Weekly quota that is not used before the weekly reset is lost, so the account whose week ends soonest goes first. Your own accounts are used up almost fully; a teammate's account is borrowed only up to its caps, so the teammate always keeps the rest, and only when its quota is about to expire while one of yours is still usable.

## Before you start

- Claude Code. A pool needs an agent that asks for its key again while it runs (`apiKeyHelper`) and whose logins agx can probe; other agents are refused with the reason (`codex cannot use a login pool (…)`, exit 4).
- Two or more stored token logins for Claude ([Logins](../logins.md#adding-a-login)): plan tokens (`oauth-token`) or API keys, each from wherever its token lives (gopass, a `$(…)` command, `tokens.jsonc`). A key-helper login cannot be a member: agx cannot fetch its key.
- Nothing in the launch may set a variable that outranks the key helper (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`), as for [live login](live-login.md#before-you-start).

The example ([`examples/acct-login-pool`](../../examples/acct-login-pool/README.md)) has five fake logins: `alice` and `bob` are yours, `carol`, `derek` and `erin` are teammates'.

## Step 1: make the pool

With flags, then the numbers in `config.jsonc`:

```sh
agx login add --name auto --agent claude --kind pool --pool-own alice --pool-own bob --pool-shared carol --pool-shared derek --pool-shared erin
```

Or write the whole login by hand. Every key but `own` or `shared` may be left out; the values shown are the defaults, except the 95/98 caps of your own accounts:

<!-- snippet: examples/acct-login-pool/config.jsonc#pool -->
```jsonc
    "auto": {
      "agents": ["claude"],
      "kind": "pool",
      "pool": {
        "own": ["alice", "bob"],              // yours: used up almost fully
        "shared": ["carol", "derek", "erin"], // teammates': borrowed up to their caps
        "force": null,                        // a member name overrides every rule
        "shared_borrow_window": "48h",
        "short_unavailable_at_percent": 99,
        "defaults": { "short": 50, "week": 70 },
        "accounts": {
          "alice": { "short": 95, "week": 98 },
          "bob": { "short": 95, "week": 98 }
        },
        "usage_max_age": "60s",
        "key_ttl": "60s"
      }
    }
```

Your own accounts get caps a little under 100 (5-hour 95%, weekly 98%) so agx moves on before Claude gets a 429 for the account (see [When an account runs out mid-task](#when-an-account-runs-out-mid-task)). `agx config check` checks every member: it exists, serves Claude, is a token login, and is named once. Every key is in the [config reference](../config.md#login-pools).

## Step 2: see what it picks

`agx login pick` reads each member's usage (one small probe each, a few tokens of that plan, unless the [usage cache](#usage-and-the-cache) is younger than `usage_max_age`) and prints the pick, the rule that chose it, and every member:

<!-- snippet: examples/acct-login-pool/session.txt#pick -->
```sh
$ agx login pick --id auto
Use: carol (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
pool auto · claude · usage from cache (20s old) · borrow window 48h

Members (ready in pick order):
  carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
  alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 69%)
  derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
  bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr, room 37%)
  erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
```

`#N READY` is the pick order, `WAIT` is usable but held back by the borrow window, `skip:` lists every reason. `room` is the weekly cap minus the weekly use. `--json` carries the same, for scripts: `agx login pick --id auto --json --field data.selected.name` prints `carol`.

## Step 3: launch on the pool

```sh
agx run --login auto
```

A run on a pool is always live: Claude gets an `apiKeyHelper` setting that runs agx's hidden `agx credential`, and `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` from `key_ttl`, so it asks for a key again every minute. Each time, `agx credential` picks a member by the rules and prints that member's token to Claude alone. Make it the default with `agx login use --id auto --profile work`, or bind a folder to it (`agx project use --agent claude --login auto`).

A dry run probes nothing: it shows the members and the member the pool would pick now from the cache, of any age, or that it picks when Claude first asks:

<!-- snippet: examples/acct-login-pool/session.txt#run -->
```sh
$ agx run --login auto --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent     claude                                                                                                      from default_agent
profile   personal                                                                                                    from default profile of claude
home      ~/.claude                                                                                                   from profile personal
login     auto                                                                                                        from --login
binary    /usr/local/bin/claude version unknown                                                                       from no install metadata
folder    ~/projects/app                                                                                              from current folder
note      login pool auto: own alice, bob; shared carol, derek, erin; picks a member each time claude asks for a key  from login auto (pool)
          pick now: carol, from the cache (20s old); agx login pick --id auto probes                                  from login auto (pool)
          live login: switch with agx live use --id 20261119-100000-oc0f --login NAME (refresh 1m0s)                  from login auto (pool)
settings  apiKeyHelper                                                                                                from login auto (pool)
          delivered as file ~/.agx/runs/20261119-100000-oc0f/settings-20261119-100000-oc0f.json (0600)                from settings_delivery file
run       ~/.agx/runs/20261119-100000-oc0f                                                                            from AGX_RUN_ID=20261119-100000-oc0f
command   claude --settings ~/.agx/runs/20261119-100000-oc0f/settings-20261119-100000-oc0f.json                       from built by agentio
env       set AGX_RUN_ID=20261119-100000-oc0f                                                                         from launch environment
          set AGX_RUN_AGENT=claude                                                                                    from launch environment
          set AGX_RUN_PROFILE=personal                                                                                from launch environment
          set AGX_RUN_LOGIN=auto                                                                                      from launch environment
          set AGX_RUN_HOME=~/.claude                                                                                  from launch environment
          set CLAUDE_CODE_API_KEY_HELPER_TTL_MS=60000                                                                 from launch environment
note      installed Claude Code version unknown; flags not checked
```

`agx live list` shows the run; its `NEXT` is the pool and `IN USE` the member Claude got last.

## Step 4: steer it while sessions run

agx reads `config.jsonc` on every key request, so a change applies from the next refresh, without restarting anything:

- Force one member for every session: `agx login edit --id auto --pool-force alice` (back with `--pool-force ''`). Every rule is ignored, even a member at its limit; the rules it breaks are warnings.
- Force one member for one run only: `agx live use --id RUN --login alice` (back to the pool with `agx live use --id RUN --start-login`).
- Leave a member out: `"accounts": { "derek": { "enabled": false } }`. It is not probed any more.
- Any cap, list or window change.

`agx login pick --id auto --force alice` shows what a force would do without changing anything.

## The rules

1. **Force.** `--force` (one `login pick` or `login token` run), else the pool's `force`, wins outright and every rule below is ignored. Each rule the forced member breaks is printed as a warning. Forcing a login without a usage reading is an error (exit 1). Forcing a login listed in neither group works, checked against the default caps, with the warning `not listed in own or shared`.
2. **Usable.** A member is skipped when any of these holds, and every reason is listed: disabled in config; no usage reading; status is not `ok` (`limit-reached`, `rejected`, `secret`, `network`, …); 5-hour use at or over `short_unavailable_at_percent` (99), even when the status is `ok`; 5-hour use at or over its 5-hour cap; weekly use at or over its weekly cap. Caps are exclusive: under the cap is usable. Caps count the account's total use: at 60% of a 70% weekly cap you can use 10%, and the teammate keeps 30%. A window the probe does not report is not checked.
3. **Pool.** While any own member is usable, the pool is the usable own members plus the usable shared members whose weekly reset is within `shared_borrow_window`; the other usable shared members wait. With no own member usable, the pool is every usable shared member.
4. **Order.** The pool is sorted by weekly reset, soonest first. An unknown reset, or one that has already passed (stale data, noted on the member), sorts last. Ties keep config order, own before shared.
5. **Pick.** The first of the pool. An empty pool picks nothing: `agx login pick` and `agx login token` exit 1 with error code `auto-pick` and each member's reasons, and the key helper fails, so Claude's request fails with that line.

Caps always apply: a reset coming soon moves a member up the order, it never lifts a cap. Examples with the example's names:

| Situation | Pick | Why |
| --- | --- | --- |
| alice (own) week 29%, resets in 6 days; carol (shared) week 26%, resets in 38 h | carol | her 44% of room is lost in 38 h; alice can wait |
| alice (own) week 29%, 6 days; derek (shared) week 15%, resets in 4 days | alice | derek is outside the 48 h window: `WAIT` |
| alice (own) week 29%, 6 days; erin (shared) 5h 55%, week resets in 3 h | alice | erin is over her 5-hour cap of 50%, even on reset day |
| alice and bob limit-reached; carol week 1% in 7 days, derek week 3% in 6 days | derek | with no own member usable the window does not apply; soonest reset wins |

The 48 h window is fixed: it does not know how fast you use quota, so if you borrow a lot an own account can still end its week with room left. Lower `shared_borrow_window` then (`0s` borrows only when none of yours is usable). There is no daily cap: the probe reports no daily window.

## Usage and the cache

agx reads a member's usage with the same small probe `agx login check` and `--auto-login` use (cost Q). Every pick shares one cache, `~/.agx/cache/login-usage.jsonc` (0600, in a 0700 folder: it holds account labels, never a token), written atomically under a lock so several sessions never read half a file:

- A member's reading younger than `usage_max_age` (60 s) is reused; older ones are probed again, in parallel. So however many sessions run, each member is probed about once a minute.
- A probe that fails for a passing reason (no network, a 429 without usage, an unexpected answer) falls back to that member's last cached reading, with a warning on stderr (never on stdout, which Claude reads); the next request probes again.
- `agx login pick --cached` uses the cache only, of any age, and probes nothing; `--max-age 10s` asks for fresher readings once.

With a `log` path, every key request appends one line, so you can watch the switching (`tail -f`):

```text
2026-11-19T10:00:47Z mode=credential pool=auto picked=carol group=shared forced=false source="live probe"
2026-11-19T10:01:12Z mode=credential pool=auto picked=carol group=shared forced=false source="cache (25s old)"
```

## How long a switch takes

Up to about two minutes plus one request: the cache can be up to `usage_max_age` old, Claude keeps a key for `key_ttl`, and the request that fetches a new key still goes out with the old one (Claude refreshes the key in the background). Lower both for faster switching; each refresh past the cache age probes every enabled member.

## When an account runs out mid-task

agx never picks a member that is at its limit or over its cap, but a member can run out during a task, before the next refresh. Then:

1. Claude's request fails with `API Error: Request rejected (429) · This request would exceed your account's rate limit`, and the task stops. Claude does not retry it on another key by itself.
2. The session itself is fine: no restart. Type `continue`.
3. The first `continue` can fail the same way: it is the request that fetches the new key, and it still uses the old one. The next `continue` runs on the new account with the task's full context.

To make this rare, give your own members caps a little under 100 (95 and 98), so agx moves on while the account still has room for the refresh delay; a shorter `key_ttl` and `usage_max_age` shrink the window further.

## Pitfalls

| Gotcha | What you see | Why | What to do |
| --- | --- | --- | --- |
| A rejected request is not retried | the task stops with a 429 | Claude Code does not re-run the key helper on a 429 | type `continue` (maybe twice) |
| A new key applies one request late | the request right after a switch still fails | Claude refreshes the key in the background | expect one extra failed request after a switch |
| No switch while idle | the old member is kept for a while | Claude asks for a key only when it is about to call the API and the TTL has passed | nothing; it switches on the next request |
| claude.ai connectors warning | `claude.ai connectors are disabled because ANTHROPIC_API_KEY or another auth source is set` | a helper key counts as another auth source | expected; connectors do not load in that session |
| Nothing usable | Claude's request fails with `pool auto: no member is usable: …` | the helper exits 1 with no key | `agx login pick --id auto` shows why; force a member if you must go on |
| Forced member at its limit | every request is a 429 | force ignores every rule | clear it: `agx login edit --id auto --pool-force ''` |
| A teammate's account is picked before yours | `Use: carol (shared)` while yours have room | her weekly quota expires within the window; yours can wait | intended; `"shared_borrow_window": "0s"` borrows only when none of yours is usable |
| Teammate caps can be overshot | a teammate's 5-hour use goes past 50% | usage is checked only when Claude asks for a key | lower the cap for a buffer (40) |
| `WAIT` is not `skip` | `derek [shared] WAIT` | usable, held back by the window while one of yours is usable | nothing; it moves up as its reset nears |
| A just-limited member is picked | picked, then a 429 | the cache can be up to `usage_max_age` old | covered by the 95/98 caps, or lower `usage_max_age` |
| Probes fail | `warning: probe of alice failed (network: …); using its cached usage from 2m0s ago` | no network, or the endpoint is busy | check `agx login check --id alice`; picks may be stale meanwhile |
| Stale data | a member noted `weekly reset time has passed; usage data is stale` | a `--cached` reading older than the member's weekly reset | run without `--cached` |
| `--cached` never refreshes | the same answer every time | it reads the cache only | use `--max-age` in scripts |
| The cache holds emails | — | account labels are kept with each reading | it is 0600; do not share it |

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-pool$'
```

[`examples/acct-login-pool`](../../examples/acct-login-pool/README.md) runs every step above on fake logins with a seeded cache, so nothing is probed.

## Related

- [Live login](live-login.md): switch a session by hand; a pool run is a live run whose next login is the pool.
- [Let agx pick by headroom](auto-pick.md): `--auto-login` picks one login at launch by the most headroom; a pool keeps picking while the session runs, by the rules above.
- [`agx login pick`](../commands/login.md#agx-login-pick) and [`agx login token`](../commands/login.md#agx-login-token) in the command reference.
