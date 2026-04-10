package cli

import (
	"flag"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/connectors"
	"github.com/jtsang4/search-paper-cli/internal/paper"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

type searchResponse struct {
	Status           string            `json:"status"`
	Mode             string            `json:"mode"`
	Query            string            `json:"query"`
	RequestedSources []string          `json:"requested_sources"`
	UsedSources      []string          `json:"used_sources"`
	InvalidSources   []string          `json:"invalid_sources"`
	FailedSources    []string          `json:"failed_sources,omitempty"`
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

type yearConstraint struct {
	start int
	end   int
}

var publishedYearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

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
	year := flags.String("year", "", "Optional year filter in YYYY or YYYY-YYYY form. Forwarded upstream to Semantic Scholar and enforced locally on final results.")
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

	yearFilter, yearValue, yearErr := parseYearConstraint(strings.TrimSpace(*year))
	if yearErr != nil {
		return writeInvalidUsage(stdout, yearErr.Error(), map[string]any{
			"command": "search",
			"flag":    "year",
			"format":  "YYYY or YYYY-YYYY",
			"example": "2024 or 2022-2024",
		})
	}

	_, cfg, exitCode := loadRuntimeConfig(stdout, stderr, opts)
	if exitCode != 0 {
		return exitCode
	}

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
	failedSources := make([]string, 0)
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
			failedSources = append(failedSources, descriptor.ID)
			runtimeFailures++
			continue
		}

		request := sources.SearchRequest{
			Query: query,
			Limit: limitOrZero(*limit),
		}
		if descriptor.ID == "semantic" {
			request.Year = yearValue
		}

		result, err := connector.Search(request)
		if err != nil {
			sourceResults[descriptor.ID] = 0
			sourceErrors[descriptor.ID] = err.Error()
			failedSources = append(failedSources, descriptor.ID)
			runtimeFailures++
			continue
		}
		filteredPapers := filterPapersByYear(result.Papers, yearFilter)
		if yearFilter != nil {
			sourceResults[descriptor.ID] = len(filteredPapers)
		} else {
			sourceResults[descriptor.ID] = result.Count
		}
		allPapers = append(allPapers, filteredPapers...)
	}

	deduped := paper.Dedupe(allPapers)
	mode := "complete"
	if len(failedSources) != 0 {
		mode = "degraded"
	}
	response := searchResponse{
		Status:           "ok",
		Mode:             mode,
		Query:            query,
		RequestedSources: requestedSearchSources(requested, registry, invalid),
		UsedSources:      usedSources,
		InvalidSources:   invalid,
		FailedSources:    failedSources,
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
	b.WriteString(fmt.Sprintf("mode: %s\n", response.Mode))
	b.WriteString(fmt.Sprintf("query: %s\n", response.Query))
	b.WriteString(fmt.Sprintf("requested_sources: %s\n", strings.Join(response.RequestedSources, ", ")))
	b.WriteString(fmt.Sprintf("used_sources: %s\n", strings.Join(response.UsedSources, ", ")))
	if len(response.InvalidSources) != 0 {
		b.WriteString(fmt.Sprintf("invalid_sources: %s\n", strings.Join(response.InvalidSources, ", ")))
	}
	if len(response.FailedSources) != 0 {
		b.WriteString(fmt.Sprintf("failed_sources: %s\n", strings.Join(response.FailedSources, ", ")))
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

func parseYearConstraint(value string) (*yearConstraint, string, error) {
	if value == "" {
		return nil, "", nil
	}

	rangeParts := strings.Split(value, "-")
	switch len(rangeParts) {
	case 1:
		year, err := parseYearValue(rangeParts[0])
		if err != nil {
			return nil, "", err
		}
		return &yearConstraint{start: year, end: year}, strconv.Itoa(year), nil
	case 2:
		start, err := parseYearValue(rangeParts[0])
		if err != nil {
			return nil, "", err
		}
		end, err := parseYearValue(rangeParts[1])
		if err != nil {
			return nil, "", err
		}
		if start > end {
			return nil, "", fmt.Errorf("search --year must be YYYY or YYYY-YYYY with an ascending range")
		}
		return &yearConstraint{start: start, end: end}, fmt.Sprintf("%04d-%04d", start, end), nil
	default:
		return nil, "", fmt.Errorf("search --year must be YYYY or YYYY-YYYY")
	}
}

func parseYearValue(value string) (int, error) {
	value = strings.TrimSpace(value)
	if len(value) != 4 {
		return 0, fmt.Errorf("search --year must be YYYY or YYYY-YYYY")
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 1900 || year > 2099 {
		return 0, fmt.Errorf("search --year must be YYYY or YYYY-YYYY")
	}
	return year, nil
}

func filterPapersByYear(papers []paper.Paper, constraint *yearConstraint) []paper.Paper {
	if constraint == nil {
		return papers
	}
	if len(papers) == 0 {
		return []paper.Paper{}
	}

	filtered := make([]paper.Paper, 0, len(papers))
	for _, item := range papers {
		year, ok := extractPublishedYear(item.PublishedDate)
		if !ok {
			continue
		}
		if year < constraint.start || year > constraint.end {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func extractPublishedYear(value string) (int, bool) {
	match := publishedYearPattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return 0, false
	}
	year, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	return year, true
}
