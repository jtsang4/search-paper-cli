package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

var (
	doiPattern  = regexp.MustCompile(`(?i)\b(?:https?://(?:dx\.)?doi\.org/|doi:\s*)?(10\.\d{4,9}/[-._;()/:A-Z0-9]+)\b`)
	tagPattern  = regexp.MustCompile(`<[^>]+>`)
	spaceRegexp = regexp.MustCompile(`\s+`)
)

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func executeJSON(client *http.Client, request *http.Request, target any) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	return json.NewDecoder(response.Body).Decode(target)
}

func executeBytes(client *http.Client, request *http.Request) ([]byte, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	return io.ReadAll(response.Body)
}

func searchResult(items []paper.Paper, limit int) sources.SearchResult {
	normalized := make([]paper.Paper, 0, len(items))
	for _, item := range items {
		normalized = append(normalized, item.Normalized())
	}
	if limit > 0 && len(normalized) > limit {
		normalized = normalized[:limit]
	}
	return sources.SearchResult{
		Count:  len(normalized),
		Papers: normalized,
	}
}

func unsupportedDownload(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q direct download is not supported", sourceID),
	}, nil
}

func unsupportedRead(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q direct read is not supported", sourceID),
	}, nil
}

func metadataOnlyUnsupportedDownload(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q exposes metadata and OA link hints only; direct download is not supported", sourceID),
	}, nil
}

func metadataOnlyUnsupportedRead(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q exposes metadata and OA link hints only; direct read is not supported", sourceID),
	}, nil
}

func gatedSkeletonDownload(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q retrieval is an env-gated skeleton and direct download is not implemented yet", sourceID),
	}, nil
}

func gatedSkeletonRead(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q retrieval is an env-gated skeleton and direct read is not implemented yet", sourceID),
	}, nil
}

func nativeDownload(sourceID string, request sources.DownloadRequest) (sources.RetrievalResult, error) {
	result, _, err := retrievePaperPDF(sourceID, request.Paper, request.SaveDir)
	return result, err
}

func nativeRead(sourceID string, request sources.ReadRequest) (sources.RetrievalResult, error) {
	result, body, err := retrievePaperPDF(sourceID, request.Paper, request.SaveDir)
	if err != nil {
		return sources.RetrievalResult{}, err
	}
	if result.State != sources.RetrievalStateDownloaded {
		return result, nil
	}

	content := extractPDFText(body)
	if content == "" {
		return sources.RetrievalResult{
			State:   sources.RetrievalStateDownloadedButNotExtractable,
			Path:    result.Path,
			Message: fmt.Sprintf("source %q downloaded a PDF but no extractable text was detected", sourceID),
		}, nil
	}

	return sources.RetrievalResult{
		State:   sources.RetrievalStateExtracted,
		Path:    result.Path,
		Content: content,
	}, nil
}

func informationalRead(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateInformational,
		Message: fmt.Sprintf("source %q only exposes metadata through search at this stage", sourceID),
	}, nil
}

func informationalDownload(sourceID string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:   sources.RetrievalStateInformational,
		Message: fmt.Sprintf("source %q does not provide direct download through search metadata", sourceID),
	}, nil
}

func parseDate(values ...string) string {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		"2006-01",
		"2006",
		"2006/01/02",
	}

	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, raw); err == nil {
				switch layout {
				case "2006":
					return parsed.Format("2006")
				case "2006-01":
					return parsed.Format("2006-01")
				default:
					return parsed.Format("2006-01-02")
				}
			}
		}
		if len(raw) >= 10 {
			candidate := raw[:10]
			if _, err := time.Parse("2006-01-02", candidate); err == nil {
				return candidate
			}
		}
	}

	return ""
}

func extractDOI(values ...string) string {
	for _, raw := range values {
		matches := doiPattern.FindStringSubmatch(raw)
		if len(matches) > 1 {
			return strings.ToLower(strings.TrimSpace(matches[1]))
		}
	}
	return ""
}

func stripHTML(value string) string {
	value = tagPattern.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "&nbsp;", " ")
	value = strings.ReplaceAll(value, "&amp;", "&")
	value = strings.ReplaceAll(value, "&lt;", "<")
	value = strings.ReplaceAll(value, "&gt;", ">")
	value = spaceRegexp.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func splitAuthors(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ";")
	authors := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			authors = append(authors, part)
		}
	}
	return authors
}

func firstPDFURL(urls []string) string {
	for _, item := range urls {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(item)), ".pdf") {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func joinURL(baseURL, href string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := toString(item)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return []string{}
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return []string{}
	}
}

func requireQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("query must not be empty")
	}
	return nil
}

func retrievePaperPDF(sourceID string, p paper.Paper, saveDir string) (sources.RetrievalResult, []byte, error) {
	p = p.Normalized()

	resolvedSaveDir := strings.TrimSpace(saveDir)
	if resolvedSaveDir == "" {
		resolvedSaveDir = "."
	}
	if err := os.MkdirAll(resolvedSaveDir, 0o755); err != nil {
		return sources.RetrievalResult{}, nil, err
	}

	pdfURL := retrievalPDFURL(p)
	if pdfURL == "" {
		return sources.RetrievalResult{
			State:   sources.RetrievalStateNotFound,
			Message: fmt.Sprintf("source %q does not expose a public PDF for this record", sourceID),
		}, nil, nil
	}

	req, err := http.NewRequest(http.MethodGet, pdfURL, nil)
	if err != nil {
		return sources.RetrievalResult{}, nil, err
	}
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return sources.RetrievalResult{}, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sources.RetrievalResult{}, nil, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return sources.RetrievalResult{
			State:   sources.RetrievalStateNotFound,
			Message: fmt.Sprintf("source %q does not expose a public PDF for this record", sourceID),
		}, nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return sources.RetrievalResult{
			State:   sources.RetrievalStateFailed,
			Message: fmt.Sprintf("source %q returned unexpected status %d", sourceID, resp.StatusCode),
		}, nil, nil
	}
	if !looksLikePDF(resp.Header.Get("Content-Type"), pdfURL, body) {
		return sources.RetrievalResult{
			State:   sources.RetrievalStateNotFound,
			Message: fmt.Sprintf("source %q did not provide a public PDF for this record", sourceID),
		}, nil, nil
	}

	filename := retrievalFilename(sourceID, p)
	targetPath := filepath.Join(resolvedSaveDir, filename)
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, body, 0o644); err != nil {
		return sources.RetrievalResult{}, nil, err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return sources.RetrievalResult{}, nil, err
	}

	return sources.RetrievalResult{
		State: sources.RetrievalStateDownloaded,
		Path:  targetPath,
	}, body, nil
}

func retrievalPDFURL(p paper.Paper) string {
	for _, candidate := range []string{p.PDFURL, p.URL} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		lower := strings.ToLower(candidate)
		if strings.HasSuffix(lower, ".pdf") || strings.Contains(lower, "/pdf") {
			return candidate
		}
	}
	return ""
}

func looksLikePDF(contentType string, requestURL string, body []byte) bool {
	if len(body) == 0 {
		return false
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "%PDF-") {
		return true
	}
	lowerType := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(lowerType, "application/pdf") {
		return true
	}
	lowerURL := strings.ToLower(strings.TrimSpace(requestURL))
	return strings.HasSuffix(lowerURL, ".pdf") && strings.HasPrefix(trimmed, "%PDF-")
}

func retrievalFilename(sourceID string, p paper.Paper) string {
	base := sanitizeFilename(p.PaperID)
	if base == "" {
		base = sanitizeFilename(p.Title)
	}
	if base == "" {
		base = sourceID + "-paper"
	}
	if !strings.HasSuffix(strings.ToLower(base), ".pdf") {
		base += ".pdf"
	}
	return base
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-.")
	if result == "" {
		return ""
	}
	return result
}

var pdfTextPattern = regexp.MustCompile(`\(([^()]|\\.)+\)\s*Tj`)

func extractPDFText(body []byte) string {
	matches := pdfTextPattern.FindAllString(string(body), -1)
	if len(matches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		start := strings.Index(match, "(")
		end := strings.LastIndex(match, ")")
		if start < 0 || end <= start {
			continue
		}
		text := match[start+1 : end]
		text = strings.ReplaceAll(text, `\(`, "(")
		text = strings.ReplaceAll(text, `\)`, ")")
		text = strings.ReplaceAll(text, `\\`, `\`)
		text = strings.ReplaceAll(text, `\n`, " ")
		text = strings.ReplaceAll(text, `\r`, " ")
		text = strings.TrimSpace(text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
