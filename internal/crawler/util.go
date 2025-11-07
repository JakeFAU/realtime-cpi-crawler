package crawler

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func containsLower(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func canonicalizeURL(raw string) (string, *url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, err
	}
	parsed.Fragment = ""
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), parsed, nil
}

func safeBasename(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return hashURL(raw)
	}
	host := invalidFilenameChars.ReplaceAllString(u.Hostname(), "_")
	p := strings.Trim(u.EscapedPath(), "/")
	if p == "" {
		p = "root"
	}
	p = invalidFilenameChars.ReplaceAllString(p, "_")
	hash := hashURL(raw)[:16]
	return fmt.Sprintf("%s_%s_%s", host, p, hash)
}

func hashURL(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func htmlFilePath(root string, page Page) string {
	base := safeBasename(page.FinalURL)
	if base == "" {
		base = safeBasename(page.URL)
	}
	return filepath.Join(root, fmt.Sprintf("%s.html", base))
}

func metaFilePath(root string, page Page) string {
	base := safeBasename(page.FinalURL)
	if base == "" {
		base = safeBasename(page.URL)
	}
	return filepath.Join(root, fmt.Sprintf("%s.json", base))
}

func sameHost(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Hostname(), b.Hostname())
}
