package paper

import (
	"strings"
	"unicode"
)

type Paper struct {
	PaperID       string   `json:"paper_id"`
	Title         string   `json:"title"`
	Authors       []string `json:"authors"`
	Abstract      string   `json:"abstract"`
	DOI           string   `json:"doi"`
	PublishedDate string   `json:"published_date"`
	PDFURL        string   `json:"pdf_url"`
	URL           string   `json:"url"`
	Source        string   `json:"source"`
}

func (p Paper) Normalized() Paper {
	normalized := Paper{
		PaperID:       normalizeSpace(p.PaperID),
		Title:         normalizeSpace(p.Title),
		Authors:       make([]string, 0, len(p.Authors)),
		Abstract:      normalizeSpace(p.Abstract),
		DOI:           normalizeDOI(p.DOI),
		PublishedDate: normalizeSpace(p.PublishedDate),
		PDFURL:        strings.TrimSpace(p.PDFURL),
		URL:           strings.TrimSpace(p.URL),
		Source:        strings.ToLower(strings.TrimSpace(p.Source)),
	}

	if len(p.Authors) != 0 {
		for _, author := range p.Authors {
			author = normalizeSpace(author)
			if author == "" {
				continue
			}
			normalized.Authors = append(normalized.Authors, author)
		}
	}

	return normalized
}

func (p Paper) IdentityKey() string {
	normalized := p.Normalized()
	if normalized.DOI != "" {
		return "doi:" + normalized.DOI
	}

	if normalized.Title != "" && len(normalized.Authors) != 0 {
		parts := []string{strings.ToLower(normalized.Title)}
		for _, author := range normalized.Authors {
			parts = append(parts, strings.ToLower(author))
		}
		return "title-authors:" + strings.Join(parts, "|")
	}

	if normalized.PaperID != "" {
		return "paper-id:" + strings.ToLower(normalized.PaperID)
	}

	return ""
}

func Dedupe(papers []Paper) []Paper {
	if len(papers) == 0 {
		return []Paper{}
	}

	result := make([]Paper, 0, len(papers))
	seen := make(map[string]struct{}, len(papers))
	for _, raw := range papers {
		normalized := raw.Normalized()
		key := normalized.IdentityKey()
		if key == "" {
			result = append(result, normalized)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, normalized)
	}

	return result
}

func normalizeDOI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	lower := strings.ToLower(value)
	for _, prefix := range []string{
		"https://doi.org/",
		"http://doi.org/",
		"https://dx.doi.org/",
		"http://dx.doi.org/",
		"doi:",
	} {
		if strings.HasPrefix(lower, prefix) {
			value = value[len(prefix):]
			break
		}
	}

	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSpace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	lastWasSpace := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			if lastWasSpace {
				continue
			}
			b.WriteByte(' ')
			lastWasSpace = true
			continue
		}

		b.WriteRune(r)
		lastWasSpace = false
	}

	return b.String()
}
