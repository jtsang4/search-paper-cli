package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactLayoutNamesAndPaths(t *testing.T) {
	t.Parallel()

	layout := ArtifactLayout("/tmp/search-paper-cli")
	if layout.DistDir != "/tmp/search-paper-cli/dist" {
		t.Fatalf("expected dist dir path, got %q", layout.DistDir)
	}
	if layout.ArtifactDir != "/tmp/search-paper-cli/dist/"+ArtifactDirName {
		t.Fatalf("expected artifact dir path, got %q", layout.ArtifactDir)
	}
	if layout.BinaryPath != layout.ArtifactDir+"/"+BinaryName {
		t.Fatalf("expected binary path, got %q", layout.BinaryPath)
	}
	if layout.ArchivePath != layout.DistDir+"/"+ArchiveName {
		t.Fatalf("expected archive path, got %q", layout.ArchivePath)
	}
}

func TestBuiltArtifactPreservesEnvLoadingRules(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)

	outsideDir := t.TempDir()
	cleanPayload := runSourcesArtifact(t, binaryPath, outsideDir, nil)
	assertSourceEnabled(t, cleanPayload.Sources, "ieee", false)
	assertSourceEnabled(t, cleanPayload.Sources, "acm", false)

	writeFile(t, filepath.Join(outsideDir, ".env"), "PAPER_SEARCH_MCP_IEEE_API_KEY=ieee-from-cwd\n")
	cwdPayload := runSourcesArtifact(t, binaryPath, outsideDir, nil)
	assertSourceEnabled(t, cwdPayload.Sources, "ieee", true)
	assertSourceEnabled(t, cwdPayload.Sources, "acm", false)

	explicitEnvFile := filepath.Join(t.TempDir(), "explicit.env")
	writeFile(t, explicitEnvFile, "PAPER_SEARCH_MCP_ACM_API_KEY=acm-from-explicit\n")
	explicitPayload := runSourcesArtifact(t, binaryPath, outsideDir, []string{"PAPER_SEARCH_MCP_ENV_FILE=" + explicitEnvFile})
	assertSourceEnabled(t, explicitPayload.Sources, "ieee", false)
	assertSourceEnabled(t, explicitPayload.Sources, "acm", true)

	fakeRepoRoot := t.TempDir()
	writeFile(t, filepath.Join(fakeRepoRoot, "go.mod"), "module example.com/fake\n\ngo 1.26\n")
	writeFile(t, filepath.Join(fakeRepoRoot, ".factory", "services.yaml"), "commands: {}\nservices: {}\n")
	writeFile(t, filepath.Join(fakeRepoRoot, ".env"), "PAPER_SEARCH_MCP_IEEE_API_KEY=ieee-from-repo-root\n")
	nestedDir := filepath.Join(fakeRepoRoot, "nested", "workspace")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	repoFallbackPayload := runSourcesArtifact(t, binaryPath, nestedDir, nil)
	assertSourceEnabled(t, repoFallbackPayload.Sources, "ieee", true)
	assertSourceEnabled(t, repoFallbackPayload.Sources, "acm", false)
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
	cmd.Env = append(filteredEnv(), "GOOS="+TargetOS, "GOARCH="+TargetArch, "CGO_ENABLED=0")
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
	cmd.Env = append(filteredEnv(), extraEnv...)

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
		case strings.HasPrefix(entry, "PAPER_SEARCH_MCP_"):
		case strings.HasPrefix(entry, "UNPAYWALL_EMAIL="):
		case strings.HasPrefix(entry, "CORE_API_KEY="):
		default:
			filtered = append(filtered, entry)
		}
	}
	return filtered
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
