package upload

import "testing"

func TestCopySource_PlainKey(t *testing.T) {
	got := copySource("my-bucket", "flash/7/abc/index.html")
	want := "my-bucket/flash/7/abc/index.html"
	if got != want {
		t.Fatalf("copySource() = %q, want %q", got, want)
	}
}

func TestCopySource_EncodesSpecialCharsKeepingSlashes(t *testing.T) {
	// Spaces and reserved characters in the key must be percent-encoded so the
	// x-amz-copy-source header survives transport, but the path separators that
	// structure the key must stay literal.
	got := copySource("my-bucket", "keep/happy cat/a#b.txt")
	want := "my-bucket/keep/happy%20cat/a%23b.txt"
	if got != want {
		t.Fatalf("copySource() = %q, want %q", got, want)
	}
}
