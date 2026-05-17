package connectors

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	pdf "github.com/ledongthuc/pdf"

	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

var (
	doiPattern     = regexp.MustCompile(`(?i)\b(?:https?://(?:dx\.)?doi\.org/|doi:\s*)?(10\.\d{4,9}/[-._;()/:A-Z0-9]+)\b`)
	spaceRegexp    = regexp.MustCompile(`\s+`)
	pdfTextPattern = regexp.MustCompile(`\(([^()]|\\.)+\)\s*Tj`)
	httpCache      = responseCache{
		entries: map[string]cachedResponse{},
	}
)

type cachedResponse struct {
	body      []byte
	expiresAt time.Time
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]cachedResponse
}

type HTTPStatusError struct {
	StatusCode  int
	BodySnippet string
	RetryAfter  string
	Attempts    int
}

func (e HTTPStatusError) Error() string {
	parts := []string{fmt.Sprintf("unexpected status %d after %d attempt(s)", e.StatusCode, e.Attempts)}
	if e.RetryAfter != "" {
		parts = append(parts, "retry_after="+e.RetryAfter)
	}
	if e.BodySnippet != "" {
		parts = append(parts, "body="+e.BodySnippet)
	}
	return strings.Join(parts, "; ")
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func executeJSON(client *http.Client, request *http.Request, target any) error {
	body, err := executeRequestBytes(client, request)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func executeBytes(client *http.Client, request *http.Request) ([]byte, error) {
	return executeRequestBytes(client, request)
}

func executeRequestBytes(client *http.Client, request *http.Request) ([]byte, error) {
	if client == nil {
		client = defaultHTTPClient()
	}

	cacheKey := httpCacheKey(request)
	if cacheKey != "" {
		if body, ok := httpCache.get(cacheKey); ok {
			return body, nil
		}
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := cloneRequestForAttempt(request)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts && retryableNetworkError(err) {
				time.Sleep(retryDelay(attempt, ""))
				continue
			}
			return nil, err
		}

		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxAttempts && retryableNetworkError(readErr) {
				time.Sleep(retryDelay(attempt, ""))
				continue
			}
			return nil, readErr
		}

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			if cacheKey != "" {
				httpCache.set(cacheKey, body, 5*time.Minute)
			}
			return body, nil
		}

		statusErr := HTTPStatusError{
			StatusCode:  response.StatusCode,
			BodySnippet: bodySnippet(body),
			RetryAfter:  strings.TrimSpace(response.Header.Get("Retry-After")),
			Attempts:    attempt,
		}
		lastErr = statusErr
		if attempt < maxAttempts && retryableStatus(response.StatusCode) {
			time.Sleep(retryDelay(attempt, statusErr.RetryAfter))
			continue
		}
		return nil, statusErr
	}

	return nil, lastErr
}

func cloneRequestForAttempt(request *http.Request) (*http.Request, error) {
	req := request.Clone(request.Context())
	if request.Body == nil {
		return req, nil
	}
	if request.GetBody == nil {
		req.Body = request.Body
		return req, nil
	}
	body, err := request.GetBody()
	if err != nil {
		return nil, err
	}
	req.Body = body
	return req, nil
}

func retryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryableNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "temporary") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "eof")
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
			delay := time.Duration(seconds) * time.Second
			if delay <= 5*time.Second {
				return delay
			}
			return 5 * time.Second
		}
		if parsed, err := http.ParseTime(retryAfter); err == nil {
			delay := time.Until(parsed)
			if delay > 0 && delay <= 5*time.Second {
				return delay
			}
			if delay > 5*time.Second {
				return 5 * time.Second
			}
		}
	}

	switch attempt {
	case 1:
		return 250 * time.Millisecond
	case 2:
		return 500 * time.Millisecond
	default:
		return time.Second
	}
}

func bodySnippet(body []byte) string {
	value := strings.TrimSpace(string(body))
	if len(value) > 180 {
		value = value[:180]
	}
	return strings.ReplaceAll(value, "\n", " ")
}

func httpCacheKey(request *http.Request) string {
	if request == nil || request.Method != http.MethodGet || request.URL == nil {
		return ""
	}
	return request.Method + " " + request.URL.String()
}

func (c *responseCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	body := append([]byte(nil), entry.body...)
	return body, true
}

func (c *responseCache) set(key string, body []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cachedResponse{
		body:      append([]byte(nil), body...),
		expiresAt: time.Now().Add(ttl),
	}
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

func retrievalResult(state sources.RetrievalState, message string) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{
		State:    state,
		Message:  message,
		Attempts: []sources.RetrievalAttempt{},
	}, nil
}

func unsupportedDownload(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q direct download is not supported", sourceID))
}

func unsupportedRead(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q direct read is not supported", sourceID))
}

func metadataOnlyUnsupportedDownload(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q exposes metadata and OA link hints only; direct download is not supported", sourceID))
}

func metadataOnlyUnsupportedRead(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q exposes metadata and OA link hints only; direct read is not supported", sourceID))
}

func gatedSkeletonDownload(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q retrieval is an env-gated skeleton and direct download is not implemented yet", sourceID))
}

func gatedSkeletonRead(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateUnsupported, fmt.Sprintf("source %q retrieval is an env-gated skeleton and direct read is not implemented yet", sourceID))
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
			State:    sources.RetrievalStateDownloadedButNotExtractable,
			Path:     result.Path,
			Message:  fmt.Sprintf("source %q downloaded a PDF but no extractable text was detected", sourceID),
			Attempts: cloneAttempts(result.Attempts),
		}, nil
	}

	return sources.RetrievalResult{
		State:        sources.RetrievalStateExtracted,
		Path:         result.Path,
		Content:      content,
		WinningStage: result.WinningStage,
		Attempts:     cloneAttempts(result.Attempts),
	}, nil
}

func informationalRead(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateInformational, fmt.Sprintf("source %q only exposes metadata through search at this stage", sourceID))
}

func informationalDownload(sourceID string) (sources.RetrievalResult, error) {
	return retrievalResult(sources.RetrievalStateInformational, fmt.Sprintf("source %q does not provide direct download through search metadata", sourceID))
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
			State:    sources.RetrievalStateNotFound,
			Message:  fmt.Sprintf("source %q does not expose a public PDF for this record", sourceID),
			Attempts: []sources.RetrievalAttempt{},
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
			State:    sources.RetrievalStateNotFound,
			Message:  fmt.Sprintf("source %q does not expose a public PDF for this record", sourceID),
			Attempts: []sources.RetrievalAttempt{},
		}, nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateFailed,
			Message:  fmt.Sprintf("source %q returned unexpected status %d", sourceID, resp.StatusCode),
			Attempts: []sources.RetrievalAttempt{},
		}, nil, nil
	}
	if !looksLikePDF(resp.Header.Get("Content-Type"), pdfURL, body) {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateNotFound,
			Message:  fmt.Sprintf("source %q did not provide a public PDF for this record", sourceID),
			Attempts: []sources.RetrievalAttempt{},
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
		State:        sources.RetrievalStateDownloaded,
		Path:         targetPath,
		Attempts:     []sources.RetrievalAttempt{},
		WinningStage: "",
	}, body, nil
}

func cloneAttempts(attempts []sources.RetrievalAttempt) []sources.RetrievalAttempt {
	if len(attempts) == 0 {
		return []sources.RetrievalAttempt{}
	}
	cloned := make([]sources.RetrievalAttempt, len(attempts))
	copy(cloned, attempts)
	return cloned
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

func looksLikePDF(_ string, _ string, body []byte) bool {
	if len(body) == 0 {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("%PDF-")) {
		return true
	}
	return false
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

func extractPDFText(body []byte) string {
	reader, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err == nil {
		plain, plainErr := reader.GetPlainText()
		if plainErr == nil {
			var parts []string
			scanner := bufio.NewScanner(plain)
			for scanner.Scan() {
				text := normalizeText(scanner.Text())
				if text != "" {
					parts = append(parts, text)
				}
			}
			if scanner.Err() == nil {
				if content := strings.TrimSpace(strings.Join(parts, " ")); content != "" {
					return content
				}
			}
		}
	}
	return extractPDFTextLegacy(body)
}

func extractPDFTextLegacy(body []byte) string {
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
		text = normalizeText(text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
