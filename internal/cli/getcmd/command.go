package getcmd

import (
	"context"
	"fmt"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the get command. reader, bucket, and baseURL are injected so
// tests can supply fakes; reader is nil when no R2 credentials are configured.
func New(reader upload.ObjectReader, bucket, baseURL string) cli.Command {
	return cli.Command{
		Name:      "get",
		Usage:     "download an upload, or a file or directory within it",
		ArgsUsage: "<url-or-path> [target-dir]",
		Description: `Downloads from a single upload into a local directory (default: the
current directory). The first argument is the full URL printed by 'create'
or a bare path: flash/<days>/<id> or keep/<name>, optionally followed by a
path within the upload.

  dollop get https://drop.example.com/flash/7/abc123/        # whole upload
  dollop get flash/7/abc123/docs out/                        # one directory
  dollop get keep/happy-cat/report.pdf                       # one file

A path naming a file downloads just that file into the target directory;
otherwise everything under the path is downloaded, keeping its layout.
Objects are downloaded exactly as stored, including rendered .html pages.

Only a single upload can be downloaded: other prefixes and the bucket root
are refused, as is any path containing empty, '.' or '..' segments.
Existing files are never overwritten unless --force is given; all targets
are checked before anything is downloaded.

Progress is written to stderr; the downloaded file or target directory is
printed to stdout.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "overwrite existing files in the target directory",
			},
			&cli.StringFlag{
				Name:   "copy-dir",
				Usage:  "read from this local directory instead of R2 (integration testing only)",
				Hidden: true,
			},
		},
		Action: newAction(reader, bucket, baseURL),
	}
}

func newAction(reader upload.ObjectReader, bucket, baseURL string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		n := cmd.Args().Len()
		if n < 1 || n > 2 {
			return cli.Exit("usage: get <url-or-path> [target-dir]", 1)
		}
		target := "."
		if n == 2 {
			target = cmd.Args().Get(1)
		}

		ref, err := upload.ResolveGetRef(baseURL, cmd.Args().Get(0))
		if err != nil {
			return cli.Exit(fmt.Sprintf("resolve upload reference: %v", err), 1)
		}

		active := reader
		if copyDir := cmd.String("copy-dir"); copyDir != "" {
			fmt.Fprintf(cmd.Root().ErrWriter, "note: reading from local directory %s instead of R2\n", copyDir) //nolint:errcheck
			active = &upload.DirUploader{Root: copyDir}
		}
		if active == nil {
			return cli.Exit("no R2 credentials configured; run 'dollop config set account-id <id>', "+
				"'dollop config auth r2-key <key>', and 'dollop config auth r2-secret <secret>' (or use --copy-dir)", 1)
		}

		out, err := upload.Download(ctx, active, bucket, ref, target, cmd.Bool("force"), cmd.Root().ErrWriter)
		if err != nil {
			fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
			return cli.Exit("download failed", 1)
		}

		if _, err := fmt.Fprintln(cmd.Root().Writer, out); err != nil {
			return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
		}
		return nil
	}
}
