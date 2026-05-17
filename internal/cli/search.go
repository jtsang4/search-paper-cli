package cli

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

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
	PartialReason    string            `json:"partial_success_reason,omitempty"`
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

type dateConstraint struct {
	start *time.Time
	end   *time.Time
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
	year := flags.String("year", "", "Optional year filter in YYYY or YYYY-YYYY form. Forwarded upstream to Semantic Scholar and enforced locally on final results.")
	fromDate := flags.String("from-date", "", "Optional inclusive start date filter in YYYY-MM-DD form. Enforced locally on final results.")
	toDate := flags.String("to-date", "", "Optional inclusive end date filter in YYYY-MM-DD form. Enforced locally on final results.")
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
	dateFilter, dateYearValue, dateErr := parseDateConstraint(yearFilter, strings.TrimSpace(*fromDate), strings.TrimSpace(*toDate))
	if dateErr != nil {
		return writeInvalidUsage(stdout, dateErr.Error(), map[string]any{
			"command": "search",
			"format":  "YYYY-MM-DD",
			"example": "--from-date 2026-04-01 --to-date 2026-05-18",
		})
	}
	if yearValue == "" {
		yearValue = dateYearValue
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
		filteredPapers := filterPapersByDateRange(result.Papers, dateFilter)
		if dateFilter != nil {
			sourceResults[descriptor.ID] = len(filteredPapers)
		} else {
			sourceResults[descriptor.ID] = result.Count
		}
		allPapers = append(allPapers, filteredPapers...)
	}

	deduped := rankPapers(query, paper.Dedupe(allPapers))
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
		PartialReason:    partialSuccessReason(failedSources, sourceErrors),
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
	if response.PartialReason != "" {
		b.WriteString(fmt.Sprintf("partial_success_reason: %s\n", response.PartialReason))
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

func parseDateConstraint(yearFilter *yearConstraint, fromValue, toValue string) (*dateConstraint, string, error) {
	if yearFilter != nil && (fromValue != "" || toValue != "") {
		return nil, "", fmt.Errorf("search --year cannot be combined with --from-date or --to-date")
	}

	if yearFilter != nil {
		start := time.Date(yearFilter.start, time.January, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(yearFilter.end, time.December, 31, 0, 0, 0, 0, time.UTC)
		return &dateConstraint{start: &start, end: &end}, "", nil
	}

	if fromValue == "" && toValue == "" {
		return nil, "", nil
	}

	var start *time.Time
	var end *time.Time
	if fromValue != "" {
		parsed, err := parseSearchDate(fromValue, "--from-date")
		if err != nil {
			return nil, "", err
		}
		start = &parsed
	}
	if toValue != "" {
		parsed, err := parseSearchDate(toValue, "--to-date")
		if err != nil {
			return nil, "", err
		}
		end = &parsed
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, "", fmt.Errorf("search --from-date must be on or before --to-date")
	}

	return &dateConstraint{start: start, end: end}, semanticYearRange(start, end), nil
}

func parseSearchDate(value, flagName string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("search %s must be YYYY-MM-DD", flagName)
	}
	year := parsed.Year()
	if year < 1900 || year > 2099 {
		return time.Time{}, fmt.Errorf("search %s must be YYYY-MM-DD", flagName)
	}
	return parsed, nil
}

func semanticYearRange(start, end *time.Time) string {
	if start == nil || end == nil {
		return ""
	}
	if start.Year() == end.Year() {
		return strconv.Itoa(start.Year())
	}
	return fmt.Sprintf("%04d-%04d", start.Year(), end.Year())
}

func filterPapersByDateRange(papers []paper.Paper, constraint *dateConstraint) []paper.Paper {
	if constraint == nil {
		return papers
	}
	if len(papers) == 0 {
		return []paper.Paper{}
	}

	filtered := make([]paper.Paper, 0, len(papers))
	for _, item := range papers {
		start, end, ok := paperDateBounds(item)
		if !ok {
			continue
		}
		if constraint.start != nil && start.Before(*constraint.start) {
			continue
		}
		if constraint.end != nil && end.After(*constraint.end) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func paperDateBounds(item paper.Paper) (time.Time, time.Time, bool) {
	value := strings.TrimSpace(item.PublishedDate)
	precision := strings.TrimSpace(item.DatePrecision)
	if precision == "" {
		precision = item.Normalized().DatePrecision
	}

	switch precision {
	case "day":
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		return parsed, parsed, true
	case "month":
		parsed, err := time.Parse("2006-01", value)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		end := parsed.AddDate(0, 1, -1)
		return parsed, end, true
	case "year":
		parsed, err := time.Parse("2006", value)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		end := time.Date(parsed.Year(), time.December, 31, 0, 0, 0, 0, time.UTC)
		return parsed, end, true
	default:
		return time.Time{}, time.Time{}, false
	}
}
