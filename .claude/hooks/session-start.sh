#!/bin/bash
set -euo pipefail

# Only run in remote Claude Code on the web sessions
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

if [ -n "${GH_RELEASE_DOWNLOAD:-}" ]; then
  export GITHUB_TOKEN="${GH_RELEASE_DOWNLOAD}"
fi

mise trust
mise install
mise reshim

if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  printf 'export PATH="%s/.local/share/mise/shims:$PATH"\n' "$HOME" >> "$CLAUDE_ENV_FILE"
fi

just build || true
