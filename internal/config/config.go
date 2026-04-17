package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
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
	ConfigFile string    `json:"config_file,omitempty"`
	Warnings   []Warning `json:"warnings,omitempty"`
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
	yamlKey  string
	assign   func(*Config, string)
}

var bindings = []binding{
	{
		prefixed: "SEARCH_PAPER_UNPAYWALL_EMAIL",
		yamlKey:  "unpaywall_email",
		assign: func(cfg *Config, value string) {
			cfg.UnpaywallEmail = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_CORE_API_KEY",
		yamlKey:  "core_api_key",
		assign: func(cfg *Config, value string) {
			cfg.CoreAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_SEMANTIC_SCHOLAR_API_KEY",
		yamlKey:  "semantic_scholar_api_key",
		assign: func(cfg *Config, value string) {
			cfg.SemanticScholarAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_GOOGLE_SCHOLAR_PROXY_URL",
		yamlKey:  "google_scholar_proxy_url",
		assign: func(cfg *Config, value string) {
			cfg.GoogleScholarProxyURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_DOAJ_API_KEY",
		yamlKey:  "doaj_api_key",
		assign: func(cfg *Config, value string) {
			cfg.DOAJAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ZENODO_ACCESS_TOKEN",
		yamlKey:  "zenodo_access_token",
		assign: func(cfg *Config, value string) {
			cfg.ZenodoAccessToken = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_IEEE_API_KEY",
		yamlKey:  "ieee_api_key",
		assign: func(cfg *Config, value string) {
			cfg.IEEEAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ACM_API_KEY",
		yamlKey:  "acm_api_key",
		assign: func(cfg *Config, value string) {
			cfg.ACMAPIKey = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_ARXIV_BASE_URL",
		yamlKey:  "arxiv_base_url",
		assign: func(cfg *Config, value string) {
			cfg.ArxivBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_OPENAIRE_BASE_URL",
		yamlKey:  "openaire_base_url",
		assign: func(cfg *Config, value string) {
			cfg.OpenAIREBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_OPENAIRE_LEGACY_BASE_URL",
		yamlKey:  "openaire_legacy_base_url",
		assign: func(cfg *Config, value string) {
			cfg.OpenAIRELegacyBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_CORE_BASE_URL",
		yamlKey:  "core_base_url",
		assign: func(cfg *Config, value string) {
			cfg.CoreBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_EUROPEPMC_BASE_URL",
		yamlKey:  "europepmc_base_url",
		assign: func(cfg *Config, value string) {
			cfg.EuropePMCBaseURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_PMC_SEARCH_URL",
		yamlKey:  "pmc_search_url",
		assign: func(cfg *Config, value string) {
			cfg.PMCSearchURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_PMC_SUMMARY_URL",
		yamlKey:  "pmc_summary_url",
		assign: func(cfg *Config, value string) {
			cfg.PMCSummaryURL = value
		},
	},
	{
		prefixed: "SEARCH_PAPER_UNPAYWALL_BASE_URL",
		yamlKey:  "unpaywall_base_url",
		assign: func(cfg *Config, value string) {
			cfg.UnpaywallBaseURL = value
		},
	},
}

func Load(opts LoadOptions) (Config, Diagnostics, error) {
	processEnv := environMap(opts.Environ)
	diagnostics := Diagnostics{}

	fileValues := map[string]string{}
	configPath, ok := selectGlobalConfigPath(processEnv, opts.Environ == nil)
	if ok {
		diagnostics.ConfigFile = configPath
		loaded, warnings := loadGlobalConfigFile(configPath)
		diagnostics.Warnings = append(diagnostics.Warnings, warnings...)
		fileValues = loaded
	}

	cfg := Config{}
	for _, item := range bindings {
		if value, present := processEnv[item.prefixed]; present {
			item.assign(&cfg, value.value)
			continue
		}
		if value, present := fileValues[item.yamlKey]; present {
			item.assign(&cfg, value)
		}
	}

	return cfg, diagnostics, nil
}

func selectGlobalConfigPath(env map[string]envValue, allowProcessFallback bool) (string, bool) {
	homeDir := resolveHomeDir(env, allowProcessFallback)
	if strings.TrimSpace(homeDir) == "" {
		return "", false
	}

	configDir := filepath.Join(homeDir, ".config", "search-paper-cli")
	yamlPath := filepath.Join(configDir, "config.yaml")
	if fileExists(yamlPath) {
		return yamlPath, true
	}

	ymlPath := filepath.Join(configDir, "config.yml")
	if fileExists(ymlPath) {
		return ymlPath, true
	}

	return "", false
}

func resolveHomeDir(env map[string]envValue, allowProcessFallback bool) string {
	if value, ok := env["HOME"]; ok {
		return strings.TrimSpace(value.value)
	}
	if !allowProcessFallback {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(homeDir)
}

func loadGlobalConfigFile(path string) (map[string]string, []Warning) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}, []Warning{{
			Message: fmt.Sprintf("failed to read global config %s; ignoring file", path),
		}}
	}

	var raw map[string]any
	if err := yaml.Unmarshal(contents, &raw); err != nil {
		return map[string]string{}, []Warning{{
			Message: fmt.Sprintf("malformed global config %s; ignoring file", path),
		}}
	}

	values := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}
		normalized, ok := normalizeConfigScalar(value)
		if !ok {
			continue
		}
		values[key] = normalized
	}
	return values, nil
}

func normalizeConfigScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	default:
		return "", false
	}
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
