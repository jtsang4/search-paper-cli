package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefixedEnvOverridesLegacy(t *testing.T) {
	cfg, diagnostics, err := Load(LoadOptions{
		Environ: []string{
			"UNPAYWALL_EMAIL=legacy@example.com",
			"PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=prefixed@example.com",
			"CORE_API_KEY=legacy-core-key",
			"PAPER_SEARCH_MCP_CORE_API_KEY=",
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

func TestEnvFilePrecedence(t *testing.T) {
	t.Run("explicit env file wins and malformed lines do not crash", func(t *testing.T) {
		repoRoot := t.TempDir()
		cwd := filepath.Join(repoRoot, "nested", "workspace")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		explicitPath := filepath.Join(repoRoot, "explicit.env")
		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=repo@example.com\n")
		writeEnvFile(t, filepath.Join(cwd, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=cwd@example.com\n")
		writeEnvFile(t, explicitPath, strings.Join([]string{
			"# comment",
			"",
			"export PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=\"explicit@example.com\"",
			"PAPER_SEARCH_MCP_IEEE_API_KEY='quoted-key'",
			"NOT_A_VALID_ASSIGNMENT sentinel-secret",
			"",
		}, "\n"))

		cfg, diagnostics, err := Load(LoadOptions{
			Environ: []string{
				"PAPER_SEARCH_MCP_ENV_FILE=" + explicitPath,
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

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=repo@example.com\n")
		writeEnvFile(t, filepath.Join(cwd, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=cwd@example.com\n")

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

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=repo@example.com\n")

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

		writeEnvFile(t, filepath.Join(repoRoot, ".env"), "PAPER_SEARCH_MCP_UNPAYWALL_EMAIL=repo@example.com\n")
		missingPath := filepath.Join(repoRoot, "missing.env")

		cfg, diagnostics, err := Load(LoadOptions{
			Environ: []string{
				"PAPER_SEARCH_MCP_ENV_FILE=" + missingPath,
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
