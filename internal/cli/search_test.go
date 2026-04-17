package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/connectors"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type searchCommandResponse struct {
	Status           string            `json:"status"`
	Mode             string            `json:"mode"`
	Query            string            `json:"query"`
	RequestedSources []string          `json:"requested_sources"`
	UsedSources      []string          `json:"used_sources"`
	InvalidSources   []string          `json:"invalid_sources"`
	FailedSources    []string          `json:"failed_sources"`
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
	if payload.Mode != "degraded" {
		t.Fatalf("expected degraded mode, got %#v", payload)
	}
	if payload.SourceResults["semantic"] != 1 || payload.SourceResults["crossref"] != 0 {
		t.Fatalf("unexpected source counts %#v", payload.SourceResults)
	}
	if !slices.Equal(payload.FailedSources, []string{"crossref"}) {
		t.Fatalf("unexpected failed sources %#v", payload.FailedSources)
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

func TestSearchJSONUsesEmptyArrays(t *testing.T) {
	t.Parallel()

	t.Run("default search serializes invalid sources as empty array", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "default sources query"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				}), nil
			},
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeSearchResponse(t, stdout.Bytes())
		if payload.InvalidSources == nil {
			t.Fatalf("expected invalid sources to be an empty slice, got nil in %#v", payload)
		}
		raw := stdout.String()
		if !strings.Contains(raw, `"invalid_sources":[]`) {
			t.Fatalf("expected invalid sources empty array in json, got %s", raw)
		}
		if strings.Contains(raw, `"invalid_sources":null`) {
			t.Fatalf("expected invalid sources to avoid null in json, got %s", raw)
		}
	})

	t.Run("zero results serialize papers and invalid sources as empty arrays", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic", "empty query"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				}), nil
			},
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeSearchResponse(t, stdout.Bytes())
		if payload.Papers == nil {
			t.Fatalf("expected papers to be an empty slice, got nil in %#v", payload)
		}
		if payload.InvalidSources == nil {
			t.Fatalf("expected invalid sources to be an empty slice, got nil in %#v", payload)
		}
		raw := stdout.String()
		if !strings.Contains(raw, `"papers":[]`) || !strings.Contains(raw, `"invalid_sources":[]`) {
			t.Fatalf("expected empty arrays in json, got %s", raw)
		}
		if strings.Contains(raw, `"papers":null`) || strings.Contains(raw, `"invalid_sources":null`) {
			t.Fatalf("expected no null arrays in json, got %s", raw)
		}
	})

	t.Run("paper with no authors serializes authors as empty array", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic", "authorless query"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{{
						PaperID: "authorless",
						Title:   "Authorless Paper",
						Source:  id,
					}},
				}), nil
			},
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeSearchResponse(t, stdout.Bytes())
		if len(payload.Papers) != 1 {
			t.Fatalf("expected one paper, got %#v", payload)
		}
		if payload.Papers[0].Authors == nil {
			t.Fatalf("expected authors to be an empty slice, got nil in %#v", payload.Papers[0])
		}
		raw := stdout.String()
		if !strings.Contains(raw, `"authors":[]`) {
			t.Fatalf("expected empty authors array in json, got %s", raw)
		}
		if strings.Contains(raw, `"authors":null`) {
			t.Fatalf("expected authors to avoid null in json, got %s", raw)
		}
	})
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

	t.Run("year is enforced on final json results for all sources", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic,crossref", "--year", "2024", "filtered results"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				switch id {
				case "semantic":
					return sources.NewStubConnector(sources.StubConnector{
						DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
						SearchResults: []paper.Paper{
							{PaperID: "semantic-2024", Title: "Semantic 2024", PublishedDate: "2024-06-01", Source: id},
							{PaperID: "semantic-2023", Title: "Semantic 2023", PublishedDate: "2023-06-01", Source: id},
						},
					}), nil
				case "crossref":
					return sources.NewStubConnector(sources.StubConnector{
						DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
						SearchResults: []paper.Paper{
							{PaperID: "crossref-2024", Title: "Crossref 2024", PublishedDate: "2024-01-05", Source: id},
							{PaperID: "crossref-2022", Title: "Crossref 2022", PublishedDate: "2022-01-05", Source: id},
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
		if payload.Total != 2 || len(payload.Papers) != 2 {
			t.Fatalf("expected two 2024 papers, got %#v", payload)
		}
		if payload.SourceResults["semantic"] != 1 || payload.SourceResults["crossref"] != 1 {
			t.Fatalf("expected post-filter source counts, got %#v", payload.SourceResults)
		}
		for _, item := range payload.Papers {
			if !strings.HasPrefix(item.PublishedDate, "2024") {
				t.Fatalf("expected only 2024 papers after local filter, got %#v", payload.Papers)
			}
		}
	})

	t.Run("invalid year format returns invalid usage", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic", "--year", "2024/2025", "bad year"}, &stdout, &stderr, runOptions{
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
		})
		if exitCode != 2 {
			t.Fatalf("expected exit code 2, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		var payload struct {
			Status string `json:"status"`
			Error  struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("expected json error, got %q: %v", stdout.String(), err)
		}
		if payload.Error.Code != "invalid_usage" || !strings.Contains(payload.Error.Message, "YYYY or YYYY-YYYY") {
			t.Fatalf("unexpected invalid year payload %#v", payload)
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
	if payload.Status != "ok" || payload.Mode != "degraded" || payload.Total != 0 || len(payload.Errors) != 2 || payload.SourceResults["semantic"] != 0 || payload.SourceResults["crossref"] != 0 {
		t.Fatalf("unexpected all-failed payload %#v", payload)
	}
	if !slices.Equal(payload.FailedSources, []string{"semantic", "crossref"}) {
		t.Fatalf("expected failed sources list, got %#v", payload)
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

func TestGatedSearchSources(t *testing.T) {
	t.Parallel()

	t.Run("only gated sources return unsupported machine-readable error", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "ieee", "gated query"}, &stdout, &stderr, runOptions{
			environ:        []string{},
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
		})
		if exitCode != 3 {
			t.Fatalf("expected exit code 3, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected empty stderr, got %q", stderr.String())
		}

		var payload struct {
			Status string `json:"status"`
			Error  struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("expected json error, got %q: %v", stdout.String(), err)
		}
		if payload.Status != "error" || payload.Error.Code != "gated_source" {
			t.Fatalf("unexpected gated source payload %#v", payload)
		}
		blocked, ok := payload.Error.Details["blocked_sources"].([]any)
		if !ok || len(blocked) != 1 {
			t.Fatalf("expected one blocked source entry, got %#v", payload.Error.Details)
		}
		entry, ok := blocked[0].(map[string]any)
		if !ok {
			t.Fatalf("expected blocked source map, got %#v", blocked[0])
		}
		if entry["id"] != "ieee" || entry["capability"] != "gated" {
			t.Fatalf("expected ieee gated entry, got %#v", entry)
		}
		if reason, _ := entry["reason"].(string); !strings.Contains(strings.ToLower(reason), "missing required credential") {
			t.Fatalf("expected missing credential reason, got %#v", entry)
		}
	})

	t.Run("mixed valid and gated sources keep partial success explicit", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "semantic,ieee", "mixed gated query"}, &stdout, &stderr, runOptions{
			environ:        []string{},
			workingDir:     t.TempDir(),
			repositoryRoot: t.TempDir(),
			connectorFactory: func(id string, _ config.Config) (sources.Connector, error) {
				if id != "semantic" {
					t.Fatalf("unexpected connector %q", id)
				}
				return sources.NewStubConnector(sources.StubConnector{
					DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
					SearchResults: []paper.Paper{
						{PaperID: "semantic-1", Title: "Semantic", Source: id},
					},
				}), nil
			},
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		payload := decodeSearchResponse(t, stdout.Bytes())
		if !slices.Equal(payload.RequestedSources, []string{"semantic", "ieee"}) {
			t.Fatalf("expected requested sources to preserve gated request, got %#v", payload)
		}
		if !slices.Equal(payload.UsedSources, []string{"semantic"}) {
			t.Fatalf("expected only supported source to run, got %#v", payload)
		}
		if payload.SourceResults["ieee"] != 0 {
			t.Fatalf("expected gated source count to remain zero, got %#v", payload.SourceResults)
		}
		if !strings.Contains(strings.ToLower(payload.Errors["ieee"]), "missing required credential") {
			t.Fatalf("expected gated source error to be machine-readable, got %#v", payload.Errors)
		}
		if payload.Total != 1 || len(payload.Papers) != 1 || payload.Papers[0].Source != "semantic" {
			t.Fatalf("expected semantic results to remain available, got %#v", payload)
		}
	})
}

func TestSearchUsesMergedConfigForEndpointAndGating(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	writeCLIConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"acm_api_key: acm-from-file",
		"arxiv_base_url: https://yaml.example/arxiv",
		"",
	}, "\n"))

	var requestedPath string
	var requestedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.Query().Get("search_query")
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/1234.5678v1</id>
    <title>Merged Config Search</title>
    <summary>Search result from env-selected endpoint.</summary>
    <published>2024-04-08T00:00:00Z</published>
    <author><name>Alice Example</name></author>
    <link rel="alternate" type="text/html" href="http://arxiv.org/abs/1234.5678v1"></link>
  </entry>
</feed>`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"search", "--source", "arxiv", "merged config query"}, &stdout, &stderr, runOptions{
		environ:          []string{"HOME=" + homeDir, "SEARCH_PAPER_ARXIV_BASE_URL=" + server.URL + "/env-arxiv"},
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectors.New,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	if requestedPath != "/env-arxiv" {
		t.Fatalf("expected env endpoint to win over config endpoint, got path %q", requestedPath)
	}
	if requestedQuery != "all:merged config query" {
		t.Fatalf("expected arxiv query to hit env endpoint, got %q", requestedQuery)
	}

	payload := decodeSearchResponse(t, stdout.Bytes())
	if payload.Total != 1 || len(payload.Papers) != 1 {
		t.Fatalf("expected one search result, got %#v", payload)
	}

	var sourcesStdout bytes.Buffer
	var sourcesStderr bytes.Buffer
	sourcesExit := runWithOptions([]string{"sources", "--format", "json"}, &sourcesStdout, &sourcesStderr, runOptions{
		environ:        []string{"HOME=" + homeDir, "SEARCH_PAPER_ARXIV_BASE_URL=" + server.URL + "/env-arxiv"},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if sourcesExit != 0 {
		t.Fatalf("expected sources exit code 0, got %d with stdout=%q stderr=%q", sourcesExit, sourcesStdout.String(), sourcesStderr.String())
	}

	var sourcesPayload struct {
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(sourcesStdout.Bytes(), &sourcesPayload); err != nil {
		t.Fatalf("expected valid sources payload, got %q: %v", sourcesStdout.String(), err)
	}
	assertSourceCapability(t, sourcesPayload.Sources, "acm", true, "", "supported", "unsupported", "unsupported")
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
