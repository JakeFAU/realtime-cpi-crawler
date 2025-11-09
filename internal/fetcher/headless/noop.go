package headless

import (
	"context"
	"errors"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Noop implements Fetcher but always returns an error to indicate that
// headless browsing is not available in the current build.
type Noop struct{}

// NewNoop creates a new Noop fetcher.
func NewNoop() *Noop {
	return &Noop{}
}

// Fetch returns an error since this is a stub implementation.
func (Noop) Fetch(_ context.Context, _ crawler.FetchRequest) (crawler.FetchResponse, error) {
	return crawler.FetchResponse{}, errors.New("headless fetcher not configured")
}
