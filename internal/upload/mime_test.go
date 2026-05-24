package upload_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
)

func TestContentType_KnownExtension(t *testing.T) {
	assert.Equal(t, "text/css; charset=utf-8", upload.ContentType("style.css"))
	assert.Equal(t, "application/json", upload.ContentType("data.json"))
}

func TestContentType_UnknownExtension(t *testing.T) {
	assert.Equal(t, "application/octet-stream", upload.ContentType("archive.xyz123abc"))
}

func TestContentType_NoExtension(t *testing.T) {
	assert.Equal(t, "application/octet-stream", upload.ContentType("Makefile"))
}

func TestContentType_HTMLPreservesCharset(t *testing.T) {
	ct := upload.ContentType("index.html")
	assert.Contains(t, ct, "text/html")
}
