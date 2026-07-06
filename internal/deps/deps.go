// Package deps manages the shared, version-pinned mermaid engine that rendered
// pages reference. The mermaid ESM distribution is fetched from npm at publish
// time, verified against a pinned sha512, and uploaded once to a shared,
// immutable location in the bucket (deps/mermaid/<version>/…) that lives
// outside the expiring flash/ prefixes.
package deps

import (
	"context"
	"fmt"
	"slices"

	"github.com/jamestelfer/dollop/internal/upload"
)

// entrypoint is the ESM entry module extracted from the tarball and uploaded at
// the root of the version prefix. Its presence is the authoritative signal that
// a version has been published.
const entrypoint = "mermaid.esm.min.mjs"

// VersionPrefix returns the bucket key prefix for a published mermaid version,
// rooted at the bucket and outside the expiring flash/ prefixes.
func VersionPrefix(version string) string {
	return "deps/mermaid/" + version
}

// entrypointKey returns the full object key of the ESM entrypoint for a version.
func entrypointKey(version string) string {
	return VersionPrefix(version) + "/" + entrypoint
}

// Present reports whether the shipped mermaid version is published in the
// bucket. It is the single shared presence check reused by `deps status`,
// `deps publish`, the create/update missing-deps warning, and `doctor`.
// Presence is determined by the entrypoint object existing under the version
// prefix.
func Present(ctx context.Context, lister upload.ObjectLister, bucket, version string) (bool, error) {
	keys, err := lister.ListObjects(ctx, bucket, VersionPrefix(version))
	if err != nil {
		return false, fmt.Errorf("list deps for mermaid %s: %w", version, err)
	}
	return slices.Contains(keys, entrypointKey(version)), nil
}
