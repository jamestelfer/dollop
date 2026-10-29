#!/usr/bin/env bash
#
# Write a verified mermaid integrity to mermaid.sha512, commit it, and push it
# to a branch using the token in $GH_TOKEN.
#
# Usage: commit.sh <version> <integrity> <branch>

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly INTEGRITY_FILE="${SCRIPT_DIR}/../../../internal/render/mermaid.sha512"
# Listed in renovate.json5 gitIgnoredAuthors so Renovate keeps managing the PR.
readonly AUTHOR_NAME="github-actions[bot]"
readonly AUTHOR_EMAIL="41898282+github-actions[bot]@users.noreply.github.com"

# Print a GitHub Actions error annotation to STDERR.
err() {
  printf '::error::%s\n' "$*" >&2
}

main() {
  if (( $# != 3 )); then
    err "usage: commit.sh <version> <integrity> <branch>"
    exit 2
  fi
  local version="$1" integrity="$2" branch="$3"
  local auth

  : "${GH_TOKEN:?GH_TOKEN must hold a contents:write token}"
  : "${GITHUB_REPOSITORY:?must run under GitHub Actions}"

  printf '%s\n' "${integrity}" > "${INTEGRITY_FILE}"

  git -c user.name="${AUTHOR_NAME}" -c user.email="${AUTHOR_EMAIL}" \
    commit -m "fix(deps): pin mermaid ${version} integrity" \
    -- "${INTEGRITY_FILE}"

  auth="$(printf 'x-access-token:%s' "${GH_TOKEN}" | base64 -w0)"
  printf '::add-mask::%s\n' "${auth}"
  git -c http.https://github.com/.extraheader="AUTHORIZATION: basic ${auth}" \
    push "https://github.com/${GITHUB_REPOSITORY}.git" \
    "HEAD:refs/heads/${branch}"
}

main "$@"
