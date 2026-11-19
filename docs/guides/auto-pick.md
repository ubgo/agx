# Let agx pick by headroom

With two or more accounts you usually want the one with the most quota left. `--auto-profile` picks the profile whose home has the most plan headroom, and `--auto-login` picks the stored login with the most headroom. Both read usage first, then choose, and the dry run names the choice and the numbers behind it. Nothing is picked silently: when no candidate qualifies, the launch is refused with every candidate's reason.

You want this when you run out of a plan's five-hour or weekly window often and do not want to check `agx usage` before each launch.

`--auto-login` picks once, at launch, by the most headroom. A [login pool](login-pool.md) is the other way to pick among Claude logins, and the two use different rules on purpose:

| | `--auto-login` | a `pool` login |
|---|---|---|
| when it picks | once, at launch | each time Claude asks for a key (every `key_ttl`, 60 s), while the session runs |
| candidates | every token login serving the agent | the pool's `own` and `shared` members |
| rule | lowest highest-window percent wins; a window at 100% skips | soonest weekly reset first, own before shared, teammates' accounts only up to their caps and within the borrow window ([the rules](login-pool.md#the-rules)) |
| usage | probes every candidate, every launch | probes members once per `usage_max_age`, shared by every session through a cache |
| agents | Claude Code (Codex homes with `--auto-profile`) | Claude Code |

`--auto-login` never considers a pool login (a pool has no token to probe), and its candidates and rule did not change when pools were added.

## Before you start

- Two or more profiles of one agent ([A second account](second-account.md)) for `--auto-profile`, or two or more stored logins serving one agent ([Logins](../logins.md#adding-a-login)) for `--auto-login`.
- Which agents can take part (from `agx agent show`):

| Agent | `--auto-profile` (live usage per home) | `--auto-login` (probe per login) |
|---|---|---|
| Claude Code | yes | yes |
| Codex | yes | no: Codex has no token login to probe |
| Gemini CLI, pi, custom agents | no: they have no plan usage agx can read | no |

- The cost. `--auto-profile` makes one live usage request per home (cost N; a home shared by several profiles is read once). `--auto-login` resolves each login's token (cost X when the reference runs a command) and sends a minimal request with each plan token (cost Q: it spends a few tokens of each plan); an API key or key helper is never probed, since it bills per token. Usage endpoints rate-limit, so do not put either flag in a loop.

## What headroom means

Each candidate's usage has windows (Claude: the five-hour session and the weekly windows). Its score is its highest window percent, and the lowest score wins: a home at 5% of its session and 30% of its week (score 30) beats one at 1% and 62% (score 62). A candidate with any window at 100% has no headroom and is skipped as `rate-limited, resets HH:MM`, even when every candidate is full.

Candidates are skipped, never silently dropped, when:

- the profile bills per token (`"billing": "api"`), or a login is an API key or key helper: per-token billing has no windows;
- the home does not exist, or is not logged in;
- the usage request fails (rate limited, expired, rejected, network);
- a login's token does not resolve (an unset variable, a missing gopass entry).

When a home answers 429 (rate limited) to `--auto-profile`, agx uses its cached reading if the home has one, and the explanation shows its age. Claude Code 2.1.323 and later no longer store a reading on disk, so for current Claude homes there is usually none to fall back to (see [Plan usage](usage.md#cached-readings-and-claude-code-21323)).

## Step 1: preview the pick

Add `--dry-run` to see the choice without launching. The `profile` (or `login`) line's source carries the explanation, in the form `--auto-profile (auto → work (max 5%) over personal (max 62%))`: the chosen candidate with its score, then the others. With a cached fallback the score reads `max 5%, cached 4m ago`.

```sh
agx run --agent claude --auto-profile --dry-run
agx run --profile work --auto-login --dry-run
```

## Step 2: launch on the pick

Drop `--dry-run`. The two flags combine: `--auto-profile` picks the home, then `--auto-login` picks the login it runs on.

```sh
agx run --agent claude --auto-profile
agx run --profile work --auto-login
agx run --agent claude --auto-profile --auto-login
```

`agx run` and `agx new` take both flags. `agx resume` takes `--auto-login` only: the conversation's own home decides the profile.

## Try it

Picking needs real accounts and the network, so the tested example shows the refusals, which are all decided before any request ([`examples/acct-auto-pick`](../../examples/acct-auto-pick/README.md)). Its config:

<!-- snippet: examples/acct-auto-pick/config.jsonc -->
```jsonc
{
  "version": 1,
  "default_agent": "claude",
  "profiles": {
    "kimi": { "agent": "claude", "home": "~/.claude", "billing": "api", "default": true },
    "kimi-work": { "agent": "claude", "home": "~/.claude-work", "billing": "api" },
    "old": { "agent": "claude", "home": "~/.claude-old" },
    "codex": { "agent": "codex", "home": "~/.codex", "default": true },
    "gem": { "agent": "gemini", "home": "~/.gemini", "default": true }
  },
  "logins": {
    "carol": { "agents": ["claude"], "kind": "oauth-token", "token": "${CAROL_TOKEN}" },
    "console": { "agents": ["claude"], "kind": "api-key-helper", "command": "gopass show -o work/anthropic-api-key" },
    "gem-work": { "agents": ["gemini"], "kind": "api-key", "token": "${gopass:google/gemini-key}" }
  }
}
```

No Claude profile qualifies: two bill per token and one home is missing. Once the missing one is gone, the refusal says so in one line:

<!-- snippet: examples/acct-auto-pick/session.txt#profile-none -->
```sh
$ agx run --agent claude --auto-profile --dry-run
agx: auto profile: none eligible: kimi (bills per token); kimi-work (bills per token); old (home ~/.claude-old does not exist)
[exit 125]
```

<!-- snippet: examples/acct-auto-pick/session.txt#profile-per-token -->
```sh
$ agx profile remove --id old --yes
removed profile old (home ~/.claude-old is kept)

$ agx run --agent claude --auto-profile --dry-run
agx: auto profile: every candidate bills per token
[exit 125]
```

No Claude login qualifies: `carol`'s variable is not set, and `console` is a key helper, which bills per token:

<!-- snippet: examples/acct-auto-pick/session.txt#login-none -->
```sh
$ agx run --agent claude --auto-login --dry-run
agx: auto login: none eligible: carol (login carol: CAROL_TOKEN is not set); console (bills per token)
[exit 125]
```

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/acct-auto-pick$'
```

## Pitfalls

- An agent without usage is refused with its reason (exit 125), and the hint names a profile or login to choose by hand. Codex can pick a profile (here its home is not logged in), but has no login to probe:

<!-- snippet: examples/acct-auto-pick/session.txt#profile-unsupported -->
```sh
$ agx run --agent gemini --auto-profile --dry-run
agx: gemini has no usage; choose a profile
  agx run --profile gem
[exit 125]

$ agx run --agent codex --auto-profile --dry-run
agx: auto profile: none eligible: codex (not-logged-in)
[exit 125]
```

<!-- snippet: examples/acct-auto-pick/session.txt#login-unsupported -->
```sh
$ agx run --agent gemini --auto-login --dry-run
agx: gemini cannot probe a login's usage (probing a Gemini key spends quota through a model call that is not modelled); choose a login
  agx run --login gem-work
[exit 125]

$ agx run --agent codex --auto-login --dry-run
agx: auto login: no stored login serves codex
[exit 125]
```

- `--auto-profile` with `--profile`, and `--auto-login` with `--login`, `--home-login` or `--token-ref`, both choose the same thing and are refused (exit 125):

<!-- snippet: examples/acct-auto-pick/session.txt#conflict -->
```sh
$ agx run --profile kimi --auto-profile --dry-run
agx: --profile and --auto-profile both choose the profile; pick one
[exit 125]

$ agx run --login carol --auto-login --dry-run
agx: --login and --auto-login both choose the login; pick one
[exit 125]
```

- `--auto-login` probes every stored login that serves the agent, all of them, each time. Keep only the logins you want considered, or name one with `--login`.
- A per-token profile on a shared home (such as `kimi` on `~/.claude`) is never a candidate, but the plan profile on the same home is.
- The pick is made once, at launch. To move a running Claude session to another account, use [live login](live-login.md).

## Related

- [Plan usage](usage.md): the numbers the pick reads.
- [Logins](../logins.md): stored logins and kinds.
- [How agx decides](../how-agx-decides.md#the-chain): where `--auto-profile` and `--auto-login` sit in the order.
- [Launch reference](../commands/launch.md): the launching commands and their flags.
- [`examples/acct-auto-pick`](../../examples/acct-auto-pick/README.md): the refusals above, tested.
- [Where to set the profile and login](../choosing/where-to-set.md): the decision guide.
