package updatecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jamestelfer/dollop/internal/cli/urlout"
	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/jamestelfer/dollop/internal/render"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the update command.
// uploader, bucket, and baseURL are injected so tests can supply fakes.
func New(
	uploader upload.Uploader,
	bucket string,
	baseURL string,
) cli.Command {
	return cli.Command{
		Name:      "update",
		Usage:     "publish new content into an existing upload's link",
		ArgsUsage: "<url-or-prefix> <path>",
		Description: `Publishes a local file or directory into an existing upload, reusing its
link. The first argument identifies the existing upload: either the full
URL printed by 'create' or its bare prefix (e.g. flash/7/<id> or
keep/<name>). The second argument is the local file or directory to
publish into it.

Files are uploaded into the existing prefix, overwriting any objects with
matching keys. The shareable URL is printed to stdout; upload progress is
written to stderr.

  dollop update https://drop.example.com/flash/7/abc123/ report.pdf
  dollop update flash/7/abc123 site/
  dollop update keep/happy-cat notes.md

Rendering of .md files and shared assets works exactly as in 'create';
use --no-render to disable it and --index to generate an index.html.`,
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
		Action: newAction(uploader, bucket, baseURL),
	}
}

// warnMissingMermaidDeps writes a non-fatal warning when localPath renders any
// mermaid diagram but the shared engine is not present in the bucket.
func warnMissingMermaidDeps(ctx context.Context, up upload.Uploader, bucket, localPath string, stderr io.Writer) {
	uses, err := render.UsesMermaid(localPath)
	if err != nil {
		return
	}
	lister, _ := up.(upload.ObjectLister)
	deps.WarnIfAbsent(ctx, lister, bucket, render.MermaidVersion, uses, stderr)
}

func newAction(uploader upload.Uploader, bucket, baseURL string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() != 2 {
			return cli.Exit("usage: update <url-or-prefix> <path>", 1)
		}

		genIndex := cmd.Bool("index")
		noRender := cmd.Bool("no-render")
		copyDir := cmd.String("copy-dir")
		ref := cmd.Args().Get(0)
		localPath := cmd.Args().Get(1)

		prefix, err := upload.ResolvePrefix(baseURL, ref)
		if err != nil {
			return cli.Exit(fmt.Sprintf("resolve upload reference: %v", err), 1)
		}

		activeUploader := uploader
		if copyDir != "" {
			fmt.Fprintf(cmd.Root().ErrWriter, "note: writing to local directory %s instead of R2\n", copyDir) //nolint:errcheck
			activeUploader = &upload.DirUploader{Root: copyDir}
		}

		if activeUploader == nil {
			return cli.Exit("no R2 credentials configured; run 'dollop config set account-id <id>', "+
				"'dollop config auth r2-key <key>', and 'dollop config auth r2-secret <secret>' (or use --copy-dir)", 1)
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

		// Reset the expiry clock on pre-existing objects this update did not
		// rewrite, so a partial refresh doesn't let the rest of the upload
		// expire early. The pass applies only to directory updates on flash/
		// prefixes: keep/ has no expiry clock, and a single-file update leaves
		// nothing else under the prefix worth preserving.
		if info, statErr := os.Stat(localPath); statErr == nil && info.IsDir() && strings.HasPrefix(prefix, "flash/") {
			lister, lok := activeUploader.(upload.ObjectLister)
			copier, cok := activeUploader.(upload.ObjectCopier)
			if lok && cok {
				if err := upload.TouchUntouched(ctx, lister, copier, bucket, prefix, result.WrittenKeys, cmd.Root().ErrWriter); err != nil {
					fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
					return cli.Exit("touch failed", 1)
				}
			}
		}

		// Warn (non-fatal) when the publish renders mermaid but the shared
		// engine is not published; the upload above already succeeded.
		if !noRender {
			warnMissingMermaidDeps(ctx, activeUploader, bucket, localPath, cmd.Root().ErrWriter)
		}

		suffix := upload.URLSuffix(genIndex, result.SourceRelPaths)
		url := upload.PublicURL(baseURL, prefix, suffix)
		if _, err := fmt.Fprintln(cmd.Root().Writer, urlout.Format(url, urlout.IsTerminalWriter(cmd.Root().Writer))); err != nil {
			return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
		}
		return nil
	}
}
