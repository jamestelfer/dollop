package createcmd

import (
	"context"
	"fmt"

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
memorable two-word petname instead of a random ID.

  dollop create --keep notes.txt
  dollop create --keep project/`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "days",
				Usage: "number of days before the link expires; allowed values: 1, 7, 14 (ignored with --keep)",
				Value: 1,
			},
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "publish to a permanent link with a memorable petname (no expiry)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return cli.Exit("usage: create [--days N] [--keep] <path>", 1)
			}

			days := cmd.Int("days")
			keep := cmd.Bool("keep")
			localPath := cmd.Args().Get(0)

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

			if err := upload.UploadFiles(ctx, uploader, bucket, prefix, localPath, cmd.Root().ErrWriter); err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("upload failed", 1)
			}

			if _, err := fmt.Fprintln(cmd.Root().Writer, upload.PublicURL(baseURL, prefix)); err != nil {
				return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
			}
			return nil
		},
	}
}
