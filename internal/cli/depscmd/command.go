package depscmd

import (
	"context"
	"fmt"

	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the deps command tree (publish, status).
// uploader and lister are injected so tests can supply fakes; both may be nil
// when credentials are absent. bucket is the target R2 bucket. version and
// integrity are the pinned mermaid release and its npm sha512 integrity string.
// fetch downloads the tarball; inject an http-backed fetch for production.
func New(
	uploader upload.Uploader,
	lister upload.ObjectLister,
	bucket string,
	version string,
	integrity string,
	fetch deps.FetchFunc,
) cli.Command {
	return cli.Command{
		Name:  "deps",
		Usage: "manage the shared mermaid engine published to the bucket",
		Description: `Publishes and inspects the shared, version-pinned mermaid engine that
rendered pages reference. The engine is fetched from npm, verified against a
pinned checksum, and uploaded once to deps/mermaid/<version>/ — outside the
expiring flash/ prefixes and cached indefinitely. Run 'deps publish' once per
mermaid version; rendered pages then load it from this shared location.`,
		Commands: []*cli.Command{
			publishCommand(uploader, lister, bucket, version, integrity, fetch),
			statusCommand(lister, bucket, version),
		},
	}
}

// copyDirFlag is the hidden integration-testing flag shared by both subcommands.
func copyDirFlag() cli.Flag {
	return &cli.StringFlag{
		Name:   "copy-dir",
		Usage:  "copy files to this local directory instead of uploading to R2 (integration testing only)",
		Hidden: true,
	}
}

func publishCommand(uploader upload.Uploader, lister upload.ObjectLister, bucket, version, integrity string, fetch deps.FetchFunc) *cli.Command {
	return &cli.Command{
		Name:  "publish",
		Usage: "fetch and publish the pinned mermaid engine to the bucket",
		Description: `Downloads the pinned mermaid ESM distribution from npm, verifies its
checksum, and uploads it to deps/mermaid/<version>/. Idempotent: skips upload
when the version is already present unless --force is given.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "re-upload even when the version is already present"},
			copyDirFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			force := cmd.Bool("force")
			copyDir := cmd.String("copy-dir")

			up := uploader
			lst := lister
			if copyDir != "" {
				fmt.Fprintf(cmd.Root().ErrWriter, "note: writing to local directory %s instead of R2\n", copyDir) //nolint:errcheck
				dir := &upload.DirUploader{Root: copyDir}
				up, lst = dir, dir
			}
			if up == nil || lst == nil {
				return cli.Exit(noCredsMessage, 1)
			}

			uploaded, err := deps.Publish(ctx, up, lst, bucket, version, integrity, fetch, force, cmd.Root().ErrWriter)
			if err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("publish failed", 1)
			}

			w := cmd.Root().Writer
			if uploaded {
				fmt.Fprintf(w, "published mermaid %s to %s/\n", version, deps.VersionPrefix(version)) //nolint:errcheck
			} else {
				fmt.Fprintf(w, "mermaid %s already present at %s/\n", version, deps.VersionPrefix(version)) //nolint:errcheck
			}
			return nil
		},
	}
}

func statusCommand(lister upload.ObjectLister, bucket, version string) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "report the shipped mermaid version and whether it is published",
		Flags: []cli.Flag{copyDirFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			copyDir := cmd.String("copy-dir")
			w := cmd.Root().Writer

			fmt.Fprintf(w, "mermaid version: %s\n", version) //nolint:errcheck

			lst := lister
			if copyDir != "" {
				lst = &upload.DirUploader{Root: copyDir}
			}
			if lst == nil {
				fmt.Fprintln(w, "bucket: not configured") //nolint:errcheck
				return nil
			}

			present, err := deps.Present(ctx, lst, bucket, version)
			if err != nil {
				return cli.Exit(fmt.Sprintf("check deps: %v", err), 1)
			}
			if present {
				fmt.Fprintf(w, "bucket: present at %s/\n", deps.VersionPrefix(version)) //nolint:errcheck
			} else {
				fmt.Fprintf(w, "bucket: absent (run 'dollop deps publish')\n") //nolint:errcheck
			}
			return nil
		},
	}
}

const noCredsMessage = "no R2 credentials configured; run 'dollop config set account-id <id>', " +
	"'dollop config auth r2-key <key>', and 'dollop config auth r2-secret <secret>' (or use --copy-dir)"
