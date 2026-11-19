# Selector test port

The login pool ports a stand-alone account selector (Node, `agx-pick`), matched at its commit `f7e17c6` ("Make account picking strict and prove it with randomised and mutation tests"). Its suite has 144 cases in six files: `selector.test.mjs` (68), `index.test.mjs` (26), `tokens.test.mjs` (19), `cli.test.mjs` (13), `flow.test.mjs` (12) and `property.test.mjs` (6), plus a mutation check (41 breaks). Every case is listed here with the Go test that covers it, or why it does not apply to agx. Names in the selector's snapshot tests are replaced by the fake names of `snapshot-2026-11-18-anon.json` (same numbers, statuses and resets).

Paths are relative to the repository root. `TestPortX/name` is a subtest whose name is the selector test's name.

## selector.test.mjs (68): the rules

| # | Selector test | Go test |
|---|---|---|
| 1 | own account with sooner reset wins over config order | `pool/selector_port_test.go` `TestPortRanking/own_account_with_sooner_reset_wins_over_config_order` |
| 2 | shared within borrow window goes ahead of own with later reset | `TestPortRanking/shared_within_borrow_window…` |
| 3 | shared outside borrow window waits while an own account is usable | `TestPortRanking/shared_outside_borrow_window…` |
| 4 | own with sooner reset beats shared inside the window | `TestPortRanking/own_with_sooner_reset…` |
| 5 | borrow window boundary: exactly 48h is inside, just over is outside | `TestPortRanking/borrow_window_boundary…`, plus `TestBoundaryBorrowWindowSeconds` (48h ± 1s) |
| 6 | borrow window 0 means shared never goes before a usable own account | `TestPortRanking/borrow_window_0…`, `pool/pool_test.go` `TestBorrowWindowZeroNeverBorrows` |
| 7 | no own account usable: soonest shared wins regardless of window | `TestPortRanking/no_own_account_usable…` |
| 8 | tie on reset: own before shared, then config order | `TestPortRanking/tie_on_reset…`, `TestBoundaryTies` |
| 9 | unknown reset sorts after known resets | `TestPortRanking/unknown_reset…`, `TestBoundaryResetOrder` |
| 10 | passed reset is treated as stale: sorts last and is noted | `TestPortRanking/passed_reset…`, `TestBoundaryResetOrder` |
| 11 | stale shared reset is not inside the borrow window | `TestPortRanking/stale_shared_reset…` |
| 12 | shared 5h at 50% is blocked, 49.9% is usable | `TestPortCaps/shared_5h_at_50%…`, `TestBoundaryCapsExclusive` (cap − 0.0001) |
| 13 | shared week at 70% is blocked even if it resets in 1 hour | `TestPortCaps/shared_week_at_70%…` |
| 14 | shared 5h over cap is blocked even if week resets today | `TestPortCaps/shared_5h_over_cap…` |
| 15 | shared cap counts total usage: teammate at 60% leaves 10% room | `TestPortCaps/shared_cap_counts_total_usage…` |
| 16 | own 5h at 99% hits the safety cutoff | `TestPortCaps/own_5h_at_99%…`, `TestBoundaryShortCutoff` |
| 17 | own week at 100% is blocked even with status ok | `TestPortCaps/own_week_at_100%…` |
| 18 | safety cutoff applies even when an account cap is higher | `TestPortCaps/safety_cutoff_applies…`, `TestShortCutoffAppliesToOwnAccounts` |
| 19 | a window agx does not report is not capped | `TestPortCaps/a_window_agx_does_not_report…`, `TestBoundaryMissingShortWindow` |
| 20 | defaults apply to accounts without an entry, and can be changed | `TestPortCaps/defaults_apply…` |
| 21 | per-account entry overrides defaults | `TestPortCaps/per-account_entry…` |
| 22 | all reasons are listed, not only the first | `TestPortCaps/all_reasons_are_listed…`, `TestUsableChecksListEveryReason` |
| 23 | non-ok status is skipped | `TestPortSkipping/non-ok_status_is_skipped` |
| 24 | missing account is skipped | `TestPortSkipping/missing_account_is_skipped` (reason reads `no usage reading`, agx's wording for `not found in agx result`) |
| 25 | other agent and non-login records are ignored | Does not apply: agx reads each member's own usage with the pool's agent, so there are no records of other agents or kinds to filter. The equivalent guard is at load: a member serving another agent is refused, `config/pool_test.go` `TestPoolProblems/other_agent's_member`. |
| 26 | agent unset considers every agent | Does not apply: a pool always names exactly one agent (`TestPoolProblems/two_agents`). |
| 27 | enabled: false skips the account | `TestPortSkipping/enabled:_false_skips_the_account` |
| 28 | disabled account missing from agx still says disabled | `TestPortSkipping/disabled_account_missing…` |
| 29 | disabled own accounts do not hold shared accounts back | `TestPortSkipping/disabled_own_accounts…` |
| 30 | enabled: true behaves like no entry | `TestPortSkipping/enabled:_true…` |
| 31 | nothing usable returns selected null | `TestPortSkipping/nothing_usable…`, `TestNoneUsable`, `TestBoundaryEmptyAndAll` |
| 32 | config force ignores every rule and reports why it would be skipped | `TestPortForce/config_force…` |
| 33 | --force option overrides config force | `TestPortForce/--force_option_overrides_config_force` (agx's flag is `--pool-force`) |
| 34 | force overrides enabled: false | `TestPortForce/force_overrides_enabled:_false`, `TestBoundaryForceCases` |
| 35 | force may pick an account not listed in own/shared, with a warning | `TestPortForce/force_may_pick_an_account_not_listed…` |
| 36 | force of a name not in agx data is an error | `TestPortForce/force_of_a_name_not_in_agx_data…`; through the CLI, `internal/cli/login_pick_test.go` `TestLoginPickFlags` (unknown login, exit 1) |
| 37 | force: null means normal selection | `TestPortForce/force:_null…`, `config/pool_test.go` `TestPoolLoads` (null loads) |
| 38 | candidates are still ranked when forced | `TestPortForce/candidates_are_still_ranked_when_forced` |
| 39 | missing own/shared arrays | `config/pool_port_test.go` `TestPortConfigValidation/missing_own/shared_arrays` (agx: either may be omitted; a pool with no member is refused) |
| 40 | duplicate names across own and shared | `TestPortConfigValidation/duplicate_names…` |
| 41 | old setting names are rejected so nothing is silently ignored | `TestPortConfigValidation/old_setting_names…` (unknown keys at their line) |
| 42 | force must be a string or null | `TestPortConfigValidation/force_must_be_a_string_or_null` |
| 43 | bad borrow window | `TestPortConfigValidation/bad_borrow_window`, `…(a_number)` |
| 44 | accounts entry for an unlisted name is a warning | `TestPortConfigValidation/accounts_entry_for_an_unlisted_name_is_an_error`: deliberately an error in agx (a typo must not be ignored silently) |
| 45 | failed agx response is rejected | Does not apply as written (agx reads no agx JSON); the equivalent, a failed probe with no cache, blocks the member and is never cached: `internal/app/pool_override_test.go` `TestPoolProbeWithoutCacheBlocks` |
| 46 | same input twice gives the same result and inputs are not mutated | `TestPortStability/same_input_twice…` |
| 47 | picks grace, then ivan, carol, alice (names anonymised) | `TestPortSnapshot/picks_grace,_then_ivan,_carol,_alice` |
| 48 | with no borrow window, own accounts come first by reset | `TestPortSnapshot/with_no_borrow_window…` |
| 49 | after grace and ivan are disabled, carol is next (names anonymised) | `TestPortSnapshot/after_grace_and_ivan_are_disabled,_carol_is_next` |

### Added at f7e17c6: strict config, odd data, defaults (19)

| # | Selector test | Go test |
|---|---|---|
| 50 | unknown top-level setting (typo) is an error | `config/pool_port_test.go` `TestPortStrictConfig/unknown_top-level_setting…` (schema strict keys, at the line) |
| 51 | unknown key inside accounts or defaults is an error | `TestPortStrictConfig/unknown_key_inside_accounts_or_defaults…` |
| 52 | enabled must be a real boolean | `TestPortStrictConfig/enabled_must_be_a_real_boolean` (`"false"`, `0`, `null`) |
| 53 | caps must be numbers from 0 to 100 | `TestPortStrictConfig/caps_must_be_numbers_from_0_to_100` (`"50"`, `"abc"`, `null`, −1, 101, an overflowing number; JSON has no NaN or Infinity, and `pool.Config.Validate` refuses both: `pool/pool_test.go` `TestValidate/NaN_cap`, `…/infinite_cutoff`) |
| 54 | other settings are type-checked | `TestPortStrictConfig/other_settings_are_type-checked` (cutoff 120, `"99"`, `null`; window `48`; `force ""`; `accounts []`, `{"alice": 70}`; `defaults []`; agent `""`) |
| 55 | a valid full config passes | `TestPortStrictConfig/a_valid_full_config_passes` |
| 56 | cap 0 always blocks; cap 100 blocks only at 100 | `pool/selector_port_test.go` `TestPortOddData/cap_0_always_blocks…` |
| 57 | usage that isn't a number blocks the account instead of skipping the cap | `TestPortOddData/usage_that_isn't_a_number…` (NaN, ±Inf; reason `week usage is unreadable`, agx's wording without "in agx result") |
| 58 | windows missing or not a list: account usable, reset unknown, sorted last | `TestPortOddData/windows_missing_or_not_a_list…` (a reading without windows; "not a list" cannot occur in a typed reading) |
| 59 | unreadable reset time: noted, treated as unknown, sorted last | `TestPortOddData/unreadable_reset_time…` (a zero time); in the cache file an unreadable time makes the whole cache unreadable: it is ignored with a warning and rewritten (`login/usagecache_test.go` `TestUsageCacheBrokenOrForeign`) |
| 60 | both resets unknown: config order | `TestPortOddData/both_resets_unknown…` |
| 61 | duplicate login record: first one used, with a warning | `TestPortOddData/duplicate_login_record…`: does not apply as written; readings are a map keyed by login name and the cache and config are JSON objects, so one reading per name holds by type |
| 62 | record order from agx never changes the pick | `TestPortOddData/record_order_from_agx_never_changes_the_pick` (200 runs over Go's randomised map order) |
| 63 | forced name that exists only for another agent is an error | `TestPortOddData/forced_name_that_exists_only_for_another_agent…` (`ErrForcedMissing`); at load, `config/pool_test.go` `TestPoolProblems/other_agent's_member` and the force check |
| 64 | bad now is an error, not a silent wrong order | `TestPortOddData/bad_now…` (`ErrBadNow` for a zero `Options.Now`); CLI `--now yesterday`: `TestLoginPickFlags` |
| 65 | other statuses all block | `TestPortOddData/other_statuses_all_block` |
| 66 | 5h safety cutoff defaults to 99% | `TestPortDefaults/5h_safety_cutoff_defaults_to_99%` |
| 67 | borrow window defaults to 48h | `TestPortDefaults/borrow_window_defaults_to_48h` |
| 68 | caps default to 5h 50% and week 70% | `TestPortDefaults/caps_default_to_5h_50%_and_week_70%` |

## property.test.mjs (6): randomised, against an independent implementation

`pool/property_port_test.go` `TestPortRandomised`: seeded (`SEED=n` replays one, `CASES=n` runs more; 4000 by default, 50 000 also run), with a reference implementation written separately in the test. Generated readings include missing ones, no windows, usage that is not a number, stale and unreadable resets and bad statuses; the selector's records of other agents and kinds have no agx equivalent (one reading per member).

| # | Selector test | Go test |
|---|---|---|
| 1 | generated configs are valid (the generator itself is sound) | `TestPortRandomised/generated_configs_are_valid…` |
| 2 | pick and full pick order match the independent reference | `TestPortRandomised/pick_and_full_pick_order_match…` |
| 3 | invariants hold for every case | `TestPortRandomised/invariants_hold_for_every_case`, plus `pool/boundary_test.go` `TestPropertyPickIsAlwaysRight` and `FuzzPickIsAlwaysRight` |
| 4 | agx record order never changes the result (unique records) | `TestPortRandomised/agx_record_order_never_changes…` |
| 5 | force always returns the forced account, whatever its state | `TestPortRandomised/force_always_returns_the_forced_account…` |
| 6 | selection is pure: same input, same output, inputs untouched | `TestPortRandomised/selection_is_pure…` |

## index.test.mjs (26): the library and usage source

| # | Selector test | Go test |
|---|---|---|
| 1 | library exposes the high- and low-level API | Public package `pool` (`Select`, `New`, `Config.Validate`, `Config.Reasons`, `CapsFor`, `Enabled`, `Hours`), compiled by every test in `pool/`. |
| 2 | package name resolves to index.mjs | Does not apply (Go import path `github.com/ubgo/agx/pool`). |
| 3 | default config is the package's config.json | Does not apply: the pool is a login in `~/.agx/config.jsonc`; the cache follows the config dir, `internal/cli/login_pick_test.go` `TestLoginPickCacheFollowsConfigDir`. |
| 4 | with a report and config object: no files touched | `pool.Select` is pure (all of `pool/`); `TestPortStability`. |
| 5 | now accepts Date, ms and ISO string | `pool.Options.Now` (`time.Time`); through the CLI, the hidden `--now` takes RFC 3339: `TestLoginPickFlags`. |
| 6 | bad now throws SelectorError | `TestLoginPickFlags` (`--now yesterday`, exit 2) |
| 7 | force option wins over config.force | `TestPortForce/--force_option_overrides_config_force`, `pool/pool_test.go` `TestFlowExamples/E_--force_wins_over_config_force` |
| 8 | errors throw SelectorError instead of exiting | `TestForcedMissingIsAnError` (`ErrForcedMissing`), `TestValidate` |
| 9 | configPath: live run writes the cache next to it, cached reads it back | `internal/app/pool_test.go` `TestPoolPickProbesOnceThenReusesCache`, `TestPoolProbeFailureFallsBackToCache` (`--cached`) |
| 10 | explicit cachePath is used | `login/usagecache_test.go` `TestUsageCacheRoundTrip` (the cache path comes from the config dir); `TestLoginPickCacheFollowsConfigDir` |
| 11 | input path is read without running agx | `--cached` reads a seeded cache without probing: `TestLoginPickGoldens`, `examples/acct-login-pool` |
| 12 | same call twice gives the same pick | `TestPortStability`, `TestPoolPickProbesOnceThenReusesCache` (second pick) |
| 13 | failed agx falls back to the cache with a warning | `TestPoolProbeFailureFallsBackToCache`, `TestPortFlow/7._…` |
| 14 | failed agx without cache throws | `TestPoolProbeWithoutCacheBlocks` (agx: the member is blocked with `status is network`; no member usable is exit 1) |
| 15 | ok:false from agx does not overwrite the cache | `TestPoolProbeWithoutCacheBlocks` (a failed probe is not cached), `TestPoolProbeFailureFallsBackToCache` |
| 16 | input and cached together is an error | Does not apply (agx has no `--input`). |
| 17 | missing command is an error | Does not apply (agx probes itself; there is no usage command to configure). |
| 18 | timeout kills a hung agx and falls back | `internal/app/pool_override_test.go` `TestPoolProbeTimeoutFallsBack` |
| 19 | fresh cache is reused without running agx | `TestPoolPickProbesOnceThenReusesCache`, `TestPortFlow/2._…` |
| 20 | stale cache runs agx and refreshes the cache | `TestPortFlow/4._…`, `TestPoolMaxAge` |
| 21 | no cache runs agx | `TestPortFlow/1._…` |
| 22 | maxAge 0 always runs agx | `TestPoolMaxAge` (`--max-age 0`; a reading exactly max-age old is stale) |
| 23 | bad maxAge is an error | `TestLoginPickFlags` (`--max-age -5s`, exit 2); `config/pool_test.go` `TestPoolProblems` (bad durations) |
| 24 | pickAccount passes maxAge through | `TestPoolMaxAge` |
| 25 | cacheAgeSeconds is null without a cache | `TestUsageCacheRoundTrip` (missing cache reads empty); `age_seconds` is null for a live reading (`login-pick-json` golden) |
| 26 | reads the shipped config.json and it is valid | `examples/acct-login-pool` (its `config.jsonc` loads in every run) |

## tokens.test.mjs (19): tokens

| # | Selector test | Go test |
|---|---|---|
| 1 | returns what agx login token prints, trimmed | `internal/cli/login_token_test.go` `TestLoginTokenPrintsOnlyTheValue` |
| 2 | agxConfig is passed to agx as --config | Does not apply (no subprocess); the global `--config` picks the config: `TestLoginPickCacheFollowsConfigDir` |
| 3 | agx error: its one-line message, as SelectorError | `TestLoginTokenErrors` |
| 4 | empty output is an error | `internal/cli/live_test.go` `TestCredentialNeverEmpty`, `internal/cli/login_test.go` `TestLoginAddRefusals` |
| 5 | hung agx times out | `login/login_test.go` `TestEdge16GopassTimeout` (a reference that hangs) |
| 6 | missing agx binary is reported clearly | Does not apply (agx is the binary). |
| 7 | a name is required | `TestLoginTokenErrors` (`--id` missing, exit 2) |
| 8 | returns the picked name and its token | `internal/app/pool_test.go` `TestPoolLoginToken`, `internal/cli/login_pick_test.go` `TestLoginTokenOnPool` |
| 9 | force picks the forced account's token | `internal/app/pool_override_test.go` `TestPoolTokenForce`, `TestLoginTokenOnPool` |
| 10 | nothing usable gives null name and token, and agx is not called | `TestLoginTokenOnPool` (exit 1, empty stdout), `TestPoolCredentialNoneUsableIsOneLine` |
| 11 | token <login> prints only the token on stdout | `TestLoginTokenPrintsOnlyTheValue` |
| 12 | token picks the account: token on stdout, name on stderr | `TestLoginTokenOnPool`: stdout carries the token; agx prints no name on stderr (stderr stays empty for a key helper; the pool's `log` and `agx live list` show the pick) |
| 13 | token with --force uses the forced account | `TestLoginTokenOnPool` (`--pool-force`) |
| 14 | --agx-config is passed through to agx | Does not apply (no subprocess). |
| 15 | agx error: exit 1, agx's message, empty stdout | `TestLoginTokenErrors` |
| 16 | nothing usable exits 2 with empty stdout | `TestLoginTokenOnPool` (agx exits 1, code `auto-pick`; 2 is a usage error in agx) |
| 17 | --reveal is accepted with token and refused without it | `TestLoginTokenTerminal` |
| 18 | token with --json is refused | `TestLoginTokenRefusesJSONAndField` |
| 19 | on a terminal, token refuses without --reveal | `TestLoginTokenTerminal` |

## cli.test.mjs (13): the report command

| # | Selector test | Go test |
|---|---|---|
| 1 | --input prints the pick and exits 0 | `internal/cli/login_pick_test.go` `TestLoginPickPlainShape`, `TestLoginPickGoldens` |
| 2 | --json is valid JSON with the same pick | `TestLoginPickGoldens` (`login-pick-json`), `TestLoginPickFlags` (`--field`) |
| 3 | --force prints FORCED with warnings | `TestLoginPickGoldens` (`login-pick-forced`) |
| 4 | config force works the same way | `examples/acct-login-pool` region `force`, `TestPortFlow/9._…` |
| 5 | forcing an unknown account exits 1 | `TestLoginPickFlags` |
| 6 | nothing usable exits 2 | `TestLoginPickNoneUsable` (exit 1, code `auto-pick`, the selector's line) |
| 7 | invalid config exits 1 | `config/pool_port_test.go` `TestPortConfigValidation` (a config problem; exit 2 in agx) |
| 8 | bad arguments exit 1 | `TestLoginPickFlags` (exit 2 in agx) |
| 9 | live run caches the result, --cached reads it back | `TestPoolPickProbesOnceThenReusesCache`, `TestPoolProbeFailureFallsBackToCache` |
| 10 | failed agx falls back to cache and says so | `TestPoolProbeFailureFallsBackToCache` (`warning: probe of … failed`), `TestPoolReportText` (the warning line) |
| 11 | agx returning ok:false does not overwrite the cache | `TestPoolProbeWithoutCacheBlocks` |
| 12 | failed agx without a cache exits 1 | `TestPoolProbeWithoutCacheBlocks` |
| 13 | default config.json and .cache.json live next to the script, not in cwd | `TestLoginPickCacheFollowsConfigDir` |

## flow.test.mjs (12): the key-helper flow

| # | Selector test | Go test |
|---|---|---|
| 1–12 | the twelve steps, from "first call: no cache" to "the log records every call and never contains a token" | `internal/app/pool_flow_test.go` `TestPortFlow/1._…` to `TestPortFlow/12._…`, same names and order, through `agx credential` on a pool run with a moving clock and an httptest usage server. Step 10 asserts agx's exit 1 for "nothing usable" (the selector's 2). |

## Mutation check (scripts/mutation-check.mjs, 41 breaks)

`pool/mutation_test.go` (build tag `mutation`, run with `task mutation`, not in `task check`) copies the repository to a temporary folder, applies one break at a time to `pool/pool.go` or to the pool validation in `config/`, runs the pool and config tests, and expects them to fail. It has 46 breaks: the selector's 41 kinds adapted to the Go code, and the agx-only checks. The selector's breaks of JSON type checks (enabled, cap types, unknown cap keys) are one break here, dropping the schema's strict keys, since agx's schema types every key. Its "duplicates: last record wins", "other agents not filtered" and "non-login records not filtered" have no agx code to break (one reading per member, read with the pool's agent); in their place: room ignores the cap, an unreadable reset counted as a deadline, NaN caps allowed, an accounts entry for an unlisted name allowed, unknown member, member of another agent, a `null` cap allowed. Every break must be caught; the last run caught all of them.

## Beyond the port

- Boundaries: `pool/boundary_test.go` (`TestBoundary*`): caps exclusive, borrow window to the second, cutoff, missing 5-hour window, reset order, ties, force cases, empty and all-blocked pools.
- Property: `TestPropertyPickIsAlwaysRight` (20 000 random pools) and `FuzzPickIsAlwaysRight`: the unforced pick is usable, under every cap, in the allowed pool, and no allowed member resets sooner.
- Overrides and picker: `internal/app/pool_override_test.go` (`--login` member, `AGX_LOGIN`, `--pool-force`, `agx live use` pin and unpin, picker member, non-member, no line, extra lines, non-zero exit, timeout, stdin JSON without tokens, dry-run sources).
