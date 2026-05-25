package upload_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
)

func TestEphemeralPrefix(t *testing.T) {
	assert.Equal(t, "dollop/1/abc123", upload.EphemeralPrefix(1, "abc123"))
	assert.Equal(t, "dollop/7/xyz", upload.EphemeralPrefix(7, "xyz"))
	assert.Equal(t, "dollop/14/id99", upload.EphemeralPrefix(14, "id99"))
}

func TestPermanentPrefix(t *testing.T) {
	assert.Equal(t, "keep/happy-dog", upload.PermanentPrefix("happy-dog"))
}

func TestPublicURL(t *testing.T) {
	tests := []struct {
		baseURL string
		prefix  string
		want    string
	}{
		{"https://drop.example.com", "dollop/7/abc123", "https://drop.example.com/dollop/7/abc123/"},
		{"https://drop.example.com/", "keep/happy-dog", "https://drop.example.com/keep/happy-dog/"},
		{"drop.example.com", "dollop/1/xyz", "https://drop.example.com/dollop/1/xyz/"},
		{"HTTPS://drop.example.com", "dollop/1/xyz", "HTTPS://drop.example.com/dollop/1/xyz/"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, upload.PublicURL(tc.baseURL, tc.prefix))
	}
}
