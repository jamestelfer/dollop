package urlout

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHyperlink_Format(t *testing.T) {
	result := hyperlink("https://example.com/path", "click here")
	// OSC 8 ; ; URL ST text OSC 8 ; ; ST
	assert.Equal(t, "\x1b]8;;https://example.com/path\x1b\\click here\x1b]8;;\x1b\\", result)
}

func TestFormat_NonTTY_ReturnsPlain(t *testing.T) {
	url := "https://example.com/flash/1/abc/"
	result := Format(url, false)
	assert.Equal(t, url, result, "non-TTY output must be a plain URL with no escape sequences")
}

func TestFormat_TTY_ReturnsOSC8(t *testing.T) {
	url := "https://example.com/flash/1/abc/"
	result := Format(url, true)
	assert.True(t, strings.HasPrefix(result, "\x1b]8;;"), "TTY output must start with OSC 8 opener")
	assert.Contains(t, result, url, "TTY output must contain the URL")
	assert.True(t, strings.HasSuffix(result, "\x1b]8;;\x1b\\"), "TTY output must end with OSC 8 closer")
}

func TestFormat_TTY_URLAndTextAreTheSame(t *testing.T) {
	url := "https://example.com/keep/happy-cat/"
	result := Format(url, true)
	// the URL should appear twice: once as the link target and once as the display text
	assert.Equal(t, 2, strings.Count(result, url), "URL must appear as both link target and display text")
}
