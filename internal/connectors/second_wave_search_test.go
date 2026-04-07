package connectors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

func TestSemantic(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "graph neural networks" {
			t.Fatalf("expected semantic query, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Fatalf("expected semantic limit, got %q", got)
		}
		if got := r.URL.Query().Get("year"); got != "2024-2025" {
			t.Fatalf("expected semantic year filter, got %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "semantic-key" {
			t.Fatalf("expected semantic api key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "data": [
    {
      "paperId": "semantic-1",
      "title": "  Semantic   Paper ",
      "authors": [{"name": "Alice Smith"}, {"name": "Bob Jones"}],
      "abstract": " Semantic abstract ",
      "publicationDate": "2024-06-01",
      "externalIds": {"DOI": "10.1000/SEMANTIC-1"},
      "openAccessPdf": {"url": "https://semantic.example/paper.pdf"},
      "url": "https://semanticscholar.org/paper/semantic-1"
    }
  ]
}`))
	}))
	defer server.Close()

	connector := NewSemantic(config.Config{SemanticScholarAPIKey: "semantic-key"})
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{
		Query: "graph neural networks",
		Limit: 2,
		Year:  "2024-2025",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "semantic" || got.PaperID != "semantic-1" || got.DOI != "10.1000/semantic-1" {
		t.Fatalf("unexpected semantic paper %#v", got)
	}
	if got.PDFURL != "https://semantic.example/paper.pdf" || got.URL != "https://semanticscholar.org/paper/semantic-1" {
		t.Fatalf("unexpected semantic urls %#v", got)
	}
}

func TestCrossref(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query.bibliographic"); got != "transformers" {
			t.Fatalf("expected crossref query, got %q", got)
		}
		if got := r.URL.Query().Get("rows"); got != "1" {
			t.Fatalf("expected crossref rows, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "message": {
    "items": [
      {
        "DOI": "10.1000/CROSSREF-1",
        "title": ["  Crossref   Paper "],
        "author": [
          {"given": "Alice", "family": "Smith"},
          {"given": "Bob", "family": "Jones"}
        ],
        "abstract": "<jats:p> Crossref abstract </jats:p>",
        "issued": {"date-parts": [[2024, 5, 3]]},
        "URL": "https://doi.org/10.1000/CROSSREF-1"
      }
    ]
  }
}`))
	}))
	defer server.Close()

	connector := NewCrossref()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "transformers", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "crossref" || got.PaperID != "10.1000/CROSSREF-1" {
		t.Fatalf("unexpected crossref identity %#v", got)
	}
	if got.Abstract != "Crossref abstract" || got.PublishedDate != "2024-05-03" {
		t.Fatalf("unexpected crossref normalization %#v", got)
	}
}

func TestOpenAlex(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "knowledge graphs" {
			t.Fatalf("expected openalex search, got %q", got)
		}
		if got := r.URL.Query().Get("per-page"); got != "1" {
			t.Fatalf("expected openalex per-page, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "results": [
    {
      "id": "https://openalex.org/W1234567890",
      "display_name": "  OpenAlex   Paper ",
      "authorships": [
        {"author": {"display_name": "Alice Smith"}},
        {"author": {"display_name": "Bob Jones"}}
      ],
      "abstract_inverted_index": {
        "OpenAlex": [0],
        "abstract": [1]
      },
      "publication_date": "2024-04-09",
      "ids": {"doi": "https://doi.org/10.1000/OPENALEX-1"},
      "primary_location": {
        "landing_page_url": "https://openalex.example/paper",
        "pdf_url": "https://openalex.example/paper.pdf"
      },
      "open_access": {
        "oa_url": "https://openalex.example/paper.pdf"
      }
    }
  ]
}`))
	}))
	defer server.Close()

	connector := NewOpenAlex()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "knowledge graphs", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "openalex" || got.PaperID != "W1234567890" || got.DOI != "10.1000/openalex-1" {
		t.Fatalf("unexpected openalex paper %#v", got)
	}
	if got.Abstract != "OpenAlex abstract" || got.PDFURL != "https://openalex.example/paper.pdf" {
		t.Fatalf("unexpected openalex normalization %#v", got)
	}
}

func TestGoogleScholar(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "federated learning" {
			t.Fatalf("expected google scholar query, got %q", got)
		}
		if got := r.URL.Query().Get("num"); got != "1" {
			t.Fatalf("expected google scholar num, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
  <div class="gs_r gs_or gs_scl">
    <div class="gs_or_ggsm"><a href="https://scholar.example/paper.pdf">[PDF]</a></div>
    <h3 class="gs_rt"><a href="https://scholar.example/paper">  Google   Scholar Paper </a></h3>
    <div class="gs_a">Alice Smith, Bob Jones - 2024 - Example Journal</div>
    <div class="gs_rs"> Scholar abstract doi:10.1000/SCHOLAR-1 </div>
  </div>
</body></html>`))
	}))
	defer server.Close()

	connector := NewGoogleScholar(config.Config{GoogleScholarProxyURL: server.URL})
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "federated learning", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "google-scholar" || got.DOI != "10.1000/scholar-1" {
		t.Fatalf("unexpected google scholar paper %#v", got)
	}
	if got.PDFURL != "https://scholar.example/paper.pdf" || got.URL != "https://scholar.example/paper" {
		t.Fatalf("unexpected google scholar urls %#v", got)
	}
}

func TestDBLP(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "program synthesis" {
			t.Fatalf("expected dblp query, got %q", got)
		}
		if got := r.URL.Query().Get("h"); got != "1" {
			t.Fatalf("expected dblp limit, got %q", got)
		}
		if got := r.URL.Query().Get("format"); got != "xml" {
			t.Fatalf("expected dblp format, got %q", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<result>
  <hits>
    <hit>
      <info>
        <key>journals/example/Synthesis24</key>
        <title>  DBLP   Paper </title>
        <authors>
          <author>Alice Smith</author>
          <author>Bob Jones</author>
        </authors>
        <year>2024</year>
        <ee>https://dblp.example/paper</ee>
        <url>db/journals/example/example24</url>
      </info>
    </hit>
  </hits>
</result>`))
	}))
	defer server.Close()

	connector := NewDBLP()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "program synthesis", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "dblp" || got.PaperID != "journals/example/Synthesis24" {
		t.Fatalf("unexpected dblp paper %#v", got)
	}
	if got.URL != "https://dblp.org/rec/db/journals/example/example24" {
		t.Fatalf("unexpected dblp url %#v", got)
	}
}

func TestOpenAIRE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("keywords"); got != "climate science" {
			t.Fatalf("expected openaire keywords, got %q", got)
		}
		if got := r.URL.Query().Get("size"); got != "1" {
			t.Fatalf("expected openaire size, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "results": {
    "result": [
      {
        "id": "openaire-1",
        "title": "  OpenAIRE   Paper ",
        "authors": ["Alice Smith", "Bob Jones"],
        "description": " OpenAIRE abstract ",
        "doi": "10.1000/OPENAIRE-1",
        "dateofacceptance": "2024-04-09",
        "url": "https://openaire.example/paper"
      }
    ]
  }
}`))
	}))
	defer server.Close()

	connector := NewOpenAIRE()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "climate science", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "openaire" || got.PaperID != "openaire-1" || got.DOI != "10.1000/openaire-1" {
		t.Fatalf("unexpected openaire paper %#v", got)
	}
}

func TestCiteSeerX(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "static analysis" {
			t.Fatalf("expected citeseerx query, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
  <div class="result">
    <a class="remove doc_details" href="/viewdoc/summary?doi=10.1.1.1.1">  CiteSeerX   Paper </a>
    <div class="pubinfo">Alice Smith, Bob Jones - 2024</div>
    <div class="snippet"> CiteSeerX abstract </div>
    <a href="/viewdoc/download?doi=10.1.1.1.1&amp;rep=rep1&amp;type=pdf">PDF</a>
  </div>
</body></html>`))
	}))
	defer server.Close()

	connector := NewCiteSeerX()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "static analysis", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "citeseerx" || got.PaperID != "10.1.1.1.1" {
		t.Fatalf("unexpected citeseerx paper %#v", got)
	}
	if got.PDFURL == "" || !strings.Contains(got.PDFURL, "type=pdf") {
		t.Fatalf("expected citeseerx pdf url, got %#v", got)
	}
}

func TestSSRN(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("txtKey_Words"); got != "behavioral economics" {
			t.Fatalf("expected ssrn query, got %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
  <div class="search-result-content">
    <h2><a href="/sol3/papers.cfm?abstract_id=1234567">  SSRN   Paper </a></h2>
    <p class="authors">Alice Smith, Bob Jones</p>
    <div class="abstract-text"> SSRN abstract doi:10.1000/SSRN-1 </div>
    <a class="opt-link" href="https://papers.ssrn.com/sol3/Delivery.cfm/1234567.pdf">Download This Paper</a>
  </div>
</body></html>`))
	}))
	defer server.Close()

	connector := NewSSRN()
	connector.BaseURL = server.URL
	connector.Client = server.Client()

	result, err := connector.Search(sources.SearchRequest{Query: "behavioral economics", Limit: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	got := singlePaper(t, result)
	if got.Source != "ssrn" || got.PaperID != "1234567" || got.DOI != "10.1000/ssrn-1" {
		t.Fatalf("unexpected ssrn paper %#v", got)
	}
}

func TestUnpaywall(t *testing.T) {
	t.Parallel()

	t.Run("returns empty without doi or email", func(t *testing.T) {
		t.Parallel()

		connector := NewUnpaywall(config.Config{})
		result, err := connector.Search(sources.SearchRequest{Query: "plain text query", Limit: 1})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if result.Count != 0 || len(result.Papers) != 0 {
			t.Fatalf("expected empty unpaywall result, got %#v", result)
		}
	})

	t.Run("returns doi metadata when configured", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("email"); got != "tester@example.com" {
				t.Fatalf("expected unpaywall email, got %q", got)
			}
			if got := r.URL.Path; !strings.HasSuffix(strings.ToUpper(got), "/10.1000/UNPAYWALL-1") {
				t.Fatalf("expected doi path, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "doi": "10.1000/UNPAYWALL-1",
  "title": "  Unpaywall   Paper ",
  "published_date": "2024-02-04",
  "best_oa_location": {
    "url": "https://publisher.example/paper",
    "url_for_pdf": "https://publisher.example/paper.pdf"
  },
  "z_authors": [
    {"family": "Smith", "given": "Alice"},
    {"family": "Jones", "given": "Bob"}
  ]
}`))
		}))
		defer server.Close()

		connector := NewUnpaywall(config.Config{UnpaywallEmail: "tester@example.com"})
		connector.BaseURL = server.URL
		connector.Client = server.Client()

		result, err := connector.Search(sources.SearchRequest{
			Query: "See doi:10.1000/UNPAYWALL-1 for details",
			Limit: 1,
		})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}

		got := singlePaper(t, result)
		if got.Source != "unpaywall" || got.PaperID != "10.1000/UNPAYWALL-1" || got.DOI != "10.1000/unpaywall-1" {
			t.Fatalf("unexpected unpaywall paper %#v", got)
		}
		if got.URL != "https://publisher.example/paper" || got.PDFURL != "https://publisher.example/paper.pdf" {
			t.Fatalf("unexpected unpaywall urls %#v", got)
		}
	})
}

func TestIEEE(t *testing.T) {
	t.Parallel()

	connector := NewIEEE(config.Config{IEEEAPIKey: "ieee-key"})
	if connector.Descriptor().ID != "ieee" {
		t.Fatalf("expected ieee descriptor, got %#v", connector.Descriptor())
	}

	_, err := connector.Search(sources.SearchRequest{Query: "edge computing", Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected ieee unimplemented search error, got %v", err)
	}
}

func TestACM(t *testing.T) {
	t.Parallel()

	connector := NewACM(config.Config{ACMAPIKey: "acm-key"})
	if connector.Descriptor().ID != "acm" {
		t.Fatalf("expected acm descriptor, got %#v", connector.Descriptor())
	}

	_, err := connector.Search(sources.SearchRequest{Query: "human computer interaction", Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected acm unimplemented search error, got %v", err)
	}
}
