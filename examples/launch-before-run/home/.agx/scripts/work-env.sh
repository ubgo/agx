#!/bin/sh
# #region work-env
# Runs before every launch of the work profile. Its output is discarded;
# a non-zero exit stops the launch.
test -d "$HOME/.claude-work" || { echo "work home missing" >&2; exit 1; }
# #endregion
