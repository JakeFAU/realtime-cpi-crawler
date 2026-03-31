package detector

import (
	"strings"
	"testing"
)

func BenchmarkScriptDensityHigh(b *testing.B) {
	body := []byte("<html><head><title>Test</title>" +
		strings.Repeat("<SCRIPT>var a=1;</script>", 100) +
		"</head><body>" + strings.Repeat("<p>Test</p>", 1000) +
		"</body></html>")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scriptDensityHigh(body)
	}
}
