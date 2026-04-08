package connectors

import (
	"regexp"
	"strings"
)

var (
	sciHubEmbedPattern  = regexp.MustCompile(`(?is)<embed[^>]+type=["']application/pdf["'][^>]+src=["']([^"']+)["']`)
	sciHubIframePattern = regexp.MustCompile(`(?is)<iframe[^>]+src=["']([^"']+)["']`)
	sciHubLinkPattern   = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*pdf[^"']*)["']`)
)

func parseSciHubPDFURL(baseURL string, body string) string {
	for _, pattern := range []*regexp.Regexp{sciHubEmbedPattern, sciHubIframePattern, sciHubLinkPattern} {
		matches := pattern.FindStringSubmatch(body)
		if len(matches) < 2 {
			continue
		}
		return resolveSciHubURL(baseURL, matches[1])
	}
	return ""
}

func resolveSciHubURL(baseURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	if strings.HasPrefix(href, "/") {
		return strings.TrimRight(baseURL, "/") + href
	}
	return strings.TrimRight(baseURL, "/") + "/" + href
}
