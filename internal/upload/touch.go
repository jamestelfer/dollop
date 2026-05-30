package upload

import (
	"context"
	"fmt"
	"io"
)

// TouchUntouched resets the lifecycle expiry clock on every object under prefix
// that was not written this run. It lists the prefix, subtracts writtenKeys, and
// self-copies (touches) each remaining key, reporting it to stderr. This keeps
// pre-existing objects alive when an update refreshes only part of an upload;
// freshly-written objects already carry a new modification time and are skipped.
//
// The first touch failure aborts and is returned.
func TouchUntouched(ctx context.Context, lister ObjectLister, copier ObjectCopier, bucket, prefix string, writtenKeys []string, stderr io.Writer) error {
	existing, err := lister.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("list objects under %s: %w", prefix, err)
	}

	written := make(map[string]struct{}, len(writtenKeys))
	for _, k := range writtenKeys {
		written[k] = struct{}{}
	}

	for _, key := range existing {
		if _, ok := written[key]; ok {
			continue
		}
		fmt.Fprintf(stderr, "touching [%s]...", key) //nolint:errcheck
		if err := copier.CopyObject(ctx, bucket, key); err != nil {
			fmt.Fprintln(stderr, "failed") //nolint:errcheck
			return fmt.Errorf("touch %s: %w", key, err)
		}
		fmt.Fprintln(stderr, "done") //nolint:errcheck
	}
	return nil
}
