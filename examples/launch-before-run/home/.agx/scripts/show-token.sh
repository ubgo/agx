#!/bin/sh
# #region show-token
# Says whether the login token reached the script, then stops the launch.
if [ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]; then seen=visible; else seen=hidden; fi
echo "token $seen" >&2
exit 1
# #endregion
