package connectors

import (
	"context"
	"encoding/json"
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

type summaryItem struct {
	Name  string        `xml:"Name,attr"`
	Type  string        `xml:"Type,attr"`
	Value string        `xml:",chardata"`
	Items []summaryItem `xml:"Item"`
}

type xmlStringField struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

type xmlArrayField struct {
	Name   string           `xml:"name,attr"`
	Values []xmlStringField `xml:"str"`
}

type Arxiv struct {
	BaseURL string
	Client  *http.Client
}

func NewArxiv() *Arxiv {
	return &Arxiv{
		BaseURL: "http://export.arxiv.org/api/query",
		Client:  defaultHTTPClient(),
	}
}

func (c *Arxiv) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "arxiv",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *Arxiv) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	params := url.Values{}
	params.Set("search_query", "all:"+strings.TrimSpace(request.Query))
	params.Set("max_results", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	endpoint := c.BaseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}

	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type link struct {
		Href  string `xml:"href,attr"`
		Type  string `xml:"type,attr"`
		Title string `xml:"title,attr"`
	}
	type author struct {
		Name string `xml:"name"`
	}
	type tag struct {
		Term string `xml:"term,attr"`
	}
	type entry struct {
		ID        string   `xml:"id"`
		Title     string   `xml:"title"`
		Summary   string   `xml:"summary"`
		Published string   `xml:"published"`
		Updated   string   `xml:"updated"`
		DOI       string   `xml:"http://arxiv.org/schemas/atom doi"`
		Authors   []author `xml:"author"`
		Links     []link   `xml:"link"`
		Tags      []tag    `xml:"category"`
	}
	type feed struct {
		Entries []entry `xml:"entry"`
	}

	var parsed feed
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(parsed.Entries))
	for _, item := range parsed.Entries {
		authors := make([]string, 0, len(item.Authors))
		for _, author := range item.Authors {
			authors = append(authors, author.Name)
		}
		pdfURL := ""
		landingURL := strings.TrimSpace(item.ID)
		doi := extractDOI(item.DOI, item.Summary, item.ID)
		for _, link := range item.Links {
			if strings.EqualFold(link.Type, "application/pdf") || strings.EqualFold(link.Title, "pdf") {
				pdfURL = strings.TrimSpace(link.Href)
			}
			if strings.EqualFold(link.Title, "doi") {
				doi = extractDOI(doi, link.Href)
			}
			if landingURL == "" && strings.Contains(strings.ToLower(link.Type), "html") {
				landingURL = strings.TrimSpace(link.Href)
			}
		}
		paperID := landingURL
		if idx := strings.LastIndex(landingURL, "/"); idx >= 0 {
			paperID = landingURL[idx+1:]
		}
		items = append(items, paper.Paper{
			PaperID:       paperID,
			Title:         item.Title,
			Authors:       authors,
			Abstract:      item.Summary,
			DOI:           doi,
			PublishedDate: parseDate(item.Published),
			PDFURL:        pdfURL,
			URL:           landingURL,
			Source:        "arxiv",
		})
	}

	return searchResult(items, request.Limit), nil
}

func (c *Arxiv) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("arxiv", request)
}

func (c *Arxiv) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("arxiv", request)
}

func (c *Arxiv) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type BioRxiv struct {
	BaseURL string
	Client  *http.Client
}

func NewBioRxiv() *BioRxiv {
	return &BioRxiv{
		BaseURL: "https://www.biorxiv.org/search",
		Client:  defaultHTTPClient(),
	}
}

func (c *BioRxiv) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "biorxiv",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *BioRxiv) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	return c.searchPreprint("biorxiv", "https://www.biorxiv.org/content/", request)
}

func (c *BioRxiv) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("biorxiv", request)
}

func (c *BioRxiv) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("biorxiv", request)
}

type MedRxiv struct {
	BaseURL string
	Client  *http.Client
}

func NewMedRxiv() *MedRxiv {
	return &MedRxiv{
		BaseURL: "https://www.medrxiv.org/search",
		Client:  defaultHTTPClient(),
	}
}

func (c *MedRxiv) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "medrxiv",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *MedRxiv) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	return c.searchPreprint("medrxiv", "https://www.medrxiv.org/content/", request)
}

func (c *MedRxiv) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("medrxiv", request)
}

func (c *MedRxiv) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("medrxiv", request)
}

func (c *BioRxiv) searchPreprint(sourceID, landingBase string, request sources.SearchRequest) (sources.SearchResult, error) {
	return searchPreprint(c.client(), c.BaseURL, sourceID, landingBase, request)
}

func (c *MedRxiv) searchPreprint(sourceID, landingBase string, request sources.SearchRequest) (sources.SearchResult, error) {
	return searchPreprint(c.client(), c.BaseURL, sourceID, landingBase, request)
}

func searchPreprint(client *http.Client, baseURL, sourceID, landingBase string, request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	endpoint, err := preprintSearchURL(baseURL, sourceID, request.Query, request.Limit)
	if err != nil {
		return sources.SearchResult{}, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}

	body, err := executeBytes(client, req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	items := parsePreprintSearchResults(body, baseURL, landingBase, sourceID)
	return searchResult(items, request.Limit), nil
}

func (c *BioRxiv) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

func (c *MedRxiv) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type PubMed struct {
	SearchURL string
	FetchURL  string
	Client    *http.Client
}

func NewPubMed() *PubMed {
	return &PubMed{
		SearchURL: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi",
		FetchURL:  "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi",
		Client:    defaultHTTPClient(),
	}
}

func (c *PubMed) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "pubmed",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityInformational,
			Read:     sources.CapabilityInformational,
		},
	}
}

func (c *PubMed) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	searchReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.SearchURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	searchValues := searchReq.URL.Query()
	searchValues.Set("db", "pubmed")
	searchValues.Set("term", strings.TrimSpace(request.Query))
	searchValues.Set("retmax", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	searchValues.Set("retmode", "xml")
	searchReq.URL.RawQuery = searchValues.Encode()

	body, err := executeBytes(c.client(), searchReq)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type searchResponse struct {
		IDs []string `xml:"IdList>Id"`
	}
	var ids searchResponse
	if err := xml.Unmarshal(body, &ids); err != nil {
		return sources.SearchResult{}, err
	}
	if len(ids.IDs) == 0 {
		return sources.SearchResult{}, nil
	}

	fetchReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.FetchURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	fetchValues := fetchReq.URL.Query()
	fetchValues.Set("db", "pubmed")
	fetchValues.Set("id", strings.Join(ids.IDs, ","))
	fetchValues.Set("retmode", "xml")
	fetchReq.URL.RawQuery = fetchValues.Encode()

	fetchBody, err := executeBytes(c.client(), fetchReq)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type article struct {
		PMID     string `xml:"MedlineCitation>PMID"`
		Title    string `xml:"MedlineCitation>Article>ArticleTitle"`
		Abstract []struct {
			Text string `xml:",innerxml"`
		} `xml:"MedlineCitation>Article>Abstract>AbstractText"`
		Authors []struct {
			LastName string `xml:"LastName"`
			Initials string `xml:"Initials"`
		} `xml:"MedlineCitation>Article>AuthorList>Author"`
		Year string `xml:"MedlineCitation>Article>Journal>JournalIssue>PubDate>Year"`
		DOIs []struct {
			Type string `xml:"IdType,attr"`
			Text string `xml:",chardata"`
		} `xml:"PubmedData>ArticleIdList>ArticleId"`
	}
	type articleSet struct {
		Articles []article `xml:"PubmedArticle"`
	}

	var parsed articleSet
	if err := xml.Unmarshal(fetchBody, &parsed); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(parsed.Articles))
	for _, item := range parsed.Articles {
		authors := make([]string, 0, len(item.Authors))
		for _, author := range item.Authors {
			name := strings.TrimSpace(author.LastName)
			if initials := strings.TrimSpace(author.Initials); initials != "" {
				name = strings.TrimSpace(name + " " + initials)
			}
			if name != "" {
				authors = append(authors, name)
			}
		}
		abstractParts := make([]string, 0, len(item.Abstract))
		for _, abstract := range item.Abstract {
			text := stripHTML(abstract.Text)
			if text != "" {
				abstractParts = append(abstractParts, text)
			}
		}
		doi := ""
		for _, candidate := range item.DOIs {
			if strings.EqualFold(candidate.Type, "doi") {
				doi = extractDOI(candidate.Text)
				break
			}
		}
		items = append(items, paper.Paper{
			PaperID:       item.PMID,
			Title:         item.Title,
			Authors:       authors,
			Abstract:      strings.Join(abstractParts, " "),
			DOI:           doi,
			PublishedDate: parseDate(item.Year),
			URL:           "https://pubmed.ncbi.nlm.nih.gov/" + strings.TrimSpace(item.PMID) + "/",
			Source:        "pubmed",
		})
	}

	return searchResult(items, request.Limit), nil
}

func (c *PubMed) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return informationalDownload("pubmed")
}

func (c *PubMed) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return informationalRead("pubmed")
}

func (c *PubMed) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type IACR struct {
	BaseURL string
	Client  *http.Client
}

func NewIACR() *IACR {
	return &IACR{
		BaseURL: "https://eprint.iacr.org/search",
		Client:  defaultHTTPClient(),
	}
}

func (c *IACR) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "iacr",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *IACR) Search(request sources.SearchRequest) (sources.SearchResult, error) {
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

	pattern := regexp.MustCompile(`(?s)<div class="mb-4">.*?<a href="([^"]+)">\s*(.*?)\s*</a>.*?<div class="authors">\s*(.*?)\s*</div>.*?<p class="mt-2">\s*(.*?)\s*</p>`)
	matches := pattern.FindAllSubmatch(body, -1)
	items := make([]paper.Paper, 0, len(matches))
	for _, match := range matches {
		href := strings.TrimSpace(string(match[1]))
		title := stripHTML(string(match[2]))
		authors := strings.Split(stripHTML(string(match[3])), ",")
		for i := range authors {
			authors[i] = strings.TrimSpace(authors[i])
		}
		id := strings.TrimPrefix(href, "/")
		items = append(items, paper.Paper{
			PaperID:  id,
			Title:    title,
			Authors:  authors,
			Abstract: stripHTML(string(match[4])),
			URL:      joinURL(c.BaseURL, href),
			PDFURL:   joinURL(c.BaseURL, href+".pdf"),
			Source:   "iacr",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *IACR) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("iacr", request)
}

func (c *IACR) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("iacr", request)
}

func (c *IACR) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type PMC struct {
	SearchURL  string
	SummaryURL string
	Client     *http.Client
}

func NewPMC() *PMC {
	return &PMC{
		SearchURL:  "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi",
		SummaryURL: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi",
		Client:     defaultHTTPClient(),
	}
}

func (c *PMC) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "pmc",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *PMC) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	searchReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.SearchURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	searchValues := searchReq.URL.Query()
	searchValues.Set("db", "pmc")
	searchValues.Set("term", strings.TrimSpace(request.Query))
	searchValues.Set("retmax", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	searchReq.URL.RawQuery = searchValues.Encode()
	searchBody, err := executeBytes(c.client(), searchReq)
	if err != nil {
		return sources.SearchResult{}, err
	}
	type idList struct {
		IDs []string `xml:"IdList>Id"`
	}
	var ids idList
	if err := xml.Unmarshal(searchBody, &ids); err != nil {
		return sources.SearchResult{}, err
	}
	if len(ids.IDs) == 0 {
		return sources.SearchResult{}, nil
	}

	summaryReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.SummaryURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	summaryValues := summaryReq.URL.Query()
	summaryValues.Set("db", "pmc")
	summaryValues.Set("id", strings.Join(ids.IDs, ","))
	summaryReq.URL.RawQuery = summaryValues.Encode()
	summaryBody, err := executeBytes(c.client(), summaryReq)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type docSum struct {
		ID    string        `xml:"Id"`
		Items []summaryItem `xml:"Item"`
	}
	type summary struct {
		DocSums []docSum `xml:"DocSum"`
	}
	var parsed summary
	if err := xml.Unmarshal(summaryBody, &parsed); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(parsed.DocSums))
	for _, doc := range parsed.DocSums {
		title := itemValue(doc.Items, "Title")
		pubDate := itemValue(doc.Items, "PubDate")
		doi := extractDOI(itemValue(doc.Items, "DOI"))
		authors := listItemValues(doc.Items, "AuthorList")
		pmcID := "PMC" + strings.TrimPrefix(strings.TrimSpace(doc.ID), "PMC")
		items = append(items, paper.Paper{
			PaperID:       pmcID,
			Title:         title,
			Authors:       authors,
			DOI:           doi,
			PublishedDate: parseDate(pubDate),
			URL:           "https://pmc.ncbi.nlm.nih.gov/articles/" + pmcID + "/",
			PDFURL:        "https://pmc.ncbi.nlm.nih.gov/articles/" + pmcID + "/pdf/",
			Source:        "pmc",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *PMC) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("pmc", request)
}

func (c *PMC) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("pmc", request)
}

func (c *PMC) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

func itemValue(items []summaryItem, name string) string {
	for _, item := range items {
		if item.Name == name {
			return strings.TrimSpace(item.Value)
		}
	}
	return ""
}

func listItemValues(items []summaryItem, name string) []string {
	for _, item := range items {
		if item.Name != name {
			continue
		}
		values := make([]string, 0, len(item.Items))
		for _, nested := range item.Items {
			if text := strings.TrimSpace(nested.Value); text != "" {
				values = append(values, text)
			}
		}
		return values
	}
	return nil
}

type EuropePMC struct {
	BaseURL string
	Client  *http.Client
}

func NewEuropePMC() *EuropePMC {
	return &EuropePMC{
		BaseURL: "https://www.ebi.ac.uk/europepmc/webservices/rest/search",
		Client:  defaultHTTPClient(),
	}
}

func (c *EuropePMC) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "europepmc",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *EuropePMC) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("query", strings.TrimSpace(request.Query))
	values.Set("format", "json")
	values.Set("resultType", "core")
	values.Set("pageSize", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	req.URL.RawQuery = values.Encode()

	var payload struct {
		ResultList struct {
			Result []map[string]any `json:"result"`
		} `json:"resultList"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]paper.Paper, 0, len(payload.ResultList.Result))
	for _, item := range payload.ResultList.Result {
		pmcid := toString(item["pmcid"])
		paperID := pmcid
		if paperID == "" {
			paperID = toString(item["id"])
			if strings.EqualFold(toString(item["source"]), "MED") {
				paperID = "PMID:" + paperID
			}
		}
		authors := extractEuropeAuthors(item["authorList"])
		urls := extractFullTextURLs(item["fullTextUrlList"])
		landingURL, pdfURL := "", ""
		for _, candidate := range urls {
			style := strings.ToLower(candidate.style)
			if style == "html" && landingURL == "" {
				landingURL = candidate.url
			}
			if style == "pdf" && pdfURL == "" {
				pdfURL = candidate.url
			}
		}
		if landingURL == "" && pmcid != "" {
			landingURL = "https://pmc.ncbi.nlm.nih.gov/articles/" + pmcid + "/"
		}
		items = append(items, paper.Paper{
			PaperID:       paperID,
			Title:         toString(item["title"]),
			Authors:       authors,
			Abstract:      toString(item["abstractText"]),
			DOI:           extractDOI(toString(item["doi"]), toString(item["doiId"])),
			PublishedDate: parseDate(fmt.Sprintf("%s-%s-%s", toString(item["pubYear"]), zeroPad(toString(item["pubMonth"])), zeroPad(toString(item["pubDay"]))), toString(item["pubYear"])),
			URL:           landingURL,
			PDFURL:        pdfURL,
			Source:        "europepmc",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *EuropePMC) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("europepmc", request)
}

func (c *EuropePMC) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("europepmc", request)
}

func (c *EuropePMC) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type CORE struct {
	BaseURL string
	Client  *http.Client
	Config  config.Config
}

func NewCORE(cfg config.Config) *CORE {
	return &CORE{
		BaseURL: "https://api.core.ac.uk/v3/search/works",
		Client:  defaultHTTPClient(),
		Config:  cfg,
	}
}

func (c *CORE) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "core",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilitySupported,
		},
	}
}

func (c *CORE) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}

	makeRequest := func(withAuth bool) (*http.Request, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
		if err != nil {
			return nil, err
		}
		values := req.URL.Query()
		values.Set("q", strings.TrimSpace(request.Query))
		values.Set("limit", strconv.Itoa(limitOrDefault(request.Limit, 10)))
		req.URL.RawQuery = values.Encode()
		req.Header.Set("Accept", "application/json")
		if withAuth && strings.TrimSpace(c.Config.CoreAPIKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Config.CoreAPIKey))
		}
		return req, nil
	}

	exec := func(req *http.Request) (*http.Response, error) {
		return c.client().Do(req)
	}

	req, err := makeRequest(strings.TrimSpace(c.Config.CoreAPIKey) != "")
	if err != nil {
		return sources.SearchResult{}, err
	}
	response, err := exec(req)
	if err != nil {
		return sources.SearchResult{}, err
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized {
		response.Body.Close()
		req, err = makeRequest(false)
		if err != nil {
			return sources.SearchResult{}, err
		}
		response, err = exec(req)
		if err != nil {
			return sources.SearchResult{}, err
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sources.SearchResult{}, fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return sources.SearchResult{}, err
	}
	items := make([]paper.Paper, 0, len(payload.Results))
	for _, item := range payload.Results {
		doi := extractDOI(toString(item["doi"]), toString(item["abstract"]))
		pdfURL := ""
		if downloadURL := toString(item["downloadUrl"]); strings.HasSuffix(strings.ToLower(downloadURL), ".pdf") {
			pdfURL = downloadURL
		}
		if pdfURL == "" {
			pdfURL = firstPDFURL(stringSlice(item["fullTextUrls"]))
		}
		authors := extractNamedAuthors(item["authors"])
		landingURL := toString(item["url"])
		if landingURL == "" && doi != "" {
			landingURL = "https://doi.org/" + doi
		}
		items = append(items, paper.Paper{
			PaperID:       toString(item["id"]),
			Title:         toString(item["title"]),
			Authors:       authors,
			Abstract:      toString(item["abstract"]),
			DOI:           doi,
			PublishedDate: parseDate(toString(item["publishedDate"])),
			URL:           landingURL,
			PDFURL:        pdfURL,
			Source:        "core",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *CORE) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("core", request)
}

func (c *CORE) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("core", request)
}

func (c *CORE) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type DOAJ struct {
	BaseURL string
	Client  *http.Client
}

func NewDOAJ() *DOAJ {
	return &DOAJ{
		BaseURL: "https://doaj.org/api/search/articles",
		Client:  defaultHTTPClient(),
	}
}

func (c *DOAJ) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "doaj",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *DOAJ) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + url.PathEscape(strings.TrimSpace(request.Query))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	var payload struct {
		Results []struct {
			ID      string         `json:"id"`
			BibJSON map[string]any `json:"bibjson"`
		} `json:"results"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}
	items := make([]paper.Paper, 0, len(payload.Results))
	for _, item := range payload.Results {
		links := extractDOAJLinks(item.BibJSON["link"])
		items = append(items, paper.Paper{
			PaperID:       item.ID,
			Title:         toString(item.BibJSON["title"]),
			Authors:       extractNamedAuthors(item.BibJSON["author"]),
			Abstract:      toString(item.BibJSON["abstract"]),
			DOI:           extractDOIIdentifiers(item.BibJSON["identifier"]),
			PublishedDate: parseDate(fmt.Sprintf("%s-%s-%s", toString(item.BibJSON["year"]), zeroPad(toString(item.BibJSON["month"])), zeroPad(toString(item.BibJSON["day"]))), toString(item.BibJSON["year"])),
			URL:           links.article,
			PDFURL:        links.fulltext,
			Source:        "doaj",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *DOAJ) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("doaj", request)
}

func (c *DOAJ) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("doaj", request)
}

func (c *DOAJ) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type BASE struct {
	BaseURL string
	Client  *http.Client
}

func NewBASE() *BASE {
	return &BASE{
		BaseURL: "https://api.base-search.net/cgi-bin/BaseHttpSearchInterface.fcgi",
		Client:  defaultHTTPClient(),
	}
}

func (c *BASE) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "base",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *BASE) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("query", strings.TrimSpace(request.Query))
	req.URL.RawQuery = values.Encode()
	body, err := executeBytes(c.client(), req)
	if err != nil {
		return sources.SearchResult{}, err
	}

	type doc struct {
		Strings []xmlStringField `xml:"str"`
		Arrays  []xmlArrayField  `xml:"arr"`
	}
	type response struct {
		Docs []doc `xml:"results>doc"`
	}
	var parsed response
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return sources.SearchResult{}, err
	}
	items := make([]paper.Paper, 0, len(parsed.Docs))
	for _, doc := range parsed.Docs {
		identifiers := arrayValues(doc.Arrays, "identifier")
		landingURL, pdfURL := "", ""
		for _, identifier := range identifiers {
			lower := strings.ToLower(identifier)
			switch {
			case pdfURL == "" && strings.HasSuffix(lower, ".pdf"):
				pdfURL = identifier
			case landingURL == "" && strings.HasPrefix(lower, "http"):
				landingURL = identifier
			}
		}
		items = append(items, paper.Paper{
			PaperID:       fieldValue(doc.Strings, "id"),
			Title:         fieldValue(doc.Strings, "title"),
			Authors:       arrayValues(doc.Arrays, "creator"),
			Abstract:      fieldValue(doc.Strings, "description"),
			DOI:           extractDOI(strings.Join(identifiers, " ")),
			PublishedDate: parseDate(fieldValue(doc.Strings, "date")),
			URL:           landingURL,
			PDFURL:        pdfURL,
			Source:        "base",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *BASE) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("base", request)
}

func (c *BASE) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("base", request)
}

func (c *BASE) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type Zenodo struct {
	BaseURL string
	Client  *http.Client
}

func NewZenodo() *Zenodo {
	return &Zenodo{
		BaseURL: "https://zenodo.org/api/records",
		Client:  defaultHTTPClient(),
	}
}

func (c *Zenodo) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "zenodo",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *Zenodo) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("q", strings.TrimSpace(request.Query))
	values.Set("size", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	req.URL.RawQuery = values.Encode()
	var payload struct {
		Hits struct {
			Hits []map[string]any `json:"hits"`
		} `json:"hits"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}
	items := make([]paper.Paper, 0, len(payload.Hits.Hits))
	for _, item := range payload.Hits.Hits {
		metadata, _ := item["metadata"].(map[string]any)
		links, _ := item["links"].(map[string]any)
		files, _ := item["files"].([]any)
		pdfURL := ""
		for _, raw := range files {
			fileMap, _ := raw.(map[string]any)
			key := strings.ToLower(toString(fileMap["key"]))
			if !strings.HasSuffix(key, ".pdf") {
				continue
			}
			if linkMap, ok := fileMap["links"].(map[string]any); ok {
				pdfURL = toString(linkMap["self"])
				break
			}
		}
		items = append(items, paper.Paper{
			PaperID:       toString(item["id"]),
			Title:         toString(metadata["title"]),
			Authors:       extractNamedAuthors(metadata["creators"]),
			Abstract:      stripHTML(toString(metadata["description"])),
			DOI:           extractDOI(toString(metadata["doi"])),
			PublishedDate: parseDate(toString(metadata["publication_date"])),
			URL:           toString(links["html"]),
			PDFURL:        pdfURL,
			Source:        "zenodo",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *Zenodo) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("zenodo", request)
}

func (c *Zenodo) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("zenodo", request)
}

func (c *Zenodo) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type HAL struct {
	BaseURL string
	Client  *http.Client
}

func NewHAL() *HAL {
	return &HAL{
		BaseURL: "https://api.archives-ouvertes.fr/search/",
		Client:  defaultHTTPClient(),
	}
}

func (c *HAL) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "hal",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilitySupported,
			Download: sources.CapabilityRecordDependent,
			Read:     sources.CapabilityRecordDependent,
		},
	}
}

func (c *HAL) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if err := requireQuery(request.Query); err != nil {
		return sources.SearchResult{}, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return sources.SearchResult{}, err
	}
	values := req.URL.Query()
	values.Set("q", strings.TrimSpace(request.Query))
	values.Set("rows", strconv.Itoa(limitOrDefault(request.Limit, 10)))
	values.Set("wt", "json")
	req.URL.RawQuery = values.Encode()
	var payload struct {
		Response struct {
			Docs []map[string]any `json:"docs"`
		} `json:"response"`
	}
	if err := executeJSON(c.client(), req, &payload); err != nil {
		return sources.SearchResult{}, err
	}
	items := make([]paper.Paper, 0, len(payload.Response.Docs))
	for _, doc := range payload.Response.Docs {
		items = append(items, paper.Paper{
			PaperID:       toString(doc["docid"]),
			Title:         firstValue(doc["title_s"]),
			Authors:       stringSlice(doc["authFullName_s"]),
			Abstract:      firstValue(doc["abstract_s"]),
			DOI:           extractDOI(toString(doc["doiId_id"])),
			PublishedDate: parseDate(toString(doc["producedDateY_i"])),
			URL:           toString(doc["uri_s"]),
			PDFURL:        toString(doc["fileMain_s"]),
			Source:        "hal",
		})
	}
	return searchResult(items, request.Limit), nil
}

func (c *HAL) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return nativeDownload("hal", request)
}

func (c *HAL) Read(request sources.ReadRequest) (sources.RetrievalResult, error) {
	return nativeRead("hal", request)
}

func (c *HAL) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return defaultHTTPClient()
}

type doajLinks struct {
	article  string
	fulltext string
}

func extractDOAJLinks(value any) doajLinks {
	var links doajLinks
	for _, raw := range toMapSlice(value) {
		linkType := strings.ToLower(toString(raw["type"]))
		linkURL := toString(raw["url"])
		switch linkType {
		case "fulltext":
			if links.fulltext == "" {
				links.fulltext = linkURL
			}
		case "article":
			if links.article == "" {
				links.article = linkURL
			}
		}
	}
	return links
}

func extractDOIIdentifiers(value any) string {
	for _, raw := range toMapSlice(value) {
		if strings.EqualFold(toString(raw["type"]), "doi") {
			return extractDOI(toString(raw["id"]))
		}
	}
	return ""
}

type fullTextURL struct {
	style string
	url   string
}

func extractFullTextURLs(value any) []fullTextURL {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	raw := m["fullTextUrl"]
	results := make([]fullTextURL, 0)
	for _, item := range toMapSlice(raw) {
		results = append(results, fullTextURL{
			style: toString(item["documentStyle"]),
			url:   toString(item["url"]),
		})
	}
	return results
}

func extractEuropeAuthors(value any) []string {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return extractNamedAuthors(m["author"])
}

func extractNamedAuthors(value any) []string {
	if value == nil {
		return nil
	}
	results := make([]string, 0)
	for _, item := range toMapSlice(value) {
		name := toString(item["name"])
		if name == "" {
			name = toString(item["fullName"])
		}
		if name != "" {
			results = append(results, name)
		}
	}
	if len(results) != 0 {
		return results
	}
	return stringSlice(value)
}

func toMapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{typed}
	default:
		return nil
	}
}

func firstValue(value any) string {
	values := stringSlice(value)
	if len(values) != 0 {
		return values[0]
	}
	return toString(value)
}

func zeroPad(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "01"
	}
	if len(value) == 1 {
		return "0" + value
	}
	return value
}

func fieldValue(fields []xmlStringField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func arrayValues(fields []xmlArrayField, name string) []string {
	for _, field := range fields {
		if field.Name != name {
			continue
		}
		values := make([]string, 0, len(field.Values))
		for _, value := range field.Values {
			text := strings.TrimSpace(value.Value)
			if text != "" {
				values = append(values, text)
			}
		}
		return values
	}
	return nil
}

func limitOrDefault(limit, fallback int) int {
	if limit > 0 {
		return limit
	}
	return fallback
}

var (
	preprintSearchResultPattern = regexp.MustCompile(`(?s)<li class="[^"]*\bsearch-result\b[^"]*".*?</li>`)
	preprintTitlePattern        = regexp.MustCompile(`(?s)<a href="([^"]+)" class="highwire-cite-linked-title"[^>]*>\s*<span class="highwire-cite-title">(.*?)</span>`)
	preprintAuthorPattern       = regexp.MustCompile(`(?s)<span class="highwire-citation-author(?:\s+first)?".*?<span class="nlm-given-names">(.*?)</span>\s*<span class="nlm-surname">(.*?)</span>`)
	preprintPISAPattern         = regexp.MustCompile(`data-pisa="([^"]+)"`)
	preprintAPATHPattern        = regexp.MustCompile(`data-apath="([^"]+)"`)
	preprintDatePattern         = regexp.MustCompile(`/(?:biorxiv|medrxiv)/(?:early|content)/(\d{4})/(\d{2})/(\d{2})/`)
	preprintVersionPattern      = regexp.MustCompile(`v(\d+)$`)
	preprintVersionSuffix       = regexp.MustCompile(`(?i)v\d+$`)
)

func preprintSearchURL(baseURL, sourceID, query string, limit int) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("missing %s search base url", sourceID)
	}

	escapedQuery := url.PathEscape(strings.TrimSpace(query) + " jcode:" + sourceID)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/"+escapedQuery, nil)
	if err != nil {
		return "", err
	}

	params := req.URL.Query()
	params.Set("sort", "relevance-rank")
	params.Set("format_result", "standard")
	params.Set("numresults", strconv.Itoa(limitOrDefault(limit, 10)))
	req.URL.RawQuery = params.Encode()
	return req.URL.String(), nil
}

func parsePreprintSearchResults(body []byte, baseURL, landingBase, sourceID string) []paper.Paper {
	matches := preprintSearchResultPattern.FindAllString(string(body), -1)
	items := make([]paper.Paper, 0, len(matches))
	for _, match := range matches {
		titleMatch := preprintTitlePattern.FindStringSubmatch(match)
		if len(titleMatch) == 0 {
			continue
		}

		href := resolveRelativeURL(baseURL, html.UnescapeString(groupValue(titleMatch, 1)))
		title := html.UnescapeString(stripHTML(groupValue(titleMatch, 2)))
		authors := extractPreprintAuthors(match)

		doi := trimPreprintVersion(extractDOI(match))
		if doi == "" {
			doi = trimPreprintVersion(extractDOI(href))
		}
		paperID := doi
		if paperID == "" {
			paperID = preprintIdentifier(match)
		}

		version := preprintVersion(match, href)
		if href == "" && paperID != "" {
			href = strings.TrimRight(landingBase, "/") + "/" + paperID
			if version != "" {
				href += "v" + version
			}
		}

		pdfURL := ""
		if href != "" {
			pdfURL = href + ".full.pdf"
		}

		items = append(items, paper.Paper{
			PaperID:       paperID,
			Title:         title,
			Authors:       authors,
			DOI:           doi,
			PublishedDate: preprintPublishedDate(match),
			PDFURL:        pdfURL,
			URL:           href,
			Source:        sourceID,
		})
	}
	return items
}

func extractPreprintAuthors(block string) []string {
	matches := preprintAuthorPattern.FindAllStringSubmatch(block, -1)
	authors := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.TrimSpace(spaceRegexp.ReplaceAllString(html.UnescapeString(groupValue(match, 1))+" "+html.UnescapeString(groupValue(match, 2)), " "))
		if name != "" {
			authors = append(authors, name)
		}
	}
	return authors
}

func preprintIdentifier(block string) string {
	if match := preprintPISAPattern.FindStringSubmatch(block); len(match) > 1 {
		parts := strings.Split(strings.TrimSpace(match[1]), ";")
		if len(parts) > 1 {
			return trimPreprintVersion(parts[1])
		}
	}
	return ""
}

func preprintPublishedDate(block string) string {
	match := preprintAPATHPattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	parts := preprintDatePattern.FindStringSubmatch(match[1])
	if len(parts) != 4 {
		return ""
	}
	return parseDate(parts[1] + "-" + parts[2] + "-" + parts[3])
}

func preprintVersion(values ...string) string {
	for _, value := range values {
		match := preprintVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
		if len(match) > 1 {
			return match[1]
		}
	}
	return "1"
}

func trimPreprintVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return preprintVersionSuffix.ReplaceAllString(value, "")
}
