package crawler

import (
	"bytes"
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HeuristicDetector implements the Detector interface using simple HTML signals.
type HeuristicDetector struct {
	minHTMLBytes int
	selectors    []string
	keywords     [][]byte
}

// NewHeuristicDetector constructs a Detector with the configured thresholds.
func NewHeuristicDetector(minBytes int, selectors, keywords []string) *HeuristicDetector {
	lowerKeywords := make([][]byte, 0, len(keywords))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		lowerKeywords = append(lowerKeywords, bytes.ToLower([]byte(kw)))
	}
	return &HeuristicDetector{
		minHTMLBytes: minBytes,
		selectors:    selectors,
		keywords:     lowerKeywords,
	}
}

// NeedsJS inspects the page for signals that indicate JS rendering is required.
func (d *HeuristicDetector) NeedsJS(_ context.Context, page Page) bool {
	if d == nil {
		return false
	}
	bodyLen := len(page.Body)
	if d.minHTMLBytes > 0 && bodyLen < d.minHTMLBytes {
		return true
	}
	lowerBody := bytes.ToLower(page.Body)
	for _, kw := range d.keywords {
		if len(kw) == 0 {
			continue
		}
		if bytes.Contains(lowerBody, kw) {
			return true
		}
	}
	if len(d.selectors) == 0 || bodyLen == 0 {
		return false
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page.Body))
	if err != nil {
		return true
	}
	for _, sel := range d.selectors {
		if sel == "" {
			continue
		}
		if doc.Find(sel).Length() == 0 {
			return true
		}
	}
	return false
}
