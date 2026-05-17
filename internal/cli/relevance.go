package cli

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jtsang4/search-paper-cli/internal/paper"
)

var tokenPattern = regexp.MustCompile(`[a-z0-9]+`)

func rankPapers(query string, papers []paper.Paper) []paper.Paper {
	if len(papers) == 0 {
		return []paper.Paper{}
	}

	ranked := make([]paper.Paper, 0, len(papers))
	for _, item := range papers {
		normalized := item.Normalized()
		score, reasons := relevanceScore(query, normalized)
		if score > 0 {
			normalized.RelevanceScore = roundScore(score)
			normalized.RelevanceReasons = reasons
		}
		ranked = append(ranked, normalized)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].RelevanceScore > ranked[j].RelevanceScore
	})
	return ranked
}

func relevanceScore(query string, item paper.Paper) (float64, []string) {
	query = strings.ToLower(strings.TrimSpace(query))
	title := strings.ToLower(item.Title)
	abstract := strings.ToLower(item.Abstract)
	fullText := strings.TrimSpace(title + " " + abstract)
	if query == "" || fullText == "" {
		return 0, nil
	}

	terms := expandedQueryTerms(query)
	titleHits := 0
	abstractHits := 0
	score := 0.0
	for term, weight := range terms {
		if containsTerm(title, term) {
			titleHits++
			score += 6 * weight
		}
		if containsTerm(abstract, term) {
			abstractHits++
			score += 2 * weight
		}
	}

	if strings.Contains(title, query) {
		score += 10
	}
	if strings.Contains(abstract, query) {
		score += 4
	}

	reasons := make([]string, 0, 6)
	if titleHits > 0 {
		reasons = append(reasons, "title_match")
	}
	if abstractHits > 0 {
		reasons = append(reasons, "abstract_match")
	}

	if cssAmbiguousQuantumMismatch(query, fullText) {
		score -= 10
		reasons = append(reasons, "ambiguous_css_context_penalty")
	}

	if titleHits == 0 && abstractHits == 0 && score <= 0 {
		return 0, nil
	}

	if recency := recencyBoost(item.PublishedDate); recency > 0 {
		score += recency
		reasons = append(reasons, "recent")
	}
	if strings.TrimSpace(item.Abstract) != "" {
		score += 1
		reasons = append(reasons, "abstract_available")
	}
	if strings.TrimSpace(item.PDFURL) != "" {
		score += 1
		reasons = append(reasons, "pdf_available")
	}
	if sourceWeight := sourceReliabilityBoost(item.Source); sourceWeight > 0 {
		score += sourceWeight
		reasons = append(reasons, "source_weight")
	}

	return math.Max(score, 0), compactReasons(reasons)
}

func expandedQueryTerms(query string) map[string]float64 {
	terms := map[string]float64{}
	for _, token := range tokenPattern.FindAllString(strings.ToLower(query), -1) {
		if len(token) < 2 {
			continue
		}
		terms[token] = math.Max(terms[token], 1)
	}

	if hasAnyToken(terms, "frontend", "front", "ui", "web", "html", "css") {
		for _, term := range []string{
			"frontend", "front-end", "front end", "ui", "user interface", "web",
			"browser", "html", "css", "stylesheet", "style", "javascript",
			"typescript", "react", "vue", "svelte",
		} {
			terms[term] = math.Max(terms[term], 0.7)
		}
	}
	if strings.Contains(query, "code generation") || hasAnyToken(terms, "codegen", "generation", "generate", "generated") {
		for _, term := range []string{
			"code generation", "codegen", "program synthesis", "generate code",
			"generated code", "llm", "large language model",
		} {
			terms[term] = math.Max(terms[term], 0.7)
		}
	}

	if _, ok := terms["css"]; ok && !frontendContextTerms(terms) {
		terms["css"] = 0.25
	}

	return terms
}

func hasAnyToken(terms map[string]float64, tokens ...string) bool {
	for _, token := range tokens {
		if _, ok := terms[token]; ok {
			return true
		}
	}
	return false
}

func frontendContextTerms(terms map[string]float64) bool {
	return hasAnyToken(terms, "frontend", "front", "ui", "web", "html", "stylesheet", "javascript", "typescript")
}

func containsTerm(text, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	if strings.Contains(term, " ") || strings.Contains(term, "-") {
		return strings.Contains(text, term)
	}
	for _, token := range tokenPattern.FindAllString(text, -1) {
		if token == term {
			return true
		}
	}
	return false
}

func cssAmbiguousQuantumMismatch(query, text string) bool {
	if !strings.Contains(query, "css") {
		return false
	}
	if strings.Contains(query, "html") || strings.Contains(query, "frontend") || strings.Contains(query, "front-end") || strings.Contains(query, "ui") || strings.Contains(query, "web") {
		return strings.Contains(text, "quantum") && (strings.Contains(text, "error correction") || strings.Contains(text, "quantum code") || strings.Contains(text, "css code"))
	}
	return false
}

func recencyBoost(value string) float64 {
	start, _, ok := paperDateBounds(paper.Paper{PublishedDate: value})
	if !ok {
		return 0
	}
	age := time.Now().UTC().Year() - start.Year()
	switch {
	case age <= 1:
		return 1.5
	case age <= 3:
		return 1
	case age <= 5:
		return 0.5
	default:
		return 0
	}
}

func sourceReliabilityBoost(source string) float64 {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "semantic", "crossref", "arxiv", "pmc", "pubmed", "openalex":
		return 0.5
	default:
		return 0
	}
}

func compactReasons(input []string) []string {
	output := make([]string, 0, len(input))
	for _, reason := range input {
		if slices.Contains(output, reason) {
			continue
		}
		output = append(output, reason)
	}
	return output
}

func roundScore(score float64) float64 {
	return math.Round(score*100) / 100
}
