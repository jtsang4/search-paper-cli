package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/connectors"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type retrievalResponse struct {
	Status    string `json:"status"`
	Operation string `json:"operation"`
	State     string `json:"state"`
	Source    string `json:"source"`
	PaperID   string `json:"paper_id"`
	Path      string `json:"path,omitempty"`
	Content   string `json:"content,omitempty"`
	Message   string `json:"message,omitempty"`
	Attempts  []any  `json:"attempts"`
}

func runDownloadCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	return runRetrievalCommand("download", args, stdout, stderr, opts)
}

func runReadCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	return runRetrievalCommand("read", args, stdout, stderr, opts)
}

func runRetrievalCommand(operation string, args []string, stdout, stderr io.Writer, opts runOptions) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, _ = io.WriteString(stdout, retrievalCommandHelp(operation))
			return exitCodeOK
		}
	}

	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	format := addFormatFlag(flags)
	sourceID := flags.String("source", "", "Source id for source-native retrieval.")
	saveDir := flags.String("save-dir", "", "Directory where downloaded PDFs should be saved.")
	paperJSON := flags.String("paper-json", "", "Normalized paper JSON object from prior search output.")
	paperID := flags.String("paper-id", "", "Paper identifier.")
	title := flags.String("title", "", "Paper title.")
	doi := flags.String("doi", "", "Paper DOI.")
	pdfURL := flags.String("pdf-url", "", "Direct PDF URL.")
	urlValue := flags.String("url", "", "Landing page URL.")
	if err := flags.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), map[string]any{
			"command": operation,
		})
	}

	if !isSupportedFormat(*format) {
		response := validateFormat(*format)
		response.Error.Details["command"] = operation
		return writeError(stdout, response, exitCodeInvalidUsage)
	}

	if len(flags.Args()) != 0 {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown argument %q for %s command", flags.Args()[0], operation), map[string]any{
			"command": operation,
			"args":    flags.Args(),
		})
	}

	workingDir, _, cfg, exitCode := loadRuntimeConfig(stdout, stderr, opts)
	if exitCode != 0 {
		return exitCode
	}

	p, err := parseRetrievalPaper(retrievalPaperInput{
		PaperJSON: *paperJSON,
		PaperID:   *paperID,
		Title:     *title,
		DOI:       *doi,
		PDFURL:    *pdfURL,
		URL:       *urlValue,
		Source:    *sourceID,
	})
	if err != nil {
		return writeInvalidUsage(stdout, err.Error(), map[string]any{
			"command": operation,
		})
	}
	if p.Source == "" {
		return writeInvalidUsage(stdout, fmt.Sprintf("%s requires a source via --source or --paper-json", operation), map[string]any{
			"command": operation,
		})
	}

	resolvedSaveDir := strings.TrimSpace(*saveDir)
	if resolvedSaveDir == "" {
		resolvedSaveDir = workingDir
	}
	if !filepath.IsAbs(resolvedSaveDir) {
		resolvedSaveDir = filepath.Join(workingDir, resolvedSaveDir)
	}
	resolvedSaveDir = filepath.Clean(resolvedSaveDir)

	descriptor, ok := lookupSourceDescriptor(cfg, p.Source)
	if !ok {
		return writeError(stdout, errorResponse{
			Status: "error",
			Error: struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details,omitempty"`
			}{
				Code:    "invalid_source",
				Message: fmt.Sprintf("unknown source %q", p.Source),
				Details: map[string]any{"invalid_source": p.Source, "valid_sources": sources.ValidIDs()},
			},
		}, exitCodeInvalidUsage)
	}
	if !descriptor.Enabled && capabilityForOperation(descriptor, operation) == sources.CapabilityGated {
		return writeUnsupportedError(stdout, "gated_source", fmt.Sprintf("requested source %q is gated for %s", p.Source, operation), map[string]any{
			"source":  p.Source,
			"reason":  descriptor.DisableReason,
			"command": operation,
		})
	}

	factory := opts.connectorFactory
	if factory == nil {
		factory = connectors.New
	}

	connector, err := factory(p.Source, cfg)
	if err != nil {
		return writeRuntimeError(stdout, err.Error())
	}

	var result sources.RetrievalResult
	switch operation {
	case "download":
		result, err = connector.Download(sources.DownloadRequest{Paper: p, SaveDir: resolvedSaveDir})
	case "read":
		result, err = connector.Read(sources.ReadRequest{Paper: p, SaveDir: resolvedSaveDir})
	default:
		return writeRuntimeError(stdout, "unknown retrieval operation")
	}
	if err != nil {
		return writeRuntimeError(stdout, err.Error())
	}
	if result.Path != "" {
		if _, err := os.Stat(result.Path); err != nil {
			return writeRuntimeError(stdout, fmt.Sprintf("retrieval reported path %q but the file does not exist", result.Path))
		}
	}

	response := retrievalResponse{
		Status:    "ok",
		Operation: operation,
		State:     string(result.State),
		Source:    p.Source,
		PaperID:   p.PaperID,
		Path:      result.Path,
		Content:   result.Content,
		Message:   result.Message,
		Attempts:  []any{},
	}

	if outputFormat(*format) == formatText {
		_, _ = io.WriteString(stdout, renderRetrievalText(response))
		return retrievalExitCode(result.State)
	}
	return writeJSON(stdout, response, retrievalExitCode(result.State))
}

func loadRuntimeConfig(stdout, stderr io.Writer, opts runOptions) (string, string, config.Config, int) {
	workingDir := opts.workingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return "", "", config.Config{}, writeRuntimeError(stdout, "failed to determine working directory")
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
		return "", "", config.Config{}, writeRuntimeError(stdout, "failed to load configuration")
	}
	writeWarnings(stderr, diagnostics)
	return workingDir, repositoryRoot, cfg, 0
}

type retrievalPaperInput struct {
	PaperJSON string
	PaperID   string
	Title     string
	DOI       string
	PDFURL    string
	URL       string
	Source    string
}

func parseRetrievalPaper(input retrievalPaperInput) (paper.Paper, error) {
	var p paper.Paper
	if strings.TrimSpace(input.PaperJSON) != "" {
		if err := json.Unmarshal([]byte(input.PaperJSON), &p); err != nil {
			return paper.Paper{}, fmt.Errorf("invalid --paper-json: %w", err)
		}
	}

	override := paper.Paper{
		PaperID: input.PaperID,
		Title:   input.Title,
		DOI:     input.DOI,
		PDFURL:  input.PDFURL,
		URL:     input.URL,
		Source:  input.Source,
	}
	if strings.TrimSpace(override.PaperID) != "" {
		p.PaperID = override.PaperID
	}
	if strings.TrimSpace(override.Title) != "" {
		p.Title = override.Title
	}
	if strings.TrimSpace(override.DOI) != "" {
		p.DOI = override.DOI
	}
	if strings.TrimSpace(override.PDFURL) != "" {
		p.PDFURL = override.PDFURL
	}
	if strings.TrimSpace(override.URL) != "" {
		p.URL = override.URL
	}
	if strings.TrimSpace(override.Source) != "" {
		if strings.TrimSpace(p.Source) != "" && !strings.EqualFold(strings.TrimSpace(p.Source), strings.TrimSpace(override.Source)) {
			return paper.Paper{}, fmt.Errorf("--source %q does not match paper source %q", override.Source, p.Source)
		}
		p.Source = override.Source
	}

	return p.Normalized(), nil
}

func retrievalCommandHelp(operation string) string {
	return "Usage:\n  search-paper-cli " + operation + " --source <id> [--save-dir <dir>] [--paper-json <json>]\n\n" +
		"Fetch paper full text or extracted content using source-native retrieval.\n"
}

func lookupSourceDescriptor(cfg config.Config, sourceID string) (sources.Descriptor, bool) {
	for _, descriptor := range sources.List(cfg) {
		if descriptor.ID == strings.ToLower(strings.TrimSpace(sourceID)) {
			return descriptor, true
		}
	}
	return sources.Descriptor{}, false
}

func capabilityForOperation(descriptor sources.Descriptor, operation string) sources.CapabilityState {
	if operation == "read" {
		return descriptor.Capabilities.Read
	}
	return descriptor.Capabilities.Download
}

func retrievalExitCode(state sources.RetrievalState) int {
	switch state {
	case sources.RetrievalStateInformational, sources.RetrievalStateUnsupported:
		return exitCodeUnsupported
	case sources.RetrievalStateFailed:
		return exitCodeRuntimeError
	default:
		return exitCodeOK
	}
}

func renderRetrievalText(response retrievalResponse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("operation: %s\n", response.Operation))
	b.WriteString(fmt.Sprintf("state: %s\n", response.State))
	b.WriteString(fmt.Sprintf("source: %s\n", response.Source))
	if response.PaperID != "" {
		b.WriteString(fmt.Sprintf("paper_id: %s\n", response.PaperID))
	}
	if response.Path != "" {
		b.WriteString(fmt.Sprintf("path: %s\n", response.Path))
	}
	if response.Message != "" {
		b.WriteString(fmt.Sprintf("message: %s\n", response.Message))
	}
	if response.Content != "" {
		b.WriteString(fmt.Sprintf("content: %s\n", response.Content))
	}
	return b.String()
}
