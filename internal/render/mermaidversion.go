package render

import (
	_ "embed"
	"strings"
)

var (
	//go:embed mermaid.version
	mermaidVersionFile string
	//go:embed mermaid.sha512
	mermaidSHA512File string
)

// MermaidVersion is the pinned mermaid release published to the shared deps
// namespace and referenced by rendered pages. It is the single source of truth
// for both the `deps publish` upload target (deps/mermaid/<MermaidVersion>/…)
// and the HTML reference path, so the two cannot diverge. It lives in its own
// file so Renovate can bump it without touching Go source.
var MermaidVersion = strings.TrimSpace(mermaidVersionFile)

// MermaidSHA512 is the npm integrity string (sha512-<base64>) for the pinned
// mermaid tarball. `deps publish` verifies the downloaded tarball against this
// value before uploading anything; a mismatch aborts the publish. The
// mermaid-integrity workflow rewrites its file after a Renovate version bump.
var MermaidSHA512 = strings.TrimSpace(mermaidSHA512File)
