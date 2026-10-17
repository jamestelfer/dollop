package upload

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTMLTitle(t *testing.T) {
	bigStyle := "<style>" + strings.Repeat("a{}", (titleReadLimit/3)+1) + "</style>"
	tests := []struct {
		name string
		html string
		want string
	}{
		{"h1 text", "<h1>Alpha</h1>", "Alpha"},
		{"nested tags dropped", "<h1>Alpha <code>x</code></h1>", "Alpha x"},
		{"entities decoded", "<h1>Fish &amp; Chips &lt;3</h1>", "Fish & Chips <3"},
		{"whitespace collapsed", "<h1>\n  Alpha\t\n  Beta  </h1>", "Alpha Beta"},
		{"first h1 wins", "<h1>One</h1><h1>Two</h1>", "One"},
		{"h1 preferred over title", "<title>Tab</title><h1>Heading</h1>", "Heading"},
		{"title fallback", "<html><head><title>Tab &amp; more</title></head><body><p>x</p></body></html>", "Tab & more"},
		{"unclosed h1 falls back to title", "<title>Tab</title><h1>Cut off", "Tab"},
		{"unclosed h1 without title", "<h1>Cut off", ""},
		{"h1 past the cap falls back to title", "<title>Tab</title>" + bigStyle + "<h1>Late</h1>", "Tab"},
		{"no title", "<p>just text</p>", ""},
		{"empty h1 falls back to title", "<title>Tab</title><h1> </h1>", "Tab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, htmlTitle(strings.NewReader(tt.html)))
		})
	}
}

// failingReader fails every read, recording that it was reached.
type failingReader struct{ reached bool }

func (r *failingReader) Read([]byte) (int, error) {
	r.reached = true
	return 0, errors.New("read past the title")
}

func TestHTMLTitle_StopsAtFirstH1(t *testing.T) {
	tail := &failingReader{}
	got := htmlTitle(io.MultiReader(strings.NewReader("<h1>Alpha</h1>"), tail))

	assert.Equal(t, "Alpha", got)
	assert.False(t, tail.reached, "reading must stop at the first </h1>")
}

// countingReader serves an endless run of text, counting the bytes served.
type countingReader struct{ n int }

func (r *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	r.n += len(p)
	return len(p), nil
}

func TestHTMLTitle_ReadsAtMostTheCap(t *testing.T) {
	r := &countingReader{}
	assert.Empty(t, htmlTitle(r))
	assert.LessOrEqual(t, r.n, titleReadLimit+4096, "the read is bounded by the cap (plus one buffer)")
}

func TestNormalizeTitle(t *testing.T) {
	assert.Equal(t, "a b", normalizeTitle("  a \n\t b "))
	assert.Equal(t, "", normalizeTitle(" \n "))

	exact := strings.Repeat("é", maxTitleRunes)
	assert.Equal(t, exact, normalizeTitle(exact))

	long := strings.Repeat("é", maxTitleRunes+5)
	got := normalizeTitle(long)
	assert.Equal(t, strings.Repeat("é", maxTitleRunes-1)+"…", got)
}
