package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/connectors"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type retrievalCommandResponse struct {
	Status       string `json:"status"`
	Operation    string `json:"operation"`
	State        string `json:"state"`
	Source       string `json:"source"`
	PaperID      string `json:"paper_id"`
	Path         string `json:"path"`
	Content      string `json:"content"`
	Message      string `json:"message"`
	WinningStage string `json:"winning_stage"`
	Attempts     []any  `json:"attempts"`
}

func TestNativeDownloadSavePath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/paper.pdf" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(minimalPDF("Native download text"))
	}))
	defer server.Close()

	saveDir := filepath.Join(t.TempDir(), "downloads")
	paperJSON := `{"paper_id":"1234.5678","title":"Native Download","pdf_url":"` + server.URL + `/paper.pdf","source":"arxiv"}`

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
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

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.Status != "ok" || payload.Operation != "download" || payload.State != "downloaded" {
		t.Fatalf("unexpected payload %#v", payload)
	}
	if payload.Source != "arxiv" || payload.PaperID != "1234.5678" {
		t.Fatalf("unexpected source metadata %#v", payload)
	}
	if payload.Path == "" {
		t.Fatalf("expected saved path, got %#v", payload)
	}
	if !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected path inside save dir %q, got %q", saveDir, payload.Path)
	}
	info, err := os.Stat(payload.Path)
	if err != nil {
		t.Fatalf("expected downloaded file to exist: %v", err)
	}
	if info.IsDir() || filepath.Ext(payload.Path) != ".pdf" {
		t.Fatalf("expected saved pdf file, got %#v", info)
	}
}

func TestReadStates(t *testing.T) {
	t.Parallel()

	t.Run("extractable PDF returns extracted state and content", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Extracted retrieval text"))
		}))
		defer server.Close()

		saveDir := filepath.Join(t.TempDir(), "readable")
		paperJSON := `{"paper_id":"read-1","title":"Readable","pdf_url":"` + server.URL + `/paper.pdf","source":"pmc"}`

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"read", "--source", "pmc", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectors.New,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeRetrievalResponse(t, stdout.Bytes())
		if payload.State != "extracted" {
			t.Fatalf("expected extracted state, got %#v", payload)
		}
		if !strings.Contains(payload.Content, "Extracted retrieval text") {
			t.Fatalf("expected extracted content, got %#v", payload)
		}
		if payload.Path == "" {
			t.Fatalf("expected downloaded path, got %#v", payload)
		}
	})

	t.Run("non-extractable PDF returns downloaded-but-not-extractable state", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<<>>\n%%EOF"))
		}))
		defer server.Close()

		saveDir := filepath.Join(t.TempDir(), "opaque")
		paperJSON := `{"paper_id":"read-2","title":"Opaque","pdf_url":"` + server.URL + `/paper.pdf","source":"core"}`

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"read", "--source", "core", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectors.New,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeRetrievalResponse(t, stdout.Bytes())
		if payload.State != "downloaded_but_not_extractable" {
			t.Fatalf("expected downloaded_but_not_extractable state, got %#v", payload)
		}
		if payload.Content != "" {
			t.Fatalf("expected empty content, got %#v", payload)
		}
		if payload.Path == "" {
			t.Fatalf("expected downloaded path, got %#v", payload)
		}
	})
}

func TestMissingDirectoryCreation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(minimalPDF("Directory creation text"))
	}))
	defer server.Close()

	root := t.TempDir()
	saveDir := filepath.Join(root, "missing", "nested")
	if _, err := os.Stat(saveDir); !os.IsNotExist(err) {
		t.Fatalf("expected save dir to start missing, got err=%v", err)
	}

	paperJSON := `{"paper_id":"mkdir-1","title":"Create Directory","pdf_url":"` + server.URL + `/paper.pdf","source":"biorxiv"}`

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"read", "--source", "biorxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectors.New,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.Path == "" {
		t.Fatalf("expected path in payload, got %#v", payload)
	}
	if !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected path inside created directory %q, got %q", saveDir, payload.Path)
	}
	if _, err := os.Stat(saveDir); err != nil {
		t.Fatalf("expected save dir to exist after read, got %v", err)
	}
}

func TestRecordDependentRetrieval(t *testing.T) {
	t.Parallel()

	t.Run("record-dependent source succeeds when a public PDF exists", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Record dependent success"))
		}))
		defer server.Close()

		saveDir := filepath.Join(t.TempDir(), "semantic")
		paperJSON := `{"paper_id":"semantic-1","title":"Semantic OA","pdf_url":"` + server.URL + `/paper.pdf","source":"semantic"}`

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"download", "--source", "semantic", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectors.New,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeRetrievalResponse(t, stdout.Bytes())
		if payload.State != "downloaded" || payload.Path == "" {
			t.Fatalf("expected successful download, got %#v", payload)
		}
		if _, err := os.Stat(payload.Path); err != nil {
			t.Fatalf("expected downloaded file to exist, got %v", err)
		}
	})

	t.Run("record-dependent source returns explicit not-found without stray files", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body>sign in required</body></html>"))
		}))
		defer server.Close()

		saveDir := filepath.Join(t.TempDir(), "semantic-miss")
		paperJSON := `{"paper_id":"semantic-2","title":"Semantic Missing","pdf_url":"` + server.URL + `/paper.pdf","source":"semantic"}`

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"download", "--source", "semantic", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectors.New,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeRetrievalResponse(t, stdout.Bytes())
		if payload.State != "not_found" || payload.Path != "" {
			t.Fatalf("expected explicit not_found without file path, got %#v", payload)
		}
		entries, err := os.ReadDir(saveDir)
		if err != nil {
			t.Fatalf("expected save dir to exist, got %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected no stray files, got %#v", entries)
		}
	})

	t.Run("mislabeled non-PDF body does not count as success", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("<html><body>login required</body></html>"))
		}))
		defer server.Close()

		saveDir := filepath.Join(t.TempDir(), "semantic-mislabeled")
		paperJSON := `{"paper_id":"semantic-3","title":"Semantic Mislabeled","pdf_url":"` + server.URL + `/paper.pdf","source":"semantic"}`

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runWithOptions([]string{"download", "--source", "semantic", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectors.New,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
		}

		payload := decodeRetrievalResponse(t, stdout.Bytes())
		if payload.State != "not_found" || payload.Path != "" {
			t.Fatalf("expected explicit not_found without file path, got %#v", payload)
		}
		entries, err := os.ReadDir(saveDir)
		if err != nil {
			t.Fatalf("expected save dir to exist, got %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected no stray files, got %#v", entries)
		}
	})
}

func TestSSRNBestEffort(t *testing.T) {
	t.Parallel()

	saveDir := filepath.Join(t.TempDir(), "ssrn")
	paperJSON := `{"paper_id":"1234567","title":"SSRN Paper","url":"https://papers.ssrn.com/sol3/papers.cfm?abstract_id=1234567","source":"ssrn"}`

	for _, operation := range []string{"download", "read"} {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := runWithOptions([]string{operation, "--source", "ssrn", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
				workingDir:       t.TempDir(),
				repositoryRoot:   t.TempDir(),
				connectorFactory: connectors.New,
			})
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
			}

			payload := decodeRetrievalResponse(t, stdout.Bytes())
			if payload.State != "not_found" {
				t.Fatalf("expected not_found state, got %#v", payload)
			}
			if !strings.Contains(strings.ToLower(payload.Message), "public pdf") {
				t.Fatalf("expected explanatory message about missing public PDF, got %#v", payload)
			}
			entries, err := os.ReadDir(saveDir)
			if err != nil {
				t.Fatalf("expected save dir to exist, got %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected no stray files, got %#v", entries)
			}
		})
	}
}

func TestInformationalRetrieval(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		sourceID  string
		operation string
		wantText  string
	}{
		{name: "pubmed-download", sourceID: "pubmed", operation: "download", wantText: "does not provide direct download"},
		{name: "pubmed-read", sourceID: "pubmed", operation: "read", wantText: "only exposes metadata"},
		{name: "crossref-download", sourceID: "crossref", operation: "download", wantText: "does not provide direct download"},
		{name: "openalex-read", sourceID: "openalex", operation: "read", wantText: "only exposes metadata"},
		{name: "google-scholar-download", sourceID: "google-scholar", operation: "download", wantText: "does not provide direct download"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			saveDir := filepath.Join(t.TempDir(), "informational")
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				t.Fatalf("mkdir save dir: %v", err)
			}

			paperJSON := `{"paper_id":"info-1","title":"Metadata only","source":"` + tc.sourceID + `"}`

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := runWithOptions([]string{tc.operation, "--source", tc.sourceID, "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
				workingDir:       t.TempDir(),
				repositoryRoot:   t.TempDir(),
				connectorFactory: connectors.New,
			})
			if exitCode != exitCodeUnsupported {
				t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeUnsupported, exitCode, stdout.String(), stderr.String())
			}

			payload := decodeRetrievalResponse(t, stdout.Bytes())
			if payload.Status != "ok" || payload.State != "informational" {
				t.Fatalf("expected informational retrieval payload, got %#v", payload)
			}
			if payload.Path != "" || payload.Content != "" {
				t.Fatalf("expected no file/content side effects, got %#v", payload)
			}
			if !strings.Contains(strings.ToLower(payload.Message), strings.ToLower(tc.wantText)) {
				t.Fatalf("expected informational message to contain %q, got %#v", tc.wantText, payload)
			}

			entries, err := os.ReadDir(saveDir)
			if err != nil {
				t.Fatalf("expected save dir to exist, got %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected no stray files, got %#v", entries)
			}
		})
	}
}

func TestUnsupportedRetrieval(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		sourceID  string
		operation string
	}{
		{name: "dblp-download", sourceID: "dblp", operation: "download"},
		{name: "dblp-read", sourceID: "dblp", operation: "read"},
		{name: "openaire-download", sourceID: "openaire", operation: "download"},
		{name: "openaire-read", sourceID: "openaire", operation: "read"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			saveDir := filepath.Join(t.TempDir(), "unsupported")
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				t.Fatalf("mkdir save dir: %v", err)
			}

			paperJSON := `{"paper_id":"unsupported-1","title":"Unsupported","source":"` + tc.sourceID + `"}`

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := runWithOptions([]string{tc.operation, "--source", tc.sourceID, "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
				workingDir:       t.TempDir(),
				repositoryRoot:   t.TempDir(),
				connectorFactory: connectors.New,
			})
			if exitCode != exitCodeUnsupported {
				t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeUnsupported, exitCode, stdout.String(), stderr.String())
			}

			payload := decodeRetrievalResponse(t, stdout.Bytes())
			if payload.Status != "ok" || payload.State != "unsupported" {
				t.Fatalf("expected unsupported retrieval payload, got %#v", payload)
			}
			if payload.Path != "" || payload.Content != "" {
				t.Fatalf("expected no file/content side effects, got %#v", payload)
			}
			if !strings.Contains(strings.ToLower(payload.Message), "not supported") {
				t.Fatalf("expected unsupported message, got %#v", payload)
			}

			entries, err := os.ReadDir(saveDir)
			if err != nil {
				t.Fatalf("expected save dir to exist, got %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected no stray files, got %#v", entries)
			}
		})
	}
}

func TestUnpaywallStandaloneLimits(t *testing.T) {
	t.Parallel()

	t.Run("search requires configured email but remains metadata-only when configured", func(t *testing.T) {
		t.Parallel()

		connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
			connector, err := connectors.New(id, cfg)
			if err != nil {
				return nil, err
			}
			unpaywall, ok := connector.(*connectors.Unpaywall)
			if ok {
				unpaywall.BaseURL = "http://example.invalid/unpaywall"
				unpaywall.Client = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						body := `{"doi":"10.1000/UNPAYWALL-1","title":"Unpaywall Metadata","published_date":"2024-04-08","best_oa_location":{"url":"https://publisher.example/paper","url_for_pdf":"https://publisher.example/paper.pdf"},"z_authors":[{"given":"Ada","family":"Lovelace"}]}`
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader(body)),
							Request:    req,
						}, nil
					}),
				}
			}
			return connector, nil
		}

		var cleanStdout bytes.Buffer
		var cleanStderr bytes.Buffer
		exitCode := runWithOptions([]string{"search", "--source", "unpaywall", "doi:10.1000/unpaywall-1"}, &cleanStdout, &cleanStderr, runOptions{
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectorFactory,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, cleanStdout.String(), cleanStderr.String())
		}

		cleanPayload := decodeSearchResponse(t, cleanStdout.Bytes())
		if cleanPayload.Total != 0 || len(cleanPayload.Papers) != 0 {
			t.Fatalf("expected unconfigured unpaywall search to return no results, got %#v", cleanPayload)
		}

		var configuredStdout bytes.Buffer
		var configuredStderr bytes.Buffer
		exitCode = runWithOptions([]string{"search", "--source", "unpaywall", "doi:10.1000/unpaywall-1"}, &configuredStdout, &configuredStderr, runOptions{
			environ:          []string{"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=tester@example.com"},
			workingDir:       t.TempDir(),
			repositoryRoot:   t.TempDir(),
			connectorFactory: connectorFactory,
		})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, configuredStdout.String(), configuredStderr.String())
		}

		configuredPayload := decodeSearchResponse(t, configuredStdout.Bytes())
		if configuredPayload.Total != 1 || len(configuredPayload.Papers) != 1 {
			t.Fatalf("expected configured unpaywall search to return one metadata result, got %#v", configuredPayload)
		}
		if configuredPayload.Papers[0].Source != "unpaywall" || configuredPayload.Papers[0].PDFURL == "" {
			t.Fatalf("expected metadata-only unpaywall record, got %#v", configuredPayload.Papers[0])
		}
	})

	for _, operation := range []string{"download", "read"} {
		operation := operation
		t.Run(operation+" remains unsupported", func(t *testing.T) {
			t.Parallel()

			saveDir := filepath.Join(t.TempDir(), "unpaywall")
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				t.Fatalf("mkdir save dir: %v", err)
			}

			paperJSON := `{"paper_id":"10.1000/unpaywall-1","title":"Unpaywall Metadata","doi":"10.1000/unpaywall-1","pdf_url":"https://publisher.example/paper.pdf","source":"unpaywall"}`

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := runWithOptions([]string{operation, "--source", "unpaywall", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
				environ:          []string{"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=tester@example.com"},
				workingDir:       t.TempDir(),
				repositoryRoot:   t.TempDir(),
				connectorFactory: connectors.New,
			})
			if exitCode != exitCodeUnsupported {
				t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeUnsupported, exitCode, stdout.String(), stderr.String())
			}

			payload := decodeRetrievalResponse(t, stdout.Bytes())
			if payload.State != "unsupported" {
				t.Fatalf("expected unsupported unpaywall retrieval, got %#v", payload)
			}
			if !strings.Contains(strings.ToLower(payload.Message), "metadata") {
				t.Fatalf("expected metadata-only explanation, got %#v", payload)
			}
			entries, err := os.ReadDir(saveDir)
			if err != nil {
				t.Fatalf("expected save dir to exist, got %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected no stray files, got %#v", entries)
			}
		})
	}
}

func TestIEEEACMRetrievalSkeletons(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		sourceID string
		envVar   string
	}{
		{name: "ieee", sourceID: "ieee", envVar: "PAPER_SEARCH_MCP_IEEE_API_KEY=ieee-key"},
		{name: "acm", sourceID: "acm", envVar: "PAPER_SEARCH_MCP_ACM_API_KEY=acm-key"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var sourcesStdout bytes.Buffer
			var sourcesStderr bytes.Buffer
			exitCode := runWithOptions([]string{"sources"}, &sourcesStdout, &sourcesStderr, runOptions{
				environ:        []string{tc.envVar},
				workingDir:     t.TempDir(),
				repositoryRoot: t.TempDir(),
			})
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, sourcesStdout.String(), sourcesStderr.String())
			}

			var listing struct {
				Sources []sourceRegistryEntry `json:"sources"`
			}
			if err := json.Unmarshal(sourcesStdout.Bytes(), &listing); err != nil {
				t.Fatalf("expected valid json, got %q: %v", sourcesStdout.String(), err)
			}
			entry := findSource(t, listing.Sources, tc.sourceID)
			if !entry.Enabled {
				t.Fatalf("expected %s to be enabled with credential, got %#v", tc.sourceID, entry)
			}
			if entry.Capabilities.Download != "unsupported" || entry.Capabilities.Read != "unsupported" {
				t.Fatalf("expected unsupported retrieval capabilities for %s, got %#v", tc.sourceID, entry)
			}

			for _, operation := range []string{"download", "read"} {
				operation := operation
				t.Run(operation, func(t *testing.T) {
					t.Parallel()

					saveDir := filepath.Join(t.TempDir(), "skeleton")
					if err := os.MkdirAll(saveDir, 0o755); err != nil {
						t.Fatalf("mkdir save dir: %v", err)
					}

					paperJSON := `{"paper_id":"skeleton-1","title":"Gated Skeleton","source":"` + tc.sourceID + `"}`

					var stdout bytes.Buffer
					var stderr bytes.Buffer
					exitCode := runWithOptions([]string{operation, "--source", tc.sourceID, "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
						environ:          []string{tc.envVar},
						workingDir:       t.TempDir(),
						repositoryRoot:   t.TempDir(),
						connectorFactory: connectors.New,
					})
					if exitCode != exitCodeUnsupported {
						t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeUnsupported, exitCode, stdout.String(), stderr.String())
					}

					payload := decodeRetrievalResponse(t, stdout.Bytes())
					if payload.State != "unsupported" {
						t.Fatalf("expected unsupported skeleton retrieval, got %#v", payload)
					}
					if !strings.Contains(strings.ToLower(payload.Message), "skeleton") || !strings.Contains(strings.ToLower(payload.Message), "not implemented") {
						t.Fatalf("expected skeleton explanation, got %#v", payload)
					}

					entries, err := os.ReadDir(saveDir)
					if err != nil {
						t.Fatalf("expected save dir to exist, got %v", err)
					}
					if len(entries) != 0 {
						t.Fatalf("expected no stray files, got %#v", entries)
					}
				})
			}
		})
	}
}

func TestRepositoryFallbackWins(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repository.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Repository fallback text"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	saveDir := filepath.Join(t.TempDir(), "repository-fallback")
	paperJSON := `{"paper_id":"1234.5678","title":"Repository Winner","doi":"10.1000/repository-wins","source":"arxiv"}`

	connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
		switch id {
		case "arxiv":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Download: sources.CapabilitySupported, Read: sources.CapabilitySupported}},
				DownloadResult:  &sources.RetrievalResult{State: sources.RetrievalStateNotFound, Message: "primary failed"},
			}), nil
		case "openaire":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "openaire", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				SearchResults: []paper.Paper{{
					PaperID: "repo-1",
					Title:   "Repository Winner",
					PDFURL:  server.URL + "/repository.pdf",
					Source:  "openaire",
				}},
			}), nil
		case "core", "europepmc", "pmc", "unpaywall":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
			}), nil
		default:
			return connectors.New(id, cfg)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--fallback", "--allow-scihub", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectorFactory,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "downloaded" || payload.WinningStage != "repositories" {
		t.Fatalf("expected repository fallback success, got %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected saved path inside %q, got %#v", saveDir, payload)
	}
	if stages := attemptStages(payload.Attempts); !slices.Equal(stages, []string{"primary", "repositories"}) {
		t.Fatalf("expected repository win to short-circuit later stages, got %#v", payload.Attempts)
	}
	if repositoryAttempt := decodeAttempt(t, payload.Attempts[1]); repositoryAttempt.Source != "openaire" || repositoryAttempt.Path == "" {
		t.Fatalf("expected repository attempt details, got %#v", repositoryAttempt)
	}
}

func TestUnpaywallAfterRepositoryMiss(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unpaywall.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Unpaywall fallback text"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	saveDir := filepath.Join(t.TempDir(), "unpaywall-fallback")
	paperJSON := `{"paper_id":"1234.5678","title":"Unpaywall Winner","doi":"10.1000/unpaywall-wins","source":"arxiv"}`

	connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
		switch id {
		case "arxiv":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Download: sources.CapabilitySupported}},
				DownloadResult:  &sources.RetrievalResult{State: sources.RetrievalStateNotFound, Message: "primary failed"},
			}), nil
		case "openaire", "core", "europepmc", "pmc":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
			}), nil
		case "unpaywall":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "unpaywall", Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
				SearchResults: []paper.Paper{{
					PaperID: "10.1000/unpaywall-wins",
					Title:   "Unpaywall Winner",
					DOI:     "10.1000/unpaywall-wins",
					PDFURL:  server.URL + "/unpaywall.pdf",
					Source:  "unpaywall",
				}},
			}), nil
		default:
			return connectors.New(id, cfg)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--fallback", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
		environ:          []string{"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=tester@example.com"},
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectorFactory,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "downloaded" || payload.WinningStage != "unpaywall" {
		t.Fatalf("expected unpaywall fallback success, got %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected path inside save dir %q, got %#v", saveDir, payload)
	}
	if stages := attemptStages(payload.Attempts); !slices.Equal(stages, []string{"primary", "repositories", "repositories", "repositories", "repositories", "unpaywall"}) {
		t.Fatalf("expected repository attempts before unpaywall success, got %#v", payload.Attempts)
	}
}

func TestSciHubDisabledStopsChain(t *testing.T) {
	t.Parallel()

	saveDir := filepath.Join(t.TempDir(), "disabled-scihub")
	paperJSON := `{"paper_id":"1234.5678","title":"OA Failure","doi":"10.1000/oa-failure","source":"arxiv"}`

	connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
		switch id {
		case "arxiv":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Download: sources.CapabilitySupported}},
				DownloadResult:  &sources.RetrievalResult{State: sources.RetrievalStateNotFound, Message: "primary failed"},
			}), nil
		case "openaire", "core", "europepmc", "pmc", "unpaywall":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
			}), nil
		default:
			return connectors.New(id, cfg)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--fallback", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
		environ:          []string{"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=tester@example.com"},
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectorFactory,
	})
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeRuntimeError, exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "failed" || payload.WinningStage != "" {
		t.Fatalf("expected terminal OA-chain failure, got %#v", payload)
	}
	if !strings.Contains(strings.ToLower(payload.Message), "oa fallback chain") {
		t.Fatalf("expected OA-chain failure message, got %#v", payload)
	}
	for _, attempt := range payload.Attempts {
		if decodeAttempt(t, attempt).Stage == "scihub" {
			t.Fatalf("expected no scihub attempt when disabled, got %#v", payload.Attempts)
		}
	}
}

func TestMissingDOIReported(t *testing.T) {
	t.Parallel()

	saveDir := filepath.Join(t.TempDir(), "missing-doi")
	paperJSON := `{"paper_id":"1234.5678","title":"Missing DOI","source":"arxiv"}`

	connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
		switch id {
		case "arxiv":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Download: sources.CapabilitySupported}},
				DownloadResult:  &sources.RetrievalResult{State: sources.RetrievalStateNotFound, Message: "primary failed"},
			}), nil
		case "openaire", "core", "europepmc", "pmc":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
			}), nil
		default:
			return connectors.New(id, cfg)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--fallback", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON}, &stdout, &stderr, runOptions{
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectorFactory,
	})
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeRuntimeError, exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	found := false
	for _, attempt := range payload.Attempts {
		item := decodeAttempt(t, attempt)
		if item.Stage == "unpaywall" {
			found = true
			if item.State != "skipped" || !strings.Contains(strings.ToLower(item.Message), "doi not provided") {
				t.Fatalf("expected missing DOI detail on unpaywall attempt, got %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("expected unpaywall attempt details, got %#v", payload.Attempts)
	}
}

func TestSciHubDirectRetrieval(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/10.1000/test":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><embed type="application/pdf" src="/pdfs/test.pdf"></body></html>`))
		case "/pdfs/test.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("SciHub direct retrieval"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	saveDir := filepath.Join(t.TempDir(), "scihub")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--source", "scihub", "--paper-id", "10.1000/test", "--doi", "10.1000/test", "--save-dir", saveDir, "--scihub-base-url", server.URL}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "downloaded" || payload.WinningStage != "scihub" {
		t.Fatalf("expected scihub direct success, got %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected scihub path inside %q, got %#v", saveDir, payload)
	}
	if stages := attemptStages(payload.Attempts); !slices.Equal(stages, []string{"scihub"}) {
		t.Fatalf("expected only scihub attempt, got %#v", payload.Attempts)
	}
}

func TestSciHubDirectTransportFailureNormalized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	baseURL := server.URL
	server.Close()

	saveDir := filepath.Join(t.TempDir(), "scihub-failure")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--source", "scihub", "--paper-id", "10.1000/test", "--doi", "10.1000/test", "--save-dir", saveDir, "--scihub-base-url", baseURL}, &stdout, &stderr, runOptions{
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeRuntimeError, exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "failed" || payload.WinningStage != "" || payload.Path != "" {
		t.Fatalf("expected normalized failed scihub response without winning stage, got %#v", payload)
	}
	if stages := attemptStages(payload.Attempts); !slices.Equal(stages, []string{"scihub"}) {
		t.Fatalf("expected only scihub attempt, got %#v", payload.Attempts)
	}
	attempt := decodeAttempt(t, payload.Attempts[0])
	if attempt.State != "failed" || attempt.Path != "" || !strings.Contains(strings.ToLower(attempt.Message), "failed") {
		t.Fatalf("expected explicit failed scihub attempt, got %#v", attempt)
	}
}

func TestSciHubFallbackTransportFailureNormalized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	baseURL := server.URL
	server.Close()

	saveDir := filepath.Join(t.TempDir(), "scihub-fallback-failure")
	paperJSON := `{"paper_id":"1234.5678","title":"SciHub Failure","doi":"10.1000/scihub-failure","source":"arxiv"}`

	connectorFactory := func(id string, cfg config.Config) (sources.Connector, error) {
		switch id {
		case "arxiv":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: "arxiv", Enabled: true, Capabilities: sources.Capabilities{Download: sources.CapabilitySupported}},
				DownloadResult:  &sources.RetrievalResult{State: sources.RetrievalStateNotFound, Message: "primary failed"},
			}), nil
		case "openaire", "core", "europepmc", "pmc", "unpaywall":
			return sources.NewStubConnector(sources.StubConnector{
				DescriptorValue: sources.Descriptor{ID: id, Enabled: true, Capabilities: sources.Capabilities{Search: sources.CapabilitySupported}},
			}), nil
		default:
			return connectors.New(id, cfg)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"download", "--fallback", "--allow-scihub", "--source", "arxiv", "--save-dir", saveDir, "--paper-json", paperJSON, "--scihub-base-url", baseURL}, &stdout, &stderr, runOptions{
		environ:          []string{"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=tester@example.com"},
		workingDir:       t.TempDir(),
		repositoryRoot:   t.TempDir(),
		connectorFactory: connectorFactory,
	})
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code %d, got %d with stdout=%q stderr=%q", exitCodeRuntimeError, exitCode, stdout.String(), stderr.String())
	}

	payload := decodeRetrievalResponse(t, stdout.Bytes())
	if payload.State != "failed" || payload.WinningStage != "" {
		t.Fatalf("expected terminal failed fallback response, got %#v", payload)
	}
	lastAttempt := decodeAttempt(t, payload.Attempts[len(payload.Attempts)-1])
	if lastAttempt.Stage != "scihub" || lastAttempt.State != "failed" || lastAttempt.Path != "" || !strings.Contains(strings.ToLower(lastAttempt.Message), "failed") {
		t.Fatalf("expected explicit failed scihub fallback attempt, got %#v", lastAttempt)
	}
}

type retrievalAttemptPayload struct {
	Stage   string `json:"stage"`
	Source  string `json:"source"`
	State   string `json:"state"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

func decodeAttempt(t *testing.T, value any) retrievalAttemptPayload {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal attempt: %v", err)
	}
	var payload retrievalAttemptPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal attempt: %v", err)
	}
	return payload
}

func attemptStages(attempts []any) []string {
	stages := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		data, err := json.Marshal(attempt)
		if err != nil {
			continue
		}
		var payload retrievalAttemptPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}
		stages = append(stages, payload.Stage)
	}
	return stages
}

func decodeRetrievalResponse(t *testing.T, data []byte) retrievalCommandResponse {
	t.Helper()

	var payload retrievalCommandResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", string(data), err)
	}
	return payload
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func minimalPDF(text string) []byte {
	stream := "BT /F1 12 Tf 72 720 Td (" + escapePDFText(text) + ") Tj ET"
	return []byte("%PDF-1.4\n1 0 obj\n<< /Length " + strconv.Itoa(len(stream)) + " >>\nstream\n" + stream + "\nendstream\nendobj\ntrailer\n<<>>\n%%EOF")
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(text)
}
