package upload

import (
	"fmt"
	"net/url"
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
// If baseURL has no scheme, https:// is prepended.
func PublicURL(baseURL, prefix string) string {
	base := strings.TrimRight(baseURL, "/")
	if u, err := url.Parse(base); err != nil || u.Scheme == "" {
		base = "https://" + base
	}
	return base + "/" + prefix + "/"
}
