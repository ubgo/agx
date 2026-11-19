#!/bin/sh
# A pool's picker: the members and their usage arrive as JSON on stdin
# (never a token); print one member's login name. This one prefers alice
# whenever she is ready, and otherwise takes the built-in rules' pick.
input=$(cat)
case "$input" in
  *'"name":"alice","group":"own","state":"ready"'*) echo alice ;;
  *) printf '%s\n' "$input" | sed -n 's/.*"builtin":"\([^"]*\)".*/\1/p' ;;
esac
