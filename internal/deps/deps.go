// Package deps manages the shared, version-pinned mermaid engine that rendered
// pages reference. The mermaid ESM distribution is fetched from npm at publish
// time, verified against a pinned sha512, and uploaded once to a shared,
// immutable location in the bucket (deps/mermaid/<version>/…) that lives
// outside the expiring flash/ prefixes.
package deps

import (
	"context"
	"fmt"
	"io"
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

// WarnIfAbsent writes a non-fatal warning to stderr when a publish uses mermaid
// but the shared engine is not present in the bucket, so rendered diagrams will
// not display until the engine is published. It is a no-op when mermaid is not
// used, when the lister is nil (presence cannot be checked, e.g. no
// credentials), or when a presence check fails; publishing must never be
// blocked by this diagnostic.
func WarnIfAbsent(ctx context.Context, lister upload.ObjectLister, bucket, version string, usesMermaid bool, stderr io.Writer) {
	if !usesMermaid || lister == nil {
		return
	}
	present, err := Present(ctx, lister, bucket, version)
	if err != nil || present {
		return
	}
	fmt.Fprintf(stderr, "warning: this upload contains a mermaid diagram but the shared engine (mermaid %s) "+ //nolint:errcheck
		"is not published; run 'dollop deps publish' or diagrams will not render\n", version)
}
