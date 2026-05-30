package upload_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeListerCopier lists a fixed set of keys and records every CopyObject call.
type fakeListerCopier struct {
	existing []string
	copied   []string
	copyErr  error
}

func (f *fakeListerCopier) ListObjects(_ context.Context, _, _ string) ([]string, error) {
	return f.existing, nil
}

func (f *fakeListerCopier) CopyObject(_ context.Context, _, key string) error {
	if f.copyErr != nil {
		return f.copyErr
	}
	f.copied = append(f.copied, key)
	return nil
}

func TestTouchUntouched_TouchesOnlyPreExistingUnwrittenKeys(t *testing.T) {
	fc := &fakeListerCopier{
		existing: []string{
			"flash/7/abc/index.html", // written this run
			"flash/7/abc/style.css",  // written this run
			"flash/7/abc/old.pdf",    // pre-existing, untouched
			"flash/7/abc/legacy.txt", // pre-existing, untouched
		},
	}
	written := []string{"flash/7/abc/index.html", "flash/7/abc/style.css"}

	var stderr bytes.Buffer
	err := upload.TouchUntouched(context.Background(), fc, fc, "bucket", "flash/7/abc", written, &stderr)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"flash/7/abc/old.pdf", "flash/7/abc/legacy.txt"}, fc.copied,
		"only pre-existing keys not written this run are touched")
	assert.NotContains(t, fc.copied, "flash/7/abc/index.html", "freshly-written keys are not touched")
	assert.Contains(t, stderr.String(), "flash/7/abc/old.pdf", "each touched key is reported to stderr")
}

func TestTouchUntouched_CopyFailureReturnsError(t *testing.T) {
	fc := &fakeListerCopier{
		existing: []string{"flash/7/abc/old.pdf"},
		copyErr:  errors.New("boom"),
	}

	var stderr bytes.Buffer
	err := upload.TouchUntouched(context.Background(), fc, fc, "bucket", "flash/7/abc", nil, &stderr)
	require.Error(t, err, "a touch failure propagates")
}
