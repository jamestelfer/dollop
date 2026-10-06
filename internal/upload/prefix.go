package upload

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
)

// EphemeralPrefix returns the R2 key prefix for a time-limited upload.
func EphemeralPrefix(days int, id string) string {
	return fmt.Sprintf("flash/%d/%s", days, id)
}

// KeepSuffixAlphabet is the character set for the random suffix of a
// permanent upload name. It excludes nanoid's default '-' and '_' so the
// suffix groups stay visually distinct from the petname.
const KeepSuffixAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// keepSuffixGroups and keepSuffixGroupLen shape the random suffix: three
// dot-separated groups of three characters (~53.6 bits over 62 symbols).
const (
	keepSuffixGroups   = 3
	keepSuffixGroupLen = 3
)

// NewKeepName returns a permanent upload name of the form
// <petname>.<abc>.<def>.<ghi>, e.g. happy-otter.aB3.x9Z.q2M. The petname keeps
// the link memorable; the random suffix makes it impractical to guess.
// petname supplies the readable part; generate draws size random characters
// from alphabet (e.g. nanoid.Generate).
func NewKeepName(petname func() string, generate func(alphabet string, size int) (string, error)) (string, error) {
	size := keepSuffixGroups * keepSuffixGroupLen
	suffix, err := generate(KeepSuffixAlphabet, size)
	if err != nil {
		return "", fmt.Errorf("generate keep suffix: %w", err)
	}
	if len(suffix) != size {
		return "", fmt.Errorf("generate keep suffix: got %d characters, want %d", len(suffix), size)
	}

	parts := make([]string, 0, keepSuffixGroups+1)
	parts = append(parts, petname())
	for i := 0; i < size; i += keepSuffixGroupLen {
		parts = append(parts, suffix[i:i+keepSuffixGroupLen])
	}
	return strings.Join(parts, "."), nil
}

// PermanentPrefix returns the R2 key prefix for a permanent upload.
func PermanentPrefix(name string) string {
	return "keep/" + name
}

// PublicURL constructs the public URL for a given prefix under baseURL.
// If suffix is non-empty it is appended after the prefix slash (no additional
// trailing slash). If baseURL has no scheme, https:// is prepended.
func PublicURL(baseURL, prefix, suffix string) string {
	base := strings.TrimRight(baseURL, "/")
	if u, err := url.Parse(base); err != nil || u.Scheme == "" {
		base = "https://" + base
	}
	if suffix != "" {
		return base + "/" + prefix + "/" + suffix
	}
	return base + "/" + prefix + "/"
}

// ResolvePrefix recovers the bare R2 prefix from an update argument. The
// argument may be a full public URL (as printed by create/update) or a bare
// prefix path. It is the inverse of PublicURL: the base URL and any trailing
// filename, index.html, or slash are discarded, leaving the canonical prefix
// (flash/<days>/<id> or keep/<name>).
func ResolvePrefix(baseURL, input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("empty upload reference")
	}

	// Reduce a full URL to its path component; bare prefixes pass through
	// unchanged (url.Parse leaves Host empty for them).
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Path
	}

	// Drop the base URL's own path component when present: base_url may carry a
	// sub-path (e.g. https://cdn.example.com/files), which would otherwise be
	// mistaken for the prefix root.
	if _, basePath := splitBaseURL(baseURL); basePath != "" {
		t := strings.TrimLeft(s, "/")
		switch {
		case t == basePath:
			t = ""
		case strings.HasPrefix(t, basePath+"/"):
			t = t[len(basePath)+1:]
		}
		s = t
	}

	s = strings.Trim(s, "/")
	if s == "" {
		return "", fmt.Errorf("no prefix in %q", input)
	}
	segments := strings.Split(s, "/")

	switch segments[0] {
	case "flash":
		if len(segments) < 3 {
			return "", fmt.Errorf("incomplete flash prefix %q: expected flash/<days>/<id>", s)
		}
		return strings.Join(segments[:3], "/"), nil
	case "keep":
		if len(segments) < 2 {
			return "", fmt.Errorf("incomplete keep prefix %q: expected keep/<name>", s)
		}
		return strings.Join(segments[:2], "/"), nil
	default:
		return "", fmt.Errorf("unrecognised prefix in %q: expected flash/<days>/<id> or keep/<name>", input)
	}
}

// splitBaseURL returns the host and trimmed path of baseURL, normalising a
// missing scheme the same way PublicURL does (https:// is assumed).
func splitBaseURL(baseURL string) (host, path string) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", ""
	}
	if u, err := url.Parse(base); err != nil || u.Scheme == "" {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", ""
	}
	return u.Host, strings.Trim(u.Path, "/")
}

// URLSuffix returns the filename component to append to the public URL based
// on the upload contents and whether --index was requested:
//
//  1. --index flag or index.html present in files → "" (browser will load it)
//  2. exactly one file → that file's relative path
//  3. multiple files → the first path in alphabetical order
//  4. no files → ""
func URLSuffix(indexFlag bool, files []string) string {
	if indexFlag {
		return ""
	}
	if slices.Contains(files, "index.html") {
		return ""
	}
	switch len(files) {
	case 0:
		return ""
	case 1:
		return files[0]
	default:
		sorted := make([]string, len(files))
		copy(sorted, files)
		sort.Strings(sorted)
		return sorted[0]
	}
}
