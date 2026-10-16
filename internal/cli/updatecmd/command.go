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
// uploader, bucket, and baseURL are injected so tests can supply fakes. The
// uploader must also list and self-copy objects (upload.SyncUploader): listing
// backs the deps-presence check and the partial-refresh touch pass, and
// self-copy resets the lifecycle expiry clock on objects this run did not
// rewrite.
func New(
	uploader upload.SyncUploader,
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

Rendering of .md files, shared assets and the README redirect work
exactly as in 'create'; use --no-render to disable rendering, --theme
to choose the page theme, and --index to generate an index page listing
every file under the prefix, including those published by earlier runs.

An index page dollop generated is kept current: a generated listing is
refreshed on every update, even without --index, and keeps its title.
Any other published index page is kept by --index; --index-overwrite
replaces it with a generated one (a README.md, local or published, still
gets a redirect instead). An index page in the uploaded path always wins.`,
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{{
			Flags: [][]cli.Flag{
				{&cli.BoolFlag{
					Name:  "index",
					Usage: "generate an index.md listing all files under the prefix, rendered to index.html (skipped with a warning if index.html or index.md is present or already published, or a README.md redirect applies)",
				}},
				{&cli.BoolFlag{
					Name:  "index-overwrite",
					Usage: "like --index, but replace an index page already published under the prefix",
				}},
			},
		}},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-render",
				Usage: "disable automatic rendering of .md files to .html",
			},
			&cli.StringFlag{
				Name:  "theme",
				Usage: "page theme for rendered .md files: " + strings.Join(render.ThemeNames(), ", "),
				Value: render.DefaultTheme,
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
func warnMissingMermaidDeps(ctx context.Context, lister upload.ObjectLister, bucket, localPath string, stderr io.Writer) {
	uses, err := render.UsesMermaid(localPath)
	if err != nil {
		return
	}
	deps.WarnIfAbsent(ctx, lister, bucket, render.MermaidVersion, uses, stderr)
}

// indexMode maps the mutually exclusive --index and --index-overwrite flags
// onto the upload's index mode.
func indexMode(cmd *cli.Command) upload.IndexMode {
	switch {
	case cmd.Bool("index-overwrite"):
		return upload.IndexOverwrite
	case cmd.Bool("index"):
		return upload.IndexGenerate
	default:
		return upload.IndexNone
	}
}

// publishedOptions reports what is already published under prefix to the
// upload. Entry pages are planned over it as well as the local files: a
// listing covers both, an index page dollop generated is regenerated (a
// listing even without --index), and any other published index page is
// replaced only with --index-overwrite.
func publishedOptions(ctx context.Context, up upload.SyncUploader, bucket, prefix string) ([]upload.UploadOption, error) {
	published, err := upload.PublishedPaths(ctx, up, bucket, prefix)
	if err != nil {
		return nil, err
	}
	index, err := upload.InspectPublishedIndex(ctx, up, bucket, prefix, published)
	if err != nil {
		return nil, err
	}
	return []upload.UploadOption{upload.WithPublished(published), upload.WithPublishedIndex(index)}, nil
}

func newAction(uploader upload.SyncUploader, bucket, baseURL string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() != 2 {
			return cli.Exit("usage: update <url-or-prefix> <path>", 1)
		}

		genIndex := indexMode(cmd)
		noRender := cmd.Bool("no-render")
		copyDir := cmd.String("copy-dir")
		ref := cmd.Args().Get(0)
		localPath := cmd.Args().Get(1)

		theme, err := render.LookupTheme(cmd.String("theme"))
		if err != nil {
			return cli.Exit(err.Error(), 1)
		}

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

		uploadOpts, err := publishedOptions(ctx, activeUploader, bucket, prefix)
		if err != nil {
			fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
			return cli.Exit("upload failed", 1)
		}
		if !noRender {
			uploadOpts = append(uploadOpts, upload.WithRenderer(render.NewMarkdownRendererWithStderr(cmd.Root().ErrWriter, render.WithTheme(theme))))
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
			if err := upload.TouchUntouched(ctx, activeUploader, activeUploader, bucket, prefix, result.WrittenKeys, cmd.Root().ErrWriter); err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("touch failed", 1)
			}
		}

		// Warn (non-fatal) when the publish renders mermaid but the shared
		// engine is not published; the upload above already succeeded.
		if !noRender {
			warnMissingMermaidDeps(ctx, activeUploader, bucket, localPath, cmd.Root().ErrWriter)
		}

		url := upload.PublicURL(baseURL, prefix, result.Entry)
		if _, err := fmt.Fprintln(cmd.Root().Writer, urlout.Format(url, urlout.IsTerminalWriter(cmd.Root().Writer))); err != nil {
			return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
		}
		return nil
	}
}
