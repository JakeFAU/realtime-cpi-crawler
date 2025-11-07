package crawler

import "strings"

// domainPatternBlocklist stores exact hosts and suffix wildcards derived from configuration.
type domainPatternBlocklist struct {
	exact    map[string]struct{}
	suffixes []string
}

func newDomainPatternBlocklist(patterns []string) *domainPatternBlocklist {
	matcher := &domainPatternBlocklist{
		exact: make(map[string]struct{}),
	}
	for _, raw := range patterns {
		value := strings.TrimSpace(strings.ToLower(raw))
		if value == "" {
			continue
		}
		switch {
		case strings.HasPrefix(value, "*."):
			suffix := strings.TrimPrefix(value, "*.")
			if suffix != "" {
				matcher.addSuffix(suffix)
			}
		case strings.HasPrefix(value, "."):
			suffix := strings.TrimPrefix(value, ".")
			if suffix != "" {
				matcher.addSuffix(suffix)
			}
		default:
			matcher.exact[value] = struct{}{}
		}
	}
	if len(matcher.exact) == 0 && len(matcher.suffixes) == 0 {
		return nil
	}
	return matcher
}

func (b *domainPatternBlocklist) addSuffix(suffix string) {
	for _, existing := range b.suffixes {
		if existing == suffix {
			return
		}
	}
	b.suffixes = append(b.suffixes, suffix)
}

func (b *domainPatternBlocklist) IsBlocked(host string) bool {
	if b == nil {
		return false
	}
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if _, exact := b.exact[host]; exact {
		return true
	}
	for _, suffix := range b.suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
