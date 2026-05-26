package doctorcmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jamestelfer/dollop/internal/config"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// New returns the doctor command.
// cfg, hasKey, and hasSecret describe the current configuration state.
// uploader and lister are injected so tests can supply fakes; both may be nil
// when credentials are absent (config checks will already fail in that case).
// newID generates the nanoid used as the check object key and content.
// httpGet performs the download verification; inject http.DefaultClient.Do for production.
func New(
	cfg config.Config,
	hasKey bool,
	hasSecret bool,
	uploader upload.Uploader,
	lister upload.BucketLister,
	newID func() (string, error),
	httpGet func(ctx context.Context, url string) (*http.Response, error),
) cli.Command {
	return cli.Command{
		Name:  "doctor",
		Usage: "check configuration and end-to-end connectivity",
		Description: `Verifies that dollop is fully configured and can reach your R2 bucket.

Checks performed:
  1. All required config values are present (bucket, account-id, base-url, credentials)
  2. The R2 bucket is reachable
  3. A test object can be uploaded (flash/1/<id>/check.txt)
  4. The uploaded object is reachable via its public URL`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			w := cmd.Root().Writer

			configOK := writeCheck(w, "bucket", cfg.Bucket)
			if !writeCheck(w, "account-id", cfg.AccountID) {
				configOK = false
			}
			if !writeCheck(w, "base-url", cfg.BaseURL) {
				configOK = false
			}
			if !writeBoolCheck(w, "r2-key", hasKey) {
				configOK = false
			}
			if !writeBoolCheck(w, "r2-secret", hasSecret) {
				configOK = false
			}
			if !configOK {
				return cli.Exit("configuration incomplete", 1)
			}

			if err := lister.ListBucket(ctx, cfg.Bucket); err != nil {
				fmt.Fprintf(w, "  ✗ bucket not accessible: %v\n", err) //nolint:errcheck
				return cli.Exit("bucket check failed", 1)
			}
			fmt.Fprintln(w, "  ✓ bucket accessible") //nolint:errcheck

			id, err := newID()
			if err != nil {
				return cli.Exit(fmt.Sprintf("generate id: %v", err), 1)
			}
			prefix := upload.EphemeralPrefix(1, id)
			key := prefix + "/check.txt"
			if err := uploader.PutObject(ctx, cfg.Bucket, key, "text/plain; charset=utf-8", strings.NewReader(id)); err != nil {
				fmt.Fprintf(w, "  ✗ upload failed: %v\n", err) //nolint:errcheck
				return cli.Exit("upload check failed", 1)
			}
			fmt.Fprintf(w, "  ✓ uploaded %s\n", key) //nolint:errcheck

			publicURL := upload.PublicURL(cfg.BaseURL, prefix, "check.txt")
			resp, err := httpGet(ctx, publicURL)
			if err != nil {
				fmt.Fprintf(w, "  ✗ download failed: %v\n", err) //nolint:errcheck
				return cli.Exit("download check failed", 1)
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode != http.StatusOK {
				fmt.Fprintf(w, "  ✗ download failed: HTTP %d\n", resp.StatusCode) //nolint:errcheck
				return cli.Exit("download check failed", 1)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Fprintf(w, "  ✗ download failed: read body: %v\n", err) //nolint:errcheck
				return cli.Exit("download check failed", 1)
			}
			if string(body) != id {
				fmt.Fprintln(w, "  ✗ download failed: content mismatch") //nolint:errcheck
				return cli.Exit("download check failed", 1)
			}
			fmt.Fprintf(w, "  ✓ downloaded %s\n", publicURL) //nolint:errcheck

			if _, err := fmt.Fprintln(w, "all checks passed"); err != nil {
				return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
			}
			return nil
		},
	}
}

// writeCheck prints a pass/fail line for a config file value and returns whether it passed.
func writeCheck(w io.Writer, label, value string) bool {
	if value == "" {
		fmt.Fprintf(w, "  ✗ %s: not set\n", label) //nolint:errcheck
		return false
	}
	fmt.Fprintf(w, "  ✓ %s: %s\n", label, value) //nolint:errcheck
	return true
}

// writeBoolCheck prints a pass/fail line for a boolean config presence check and returns whether it passed.
func writeBoolCheck(w io.Writer, label string, ok bool) bool {
	if !ok {
		fmt.Fprintf(w, "  ✗ %s: not set\n", label) //nolint:errcheck
		return false
	}
	fmt.Fprintf(w, "  ✓ %s: set\n", label) //nolint:errcheck
	return true
}
