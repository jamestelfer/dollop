package buildinfo_test

import (
	"bytes"
	"testing"

	"github.com/jamestelfer/dollop/internal/buildinfo"
	"github.com/stretchr/testify/require"
)

func TestFprint_WritesNameAndVersion(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, buildinfo.Fprint(&buf, "dollop"))

	require.Equal(t, "dollop dev\n", buf.String())
}
