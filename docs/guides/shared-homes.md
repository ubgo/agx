# Shared homes

Several profiles may use one home. The home holds the agent's history, settings and stored login; the profiles on it differ in what agx adds per run: a stored login, environment variables, a model, args. The common case is one Claude home with two profiles: `personal` on your plan, and `kimi` sending the same Claude Code to another provider (Moonshot's Kimi through the Anthropic API shape) with its own base URL and key, so both see the same conversations and settings.

You want one when a second "account" is really another way to pay for the same tool (an API key, another provider, a proxy) and you want one shared history. If you want separate history, settings and quota, give the second account its own home instead (see [profiles](../commands/profile.md#agx-profile-add)).

A launch with `--profile` never needs anything more. The rule on this page matters only where agx starts from a home and must name a profile: `agx resume` (the conversation's home decides) and the home rows of `agx usage`.

## Before you start

- One profile on the home already (`agx profile list`).
- For a provider like Kimi: its base URL and a key stored where agx can read it (a `${gopass:…}` reference here). Nothing is read during a dry run.

The commands below come from [`examples/decide-shared-home`](../../examples/decide-shared-home/README.md), which runs in the test suite on a fake machine. Two conversations exist in `~/projects/client`: `3f2a9c1e`, stored in `~/.claude`, and `7b1d4e20`, stored in `~/.claude-work`.

## Step 1: add the second profile on the same home

`agx profile add` with `--home` naming the existing home adds a profile without touching the home. `--billing api` marks it as pay-per-token, which matters for the rule in step 3 and for `--auto-profile`, which skips such profiles. Here the example removes its `kimi` profile and adds it back with commands:

<!-- snippet: examples/decide-shared-home/add.session.txt#add -->
```sh
$ agx profile remove --id kimi --yes
removed profile kimi (home ~/.claude is kept)

$ agx profile add --name kimi --agent claude --home ~/.claude --billing api --dry-run
would add profile kimi: claude home ~/.claude (--home), login home login, billing api

$ agx profile add --name kimi --agent claude --home ~/.claude --billing api
added profile kimi: claude home ~/.claude (--home), login home login, billing api

$ agx profile edit --id kimi --env ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic
profile kimi: changed
  env.ANTHROPIC_BASE_URL  —  ->  (set)

$ agx profile use --id personal
profile personal is the default claude profile

$ agx profile list
PROFILE    PROVIDER  HOME            ACCOUNT  STATE  LOGIN       BILLING
*personal  claude    ~/.claude                none   home login  plan
work       claude    ~/.claude-work           none   home login  plan
kimi       claude    ~/.claude                none   home login  api
```

`agx profile edit --env` sets plain variables. The key itself belongs in `env_refs`, as a reference that is resolved only when a run really starts; write that part in the config:

<!-- snippet: examples/decide-shared-home/config.jsonc#profiles -->
```jsonc
  "profiles": {
    // the Claude plan account
    "personal": { "agent": "claude", "home": "~/.claude" },
    // Moonshot's Kimi through the Anthropic API shape, on the same home, so it sees the same history and settings
    "kimi": {
      "agent": "claude",
      "home": "~/.claude",
      "env": { "ANTHROPIC_BASE_URL": "https://api.moonshot.ai/anthropic" },
      "env_refs": { "ANTHROPIC_AUTH_TOKEN": "${gopass:moonshot/kimi-token}" }
    },
    // the only profile on ~/.claude-work
    "work": { "agent": "claude", "home": "~/.claude-work" }
  }
```

This config starts with no `"default": true` and both `~/.claude` profiles on the default `plan` billing, which is the case agx cannot decide alone.

## Step 2: launch either profile

A launch names the profile, so the shared home changes nothing; each profile adds its own environment, and the history is the same folder:

<!-- snippet: examples/decide-shared-home/session.txt#launch -->
```sh
$ agx run --profile kimi --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                    from profile kimi
profile  kimi                                                      from --profile
home     ~/.claude                                                 from profile kimi
login    home login                                                from default (no login named)
binary   /usr/local/bin/claude version unknown                     from no install metadata
folder   ~/projects/client                                         from current folder
env      ANTHROPIC_BASE_URL                                        from profile kimi env
         ANTHROPIC_AUTH_TOKEN                                      from profile kimi env_refs
run      ~/.agx/runs/20261118-061238-k2ly                          from AGX_RUN_ID=20261118-061238-k2ly
command  claude                                                    from built by agentio
env      set ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic  from launch environment
         set ANTHROPIC_AUTH_TOKEN=[redacted]                       from launch environment
         set AGX_RUN_ID=20261118-061238-k2ly                       from launch environment
         set AGX_RUN_AGENT=claude                                  from launch environment
         set AGX_RUN_PROFILE=kimi                                  from launch environment
         set AGX_RUN_LOGIN=home                                    from launch environment
         set AGX_RUN_HOME=~/.claude                                from launch environment
note     installed Claude Code version unknown; flags not checked
```

## Step 3: let a resume find its profile

`agx resume` starts from a conversation, which lives in a home, not a profile. When several profiles use that home and `--profile` is not given, agx picks, in this order:

1. the agent's default profile (`"default": true`), if it is on that home: `from home ~/.claude (default)`;
2. otherwise the home's only plan profile (`"billing": "plan"`, the default, beats `"billing": "api"`): `from home ~/.claude (plan profile)`;
3. otherwise, at a terminal, it asks which profile, and offers to remember the answer as the agent's default (`from home ~/.claude (asked)`); without a terminal it stops and lists the profiles.

With two plan profiles and no default, there is no terminal in the example, so it stops:

<!-- snippet: examples/decide-shared-home/session.txt#ambiguous -->
```sh
$ agx resume --id 3f2a9c1e --dry-run
agx: ~/.claude has 2 profiles (kimi, personal) and no default; add --profile
[exit 125]
```

`--profile` always decides, and the agent, home and folder still come from the conversation:

<!-- snippet: examples/decide-shared-home/session.txt#profile-flag -->
```sh
$ agx resume --id 3f2a9c1e --profile kimi --dry-run
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                    from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  kimi                                                      from --profile
home     ~/.claude                                                 from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login                                                from default (no login named)
binary   /usr/local/bin/claude version unknown                     from no install metadata
folder   ~/projects/client                                         from conversation 3f2a9c1e-0000-4000-8000-000000000001
...
```

Marking `kimi` as pay-per-token leaves `personal` as the only plan profile, so it wins, with a note on stderr:

<!-- snippet: examples/decide-shared-home/session.txt#billing -->
```sh
$ agx profile edit --id kimi --billing api
profile kimi: changed
  billing              plan  ->  api
$ agx resume --id 3f2a9c1e --dry-run
agx: profile personal from home ~/.claude (plan profile)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (plan profile)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
...
```

A default profile on that home wins before billing is considered:

<!-- snippet: examples/decide-shared-home/session.txt#default -->
```sh
$ agx profile use --id personal
profile personal is the default claude profile
$ agx resume --id 3f2a9c1e --dry-run
agx: profile personal from home ~/.claude (default)
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 3f2a9c1e-0000-4000-8000-000000000001
profile  personal                                              from home ~/.claude (default)
home     ~/.claude                                             from conversation 3f2a9c1e-0000-4000-8000-000000000001 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 3f2a9c1e-0000-4000-8000-000000000001
...
```

A home with one profile needs no rule:

<!-- snippet: examples/decide-shared-home/session.txt#one-profile -->
```sh
$ agx resume --id 7b1d4e20 --dry-run
agx: profile work from home ~/.claude-work
agx: warning: installed Claude Code version unknown; flags not checked
agent    claude                                                from conversation 7b1d4e20-0000-4000-8000-000000000002
profile  work                                                  from home ~/.claude-work
home     ~/.claude-work                                        from conversation 7b1d4e20-0000-4000-8000-000000000002 (stored there)
login    home login                                            from default (no login named)
binary   /usr/local/bin/claude version unknown                 from no install metadata
folder   ~/projects/client                                     from conversation 7b1d4e20-0000-4000-8000-000000000002
...
```

## Step 4: read usage per home

Usage belongs to a home's account, not to a profile, so `agx usage` shows one row per home, named after the profile the same rule picks, and lists every profile that shares it. A home whose profiles are all `"billing": "api"` shows `per-token` instead of plan windows.

<!-- snippet: examples/decide-shared-home/session.txt#usage -->
```sh
$ agx usage --cached
personal  personal · claude  ~/.claude
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage

work  work · claude  ~/.claude-work
  no cache: no reading on disk: Claude Code 2.1.323 and later no longer store a usage reading on disk (an existing one is not refreshed); use live usage
```

(The example machine has no usage reading on disk, so `--cached` explains why instead of showing numbers; see [`agx usage`](../commands/usage.md#agx-usage).)

## Try it

```sh
go test -count=1 ./internal/docscheck/ -run 'TestExamples/decide-shared-home$'
```

On your own machine, with a conversation id from `agx conversation list`:

```sh
agx profile list
agx resume --id 3f2a --dry-run
agx profile use --id personal
agx resume --id 3f2a --dry-run
```

## Pitfalls

- `~/.claude has 2 profiles (kimi, personal) and no default; add --profile` (exit 125): the rule found no default and no single plan profile, and there was no terminal to ask. Mark one profile as the default (`agx profile use --id personal`), mark the others `"billing": "api"`, or pass `--profile`.
- `"default": true` is per agent, not per home. If the agent's default profile lives on another home (say `work` on `~/.claude-work`), it does not help `~/.claude`; the plan rule or the question decides there.
- Two `plan` profiles and one `api` profile on a home still ask: the plan rule needs exactly one plan profile.
- The question offers to remember the answer only when the agent has no default profile yet and the run is not a dry run; answering yes writes `"default": true` to that profile.
- `agx resume --profile X` searches only the home of `X`. A conversation stored in another home is then not found, even when another profile on that home would find it.
- Everything a profile adds is per run, but the home is shared: a setting you change inside the agent (its own settings file, a plugin, a login you run there) is seen by every profile on that home. Put per-profile choices in the profile (`env`, `env_refs`, `model`, `args`, the agent block), never in the home's files.
- On a launch, a stored login or `--token-ref` and a credential variable from the command line (`--env ANTHROPIC_API_KEY=…`) are refused together: `--env ANTHROPIC_API_KEY and … both set the credential; pick one`. A profile's own `env_refs` with the home's login, as `kimi` does, is fine.

## Related

- [How agx decides](../how-agx-decides.md#shared-homes): the rule in the full chain.
- [`agx profile add`](../commands/profile.md#agx-profile-add), [`agx profile edit`](../commands/profile.md#agx-profile-edit), [`agx profile use`](../commands/profile.md#agx-profile-use), [`agx resume`](../commands/launch.md#agx-resume).
- [Config reference: profiles](../config.md#profiles) for `default`, `billing`, `env` and `env_refs`.
- [Passing anything to an agent](../launch-options.md#recipe-5-any-environment-variable) for environment variables and references.
- [Folder bindings](folder-bindings.md): choose a profile per folder instead.
- Choosing: [a second account](../choosing/second-account.md) (its own home, a login on your home, or a second profile on it).
- Example: [`examples/decide-shared-home`](../../examples/decide-shared-home/README.md).
