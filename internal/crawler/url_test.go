package crawler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{
			name:   "lowercase scheme and host",
			rawURL: "HTTP://EXAMPLE.COM/Path",
			want:   "http://example.com/Path",
		},
		{
			name:   "remove default http port",
			rawURL: "http://example.com:80/index.html",
			want:   "http://example.com/index.html",
		},
		{
			name:   "remove default https port",
			rawURL: "https://example.com:443/",
			want:   "https://example.com/",
		},
		{
			name:   "preserve non-default port",
			rawURL: "http://example.com:8080/path",
			want:   "http://example.com:8080/path",
		},
		{
			name:   "remove fragment",
			rawURL: "https://example.com/page#section1",
			want:   "https://example.com/page",
		},
		{
			name:   "sort query parameters",
			rawURL: "https://example.com/search?q=test&a=1&b=2",
			want:   "https://example.com/search?a=1&b=2&q=test",
		},
		{
			name:   "handle already normalized url",
			rawURL: "https://example.com/path?a=1",
			want:   "https://example.com/path?a=1",
		},
		{
			name:    "invalid url",
			rawURL:  ":%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeURL(tt.rawURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
