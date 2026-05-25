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

mise activate bash --shims >> "$CLAUDE_ENV_FILE"

just build || true
