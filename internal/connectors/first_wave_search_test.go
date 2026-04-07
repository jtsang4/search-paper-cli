package connectors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

func TestArxiv(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search_query"); got != "all:graph neural networks" {
			t.Fatalf("expected arxiv search_query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/1234.5678v1</id>
    <updated>2024-01-03T00:00:00Z</updated>
    <published>2024-01-02T00:00:00Z</published>
    <title>  Graph   Neural
Networks </title>
    <summary>  A   graph paper. </summary>
    <author><name> Alice   Smith </name></author>
    <author><name>Bob Jones</name></author>
    <link href="http://arxiv.org/abs/1234.5678v1" rel="alternate" type="text/html"></link>
    <link title="doi" href="https://doi.org/10.1000/ARXIV-123"></link>
    <link title="pdf" href="https://arxiv.org/pdf/1234.5678v1.pdf" rel="related" type="application/pdf"></link>
    <arxiv:doi>10.1000/ARXIV-123</arxiv:doi>
  </entry>
</feed>`))
	}))
	defer server.Close()

	connector := NewArxiv()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	var subject sources.Connector = connector
	result, err := subject.Search(sources.SearchRequest{Query: "graph neural networks", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "arxiv" || got.PaperID != "1234.5678v1" || got.DOI != "10.1000/arxiv-123" {
		t.Fatalf("unexpected arxiv paper %#v", got)
	}
	if got.Title != "Graph Neural Networks" || strings.Join(got.Authors, ",") != "Alice Smith,Bob Jones" {
		t.Fatalf("expected normalized arxiv fields, got %#v", got)
	}
	if got.PDFURL != "https://arxiv.org/pdf/1234.5678v1.pdf" {
		t.Fatalf("expected arxiv pdf url, got %#v", got)
	}
}

func TestBioRxiv(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.RequestURI; got != "/search/graph%20neural%20networks%20jcode:biorxiv?format_result=standard&numresults=1&sort=relevance-rank" {
			t.Fatalf("expected biorxiv query search request, got %q", got)
		}
		if got := r.URL.Query().Get("category"); got != "" {
			t.Fatalf("expected no biorxiv category param, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<div class="highwire-search-results highwire-article-citation-list no-show-abstract">
  <ul class="highwire-search-results-list">
    <li class="first odd search-result result-jcode-biorxiv search-result-highwire-citation">
      <div class="highwire-article-citation" data-pisa="biorxiv;2024.01.15.123456v2" data-pisa-master="biorxiv;2024.01.15.123456" data-apath="/biorxiv/early/2024/02/01/2024.01.15.123456.atom">
        <div class="highwire-cite highwire-cite-highwire-article highwire-citation-biorxiv-article-pap-list clearfix">
          <span class="highwire-cite-title">
            <a href="/content/10.1101/2024.01.15.123456v2" class="highwire-cite-linked-title">
              <span class="highwire-cite-title">  Older   Graph Neural   Networks Paper </span>
            </a>
          </span>
          <div class="highwire-cite-authors"><span class="highwire-citation-authors"><span class="highwire-citation-author first"><span class="nlm-given-names">Alice</span> <span class="nlm-surname">Smith</span></span>, <span class="highwire-citation-author"><span class="nlm-given-names">Bob</span> <span class="nlm-surname">Jones</span></span></span></div>
          <div class="highwire-cite-metadata"><span class="highwire-cite-metadata-journal highwire-cite-metadata">bioRxiv </span><span class="highwire-cite-metadata-pages highwire-cite-metadata">2024.01.15.123456; </span><span class="highwire-cite-metadata-doi highwire-cite-metadata"><span class="doi_label">doi:</span> https://doi.org/10.1101/2024.01.15.123456 </span></div>
        </div>
      </div>
    </li>
  </ul>
</div>`))
	}))
	defer server.Close()

	connector := NewBioRxiv()
	connector.BaseURL = server.URL + "/search"
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "graph neural networks", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "biorxiv" || got.PaperID != "10.1101/2024.01.15.123456" {
		t.Fatalf("unexpected biorxiv identity %#v", got)
	}
	if got.Title != "Older Graph Neural Networks Paper" || strings.Join(got.Authors, ",") != "Alice Smith,Bob Jones" {
		t.Fatalf("unexpected biorxiv normalization %#v", got)
	}
	if got.PublishedDate != "2024-02-01" {
		t.Fatalf("expected older biorxiv match date, got %#v", got)
	}
	if got.URL != server.URL+"/content/10.1101/2024.01.15.123456v2" {
		t.Fatalf("unexpected biorxiv landing url %#v", got)
	}
	if got.PDFURL != server.URL+"/content/10.1101/2024.01.15.123456v2.full.pdf" {
		t.Fatalf("unexpected biorxiv pdf url %#v", got)
	}
}

func TestMedRxiv(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.RequestURI; got != "/search/Alice%20Smith%20jcode:medrxiv?format_result=standard&numresults=1&sort=relevance-rank" {
			t.Fatalf("expected medrxiv query search request, got %q", got)
		}
		if got := r.URL.Query().Get("category"); got != "" {
			t.Fatalf("expected no medrxiv category param, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<div class="highwire-search-results highwire-article-citation-list no-show-abstract">
  <ul class="highwire-search-results-list">
    <li class="first odd search-result result-jcode-medrxiv search-result-highwire-citation">
      <div class="highwire-article-citation" data-pisa="medrxiv;2023.12.20.23299999v3" data-pisa-master="medrxiv;2023.12.20.23299999" data-apath="/medrxiv/early/2024/01/05/2023.12.20.23299999.atom">
        <div class="highwire-cite highwire-cite-highwire-article highwire-citation-biorxiv-article-pap-list clearfix">
          <span class="highwire-cite-title">
            <a href="/content/10.1101/2023.12.20.23299999v3" class="highwire-cite-linked-title">
              <span class="highwire-cite-title">Clinical Risk Forecasting with Graph Neural Networks</span>
            </a>
          </span>
          <div class="highwire-cite-authors"><span class="highwire-citation-authors"><span class="highwire-citation-author first"><span class="nlm-given-names">Alice</span> <span class="nlm-surname">Smith</span></span>, <span class="highwire-citation-author"><span class="nlm-given-names">Carol</span> <span class="nlm-surname">Ng</span></span></span></div>
          <div class="highwire-cite-metadata"><span class="highwire-cite-metadata-journal highwire-cite-metadata">medRxiv </span><span class="highwire-cite-metadata-pages highwire-cite-metadata">2023.12.20.23299999; </span><span class="highwire-cite-metadata-doi highwire-cite-metadata"><span class="doi_label">doi:</span> https://doi.org/10.1101/2023.12.20.23299999 </span></div>
        </div>
      </div>
    </li>
  </ul>
</div>`))
	}))
	defer server.Close()

	connector := NewMedRxiv()
	connector.BaseURL = server.URL + "/search"
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "Alice Smith", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "medrxiv" || got.PaperID != "10.1101/2023.12.20.23299999" {
		t.Fatalf("unexpected medrxiv paper %#v", got)
	}
	if got.Title != "Clinical Risk Forecasting with Graph Neural Networks" || strings.Join(got.Authors, ",") != "Alice Smith,Carol Ng" {
		t.Fatalf("unexpected medrxiv normalization %#v", got)
	}
	if got.PublishedDate != "2024-01-05" || got.URL != server.URL+"/content/10.1101/2023.12.20.23299999v3" {
		t.Fatalf("unexpected medrxiv metadata %#v", got)
	}
	if got.PDFURL != server.URL+"/content/10.1101/2023.12.20.23299999v3.full.pdf" {
		t.Fatalf("unexpected medrxiv paper %#v", got)
	}
}

func TestPubMed(t *testing.T) {
	t.Parallel()

	var searchCalled, fetchCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "esearch.fcgi"):
			searchCalled = true
			if got := r.URL.Query().Get("retmax"); got != "2" {
				t.Fatalf("expected pubmed retmax=2, got %q", got)
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<eSearchResult><IdList><Id>111111</Id><Id>222222</Id></IdList></eSearchResult>`))
		case strings.Contains(r.URL.Path, "efetch.fcgi"):
			fetchCalled = true
			if got := r.URL.Query().Get("id"); got != "111111,222222" {
				t.Fatalf("expected pubmed fetch ids, got %q", got)
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>111111</PMID>
      <Article>
        <ArticleTitle>  PubMed   Paper </ArticleTitle>
        <Abstract>
          <AbstractText>First abstract part.</AbstractText>
          <AbstractText>Second part.</AbstractText>
        </Abstract>
        <AuthorList>
          <Author><LastName>Smith</LastName><Initials>A</Initials></Author>
          <Author><LastName>Jones</LastName><Initials>B</Initials></Author>
        </AuthorList>
        <Journal><JournalIssue><PubDate><Year>2024</Year></PubDate></JournalIssue></Journal>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1000/PUBMED-1</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`))
		default:
			t.Fatalf("unexpected pubmed path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	connector := NewPubMed()
	connector.SearchURL = server.URL + "/esearch.fcgi"
	connector.FetchURL = server.URL + "/efetch.fcgi"
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "cancer", Limit: 2})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !searchCalled || !fetchCalled {
		t.Fatalf("expected pubmed search and fetch calls")
	}

	got := singlePaper(t, result)
	if got.Source != "pubmed" || got.PDFURL != "" || got.URL != "https://pubmed.ncbi.nlm.nih.gov/111111/" {
		t.Fatalf("unexpected pubmed urls %#v", got)
	}
	if got.Abstract != "First abstract part. Second part." || got.DOI != "10.1000/pubmed-1" {
		t.Fatalf("unexpected pubmed normalization %#v", got)
	}
}

func TestIACR(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "lattice cryptography" {
			t.Fatalf("expected iacr query, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
  <div class="mb-4">
    <a href="/2024/123">  Lattice   Crypto Paper </a>
    <div class="authors">Alice Smith, Bob Jones</div>
    <p class="mt-2">  Useful cryptography abstract. </p>
  </div>
</body></html>`))
	}))
	defer server.Close()

	connector := NewIACR()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "lattice cryptography", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PaperID != "2024/123" || got.URL != server.URL+"/2024/123" || got.PDFURL != server.URL+"/2024/123.pdf" {
		t.Fatalf("unexpected iacr paper %#v", got)
	}
}

func TestPMC(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "esearch.fcgi"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<eSearchResult><IdList><Id>1234567</Id></IdList></eSearchResult>`))
		case strings.Contains(r.URL.Path, "esummary.fcgi"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<eSummaryResult>
  <DocSum>
    <Id>1234567</Id>
    <Item Name="Title" Type="String">  PMC   Paper </Item>
    <Item Name="PubDate" Type="String">2024-05-03</Item>
    <Item Name="DOI" Type="String">10.1000/PMC-1</Item>
    <Item Name="AuthorList" Type="List">
      <Item Name="Author" Type="String">Alice Smith</Item>
      <Item Name="Author" Type="String">Bob Jones</Item>
    </Item>
  </DocSum>
</eSummaryResult>`))
		default:
			t.Fatalf("unexpected pmc path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	connector := NewPMC()
	connector.SearchURL = server.URL + "/esearch.fcgi"
	connector.SummaryURL = server.URL + "/esummary.fcgi"
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "genomics", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PaperID != "PMC1234567" || got.PDFURL != "https://pmc.ncbi.nlm.nih.gov/articles/PMC1234567/pdf/" {
		t.Fatalf("unexpected pmc paper %#v", got)
	}
}

func TestEuropePMC(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("format"); got != "json" {
			t.Fatalf("expected europepmc json format, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "resultList": {
    "result": [
      {
        "id": "987654",
        "source": "MED",
        "pmid": "987654",
        "pmcid": "PMC7654321",
        "doi": "10.1000/EPMC-1",
        "title": " Europe PMC   Paper ",
        "authorList": {
          "author": [
            {"fullName": "Alice Smith"},
            {"fullName": "Bob Jones"}
          ]
        },
        "abstractText": " Europe PMC abstract ",
        "pubYear": "2024",
        "pubMonth": "5",
        "pubDay": "7",
        "fullTextUrlList": {
          "fullTextUrl": [
            {"documentStyle": "html", "url": "https://pmc.ncbi.nlm.nih.gov/articles/PMC7654321/"},
            {"documentStyle": "pdf", "url": "https://pmc.ncbi.nlm.nih.gov/articles/PMC7654321/pdf/"}
          ]
        }
      }
    ]
  }
}`))
	}))
	defer server.Close()

	connector := NewEuropePMC()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "rare disease", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PaperID != "PMC7654321" || got.URL != "https://pmc.ncbi.nlm.nih.gov/articles/PMC7654321/" || got.PDFURL != "https://pmc.ncbi.nlm.nih.gov/articles/PMC7654321/pdf/" {
		t.Fatalf("unexpected europepmc paper %#v", got)
	}
}

func TestCORE(t *testing.T) {
	t.Parallel()

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "open access" {
			t.Fatalf("expected core query, got %q", got)
		}
		current := atomic.AddInt32(&attempts, 1)
		if current == 1 && r.Header.Get("Authorization") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "results": [
    {
      "id": 42,
      "title": "  CORE   Paper ",
      "authors": [{"name": "Alice Smith"}, {"name": "Bob Jones"}],
      "abstract": " CORE abstract ",
      "doi": "10.1000/CORE-1",
      "publishedDate": "2024-05-01T02:03:04Z",
      "fullTextUrls": ["https://example.org/core-paper.pdf"]
    }
  ]
}`))
	}))
	defer server.Close()

	connector := NewCORE(config.Config{CoreAPIKey: "bad-key"})
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "open access", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected core fallback without key, got %d attempts", attempts)
	}

	got := singlePaper(t, result)
	if got.URL != "https://doi.org/10.1000/core-1" || got.PDFURL != "https://example.org/core-paper.pdf" {
		t.Fatalf("unexpected core urls %#v", got)
	}
}

func TestDOAJ(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/search/articles/") {
			t.Fatalf("unexpected doaj path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "results": [
    {
      "id": "doaj-1",
      "bibjson": {
        "title": "  DOAJ   Paper ",
        "author": [{"name": "Alice Smith"}, {"name": "Bob Jones"}],
        "abstract": " DOAJ abstract ",
        "year": "2024",
        "month": "4",
        "day": "8",
        "identifier": [{"type": "doi", "id": "10.1000/DOAJ-1"}],
        "link": [
          {"type": "article", "url": "https://doaj.example/article"},
          {"type": "fulltext", "url": "https://doaj.example/paper.pdf"}
        ]
      }
    }
  ]
}`))
	}))
	defer server.Close()

	connector := NewDOAJ()
	connector.BaseURL = server.URL + "/api/search/articles"
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "machine learning", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PDFURL != "https://doaj.example/paper.pdf" || got.URL != "https://doaj.example/article" {
		t.Fatalf("unexpected doaj links %#v", got)
	}
}

func TestBASE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "institutional repositories" {
			t.Fatalf("expected base query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<response>
  <results>
    <doc>
      <str name="id">base:oai:1</str>
      <str name="title">  BASE   Paper </str>
      <arr name="creator"><str>Alice Smith</str><str>Bob Jones</str></arr>
      <str name="description"> BASE abstract </str>
      <str name="date">2024-02-01</str>
      <arr name="identifier">
        <str>https://base.example/record/1</str>
        <str>https://base.example/paper.pdf</str>
        <str>doi:10.1000/BASE-1</str>
      </arr>
    </doc>
  </results>
</response>`))
	}))
	defer server.Close()

	connector := NewBASE()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "institutional repositories", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PaperID != "base:oai:1" || got.URL != "https://base.example/record/1" || got.PDFURL != "https://base.example/paper.pdf" {
		t.Fatalf("unexpected base paper %#v", got)
	}
}

func TestZenodo(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "hits": {
    "hits": [
      {
        "id": 55,
        "metadata": {
          "title": "  Zenodo   Paper ",
          "creators": [{"name": "Alice Smith"}, {"name": "Bob Jones"}],
          "description": "<p>Zenodo abstract</p>",
          "doi": "10.1000/ZENODO-1",
          "publication_date": "2024-06-01"
        },
        "links": {
          "html": "https://zenodo.org/records/55"
        },
        "files": [
          {
            "key": "paper.pdf",
            "links": {"self": "https://zenodo.org/api/files/paper.pdf"}
          }
        ]
      }
    ]
  }
}`))
	}))
	defer server.Close()

	connector := NewZenodo()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "software citation", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Abstract != "Zenodo abstract" || got.PDFURL != "https://zenodo.org/api/files/paper.pdf" {
		t.Fatalf("unexpected zenodo paper %#v", got)
	}
}

func TestHAL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "response": {
    "docs": [
      {
        "docid": "hal-123",
        "title_s": ["  HAL   Paper "],
        "authFullName_s": [" Alice Smith ", "Bob Jones"],
        "abstract_s": [" HAL abstract "],
        "doiId_id": "10.1000/HAL-1",
        "uri_s": "https://hal.science/hal-123",
        "fileMain_s": "https://hal.science/hal-123/document",
        "producedDateY_i": 2025
      }
    ]
  }
}`))
	}))
	defer server.Close()

	connector := NewHAL()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "open science", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.PaperID != "hal-123" || got.Source != "hal" || got.PDFURL != "https://hal.science/hal-123/document" {
		t.Fatalf("unexpected hal paper %#v", got)
	}
}

func singlePaper(t *testing.T, result sources.SearchResult) paper.Paper {
	t.Helper()
	if result.Count != 1 || len(result.Papers) != 1 {
		t.Fatalf("expected single paper result, got %#v", result)
	}
	return result.Papers[0]
}
