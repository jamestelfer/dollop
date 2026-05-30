package createcmd

import (
	"context"
	"fmt"

	"github.com/jamestelfer/dollop/internal/cli/urlout"
	"github.com/jamestelfer/dollop/internal/render"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the create command.
// uploader, bucket, and baseURL are injected so tests can supply fakes.
// newID generates the nanoid for ephemeral uploads; newName generates the
// petname for permanent uploads.
func New(
	uploader upload.Uploader,
	bucket string,
	baseURL string,
	newID func() (string, error),
	newName func() string,
) cli.Command {
	return cli.Command{
		Name:      "create",
		Usage:     "publish a file or directory as a shareable browser link",
		ArgsUsage: "<path>",
		Description: `Publishes a local file or directory and prints the shareable URL to
stdout. The link is immediately accessible in a browser. Upload progress
is written to stderr.

By default, links expire after 1 day. Use --days to extend the window;
allowed values are 1, 7, and 14.

  dollop create report.pdf               # link expires in 1 day
  dollop create --days 7 archive.zip     # link expires in 7 days
  dollop create --days 14 backup/        # directory; link expires in 14 days

Use --keep when the link should never expire. The URL will contain a
memorable two-word petname instead of a random ID. --keep and --days
are mutually exclusive.

  dollop create --keep notes.txt
  dollop create --keep project/`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "index",
				Usage: "generate and upload an index.html listing all uploaded files (skipped with a warning if index.html already exists)",
			},
			&cli.BoolFlag{
				Name:  "no-render",
				Usage: "disable automatic rendering of .md files to .html",
			},
			&cli.StringFlag{
				Name:   "copy-dir",
				Usage:  "copy files to this local directory instead of uploading to R2 (integration testing only)",
				Hidden: true,
			},
		},
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{
				Flags: [][]cli.Flag{
					{
						&cli.IntFlag{
							Name:  "days",
							Usage: "number of days before the link expires; allowed values: 1, 7, 14",
							Value: 1,
						},
					},
					{
						&cli.BoolFlag{
							Name:  "keep",
							Usage: "publish to a permanent link with a memorable petname (no expiry)",
						},
					},
				},
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return cli.Exit("usage: create [--days N | --keep] <path>", 1)
			}

			days := cmd.Int("days")
			keep := cmd.Bool("keep")
			genIndex := cmd.Bool("index")
			noRender := cmd.Bool("no-render")
			copyDir := cmd.String("copy-dir")
			localPath := cmd.Args().Get(0)

			activeUploader := uploader
			if copyDir != "" {
				fmt.Fprintf(cmd.Root().ErrWriter, "note: writing to local directory %s instead of R2\n", copyDir) //nolint:errcheck
				activeUploader = &upload.DirUploader{Root: copyDir}
			}

			if activeUploader == nil {
				return cli.Exit("no R2 credentials configured; run 'dollop config set account-id <id>', "+
					"'dollop config auth r2-key <key>', and 'dollop config auth r2-secret <secret>' (or use --copy-dir)", 1)
			}

			var prefix string
			if keep {
				prefix = upload.PermanentPrefix(newName())
			} else {
				switch days {
				case 1, 7, 14:
				default:
					return cli.Exit("--days must be 1, 7, or 14", 1)
				}
				id, err := newID()
				if err != nil {
					return cli.Exit(fmt.Sprintf("generate id: %v", err), 1)
				}
				prefix = upload.EphemeralPrefix(days, id)
			}

			var uploadOpts []upload.UploadOption
			if !noRender {
				uploadOpts = append(uploadOpts, upload.WithRenderer(render.NewMarkdownRendererWithStderr(cmd.Root().ErrWriter)))
			}

			result, err := upload.UploadFiles(ctx, activeUploader, bucket, prefix, localPath, genIndex, cmd.Root().ErrWriter, uploadOpts...)
			if err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("upload failed", 1)
			}

			suffix := upload.URLSuffix(genIndex, result.SourceRelPaths)
			url := upload.PublicURL(baseURL, prefix, suffix)
			if _, err := fmt.Fprintln(cmd.Root().Writer, urlout.Format(url, urlout.IsTerminalWriter(cmd.Root().Writer))); err != nil {
				return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
			}
			return nil
		},
	}
}
