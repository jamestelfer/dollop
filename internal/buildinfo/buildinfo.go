// Package buildinfo exposes version information stamped into the binary at
// build time.
package buildinfo

import (
	"fmt"
	"io"
)

// Version is the binary version. It defaults to "dev" for un-stamped builds and
// is overridden at link time via:
//
//	-X github.com/jamestelfer/dollop/internal/buildinfo.Version=<version>
//
// goreleaser sets the released tag; "just build" sets a dev prerelease.
var Version = "dev"

// Fprint writes the version line ("<name> <version>") to w. It is the single
// formatting point so the --version flag output stays consistent.
func Fprint(w io.Writer, name string) error {
	if _, err := fmt.Fprintf(w, "%s %s\n", name, Version); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}
