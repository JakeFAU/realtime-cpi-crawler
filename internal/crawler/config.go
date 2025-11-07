// Package crawler implements the modern crawling orchestrator and helpers.
package crawler

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// CrawlerConfig captures every configuration knob that influences a crawl run.
// All values originate from Viper so the crawler can be configured via files,
// env vars, or CLI flags.
type CrawlerConfig struct {
	Seeds                  []string
	UserAgent              string
	RespectRobots          bool
	MaxDepth               int
	Concurrency            int
	RateLimitPerDomain     int
	RequestTimeout         time.Duration
	JSRenderTimeout        time.Duration
	JSRenderMaxConcurrency int
	JSRenderDomainQPS      float64
	EscalateOnlySameHost   bool
	FeatureRenderEnabled   bool
	DetectorMinHTMLBytes   int
	DetectorSelectorMust   []string
	DetectorKeywords       []string
	OutputDir              string
	MaxPageBytes           int64
}

// LoadCrawlerConfig constructs a CrawlerConfig by reading from Viper.
func LoadCrawlerConfig(v *viper.Viper) (CrawlerConfig, error) {
	cfg := CrawlerConfig{
		Seeds:                  v.GetStringSlice("crawler.target_urls"),
		UserAgent:              firstSetString(v, []string{"crawler.user_agent", "crawler.useragent"}),
		RespectRobots:          v.GetBool("crawler.respect_robots"),
		MaxDepth:               v.GetInt("crawler.max_depth"),
		Concurrency:            v.GetInt("crawler.concurrency"),
		RateLimitPerDomain:     v.GetInt("crawler.rate_limit_per_domain"),
		RequestTimeout:         v.GetDuration("crawler.request_timeout"),
		JSRenderTimeout:        v.GetDuration("crawler.js_render_timeout"),
		JSRenderMaxConcurrency: v.GetInt("crawler.js_render_max_concurrency"),
		JSRenderDomainQPS:      v.GetFloat64("crawler.js_render_domain_qps"),
		EscalateOnlySameHost:   v.GetBool("crawler.escalate_only_same_host"),
		FeatureRenderEnabled:   v.GetBool("crawler.feature_render_enabled"),
		DetectorMinHTMLBytes:   v.GetInt("detector.min_html_bytes"),
		DetectorSelectorMust:   splitSelectors(v.GetString("detector.selector_must")),
		DetectorKeywords:       normalizeKeywords(v.GetStringSlice("detector.keywords")),
		OutputDir:              v.GetString("crawler.output_dir"),
		MaxPageBytes:           v.GetInt64("crawler.max_page_bytes"),
	}
	return cfg, cfg.Validate()
}

// Validate checks for obviously bad configuration combinations.
func (c CrawlerConfig) Validate() error {
	if len(c.Seeds) == 0 {
		return fmt.Errorf("crawler.target_urls must include at least one seed URL")
	}
	if c.UserAgent == "" {
		return fmt.Errorf("crawler.user_agent must be set")
	}
	if c.MaxDepth < 0 {
		return fmt.Errorf("crawler.max_depth must be >= 0")
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("crawler.concurrency must be > 0")
	}
	if c.RateLimitPerDomain <= 0 {
		return fmt.Errorf("crawler.rate_limit_per_domain must be > 0")
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("crawler.request_timeout must be > 0")
	}
	if c.JSRenderTimeout <= 0 {
		return fmt.Errorf("crawler.js_render_timeout must be > 0")
	}
	if c.JSRenderMaxConcurrency < 0 {
		return fmt.Errorf("crawler.js_render_max_concurrency must be >= 0")
	}
	if c.JSRenderDomainQPS < 0 {
		return fmt.Errorf("crawler.js_render_domain_qps must be >= 0")
	}
	if c.DetectorMinHTMLBytes < 0 {
		return fmt.Errorf("detector.min_html_bytes must be >= 0")
	}
	if c.OutputDir == "" {
		return fmt.Errorf("crawler.output_dir must be set")
	}
	if c.MaxPageBytes <= 0 {
		return fmt.Errorf("crawler.max_page_bytes must be > 0")
	}
	return nil
}

func splitSelectors(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeKeywords(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{})
	for _, kw := range in {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if _, ok := seen[kw]; ok {
			continue
		}
		seen[kw] = struct{}{}
		out = append(out, kw)
	}
	return out
}

func firstSetString(v *viper.Viper, keys []string) string {
	for _, k := range keys {
		if v.IsSet(k) {
			return v.GetString(k)
		}
	}
	return ""
}
