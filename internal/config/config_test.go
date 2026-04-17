package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalConfigYAMLPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yml", strings.Join([]string{
		"ieee_api_key: ieee-from-yml",
		"acm_api_key: acm-from-yml",
		"",
	}, "\n"))
	writeGlobalConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"ieee_api_key: ieee-from-yaml",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.IEEEAPIKey != "ieee-from-yaml" {
		t.Fatalf("expected config.yaml to win, got %q", cfg.IEEEAPIKey)
	}

	if cfg.ACMAPIKey != "" {
		t.Fatalf("expected config.yml to be ignored when config.yaml exists, got %q", cfg.ACMAPIKey)
	}

	wantPath := filepath.Join(homeDir, ".config", "search-paper-cli", "config.yaml")
	if diagnostics.ConfigFile != wantPath {
		t.Fatalf("expected diagnostics config file %q, got %q", wantPath, diagnostics.ConfigFile)
	}

	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", diagnostics.Warnings)
	}
}

func TestGlobalConfigYMLFallback(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yml", strings.Join([]string{
		"acm_api_key: acm-from-yml",
		"ieee_api_key: ieee-from-yml",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ACMAPIKey != "acm-from-yml" || cfg.IEEEAPIKey != "ieee-from-yml" {
		t.Fatalf("expected config.yml fallback values, got %#v", cfg)
	}

	wantPath := filepath.Join(homeDir, ".config", "search-paper-cli", "config.yml")
	if diagnostics.ConfigFile != wantPath {
		t.Fatalf("expected diagnostics config file %q, got %q", wantPath, diagnostics.ConfigFile)
	}
}

func TestEnvMergePerKey(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"acm_api_key: acm-from-file",
		"arxiv_base_url: https://yaml.example/arxiv",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{
			"HOME=" + homeDir,
			"SEARCH_PAPER_IEEE_API_KEY=ieee-from-env",
			"SEARCH_PAPER_ARXIV_BASE_URL=https://env.example/arxiv",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ACMAPIKey != "acm-from-file" {
		t.Fatalf("expected config-only ACM key, got %q", cfg.ACMAPIKey)
	}
	if cfg.IEEEAPIKey != "ieee-from-env" {
		t.Fatalf("expected env-only IEEE key, got %q", cfg.IEEEAPIKey)
	}
	if cfg.ArxivBaseURL != "https://env.example/arxiv" {
		t.Fatalf("expected same-key env override to win, got %q", cfg.ArxivBaseURL)
	}
	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", diagnostics.Warnings)
	}
}

func TestEnvMergeEmptyEnvBlocksGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", "ieee_api_key: ieee-from-file\n")

	cfg, _, err := Load(LoadOptions{
		Environ: []string{
			"HOME=" + homeDir,
			"SEARCH_PAPER_IEEE_API_KEY=",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.IEEEAPIKey != "" {
		t.Fatalf("expected explicit empty env to block file fallback, got %q", cfg.IEEEAPIKey)
	}
}

func TestLegacyEnvIgnored(t *testing.T) {
	homeDir := t.TempDir()
	repoRoot := t.TempDir()
	cwd := filepath.Join(repoRoot, "nested", "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	explicitPath := filepath.Join(repoRoot, "explicit.env")
	writeEnvFile(t, explicitPath, "SEARCH_PAPER_ACM_API_KEY=acm-from-explicit\n")
	writeEnvFile(t, filepath.Join(repoRoot, ".env"), "SEARCH_PAPER_IEEE_API_KEY=ieee-from-repo\n")
	writeEnvFile(t, filepath.Join(cwd, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=cwd@example.com\n")

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{
			"HOME=" + homeDir,
			"SEARCH_PAPER_ENV_FILE=" + explicitPath,
			"SEARCH_PAPER_CORE_API_KEY=core-from-env",
		},
		WorkingDir:     cwd,
		RepositoryRoot: repoRoot,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CoreAPIKey != "core-from-env" {
		t.Fatalf("expected real env var to load, got %q", cfg.CoreAPIKey)
	}
	if cfg.ACMAPIKey != "" || cfg.IEEEAPIKey != "" || cfg.UnpaywallEmail != "" {
		t.Fatalf("expected legacy env inputs to be ignored, got %#v", cfg)
	}
	if diagnostics.ConfigFile != "" {
		t.Fatalf("expected no config file diagnostics, got %q", diagnostics.ConfigFile)
	}
	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected legacy env discovery to be ignored silently, got %#v", diagnostics.Warnings)
	}
}

func TestWarningsForMalformedGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", "ieee_api_key: [broken\n")

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg != (Config{}) {
		t.Fatalf("expected malformed config to be ignored, got %#v", cfg)
	}
	if !hasWarningContaining(diagnostics.Warnings, "malformed global config") {
		t.Fatalf("expected malformed config warning, got %#v", diagnostics.Warnings)
	}
}

func TestSecretsStayOutOfWarnings(t *testing.T) {
	homeDir := t.TempDir()
	secret := "sentinel-secret-value"
	writeGlobalConfig(t, homeDir, "config.yaml", "acm_api_key: \""+secret+"\"\n: bad\n")

	_, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, warning := range diagnostics.Warnings {
		if strings.Contains(warning.Message, secret) {
			t.Fatalf("expected warning to omit secret, got %#v", diagnostics.Warnings)
		}
	}
}

func TestGlobalConfigIgnoresUnknownAndBlankValues(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"unknown_key: value",
		"ieee_api_key: \"   \"",
		"acm_api_key: \"\t\"",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.IEEEAPIKey != "" || cfg.ACMAPIKey != "" {
		t.Fatalf("expected blank values to be ignored, got %#v", cfg)
	}
	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected blank and unknown keys to be ignored safely, got %#v", diagnostics.Warnings)
	}
}

func TestGlobalConfigIgnoresNonStringScalars(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"ieee_api_key: false",
		"acm_api_key: 12345",
		"arxiv_base_url: 67890",
		"unpaywall_base_url: true",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.IEEEAPIKey != "" || cfg.ACMAPIKey != "" {
		t.Fatalf("expected non-string credential scalars to be ignored, got %#v", cfg)
	}
	if cfg.ArxivBaseURL != "" || cfg.UnpaywallBaseURL != "" {
		t.Fatalf("expected non-string endpoint scalars to be ignored, got %#v", cfg)
	}
	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected non-string scalars to be ignored safely, got %#v", diagnostics.Warnings)
	}
}

func TestGlobalConfigLoadsBaseURLOptions(t *testing.T) {
	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", strings.Join([]string{
		"arxiv_base_url: https://arxiv.example/api",
		"openaire_base_url: https://openaire.example/search/researchProducts",
		"openaire_legacy_base_url: https://openaire.example/search/publications",
		"core_base_url: https://core.example/v3/search/works",
		"europepmc_base_url: https://europepmc.example/search",
		"pmc_search_url: https://pmc.example/esearch.fcgi",
		"pmc_summary_url: https://pmc.example/esummary.fcgi",
		"unpaywall_base_url: https://unpaywall.example/v2",
		"",
	}, "\n"))

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{"HOME=" + homeDir},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ArxivBaseURL != "https://arxiv.example/api" {
		t.Fatalf("expected arxiv base url override, got %q", cfg.ArxivBaseURL)
	}
	if cfg.OpenAIREBaseURL != "https://openaire.example/search/researchProducts" {
		t.Fatalf("expected openaire base url override, got %q", cfg.OpenAIREBaseURL)
	}
	if cfg.OpenAIRELegacyBaseURL != "https://openaire.example/search/publications" {
		t.Fatalf("expected openaire legacy base url override, got %q", cfg.OpenAIRELegacyBaseURL)
	}
	if cfg.CoreBaseURL != "https://core.example/v3/search/works" {
		t.Fatalf("expected core base url override, got %q", cfg.CoreBaseURL)
	}
	if cfg.EuropePMCBaseURL != "https://europepmc.example/search" {
		t.Fatalf("expected europe pmc base url override, got %q", cfg.EuropePMCBaseURL)
	}
	if cfg.PMCSearchURL != "https://pmc.example/esearch.fcgi" {
		t.Fatalf("expected pmc search url override, got %q", cfg.PMCSearchURL)
	}
	if cfg.PMCSummaryURL != "https://pmc.example/esummary.fcgi" {
		t.Fatalf("expected pmc summary url override, got %q", cfg.PMCSummaryURL)
	}
	if cfg.UnpaywallBaseURL != "https://unpaywall.example/v2" {
		t.Fatalf("expected unpaywall base url override, got %q", cfg.UnpaywallBaseURL)
	}
	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", diagnostics.Warnings)
	}
}

func writeEnvFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeGlobalConfig(t *testing.T, homeDir, fileName, content string) {
	t.Helper()

	configPath := filepath.Join(homeDir, ".config", "search-paper-cli", fileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}
}

func hasWarningContaining(warnings []Warning, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning.Message), strings.ToLower(want)) {
			return true
		}
	}
	return false
}
