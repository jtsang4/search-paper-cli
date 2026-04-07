package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

const (
	exitCodeOK           = 0
	exitCodeInvalidUsage = 2
	exitCodeUnsupported  = 3
	exitCodeRuntimeError = 4
)

const defaultVersion = "search-paper-cli dev"

var version = defaultVersion

type runOptions struct {
	environ          []string
	workingDir       string
	repositoryRoot   string
	connectorFactory func(string, config.Config) (sources.Connector, error)
}

type errorResponse struct {
	Status string `json:"status"`
	Error  struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

type sourcesResponse struct {
	Status  string               `json:"status"`
	Sources []sources.Descriptor `json:"sources"`
}

type command struct {
	name        string
	summary     string
	description string
}

var commands = []command{
	{
		name:        "sources",
		summary:     "List configured paper sources.",
		description: "Inspect the source registry and source capabilities.",
	},
	{
		name:        "search",
		summary:     "Search for papers across one or more sources.",
		description: "Search registered sources and return normalized paper results.",
	},
	{
		name:        "download",
		summary:     "Download paper full text when supported.",
		description: "Download source-native or fallback paper full text into a target directory.",
	},
	{
		name:        "read",
		summary:     "Read paper content when supported.",
		description: "Fetch and extract paper content from a source-native or fallback retrieval path.",
	},
	{
		name:        "version",
		summary:     "Print the CLI version.",
		description: "Print the concise CLI version string.",
	},
}

func Run(args []string, stdout, stderr io.Writer) int {
	return runWithOptions(args, stdout, stderr, runOptions{})
}

func runWithOptions(args []string, stdout, stderr io.Writer, opts runOptions) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h":
			_, _ = io.WriteString(stdout, rootHelp())
			return exitCodeOK
		}
	}

	global := flag.NewFlagSet("search-paper-cli", flag.ContinueOnError)
	global.SetOutput(io.Discard)

	showVersion := global.Bool("version", false, "Print version information and exit.")
	global.BoolVar(showVersion, "v", false, "Print version information and exit.")

	if err := global.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), nil)
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, version)
		return exitCodeOK
	}

	remaining := global.Args()
	if len(remaining) == 0 {
		_, _ = io.WriteString(stderr, rootHelp())
		return exitCodeInvalidUsage
	}

	name := remaining[0]
	if name == "help" {
		return runHelp(remaining[1:], stdout)
	}

	cmd, ok := lookupCommand(name)
	if !ok {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown command %q", name), map[string]any{
			"valid_commands": commandNames(),
		})
	}

	switch cmd.name {
	case "sources":
		return runSourcesCommand(remaining[1:], stdout, stderr, opts)
	case "search":
		return runSearchCommand(remaining[1:], stdout, stderr, opts)
	case "version":
		return runVersionCommand(remaining[1:], stdout)
	default:
		return runPlaceholderCommand(cmd.name, cmd.description)(remaining[1:], stdout, stderr)
	}
}

func runHelp(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stdout, rootHelp())
		return exitCodeOK
	}

	cmd, ok := lookupCommand(args[0])
	if !ok {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown command %q", args[0]), map[string]any{
			"valid_commands": commandNames(),
		})
	}

	_, _ = io.WriteString(stdout, commandHelp(cmd))
	return exitCodeOK
}

func lookupCommand(name string) (command, bool) {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func commandNames() []string {
	names := make([]string, 0, len(commands))
	for _, cmd := range commands {
		names = append(names, cmd.name)
	}
	return names
}

func rootHelp() string {
	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString("  search-paper-cli <command> [flags]\n")
	b.WriteString("  search-paper-cli --version\n\n")
	b.WriteString("Commands:\n")
	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", cmd.name, cmd.summary))
	}
	b.WriteString("\nGlobal Flags:\n")
	b.WriteString("  --help      Show help for the root command.\n")
	b.WriteString("  --version   Print version information and exit.\n")
	return b.String()
}

func commandHelp(cmd command) string {
	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString(fmt.Sprintf("  search-paper-cli %s\n\n", cmd.name))
	b.WriteString(fmt.Sprintf("%s\n", cmd.description))
	return b.String()
}

func runVersionCommand(args []string, stdout io.Writer) int {
	if len(args) != 0 {
		return writeInvalidUsage(stdout, "unknown arguments for version command", map[string]any{
			"command": "version",
			"args":    args,
		})
	}

	_, _ = fmt.Fprintln(stdout, version)
	return exitCodeOK
}

func runSourcesCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	flags := flag.NewFlagSet("sources", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	format := addFormatFlag(flags)
	selectedSources := flags.String("source", "", "Comma-separated source ids to inspect.")
	if err := flags.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), map[string]any{
			"command": "sources",
		})
	}

	if len(flags.Args()) != 0 {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown argument %q for sources command", flags.Args()[0]), map[string]any{
			"command": "sources",
			"args":    flags.Args(),
		})
	}

	if !isSupportedFormat(*format) {
		response := validateFormat(*format)
		response.Error.Details["command"] = "sources"
		return writeError(stdout, response, exitCodeInvalidUsage)
	}

	workingDir := opts.workingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return writeRuntimeError(stdout, "failed to determine working directory")
		}
	}

	repositoryRoot := opts.repositoryRoot
	if repositoryRoot == "" {
		repositoryRoot = discoverRepositoryRoot(workingDir)
	}

	cfg, diagnostics, err := config.Load(config.LoadOptions{
		Environ:        opts.environ,
		WorkingDir:     workingDir,
		RepositoryRoot: repositoryRoot,
	})
	if err != nil {
		return writeRuntimeError(stdout, "failed to load configuration")
	}

	writeWarnings(stderr, diagnostics)

	registry, invalid := sources.Select(cfg, splitCSV(*selectedSources))
	if len(invalid) != 0 {
		details := map[string]any{
			"invalid_source": invalid[0],
			"valid_sources":  sources.ValidIDs(),
		}
		if len(invalid) > 1 {
			details["invalid_sources"] = invalid
		}
		return writeError(stdout, errorResponse{
			Status: "error",
			Error: struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details,omitempty"`
			}{
				Code:    "invalid_source",
				Message: fmt.Sprintf("unknown source %q", invalid[0]),
				Details: details,
			},
		}, exitCodeInvalidUsage)
	}

	if outputFormat(*format) == formatText {
		_, _ = io.WriteString(stdout, renderSourcesText(registry))
		return exitCodeOK
	}

	return writeJSON(stdout, sourcesResponse{
		Status:  "ok",
		Sources: registry,
	}, exitCodeOK)
}

func runPlaceholderCommand(name, description string) func(args []string, stdout, stderr io.Writer) int {
	return func(args []string, stdout, stderr io.Writer) int {
		for _, arg := range args {
			if arg == "--help" || arg == "-h" {
				_, _ = io.WriteString(stdout, placeholderCommandHelp(name, description))
				return exitCodeOK
			}
		}

		flags := flag.NewFlagSet(name, flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		format := addFormatFlag(flags)
		if err := flags.Parse(args); err != nil {
			return writeInvalidUsage(stdout, normalizeFlagError(err), map[string]any{
				"command": name,
			})
		}

		if !isSupportedFormat(*format) {
			response := validateFormat(*format)
			response.Error.Details["command"] = name
			return writeError(stdout, response, exitCodeInvalidUsage)
		}

		if len(flags.Args()) > 0 {
			return writeInvalidUsage(stdout, fmt.Sprintf("unknown argument %q for %s command", flags.Args()[0], name), map[string]any{
				"command": name,
				"args":    flags.Args(),
			})
		}

		_, _ = io.WriteString(stderr, placeholderCommandHelp(name, description))
		return exitCodeInvalidUsage
	}
}

func placeholderCommandHelp(name, description string) string {
	return commandHelp(command{
		name:        name,
		description: description,
	})
}

func writeWarnings(stderr io.Writer, diagnostics config.Diagnostics) {
	for _, warning := range diagnostics.Warnings {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", warning.Message)
	}
}

func normalizeFlagError(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, flag.ErrHelp):
		return "help requested"
	case strings.Contains(err.Error(), "flag provided but not defined"):
		name := strings.TrimSpace(strings.TrimPrefix(err.Error(), "flag provided but not defined:"))
		return fmt.Sprintf("unknown flag %q", name)
	default:
		return err.Error()
	}
}

func writeInvalidUsage(stdout io.Writer, message string, details map[string]any) int {
	response := errorResponse{Status: "error"}
	response.Error.Code = "invalid_usage"
	response.Error.Message = message
	response.Error.Details = details

	return writeError(stdout, response, exitCodeInvalidUsage)
}

func writeError(stdout io.Writer, response errorResponse, exitCode int) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(response)
	return exitCode
}

func writeUnsupportedError(stdout io.Writer, code string, message string, details map[string]any) int {
	response := errorResponse{Status: "error"}
	response.Error.Code = code
	response.Error.Message = message
	response.Error.Details = details

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(response)
	return exitCodeUnsupported
}

func writeRuntimeError(stdout io.Writer, message string) int {
	response := errorResponse{Status: "error"}
	response.Error.Code = "runtime_error"
	response.Error.Message = message

	return writeError(stdout, response, exitCodeRuntimeError)
}

func writeJSON(stdout io.Writer, payload any, exitCode int) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
	return exitCode
}

func discoverRepositoryRoot(workingDir string) string {
	current := workingDir
	for current != "" && current != string(filepath.Separator) {
		if fileExists(filepath.Join(current, "go.mod")) && fileExists(filepath.Join(current, ".factory", "services.yaml")) {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if fileExists(filepath.Join(current, "go.mod")) && fileExists(filepath.Join(current, ".factory", "services.yaml")) {
		return current
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func isSupportedFormat(value string) bool {
	switch outputFormat(value) {
	case formatJSON, formatText:
		return true
	default:
		return false
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func renderSourcesText(registry []sources.Descriptor) string {
	var b strings.Builder
	for _, source := range registry {
		b.WriteString(fmt.Sprintf("%s\n", source.ID))
		b.WriteString(fmt.Sprintf("  enabled: %t\n", source.Enabled))
		if source.DisableReason != "" {
			b.WriteString(fmt.Sprintf("  disable_reason: %s\n", source.DisableReason))
		}
		b.WriteString("  capabilities:\n")
		b.WriteString(fmt.Sprintf("    search: %s\n", source.Capabilities.Search))
		b.WriteString(fmt.Sprintf("    download: %s\n", source.Capabilities.Download))
		b.WriteString(fmt.Sprintf("    read: %s\n", source.Capabilities.Read))
	}
	return b.String()
}
