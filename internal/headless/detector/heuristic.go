// Package detector decides when to promote crawls to headless renderers.
package detector

import (
	"bytes"
	"strings"

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
	lower := strings.ToLower(string(body))
	total := len(lower)
	if total == 0 {
		return false
	}

	const (
		openTag  = "<script"
		closeTag = "</script>"
	)
	scriptCoverage := 0
	searchPos := 0

	for {
		relativeStart := strings.Index(lower[searchPos:], openTag)
		if relativeStart == -1 {
			break
		}
		start := searchPos + relativeStart

		tagClose := strings.IndexByte(lower[start:], '>')
		if tagClose == -1 {
			// Treat the rest of the document as part of the malformed script.
			scriptCoverage += total - start
			break
		}
		contentStart := start + tagClose + 1

		relativeEnd := strings.Index(lower[contentStart:], closeTag)
		var nextSearch int
		if relativeEnd == -1 {
			// Script tag never closes; count the rest.
			nextSearch = total
		} else {
			nextSearch = contentStart + relativeEnd + len(closeTag)
		}

		scriptCoverage += nextSearch - start
		searchPos = nextSearch
	}

	if scriptCoverage == 0 {
		return false
	}
	return scriptCoverage*100/total >= 25
}
