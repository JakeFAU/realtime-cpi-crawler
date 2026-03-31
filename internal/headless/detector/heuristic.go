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

// indexScriptOpen returns the index of the first instance of "<script"
// (case-insensitive) in s, or -1 if not present.
//
//nolint:gocyclo // performance-sensitive unrolled matching
func indexScriptOpen(s []byte) int {
	for i := 0; i <= len(s)-7; {
		idx := bytes.IndexByte(s[i:], '<')
		if idx == -1 {
			return -1
		}
		i += idx
		if i > len(s)-7 {
			return -1
		}
		if (s[i+1] == 's' || s[i+1] == 'S') &&
			(s[i+2] == 'c' || s[i+2] == 'C') &&
			(s[i+3] == 'r' || s[i+3] == 'R') &&
			(s[i+4] == 'i' || s[i+4] == 'I') &&
			(s[i+5] == 'p' || s[i+5] == 'P') &&
			(s[i+6] == 't' || s[i+6] == 'T') {
			return i
		}
		i++
	}
	return -1
}

// indexScriptClose returns the index of the first instance of "</script>"
// (case-insensitive) in s, or -1 if not present.
//
//nolint:gocyclo // performance-sensitive unrolled matching
func indexScriptClose(s []byte) int {
	for i := 0; i <= len(s)-9; {
		idx := bytes.IndexByte(s[i:], '<')
		if idx == -1 {
			return -1
		}
		i += idx
		if i > len(s)-9 {
			return -1
		}
		if s[i+1] == '/' &&
			(s[i+2] == 's' || s[i+2] == 'S') &&
			(s[i+3] == 'c' || s[i+3] == 'C') &&
			(s[i+4] == 'r' || s[i+4] == 'R') &&
			(s[i+5] == 'i' || s[i+5] == 'I') &&
			(s[i+6] == 'p' || s[i+6] == 'P') &&
			(s[i+7] == 't' || s[i+7] == 'T') &&
			s[i+8] == '>' {
			return i
		}
		i++
	}
	return -1
}
