package upload

import (
	"fmt"
	"strings"
)

// EphemeralPrefix returns the R2 key prefix for a time-limited upload.
func EphemeralPrefix(days int, id string) string {
	return fmt.Sprintf("dollop/%d/%s", days, id)
}

// PermanentPrefix returns the R2 key prefix for a permanent upload.
func PermanentPrefix(name string) string {
	return "keep/" + name
}

// PublicURL constructs the public URL for a given prefix under baseURL.
func PublicURL(baseURL, prefix string) string {
	base := strings.TrimRight(baseURL, "/")
	return base + "/" + prefix + "/"
}
