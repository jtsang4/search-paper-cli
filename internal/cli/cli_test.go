package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type sourceRegistryEntry struct {
	ID            string `json:"id"`
	Enabled       bool   `json:"enabled"`
	DisableReason string `json:"disable_reason"`
	Capabilities  struct {
		Search   string `json:"search"`
		Download string `json:"download"`
		Read     string `json:"read"`
	} `json:"capabilities"`
}

func TestRootHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{"Usage:", "search", "get", "sources", "version"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help output to contain %q, got %q", want, output)
		}
	}
	for _, unwanted := range []string{"download", "read"} {
		if strings.Contains(output, "  "+unwanted) {
			t.Fatalf("expected root help to hide legacy command %q, got %q", unwanted, output)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var commandStdout bytes.Buffer
	var commandStderr bytes.Buffer
	commandExit := Run([]string{"version"}, &commandStdout, &commandStderr)
	if commandExit != 0 {
		t.Fatalf("expected version command exit 0, got %d", commandExit)
	}

	var flagStdout bytes.Buffer
	var flagStderr bytes.Buffer
	flagExit := Run([]string{"--version"}, &flagStdout, &flagStderr)
	if flagExit != 0 {
		t.Fatalf("expected --version exit 0, got %d", flagExit)
	}

	if commandStdout.String() != flagStdout.String() {
		t.Fatalf("expected matching version output, got command=%q flag=%q", commandStdout.String(), flagStdout.String())
	}

	if strings.TrimSpace(commandStdout.String()) == "" {
		t.Fatalf("expected non-empty version output")
	}

	if commandStderr.Len() != 0 || flagStderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got command=%q flag=%q", commandStderr.String(), flagStderr.String())
	}
}

func TestRootMisuse(t *testing.T) {
	t.Parallel()

	t.Run("no subcommand returns help", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(nil, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		if stdout.Len() != 0 {
			t.Fatalf("expected empty stdout, got %q", stdout.String())
		}

		if !strings.Contains(stderr.String(), "Usage:") {
			t.Fatalf("expected help text on stderr, got %q", stderr.String())
		}

		assertNoPanicText(t, stdout.String(), stderr.String())
	})

	t.Run("unknown command returns structured error", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run([]string{"nope"}, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		assertJSONInvalidUsage(t, stdout.String(), "unknown command")
		assertNoPanicText(t, stdout.String(), stderr.String())
	})

	t.Run("unknown global flag returns structured error", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run([]string{"--bogus"}, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		assertJSONInvalidUsage(t, stdout.String(), "unknown flag")
		assertNoPanicText(t, stdout.String(), stderr.String())
	})
}

func TestWarningsStayOnStderr(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	writeCLIConfig(t, homeDir, "config.yaml", "ieee_api_key: [broken\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"HOME=" + homeDir},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected stdout to remain valid json, got %q: %v", stdout.String(), err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected ok status, got %#v", payload)
	}

	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("expected warning on stderr, got %q", stderr.String())
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	secret := "sentinel-secret-value"
	writeCLIConfig(t, homeDir, "config.yaml", "acm_api_key: \""+secret+"\"\n: bad\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"HOME=" + homeDir, "SEARCH_PAPER_IEEE_API_KEY=" + secret},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	for _, output := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected secret to be redacted from output, got %q", output)
		}
	}
}

func TestSourcesJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithOptions([]string{"sources"}, &stdout, &stderr, runOptions{
		environ:        []string{},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var payload struct {
		Status  string                `json:"status"`
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected ok status, got %#v", payload)
	}

	if len(payload.Sources) < 10 {
		t.Fatalf("expected multiple canonical sources, got %#v", payload.Sources)
	}

	gotIDs := make([]string, 0, len(payload.Sources))
	for _, source := range payload.Sources {
		gotIDs = append(gotIDs, source.ID)
	}

	wantIDs := []string{
		"acm",
		"arxiv",
		"base",
		"biorxiv",
		"citeseerx",
		"core",
		"crossref",
		"dblp",
		"doaj",
		"europepmc",
		"google-scholar",
		"hal",
		"iacr",
		"ieee",
		"medrxiv",
		"openalex",
		"openaire",
		"pmc",
		"pubmed",
		"semantic",
		"scihub",
		"ssrn",
		"unpaywall",
		"zenodo",
	}

	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("expected deterministic source ordering %v, got %v", wantIDs, gotIDs)
	}
}

func TestSourcesText(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithOptions([]string{"sources", "--format", "text"}, &stdout, &stderr, runOptions{
		environ:        []string{},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Fatalf("expected non-JSON text output, got %q", output)
	}

	for _, want := range []string{"acm", "ieee", "enabled:", "capabilities:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected text output to contain %q, got %q", want, output)
		}
	}
}

func TestCapabilityRegistry(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithOptions([]string{"sources"}, &stdout, &stderr, runOptions{
		environ:        []string{},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Status  string                `json:"status"`
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	for _, source := range payload.Sources {
		if source.Capabilities.Search == "" || source.Capabilities.Download == "" || source.Capabilities.Read == "" {
			t.Fatalf("expected capability state for every source, got %#v", source)
		}
	}

	assertSourceCapability(t, payload.Sources, "arxiv", true, "", "supported", "supported", "supported")
	assertSourceCapability(t, payload.Sources, "crossref", true, "", "supported", "informational", "informational")
	assertSourceCapability(t, payload.Sources, "dblp", true, "", "supported", "unsupported", "unsupported")
	assertSourceCapability(t, payload.Sources, "scihub", true, "", "unsupported", "supported", "unsupported")
	assertSourceCapability(t, payload.Sources, "ssrn", true, "", "supported", "record_dependent", "record_dependent")
}

func TestIEEEACMGating(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithOptions([]string{"sources"}, &stdout, &stderr, runOptions{
		environ:        []string{},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	for _, id := range []string{"ieee", "acm"} {
		source := findSource(t, payload.Sources, id)
		if source.Enabled {
			t.Fatalf("expected %s to be gated off by default, got %#v", id, source)
		}
		if !strings.Contains(strings.ToLower(source.DisableReason), "missing") {
			t.Fatalf("expected missing-key disable reason for %s, got %#v", id, source)
		}
		if source.Capabilities.Search != "gated" || source.Capabilities.Download != "gated" || source.Capabilities.Read != "gated" {
			t.Fatalf("expected all capabilities gated for %s, got %#v", id, source)
		}
	}
}

func TestSourcesJSONUsesGlobalConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	writeCLIConfig(t, homeDir, "config.yml", strings.Join([]string{
		"ieee_api_key: ieee-from-yml",
		"acm_api_key: acm-from-yml",
		"",
	}, "\n"))
	writeCLIConfig(t, homeDir, "config.yaml", "ieee_api_key: ieee-from-yaml\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"HOME=" + homeDir},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var payload struct {
		Status  string                `json:"status"`
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	assertSourceCapability(t, payload.Sources, "ieee", true, "", "supported", "unsupported", "unsupported")
	assertSourceCapability(t, payload.Sources, "acm", false, "missing required credential: SEARCH_PAPER_ACM_API_KEY", "gated", "gated", "gated")
}

func TestSourcesJSONMergesEnvAndConfigPerKey(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	writeCLIConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"ieee_api_key: ieee-from-file",
		"acm_api_key: acm-from-file",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"HOME=" + homeDir, "SEARCH_PAPER_IEEE_API_KEY="},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Status  string                `json:"status"`
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	assertSourceCapability(t, payload.Sources, "acm", true, "", "supported", "unsupported", "unsupported")
	assertSourceCapability(t, payload.Sources, "ieee", false, "missing required credential: SEARCH_PAPER_IEEE_API_KEY", "gated", "gated", "gated")
}

func TestSourcesJSONIgnoresNonStringGlobalScalars(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	writeCLIConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"ieee_api_key: false",
		"acm_api_key: 12345",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"HOME=" + homeDir},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var payload struct {
		Status  string                `json:"status"`
		Sources []sourceRegistryEntry `json:"sources"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %q: %v", stdout.String(), err)
	}

	assertSourceCapability(t, payload.Sources, "acm", false, "missing required credential: SEARCH_PAPER_ACM_API_KEY", "gated", "gated", "gated")
	assertSourceCapability(t, payload.Sources, "ieee", false, "missing required credential: SEARCH_PAPER_IEEE_API_KEY", "gated", "gated", "gated")
}

func TestInvalidSourceError(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithOptions([]string{"sources", "--source", "nope"}, &stdout, &stderr, runOptions{
		environ:        []string{},
		workingDir:     t.TempDir(),
		repositoryRoot: t.TempDir(),
	})
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
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
		t.Fatalf("expected valid json error, got %q: %v", stdout.String(), err)
	}

	if payload.Status != "error" || payload.Error.Code != "invalid_source" {
		t.Fatalf("expected invalid_source error, got %#v", payload)
	}

	if payload.Error.Details.InvalidSource != "nope" {
		t.Fatalf("expected invalid source details, got %#v", payload)
	}

	if !slices.Contains(payload.Error.Details.ValidSources, "arxiv") || !slices.Contains(payload.Error.Details.ValidSources, "ieee") {
		t.Fatalf("expected valid source set in details, got %#v", payload.Error.Details.ValidSources)
	}
}

func TestInvalidFormat(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"sources", "--format", "yaml"},
		{"search", "--format", "yaml"},
		{"get", "--as", "pdf", "--format", "yaml"},
		{"download", "--format", "yaml"},
		{"read", "--format", "yaml"},
	} {
		args := args
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := runWithOptions(args, &stdout, &stderr, runOptions{
				environ:        []string{},
				workingDir:     t.TempDir(),
				repositoryRoot: t.TempDir(),
			})
			if exitCode != 2 {
				t.Fatalf("expected exit code 2, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
			}

			var payload struct {
				Status string `json:"status"`
				Error  struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("expected valid json error, got %q: %v", stdout.String(), err)
			}

			if payload.Status != "error" || payload.Error.Code != "invalid_usage" {
				t.Fatalf("expected invalid_usage response, got %#v", payload)
			}

			if !strings.Contains(payload.Error.Message, "unsupported format") {
				t.Fatalf("expected unsupported format message, got %#v", payload)
			}
		})
	}
}

func assertSourceCapability(t *testing.T, sources []sourceRegistryEntry, id string, enabled bool, disableReason string, search string, download string, read string) {
	t.Helper()

	source := findSource(t, sources, id)
	if source.Enabled != enabled || source.DisableReason != disableReason {
		t.Fatalf("unexpected enablement for %s: %#v", id, source)
	}
	if source.Capabilities.Search != search || source.Capabilities.Download != download || source.Capabilities.Read != read {
		t.Fatalf("unexpected capabilities for %s: %#v", id, source)
	}
}

func findSource(t *testing.T, sources []sourceRegistryEntry, id string) sourceRegistryEntry {
	t.Helper()

	for _, source := range sources {
		if source.ID == id {
			return source
		}
	}

	t.Fatalf("expected to find source %q in %#v", id, sources)
	return sourceRegistryEntry{}
}

func assertJSONInvalidUsage(t *testing.T, payload string, wantMessage string) {
	t.Helper()

	var response struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("expected valid json error payload, got %q: %v", payload, err)
	}

	if response.Status != "error" {
		t.Fatalf("expected status=error, got %#v", response)
	}

	if response.Error.Code != "invalid_usage" {
		t.Fatalf("expected invalid_usage code, got %#v", response)
	}

	if !strings.Contains(response.Error.Message, wantMessage) {
		t.Fatalf("expected error message to contain %q, got %#v", wantMessage, response)
	}
}

func assertNoPanicText(t *testing.T, outputs ...string) {
	t.Helper()

	for _, output := range outputs {
		if strings.Contains(strings.ToLower(output), "panic") || strings.Contains(strings.ToLower(output), "traceback") {
			t.Fatalf("expected no panic or traceback text, got %q", output)
		}
	}
}

func writeCLIConfig(t *testing.T, homeDir, fileName, contents string) {
	t.Helper()

	path := filepath.Join(homeDir, ".config", "search-paper-cli", fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
