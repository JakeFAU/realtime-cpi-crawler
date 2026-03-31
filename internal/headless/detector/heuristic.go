// Package detector decides when to promote crawls to headless renderers.
package detector

import (
	"bytes"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Heuristic implements a handful of rule-based promotions.
type Heuristic struct {
	BodyLengthThreshold int
}

// NewHeuristic creates a new detector.
func NewHeuristic(threshold int) *Heuristic {
	if threshold == 0 {
		threshold = 2048
	}
	return &Heuristic{BodyLengthThreshold: threshold}
}

var spaMarkers = [][]byte{
	[]byte("__next"),
	[]byte("id=\"root\""),
	[]byte("id=\"app\""),
	[]byte("data-reactroot"),
}

// ShouldPromote decides whether a headless fetch is required.
func (h *Heuristic) ShouldPromote(resp crawler.FetchResponse) bool {
	if resp.StatusCode != 200 {
		return false
	}
	body := resp.Body
	if len(body) == 0 {
		return true
	}
	if len(body) < h.BodyLengthThreshold && scriptDensityHigh(body) {
		return true
	}
	for _, marker := range spaMarkers {
		if bytes.Contains(body, marker) {
			return true
		}
	}
	return false
}

func scriptDensityHigh(body []byte) bool {
	total := len(body)
	if total == 0 {
		return false
	}

	scriptCoverage := 0
	searchPos := 0

	for {
		relativeStart := indexScriptOpen(body[searchPos:])
		if relativeStart == -1 {
			break
		}
		start := searchPos + relativeStart

		tagClose := bytes.IndexByte(body[start:], '>')
		if tagClose == -1 {
			// Treat the rest of the document as part of the malformed script.
			scriptCoverage += total - start
			break
		}
		contentStart := start + tagClose + 1

		relativeEnd := indexScriptClose(body[contentStart:])
		var nextSearch int
		if relativeEnd == -1 {
			// Script tag never closes; count the rest.
			nextSearch = total
		} else {
			nextSearch = contentStart + relativeEnd + 9 // len("</script>")
		}

		scriptCoverage += nextSearch - start
		searchPos = nextSearch
	}

	if scriptCoverage == 0 {
		return false
	}
	return scriptCoverage*100/total >= 25
}

// equalFold checks if a and b are equal, treating upper and lower case ASCII letters as equal.
func equalFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

var (
	scriptOpenTag  = []byte("<script")
	scriptCloseTag = []byte("</script>")
)

// indexFold returns the index of the first instance of target in s,
// ignoring case, or -1 if not present.
// It is optimized for searching strings starting with '<'.
func indexFold(s, target []byte) int {
	if len(target) == 0 {
		return 0
	}
	if len(s) < len(target) {
		return -1
	}

	for i := 0; i <= len(s)-len(target); {
		idx := bytes.IndexByte(s[i:], target[0])
		if idx == -1 {
			return -1
		}
		i += idx
		if i > len(s)-len(target) {
			return -1
		}
		if equalFold(s[i:i+len(target)], target) {
			return i
		}
		i++
	}
	return -1
}

// indexScriptOpen returns the index of the first instance of "<script"
// (case-insensitive) in s, or -1 if not present.
func indexScriptOpen(s []byte) int {
	return indexFold(s, scriptOpenTag)
}

// indexScriptClose returns the index of the first instance of "</script>"
// (case-insensitive) in s, or -1 if not present.
func indexScriptClose(s []byte) int {
	return indexFold(s, scriptCloseTag)
}
