package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	UnpaywallEmail        string
	CoreAPIKey            string
	SemanticScholarAPIKey string
	GoogleScholarProxyURL string
	DOAJAPIKey            string
	ZenodoAccessToken     string
	IEEEAPIKey            string
	ACMAPIKey             string
	ArxivBaseURL          string
	OpenAIREBaseURL       string
	OpenAIRELegacyBaseURL string
	CoreBaseURL           string
	EuropePMCBaseURL      string
	PMCSearchURL          string
	PMCSummaryURL         string
	UnpaywallBaseURL      string
}

type Warning struct {
	Message string `json:"message"`
}

type Diagnostics struct {
	EnvFile  string    `json:"env_file,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
}

type LoadOptions struct {
	Environ        []string
	WorkingDir     string
	RepositoryRoot string
}

type envValue struct {
	value   string
	present bool
}

type binding struct {
	prefixed string
	assign   func(*Config, string)
}

var bindings = []binding{
	{
		prefixed: "SEARCH_PAPER_UNPAYWALL_EMAIL",
		assign: func(cfg *Config, value string) {
			cfg.UnpaywallEmail = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_CORE_API_KEY",
		assign: func(cfg *Config, value string) {
			cfg.CoreAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_SEMANTIC_SCHOLAR_API_KEY",
		assign: func(cfg *Config, value string) {
			cfg.SemanticScholarAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_GOOGLE_SCHOLAR_PROXY_URL",
		assign: func(cfg *Config, value string) {
			cfg.GoogleScholarProxyURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_DOAJ_API_KEY",
		assign: func(cfg *Config, value string) {
			cfg.DOAJAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ZENODO_ACCESS_TOKEN",
		assign: func(cfg *Config, value string) {
			cfg.ZenodoAccessToken = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_IEEE_API_KEY",
		assign: func(cfg *Config, value string) {
			cfg.IEEEAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ACM_API_KEY",
		assign: func(cfg *Config, value string) {
			cfg.ACMAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ARXIV_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.ArxivBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_OPENAIRE_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.OpenAIREBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.OpenAIRELegacyBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_CORE_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.CoreBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_EUROPEPMC_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.EuropePMCBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_PMC_SEARCH_URL",
		assign: func(cfg *Config, value string) {
			cfg.PMCSearchURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_PMC_SUMMARY_URL",
		assign: func(cfg *Config, value string) {
			cfg.PMCSummaryURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_UNPAYWALL_BASE_URL",
		assign: func(cfg *Config, value string) {
			cfg.UnpaywallBaseURL = value
		},
	},
}

func Load(opts LoadOptions) (Config, Diagnostics, error) {
	workingDir := opts.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return Config{}, Diagnostics{}, fmt.Errorf("get working directory: %w", err)
		}
	}

	processEnv := environMap(opts.Environ)
	diagnostics := Diagnostics{}
	fileEnv := map[string]envValue{}

	envFilePath, explicit := selectEnvFile(processEnv, workingDir, opts.RepositoryRoot)
	if envFilePath != "" || explicit {
		diagnostics.EnvFile = envFilePath
	}

	switch {
	case explicit && strings.TrimSpace(envFilePath) == "":
		diagnostics.Warnings = append(diagnostics.Warnings, Warning{
			Message: "explicit env file path is empty; skipping env file loading",
		})
	case envFilePath != "":
		loaded, warnings, err := loadEnvFile(envFilePath)
		diagnostics.Warnings = append(diagnostics.Warnings, warnings...)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if explicit {
					diagnostics.Warnings = append(diagnostics.Warnings, Warning{
						Message: fmt.Sprintf("explicit env file not found: %s", envFilePath),
					})
				}
			} else {
				return Config{}, diagnostics, fmt.Errorf("read env file %q: %w", envFilePath, err)
			}
		} else {
			fileEnv = loaded
		}
	}

	merged := map[string]envValue{}
	for key, value := range fileEnv {
		merged[key] = value
	}
	for key, value := range processEnv {
		merged[key] = value
	}

	cfg := Config{}
	for _, item := range bindings {
		value, ok := resolveValue(merged, item.prefixed)
		if ok {
			item.assign(&cfg, value)
		}
	}

	return cfg, diagnostics, nil
}

func selectEnvFile(env map[string]envValue, workingDir, repositoryRoot string) (string, bool) {
	if value, ok := env["SEARCH_PAPER_ENV_FILE"]; ok {
		if value.value == "" || filepath.IsAbs(value.value) || workingDir == "" {
			return value.value, true
		}
		return filepath.Join(workingDir, value.value), true
	}

	if workingDir != "" {
		candidate := filepath.Join(workingDir, ".env")
		if fileExists(candidate) {
			return candidate, false
		}
	}

	if repositoryRoot != "" && isWithin(workingDir, repositoryRoot) {
		candidate := filepath.Join(repositoryRoot, ".env")
		if fileExists(candidate) {
			return candidate, false
		}
	}

	return "", false
}

func loadEnvFile(path string) (map[string]envValue, []Warning, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	values := map[string]envValue{}
	var warnings []Warning
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")
		key, value, ok := parseEnvAssignment(line)
		if !ok {
			warnings = append(warnings, Warning{
				Message: fmt.Sprintf("ignored malformed env line %d in %s", lineNumber, path),
			})
			continue
		}

		values[key] = envValue{value: value, present: true}
	}

	if err := scanner.Err(); err != nil {
		return nil, warnings, err
	}

	return values, warnings, nil
}

func parseEnvAssignment(line string) (string, string, bool) {
	index := strings.IndexByte(line, '=')
	if index <= 0 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:index])
	if !validEnvKey(key) {
		return "", "", false
	}

	value := strings.TrimSpace(line[index+1:])
	value = stripQuotes(value)
	return key, value, true
}

func stripQuotes(value string) string {
	if len(value) < 2 {
		return value
	}

	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}

	if value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			return unquoted
		}
		return value[1 : len(value)-1]
	}

	return value
}

func validEnvKey(key string) bool {
	for index, r := range key {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r == '_':
		case index > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return key != ""
}

func resolveValue(env map[string]envValue, prefixed string) (string, bool) {
	value, ok := env[prefixed]
	return value.value, ok
}

func environMap(environ []string) map[string]envValue {
	if environ == nil {
		environ = os.Environ()
	}

	values := make(map[string]envValue, len(environ))
	for _, entry := range environ {
		index := strings.IndexByte(entry, '=')
		if index < 0 {
			continue
		}
		values[entry[:index]] = envValue{
			value:   entry[index+1:],
			present: true,
		}
	}
	return values
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func isWithin(path, root string) bool {
	if path == "" || root == "" {
		return false
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
