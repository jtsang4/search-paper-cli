package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/connectors"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type searchResponse struct {
	Status           string            `json:"status"`
	Query            string            `json:"query"`
	RequestedSources []string          `json:"requested_sources"`
	UsedSources      []string          `json:"used_sources"`
	InvalidSources   []string          `json:"invalid_sources"`
	SourceResults    map[string]int    `json:"source_results"`
	Errors           map[string]string `json:"errors,omitempty"`
	Total            int               `json:"total"`
	Papers           []paper.Paper     `json:"papers"`
}

type blockedSearchSource struct {
	ID         string                  `json:"id"`
	Capability sources.CapabilityState `json:"capability"`
	Reason     string                  `json:"reason"`
}

func runSearchCommand(args []string, stdout, stderr io.Writer, opts runOptions) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, _ = io.WriteString(stdout, commandHelp(command{
				name:        "search",
				description: "Search registered sources and return normalized paper results.",
			}))
			return exitCodeOK
		}
	}

	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	format := addFormatFlag(flags)
	selectedSources := flags.String("source", "", "Comma-separated source ids to search.")
	limit := flags.Int("limit", 10, "Maximum number of results to request from each source.")
	year := flags.String("year", "", "Optional year or year range forwarded to Semantic Scholar only.")
	if err := flags.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), map[string]any{
			"command": "search",
		})
	}

	if !isSupportedFormat(*format) {
		response := validateFormat(*format)
		response.Error.Details["command"] = "search"
		return writeError(stdout, response, exitCodeInvalidUsage)
	}

	query := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if query == "" {
		return writeInvalidUsage(stdout, "search query is required", map[string]any{
			"command": "search",
		})
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

	requested := splitCSV(*selectedSources)
	registry, invalid, blocked := selectSearchSources(cfg, requested)
	if len(registry) == 0 && len(invalid) != 0 {
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
	if len(registry) == 0 && len(blocked) != 0 {
		details := map[string]any{
			"requested_sources": requestedSearchSources(requested, registry, invalid),
			"blocked_sources":   blockedSearchSourceDetails(blocked),
		}
		return writeUnsupportedError(stdout, blockedSearchErrorCode(blocked), blockedSearchErrorMessage(blocked), details)
	}

	factory := opts.connectorFactory
	if factory == nil {
		factory = connectors.New
	}

	usedSources := make([]string, 0, len(registry))
	sourceResults := make(map[string]int, len(registry)+len(blocked))
	sourceErrors := map[string]string{}
	for _, item := range blocked {
		sourceResults[item.ID] = 0
		sourceErrors[item.ID] = item.Reason
	}
	allPapers := make([]paper.Paper, 0)
	runtimeFailures := 0
	for _, descriptor := range registry {
		usedSources = append(usedSources, descriptor.ID)
		connector, err := factory(descriptor.ID, cfg)
		if err != nil {
			sourceResults[descriptor.ID] = 0
			sourceErrors[descriptor.ID] = err.Error()
			runtimeFailures++
			continue
		}

		request := sources.SearchRequest{
			Query: query,
			Limit: limitOrZero(*limit),
		}
		if descriptor.ID == "semantic" {
			request.Year = strings.TrimSpace(*year)
		}

		result, err := connector.Search(request)
		if err != nil {
			sourceResults[descriptor.ID] = 0
			sourceErrors[descriptor.ID] = err.Error()
			runtimeFailures++
			continue
		}
		sourceResults[descriptor.ID] = result.Count
		allPapers = append(allPapers, result.Papers...)
	}

	deduped := paper.Dedupe(allPapers)
	response := searchResponse{
		Status:           "ok",
		Query:            query,
		RequestedSources: requestedSearchSources(requested, registry, invalid),
		UsedSources:      usedSources,
		InvalidSources:   invalid,
		SourceResults:    sourceResults,
		Errors:           sourceErrors,
		Total:            len(deduped),
		Papers:           deduped,
	}

	if outputFormat(*format) == formatText {
		_, _ = io.WriteString(stdout, renderSearchText(response))
		if len(registry) > 0 && runtimeFailures == len(registry) {
			return exitCodeRuntimeError
		}
		return exitCodeOK
	}

	if len(registry) > 0 && runtimeFailures == len(registry) {
		return writeJSON(stdout, response, exitCodeRuntimeError)
	}
	return writeJSON(stdout, response, exitCodeOK)
}

func selectSearchSources(cfg config.Config, requested []string) ([]sources.Descriptor, []string, []blockedSearchSource) {
	registry := sources.List(cfg)
	if len(requested) == 0 {
		filtered := make([]sources.Descriptor, 0, len(registry))
		for _, descriptor := range registry {
			if descriptor.Capabilities.Search == sources.CapabilitySupported {
				filtered = append(filtered, descriptor)
			}
		}
		return filtered, []string{}, []blockedSearchSource{}
	}

	validDescriptors := map[string]sources.Descriptor{}
	for _, descriptor := range registry {
		validDescriptors[descriptor.ID] = descriptor
	}

	seenRequested := map[string]struct{}{}
	orderedValid := make([]sources.Descriptor, 0, len(requested))
	invalid := make([]string, 0)
	blocked := make([]blockedSearchSource, 0)
	seenInvalid := map[string]struct{}{}
	for _, raw := range requested {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if descriptor, ok := validDescriptors[id]; ok {
			if _, seen := seenRequested[id]; seen {
				continue
			}
			seenRequested[id] = struct{}{}
			if descriptor.Capabilities.Search == sources.CapabilitySupported {
				orderedValid = append(orderedValid, descriptor)
			} else {
				blocked = append(blocked, blockedSearchSource{
					ID:         descriptor.ID,
					Capability: descriptor.Capabilities.Search,
					Reason:     blockedSearchReason(descriptor),
				})
			}
			continue
		}
		if _, seen := seenInvalid[id]; !seen {
			seenInvalid[id] = struct{}{}
			invalid = append(invalid, id)
		}
	}

	return orderedValid, invalid, blocked
}

func requestedSearchSources(requested []string, used []sources.Descriptor, invalid []string) []string {
	if len(requested) == 0 {
		result := make([]string, 0, len(used))
		for _, descriptor := range used {
			result = append(result, descriptor.ID)
		}
		return result
	}

	result := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, raw := range requested {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if len(result) == 0 && len(invalid) == 0 {
		for _, descriptor := range used {
			result = append(result, descriptor.ID)
		}
	}
	return result
}

func renderSearchText(response searchResponse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("query: %s\n", response.Query))
	b.WriteString(fmt.Sprintf("requested_sources: %s\n", strings.Join(response.RequestedSources, ", ")))
	b.WriteString(fmt.Sprintf("used_sources: %s\n", strings.Join(response.UsedSources, ", ")))
	if len(response.InvalidSources) != 0 {
		b.WriteString(fmt.Sprintf("invalid_sources: %s\n", strings.Join(response.InvalidSources, ", ")))
	}
	ids := make([]string, 0, len(response.SourceResults))
	for id := range response.SourceResults {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	b.WriteString("source_results:\n")
	for _, id := range ids {
		b.WriteString(fmt.Sprintf("  %s: %d\n", id, response.SourceResults[id]))
	}
	if len(response.Errors) != 0 {
		errorIDs := make([]string, 0, len(response.Errors))
		for id := range response.Errors {
			errorIDs = append(errorIDs, id)
		}
		slices.Sort(errorIDs)
		b.WriteString("errors:\n")
		for _, id := range errorIDs {
			b.WriteString(fmt.Sprintf("  %s: %s\n", id, response.Errors[id]))
		}
	}
	b.WriteString(fmt.Sprintf("total: %d\n", response.Total))
	for _, item := range response.Papers {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", item.Source, item.Title))
	}
	return b.String()
}

func limitOrZero(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}

func blockedSearchReason(descriptor sources.Descriptor) string {
	if descriptor.DisableReason != "" {
		return descriptor.DisableReason
	}
	switch descriptor.Capabilities.Search {
	case sources.CapabilityGated:
		return fmt.Sprintf("source %q is gated for search", descriptor.ID)
	case sources.CapabilityUnsupported:
		return fmt.Sprintf("source %q does not support search", descriptor.ID)
	case sources.CapabilityInformational:
		return fmt.Sprintf("source %q is informational only for search", descriptor.ID)
	case sources.CapabilityRecordDependent:
		return fmt.Sprintf("source %q does not support direct search", descriptor.ID)
	default:
		return fmt.Sprintf("source %q is unavailable for search", descriptor.ID)
	}
}

func blockedSearchSourceDetails(blocked []blockedSearchSource) []map[string]any {
	result := make([]map[string]any, 0, len(blocked))
	for _, item := range blocked {
		result = append(result, map[string]any{
			"id":         item.ID,
			"capability": item.Capability,
			"reason":     item.Reason,
		})
	}
	return result
}

func blockedSearchErrorCode(blocked []blockedSearchSource) string {
	for _, item := range blocked {
		if item.Capability == sources.CapabilityGated {
			return "gated_source"
		}
	}
	return "unsupported_source"
}

func blockedSearchErrorMessage(blocked []blockedSearchSource) string {
	if len(blocked) == 1 {
		if blocked[0].Capability == sources.CapabilityGated {
			return fmt.Sprintf("requested source %q is gated for search", blocked[0].ID)
		}
		return fmt.Sprintf("requested source %q is unavailable for search", blocked[0].ID)
	}
	return "requested sources are unavailable for search"
}
