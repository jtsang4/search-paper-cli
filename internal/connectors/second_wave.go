package connectors

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type Semantic struct {
	BaseURL string
	Client  *http.Client
	Config  config.Config
}

func NewSemantic(cfg config.Config) *Semantic {
	return &Semantic{
		BaseURL: "https://api.semanticscholar.org/graph/v1/paper/search",
		Client:  defaultHTTPClient(),
		Config:  cfg,
	}
}

func (c *Semantic) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "semantic",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *Semantic) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("query", strings.TrimSpace(request.Query))
	values.Set("limit", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	values.Set("fields", "paperId,title,authors,abstract,publicationDate,externalIds,url,openAccessPdf")
	if year := strings.TrimSpace(request.Year); year != "" {
		values.Set("year", year)
	}
	req.URL.RawQuery = values.Encode()
	if apiKey := strings.TrimSpace(c.Config.SemanticScholarAPIKey); apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	var payload struct {
		Data []struct {
			PaperID string `json:"paperId"`
			Title   string `json:"title"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Abstract        string            `json:"abstract"`
			PublicationDate string            `json:"publicationDate"`
			ExternalIDs     map[string]string `json:"externalIds"`
			URL             string            `json:"url"`
			OpenAccessPDF   struct {
				URL string `json:"url"`
			} `json:"openAccessPdf"`
		} `json:"data"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(payload.Data))
	for _, item := range payload.Data {
		authors := make([]string, 0, len(item.Authors))
		for _, author := range item.Authors {
			if name := strings.TrimSpace(author.Name); name != "" {
				authors = append(authors, name)
			}
		}
		items = append(items, paper.Paper{
			PaperID:       item.PaperID,
			Title:         item.Title,
			Authors:       authors,
			Abstract:      item.Abstract,
			DOI:           extractDOI(item.ExternalIDs["DOI"]),
			PublishedDate: parseDate(item.PublicationDate),
			PDFURL:        item.OpenAccessPDF.URL,
			URL:           item.URL,
			Source:        "semantic",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *Semantic) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("semantic", request)
}

func (c *Semantic) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("semantic", request)
}

func (c *Semantic) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type Crossref struct {
	BaseURL string
	Client  *http.Client
}

func NewCrossref() *Crossref {
	return &Crossref{
		BaseURL: "https://api.crossref.org/works",
		Client:  defaultHTTPClient(),
	}
}

func (c *Crossref) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "crossref",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityInformational,
			Read:     sources.CapabilityInformational,
		},
	}
}

func (c *Crossref) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("query.bibliographic", strings.TrimSpace(request.Query))
	values.Set("rows", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	req.URL.RawQuery = values.Encode()

	var payload struct {
		Message struct {
			Items []struct {
				DOI    string   `json:"DOI"`
				Title  []string `json:"title"`
				Author []struct {
					Given  string `json:"given"`
					Family string `json:"family"`
				} `json:"author"`
				Abstract string `json:"abstract"`
				Issued   struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"issued"`
				URL string `json:"URL"`
			} `json:"items"`
		} `json:"message"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(payload.Message.Items))
	for _, item := range payload.Message.Items {
		authors := make([]string, 0, len(item.Author))
		for _, author := range item.Author {
			name := strings.TrimSpace(strings.TrimSpace(author.Given) + " " + strings.TrimSpace(author.Family))
			if name != "" {
				authors = append(authors, name)
			}
		}
		items = append(items, paper.Paper{
			PaperID:       strings.TrimSpace(item.DOI),
			Title:         firstValue(item.Title),
			Authors:       authors,
			Abstract:      stripHTML(item.Abstract),
			DOI:           extractDOI(item.DOI),
			PublishedDate: crossrefDate(item.Issued.DateParts),
			URL:           item.URL,
			Source:        "crossref",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *Crossref) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return informationalDownload("crossref")
}

func (c *Crossref) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return informationalRead("crossref")
}

func (c *Crossref) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type OpenAlex struct {
	BaseURL string
	Client  *http.Client
}

func NewOpenAlex() *OpenAlex {
	return &OpenAlex{
		BaseURL: "https://api.openalex.org/works",
		Client:  defaultHTTPClient(),
	}
}

func (c *OpenAlex) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "openalex",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityInformational,
			Read:     sources.CapabilityInformational,
		},
	}
}

func (c *OpenAlex) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("search", strings.TrimSpace(request.Query))
	values.Set("per-page", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	req.URL.RawQuery = values.Encode()

	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(payload.Results))
	for _, item := range payload.Results {
		primaryLocation, _ := item["primary_location"].(map[string]any)
		openAccess, _ := item["open_access"].(map[string]any)
		items = append(items, paper.Paper{
			PaperID:       lastPathComponent(toString(item["id"])),
			Title:         toString(item["display_name"]),
			Authors:       extractOpenAlexAuthors(item["authorships"]),
			Abstract:      renderInvertedIndex(item["abstract_inverted_index"]),
			DOI:           extractDOI(idValue(item["ids"], "doi")),
			PublishedDate: parseDate(toString(item["publication_date"])),
			PDFURL:        firstNonEmpty(toString(primaryLocation["pdf_url"]), toString(openAccess["oa_url"])),
			URL:           toString(primaryLocation["landing_page_url"]),
			Source:        "openalex",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *OpenAlex) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return informationalDownload("openalex")
}

func (c *OpenAlex) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return informationalRead("openalex")
}

func (c *OpenAlex) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type GoogleScholar struct {
	BaseURL string
	Client  *http.Client
}

func NewGoogleScholar(cfg config.Config) *GoogleScholar {
	baseURL := strings.TrimSpace(cfg.GoogleScholarProxyURL)
	if baseURL == "" {
		baseURL = "https://scholar.google.com/scholar"
	}
	return &GoogleScholar{
		BaseURL: baseURL,
		Client:  defaultHTTPClient(),
	}
}

func (c *GoogleScholar) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "google-scholar",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityInformational,
			Read:     sources.CapabilityInformational,
		},
	}
}

func (c *GoogleScholar) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("q", strings.TrimSpace(request.Query))
	values.Set("num", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	req.URL.RawQuery = values.Encode()

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	items := parseGoogleScholarResults(c.BaseURL, string(body))
	return searchResult(items, request.Limit), nil
}

func (c *GoogleScholar) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return informationalDownload("google-scholar")
}

func (c *GoogleScholar) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return informationalRead("google-scholar")
}

func (c *GoogleScholar) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type DBLP struct {
	BaseURL string
	Client  *http.Client
}

func NewDBLP() *DBLP {
	return &DBLP{
		BaseURL: "https://dblp.org/search/publ/api",
		Client:  defaultHTTPClient(),
	}
}

func (c *DBLP) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "dblp",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityUnsupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *DBLP) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("q", strings.TrimSpace(request.Query))
	values.Set("h", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	values.Set("format", "xml")
	req.URL.RawQuery = values.Encode()

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type hit struct {
		Info struct {
			Key     string   `xml:"key"`
			Title   string   `xml:"title"`
			Authors []string `xml:"authors>author"`
			Year    string   `xml:"year"`
			EE      string   `xml:"ee"`
			URL     string   `xml:"url"`
		} `xml:"info"`
	}
	type payload struct {
		Hits []hit `xml:"hits>hit"`
	}
	var parsed payload
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(parsed.Hits))
	for _, item := range parsed.Hits {
		items = append(items, paper.Paper{
			PaperID:       item.Info.Key,
			Title:         item.Info.Title,
			Authors:       item.Info.Authors,
			PublishedDate: parseDate(item.Info.Year),
			URL:           dblpURL(item.Info.URL),
			Source:        "dblp",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *DBLP) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return unsupportedDownload("dblp")
}

func (c *DBLP) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return unsupportedRead("dblp")
}

func (c *DBLP) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type OpenAIRE struct {
	BaseURL       string
	LegacyBaseURL string
	Client        *http.Client
}

const (
	openaireResearchProductsURL   = "https://api.openaire.eu/search/researchProducts"
	openaireLegacyPublicationsURL = "https://api.openaire.eu/search/publications"
)

func NewOpenAIRE(cfg ...config.Config) *OpenAIRE {
	baseURL := openaireResearchProductsURL
	legacyBaseURL := openaireLegacyPublicationsURL
	if len(cfg) > 0 {
		if strings.TrimSpace(cfg[0].OpenAIREBaseURL) != "" {
			baseURL = strings.TrimSpace(cfg[0].OpenAIREBaseURL)
		}
		if strings.TrimSpace(cfg[0].OpenAIRELegacyBaseURL) != "" {
			legacyBaseURL = strings.TrimSpace(cfg[0].OpenAIRELegacyBaseURL)
		}
	}
	return &OpenAIRE{
		BaseURL:       baseURL,
		LegacyBaseURL: legacyBaseURL,
		Client:        defaultHTTPClient(),
	}
}

func (c *OpenAIRE) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "openaire",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityUnsupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *OpenAIRE) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	result, err := c.searchXML(request)
	if err == nil {
		return result, nil
	}

	fallback, fallbackErr := c.searchLegacy(request)
	if fallbackErr == nil {
		return fallback, nil
	}

	return sources.SearchResult{}, fmt.Errorf("openaire xml search failed: %v; legacy fallback failed: %w", err, fallbackErr)
}

func (c *OpenAIRE) searchXML(request sources.SearchRequest) (sources.SearchResult, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("keywords", strings.TrimSpace(request.Query))
	values.Set("size", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	values.Set("page", "1")
	req.URL.RawQuery = values.Encode()
	req.Header.Set("Accept", "application/xml, text/xml;q=0.9, */*;q=0.8")

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	var payload openaireXMLElement
	if err := xml.Unmarshal(body, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	resultNodes := openaireTopLevelResults(&payload)
	items := make([]paper.Paper, 0, len(resultNodes))
	for _, node := range resultNodes {
		item, ok := parseOpenAIREXMLResult(node)
		if ok {
			items = append(items, item)
		}
	}
	return searchResult(items, request.Limit), nil
}

func (c *OpenAIRE) searchLegacy(request sources.SearchRequest) (sources.SearchResult, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.legacyBaseURL(), nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("keywords", strings.TrimSpace(request.Query))
	values.Set("size", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	values.Set("format", "json")
	req.URL.RawQuery = values.Encode()

	var payload map[string]any
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	rawResponse, _ := payload["response"].(map[string]any)
	if len(rawResponse) == 0 {
		rawResponse = payload
	}
	rawResults, _ := rawResponse["results"].(map[string]any)
	rawItems := toMapSlice(rawResults["result"])
	items := make([]paper.Paper, 0, len(rawItems))
	for _, item := range rawItems {
		paper, ok := parseOpenAIRELegacyResult(item)
		if ok {
			items = append(items, paper)
		}
	}
	return searchResult(items, request.Limit), nil
}

func (c *OpenAIRE) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return unsupportedDownload("openaire")
}

func (c *OpenAIRE) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return unsupportedRead("openaire")
}

func (c *OpenAIRE) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

func (c *OpenAIRE) legacyBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.BaseURL) != openaireResearchProductsURL && strings.TrimSpace(c.LegacyBaseURL) == openaireLegacyPublicationsURL {
		return strings.Replace(strings.TrimSpace(c.BaseURL), "/search/researchProducts", "/search/publications", 1)
	}
	if strings.TrimSpace(c.LegacyBaseURL) != "" {
		return strings.TrimSpace(c.LegacyBaseURL)
	}
	return strings.Replace(strings.TrimSpace(c.BaseURL), "/search/researchProducts", "/search/publications", 1)
}

type CiteSeerX struct {
	BaseURL string
	Client  *http.Client
}

func NewCiteSeerX() *CiteSeerX {
	return &CiteSeerX{
		BaseURL: "https://citeseerx.ist.psu.edu/search",
		Client:  defaultHTTPClient(),
	}
}

func (c *CiteSeerX) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "citeseerx",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *CiteSeerX) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("q", strings.TrimSpace(request.Query))
	req.URL.RawQuery = values.Encode()

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	items := parseCiteSeerXResults(c.BaseURL, string(body))
	return searchResult(items, request.Limit), nil
}

func (c *CiteSeerX) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("citeseerx", request)
}

func (c *CiteSeerX) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("citeseerx", request)
}

func (c *CiteSeerX) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type SSRN struct {
	BaseURL string
	Client  *http.Client
}

func NewSSRN() *SSRN {
	return &SSRN{
		BaseURL: "https://papers.ssrn.com/sol3/results.cfm",
		Client:  defaultHTTPClient(),
	}
}

func (c *SSRN) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "ssrn",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *SSRN) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("txtKey_Words", strings.TrimSpace(request.Query))
	values.Set("npage", "1")
	req.URL.RawQuery = values.Encode()

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	items := parseSSRNResults(c.BaseURL, string(body))
	return searchResult(items, request.Limit), nil
}

func (c *SSRN) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("ssrn", request)
}

func (c *SSRN) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("ssrn", request)
}

func (c *SSRN) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type Unpaywall struct {
	BaseURL string
	Client  *http.Client
	Config  config.Config
}

func NewUnpaywall(cfg config.Config) *Unpaywall {
	baseURL := "https://api.unpaywall.org/v2"
	if strings.TrimSpace(cfg.UnpaywallBaseURL) != "" {
		baseURL = strings.TrimSpace(cfg.UnpaywallBaseURL)
	}
	return &Unpaywall{
		BaseURL: baseURL,
		Client:  defaultHTTPClient(),
		Config:  cfg,
	}
}

func (c *Unpaywall) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "unpaywall",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityUnsupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *Unpaywall) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if strings.TrimSpace(c.Config.UnpaywallEmail) == "" {
		return sources.SearchResult{}, nil
	}

	doi := extractDOI(request.Query)
	if doi == "" {
		return sources.SearchResult{}, nil
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + url.PathEscape(doi)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("email", strings.TrimSpace(c.Config.UnpaywallEmail))
	req.URL.RawQuery = values.Encode()

	var payload struct {
		DOI            string `json:"doi"`
		Title          string `json:"title"`
		PublishedDate  string `json:"published_date"`
		BestOALocation struct {
			URL       string `json:"url"`
			URLForPDF string `json:"url_for_pdf"`
		} `json:"best_oa_location"`
		Authors []struct {
			Family string `json:"family"`
			Given  string `json:"given"`
		} `json:"z_authors"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	authors := make([]string, 0, len(payload.Authors))
	for _, author := range payload.Authors {
		name := strings.TrimSpace(strings.TrimSpace(author.Given) + " " + strings.TrimSpace(author.Family))
		if name != "" {
			authors = append(authors, name)
		}
	}

	return searchResult([]paper.Paper{{
		PaperID:       payload.DOI,
		Title:         payload.Title,
		Authors:       authors,
		DOI:           extractDOI(payload.DOI),
		PublishedDate: parseDate(payload.PublishedDate),
		PDFURL:        payload.BestOALocation.URLForPDF,
		URL:           payload.BestOALocation.URL,
		Source:        "unpaywall",
	}}, request.Limit), nil
}

func (c *Unpaywall) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return metadataOnlyUnsupportedDownload("unpaywall")
}

func (c *Unpaywall) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return metadataOnlyUnsupportedRead("unpaywall")
}

func (c *Unpaywall) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type IEEE struct {
	Config config.Config
}

func NewIEEE(cfg config.Config) *IEEE {
	return &IEEE{Config: cfg}
}

func (c *IEEE) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "ieee",
		Enabled: strings.TrimSpace(c.Config.IEEEAPIKey) != "",
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityUnsupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *IEEE) Search(sources.SearchRequest) (sources.SearchResult, error) {
	return sources.SearchResult{}, fmt.Errorf("source %q search is not implemented yet", "ieee")
}

func (c *IEEE) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return gatedSkeletonDownload("ieee")
}

func (c *IEEE) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return gatedSkeletonRead("ieee")
}

type ACM struct {
	Config config.Config
}

func NewACM(cfg config.Config) *ACM {
	return &ACM{Config: cfg}
}

func (c *ACM) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "acm",
		Enabled: strings.TrimSpace(c.Config.ACMAPIKey) != "",
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityUnsupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *ACM) Search(sources.SearchRequest) (sources.SearchResult, error) {
	return sources.SearchResult{}, fmt.Errorf("source %q search is not implemented yet", "acm")
}

func (c *ACM) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return gatedSkeletonDownload("acm")
}

func (c *ACM) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return gatedSkeletonRead("acm")
}

type openaireXMLElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr           `xml:",any,attr"`
	Content  string               `xml:",chardata"`
	Children []openaireXMLElement `xml:",any"`
}

type openaireRelData struct {
	Authors      []string
	PIDs         []string
	URLs         []string
	Descriptions []string
	Titles       []string
	Dates        []string
	Score        int
}

func crossrefDate(dateParts [][]int) string {
	if len(dateParts) == 0 {
		return ""
	}
	parts := dateParts[0]
	if len(parts) == 0 {
		return ""
	}
	switch len(parts) {
	case 1:
		return parseDate(strconv.Itoa(parts[0]))
	case 2:
		return parseDate(fmt.Sprintf("%04d-%02d", parts[0], parts[1]))
	default:
		return parseDate(fmt.Sprintf("%04d-%02d-%02d", parts[0], parts[1], parts[2]))
	}
}

func extractOpenAlexAuthors(value any) []string {
	results := make([]string, 0)
	for _, item := range toMapSlice(value) {
		author, _ := item["author"].(map[string]any)
		if name := toString(author["display_name"]); name != "" {
			results = append(results, name)
		}
	}
	return results
}

func renderInvertedIndex(value any) string {
	index, ok := value.(map[string]any)
	if !ok || len(index) == 0 {
		return ""
	}
	maxPos := -1
	terms := map[int]string{}
	for word, rawPositions := range index {
		for _, pos := range intSlice(rawPositions) {
			if pos > maxPos {
				maxPos = pos
			}
			terms[pos] = word
		}
	}
	if maxPos < 0 {
		return ""
	}
	words := make([]string, 0, maxPos+1)
	for i := 0; i <= maxPos; i++ {
		if word, ok := terms[i]; ok {
			words = append(words, word)
		}
	}
	return strings.Join(words, " ")
}

func intSlice(value any) []int {
	switch typed := value.(type) {
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			switch n := item.(type) {
			case float64:
				out = append(out, int(n))
			case int:
				out = append(out, n)
			}
		}
		return out
	default:
		return nil
	}
}

func idValue(value any, key string) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return toString(m[key])
}

func lastPathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimRight(value, "/")
	if idx := strings.LastIndex(value, "/"); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseGoogleScholarResults(baseURL, body string) []paper.Paper {
	cardPattern := regexp.MustCompile(`(?s)<div class="gs_r gs_or gs_scl".*?</div>\s*</div>`)
	titlePattern := regexp.MustCompile(`(?s)<h3 class="gs_rt">\s*<a href="([^"]+)">\s*(.*?)\s*</a>`)
	pdfPattern := regexp.MustCompile(`(?s)<div class="gs_or_ggsm">\s*<a href="([^"]+)">`)
	metaPattern := regexp.MustCompile(`(?s)<div class="gs_a">\s*(.*?)\s*</div>`)
	abstractPattern := regexp.MustCompile(`(?s)<div class="gs_rs">\s*(.*?)\s*</div>`)

	matches := cardPattern.FindAllString(body, -1)
	items := make([]paper.Paper, 0, len(matches))
	for _, match := range matches {
		titleMatch := titlePattern.FindStringSubmatch(match)
		if len(titleMatch) == 0 {
			continue
		}
		metaMatch := metaPattern.FindStringSubmatch(match)
		abstractMatch := abstractPattern.FindStringSubmatch(match)
		pdfMatch := pdfPattern.FindStringSubmatch(match)
		authors, publishedDate := scholarAuthorsAndDate(stripHTML(groupValue(metaMatch, 1)))
		abstract := stripHTML(groupValue(abstractMatch, 1))
		items = append(items, paper.Paper{
			PaperID:       groupValue(titleMatch, 1),
			Title:         stripHTML(groupValue(titleMatch, 2)),
			Authors:       authors,
			Abstract:      abstract,
			DOI:           extractDOI(abstract),
			PublishedDate: publishedDate,
			PDFURL:        resolveRelativeURL(baseURL, groupValue(pdfMatch, 1)),
			URL:           resolveRelativeURL(baseURL, groupValue(titleMatch, 1)),
			Source:        "google-scholar",
		})
	}
	return items
}

func scholarAuthorsAndDate(value string) ([]string, string) {
	parts := strings.Split(value, " - ")
	authors := splitAuthors(strings.ReplaceAll(firstValue(parts), ",", ";"))
	publishedDate := ""
	for _, part := range parts[1:] {
		if year := regexp.MustCompile(`\b(19|20)\d{2}\b`).FindString(part); year != "" {
			publishedDate = parseDate(year)
			break
		}
	}
	return authors, publishedDate
}

func dblpURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "https://dblp.org/rec/" + strings.TrimLeft(value, "/")
}

func parseCiteSeerXResults(baseURL, body string) []paper.Paper {
	cardPattern := regexp.MustCompile(`(?s)<div class="result">.*?<a class="remove doc_details" href="([^"]*doi=([^"&]+)[^"]*)">\s*(.*?)\s*</a>.*?<div class="pubinfo">\s*(.*?)\s*</div>.*?<div class="snippet">\s*(.*?)\s*</div>.*?<a href="([^"]*type=pdf[^"]*)">`)
	matches := cardPattern.FindAllStringSubmatch(body, -1)
	items := make([]paper.Paper, 0, len(matches))
	for _, match := range matches {
		meta := stripHTML(groupValue(match, 4))
		abstract := stripHTML(groupValue(match, 5))
		pdfURL := html.UnescapeString(groupValue(match, 6))
		items = append(items, paper.Paper{
			PaperID:  strings.TrimSpace(groupValue(match, 2)),
			Title:    stripHTML(groupValue(match, 3)),
			Authors:  splitAuthors(strings.ReplaceAll(strings.Split(meta, " - ")[0], ",", ";")),
			Abstract: abstract,
			URL:      resolveRelativeURL(baseURL, groupValue(match, 1)),
			PDFURL:   resolveRelativeURL(baseURL, pdfURL),
			Source:   "citeseerx",
		})
	}
	return items
}

func parseSSRNResults(baseURL, body string) []paper.Paper {
	cardPattern := regexp.MustCompile(`(?s)<div class="search-result-content".*?</div>`)
	titlePattern := regexp.MustCompile(`(?s)<h2>\s*<a href="([^"]*abstract_id=(\d+)[^"]*)">\s*(.*?)\s*</a>`)
	authorsPattern := regexp.MustCompile(`(?s)<p class="authors">\s*(.*?)\s*</p>`)
	abstractPattern := regexp.MustCompile(`(?s)<div class="abstract-text">\s*(.*?)\s*</div>`)
	pdfPattern := regexp.MustCompile(`(?s)<a class="opt-link" href="([^"]+\.pdf[^"]*)">`)

	matches := cardPattern.FindAllString(body, -1)
	items := make([]paper.Paper, 0, len(matches))
	for _, match := range matches {
		titleMatch := titlePattern.FindStringSubmatch(match)
		if len(titleMatch) == 0 {
			continue
		}
		authors := splitAuthors(strings.ReplaceAll(stripHTML(groupValue(authorsPattern.FindStringSubmatch(match), 1)), ",", ";"))
		abstract := stripHTML(groupValue(abstractPattern.FindStringSubmatch(match), 1))
		items = append(items, paper.Paper{
			PaperID:  groupValue(titleMatch, 2),
			Title:    stripHTML(groupValue(titleMatch, 3)),
			Authors:  authors,
			Abstract: abstract,
			DOI:      extractDOI(abstract),
			URL:      resolveRelativeURL(baseURL, groupValue(titleMatch, 1)),
			PDFURL:   resolveRelativeURL(baseURL, groupValue(pdfPattern.FindStringSubmatch(match), 1)),
			Source:   "ssrn",
		})
	}
	return items
}

func resolveRelativeURL(baseURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	return joinURL(baseURL, href)
}

func groupValue(groups []string, index int) string {
	if len(groups) <= index {
		return ""
	}
	return groups[index]
}

func openaireTopLevelResults(root *openaireXMLElement) []*openaireXMLElement {
	results := openaireFindFirst(root, "results")
	if results == nil {
		return nil
	}
	items := make([]*openaireXMLElement, 0)
	for i := range results.Children {
		child := &results.Children[i]
		if openaireLocalName(child.XMLName) == "result" {
			items = append(items, child)
		}
	}
	return items
}

func openaireFindFirst(root *openaireXMLElement, localName string) *openaireXMLElement {
	if root == nil {
		return nil
	}
	if openaireLocalName(root.XMLName) == strings.ToLower(localName) {
		return root
	}
	for i := range root.Children {
		if found := openaireFindFirst(&root.Children[i], localName); found != nil {
			return found
		}
	}
	return nil
}

func openaireFirstChild(root *openaireXMLElement, localName string) *openaireXMLElement {
	if root == nil {
		return nil
	}
	for i := range root.Children {
		child := &root.Children[i]
		if openaireLocalName(child.XMLName) == strings.ToLower(localName) {
			return child
		}
	}
	return nil
}

func openaireDirectTexts(root *openaireXMLElement, localName string) []string {
	if root == nil {
		return nil
	}
	values := make([]string, 0)
	for i := range root.Children {
		child := &root.Children[i]
		if openaireLocalName(child.XMLName) != strings.ToLower(localName) {
			continue
		}
		if text := strings.TrimSpace(child.Content); text != "" {
			values = append(values, text)
		}
	}
	return values
}

func openaireLocalName(name xml.Name) string {
	return strings.ToLower(strings.TrimSpace(name.Local))
}

func parseOpenAIREXMLResult(node *openaireXMLElement) (paper.Paper, bool) {
	header := openaireFirstChild(node, "header")
	metadata := openaireFirstChild(node, "metadata")
	entity := openaireFirstChild(metadata, "entity")
	target := openaireFirstChild(entity, "result")
	if target == nil {
		target = node
	}

	titleCandidates := openaireDirectTexts(target, "title")
	mainTitles := make([]string, 0)
	for i := range target.Children {
		child := &target.Children[i]
		if openaireLocalName(child.XMLName) != "title" {
			continue
		}
		value := strings.TrimSpace(child.Content)
		if value == "" {
			continue
		}
		classID := strings.ToLower(openaireAttr(child, "classid"))
		className := strings.ToLower(openaireAttr(child, "classname"))
		if strings.Contains(classID, "main") || strings.Contains(className, "main") {
			mainTitles = append(mainTitles, value)
		}
	}

	title := ""
	if len(mainTitles) > 0 {
		title = mainTitles[0]
	} else if len(titleCandidates) > 0 {
		title = titleCandidates[0]
	}
	title = stripHTML(title)
	if title == "" {
		return paper.Paper{}, false
	}

	primaryRel := openaireRelData{}
	if rels := openaireFirstChild(target, "rels"); rels != nil {
		relationData := make([]openaireRelData, 0)
		for i := range rels.Children {
			child := &rels.Children[i]
			if openaireLocalName(child.XMLName) == "rel" {
				relationData = append(relationData, openaireExtractRelData(child))
			}
		}
		if len(relationData) > 0 {
			best := relationData[0]
			for _, item := range relationData[1:] {
				if item.Score > best.Score {
					best = item
				}
			}
			primaryRel = best
		}
	}

	authors := stringSlice(openaireDirectTexts(target, "creator"))
	if len(authors) == 0 {
		authors = primaryRel.Authors
	}

	descriptionCandidates := stringSlice(openaireDirectTexts(target, "description"))
	if len(descriptionCandidates) == 0 {
		descriptionCandidates = primaryRel.Descriptions
	}
	abstract := ""
	if len(descriptionCandidates) > 0 {
		abstract = descriptionCandidates[0]
	}

	pidCandidates := append(stringSlice(openaireDirectTexts(target, "pid")), primaryRel.PIDs...)
	doi := extractDOI(append(append([]string{}, pidCandidates...), abstract, title)...)

	dateCandidates := append(stringSlice(openaireDirectTexts(target, "publicationdate")), openaireDirectTexts(target, "dateofacceptance")...)
	dateCandidates = append(dateCandidates, primaryRel.Dates...)
	publishedDate := parseDate(dateCandidates...)

	urlCandidates := make([]string, 0)
	urlCandidates = append(urlCandidates, filterHTTPStrings(openaireDirectTexts(target, "url"))...)
	urlCandidates = append(urlCandidates, filterHTTPStrings(openaireDirectTexts(target, "webresource"))...)
	urlCandidates = append(urlCandidates, filterHTTPStrings(openaireDirectTexts(target, "codeRepositoryUrl"))...)
	urlCandidates = append(urlCandidates, filterHTTPStrings(primaryRel.URLs)...)
	url := firstValue(urlCandidates)
	if url == "" && doi != "" {
		url = "https://doi.org/" + doi
	}

	pdfURL := ""
	for _, candidate := range urlCandidates {
		lowered := strings.ToLower(candidate)
		if strings.HasSuffix(lowered, ".pdf") || strings.Contains(lowered, "/pdf") {
			pdfURL = candidate
			break
		}
	}

	paperID := firstValue(openaireDirectTexts(header, "objIdentifier"))
	if paperID == "" {
		paperID = firstNonEmpty(doi, title)
	}

	return paper.Paper{
		PaperID:       paperID,
		Title:         title,
		Authors:       authors,
		Abstract:      stripHTML(abstract),
		DOI:           doi,
		PublishedDate: publishedDate,
		PDFURL:        pdfURL,
		URL:           url,
		Source:        "openaire",
	}, true
}

func parseOpenAIRELegacyResult(item map[string]any) (paper.Paper, bool) {
	if title := toString(item["title"]); title != "" {
		return paper.Paper{
			PaperID:       toString(item["id"]),
			Title:         title,
			Authors:       stringSlice(item["authors"]),
			Abstract:      toString(item["description"]),
			DOI:           extractDOI(toString(item["doi"])),
			PublishedDate: parseDate(toString(item["dateofacceptance"])),
			URL:           toString(item["url"]),
			Source:        "openaire",
		}.Normalized(), true
	}

	header, _ := item["header"].(map[string]any)
	metadata := nestedMap(item, "metadata", "oaf:entity", "oaf:result")
	titleCandidates := openaireJSONStringList(metadata["title"])
	title := ""
	for _, candidate := range titleCandidates {
		if candidate != "" {
			title = candidate
			break
		}
	}
	if title == "" {
		return paper.Paper{}, false
	}

	authors := openaireJSONStringList(metadata["creator"])
	abstract := firstValue(openaireJSONStringList(metadata["description"]))
	pids := openaireJSONStringList(metadata["pid"])
	doi := extractDOI(append(append([]string{}, pids...), abstract, title)...)
	urls := append(openaireJSONStringList(metadata["url"]), openaireJSONStringList(metadata["webresource"])...)
	paperID := openaireJSONText(header["dri:objIdentifier"])
	if paperID == "" {
		paperID = firstNonEmpty(doi, title)
	}

	return paper.Paper{
		PaperID:       paperID,
		Title:         title,
		Authors:       authors,
		Abstract:      abstract,
		DOI:           doi,
		PublishedDate: parseDate(openaireJSONText(metadata["publicationdate"]), openaireJSONText(metadata["dateofacceptance"])),
		PDFURL:        firstPDFURL(urls),
		URL:           firstValue(filterHTTPStrings(urls)),
		Source:        "openaire",
	}.Normalized(), true
}

func openaireAttr(node *openaireXMLElement, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attrs {
		if strings.EqualFold(attr.Name.Local, name) {
			return strings.TrimSpace(attr.Value)
		}
	}
	return ""
}

func openaireExtractRelData(node *openaireXMLElement) openaireRelData {
	data := openaireRelData{
		Authors:      []string{},
		PIDs:         []string{},
		URLs:         []string{},
		Descriptions: []string{},
		Titles:       []string{},
		Dates:        []string{},
	}
	var walk func(*openaireXMLElement, bool)
	walk = func(current *openaireXMLElement, underChildren bool) {
		if current == nil {
			return
		}
		tag := openaireLocalName(current.XMLName)
		nextUnderChildren := underChildren || tag == "children"
		if !nextUnderChildren {
			text := strings.TrimSpace(current.Content)
			if text != "" {
				switch tag {
				case "creator":
					data.Authors = appendUnique(data.Authors, text)
				case "pid", "identifier":
					data.PIDs = appendUnique(data.PIDs, text)
				case "url", "webresource":
					if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
						data.URLs = appendUnique(data.URLs, text)
					}
				case "description":
					data.Descriptions = appendUnique(data.Descriptions, text)
				case "title":
					data.Titles = appendUnique(data.Titles, text)
				case "dateofacceptance", "publicationdate":
					data.Dates = appendUnique(data.Dates, text)
				}
			}
		}
		for i := range current.Children {
			walk(&current.Children[i], nextUnderChildren)
		}
	}
	walk(node, false)
	data.Score = len(data.PIDs)*3 + len(data.URLs)*2 + len(data.Authors)
	return data
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func filterHTTPStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	current := root
	for _, key := range keys {
		next, _ := current[key].(map[string]any)
		if len(next) == 0 {
			return nil
		}
		current = next
	}
	return current
}

func openaireJSONText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return firstNonEmpty(
			toString(typed["$"]),
			toString(typed["value"]),
			toString(typed["#text"]),
		)
	case []any:
		for _, item := range typed {
			if text := openaireJSONText(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func openaireJSONStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := openaireJSONText(item); text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		if text := openaireJSONText(typed); text != "" {
			return []string{text}
		}
		return []string{}
	}
}
