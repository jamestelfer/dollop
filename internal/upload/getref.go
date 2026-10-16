package upload

import (
	"fmt"
	"net/url"
	"strings"
)

// GetRef identifies what to download from a single upload: the upload's
// canonical prefix and the requested key path within it. Path equals Prefix
// when the whole upload is requested; otherwise it names a file or a
// subdirectory under Prefix. Neither carries a trailing slash.
type GetRef struct {
	Prefix string
	Path   string
}

// Whole reports whether the reference names the entire upload rather than a
// file or subdirectory within it.
func (r GetRef) Whole() bool { return r.Path == r.Prefix }

// ResolveGetRef parses a get argument (a full public URL or a bare key path)
// into a GetRef. It is deliberately strict, so a download can never reach
// beyond a single upload:
//
//   - the path must start with flash/<digits>/<id> or keep/<name>; the bucket
//     root, partial prefixes and any other top-level path (e.g. deps/) are
//     refused;
//   - after URL-decoding, no segment may be empty, "." or "..", and no
//     segment may contain a backslash or NUL. The path is rejected rather than
//     cleaned, so the reference must already be canonical.
//
// A single leading and trailing slash is tolerated, and the base URL's own path
// component is stripped when present.
func ResolveGetRef(baseURL, input string) (GetRef, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return GetRef{}, fmt.Errorf("empty upload reference")
	}

	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Path // decoded
	} else {
		dec, err := url.PathUnescape(s)
		if err != nil {
			return GetRef{}, fmt.Errorf("invalid upload reference %q: %w", input, err)
		}
		s = dec
	}

	s = strings.TrimPrefix(s, "/")
	if _, basePath := splitBaseURL(baseURL); basePath != "" {
		switch {
		case s == basePath:
			s = ""
		case strings.HasPrefix(s, basePath+"/"):
			s = s[len(basePath)+1:]
		}
	}
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return GetRef{}, fmt.Errorf("no upload in %q: expected flash/<days>/<id> or keep/<name>", input)
	}

	segments := strings.Split(s, "/")
	if err := checkSegments(segments); err != nil {
		return GetRef{}, fmt.Errorf("invalid upload reference %q: %w", input, err)
	}

	var depth int
	switch segments[0] {
	case "flash":
		if len(segments) < 3 {
			return GetRef{}, fmt.Errorf("incomplete flash prefix %q: expected flash/<days>/<id>", s)
		}
		if !isDigits(segments[1]) {
			return GetRef{}, fmt.Errorf("invalid flash prefix %q: days must be a number", s)
		}
		depth = 3
	case "keep":
		if len(segments) < 2 {
			return GetRef{}, fmt.Errorf("incomplete keep prefix %q: expected keep/<name>", s)
		}
		depth = 2
	default:
		return GetRef{}, fmt.Errorf("unrecognised prefix in %q: expected flash/<days>/<id> or keep/<name>", input)
	}

	return GetRef{Prefix: strings.Join(segments[:depth], "/"), Path: s}, nil
}

// checkSegments rejects any path segment that could traverse or is otherwise
// non-canonical.
func checkSegments(segments []string) error {
	for _, seg := range segments {
		switch {
		case seg == "":
			return fmt.Errorf("empty path segment")
		case seg == "." || seg == "..":
			return fmt.Errorf("path traversal segment %q", seg)
		case strings.ContainsAny(seg, "\\\x00"):
			return fmt.Errorf("invalid character in path segment %q", seg)
		}
	}
	return nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
