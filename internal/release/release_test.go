package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArtifactLayoutNamesAndPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		target          Target
		wantArtifactDir string
		wantBinary      string
		wantArchive     string
	}{
		{
			name:            "linux amd64",
			target:          Target{OS: "linux", Arch: "amd64"},
			wantArtifactDir: "/tmp/search-paper-cli/dist/search-paper-cli_linux_amd64",
			wantBinary:      "/tmp/search-paper-cli/dist/search-paper-cli_linux_amd64/search-paper-cli",
			wantArchive:     "/tmp/search-paper-cli/dist/search-paper-cli_linux_amd64.tar.gz",
		},
		{
			name:            "darwin arm64",
			target:          Target{OS: "darwin", Arch: "arm64"},
			wantArtifactDir: "/tmp/search-paper-cli/dist/search-paper-cli_darwin_arm64",
			wantBinary:      "/tmp/search-paper-cli/dist/search-paper-cli_darwin_arm64/search-paper-cli",
			wantArchive:     "/tmp/search-paper-cli/dist/search-paper-cli_darwin_arm64.tar.gz",
		},
		{
			name:            "windows amd64",
			target:          Target{OS: "windows", Arch: "amd64"},
			wantArtifactDir: "/tmp/search-paper-cli/dist/search-paper-cli_windows_amd64",
			wantBinary:      "/tmp/search-paper-cli/dist/search-paper-cli_windows_amd64/search-paper-cli.exe",
			wantArchive:     "/tmp/search-paper-cli/dist/search-paper-cli_windows_amd64.zip",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			layout := ArtifactLayout("/tmp/search-paper-cli", tc.target)
			if layout.DistDir != "/tmp/search-paper-cli/dist" {
				t.Fatalf("expected dist dir path, got %q", layout.DistDir)
			}
			if layout.ArtifactDir != tc.wantArtifactDir {
				t.Fatalf("expected artifact dir path %q, got %q", tc.wantArtifactDir, layout.ArtifactDir)
			}
			if layout.BinaryPath != tc.wantBinary {
				t.Fatalf("expected binary path %q, got %q", tc.wantBinary, layout.BinaryPath)
			}
			if layout.ArchivePath != tc.wantArchive {
				t.Fatalf("expected archive path %q, got %q", tc.wantArchive, layout.ArchivePath)
			}
		})
	}
}

func TestBuiltArtifactPreservesEnvLoadingRules(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)

	outsideDir := t.TempDir()
	cleanPayload := runSourcesArtifact(t, binaryPath, outsideDir, nil)
	assertSourceEnabled(t, cleanPayload.Sources, "ieee", false)
	assertSourceEnabled(t, cleanPayload.Sources, "acm", false)

	writeGlobalConfig(t, outsideDir, "config.yml", "acm_api_key: acm-from-yml\n")
	ymlPayload := runSourcesArtifact(t, binaryPath, outsideDir, nil)
	assertSourceEnabled(t, ymlPayload.Sources, "ieee", false)
	assertSourceEnabled(t, ymlPayload.Sources, "acm", true)

	writeGlobalConfig(t, outsideDir, "config.yaml", "ieee_api_key: ieee-from-yaml\n")
	yamlWinsPayload := runSourcesArtifact(t, binaryPath, outsideDir, nil)
	assertSourceEnabled(t, yamlWinsPayload.Sources, "ieee", true)
	assertSourceEnabled(t, yamlWinsPayload.Sources, "acm", false)

	writeFile(t, filepath.Join(outsideDir, ".env"), "SEARCH_PAPER_ACM_API_KEY=acm-from-cwd-env\n")
	explicitEnvFile := filepath.Join(t.TempDir(), "explicit.env")
	writeFile(t, explicitEnvFile, "SEARCH_PAPER_ACM_API_KEY=acm-from-explicit-env\n")
	legacyIgnoredPayload := runSourcesArtifact(t, binaryPath, outsideDir, []string{"SEARCH_PAPER_ENV_FILE=" + explicitEnvFile})
	assertSourceEnabled(t, legacyIgnoredPayload.Sources, "ieee", true)
	assertSourceEnabled(t, legacyIgnoredPayload.Sources, "acm", false)
}

func TestBuiltArtifactPreservesCapabilityConsistency(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	workDir := t.TempDir()

	sourcesPayload := runSourcesArtifact(t, binaryPath, workDir, nil)
	assertSourceCapability(t, sourcesPayload.Sources, "crossref", "informational", "informational")
	assertSourceCapability(t, sourcesPayload.Sources, "dblp", "unsupported", "unsupported")

	crossrefResponse := runRetrievalArtifact(t, binaryPath, workDir, "download", "crossref", `{"paper_id":"10.1000/crossref","title":"Crossref Metadata","source":"crossref"}`)
	if crossrefResponse.ExitCode != 3 {
		t.Fatalf("expected crossref download exit code 3, got %d with stderr=%q", crossrefResponse.ExitCode, crossrefResponse.Stderr)
	}
	if crossrefResponse.Payload.State != "informational" {
		t.Fatalf("expected informational crossref retrieval, got %#v", crossrefResponse.Payload)
	}

	dblpResponse := runRetrievalArtifact(t, binaryPath, workDir, "download", "dblp", `{"paper_id":"dblp-1","title":"DBLP Metadata","source":"dblp"}`)
	if dblpResponse.ExitCode != 3 {
		t.Fatalf("expected dblp download exit code 3, got %d with stderr=%q", dblpResponse.ExitCode, dblpResponse.Stderr)
	}
	if dblpResponse.Payload.State != "unsupported" {
		t.Fatalf("expected unsupported dblp retrieval, got %#v", dblpResponse.Payload)
	}
}

func TestArtifactSearchToNativeRetrieval(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	artifactDir := t.TempDir()
	workDir := t.TempDir()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/arxiv" && r.URL.Query().Get("search_query") == "all:artifact native flow":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/1234.5678v1</id>
    <title>Artifact Native Retrieval</title>
    <summary>Native retrieval summary</summary>
    <published>2024-04-08T00:00:00Z</published>
    <author><name>Alice Example</name></author>
    <link rel="alternate" type="text/html" href="http://arxiv.org/abs/1234.5678v1"></link>
    <link title="pdf" type="application/pdf" href="` + server.URL + `/pdf/1234.5678v1.pdf"></link>
  </entry>
</feed>`))
		case r.URL.Path == "/pdf/1234.5678v1.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Artifact native retrieval PDF"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	searchPayload := runSearchArtifact(t, binaryPath, artifactDir, []string{
		"SEARCH_PAPER_ARXIV_BASE_URL=" + server.URL + "/arxiv",
	}, "artifact native flow")
	if len(searchPayload.Papers) != 1 {
		t.Fatalf("expected one search paper, got %#v", searchPayload)
	}
	paperJSON := marshalJSON(t, searchPayload.Papers[0])
	saveDir := filepath.Join(workDir, "native-downloads")
	response := runArtifactCommand(t, binaryPath, artifactDir, nil, "download", "--save-dir", saveDir, "--paper-json", paperJSON)
	if response.ExitCode != 0 {
		t.Fatalf("expected native retrieval exit code 0, got %d stdout=%q stderr=%q", response.ExitCode, response.Stdout, response.Stderr)
	}

	var payload artifactRetrievalFlowPayload
	if err := json.Unmarshal([]byte(response.Stdout), &payload); err != nil {
		t.Fatalf("expected valid retrieval payload, got %q: %v", response.Stdout, err)
	}
	if payload.State != "downloaded" || payload.Source != "arxiv" || payload.PaperID != "1234.5678v1" {
		t.Fatalf("unexpected native retrieval payload %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected saved path inside %q, got %#v", saveDir, payload)
	}
	if len(payload.Attempts) != 0 {
		t.Fatalf("expected native retrieval attempts to stay empty, got %#v", payload.Attempts)
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("expected saved file at %q: %v", payload.Path, err)
	}
}

func TestArtifactSearchToFallbackRetrieval(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	artifactDir := t.TempDir()
	workDir := t.TempDir()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/arxiv" && r.URL.Query().Get("search_query") == "all:artifact fallback flow":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/9999.0001v1</id>
    <title>Artifact Fallback Retrieval</title>
    <summary>Contains doi 10.1000/artifact-fallback-1.</summary>
    <published>2024-04-08T00:00:00Z</published>
    <author><name>Alice Example</name></author>
  </entry>
</feed>`))
		case r.URL.Path == "/openaire/search/researchProducts":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><response><results></results></response>`))
		case r.URL.Path == "/openaire/search/publications":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"results":{"result":[]}}}`))
		case r.URL.Path == "/core":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/europepmc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resultList":{"result":[]}}`))
		case r.URL.Path == "/pmc/esearch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><eSearchResult><IdList></IdList></eSearchResult>`))
		case r.URL.Path == "/unpaywall/10.1000/artifact-fallback-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"doi":"10.1000/artifact-fallback-1","title":"Artifact Fallback Retrieval","published_date":"2024-04-08","best_oa_location":{"url":"https://example.test/paper","url_for_pdf":"` + server.URL + `/files/unpaywall.pdf"},"z_authors":[{"given":"Alice","family":"Example"}]}`))
		case r.URL.Path == "/files/unpaywall.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Artifact fallback retrieval PDF"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	searchPayload := runSearchArtifact(t, binaryPath, artifactDir, []string{
		"SEARCH_PAPER_ARXIV_BASE_URL=" + server.URL + "/arxiv",
	}, "artifact fallback flow")
	if len(searchPayload.Papers) != 1 {
		t.Fatalf("expected one search paper, got %#v", searchPayload)
	}
	paperJSON := marshalJSON(t, searchPayload.Papers[0])
	saveDir := filepath.Join(workDir, "fallback-downloads")
	response := runArtifactCommand(t, binaryPath, artifactDir, []string{
		"SEARCH_PAPER_OPENAIRE_BASE_URL=" + server.URL + "/openaire/search/researchProducts",
		"SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL=" + server.URL + "/openaire/search/publications",
		"SEARCH_PAPER_CORE_BASE_URL=" + server.URL + "/core",
		"SEARCH_PAPER_EUROPEPMC_BASE_URL=" + server.URL + "/europepmc",
		"SEARCH_PAPER_PMC_SEARCH_URL=" + server.URL + "/pmc/esearch.fcgi",
		"SEARCH_PAPER_PMC_SUMMARY_URL=" + server.URL + "/pmc/esummary.fcgi",
		"SEARCH_PAPER_UNPAYWALL_EMAIL=tester@example.com",
		"SEARCH_PAPER_UNPAYWALL_BASE_URL=" + server.URL + "/unpaywall",
	}, "download", "--fallback", "--save-dir", saveDir, "--paper-json", paperJSON)
	if response.ExitCode != 0 {
		t.Fatalf("expected fallback retrieval exit code 0, got %d stdout=%q stderr=%q", response.ExitCode, response.Stdout, response.Stderr)
	}

	var payload artifactRetrievalFlowPayload
	if err := json.Unmarshal([]byte(response.Stdout), &payload); err != nil {
		t.Fatalf("expected valid retrieval payload, got %q: %v", response.Stdout, err)
	}
	if payload.State != "downloaded" || payload.WinningStage != "unpaywall" {
		t.Fatalf("expected unpaywall fallback success, got %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected saved path inside %q, got %#v", saveDir, payload)
	}
	if len(payload.Attempts) != 6 {
		t.Fatalf("expected ordered fallback attempts, got %#v", payload.Attempts)
	}
	if payload.Attempts[0].Stage != "primary" || payload.Attempts[len(payload.Attempts)-1].Stage != "unpaywall" {
		t.Fatalf("expected primary-to-unpaywall attempts, got %#v", payload.Attempts)
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("expected saved file at %q: %v", payload.Path, err)
	}
}

func TestArtifactOutsideRepoFlow(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	outsideDir := t.TempDir()
	releaseDir := filepath.Join(outsideDir, "bin")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", releaseDir, err)
	}
	releaseBinary := filepath.Join(releaseDir, BinaryName)
	copyFile(t, binaryPath, releaseBinary)

	runHelp := runArtifactCommand(t, releaseBinary, outsideDir, nil, "--help")
	if runHelp.ExitCode != 0 {
		t.Fatalf("expected outside-repo help exit code 0, got %d stdout=%q stderr=%q", runHelp.ExitCode, runHelp.Stdout, runHelp.Stderr)
	}
	if !strings.Contains(runHelp.Stdout, "search") || !strings.Contains(runHelp.Stdout, "get") {
		t.Fatalf("expected help output to include core commands, got %q", runHelp.Stdout)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/arxiv" && r.URL.Query().Get("search_query") == "all:artifact outside flow":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/4321.0001v1</id>
    <title>Artifact Outside Repo Flow</title>
    <summary>Outside repo artifact flow with doi 10.1000/artifact-outside-1.</summary>
    <published>2024-04-08T00:00:00Z</published>
    <author><name>Alice Example</name></author>
  </entry>
</feed>`))
		case r.URL.Path == "/openaire/search/researchProducts":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><response><results></results></response>`))
		case r.URL.Path == "/openaire/search/publications":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"results":{"result":[]}}}`))
		case r.URL.Path == "/core":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/europepmc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resultList":{"result":[]}}`))
		case r.URL.Path == "/pmc/esearch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><eSearchResult><IdList></IdList></eSearchResult>`))
		case r.URL.Path == "/unpaywall/10.1000/artifact-outside-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"doi":"10.1000/artifact-outside-1","title":"Artifact Outside Repo Flow","published_date":"2024-04-08","best_oa_location":{"url":"https://example.test/outside","url_for_pdf":"` + server.URL + `/files/outside.pdf"},"z_authors":[{"given":"Alice","family":"Example"}]}`))
		case r.URL.Path == "/files/outside.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Artifact outside repo flow PDF"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeGlobalConfig(t, outsideDir, "config.yaml", strings.Join([]string{
		"arxiv_base_url: " + server.URL + "/arxiv",
		"openaire_base_url: " + server.URL + "/openaire/search/researchProducts",
		"openaire_legacy_base_url: " + server.URL + "/openaire/search/publications",
		"core_base_url: " + server.URL + "/core",
		"europepmc_base_url: " + server.URL + "/europepmc",
		"pmc_search_url: " + server.URL + "/pmc/esearch.fcgi",
		"pmc_summary_url: " + server.URL + "/pmc/esummary.fcgi",
		"unpaywall_email: tester@example.com",
		"unpaywall_base_url: " + server.URL + "/unpaywall",
		"",
	}, "\n"))

	searchResponse := runArtifactCommand(t, releaseBinary, outsideDir, nil, "search", "--source", "arxiv", "artifact outside flow")
	if searchResponse.ExitCode != 0 {
		t.Fatalf("expected outside-repo search exit code 0, got %d stdout=%q stderr=%q", searchResponse.ExitCode, searchResponse.Stdout, searchResponse.Stderr)
	}
	var searchPayload artifactSearchPayload
	if err := json.Unmarshal([]byte(searchResponse.Stdout), &searchPayload); err != nil {
		t.Fatalf("expected valid search payload, got %q: %v", searchResponse.Stdout, err)
	}
	if len(searchPayload.Papers) != 1 {
		t.Fatalf("expected one outside-repo search paper, got %#v", searchPayload)
	}

	saveDir := filepath.Join(outsideDir, "downloads")
	paperJSON := marshalJSON(t, searchPayload.Papers[0])
	retrievalResponse := runArtifactCommand(t, releaseBinary, outsideDir, nil, "download", "--fallback", "--save-dir", saveDir, "--paper-json", paperJSON)
	if retrievalResponse.ExitCode != 0 {
		t.Fatalf("expected outside-repo fallback exit code 0, got %d stdout=%q stderr=%q", retrievalResponse.ExitCode, retrievalResponse.Stdout, retrievalResponse.Stderr)
	}
	var payload artifactRetrievalFlowPayload
	if err := json.Unmarshal([]byte(retrievalResponse.Stdout), &payload); err != nil {
		t.Fatalf("expected valid retrieval payload, got %q: %v", retrievalResponse.Stdout, err)
	}
	if payload.State != "downloaded" || payload.WinningStage != "unpaywall" {
		t.Fatalf("expected outside-repo fallback success, got %#v", payload)
	}
	if payload.Path == "" || !strings.HasPrefix(payload.Path, saveDir+string(os.PathSeparator)) {
		t.Fatalf("expected saved path inside %q, got %#v", saveDir, payload)
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("expected saved file at %q: %v", payload.Path, err)
	}
}

func TestArtifactOutsideRepoMergedConfigAcrossCommands(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	outsideDir := t.TempDir()
	releaseDir := filepath.Join(outsideDir, "bin")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", releaseDir, err)
	}
	releaseBinary := filepath.Join(releaseDir, BinaryName)
	copyFile(t, binaryPath, releaseBinary)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/arxiv" && r.URL.Query().Get("search_query") == "all:artifact merged flow":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/7000.0001v1</id>
    <title>Artifact Merged Config Flow</title>
    <summary>Contains doi 10.1000/artifact-merged-1.</summary>
    <published>2024-04-08T00:00:00Z</published>
    <author><name>Alice Example</name></author>
    <link title="pdf" type="application/pdf" href="` + server.URL + `/files/source.pdf"></link>
  </entry>
</feed>`))
		case r.URL.Path == "/files/source.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Artifact merged source PDF"))
		case r.URL.Path == "/openaire/search/researchProducts":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><response><results></results></response>`))
		case r.URL.Path == "/openaire/search/publications":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"results":{"result":[]}}}`))
		case r.URL.Path == "/core":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/europepmc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resultList":{"result":[]}}`))
		case r.URL.Path == "/pmc/esearch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><eSearchResult><IdList></IdList></eSearchResult>`))
		case r.URL.Path == "/unpaywall/10.1000/artifact-merged-1":
			if got := r.URL.Query().Get("email"); got != "merged@example.com" {
				t.Fatalf("expected merged Unpaywall email, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"doi":"10.1000/artifact-merged-1","title":"Artifact Merged Config Flow","published_date":"2024-04-08","best_oa_location":{"url":"https://example.test/merged","url_for_pdf":"` + server.URL + `/files/unpaywall.pdf"},"z_authors":[{"given":"Alice","family":"Example"}]}`))
		case r.URL.Path == "/files/unpaywall.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(minimalPDF("Artifact merged fallback PDF"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeGlobalConfig(t, outsideDir, "config.yaml", strings.Join([]string{
		"ieee_api_key: ieee-from-global",
		"arxiv_base_url: " + server.URL + "/arxiv",
		"openaire_base_url: " + server.URL + "/openaire/search/researchProducts",
		"openaire_legacy_base_url: " + server.URL + "/openaire/search/publications",
		"core_base_url: " + server.URL + "/core",
		"europepmc_base_url: " + server.URL + "/europepmc",
		"pmc_search_url: " + server.URL + "/pmc/esearch.fcgi",
		"pmc_summary_url: " + server.URL + "/pmc/esummary.fcgi",
		"unpaywall_email: merged@example.com",
		"unpaywall_base_url: " + server.URL + "/unpaywall",
		"",
	}, "\n"))

	sourcesResponse := runArtifactCommand(t, releaseBinary, outsideDir, []string{
		"SEARCH_PAPER_ACM_API_KEY=acm-from-env",
	}, "sources")
	if sourcesResponse.ExitCode != 0 {
		t.Fatalf("expected outside-repo sources exit code 0, got %d stdout=%q stderr=%q", sourcesResponse.ExitCode, sourcesResponse.Stdout, sourcesResponse.Stderr)
	}
	var sourcesPayload artifactSourcesPayload
	if err := json.Unmarshal([]byte(sourcesResponse.Stdout), &sourcesPayload); err != nil {
		t.Fatalf("expected valid sources payload, got %q: %v", sourcesResponse.Stdout, err)
	}
	assertSourceEnabled(t, sourcesPayload.Sources, "acm", true)
	assertSourceEnabled(t, sourcesPayload.Sources, "ieee", true)

	searchResponse := runArtifactCommand(t, releaseBinary, outsideDir, nil, "search", "--source", "arxiv", "artifact merged flow")
	if searchResponse.ExitCode != 0 {
		t.Fatalf("expected outside-repo search exit code 0, got %d stdout=%q stderr=%q", searchResponse.ExitCode, searchResponse.Stdout, searchResponse.Stderr)
	}
	var searchPayload artifactSearchPayload
	if err := json.Unmarshal([]byte(searchResponse.Stdout), &searchPayload); err != nil {
		t.Fatalf("expected valid search payload, got %q: %v", searchResponse.Stdout, err)
	}
	if len(searchPayload.Papers) != 1 {
		t.Fatalf("expected one merged-config search paper, got %#v", searchPayload)
	}
	paperJSON := marshalJSON(t, searchPayload.Papers[0])

	pdfSaveDir := filepath.Join(outsideDir, "pdf-downloads")
	pdfResponse := runArtifactCommand(t, releaseBinary, outsideDir, nil, "get", "--as", "pdf", "--fallback", "--save-dir", pdfSaveDir, "--paper-json", paperJSON)
	if pdfResponse.ExitCode != 0 {
		t.Fatalf("expected merged-config pdf retrieval exit code 0, got %d stdout=%q stderr=%q", pdfResponse.ExitCode, pdfResponse.Stdout, pdfResponse.Stderr)
	}
	var pdfPayload artifactRetrievalFlowPayload
	if err := json.Unmarshal([]byte(pdfResponse.Stdout), &pdfPayload); err != nil {
		t.Fatalf("expected valid pdf retrieval payload, got %q: %v", pdfResponse.Stdout, err)
	}
	if pdfPayload.State != "downloaded" {
		t.Fatalf("expected downloaded pdf payload, got %#v", pdfPayload)
	}
	if pdfPayload.Path == "" || !strings.HasPrefix(pdfPayload.Path, pdfSaveDir+string(os.PathSeparator)) {
		t.Fatalf("expected saved pdf path inside %q, got %#v", pdfSaveDir, pdfPayload)
	}

	textSaveDir := filepath.Join(outsideDir, "text-downloads")
	textResponse := runArtifactCommand(t, releaseBinary, outsideDir, nil, "get", "--as", "text", "--save-dir", textSaveDir, "--paper-json", paperJSON)
	if textResponse.ExitCode != 0 {
		t.Fatalf("expected merged-config text retrieval exit code 0, got %d stdout=%q stderr=%q", textResponse.ExitCode, textResponse.Stdout, textResponse.Stderr)
	}
	if !strings.Contains(textResponse.Stdout, `"target":"text"`) {
		t.Fatalf("expected text retrieval payload to report target text, got %q", textResponse.Stdout)
	}
	if !strings.Contains(textResponse.Stdout, `"status":"ok"`) {
		t.Fatalf("expected text retrieval payload to stay machine-readable, got %q", textResponse.Stdout)
	}
}

type artifactSourcesPayload struct {
	Status  string `json:"status"`
	Sources []struct {
		ID           string `json:"id"`
		Enabled      bool   `json:"enabled"`
		Capabilities struct {
			Download string `json:"download"`
			Read     string `json:"read"`
		} `json:"capabilities"`
	} `json:"sources"`
}

type artifactRetrievalPayload struct {
	Status string `json:"status"`
	State  string `json:"state"`
	Source string `json:"source"`
}

type artifactSearchPayload struct {
	Status string `json:"status"`
	Papers []struct {
		PaperID string `json:"paper_id"`
		Title   string `json:"title"`
		DOI     string `json:"doi"`
		PDFURL  string `json:"pdf_url"`
		URL     string `json:"url"`
		Source  string `json:"source"`
	} `json:"papers"`
}

type artifactRetrievalAttempt struct {
	Stage string `json:"stage"`
	State string `json:"state"`
}

type artifactRetrievalFlowPayload struct {
	Status       string                     `json:"status"`
	State        string                     `json:"state"`
	Source       string                     `json:"source"`
	PaperID      string                     `json:"paper_id"`
	Path         string                     `json:"path"`
	WinningStage string                     `json:"winning_stage"`
	Attempts     []artifactRetrievalAttempt `json:"attempts"`
}

type artifactRetrievalResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Payload  artifactRetrievalPayload
}

func buildArtifactBinary(t *testing.T) string {
	t.Helper()

	repoRoot := findRepoRoot(t)
	binaryPath := filepath.Join(t.TempDir(), BinaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/search-paper-cli")
	cmd.Dir = repoRoot
	cmd.Env = append(filteredEnv(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}

	return binaryPath
}

func runSourcesArtifact(t *testing.T, binaryPath, workingDir string, extraEnv []string) artifactSourcesPayload {
	t.Helper()

	result := runArtifactCommand(t, binaryPath, workingDir, extraEnv, "sources")
	if result.ExitCode != 0 {
		t.Fatalf("expected sources exit code 0, got %d with stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}

	var payload artifactSourcesPayload
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("expected valid sources json, got %q: %v", result.Stdout, err)
	}
	if payload.Status != "ok" {
		t.Fatalf("expected ok sources payload, got %#v", payload)
	}
	return payload
}

func runSearchArtifact(t *testing.T, binaryPath, workingDir string, extraEnv []string, query string) artifactSearchPayload {
	t.Helper()

	result := runArtifactCommand(t, binaryPath, workingDir, extraEnv, "search", "--source", "arxiv", query)
	if result.ExitCode != 0 {
		t.Fatalf("expected search exit code 0, got %d with stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}

	var payload artifactSearchPayload
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("expected valid search json, got %q: %v", result.Stdout, err)
	}
	if payload.Status != "ok" {
		t.Fatalf("expected ok search payload, got %#v", payload)
	}
	return payload
}

func runRetrievalArtifact(t *testing.T, binaryPath, workingDir, operation, sourceID, paperJSON string) artifactRetrievalResult {
	t.Helper()

	result := runArtifactCommand(t, binaryPath, workingDir, nil, operation, "--source", sourceID, "--paper-json", paperJSON)
	var payload artifactRetrievalPayload
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("expected valid retrieval json, got %q: %v", result.Stdout, err)
	}
	return artifactRetrievalResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Payload:  payload,
	}
}

type artifactCommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func runArtifactCommand(t *testing.T, binaryPath, workingDir string, extraEnv []string, args ...string) artifactCommandResult {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workingDir
	cmd.Env = append(filteredEnv(), ensureHomeEnv(workingDir, extraEnv)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("artifact command failed to run: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		}
		exitCode = exitErr.ExitCode()
	}

	return artifactCommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func filteredEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		switch {
		case strings.HasPrefix(entry, "SEARCH_PAPER_"):
		default:
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func ensureHomeEnv(workingDir string, extraEnv []string) []string {
	if hasEnvKey(extraEnv, "HOME") {
		return extraEnv
	}
	return append([]string{"HOME=" + workingDir}, extraEnv...)
}

func hasEnvKey(entries []string, key string) bool {
	prefix := key + "="
	for _, entry := range entries {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	current := workingDir
	for {
		if fileExists(filepath.Join(current, "go.mod")) && fileExists(filepath.Join(current, ".factory", "services.yaml")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("failed to find repository root from %q", workingDir)
		}
		current = parent
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeGlobalConfig(t *testing.T, homeDir, fileName, contents string) {
	t.Helper()

	writeFile(t, filepath.Join(homeDir, ".config", "search-paper-cli", fileName), contents)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", dst, err)
	}
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(data)
}

func minimalPDF(text string) []byte {
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n4 0 obj\n<< /Length 44 >>\nstream\nBT /F1 18 Tf 24 120 Td (" + text + ") Tj ET\nendstream\nendobj\n5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\nxref\n0 6\n0000000000 65535 f \n0000000010 00000 n \n0000000063 00000 n \n0000000122 00000 n \n0000000248 00000 n \n0000000341 00000 n \ntrailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n411\n%%EOF")
}

func assertSourceEnabled(t *testing.T, sources []struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	Capabilities struct {
		Download string `json:"download"`
		Read     string `json:"read"`
	} `json:"capabilities"`
}, id string, want bool) {
	t.Helper()

	for _, source := range sources {
		if source.ID == id {
			if source.Enabled != want {
				t.Fatalf("expected source %q enabled=%t, got %#v", id, want, source)
			}
			return
		}
	}
	t.Fatalf("missing source %q", id)
}

func assertSourceCapability(t *testing.T, sources []struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	Capabilities struct {
		Download string `json:"download"`
		Read     string `json:"read"`
	} `json:"capabilities"`
}, id, wantDownload, wantRead string) {
	t.Helper()

	for _, source := range sources {
		if source.ID == id {
			if source.Capabilities.Download != wantDownload || source.Capabilities.Read != wantRead {
				t.Fatalf("expected source %q capabilities download=%q read=%q, got %#v", id, wantDownload, wantRead, source)
			}
			return
		}
	}
	t.Fatalf("missing source %q", id)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
