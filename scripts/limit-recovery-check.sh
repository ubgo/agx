#!/bin/sh
# Live test: what happens to a running Claude session when its account is
# at its limit and the pool switches to a working one. Opt-in: not part of
# task check or CI. It adds a temporary pool login of two of your own
# logins (forced to the limited one), runs one Claude session with four
# messages, forces the working login after the first, and removes the
# temporary login at the end. Spends a few tokens on the working account.
# Prints no token value.
#
#   scripts/limit-recovery-check.sh LIMITED WORKING
#
# LIMITED is one of your logins at its limit, WORKING a usable one
# (agx login pick --id auto shows both). Environment: AGX (default agx),
# AGX_ARGS (global flags, such as "--config $HOME/.agx/config.pool.jsonc").
set -u

agx=${AGX:-agx}
args=${AGX_ARGS:-}
# shellcheck disable=SC2086 # AGX_ARGS is a list of flags on purpose.
run() { command "$agx" $args "$@"; }

limited=${1:-}
working=${2:-}
if [ -z "$limited" ] || [ -z "$working" ]; then
  echo "usage: scripts/limit-recovery-check.sh LIMITED WORKING   (agx login pick --id auto shows candidates)"
  exit 2
fi
tmp=$(mktemp -d)
pool="pool-limittest-$$"
trap 'run login remove --id "$pool" >/dev/null 2>&1; command rm -rf "$tmp"' EXIT

run login add --name "$pool" --agent claude --kind pool --pool-own "$limited" --pool-own "$working" >/dev/null || exit 1
run config set --key "logins.$pool.pool.key_ttl" --value 5s >/dev/null || exit 1
run config set --key "logins.$pool.pool.usage_max_age" --value 5s >/dev/null || exit 1
run config set --key "logins.$pool.pool.log" --value "$tmp/helper.log" >/dev/null || exit 1
run login edit --id "$pool" --pool-force "$limited" >/dev/null || exit 1
echo "Limited: $limited   Working: $working   Temporary pool: $pool"

msg() { printf '{"type":"user","message":{"role":"user","content":"Reply with exactly the word %s and nothing else."}}\n' "$1"; }
echo "One Claude session, 4 messages; the pool switches to $working after message 1 (about 80s)..."
(
  msg FIRST; sleep 10
  run login edit --id "$pool" --pool-force "$working" >/dev/null; sleep 8
  msg SECOND; sleep 15
  msg THIRD; sleep 15
  msg FOURTH; sleep 25
) | (cd "$tmp" && run run --login "$pool" --quiet -- -p --input-format stream-json --output-format stream-json --verbose 2>/dev/null) > "$tmp/out.jsonl"

n=0
grep '"type":"result"' "$tmp/out.jsonl" | while IFS= read -r line; do
  n=$((n + 1))
  case "$line" in
    *'"is_error":true'*) state="FAILED" ;;
    *) state="ok    " ;;
  esac
  text=$(printf '%s' "$line" | sed -n 's/.*"result":"\([^"]*\)".*/\1/p' | cut -c1-80)
  printf '  message %d: %s  %s\n' "$n" "$state" "$text"
done
echo "Key requests (time, pick):"
sed -E 's/^([^ ]+) .*picked=("[^"]*"|null).*/  \1  \2/' "$tmp/helper.log"
cat <<'MSG'

Expected (Claude Code 2.1.328): message 1 fails (429); message 2 also fails although the
pool already returned the working account (Claude refreshes the key in the background and
uses it from the next request); messages 3 and 4 succeed. Same session, no restart.
MSG
