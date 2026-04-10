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
	Status       string                     `json:"status"`
	Operation    string                     `json:"operation"`
	Target       string                     `json:"target"`
	State        string                     `json:"state"`
	Source       string                     `json:"source"`
	PaperID      string                     `json:"paper_id"`
	Path         string                     `json:"path,omitempty"`
	Content      string                     `json:"content,omitempty"`
	Message      string                     `json:"message,omitempty"`
	WinningStage string                     `json:"winning_stage,omitempty"`
	Attempts     []sources.RetrievalAttempt `json:"attempts"`
}

func runDownloadCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	return runRetrievalCommand(retrievalMode{
		commandName: "download",
		operation:   "download",
		target:      "pdf",
	}, args, stdout, stderr, opts)
}

func runReadCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	return runRetrievalCommand(retrievalMode{
		commandName: "read",
		operation:   "read",
		target:      "text",
	}, args, stdout, stderr, opts)
}

func runGetCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	return runRetrievalCommand(retrievalMode{
		commandName: "get",
	}, args, stdout, stderr, opts)
}

type retrievalMode struct {
	commandName string
	operation   string
	target      string
}

func runRetrievalCommand(mode retrievalMode, args []string, stdout, stderr io.Writer, opts runOptions) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, _ = io.WriteString(stdout, retrievalCommandHelp(mode.commandName))
			return exitCodeOK
		}
	}

	flags := flag.NewFlagSet(mode.commandName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	format := addFormatFlag(flags)
	targetFlag := flags.String("as", "", "Retrieval target: pdf or text.")
	sourceID := flags.String("source", "", "Source id for source-native retrieval.")
	saveDir := flags.String("save-dir", "", "Directory where downloaded PDFs should be saved.")
	paperJSON := flags.String("paper-json", "", "Normalized paper JSON object from prior search output.")
	paperID := flags.String("paper-id", "", "Paper identifier.")
	title := flags.String("title", "", "Paper title.")
	doi := flags.String("doi", "", "Paper DOI.")
	pdfURL := flags.String("pdf-url", "", "Direct PDF URL.")
	urlValue := flags.String("url", "", "Landing page URL.")
	fallback := flags.Bool("fallback", false, "Use OA-first fallback retrieval.")
	allowSciHub := flags.Bool("allow-scihub", false, "Allow optional Sci-Hub fallback when OA stages fail.")
	sciHubBaseURL := flags.String("scihub-base-url", "https://sci-hub.se", "Sci-Hub mirror URL for direct/fallback retrieval.")
	if err := flags.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), map[string]any{
			"command": mode.commandName,
		})
	}

	if mode.commandName == "get" {
		resolvedMode, err := retrievalModeFromTarget(*targetFlag)
		if err != nil {
			return writeInvalidUsage(stdout, err.Error(), map[string]any{
				"command":           mode.commandName,
				"flag":              "as",
				"supported_targets": []string{"pdf", "text"},
			})
		}
		mode = resolvedMode
	}

	if !isSupportedFormat(*format) {
		response := validateFormat(*format)
		response.Error.Details["command"] = mode.commandName
		return writeError(stdout, response, exitCodeInvalidUsage)
	}

	if len(flags.Args()) != 0 {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown argument %q for %s command", flags.Args()[0], mode.commandName), map[string]any{
			"command": mode.commandName,
			"args":    flags.Args(),
		})
	}

	workingDir, cfg, exitCode := loadRuntimeConfig(stdout, stderr, opts)
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
			"command": mode.commandName,
		})
	}
	if p.Source == "" {
		return writeInvalidUsage(stdout, fmt.Sprintf("%s requires a source via --source or --paper-json", mode.commandName), map[string]any{
			"command": mode.commandName,
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

	if p.Source != "scihub" {
		descriptor, ok := sourceDescriptor(cfg, p.Source)
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
		if !descriptor.Enabled && retrievalCapability(descriptor, mode.operation) == sources.CapabilityGated {
			return writeUnsupportedError(stdout, "gated_source", fmt.Sprintf("requested source %q is gated for %s", p.Source, mode.commandName), map[string]any{
				"source":  p.Source,
				"reason":  descriptor.DisableReason,
				"command": mode.commandName,
			})
		}
	}

	factory := opts.connectorFactory
	if factory == nil {
		factory = connectors.New
	}

	var result sources.RetrievalResult
	var resultErr error
	switch {
	case mode.operation == "download" && *fallback:
		result, resultErr = connectors.DownloadWithFallback(cfg, factory, p.Source, p, resolvedSaveDir, *allowSciHub, *sciHubBaseURL)
	case mode.operation == "download" && p.Source == "scihub":
		result, resultErr = connectors.DownloadSciHub(firstNonEmptyString(p.DOI, p.Title, p.PaperID, p.URL), resolvedSaveDir, *sciHubBaseURL)
		if resultErr == nil {
			result.Attempts = []sources.RetrievalAttempt{{
				Stage:   "scihub",
				Source:  "scihub",
				State:   string(result.State),
				Message: result.Message,
				Path:    result.Path,
			}}
			if result.State == sources.RetrievalStateDownloaded {
				result.WinningStage = "scihub"
			}
		}
	default:
		connector, connectorErr := factory(p.Source, cfg)
		if connectorErr != nil {
			return writeRuntimeError(stdout, connectorErr.Error())
		}
		switch mode.operation {
		case "download":
			result, resultErr = connector.Download(sources.DownloadRequest{Paper: p, SaveDir: resolvedSaveDir})
		case "read":
			result, resultErr = connector.Read(sources.ReadRequest{Paper: p, SaveDir: resolvedSaveDir})
		default:
			return writeRuntimeError(stdout, "unknown retrieval operation")
		}
	}
	if resultErr != nil {
		return writeRuntimeError(stdout, resultErr.Error())
	}
	if result.Path != "" {
		if _, err := os.Stat(result.Path); err != nil {
			return writeRuntimeError(stdout, fmt.Sprintf("retrieval reported path %q but the file does not exist", result.Path))
		}
	}
	if result.Attempts == nil {
		result.Attempts = []sources.RetrievalAttempt{}
	}

	response := retrievalResponse{
		Status:       "ok",
		Operation:    mode.operation,
		Target:       mode.target,
		State:        string(result.State),
		Source:       p.Source,
		PaperID:      p.PaperID,
		Path:         result.Path,
		Content:      result.Content,
		Message:      result.Message,
		WinningStage: result.WinningStage,
		Attempts:     result.Attempts,
	}

	if outputFormat(*format) == formatText {
		_, _ = io.WriteString(stdout, renderRetrievalText(response))
		return retrievalExitCode(result.State)
	}
	return writeJSON(stdout, response, retrievalExitCode(result.State))
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
	switch operation {
	case "get":
		return "Usage:\n  search-paper-cli get --as <pdf|text> --source <id> [--save-dir <dir>] [--paper-json <json>]\n\n" +
			"Retrieve paper content using source-native or fallback retrieval with an explicit output target.\n"
	default:
		return "Usage:\n  search-paper-cli " + operation + " --source <id> [--save-dir <dir>] [--paper-json <json>]\n\n" +
			"Fetch paper full text or extracted content using source-native retrieval.\n"
	}
}

func retrievalModeFromTarget(value string) (retrievalMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pdf":
		return retrievalMode{
			commandName: "get",
			operation:   "download",
			target:      "pdf",
		}, nil
	case "text":
		return retrievalMode{
			commandName: "get",
			operation:   "read",
			target:      "text",
		}, nil
	case "":
		return retrievalMode{}, fmt.Errorf("get requires --as with one of: pdf, text")
	default:
		return retrievalMode{}, fmt.Errorf("unsupported --as value %q", strings.TrimSpace(value))
	}
}

func sourceDescriptor(cfg config.Config, sourceID string) (sources.Descriptor, bool) {
	for _, descriptor := range sources.List(cfg) {
		if descriptor.ID == strings.ToLower(strings.TrimSpace(sourceID)) {
			return descriptor, true
		}
	}
	return sources.Descriptor{}, false
}

func retrievalCapability(descriptor sources.Descriptor, operation string) sources.CapabilityState {
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
	if response.Target != "" {
		b.WriteString(fmt.Sprintf("target: %s\n", response.Target))
	}
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
