#!/usr/bin/env bash
#
# Commit a verified mermaid integrity to mermaid.sha512 on a branch through
# the GitHub API, using the token in $GH_TOKEN. GitHub signs API-created
# commits as the token's app, which the branch ruleset requires.
#
# Usage: commit.sh <version> <integrity> <branch>

set -euo pipefail

readonly INTEGRITY_PATH="internal/render/mermaid.sha512"
# shellcheck disable=SC2016 # $input is a GraphQL variable, not a shell one
readonly MUTATION='
mutation($input: CreateCommitOnBranchInput!) {
  createCommitOnBranch(input: $input) { commit { url } }
}'

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
  local head contents

  : "${GH_TOKEN:?GH_TOKEN must hold a contents:write token}"
  : "${GITHUB_REPOSITORY:?must run under GitHub Actions}"

  # The commit is rejected if the branch has moved past the verified checkout.
  head="$(git rev-parse HEAD)"
  contents="$(printf '%s\n' "${integrity}" | base64 -w0)"

  jq -n \
    --arg query "${MUTATION}" \
    --arg repo "${GITHUB_REPOSITORY}" \
    --arg branch "${branch}" \
    --arg head "${head}" \
    --arg headline "fix(deps): pin mermaid ${version} integrity" \
    --arg path "${INTEGRITY_PATH}" \
    --arg contents "${contents}" \
    '{query: $query, variables: {input: {
      branch: {repositoryNameWithOwner: $repo, branchName: $branch},
      expectedHeadOid: $head,
      message: {headline: $headline},
      fileChanges: {additions: [{path: $path, contents: $contents}]}
    }}}' \
    | gh api graphql --input - \
      --jq '.data.createCommitOnBranch.commit.url'
}

main "$@"
