# Share a pool of Claude accounts

A login pool lets one Claude Code session move between accounts on its own as quotas fill up or expire, with no proxy and no restart. You list your own accounts and your teammates' shared ones in a login of kind `pool`; each time Claude asks for a key, agx reads the members' usage, applies the rules below, and hands Claude the token of the member to use now.

The goal is to waste as little plan quota as possible. Weekly quota that is not used before the weekly reset is lost, so the account whose week ends soonest goes first. Your own accounts are used up almost fully; a teammate's account is borrowed only up to its caps, so the teammate always keeps the rest, and only when its quota is about to expire while one of yours is still usable.

## It is optional: override each part

Nothing changes for anyone without a `pool` login: plain logins, an `api-key-helper` login with your own command, `--live-login` and `--auto-login` work exactly as before. A pool is a helper agx provides, and every part of it can be overridden:

| You want | Do | Applies to |
|---|---|---|
| one member, bypassing the pool | `agx run --login carol` (or a binding, a profile login, `AGX_LOGIN=carol`) | that run |
| one member for every key of one run | `agx run --login auto --pool-force carol` | that run |
| one member for every key of one run, its caps still enforced (no key while it is over one) | `agx run --login auto --pool-only carol` ([more](#pin-one-account-but-keep-its-limits)) | that run |
| one member for every key of every run, until you clear it | `agx login edit --id auto --pool-force carol` (clear: `--pool-force ''`) | every pool run, from its next key |
| a running session pinned to one member | `agx live use --id RUN --login carol` | that run |
| a pinned session back on the pool's rules | `agx live use --id RUN --login auto` (or `--start-login`) | that run |
| a running session kept on one member, caps enforced | `agx live use --id RUN --pool-only carol` (clear: `--login auto`) | that run |
| the pool for this terminal | `AGX_LOGIN=auto` or `AGX_LOGIN_CLAUDE=auto`, like any login | this terminal |
| what `pool pick` and `login token` would do with a force | `--pool-force carol` on either | one call |
| what `pool pick` and `login token` would do with a pin | `--pool-only carol` on either | one call |
| your own rules | a [picker command](#your-own-picker) of yours in the pool | every key request |
| your own key helper instead of a pool | an `api-key-helper` login: `agx login add --name mine --agent claude --kind api-key-helper --helper-command '/path/to/your-helper'`, then `agx run --login mine` | runs on that login |

Who decides a key, first match wins: a run's `--pool-force`, the pool's `force`, its `picker`, the built-in [rules](#the-rules). A run's `--pool-only` sets the first three aside and applies the rules to its one member. The dry run shows each pool decision with its source (a config key, `default`, `--pool-force`, the cache), like every other decision.

Every pool setting has a default: `shared_borrow_window` `48h`, `short_unavailable_at_percent` 99, `defaults` 5h 50% and week 70%, `usage_max_age` `60s`, `key_ttl` `60s`, no `force`, no `picker` (built-in rules), `picker.timeout` `10s`, no `log`. Each is in the [config reference](../config.md#login-pools).

## Recipes

Every way to use a pool, with the names of the [example](../../examples/acct-login-pool/README.md): your logins `alice` and `bob`, teammates' `carol`, `derek` and `erin`, and a pool `auto`.

1. **Switch between accounts by usage.** When to use it: the normal way; one session moves between accounts as quotas fill, using your own almost fully and teammates' only up to their caps.

   ```sh
   agx run --login auto
   ```

2. **Pin one account but keep its limits, for every run: a one-member pool.** When to use it: a binding, profile or alias should always stay on carol and never go over her caps.

   ```sh
   agx login add --name carol-only --agent claude --kind pool --pool-shared carol
   agx run --login carol-only
   ```

3. **The same for one session, with no extra pool.** When to use it: this run must stay on carol within her caps; the next one uses the pool as usual ([more](#pin-one-account-but-keep-its-limits)).

   ```sh
   agx run --login auto --pool-only carol
   agx run --login auto --pool-only carol --pool-only-week 50   # and stricter for this run
   ```

4. **Pin one account and ignore its limits.** When to use it: you must go on with carol whatever her usage (each rule she breaks is a warning).

   ```sh
   agx run --login auto --pool-force carol
   ```

5. **Change a running session.** When to use it: a session is already open (`agx live list` shows its RUN id).

   ```sh
   agx live use --id RUN --pool-only carol   # stay on carol, caps enforced
   agx live use --id RUN --login carol       # stay on carol, caps ignored
   agx live use --id RUN --login auto        # back to the pool's rules
   ```

6. **Leave one account out.** When to use it: derek must not be used until you take him back (`--pool-enable derek`).

   ```sh
   agx login edit --id auto --pool-disable derek
   ```

7. **Lower one account's caps.** When to use it: a teammate wants more of her quota kept for herself, for every run (`NAME=` removes the entry again).

   ```sh
   agx login edit --id auto --pool-cap-short erin=40 --pool-cap-week erin=60 --dry-run
   agx login edit --id auto --pool-cap-short erin=40 --pool-cap-week erin=60
   ```

8. **Which account is my session on?** When to use it: inside Claude, whose `/status` only says `apiKeyHelper`.

   ```sh
   ! agx whoami
   ```

9. **Why didn't it pick carol?** When to use it: the pick surprises you; every member is listed with `READY`, `WAIT` or `skip:` and every reason ([reading the report](#step-2-see-what-it-picks)).

   ```sh
   agx pool pick --id auto
   agx pool pick --id auto --pool-only carol   # just carol, with her reasons
   ```

| | Switches account | Caps enforced | At the cap |
|---|---|---|---|
| `agx run --login auto` | yes, by usage | yes | moves to another member; no key when none is usable |
| `--pool-only carol` | no | yes, the pool's; can be lowered for the run | no key until carol is under her caps |
| one-member pool `carol-only` | no | yes, that pool's | no key until carol is under her caps |
| `--pool-force carol` | no | no, each broken rule is a warning | Claude gets 429s from carol's account |
| `--login carol` (or `agx live use --login carol`) | no | no, never checked | Claude gets 429s from carol's account |

A one-member pool works like any pool, so `agx pool pick` shows it:

<!-- snippet: examples/acct-login-pool/session.txt#one-member -->
```sh
$ agx login add --name carol-only --agent claude --kind pool --pool-shared carol
added login carol-only (pool, pool 0 own, 1 shared)
  pool.shared  —  ->  carol

$ agx pool pick --id carol-only
Pool: carol-only (claude)
Use: carol (shared)
no own account is usable; soonest weekly reset among shared accounts
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)

$ agx run --login carol-only --dry-run --quiet --json --field data.live.start_login
carol-only
```

## Which agents

Claude Code only, today, and agx decides that from the agent's capabilities, not its name. A pool needs an agent that takes token logins and asks for its key again while it runs (Claude's `apiKeyHelper`, the capability [live login](live-login.md) uses), and whose logins agx can probe for usage. Codex reads its home's own login and has no token login or key helper; Gemini CLI and pi read their key once at start. A pool for any of them is refused with the agent's reason (`codex cannot use a login pool (…)`: exit 4 from `agx login add`, a config error at the line from `agx config check`). The selection rules themselves are agent-neutral (the public package `github.com/ubgo/agx/pool` is a pure function over usage readings), so another agent can be added when it gains both capabilities.

## Before you start

- Claude Code ([which agents](#which-agents)).
- Two or more stored token logins for Claude ([Logins](../logins.md#adding-a-login)): plan tokens (`oauth-token`) or API keys, each from wherever its token lives (gopass, a `$(…)` command, `tokens.jsonc`). A key-helper login cannot be a member: agx cannot fetch its key.
- Nothing in the launch may set a variable that outranks the key helper (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`), as for [live login](live-login.md#before-you-start).

The example ([`examples/acct-login-pool`](../../examples/acct-login-pool/README.md)) has five fake logins: `alice` and `bob` are yours, `carol`, `derek` and `erin` are teammates'.

## Step 1: make the pool

With flags: the members, then any number (each flag is in the [login reference](../commands/login.md#agx-login-add); `--dry-run` shows the keys first and writes nothing):

```sh
agx login add --name auto --agent claude --kind pool --pool-own alice --pool-own bob --pool-shared carol --pool-shared derek --pool-shared erin \
  --pool-cap-short alice=95 --pool-cap-week alice=98 --pool-cap-short bob=95 --pool-cap-week bob=98
```

Change any number later the same way, with `agx login edit --id auto` and the same flags. Each change prints its before and after; values that are already set report `no change` and write nothing:

<!-- snippet: examples/acct-login-pool/session.txt#caps -->
```sh
$ agx login edit --id team --pool-cap-short alice=95 --pool-cap-week alice=98 --pool-cap-short erin=60 --pool-disable derek --dry-run
would change login team (--dry-run)
  pool.accounts.alice.short    —     ->  95
  pool.accounts.alice.week     —     ->  98
  pool.accounts.erin.short     —     ->  60
  pool.accounts.derek.enabled  true  ->  false

$ agx login edit --id team --pool-cap-short alice=95 --pool-cap-week alice=98 --pool-cap-short erin=60 --pool-disable derek
changed login team (pool, pool 1 own, 2 shared)
  pool.accounts.alice.short    —     ->  95
  pool.accounts.alice.week     —     ->  98
  pool.accounts.erin.short     —     ->  60
  pool.accounts.derek.enabled  true  ->  false

$ agx login edit --id team --pool-cap-short alice=95 --pool-cap-week alice=98
login team: no change (every value given is already set)

$ agx config set --key logins.team.pool.defaults.week --value 80
logins.team.pool.defaults.week = 80

$ agx pool pick --id team
Pool: team (claude)
Use: erin (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)

Candidates (ready accounts in pick order):
- erin [shared] #1 READY (5h 55%, week 43%, resets in 3 hr 0 min, room 37%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 69%)
- derek [shared] skip: disabled in config; no usage reading (no windows, reset unknown)

$ agx login edit --id team --pool-enable derek --pool-cap-short erin= --pool-key-ttl 30s
changed login team (pool, pool 1 own, 2 shared)
  pool.accounts.erin.short     60     ->  —
  pool.accounts.derek.enabled  false  ->  true
  pool.key_ttl                 —      ->  30s
```

A bad value is refused before anything is written (exit 2), and so is a member the pool does not list:

<!-- snippet: examples/acct-login-pool/session.txt#cap-refusals -->
```sh
$ agx login edit --id team --pool-cap-week erin=101
agx: --pool-cap-week erin=101: 101 is not a number from 0 to 100
[exit 2]

$ agx login edit --id team --pool-disable carol
agx: --pool-disable carol: carol is not a member of pool team (members: alice, derek, erin)
[exit 2]

$ agx config set --key logins.team.pool.accounts.erin.week --value 101
agx: --key logins.team.pool.accounts.erin.week takes a number from 0 to 100, not "101"
[exit 2]

$ agx config set --key logins.team.pool.accounts.erin.week --value high
agx: --key logins.team.pool.accounts.erin.week takes a number from 0 to 100, not "high"
[exit 2]
```

`agx config set` also sets one number (`agx config set --key logins.auto.pool.accounts.alice.short --value 95`), typed by the config schema, so a cap takes a number from 0 to 100 even on a member entry that is not in the file yet.

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
    },
```

Your own accounts get caps a little under 100 (5-hour 95%, weekly 98%) so agx moves on before Claude gets a 429 for the account (see [When an account runs out mid-task](#when-an-account-runs-out-mid-task)). `agx config check` checks every member: it exists, serves Claude, is a token login, and is named once. Every key is in the [config reference](../config.md#login-pools).

`agx pool list` lists your pools, and `agx pool show --id auto` prints one pool's settings with where each value comes from (`config`, the pool's `defaults`, or agx's `default`), from config alone:

<!-- snippet: examples/acct-login-pool/session.txt#pool-list -->
```sh
$ agx pool list
NAME   AGENTS  OWN        SHARED            FORCE  PICKER
auto   claude  alice,bob  carol,derek,erin  —      —
picky  claude  alice,bob  carol             —      sh $HOME/picker.sh
```

## Step 2: see what it picks

`agx pool pick` names the pool and its agent (`Pool: auto (claude)`), reads each member's usage (one small probe each, a few tokens of that plan, unless the [usage cache](#usage-and-the-cache) is younger than `usage_max_age`) and prints the pick, the rule that chose it, and every member:

<!-- snippet: examples/acct-login-pool/session.txt#pick -->
```sh
$ agx pool pick --id auto
Pool: auto (claude)
Use: carol (shared)
shared account resets within 48h, before any usable own account; using its room before it is lost
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 69%)
- derek [shared] WAIT (5h 0%, week 15%, resets in 4 d 0 hr, room 55%) (shared reset not within 48h while an own account is usable)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 95%) (5h 100%, week 61%, resets in 3 d 0 hr, room 37%)
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
```

`#N READY` is the pick order, `WAIT` is usable but held back by the borrow window, `skip:` lists every reason. `room` is the weekly cap minus the weekly use. `--json` carries the same, for scripts: `agx pool pick --id auto --json --field data.selected.name` prints `carol`.

## Step 3: launch on the pool

```sh
agx run --login auto
```

A run on a pool is always live: Claude gets an `apiKeyHelper` setting that runs agx's hidden `agx credential`, and `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` from `key_ttl`, so it asks for a key again every minute. Each time, `agx credential` picks a member by the rules and prints that member's token to Claude alone. Make it the default with `agx login use --id auto --profile work`, or bind a folder to it (`agx project use --agent claude --login auto`).

Compare with a fixed account, `agx run --login carol`, which stays on carol until you stop it. What happens after `agx run --login auto`:

```text
1. agx reads the "auto" login: kind pool, with members and rules, no token
2. agx starts Claude with the setting apiKeyHelper = "<agx> credential" and CLAUDE_CODE_API_KEY_HELPER_TTL_MS = key_ttl
3. Claude runs the helper when it needs a key
4. agx credential reads the members' usage (cache or probe), picks a member (e.g. carol) and prints its token
5. Claude uses that token
6. after key_ttl, on its next request, Claude runs the helper again and may get another member
```

The whole connection is the pool login in `~/.agx/config.jsonc`; `agx login show --id auto` shows it, and `agx login edit --id auto` (or `agx config set`, or an edit of the file) changes it from the next key request.

A dry run probes nothing: it shows the members and the member the pool would pick now from the cache, of any age, or that it picks when Claude first asks:

<!-- snippet: examples/acct-login-pool/session.txt#run -->
```sh
$ agx run --login auto --dry-run
agx: agent claude (default_agent)
agx: warning: installed Claude Code version unknown; flags not checked
agent     claude                                                                                        from default_agent
profile   personal                                                                                      from default profile of claude
home      ~/.claude                                                                                     from profile personal
login     auto                                                                                          from --login
binary    /usr/local/bin/claude version unknown                                                         from no install metadata
folder    ~/projects/app                                                                                from current folder
pool      auto (claude): own alice, bob; shared carol, derek, erin                                      from config logins.auto.pool
          pick: the built-in rules at each key request                                                  from default
          key ttl 1m0s                                                                                  from config logins.auto.pool.key_ttl
          usage max age 1m0s                                                                            from config logins.auto.pool.usage_max_age
          shared borrow window 48h                                                                      from config logins.auto.pool.shared_borrow_window
          5h cutoff 99%                                                                                 from config logins.auto.pool.short_unavailable_at_percent
          default caps 5h 50%, week 70%                                                                 from config logins.auto.pool.defaults
          member entries: 2                                                                             from config logins.auto.pool.accounts
          pick now: carol (rules), from the cache (20s old); agx pool pick --id auto probes            from login-usage cache
note      live login: switch with agx live use --id 20261119-100000-19p7 --login NAME (refresh 1m0s)    from login auto (pool)
settings  apiKeyHelper                                                                                  from login auto (pool)
          delivered as file ~/.agx/runs/20261119-100000-19p7/settings-20261119-100000-19p7.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261119-100000-19p7                                                              from AGX_RUN_ID=20261119-100000-19p7
command   claude --settings ~/.agx/runs/20261119-100000-19p7/settings-20261119-100000-19p7.json         from built by agentio
env       set AGX_RUN_ID=20261119-100000-19p7                                                           from launch environment
          set AGX_RUN_AGENT=claude                                                                      from launch environment
          set AGX_RUN_PROFILE=personal                                                                  from launch environment
          set AGX_RUN_LOGIN=auto                                                                        from launch environment
          set AGX_RUN_HOME=~/.claude                                                                    from launch environment
          set CLAUDE_CODE_API_KEY_HELPER_TTL_MS=60000                                                   from launch environment
note      installed Claude Code version unknown; flags not checked
```

`agx live list` shows the run; its `NEXT` is the pool and `IN USE` the member Claude got last.

Which account is my session on? Run `! agx whoami` inside Claude: it shows `login:  auto (pool)`, `in use:` (the member whose key Claude got last), `only:` on a run pinned with `--pool-only`, and `next:` when a switch is pending. /status only says apiKeyHelper, and Claude's own guess reads the keychain login, not the key it uses.

## Step 4: steer it while sessions run

agx reads `config.jsonc` on every key request, so a change applies from the next refresh, without restarting anything:

- Force one member for every session: `agx login edit --id auto --pool-force alice` (back with `--pool-force ''`). Every rule is ignored, even a member at its limit; the rules it breaks are warnings.
- Force one member for one run only: `agx live use --id RUN --login alice` (back to the pool with `agx live use --id RUN --start-login`).
- Keep one run on one member within its caps: `agx live use --id RUN --pool-only alice` (add `--pool-only-week N` for a stricter cap; back with `--login auto`). See [Pin one account but keep its limits](#pin-one-account-but-keep-its-limits).
- Leave a member out: `agx login edit --id auto --pool-disable derek` (back with `--pool-enable derek`). It is not probed any more.
- Any cap, list or window change: `agx login edit --id auto --pool-cap-short NAME=N`, `--pool-cap-week NAME=N`, `--pool-default-short N`, `--pool-default-week N`, `--pool-short-cutoff N`, `--pool-borrow-window 24h`, `--pool-usage-max-age 30s`, `--pool-key-ttl 30s`, `--pool-own`/`--pool-shared` (an empty value puts a key back to its default). Add `--dry-run` to see the change first.

`agx pool pick --id auto --pool-force alice` shows what a force would do without changing anything, and `agx pool pick --id auto --pool-only alice` what a pin would.

## Pin one account but keep its limits

Sometimes a session must stay on one account, say a teammate's carol, but must never go over the caps you agreed with her. `--pool-only` does that: every key of the run is carol's while the pool's rules allow it (her caps, per-member or the pool's `defaults`, the 5-hour cutoff, `enabled`, her status), and while they do not, agx hands out no key at all, so Claude's request fails instead of moving to another member or going over the cap. It starts working again by itself once her usage falls under the cap (or the window resets), at the next key request.

```sh
agx run --login auto --pool-only carol                        # carol, within her caps
agx run --login auto --pool-only carol --pool-only-week 50    # and a stricter weekly cap for this run
agx live use --id RUN --pool-only carol                        # the same for a session already running
agx live use --id RUN --login auto                             # back to the pool's rules
```

`--pool-only-short N` and `--pool-only-week N` lower the member's caps for that run only; they never raise one (`--pool-only-week 80 is above carol's week cap 70; it may only lower it`, exit 2, or 125 from a launch). To lift a cap, change the pool (`agx login edit --id auto --pool-cap-week carol=80`). The name must be a member of the pool, and a disabled member is refused at launch: it would never get a key.

When carol reaches a cap, the key helper prints nothing on stdout and one line on stderr, `carol: week is 70% (cap 70%)`, and exits 1; `! agx whoami` shows `only:  carol (caps enforced)` and `in use:  none (carol over cap: week is 70% (cap 70%))`. `agx pool pick --id auto --pool-only carol` shows the same report the pool uses, restricted to carol:

<!-- snippet: examples/acct-login-pool/session.txt#only -->
```sh
$ agx pool pick --id auto --pool-only derek
Pool: auto (claude)
Use: derek (shared)
the only member of this run (--pool-only); its caps and the pool's rules still apply
Source: cache (20s old)

Candidates (ready accounts in pick order):
- derek [shared] #1 READY (5h 0%, week 15%, resets in 4 d 0 hr, room 55%)

$ agx pool pick --id auto --pool-only erin
agx: erin: 5h is 55% (cap 50%)
Pool: auto (claude)
No configured account is currently usable.
Source: cache (20s old)

Candidates (ready accounts in pick order):
- erin [shared] skip: 5h is 55% (cap 50%) (5h 55%, week 43%, resets in 3 hr 0 min, room 27%)
[exit 1]

$ agx pool pick --id auto --pool-only derek --pool-only-week 15
agx: derek: week is 15% (cap 15%)
Pool: auto (claude)
No configured account is currently usable.
Source: cache (20s old)

Candidates (ready accounts in pick order):
- derek [shared] skip: week is 15% (cap 15%) (5h 0%, week 15%, resets in 4 d 0 hr, room 0%)
[exit 1]

$ agx run --login auto --pool-only derek --dry-run --quiet --json --field data.live.pool_only
derek
```

A running session pinned live, from the example: derek first, then erin, who is over her 5-hour cap, so the next key request gets no key and whoami says why; `--login auto` hands the run back to the pool:

<!-- snippet: examples/acct-login-pool/session.txt#live-only -->
```sh
$ agx live use --id 20261119-095000-p7q2 --pool-only derek
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      —
next        auto
only        derek (caps enforced)
in use      carol
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ AGX_RUN_ID=20261119-095000-p7q2 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=personal AGX_RUN_LOGIN=auto AGX_RUN_HOME=~/.claude agx whoami
run:      20261119-095000-p7q2
agent:    claude
profile:  personal
login:    auto (pool)
home:     ~/.claude
only:     derek (caps enforced)
in use:   carol
next:     derek

$ agx live use --id 20261119-095000-p7q2 --pool-only erin
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      —
next        auto
only        erin (caps enforced)
in use      carol
liveness    unknown (process info unreadable: run.jsonc records no process id)

$ AGX_RUN_ID=20261119-095000-p7q2 agx credential --json --field data.login
agx: erin: 5h is 55% (cap 50%)
[exit 1]

$ AGX_RUN_ID=20261119-095000-p7q2 AGX_RUN_AGENT=claude AGX_RUN_PROFILE=personal AGX_RUN_LOGIN=auto AGX_RUN_HOME=~/.claude agx whoami
run:      20261119-095000-p7q2
agent:    claude
profile:  personal
login:    auto (pool)
home:     ~/.claude
only:     erin (caps enforced)
in use:   none (erin over cap: 5h is 55% (cap 50%))

$ agx live show --id 20261119-095000-p7q2 --json --field data.no_key
erin over cap: 5h is 55% (cap 50%)

$ agx live use --id 20261119-095000-p7q2 --login auto
run         20261119-095000-p7q2
agent       claude
profile     personal
folder      ~/projects/app
started on  auto
chosen      auto
next        auto
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

Which to use:

| You want | Use | Caps | When the member is at a cap |
|---|---|---|---|
| one member, within its caps, for one run | `--pool-only NAME` (`agx run`, `new`, `resume`, `agx live use`) | enforced; can be lowered for the run | no key until it is under them; never another member |
| one member whatever its usage, for one run | `--pool-force NAME`, or `agx live use --login NAME` | ignored, each broken rule a warning | Claude gets 429s from the account |
| one member for every run, within its caps | a one-member pool (`agx login add --name carol-only --kind pool --pool-shared carol`) | the new pool's, set on it | no key, like `--pool-only` |
| one member for every run, whatever its usage | `agx login edit --id auto --pool-force NAME` | ignored | 429s |

`--pool-only` needs no extra login and keeps the pool's own caps, so it is the usual choice; a one-member pool is for a binding or profile that should always behave that way. `--pool-only` and `--pool-force` together are refused (exit 2, 125 from a launch): one keeps the caps, the other ignores them. The pool's `force` and its picker do not apply to a pinned run, and the dry run shows the pin and the member's effective caps with their sources (`pick: only carol (caps enforced) for every key of this run  from --pool-only`, `carol week cap 50%  from --pool-only-week`).

<!-- snippet: examples/acct-login-pool/session.txt#only-refusals -->
```sh
$ agx run --login auto --pool-only derek --pool-force bob --dry-run
agx: --pool-only and --pool-force both choose the member: --pool-only keeps its caps, --pool-force ignores them; pick one
[exit 125]

$ agx run --login auto --pool-only derek --pool-only-week 80 --dry-run
agx: --pool-only-week 80 is above derek's week cap 70; it may only lower it
[exit 125]

$ agx pool pick --id auto --pool-only zoe
agx: --pool-only zoe: zoe is not a member of pool auto (members: alice, bob, carol, derek, erin)
[exit 2]
```

## Your own picker

Set `picker.command` and agx runs it at every key request instead of the built-in rules (a force still wins over it). The command runs through `sh -c` with your environment plus `AGX_POOL` (the pool's name). It reads one JSON object on stdin and prints the chosen member's login name on stdout:

```json
{
  "version": 1,
  "pool": "auto",
  "agent": "claude",
  "now": "2026-11-19T10:00:00Z",
  "builtin": "carol",
  "rules": {
    "shared_borrow_window_hours": 48,
    "short_unavailable_at_percent": 99,
    "defaults": { "short": 50, "week": 70 },
    "own": ["alice", "bob"],
    "shared": ["carol", "derek", "erin"],
    "accounts": { "alice": { "short": 95, "week": 98, "enabled": true } }
  },
  "members": [
    { "name": "carol", "group": "shared", "state": "ready", "rank": 1, "eligible": true, "enabled": true,
      "status": "ok", "account": "carol@example.com", "short": 0, "week": 26,
      "weekly_resets_at": "2026-11-21T00:00:00Z", "hours_to_reset": 38, "room_percent": 44,
      "caps": { "short": 50, "week": 70 }, "reasons": [], "notes": [], "source": "cache", "age_seconds": 20 }
  ]
}
```

`builtin` is the member the built-in rules would pick (`null` when none is usable); `members` are the [`agx pool pick --json`](../commands/pool.md#agx-pool-pick) candidates, in the rules' order. The input never holds a token. The rules are:

- The output must be exactly one non-empty line (spaces around it are trimmed) naming a member of the pool. A member the rules would skip is used anyway, with a warning per broken rule.
- Anything else falls back to the built-in rules with one warning naming why: the command does not start, exits non-zero (`exited 3: <first stderr line>`), runs past `picker.timeout` (`timed out after 10s`), prints nothing, prints more than one line, or prints a name that is not a member. A broken picker never takes the session down: a failed key request would stop Claude.

A tiny picker that prefers your own `alice` whenever she is ready, and otherwise takes the built-in pick (from the example, needing only `sh` and `sed`):

<!-- snippet: examples/acct-login-pool/home/picker.sh -->
```sh
#!/bin/sh
# A pool's picker: the members and their usage arrive as JSON on stdin
# (never a token); print one member's login name. This one prefers alice
# whenever she is ready, and otherwise takes the built-in rules' pick.
input=$(cat)
case "$input" in
  *'"name":"alice","group":"own","state":"ready"'*) echo alice ;;
  *) printf '%s\n' "$input" | sed -n 's/.*"builtin":"\([^"]*\)".*/\1/p' ;;
esac
```

<!-- snippet: examples/acct-login-pool/config.jsonc#picker -->
```jsonc
    "picky": {
      "agents": ["claude"],
      "kind": "pool",
      "pool": {
        "own": ["alice", "bob"],
        "shared": ["carol"],
        "picker": { "command": "sh $HOME/picker.sh" }
      }
    }
```

<!-- snippet: examples/acct-login-pool/session.txt#picker -->
```sh
$ agx pool pick --id picky
Pool: picky (claude)
Use: alice (own)
picked by the picker command (sh)
Source: cache (20s old)

Candidates (ready accounts in pick order):
- carol [shared] #1 READY (5h 0%, week 26%, resets in 1 d 14 hr, room 44%)
- alice [own] #2 READY (5h 3%, week 29%, resets in 6 d 0 hr, room 41%)
- bob [own] skip: status is limit-reached; 5h is 100% (unavailable at 99%); 5h is 100% (cap 50%) (5h 100%, week 61%, resets in 3 d 0 hr, room 9%)
```

## The rules

1. **Force.** `--pool-force` (one run, `pool pick` or `login token` call), else the pool's `force`, wins outright and every rule below is ignored. A `--pool-only` member instead makes the pool that one member, with every rule below applied to it and no force; when it is blocked nothing is picked. Each rule the forced member breaks is printed as a warning. Forcing a login without a usage reading is an error (exit 1). Forcing a login listed in neither group works, checked against the default caps, with the warning `not listed in own or shared`.
2. **Usable.** A member is skipped when any of these holds, and every reason is listed: disabled in config; no usage reading; its token is an API key (billed per use) while the member is declared `oauth-token` and its `allow_api_key` is off (`token is an API key (billed per use); allow_api_key is off`, the only reason then listed; such a member is never probed, and a forced or pinned one is refused, see [Is a login billed per use?](../logins.md#is-a-login-billed-per-use)); status is not `ok` (`limit-reached`, `rejected`, `secret`, `network`, …); usage that is not a number (`week usage is unreadable`: a cap cannot be checked on it, so the member is blocked rather than treated as fine); 5-hour use at or over `short_unavailable_at_percent` (99), even when the status is `ok`; 5-hour use at or over its 5-hour cap; weekly use at or over its weekly cap. Caps are exclusive: under the cap is usable. Caps count the account's total use: at 60% of a 70% weekly cap you can use 10%, and the teammate keeps 30%. A window the probe does not report is not checked.
3. **Pool.** While any own member is usable, the pool is the usable own members plus the usable shared members whose weekly reset is within `shared_borrow_window`; the other usable shared members wait. With no own member usable, the pool is every usable shared member.
4. **Order.** The pool is sorted by weekly reset, soonest first. An unknown reset, an unreadable one (noted `weekly reset time is unreadable; treated as unknown`), or one that has already passed (stale data, noted on the member), sorts last. Ties keep config order, own before shared.
5. **Pick.** The first of the pool. An empty pool picks nothing: `agx pool pick` and `agx login token` exit 1 with error code `auto-pick` and each member's reasons, and the key helper fails, so Claude's request fails with that line.

Caps always apply: a reset coming soon moves a member up the order, it never lifts a cap. Examples with the example's names:

| Situation | Pick | Why |
| --- | --- | --- |
| alice (own) week 29%, resets in 6 days; carol (shared) week 26%, resets in 38 h | carol | her 44% of room is lost in 38 h; alice can wait |
| alice (own) week 29%, 6 days; derek (shared) week 15%, resets in 4 days | alice | derek is outside the 48 h window: `WAIT` |
| alice (own) week 29%, 6 days; erin (shared) 5h 55%, week resets in 3 h | alice | erin is over her 5-hour cap of 50%, even on reset day |
| alice and bob limit-reached; carol week 1% in 7 days, derek week 3% in 6 days | derek | with no own member usable the window does not apply; soonest reset wins |

The config is strict, so nothing is ignored silently: an unknown key anywhere (a typo such as `"weekly"` or `"shared_borrow_window_hour"`), a cap that is not a number from 0 to 100 (`null` included), an `enabled` that is not `true`/`false`, a name in both lists, an empty `force` (write `null`), a member that does not exist or serves another agent, and a bad duration are each an error at their line when the config loads, and `agx config check` lists them all.

The 48 h window is fixed: it does not know how fast you use quota, so if you borrow a lot an own account can still end its week with room left. Lower `shared_borrow_window` then (`0s` borrows only when none of yours is usable). There is no daily cap: the probe reports no daily window.

## Strategies

The rules above are the `deadline` strategy, the default: soonest weekly reset first, and every running session gets the same pick. `strategy: "expiring"` keeps rules 1 to 3 (force, usable, pool) and changes the rest, to spend the room that is about to be lost:

- **Weekly caps rise near the reset.** Over the last `week_relax_window` (default `24h`) before a member's weekly reset, its weekly cap rises in a straight line from its cap to its `week_relax_max` (default 95): a shared member at cap 70 has 82.5 twelve hours before its reset and 95 at it. Room under 70 still protects the teammate for the rest of their week; the last day's room would be lost at the reset anyway. The 5-hour cap never rises: that window outlives the weekly reset and is the teammate's working time. Set `accounts.NAME.week_relax_max` to the member's weekly cap (or lower) to never relax one teammate.
- **Most room to lose first.** The pool is ordered by urgency: the room under the weekly cap divided by the hours to the reset, the rate needed to use it all before it is lost. A shared member with 30% left and 20 hours to go (1.5%/h) goes before your own with 90% and six days (0.6%/h). `agx pool pick` shows it per member (`needs 1.5%/h`).
- **A pick per session.** Each session (each `--login POOL` run) gets its own pick when it asks for a key. It stays on its member while that member is still in the pool (`sticky`, default `true`: switching loses the conversation's prompt cache), and one member takes at most `max_sessions` sessions (default 3) before the next most urgent one is used, so ten sessions spread over several accounts' 5-hour windows instead of draining one. When every member is full, the least loaded one takes the session. A session counts while it asked for a key within two `key_ttl`s.

The defaults come from the pool simulator (`task poolsim`): on the simulated weeks it leaves the waste of the deadline strategy unchanged when quota is plentiful and cuts it when sessions want more than the pool has, halves the teammates locked out because of us, and switches accounts about half as often; the price is more hours above a teammate's normal caps in the last day before their reset.

```jsonc
"pool": {
  "own": ["alice", "bob"],
  "shared": ["carol", "derek"],
  "strategy": "expiring",
  "own_caps": { "short": 95, "week": 98 },
  "week_relax_window": "24h",
  "max_sessions": 3
}
```

`own_caps` and `shared_caps` set a group's caps once instead of per member (a member's own entry still wins, then its group's, then `defaults`); they apply under both strategies.

## Usage and the cache

agx reads a member's usage with the same small probe `agx login check` and `--auto-login` use (cost Q). Every pick shares one cache, `~/.agx/cache/login-usage.jsonc` (0600, in a 0700 folder: it holds account labels, never a token), written atomically under a lock so several sessions never read half a file:

- A member's reading younger than `usage_max_age` (60 s) is reused; older ones are probed again, in parallel. So however many sessions run, each member is probed about once a minute.
- A probe that fails for a passing reason (no network, a 429 without usage, an unexpected answer) falls back to that member's last cached reading, with a warning on stderr (never on stdout, which Claude reads); the next request probes again.
- `agx pool pick --cached` uses the cache only, of any age, and probes nothing; `--max-age 10s` asks for fresher readings once.

With a `log` path, every key request appends one line, so you can watch the switching (`tail -f`):

```text
2026-11-19T10:00:47Z mode=credential pool="auto" picked="carol" group="shared" forced=false by="rules" source="live agx result"
2026-11-19T10:01:12Z mode=credential pool="auto" picked="carol" group="shared" forced=false by="rules" source="cache (25s old)"
2026-11-19T10:09:30Z mode=credential pool="auto" picked=null group=null forced=false by=null source="live agx result" error="pool auto: no member is usable: …"
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
| Nothing usable | Claude's request fails with `pool auto: no member is usable: …` | the helper exits 1 with no key | `agx pool pick --id auto` shows why; force a member if you must go on |
| Forced member at its limit | every request is a 429 | force ignores every rule | clear it: `agx login edit --id auto --pool-force ''` |
| A pinned session stops | Claude's request fails with `carol: week is 70% (cap 70%)` | `--pool-only` never goes over a cap and never switches | wait for it to fall under the cap, or hand the run back: `agx live use --id RUN --login auto` |
| A teammate's account is picked before yours | `Use: carol (shared)` while yours have room | her weekly quota expires within the window; yours can wait | intended; `"shared_borrow_window": "0s"` borrows only when none of yours is usable |
| Teammate caps can be overshot | a teammate's 5-hour use goes past 50% | usage is checked only when Claude asks for a key | lower the cap for a buffer (40) |
| `WAIT` is not `skip` | `derek [shared] WAIT` | usable, held back by the window while one of yours is usable | nothing; it moves up as its reset nears |
| A just-limited member is picked | picked, then a 429 | the cache can be up to `usage_max_age` old | covered by the 95/98 caps, or lower `usage_max_age` |
| Probes fail | `warning: probe of alice failed (network: …); using its cached usage from 2m0s ago` | no network, or the endpoint is busy | check `agx login check --id alice`; picks may be stale meanwhile |
| Stale data | a member noted `weekly reset time has passed; usage data is stale` | a `--cached` reading older than the member's weekly reset | run without `--cached` |
| `--cached` never refreshes | the same answer every time | it reads the cache only | use `--max-age` in scripts |
| The cache holds emails | — | account labels are kept with each reading | it is 0600; do not share it |
| Usage that is not a number | `skip: week usage is unreadable` | a cap cannot be checked on it, so the member is blocked rather than treated as fine | check `agx login check --id NAME` |
| Unreadable reset time | note `weekly reset time is unreadable; treated as unknown` | no deadline can be read, so the member sorts after those with one | check `agx login check --id NAME` |
| A broken cache file | `warning: login-usage cache: …`, then fresh probes | a cache agx cannot read is ignored and rewritten, never half-used | nothing; it heals on the next write |
| A typo in config | `unknown key "weekly"` at its line | every key is checked; unknown ones are errors so they cannot be ignored silently | fix the name |
| Wrong value type | `logins.auto.pool.accounts.alice.week: must be number, got string`, `….week: must be number, got null`, `….week: must be <= 100`, `force is empty; name a login or write null` | `"70"`, `"false"`, `""` or a `null` cap would otherwise behave unexpectedly | set numbers with the pool flags or `agx config set`, which check them first; `null` only for `force` |

## Testing

Picking cannot silently go wrong because three layers check the rules (package `pool`), all in `task check` except the last, none touching a real account, network or token:

1. **Example tests**: every case of the stand-alone selector's suite ported one to one (the mapping, case by case, is in [`pool/testdata/SELECTOR_PORT.md`](../../pool/testdata/SELECTOR_PORT.md)), every config mistake and the built-in defaults, plus boundary tables for every cap, window and cutoff.
2. **Randomised tests**: thousands of seeded pools and readings, odd ones included (missing windows, usage that is not a number, stale or unreadable resets, caps at their edges). For each, the pick and the whole pick order must equal a second implementation of the rules written separately inside the test, and invariants must hold: the pick is never blocked, disabled, at or over a cap or the 5-hour cutoff; a teammate's account goes before a usable own one only inside the window and only when it resets sooner; nothing in the pool resets sooner than the pick; reading order never changes the result; a force always wins; the same input always gives the same output. A failure names its seed: `SEED=1234 go test -run TestPortRandomised ./pool/` replays it, `CASES=50000` runs more.
3. **Mutation check** (`task mutation`, opt-in, a few minutes): breaks the rules on purpose 60 ways (a `>=` turned into `>`, a rule skipped, an order reversed, a default changed, a config check removed, a `--pool-only` pin ignored or its caps lifted) on a temporary copy of the repository and confirms the tests fail every time. Last run: 60/60 caught. Run it after any change to the rules; if a break survives, add a test for it. The key-helper flow runs end to end through `agx credential` with a moving clock and a fake usage server: first call, cache reuse, a change while fresh, re-probe after max age, caps, limits, probe failure, disabled, force, nothing usable, six helpers at once, and a log that never holds a token.

Live, opt-in (run them yourself; they print no token value, only its length):

```sh
scripts/live-check.sh                 # 1-4: pick, token length, cache reuse, live dry run (at most one probe per member)
scripts/live-check.sh --with-claude   # 5: one tiny real Claude call through the pool (spends a few tokens)
scripts/limit-recovery-check.sh LIMITED WORKING   # a real session on a limited login, switched mid-session (spends a few tokens)
```

Both read `AGX` (the binary, default `agx`), `AGX_POOL` (default `auto`) and `AGX_ARGS` (extra global flags, such as `--config FILE`; by default none, so the scripts use plain `agx` and your normal config). `limit-recovery-check.sh` adds a temporary pool of your two logins, forced to the limited one, switches the force after the first message of a four-message session, and removes the temporary login at the end. Expected with Claude Code 2.1.328: message 1 fails (429), message 2 fails too (the new key applies one request late), messages 3 and 4 succeed in the same session.

## Open items

Not built; listed so they are not forgotten:

- A short shell alias for the pool launch (`alias cauto='agx run --login auto'`): documented here, added by nobody's shell file. `agx alias add` can make one.
- An automatic `continue` after a 429 mid-task: Claude Code does not retry a rejected request on a new key by itself; a Claude Code hook might, not investigated.
- A borrow window based on how fast quota is used, instead of the fixed `shared_borrow_window`, if an own account ever ends its week with room left.

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-login-pool$'
```

[`examples/acct-login-pool`](../../examples/acct-login-pool/README.md) runs every step above on fake logins with a seeded cache, so nothing is probed.

## Related

- [Live login](live-login.md): switch a session by hand; a pool run is a live run whose next login is the pool.
- [Let agx pick by headroom](auto-pick.md): `--auto-login` picks one login at launch by the most headroom; a pool keeps picking while the session runs, by the rules above.
- [`agx pool list`, `show` and `pick`](../commands/pool.md) and [`agx login token`](../commands/login.md#agx-login-token) in the command reference.
