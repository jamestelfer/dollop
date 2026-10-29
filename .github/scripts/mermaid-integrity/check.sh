#!/usr/bin/env bash
#
# Fail when a change to the mermaid pin leaves mermaid.sha512 out of step with
# the verified npm tarball. Passes without network access when neither pin
# file changed since the base commit.
#
# Usage: check.sh <base-sha>

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly REPO_ROOT="${SCRIPT_DIR}/../../.."
readonly PIN_FILES=(
  internal/render/mermaid.version
  internal/render/mermaid.sha512
)

# Print a GitHub Actions error annotation to STDERR.
err() {
  printf '::error::%s\n' "$*" >&2
}

main() {
  if (( $# != 1 )); then
    err "usage: check.sh <base-sha>"
    exit 2
  fi
  local base="$1"
  local outputs

  if git -C "${REPO_ROOT}" diff --quiet "${base}" HEAD -- \
    "${PIN_FILES[@]}"; then
    printf 'mermaid pin unchanged since %s; nothing to verify\n' "${base}"
    return 0
  fi

  outputs="$("${SCRIPT_DIR}/verify.sh")"
  printf '%s\n' "${outputs}"
  if grep -qx 'changed=true' <<< "${outputs}"; then
    err "mermaid.sha512 does not match the verified tarball for" \
      "mermaid.version; wait for the mermaid-integrity workflow to update it"
    exit 1
  fi
}

main "$@"
