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
