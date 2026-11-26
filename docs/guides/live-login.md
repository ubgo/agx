# Live login

Live login lets you switch the account of a Claude Code session while it runs. A long session hits its plan's five-hour limit; instead of quitting and resuming on another account, you run one command in another terminal, and the session continues on the other plan at its next key refresh, about ten seconds later.

It relies on Claude Code behaviour (re-running its key helper on a timer and after a rejected key), checked with Claude Code 2.1.327. What happens when a real rate limit (HTTP 429) is followed by a switch has not been tested.

## Before you start

- Claude Code. Other agents read their login once at start (agx refuses `--live-login` for them with the agent's reason).
- Two or more stored token logins for Claude ([Logins](../logins.md#adding-a-login)): plan tokens (`oauth-token`) or API keys. A key-helper login cannot be switched to, since the agent would run its command, not agx's.
- The session must start on a stored login, not on the home login: agx can only hand over tokens it holds.
- Nothing in the launch may set a variable that outranks the key helper (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`): not the profile's `env` or `env_refs`, not `--env`, not `--setting env.NAME=…`.

The example config ([`examples/acct-live-login/config.jsonc`](../../examples/acct-live-login/config.jsonc)) has the profile `work` on login `carol` with live login on, and a second login `alice`.

## Step 1: start the session switchable

Add `--live-login` to the launch, or set it once on the profile:

<!-- snippet: examples/acct-live-login/config.jsonc#profile -->
```jsonc
    "work": { "agent": "claude", "home": "~/.claude-work", "login": "carol", "live_login": true },
```

```sh
agx run --profile work                                # live_login is on for this profile
agx run --profile personal --login carol --live-login # or per run
```

The dry run shows what it adds. Instead of `CLAUDE_CODE_OAUTH_TOKEN`, Claude gets an `apiKeyHelper` setting (in a settings file in the run folder, mode 0600) that runs agx's hidden `agx credential` command, and `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` so it runs it again every ten seconds. (`agx credential` is run by the agent and prints the token of the login chosen for this run, which `agx live use` can change; [`agx login token --id NAME`](../logins.md#print-a-logins-token) is the command you run yourself and always prints that one login's token.) The `note` line names the command to switch:

<!-- snippet: examples/acct-live-login/session.txt#dry-run -->
```sh
$ agx run --profile personal --login carol --live-login --dry-run --quiet
agent     claude                                                                                        from profile personal
profile   personal                                                                                      from --profile
home      ~/.claude                                                                                     from profile personal
login     carol (not resolved: --dry-run)                                                               from --login
binary    /usr/local/bin/claude version unknown                                                         from no install metadata
folder    ~/projects/app                                                                                from current folder
note      live login: switch with agx live use --id 20261118-120000-8pyt --login NAME (refresh 10s)     from --live-login
          when no key is usable: stop (the agent waits for a usable key)                                from default
settings  apiKeyHelper                                                                                  from --live-login
          statusLine agx statusline (the account in use)                                                from default
          delivered as file ~/.agx/runs/20261118-120000-8pyt/settings-20261118-120000-8pyt.json (0600)  from settings_delivery file
run       ~/.agx/runs/20261118-120000-8pyt                                                              from AGX_RUN_ID=20261118-120000-8pyt
command   claude --settings ~/.agx/runs/20261118-120000-8pyt/settings-20261118-120000-8pyt.json         from built by agentio
env       set AGX_RUN_ID=20261118-120000-8pyt                                                           from launch environment
          set AGX_RUN_AGENT=claude                                                                      from launch environment
          set AGX_RUN_PROFILE=personal                                                                  from launch environment
          set AGX_RUN_LOGIN=carol                                                                       from launch environment
          set AGX_RUN_HOME=~/.claude                                                                    from launch environment
          set CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000                                                   from launch environment
note      installed Claude Code version unknown; flags not checked
```

The run folder also gets `live.jsonc`: the login the run started on, the login chosen for it, and the login in use at the agent's last refresh. Never a token.

## Step 2: find the run

In another terminal, `agx live list` shows every live run whose agent has not ended: its label, folder and process id, so you can tell which terminal it is, the login in use (`IN USE`), when a key was last handed out (`LAST KEY`) and its liveness. `--wide` adds the agent, profile, terminal, parent process, the login it started on and gets next (`STARTED ON`, `NEXT`), and the conversation with its title. Name a run when you start it, `agx run --login carol --live-login --label review`, or let your terminal tool set `AGX_RUN_LABEL`. `agx live show --id RUN` shows one run, ended or not.

<!-- snippet: examples/acct-live-login/session.txt#list -->
```sh
$ agx live list --agent claude
RUN                   LABEL  FOLDER          PID  IN USE  LAST KEY  LIVENESS
20261114-101500-k3x9  —      ~/projects/app  —    —       —         unknown

$ agx live show --id 20261114-101500-k3x9
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      —
next        carol
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

In this example the run folder is seeded (the test's fake agent exits at once), and its record has no process id, so liveness is `unknown`. A real running session shows `alive`.

## Step 3: switch

`agx live use --id RUN --login NAME` records the choice. Nothing is resolved yet:

<!-- snippet: examples/acct-live-login/session.txt#use -->
```sh
$ agx live use --id 20261114-101500-k3x9 --login alice
run         20261114-101500-k3x9
agent       claude
profile     work
folder      ~/projects/app
started on  carol
chosen      alice
next        alice
in use      —
liveness    unknown (process info unreadable: run.jsonc records no process id)
```

At its next key refresh, Claude Code runs the helper. `agx credential` finds its run through `AGX_RUN_ID`, resolves the chosen login, prints its token for the agent alone, and records it as in use. You never run it yourself; here it is run by hand to show the effect (with the fake token of the test machine):

<!-- snippet: examples/acct-live-login/session.txt#credential -->
```sh
$ AGX_RUN_ID=20261114-101500-k3x9 agx credential
fake-token-alice
```

## Step 4: go back

`--start-login` clears the choice: the next refresh goes back to the login the run started on. `in use` changes at that refresh.

<!-- snippet: examples/acct-live-login/session.txt#after -->
```sh
$ agx live show --id 20261114-101500-k3x9
run           20261114-101500-k3x9
agent         claude
profile       work
folder        ~/projects/app
started on    carol
chosen        alice
next          alice
in use        alice
in use since  2026-11-18 12:00:00
last key      2026-11-18 12:00:00
liveness      unknown (process info unreadable: run.jsonc records no process id)

$ agx live use --id 20261114-101500-k3x9 --start-login
run           20261114-101500-k3x9
agent         claude
profile       work
folder        ~/projects/app
started on    carol
chosen        —
next          carol
in use        alice
in use since  2026-11-18 12:00:00
last key      2026-11-18 12:00:00
liveness      unknown (process info unreadable: run.jsonc records no process id)
```

## Let a pool switch for you

A run on a [`pool` login](login-pool.md) is a live run that switches by itself: its start login is the pool, and each time Claude runs its key helper, `agx credential` picks a member by usage and hands out that member's token, with the pool's `key_ttl` (60 s by default) as the refresh interval instead of ten seconds. `agx live use --id RUN --login alice` pins one member for that run, ignoring her caps; `agx live use --id RUN --pool-only alice` pins her with her caps enforced (the session waits with no key while she is over one); `agx live use --id RUN --start-login` goes back to the pool.

```sh
agx run --login auto                 # live without --live-login
agx pool pick --id auto             # which member the next key comes from, and why
```

When every member is over its caps, the session waits instead of going past them: `agx credential` hands Claude a placeholder that is not a key, and Claude continues by itself once a member is usable ([when no account is usable](pool-rules.md#when-no-account-is-usable)).

Which account is my session on? The bottom of Claude's screen says it: a live run's launch sets [`agx statusline`](../commands/live.md#agx-statusline) as Claude's status line (`pool auto: alice · 5h 40% · week 61%`, `login carol`), unless you set your own. Or run `! agx whoami` inside Claude: `in use:` is the login whose key Claude got last, and `next:` the one it gets at its next refresh when a switch is pending. /status only says apiKeyHelper, and Claude's own guess reads the keychain login, not the key it uses.

## When a switch applies

`agx live use` only records the choice. Claude Code picks it up the next time it runs its key helper, which happens:

- at the next timed refresh: Claude re-runs the helper once the interval in `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` has passed (agx sets ten seconds);
- at once after a rejected key: when a request gets HTTP 401, Claude re-runs the helper before each retry (up to 10 retries, with backoff) and carries on as soon as the helper returns a good token. The session is not restarted.

The timed refresh is stale-while-revalidate: Claude fetches the new key in the background, so the first request after a refresh can still go out with the old login's key, and the next one uses the new key. `in use` in `agx live show` changes when the helper ran, which can be one request before the new login is actually used.

## Where agx sets the refresh interval

agx puts `CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000` in the agent's process environment, next to `AGX_RUN_ID`; the dry run lists it under `env` (`from launch environment`). Claude Code also honours the variable from a settings `env` block, but agx keeps it in the process environment because:

- the value comes from agentio's live-login plan for Claude, which names it as a process environment change; agx applies the plan as given and holds no Claude-specific rules of its own;
- it then stays out of the run's settings, so the settings carry only `apiKeyHelper` and whatever your profile and flags set;
- the dry run shows it with its source like every other variable.

Keep the interval in one place: setting it again with `--setting env.CLAUDE_CODE_API_KEY_HELPER_TTL_MS=…` puts a second value in play, and agx does not check which one Claude uses.

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-live-login$'
```

See [`examples/acct-live-login`](../../examples/acct-live-login/README.md) for the config and the seeded run folder.

## Pitfalls

<!-- snippet: examples/acct-live-login/session.txt#refusals -->
```sh
$ agx run --profile personal --live-login --dry-run
agx: --live-login needs a stored login; this run starts on the home login (default (no login named))
  agx run --login NAME --live-login
[exit 125]

$ agx run --profile keyed --live-login --dry-run
agx: profile keyed env sets ANTHROPIC_API_KEY, which outranks the live-login helper; drop it or --live-login
[exit 125]

$ agx run --profile codex --live-login --dry-run
agx: codex is not switchable (Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting); --live-login cannot apply
[exit 125]

$ agx live use --id 20261114-101500-k3x9 --login alice --start-login
agx: --login and --start-login both choose the login; pick one
[exit 2]
```

- A run on the home login is refused (exit 125): name a login with `--login` or the profile's `login`.
- A launch that sets a variable which outranks the key helper is refused (exit 125), naming where it comes from (profile `env`, `env_refs`, `--env` or `--setting env.NAME`): drop the variable, or drop live login for that run.
- A one-off `--token-ref` is refused with `--live-login` (exit 125): `agx credential` could not fetch it again. Use a stored login.
- Codex, Gemini CLI and pi are refused with their own reason (exit 125): they read their login once at start.
- A raw `--settings` of your own is merged with the settings live login adds (one `--settings` reaches Claude, see [launch options, recipe 8](../launch-options.md#a-raw---settings-of-your-own)), so your hooks or theme and the helper both apply. A raw value that sets `apiKeyHelper` itself is two credential helpers in one command and is refused: `agx: raw --settings apiKeyHelper and --live-login both set the credential; pick one` (exit 125). Drop `apiKeyHelper` from your settings, or drop `--live-login`.
- `agx live use` needs exactly one of `--login` and `--start-login` (exit 2 for both or neither). The login must exist, serve the run's agent and be a token login; a key-helper login is refused.
- A run whose agent has ended is refused (`run RUN has ended; its login can no longer change`, exit 1); `agx clean --yes` removes ended run folders. An ended run no longer shows in `agx live list`.
- The switch takes effect at the agent's next key refresh or right after a rejected key, not at once, and the first request after a refresh may still use the old key ([When a switch applies](#when-a-switch-applies)); `in use` tells you when the helper ran.
- Live login replaces the token variable with the helper, so the run's `CLAUDE_CODE_OAUTH_TOKEN` is not set. `agx run --dry-run` shows exactly what is set.

### When the helper has no key

Claude Code treats its key helper's answer in two ways. A helper that prints nothing or exits non-zero is a failure: with no key yet (the session's first request) Claude runs it three times, then exits 1 (`apiKeyHelper failed: did not return a value`, or `exited 1`), never falling back to the home's own keychain login; once it holds a key, it prints `apiKeyHelper failed` and goes on with the key it already has (tested with Claude Code 2.1.330). A helper that prints a value Claude cannot use is not a failure: the next request gets 401, and Claude retries it about ten times over four to five minutes, asking the helper before each try; after the last try it stops the turn (`Invalid API key · Fix external API key`, `Once fixed, resend your message`) and the next message you send asks again (tested with Claude Code 2.1.332).

So `agx credential` never prints an empty value. When it has no key for its run (a token that is empty, only whitespace, more than one line, or does not resolve: an unset variable, a missing gopass entry, a failing command, no stored value; or a pool with no usable member), it answers by the run's `when_none_usable` ([when no account is usable](pool-rules.md#when-no-account-is-usable)):

- `stop` (the default) prints `agx-no-account-usable` (not a key) and exits 0, with one line on stderr: `agx: warning: no key handed out: <reason>`. The session stops spending and waits; it goes on by itself once the login resolves again (or a pool member is usable).
- `keep` exits 1 with nothing on stdout and one line on stderr, which Claude shows in `/status`; a running session goes on with its old key.

The line names the login and the reference, never a value and never what the secret store itself printed; `agx login check --id NAME --no-probe` shows the full reason. `agx live show` and `agx whoami` show the refusal as `none (key not resolved) since 10:52` with the full line under `no key`.

<!-- snippet: examples/acct-live-login/config.jsonc#broken -->
```jsonc
    // Three logins whose token never becomes one usable line.
    "blank": { "agents": ["claude"], "kind": "oauth-token", "token": "$(printf ' ')" },
    "twolines": { "agents": ["claude"], "kind": "oauth-token", "token": "$(printf 'fake-token-a\\nfake-token-b')" },
    "unset": { "agents": ["claude"], "kind": "oauth-token", "token": "${DAVE_TOKEN}" }
```

<!-- snippet: examples/acct-live-login/session.txt#credential-refusals -->
```sh
$ agx live use --id 20261114-101500-k3x9 --login blank --field data.next
blank

$ AGX_RUN_ID=20261114-101500-k3x9 agx credential
agx: warning: no key handed out: login blank: token is empty (try: agx login check --id blank --no-probe)
agx-no-account-usable

$ agx live use --id 20261114-101500-k3x9 --login twolines --field data.next
twolines

$ AGX_RUN_ID=20261114-101500-k3x9 agx credential
agx: warning: no key handed out: login twolines: token has 2 lines (try: agx login check --id twolines --no-probe)
agx-no-account-usable

$ agx live use --id 20261114-101500-k3x9 --login unset --field data.next
unset

$ AGX_RUN_ID=20261114-101500-k3x9 agx credential
agx: warning: no key handed out: login unset: DAVE_TOKEN is not set (try: agx login check --id unset --no-probe)
agx-no-account-usable
```

Before switching to a login, check that it resolves: `agx login check --id NAME --no-probe` (no tokens spent). A waiting session goes on once you switch it to a login that resolves (`agx live use --id RUN --login NAME`); type `continue` if Claude has given up. If a session already exited, its run has ended (`agx live use` refuses it): resume the conversation on a login that resolves, `agx resume --last --login NAME --live-login`.

## Related

- [Logins](../logins.md): stored logins and how they reach Claude.
- [Live reference](../commands/live.md): `agx live list`, `show`, `use`.
- [Let agx pick by headroom](auto-pick.md): choose the account at launch instead.
- [Plan usage](usage.md): which login still has room to switch to.
- [`examples/acct-live-login`](../../examples/acct-live-login/README.md): the session this page shows.
- [Continue on another account](../choosing/continue-on-another-account.md): live login, resume on another login, or move the conversation.
