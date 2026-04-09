package connectors

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type fallbackConnectorFactory func(string, config.Config) (sources.Connector, error)

func DownloadWithFallback(cfg config.Config, factory fallbackConnectorFactory, sourceID string, p paper.Paper, saveDir string, allowSciHub bool, sciHubBaseURL string) (sources.RetrievalResult, error) {
	p = p.Normalized()
	if factory == nil {
		factory = New
	}

	attempts := make([]sources.RetrievalAttempt, 0, 8)

	primaryResult, err := downloadPrimary(cfg, factory, sourceID, p, saveDir)
	if err != nil {
		return sources.RetrievalResult{}, err
	}
	attempts = append(attempts, primaryResult.Attempts...)
	if primaryResult.State == sources.RetrievalStateDownloaded {
		primaryResult.Attempts = attempts
		primaryResult.WinningStage = "primary"
		return primaryResult, nil
	}

	repositorySources := []string{"openaire", "core", "europepmc", "pmc"}
	for _, repositoryID := range repositorySources {
		attempt, result, err := repositoryFallbackAttempt(cfg, factory, repositoryID, p, saveDir)
		if err != nil {
			return sources.RetrievalResult{}, err
		}
		attempts = append(attempts, attempt)
		if result != nil {
			result.Attempts = attempts
			result.WinningStage = "repositories"
			return *result, nil
		}
	}

	unpaywallAttempt, unpaywallResult, err := unpaywallFallbackAttempt(cfg, factory, p, saveDir)
	if err != nil {
		return sources.RetrievalResult{}, err
	}
	attempts = append(attempts, unpaywallAttempt)
	if unpaywallResult != nil {
		unpaywallResult.Attempts = attempts
		unpaywallResult.WinningStage = "unpaywall"
		return *unpaywallResult, nil
	}

	if !allowSciHub {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateFailed,
			Message:  "download failed after OA fallback chain",
			Attempts: attempts,
		}, nil
	}

	sciHubAttempt, sciHubResult := scihubFallbackAttempt(p, saveDir, sciHubBaseURL)
	attempts = append(attempts, sciHubAttempt)
	if sciHubResult != nil {
		sciHubResult.Attempts = attempts
		sciHubResult.WinningStage = "scihub"
		return *sciHubResult, nil
	}

	return sources.RetrievalResult{
		State:    sources.RetrievalStateFailed,
		Message:  "download failed after OA fallback chain and Sci-Hub fallback",
		Attempts: attempts,
	}, nil
}

func downloadPrimary(cfg config.Config, factory fallbackConnectorFactory, sourceID string, p paper.Paper, saveDir string) (sources.RetrievalResult, error) {
	connector, err := factory(sourceID, cfg)
	if err != nil {
		return sources.RetrievalResult{}, err
	}

	result, err := connector.Download(sources.DownloadRequest{Paper: p, SaveDir: saveDir})
	if err != nil {
		return sources.RetrievalResult{}, err
	}
	result.Attempts = []sources.RetrievalAttempt{{
		Stage:   "primary",
		Source:  sourceID,
		State:   string(result.State),
		Message: result.Message,
		Path:    result.Path,
	}}
	if result.State == sources.RetrievalStateDownloaded {
		result.WinningStage = "primary"
	}
	return result, nil
}

func repositoryFallbackAttempt(cfg config.Config, factory fallbackConnectorFactory, repositoryID string, p paper.Paper, saveDir string) (sources.RetrievalAttempt, *sources.RetrievalResult, error) {
	query := strings.TrimSpace(p.DOI)
	if query == "" {
		query = strings.TrimSpace(p.Title)
	}
	if query == "" {
		return sources.RetrievalAttempt{
			Stage:   "repositories",
			Source:  repositoryID,
			State:   "skipped",
			Message: "doi or title not provided",
		}, nil, nil
	}

	connector, err := factory(repositoryID, cfg)
	if err != nil {
		return sources.RetrievalAttempt{}, nil, err
	}

	searchResult, err := connector.Search(sources.SearchRequest{Query: query, Limit: 5})
	if err != nil {
		return sources.RetrievalAttempt{
			Stage:   "repositories",
			Source:  repositoryID,
			State:   "failed",
			Message: err.Error(),
		}, nil, nil
	}

	for _, candidate := range searchResult.Papers {
		candidate = candidate.Normalized()
		if strings.TrimSpace(candidate.PDFURL) == "" {
			continue
		}
		result, _, err := retrievePaperPDF(repositoryID, candidate, saveDir)
		if err != nil {
			return sources.RetrievalAttempt{}, nil, err
		}
		if result.State == sources.RetrievalStateDownloaded {
			attempt := sources.RetrievalAttempt{
				Stage:   "repositories",
				Source:  repositoryID,
				State:   string(result.State),
				Message: "repository rediscovery succeeded",
				Path:    result.Path,
			}
			result.Attempts = []sources.RetrievalAttempt{attempt}
			return attempt, &result, nil
		}
	}

	return sources.RetrievalAttempt{
		Stage:   "repositories",
		Source:  repositoryID,
		State:   "not_found",
		Message: "repository rediscovery found no downloadable pdf",
	}, nil, nil
}

func unpaywallFallbackAttempt(cfg config.Config, factory fallbackConnectorFactory, p paper.Paper, saveDir string) (sources.RetrievalAttempt, *sources.RetrievalResult, error) {
	if strings.TrimSpace(p.DOI) == "" {
		return sources.RetrievalAttempt{
			Stage:   "unpaywall",
			Source:  "unpaywall",
			State:   "skipped",
			Message: "doi not provided",
		}, nil, nil
	}

	if strings.TrimSpace(cfg.UnpaywallEmail) == "" {
		return sources.RetrievalAttempt{
			Stage:   "unpaywall",
			Source:  "unpaywall",
			State:   "skipped",
			Message: "SEARCH_PAPER_UNPAYWALL_EMAIL missing",
		}, nil, nil
	}

	connector, err := factory("unpaywall", cfg)
	if err != nil {
		return sources.RetrievalAttempt{}, nil, err
	}

	searchResult, err := connector.Search(sources.SearchRequest{Query: p.DOI, Limit: 1})
	if err != nil {
		return sources.RetrievalAttempt{
			Stage:   "unpaywall",
			Source:  "unpaywall",
			State:   "failed",
			Message: err.Error(),
		}, nil, nil
	}
	if len(searchResult.Papers) == 0 {
		return sources.RetrievalAttempt{
			Stage:   "unpaywall",
			Source:  "unpaywall",
			State:   "not_found",
			Message: "no oa url found",
		}, nil, nil
	}

	result, _, err := retrievePaperPDF("unpaywall", searchResult.Papers[0], saveDir)
	if err != nil {
		return sources.RetrievalAttempt{}, nil, err
	}
	if result.State == sources.RetrievalStateDownloaded {
		attempt := sources.RetrievalAttempt{
			Stage:   "unpaywall",
			Source:  "unpaywall",
			State:   string(result.State),
			Message: "resolved oa url and downloaded file",
			Path:    result.Path,
		}
		result.Attempts = []sources.RetrievalAttempt{attempt}
		return attempt, &result, nil
	}

	return sources.RetrievalAttempt{
		Stage:   "unpaywall",
		Source:  "unpaywall",
		State:   string(result.State),
		Message: firstNonEmpty(result.Message, "resolved oa url but download failed"),
	}, nil, nil
}

func scihubFallbackAttempt(p paper.Paper, saveDir string, sciHubBaseURL string) (sources.RetrievalAttempt, *sources.RetrievalResult) {
	identifier := firstNonEmpty(strings.TrimSpace(p.DOI), strings.TrimSpace(p.Title), strings.TrimSpace(p.PaperID), strings.TrimSpace(p.URL))
	if identifier == "" {
		return sources.RetrievalAttempt{
			Stage:   "scihub",
			Source:  "scihub",
			State:   "not_found",
			Message: "no doi, title, paper_id, or url provided",
		}, nil
	}

	result, err := DownloadSciHub(identifier, saveDir, sciHubBaseURL)
	if err != nil {
		return sources.RetrievalAttempt{
			Stage:   "scihub",
			Source:  "scihub",
			State:   "failed",
			Message: err.Error(),
		}, nil
	}

	attempt := sources.RetrievalAttempt{
		Stage:   "scihub",
		Source:  "scihub",
		State:   string(result.State),
		Message: result.Message,
		Path:    result.Path,
	}
	if result.State == sources.RetrievalStateDownloaded {
		result.Attempts = []sources.RetrievalAttempt{attempt}
		return attempt, &result
	}
	return attempt, nil
}

func DownloadSciHub(identifier string, saveDir string, baseURL string) (sources.RetrievalResult, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateNotFound,
			Message:  "scihub identifier is empty",
			Attempts: []sources.RetrievalAttempt{},
		}, nil
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return sources.RetrievalResult{}, err
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://sci-hub.se"
	}

	searchURL := baseURL + "/" + strings.TrimLeft(identifier, "/")
	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateFailed,
			Message:  fmt.Sprintf("Sci-Hub request failed: %v", err),
			Attempts: []sources.RetrievalAttempt{},
		}, nil
	}
	body, err := executeBytes(defaultHTTPClient(), req)
	if err != nil {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateFailed,
			Message:  fmt.Sprintf("Sci-Hub request failed: %v", err),
			Attempts: []sources.RetrievalAttempt{},
		}, nil
	}

	pdfURL := parseSciHubPDFURL(baseURL, string(body))
	if pdfURL == "" {
		return sources.RetrievalResult{
			State:    sources.RetrievalStateNotFound,
			Message:  "Sci-Hub download failed. Try DOI first, then title, or change mirror URL.",
			Attempts: []sources.RetrievalAttempt{},
		}, nil
	}

	result, _, err := retrievePaperPDF("scihub", paper.Paper{
		PaperID: identifier,
		Title:   identifier,
		PDFURL:  pdfURL,
		Source:  "scihub",
	}, saveDir)
	if err != nil {
		return sources.RetrievalResult{}, err
	}
	if result.State != sources.RetrievalStateDownloaded {
		return sources.RetrievalResult{
			State:    result.State,
			Message:  firstNonEmpty(result.Message, "Sci-Hub download failed. Try DOI first, then title, or change mirror URL."),
			Attempts: []sources.RetrievalAttempt{},
		}, nil
	}

	result.Path = filepath.Clean(result.Path)
	return result, nil
}
