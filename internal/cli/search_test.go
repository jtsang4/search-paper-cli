package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type searchCommandResponse struct {
	Status           string            `json:"status"`
	Query            string            `json:"query"`
	RequestedSources []string          `json:"requested_sources"`
	UsedSources      []string          `json:"used_sources"`
	InvalidSources   []string          `json:"invalid_sources"`
	SourceResults    map[string]int    `json:"source_results"`
	Errors           map[string]string `json:"errors"`
	Total            int               `json:"total"`
	Papers           []paper.Paper     `json:"papers"`
	Error            map[string]any    `json:"error"`
}

func TestSingleSourceSearch(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic", "--limit", "1", "graph neural networks"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, cfg config.Config) (sources.Connector, error) {
			if cfg != (config.Config{}) {
				t.Fatalf("expected empty config, got %#v", cfg)
			}
			if id != "semantic" {
				t.Fatalf("unexpected connector id %q", id)
			}
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{
					ID:      "semantic",
					Enabled: true,
					Capabilities: sources.Capabilities{
						Search:   sources.CapabilitySupported,
						Download: sources.CapabilityRecordDependent,
						Read:     sources.CapabilityRecordDependent,
					},
				},
				SearchResults: []paper.Paper{
					{PaperID: "s-1", Title: "First Result", Authors: []string{"Alice"}, DOI: "10.1000/first", Source: "semantic"},
					{PaperID: "s-2", Title: "Second Result", Authors: []string{"Bob"}, DOI: "10.1000/second", Source: "semantic"},
				},
			}), nil
		},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	payload := decodeSearchResponse(t, stdout.Bytes())
	if payload.Status != "ok" || payload.Total != 1 || len(payload.Papers) != 1 {
		t.Fatalf("unexpected payload %#v", payload)
	}
	if !slices.Equal(payload.RequestedSources, []string{"semantic"}) || !slices.Equal(payload.UsedSources, []string{"semantic"}) {
		t.Fatalf("unexpected sources %#v", payload)
	}
	if payload.Papers[0].Source != "semantic" {
		t.Fatalf("expected semantic result, got %#v", payload.Papers[0])
	}
	if payload.SourceResults["semantic"] != 1 {
		t.Fatalf("expected source result count 1, got %#v", payload.SourceResults)
	}
}

func TestInvalidSources(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "bogus", "transformers"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var payload struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				InvalidSource string   `json:"invalid_source"`
				ValidSources  []string `json:"valid_sources"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected json error, got %q: %v", stdout.String(), err)
	}
	if payload.Status != "error" || payload.Error.Code != "invalid_source" || payload.Error.Details.InvalidSource != "bogus" {
		t.Fatalf("unexpected invalid source payload %#v", payload)
	}
	if !slices.Contains(payload.Error.Details.ValidSources, "semantic") {
		t.Fatalf("expected valid sources in payload, got %#v", payload)
	}
}

func TestUnifiedPartialSuccess(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic,crossref", "agentic retrieval"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			switch id {
			case "semantic":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{
						ID:           "semantic",
						Enabled:      true,
						Capabilities: sources.Capabilities{Search: sources.CapabilitySupported},
					},
					SearchResults: []paper.Paper{
						{PaperID: "sem-1", Title: "Semantic Result", Authors: []string{"Alice"}, Source: "semantic"},
					},
				}), nil
			case "crossref":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{
						ID:           "crossref",
						Enabled:      true,
						Capabilities: sources.Capabilities{Search: sources.CapabilitySupported},
					},
					SearchError: errors.New("crossref upstream failure"),
				}), nil
			default:
				t.Fatalf("unexpected connector %q", id)
				return nil, nil
			}
		},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	if payload.Total != 1 || len(payload.Papers) != 1 {
		t.Fatalf("expected one surviving paper, got %#v", payload)
	}
	if payload.SourceResults["semantic"] != 1 || payload.SourceResults["crossref"] != 0 {
		t.Fatalf("unexpected source counts %#v", payload.SourceResults)
	}
	if payload.Errors["crossref"] != "crossref upstream failure" {
		t.Fatalf("unexpected source errors %#v", payload.Errors)
	}
}

func TestDedupeOrder(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "crossref,semantic,arxiv", "dedupe me"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			switch id {
			case "crossref":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: "crossref", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "cr-doi", Title: "DOI Winner", Authors: []string{"Alice"}, DOI: "10.1000/shared", Source: "crossref"},
						{PaperID: "cr-title", Title: "Shared Title", Authors: []string{"Alice", "Bob"}, Source: "crossref"},
						{PaperID: "dup-id", Source: "crossref"},
					},
				}), nil
			case "semantic":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: "semantic", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "sem-doi", Title: "Other", Authors: []string{"Else"}, DOI: "https://doi.org/10.1000/shared", Source: "semantic"},
						{PaperID: "sem-title", Title: " Shared  Title ", Authors: []string{" Alice ", "Bob"}, Source: "semantic"},
						{PaperID: "dup-id", Source: "semantic"},
					},
				}), nil
			case "arxiv":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "unique-arxiv", Title: "Unique", Authors: []string{"Carol"}, Source: "arxiv"},
					},
				}), nil
			default:
				t.Fatalf("unexpected connector %q", id)
				return nil, nil
			}
		},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	if !slices.Equal(payload.UsedSources, []string{"crossref", "semantic", "arxiv"}) {
		t.Fatalf("unexpected used sources %#v", payload.UsedSources)
	}
	if payload.Total != 4 {
		t.Fatalf("expected 4 deduped papers, got %#v", payload)
	}
	if payload.Papers[0].Source != "crossref" || payload.Papers[1].Source != "crossref" || payload.Papers[2].Source != "crossref" {
		t.Fatalf("expected first survivors from first selected source, got %#v", payload.Papers)
	}
}

func TestSearchSchema(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic", "schema query"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				SearchResults: []paper.Paper{{
					PaperID:       "schema-1",
					Title:         "Schema Result",
					Authors:       []string{"Alice Smith"},
					Abstract:      "A summary",
					DOI:           "10.1000/schema",
					PublishedDate: "2024-04-01",
					PDFURL:        "https://example.com/paper.pdf",
					URL:           "https://example.com/paper",
					Source:        id,
				}},
			}), nil
		},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	got := payload.Papers[0]
	if got.PaperID == "" || got.Title == "" || got.Abstract == "" || got.DOI == "" || got.PublishedDate == "" || got.PDFURL == "" || got.URL == "" || got.Source == "" || len(got.Authors) != 1 {
		t.Fatalf("expected normalized schema fields, got %#v", got)
	}
}

func TestSemanticYearFilter(t *testing.T) {
	t.Parallel()

	t.Run("year forwarded for semantic only", func(t *testing.T) {
		t.Parallel()

		requests := map[string][]sources.SearchRequest{}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic,crossref", "--year", "2024-2025", "multimodal agents"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				return recordingConnector{
					id:       id,
					requests: requests,
				}, nil
			},
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		if got := requests["semantic"][0].Year; got != "2024-2025" {
			t.Fatalf("expected semantic year filter, got %#v", requests)
		}
		if got := requests["crossref"][0].Year; got != "" {
			t.Fatalf("expected non-semantic sources to receive empty year, got %#v", requests)
		}
	})
}

func TestUnpaywallDOIOnly(t *testing.T) {
	t.Parallel()

	connectorFactory := func(id string, _ config.Config) (sources.Connector, error) {
		return doiOnlyConnector{id: id}, nil
	}

	t.Run("doi query returns one result", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "unpaywall", "doi:10.1000/unified"}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectorFactory,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		payload := decodeSearchResponse(t, stdout.Bytes())
		if payload.Total != 1 || payload.Papers[0].Source != "unpaywall" {
			t.Fatalf("unexpected payload %#v", payload)
		}
	})

	t.Run("free text query soft fails to empty", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "unpaywall", "plain text search"}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectorFactory,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		payload := decodeSearchResponse(t, stdout.Bytes())
		if payload.Total != 0 || len(payload.Papers) != 0 || payload.SourceResults["unpaywall"] != 0 {
			t.Fatalf("unexpected payload %#v", payload)
		}
	})
}

func TestSearchCounts(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic,crossref", "overlap query"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			switch id {
			case "semantic":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: "semantic", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "same", DOI: "10.1000/shared", Source: "semantic"},
						{PaperID: "only-semantic", Source: "semantic"},
					},
				}), nil
			case "crossref":
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: "crossref", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "same-other", DOI: "10.1000/shared", Source: "crossref"},
						{PaperID: "only-crossref", Source: "crossref"},
					},
				}), nil
			default:
				return nil, errors.New("unexpected connector")
			}
		},
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	sum := payload.SourceResults["semantic"] + payload.SourceResults["crossref"]
	if payload.SourceResults["semantic"] != 2 || payload.SourceResults["crossref"] != 2 || payload.Total != 3 || payload.Total > sum {
		t.Fatalf("expected pre-dedupe counts and post-dedupe total, got %#v", payload)
	}
}

func TestAllSourcesFail(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic,crossref", "total failure"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				SearchError:     errors.New(id + " failed"),
			}), nil
		},
	})
	if exitCode != 4 {
		t.Fatalf("expected exit code 4, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	if payload.Status != "ok" || payload.Total != 0 || len(payload.Errors) != 2 || payload.SourceResults["semantic"] != 0 || payload.SourceResults["crossref"] != 0 {
		t.Fatalf("unexpected all-failed payload %#v", payload)
	}
}

func TestMixedValidInvalidSources(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "semantic,typo,crossref", "mixed query"}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
		connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				SearchResults: []paper.Paper{
					{PaperID: id + "-1", Title: strings.ToUpper(id), Source: id},
				},
			}), nil
		},
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload := decodeSearchResponse(t, stdout.Bytes())
	if !slices.Equal(payload.InvalidSources, []string{"typo"}) {
		t.Fatalf("expected explicit invalid sources, got %#v", payload)
	}
	if !slices.Equal(payload.UsedSources, []string{"semantic", "crossref"}) {
		t.Fatalf("expected valid used sources only, got %#v", payload)
	}
	if payload.Total != 2 {
		t.Fatalf("expected valid source results only, got %#v", payload)
	}
}

func decodeSearchResponse(t *testing.T, data []byte) searchCommandResponse {
	t.Helper()

	var payload searchCommandResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", string(data), err)
	}
	return payload
}

type recordingConnector struct {
	id       string
	requests map[string][]sources.SearchRequest
}

func (c recordingConnector) Descriptor() sources.Descriptor {
	return sources.Descriptor{ID: c.id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}}
}

func (c recordingConnector) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	c.requests[c.id] = append(c.requests[c.id], request)
	return sources.SearchResult{
		Count:  1,
		Papers: []paper.Paper{{PaperID: c.id + "-1", Title: c.id, Source: c.id}},
	}, nil
}

func (c recordingConnector) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{}, nil
}

func (c recordingConnector) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{}, nil
}

type doiOnlyConnector struct {
	id string
}

func (c doiOnlyConnector) Descriptor() sources.Descriptor {
	return sources.Descriptor{ID: c.id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}}
}

func (c doiOnlyConnector) Search(request sources.SearchRequest) (sources.SearchResult, error) {
	if !strings.Contains(strings.ToLower(request.Query), "10.") {
		return sources.SearchResult{}, nil
	}
	return sources.SearchResult{
		Count: 1,
		Papers: []paper.Paper{{
			PaperID: "10.1000/unified",
			Title:   "Unpaywall Result",
			DOI:     "10.1000/unified",
			Source:  c.id,
		}},
	}, nil
}

func (c doiOnlyConnector) Download(sources.DownloadRequest) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{}, nil
}

func (c doiOnlyConnector) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return sources.RetrievalResult{}, nil
}
