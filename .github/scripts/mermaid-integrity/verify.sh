#!/usr/bin/env bash
#
# Verify the npm tarball for the pinned mermaid version against its SLSA
# provenance and npm integrity.
#
# Prints version, integrity and changed (whether mermaid.sha512 is stale) as
# key=value lines on STDOUT, in $GITHUB_OUTPUT format. Diagnostics go to STDERR.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly RENDER_DIR="${SCRIPT_DIR}/../../../internal/render"
readonly VERSION_FILE="${RENDER_DIR}/mermaid.version"
readonly INTEGRITY_FILE="${RENDER_DIR}/mermaid.sha512"
readonly REGISTRY="https://registry.npmjs.org"
readonly SLSA_PREDICATE="https://slsa.dev/provenance/v1"
readonly SIGNER_IDENTITY="https://github.com/mermaid-js/mermaid/.github/workflows/release.yml@refs/heads/master"
readonly SIGNER_ISSUER="https://token.actions.githubusercontent.com"

# Print a GitHub Actions error annotation to STDERR.
err() {
  printf '::error::%s\n' "$*" >&2
}

# Print a file's contents with all whitespace removed.
read_trimmed() {
  tr -d '[:space:]' < "$1"
}

# Download the tarball and its SLSA provenance bundle into a directory.
fetch() {
  local version="$1" dir="$2"

  curl -fsSL -o "${dir}/mermaid.tgz" \
    "${REGISTRY}/mermaid/-/mermaid-${version}.tgz"
  curl -fsSL -o "${dir}/attestations.json" \
    "${REGISTRY}/-/npm/v1/attestations/mermaid@${version}"
  jq --arg type "${SLSA_PREDICATE}" \
    '.attestations[] | select(.predicateType == $type) | .bundle' \
    "${dir}/attestations.json" > "${dir}/slsa.json"

  if [[ ! -s "${dir}/slsa.json" ]]; then
    err "mermaid@${version} has no SLSA provenance on npm"
    return 1
  fi
}

# Check the tarball was built by mermaid's own release workflow.
verify_provenance() {
  local dir="$1"

  cosign verify-blob-attestation \
    --bundle "${dir}/slsa.json" \
    --type slsaprovenance1 \
    --certificate-identity "${SIGNER_IDENTITY}" \
    --certificate-oidc-issuer "${SIGNER_ISSUER}" \
    "${dir}/mermaid.tgz" >&2
}

# Print the tarball's integrity after checking it matches npm's record.
tarball_integrity() {
  local version="$1" dir="$2"
  local digest computed registry

  digest="$(openssl dgst -sha512 -binary "${dir}/mermaid.tgz" | base64 -w0)"
  computed="sha512-${digest}"
  registry="$(curl -fsSL "${REGISTRY}/mermaid/${version}" \
    | jq -r .dist.integrity)"

  if [[ "${computed}" != "${registry}" ]]; then
    err "tarball digest ${computed} does not match npm integrity ${registry}"
    return 1
  fi
  printf '%s\n' "${computed}"
}

main() {
  local version current work integrity changed

  version="$(read_trimmed "${VERSION_FILE}")"
  if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    err "mermaid.version does not hold an exact release: '${version}'"
    exit 1
  fi
  current="$(read_trimmed "${INTEGRITY_FILE}")"

  work="$(mktemp -d "${TMPDIR:-/tmp}/mermaid-integrity.XXXXXX")"
  # shellcheck disable=SC2064 # expand work now; it is local to main
  trap "rm -rf '${work}'" EXIT

  fetch "${version}" "${work}"
  verify_provenance "${work}"
  integrity="$(tarball_integrity "${version}" "${work}")"

  changed=true
  if [[ "${integrity}" == "${current}" ]]; then
    changed=false
  fi

  printf 'version=%s\n' "${version}"
  printf 'integrity=%s\n' "${integrity}"
  printf 'changed=%s\n' "${changed}"
}

main "$@"
