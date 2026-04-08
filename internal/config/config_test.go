package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefixedEnvOverridesLegacy(t *testing.T) {
	assertPrefixedEnvOverridesLegacy(t)
}

func assertPrefixedEnvOverridesLegacy(t *testing.T) {
	t.Helper()

	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{
			"UNPAYWALL_EMAIL=legacy@example.com",
			"SEARCH_PAPER_UNPAYWALL_EMAIL=prefixed@example.com",
			"CORE_API_KEY=legacy-core-key",
			"SEARCH_PAPER_CORE_API_KEY=",
		},
		WorkingDir:     t.TempDir(),
		RepositoryRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.UnpaywallEmail != "prefixed@example.com" {
		t.Fatalf("expected prefixed unpaywall email to win, got %q", cfg.UnpaywallEmail)
	}

	if cfg.CoreAPIKey != "" {
		t.Fatalf("expected explicitly empty prefixed core key to mask legacy alias, got %q", cfg.CoreAPIKey)
	}

	if len(diagnostics.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", diagnostics.Warnings)
	}
}

func TestBaseURLOptionsLoadFromPrefixedEnv(t *testing.T) {
	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{
			"SEARCH_PAPER_ARXIV_BASE_URL=https://arxiv.example/api",
			"SEARCH_PAPER_OPENAIRE_BASE_URL=https://openaire.example/search/researchProducts",
			"SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL=https://openaire.example/search/publications",
			"SEARCH_PAPER_CORE_BASE_URL=https://core.example/v3/search/works",
			"SEARCH_PAPER_EUROPEPMC_BASE_URL=https://europepmc.example/search",
			"SEARCH_PAPER_PMC_SEARCH_URL=https://pmc.example/esearch.fcgi",
			"SEARCH_PAPER_PMC_SUMMARY_URL=https://pmc.example/esummary.fcgi",
			"SEARCH_PAPER_UNPAYWALL_BASE_URL=https://unpaywall.example/v2",
		},
		WorkingDir:     t.TempDir(),
		RepositoryRoot: t.TempDir(),
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

func TestEnvFilePrecedence(t *testing.T) {
	t.Run("prefixed env overrides legacy aliases and empty values mask fallback", func(t *testing.T) {
		assertPrefixedEnvOverridesLegacy(t)
	})

	t.Run("explicit env file wins and malformed lines do not crash", func(t *testing.T) {
		repoRoot := t.TempDir()
		cwd := filepath.Join(repoRoot, "nested", "workspace")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		explicitPath := filepath.Join(repoRoot, "explicit.env")
		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=repo@example.com\n")
		writeEnvFile(t, filepath.Join(cwd, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=cwd@example.com\n")
		writeEnvFile(t, explicitPath, strings.Join([]string{
			"# comment",
			"",
			"export SEARCH_PAPER_UNPAYWALL_EMAIL=\"explicit@example.com\"",
			"SEARCH_PAPER_IEEE_API_KEY='quoted-key'",
			"NOT_A_VALID_ASSIGNMENT sentinel-secret",
			"",
		}, "\n"))

		cfg, diagnostics, err := Load(LoadOptions{
			Environ: []string{
				"SEARCH_PAPER_ENV_FILE=" + explicitPath,
			},
			WorkingDir:     cwd,
			RepositoryRoot: repoRoot,
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.UnpaywallEmail != "explicit@example.com" {
			t.Fatalf("expected explicit env file to win, got %q", cfg.UnpaywallEmail)
		}

		if cfg.IEEEAPIKey != "quoted-key" {
			t.Fatalf("expected quoted export syntax to parse, got %q", cfg.IEEEAPIKey)
		}

		if diagnostics.EnvFile != explicitPath {
			t.Fatalf("expected explicit env file path %q, got %q", explicitPath, diagnostics.EnvFile)
		}

		if !hasWarningContaining(diagnostics.Warnings, "malformed") {
			t.Fatalf("expected malformed line warning, got %#v", diagnostics.Warnings)
		}
	})

	t.Run("cwd env file wins over repository root", func(t *testing.T) {
		repoRoot := t.TempDir()
		cwd := filepath.Join(repoRoot, "child")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=repo@example.com\n")
		writeEnvFile(t, filepath.Join(cwd, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=cwd@example.com\n")

		cfg, diagnostics, err := Load(LoadOptions{
			WorkingDir:     cwd,
			RepositoryRoot: repoRoot,
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.UnpaywallEmail != "cwd@example.com" {
			t.Fatalf("expected cwd env file to win, got %q", cfg.UnpaywallEmail)
		}

		if diagnostics.EnvFile != filepath.Join(cwd, ".env") {
			t.Fatalf("expected cwd env file to be used, got %q", diagnostics.EnvFile)
		}
	})

	t.Run("repository root env file is used when cwd is inside source tree", func(t *testing.T) {
		repoRoot := t.TempDir()
		cwd := filepath.Join(repoRoot, "nested", "child")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=repo@example.com\n")

		cfg, diagnostics, err := Load(LoadOptions{
			WorkingDir:     cwd,
			RepositoryRoot: repoRoot,
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.UnpaywallEmail != "repo@example.com" {
			t.Fatalf("expected repository root env file to be used, got %q", cfg.UnpaywallEmail)
		}

		if diagnostics.EnvFile != filepath.Join(repoRoot, ".env") {
			t.Fatalf("expected repository root env file to be used, got %q", diagnostics.EnvFile)
		}
	})

	t.Run("missing explicit env file does not crash and does not fall back", func(t *testing.T) {
		repoRoot := t.TempDir()
		cwd := filepath.Join(repoRoot, "child")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "SEARCH_PAPER_UNPAYWALL_EMAIL=repo@example.com\n")
		missingPath := filepath.Join(repoRoot, "missing.env")

		cfg, diagnostics, err := Load(LoadOptions{
			Environ: []string{
				"SEARCH_PAPER_ENV_FILE=" + missingPath,
			},
			WorkingDir:     cwd,
			RepositoryRoot: repoRoot,
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.UnpaywallEmail != "" {
			t.Fatalf("expected no fallback when explicit env file is missing, got %q", cfg.UnpaywallEmail)
		}

		if diagnostics.EnvFile != missingPath {
			t.Fatalf("expected missing explicit path to be reported, got %q", diagnostics.EnvFile)
		}

		if !hasWarningContaining(diagnostics.Warnings, "not found") {
			t.Fatalf("expected missing file warning, got %#v", diagnostics.Warnings)
		}
	})
}

func writeEnvFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
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
