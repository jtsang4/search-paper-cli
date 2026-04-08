package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/connectors"
)

type retrievalCommandResponse struct {
	Status    string `json:"status"`
	Operation string `json:"operation"`
	State     string `json:"state"`
	Source    string `json:"source"`
	PaperID   string `json:"paper_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Message   string `json:"message"`
	Attempts  []any  `json:"attempts"`
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

func decodeRetrievalResponse(t *testing.T, data []byte) retrievalCommandResponse {
	t.Helper()

	var payload retrievalCommandResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", string(data), err)
	}
	return payload
}

func minimalPDF(text string) []byte {
	stream := "BT /F1 12 Tf 72 720 Td (" + escapePDFText(text) + ") Tj ET"
	return []byte("%PDF-1.4\n1 0 obj\n<< /Length " + strconv.Itoa(len(stream)) + " >>\nstream\n" + stream + "\nendstream\nendobj\ntrailer\n<<>>\n%%EOF")
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(text)
}
