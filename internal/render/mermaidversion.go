package render

// MermaidVersion is the pinned mermaid release published to the shared deps
// namespace and referenced by rendered pages. It is the single source of truth
// for both the `deps publish` upload target (deps/mermaid/<MermaidVersion>/…)
// and the HTML reference path, so the two cannot diverge.
const MermaidVersion = "11.16.0"

// MermaidSHA512 is the npm integrity string (sha512-<base64>) for the pinned
// mermaid tarball. `deps publish` verifies the downloaded tarball against this
// value before uploading anything; a mismatch aborts the publish.
const MermaidSHA512 = "sha512-Zvm3kbstgdpvIJPPItlL7fppIZ3kibvc1oZIGxdvk9t6UFz6flv+Jw7FtRGKwfcI8OckmH04LqG6LlS6X4B1pA=="
