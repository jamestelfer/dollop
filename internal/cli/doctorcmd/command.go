package doctorcmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/jamestelfer/dollop/internal/config"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/urfave/cli/v3"
)

// Status markers used at the start of each list item.
const (
	markPass = "✓" // a check passed
	markFail = "✗" // a check failed
	markWarn = "⚠" // informational warning (non-fatal)
	markInfo = "ℹ" // informational note
)

// New returns the doctor command.
// cfg, hasKey, and hasSecret describe the current configuration state.
// configPath and authPath are the locations of the config file and the
// plain-text auth file; they are shown to the user (with the home directory
// collapsed to "~"). secureStorage reports whether credentials are held in the
// OS secrets storage (true) rather than the plain-text auth file (false).
// uploader and lister are injected so tests can supply fakes; both may be nil
// when credentials are absent (config checks will already fail in that case).
// newID generates the nanoid used as the check object key and content.
// httpGet performs the download verification; inject http.DefaultClient.Do for production.
func New(
	cfg config.Config,
	configPath string,
	authPath string,
	secureStorage bool,
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
Checks are reported in three groups:

  config     required config values (bucket, account-id, base-url) and the
             location of the config file
  auth       presence of the R2 credentials and where they are stored
  roundtrip  the bucket is reachable, a test object uploads, and it can be
             downloaded again via its public URL`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			w := cmd.Root().Writer

			// config group
			fmt.Fprintln(w, "config:") //nolint:errcheck
			writeInfo(w, "config file: %s", displayPath(configPath))
			configOK := writeValueCheck(w, "bucket", cfg.Bucket)
			if !writeValueCheck(w, "account-id", cfg.AccountID) {
				configOK = false
			}
			if !writeValueCheck(w, "base-url", cfg.BaseURL) {
				configOK = false
			}

			// auth group
			fmt.Fprintln(w, "auth:") //nolint:errcheck
			authOK := writeBoolCheck(w, "r2-key", hasKey)
			if !writeBoolCheck(w, "r2-secret", hasSecret) {
				authOK = false
			}
			if secureStorage {
				writeInfo(w, "stored in OS secrets storage")
			} else {
				writeWarn(w, "no secure secret storage available, stored in %s", displayPath(authPath))
			}

			if !configOK || !authOK {
				return cli.Exit("configuration incomplete", 1)
			}

			// roundtrip group
			fmt.Fprintln(w, "roundtrip:") //nolint:errcheck

			if err := lister.ListBucket(ctx, cfg.Bucket); err != nil {
				writeFail(w, "bucket not accessible: %v", err)
				return cli.Exit("bucket check failed", 1)
			}
			writeOK(w, "bucket accessible")

			id, err := newID()
			if err != nil {
				return cli.Exit(fmt.Sprintf("generate id: %v", err), 1)
			}
			prefix := upload.EphemeralPrefix(1, id)
			key := prefix + "/check.txt"
			if err := uploader.PutObject(ctx, cfg.Bucket, key, "text/plain; charset=utf-8", strings.NewReader(id)); err != nil {
				writeFail(w, "upload failed: %v", err)
				return cli.Exit("upload check failed", 1)
			}
			writeOK(w, "uploaded %s", key)

			publicURL := upload.PublicURL(cfg.BaseURL, prefix, "check.txt")
			resp, err := httpGet(ctx, publicURL)
			if err != nil {
				writeFail(w, "download failed: %v", err)
				return cli.Exit("download check failed", 1)
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode != http.StatusOK {
				writeFail(w, "download failed: HTTP %d", resp.StatusCode)
				return cli.Exit("download check failed", 1)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				writeFail(w, "download failed: read body: %v", err)
				return cli.Exit("download check failed", 1)
			}
			if string(body) != id {
				writeFail(w, "download failed: content mismatch")
				return cli.Exit("download check failed", 1)
			}
			writeOK(w, "downloaded %s", publicURL)

			if _, err := fmt.Fprintln(w, "all checks passed"); err != nil {
				return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
			}
			return nil
		},
	}
}

// writeItem prints a single indented list item prefixed with a status marker.
func writeItem(w io.Writer, mark, format string, args ...any) {
	fmt.Fprintf(w, "  %s %s\n", mark, fmt.Sprintf(format, args...)) //nolint:errcheck
}

func writeOK(w io.Writer, format string, args ...any)   { writeItem(w, markPass, format, args...) }
func writeFail(w io.Writer, format string, args ...any) { writeItem(w, markFail, format, args...) }
func writeWarn(w io.Writer, format string, args ...any) { writeItem(w, markWarn, format, args...) }
func writeInfo(w io.Writer, format string, args ...any) { writeItem(w, markInfo, format, args...) }

// writeValueCheck prints a pass/fail item for a config file value and returns whether it passed.
func writeValueCheck(w io.Writer, label, value string) bool {
	if value == "" {
		writeFail(w, "%s: not set", label)
		return false
	}
	writeOK(w, "%s: %s", label, value)
	return true
}

// writeBoolCheck prints a pass/fail item for a boolean presence check and returns whether it passed.
func writeBoolCheck(w io.Writer, label string, ok bool) bool {
	if !ok {
		writeFail(w, "%s: not set", label)
		return false
	}
	writeOK(w, "%s: set", label)
	return true
}

// displayPath collapses the user's home directory to "~" for display. Paths
// outside the home directory (or when it cannot be resolved) are returned
// unchanged.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(os.PathSeparator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(os.PathSeparator) + path[len(prefix):]
	}
	return path
}
