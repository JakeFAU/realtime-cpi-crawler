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
	switch {
	case d.bodyBelowThreshold(page.Body):
		return true
	case d.containsKeywords(page.Body):
		return true
	default:
		return d.missingSelectors(page.Body)
	}
}

func (d *HeuristicDetector) bodyBelowThreshold(body []byte) bool {
	return d.minHTMLBytes > 0 && len(body) < d.minHTMLBytes
}

func (d *HeuristicDetector) containsKeywords(body []byte) bool {
	if len(body) == 0 || len(d.keywords) == 0 {
		return false
	}
	lowerBody := bytes.ToLower(body)
	for _, kw := range d.keywords {
		if len(kw) == 0 {
			continue
		}
		if bytes.Contains(lowerBody, kw) {
			return true
		}
	}
	return false
}

func (d *HeuristicDetector) missingSelectors(body []byte) bool {
	if len(d.selectors) == 0 || len(body) == 0 {
		return false
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
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
