package crawler

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeURL standardizes a URL to avoid duplicates.
// It lowercases the scheme and host, removes default ports, and sorts query parameters.
// It also removes fragments.
func NormalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports
	if u.Scheme == "http" && strings.HasSuffix(u.Host, ":80") {
		u.Host = strings.TrimSuffix(u.Host, ":80")
	}
	if u.Scheme == "https" && strings.HasSuffix(u.Host, ":443") {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}

	// Remove fragment
	u.Fragment = ""

	// Sort query parameters
	q := u.Query()
	u.RawQuery = q.Encode()

	return u.String(), nil
}
