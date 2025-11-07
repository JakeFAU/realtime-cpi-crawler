package crawler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConcurrentVisitTracker(t *testing.T) {
	tracker := newConcurrentVisitTracker()
	require.True(t, tracker.MarkIfNew("https://example.org/first"))
	require.False(t, tracker.MarkIfNew("https://example.org/first"))
	require.True(t, tracker.MarkIfNew("https://example.org/second"))
}

func TestThresholdDomainBlocker(t *testing.T) {
	blocker := newThresholdDomainBlocker(2)
	require.False(t, blocker.IsBlocked("example.org"))
	require.False(t, blocker.MarkForbidden("example.org"))
	require.True(t, blocker.MarkForbidden("example.org"))
	require.True(t, blocker.IsBlocked("EXAMPLE.ORG"), "host comparison should be case-insensitive")
}

func TestTimerPauseControllerHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pauser := &timerPauseController{}
	start := time.Now()
	pauser.Pause(ctx, 5*time.Second)
	require.Less(t, time.Since(start), time.Second, "pause should exit immediately when context is done")
}
