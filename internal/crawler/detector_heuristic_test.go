package crawler

import (
	"context"
	"testing"
)

func TestHeuristicDetector(t *testing.T) {
	d := NewHeuristicDetector(10, []string{"#content"}, []string{"lazy"})
	ctx := context.Background()

	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "small body triggers", body: "hi", want: true},
		{name: "keyword triggers", body: "<html>lazy markup</html>", want: true},
		{name: "missing selector triggers", body: "<html><body><div id=\"other\"></div></body></html>", want: true},
		{name: "all conditions satisfied", body: "<div id=\"content\">ok</div> and enough bytes", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.NeedsJS(ctx, Page{Body: []byte(tt.body)})
			if got != tt.want {
				t.Fatalf("expected %v got %v", tt.want, got)
			}
		})
	}
}
