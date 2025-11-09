package detector

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestHeuristic_ShouldPromote_EmptyBody(t *testing.T) {
	t.Parallel()

	h := NewHeuristic(100)
	resp := crawler.FetchResponse{
		StatusCode: 200,
		Body:       []byte(""),
	}
	require.True(t, h.ShouldPromote(resp))
}

func TestHeuristic_ShouldPromote_SPAMarkers(t *testing.T) {
	t.Parallel()

	h := NewHeuristic(100)
	resp := crawler.FetchResponse{
		StatusCode: 200,
		Body:       []byte(`<div id="__next"></div>`),
	}
	require.True(t, h.ShouldPromote(resp))
}

func TestHeuristic_ShouldPromote_ScriptDensity(t *testing.T) {
	t.Parallel()

	h := NewHeuristic(1000)
	resp := crawler.FetchResponse{
		StatusCode: 200,
		Body:       []byte(`<html><script>var a=1;</script><p>t</p></html>`),
	}
	require.True(t, h.ShouldPromote(resp))
}

func TestHeuristic_ShouldPromote_DisabledForNon200(t *testing.T) {
	t.Parallel()

	h := NewHeuristic(100)
	resp := crawler.FetchResponse{
		StatusCode: 404,
		Body:       []byte("not found"),
	}
	require.False(t, h.ShouldPromote(resp))
}
