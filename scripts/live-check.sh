#!/bin/sh
# Live check of a login pool against your real logins. Opt-in: it is not
# part of task check or CI. It prints no token value, only its length.
# Steps 1-4 spend at most one probe per pool member (a few tokens each,
# only when the login-usage cache is older than the pool's
# usage_max_age); --with-claude adds one tiny real Claude call through the
# pool (spends a few tokens on whichever member is picked).
#
#   scripts/live-check.sh                    # steps 1-4
#   scripts/live-check.sh --with-claude      # also step 5
#
# Environment: AGX (the agx binary, default agx), AGX_POOL (the pool
# login, default auto), AGX_ARGS (global flags for every call, such as
# "--config $HOME/.agx/config.pool.jsonc").
set -u

agx=${AGX:-agx}
pool=${AGX_POOL:-auto}
args=${AGX_ARGS:-}
fails=0

ok()   { printf '  ok    %s\n' "$1"; }
bad()  { printf '  FAIL  %s\n' "$1"; fails=$((fails + 1)); }
step() { printf '\n%s\n' "$1"; }
# shellcheck disable=SC2086 # AGX_ARGS is a list of flags on purpose.
run()  { command "$agx" $args "$@"; }

step "1. the pool reads its members' usage and picks one"
if name=$(run login pick --id "$pool" --color never --json --field data.selected.name 2>/dev/null); then
  source=$(run login pick --id "$pool" --color never --json --field data.source 2>/dev/null)
  ok "picked $name (source: $source)"
else
  bad "agx login pick exited non-zero (1 = no member usable or an error); run: $agx $args login pick --id $pool"
fi

step "2. the picked member's token resolves (agx login token on the pool)"
len=$(run login token --id "$pool" 2>/dev/null | tr -d '\n' | wc -c | tr -d ' ')
if [ "${len:-0}" -gt 20 ]; then ok "token is $len characters"; else bad "no token (run: $agx $args login token --id $pool | wc -c)"; fi

step "3. a second pick within usage_max_age reuses the cache"
src=$(run login pick --id "$pool" --color never --json --field data.source 2>/dev/null)
case "$src" in
  *"cache ("*) ok "second call used the cache: $src" ;;
  *) bad "expected the cache to be reused, got: $src" ;;
esac

step "4. \"$pool\" is a pool login, and a dry run makes it live"
kind=$(run login show --id "$pool" --json --field data.kind 2>/dev/null)
if [ "$kind" = "pool" ]; then ok "kind pool"; else bad "login $pool is \"$kind\", not a pool"; fi
if run run --login "$pool" --dry-run --quiet --json --field data.live.start_login 2>/dev/null | grep -qx "$pool"; then
  ok "agx run --login $pool is a live run on the pool"
else
  bad "agx run --login $pool --dry-run is not a live pool run"
fi

if [ "${1:-}" = "--with-claude" ]; then
  step "5. a real Claude call through agx run --login $pool (spends a few tokens)"
  dir=$(mktemp -d)
  reply=$(cd "$dir" && run run --login "$pool" --quiet -- -p "Reply with exactly the word OK and nothing else." < /dev/null 2>/dev/null | tail -1)
  command rm -rf "$dir"
  if [ "$reply" = "OK" ]; then ok "Claude replied OK"; else bad "Claude replied: ${reply:-<nothing>}"; fi
fi

printf '\n'
if [ "$fails" -eq 0 ]; then echo "All checks passed."; else echo "$fails check(s) failed."; exit 1; fi
