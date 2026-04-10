package connectors

import (
	"bytes"
	"html"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func parseHTMLDocument(body []byte) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(bytes.NewReader(body))
}

func normalizeText(value string) string {
	value = html.UnescapeString(value)
	value = spaceRegexp.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func selectionText(selection *goquery.Selection) string {
	if selection == nil {
		return ""
	}
	return normalizeText(selection.Text())
}

func selectionAttr(selection *goquery.Selection, name string) string {
	if selection == nil {
		return ""
	}
	value, ok := selection.Attr(name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(value))
}

func splitCSVValues(value string, sep string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, sep)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeText(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stripHTML(value string) string {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(value))
	if err != nil {
		return normalizeText(value)
	}
	return selectionText(document.Selection)
}
