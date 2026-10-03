package depscmd

import (
	"context"
	"fmt"
	"io"

	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the deps command tree (publish, fonts, status).
// uploader and lister are injected so tests can supply fakes; both may be nil
// when credentials are absent. bucket is the target R2 bucket. version and
// integrity are the pinned mermaid release and its npm sha512 integrity string.
// fetch downloads the tarball; inject an http-backed fetch for production.
// fonts is the optional shared body font uploaded from a local directory.
func New(
	uploader upload.Uploader,
	lister upload.ObjectLister,
	bucket string,
	version string,
	integrity string,
	fetch deps.FetchFunc,
	fonts deps.FontSet,
) cli.Command {
	return cli.Command{
		Name:  "deps",
		Usage: "manage the shared mermaid engine and fonts published to the bucket",
		Description: `Publishes and inspects the shared, version-pinned mermaid engine that
rendered pages reference. The engine is fetched from npm, verified against a
pinned checksum, and uploaded once to deps/mermaid/<version>/ — outside the
expiring flash/ prefixes and cached indefinitely. Run 'deps publish' once per
mermaid version; rendered pages then load it from this shared location.

'deps fonts' uploads the optional body font from a local directory. dollop
never ships the font files; pages fall back to Inter when they are absent.`,
		Commands: []*cli.Command{
			publishCommand(uploader, lister, bucket, version, integrity, fetch),
			fontsCommand(uploader, bucket, fonts),
			statusCommand(lister, bucket, version, fonts),
		},
	}
}

// copyDirFlag is the hidden integration-testing flag shared by every subcommand.
func copyDirFlag() cli.Flag {
	return &cli.StringFlag{
		Name:   "copy-dir",
		Usage:  "copy files to this local directory instead of uploading to R2 (integration testing only)",
		Hidden: true,
	}
}

// writeTargets returns the uploader and lister a write subcommand should use:
// the injected pair, or a local directory when --copy-dir is set.
func writeTargets(cmd *cli.Command, uploader upload.Uploader, lister upload.ObjectLister) (upload.Uploader, upload.ObjectLister) {
	copyDir := cmd.String("copy-dir")
	if copyDir == "" {
		return uploader, lister
	}
	fmt.Fprintf(cmd.Root().ErrWriter, "note: writing to local directory %s instead of R2\n", copyDir) //nolint:errcheck
	dir := &upload.DirUploader{Root: copyDir}
	return dir, dir
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
			up, lst := writeTargets(cmd, uploader, lister)
			if up == nil || lst == nil {
				return cli.Exit(noCredsMessage, 1)
			}

			uploaded, err := deps.Publish(ctx, up, lst, bucket, version, integrity, fetch, cmd.Bool("force"), cmd.Root().ErrWriter)
			if err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("publish failed", 1)
			}

			w := cmd.Root().Writer
			if uploaded {
				return writeOutput(w, "published mermaid %s to %s/\n", version, deps.VersionPrefix(version))
			}
			return writeOutput(w, "mermaid %s already present at %s/\n", version, deps.VersionPrefix(version))
		},
	}
}

func fontsCommand(uploader upload.Uploader, bucket string, fonts deps.FontSet) *cli.Command {
	return &cli.Command{
		Name:      "fonts",
		Usage:     "upload the optional body font from a local directory to the bucket",
		ArgsUsage: "<dir>",
		Description: `Uploads the body font files found under <dir> (searched recursively, so an
unpacked font archive can be passed as-is) to ` + fonts.Prefix + `/. Rendered pages
reference the font there and fall back to Inter when it is absent, so pages
already published pick it up without re-rendering.

The font is proprietary and is not distributed with dollop. Supply files you
hold a licence for, and check that the licence permits web hosting.`,
		Flags: []cli.Flag{copyDirFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return cli.Exit("usage: dollop deps fonts <dir>", 1)
			}
			up, _ := writeTargets(cmd, uploader, nil)
			if up == nil {
				return cli.Exit(noCredsMessage, 1)
			}

			uploaded, err := deps.PublishFonts(ctx, up, bucket, fonts, cmd.Args().First(), cmd.Root().ErrWriter)
			if err != nil {
				fmt.Fprintf(cmd.Root().ErrWriter, "error: %v\n", err) //nolint:errcheck
				return cli.Exit("fonts upload failed", 1)
			}

			noun := "files"
			if len(uploaded) == 1 {
				noun = "file"
			}
			return writeOutput(cmd.Root().Writer, "published %d font %s to %s/\n", len(uploaded), noun, fonts.Prefix)
		},
	}
}

func statusCommand(lister upload.ObjectLister, bucket, version string, fonts deps.FontSet) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "report the shipped mermaid version and whether it and the fonts are published",
		Flags: []cli.Flag{copyDirFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			copyDir := cmd.String("copy-dir")
			w := cmd.Root().Writer

			if err := writeOutput(w, "mermaid version: %s\n", version); err != nil {
				return err
			}

			lst := lister
			if copyDir != "" {
				lst = &upload.DirUploader{Root: copyDir}
			}
			if lst == nil {
				return writeOutput(w, "bucket: not configured\n")
			}

			present, err := deps.Present(ctx, lst, bucket, version)
			if err != nil {
				return cli.Exit(fmt.Sprintf("check deps: %v", err), 1)
			}
			mermaidState := "absent (run 'dollop deps publish')"
			if present {
				mermaidState = "present at " + deps.VersionPrefix(version) + "/"
			}
			if err := writeOutput(w, "bucket: %s\n", mermaidState); err != nil {
				return err
			}

			published, err := deps.FontsPresent(ctx, lst, bucket, fonts)
			if err != nil {
				return cli.Exit(fmt.Sprintf("check fonts: %v", err), 1)
			}
			return writeOutput(w, "fonts: %d of %d published at %s/\n", len(published), len(fonts.Files), fonts.Prefix)
		},
	}
}

// writeOutput writes a line of the command's primary output, turning a writer
// failure into an exit error rather than dropping it.
func writeOutput(w io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
	}
	return nil
}

const noCredsMessage = "no R2 credentials configured; run 'dollop config set account-id <id>', " +
	"'dollop config auth r2-key <key>', and 'dollop config auth r2-secret <secret>' (or use --copy-dir)"
