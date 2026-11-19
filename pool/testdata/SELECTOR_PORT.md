# Selector test port

The login pool ports a stand-alone account selector (Node, `agx-pick`) whose test suite had 119 cases in five files. Every case is listed here with the Go test that covers it, or why it does not apply to agx. Names in the selector's snapshot tests are replaced by the fake names of `snapshot-2026-11-18-anon.json` (same numbers, statuses and resets).

Paths are relative to the repository root. `TestPortX/name` is a subtest whose name is the selector test's name.

## selector.test.mjs (49): the rules

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

## Beyond the port

- Boundaries: `pool/boundary_test.go` (`TestBoundary*`): caps exclusive, borrow window to the second, cutoff, missing 5-hour window, reset order, ties, force cases, empty and all-blocked pools.
- Property: `TestPropertyPickIsAlwaysRight` (20 000 random pools) and `FuzzPickIsAlwaysRight`: the unforced pick is usable, under every cap, in the allowed pool, and no allowed member resets sooner.
- Overrides and picker: `internal/app/pool_override_test.go` (`--login` member, `AGX_LOGIN`, `--pool-force`, `agx live use` pin and unpin, picker member, non-member, no line, extra lines, non-zero exit, timeout, stdin JSON without tokens, dry-run sources).
