package connectors

import "strings"

func parseSciHubPDFURL(baseURL string, body string) string {
	document, err := parseHTMLDocument([]byte(body))
	if err != nil {
		return ""
	}

	for _, selector := range []string{`embed[type="application/pdf"]`, "iframe", "a[href*='.pdf']", "a[href*='pdf']"} {
		if href := selectionAttr(document.Find(selector).First(), "src"); href != "" {
			return resolveSciHubURL(baseURL, href)
		}
		if href := selectionAttr(document.Find(selector).First(), "href"); href != "" {
			return resolveSciHubURL(baseURL, href)
		}
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
