package cli

import (
	"fmt"
	"slices"
	"strings"
)

func partialSuccessReason(failedSources []string, sourceErrors map[string]string) string {
	if len(failedSources) == 0 {
		return ""
	}

	parts := make([]string, 0, len(failedSources))
	for _, source := range failedSources {
		reason := classifySourceFailure(sourceErrors[source])
		parts = append(parts, fmt.Sprintf("%s: %s", source, reason))
	}
	slices.Sort(parts)
	return strings.Join(parts, "; ")
}

func classifySourceFailure(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return "rate_limited"
	case strings.Contains(lower, "cannot unmarshal") || strings.Contains(lower, "invalid character") || strings.Contains(lower, "json") || strings.Contains(lower, "xml"):
		return "parse_error"
	case strings.Contains(lower, "connection reset") || strings.Contains(lower, "timeout") || strings.Contains(lower, "eof") || strings.Contains(lower, "temporary"):
		return "network_error"
	case strings.Contains(lower, "gated") || strings.Contains(lower, "credential") || strings.Contains(lower, "unsupported"):
		return "source_unavailable"
	default:
		return "upstream_error"
	}
}
