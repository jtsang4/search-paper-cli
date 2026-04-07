package sources

import (
	"fmt"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
)

type CapabilityState string

const (
	CapabilitySupported       CapabilityState = "supported"
	CapabilityRecordDependent CapabilityState = "record_dependent"
	CapabilityInformational   CapabilityState = "informational"
	CapabilityUnsupported     CapabilityState = "unsupported"
	CapabilityGated           CapabilityState = "gated"
)

type Capabilities struct {
	Search   CapabilityState `json:"search"`
	Download CapabilityState `json:"download"`
	Read     CapabilityState `json:"read"`
}

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
	Year  string `json:"year,omitempty"`
}

type SearchResult struct {
	Count  int           `json:"count"`
	Papers []paper.Paper `json:"papers"`
}

type DownloadRequest struct {
	Paper   paper.Paper `json:"paper"`
	SaveDir string      `json:"save_dir,omitempty"`
}

type ReadRequest struct {
	Paper   paper.Paper `json:"paper"`
	SaveDir string      `json:"save_dir,omitempty"`
}

type RetrievalState string

const (
	RetrievalStateDownloaded                  RetrievalState = "downloaded"
	RetrievalStateExtracted                   RetrievalState = "extracted"
	RetrievalStateDownloadedButNotExtractable RetrievalState = "downloaded_but_not_extractable"
	RetrievalStateInformational               RetrievalState = "informational"
	RetrievalStateUnsupported                 RetrievalState = "unsupported"
	RetrievalStateNotFound                    RetrievalState = "not_found"
	RetrievalStateFailed                      RetrievalState = "failed"
)

type RetrievalResult struct {
	State   RetrievalState `json:"state"`
	Path    string         `json:"path,omitempty"`
	Content string         `json:"content,omitempty"`
	Message string         `json:"message,omitempty"`
}

type Descriptor struct {
	ID            string       `json:"id"`
	Enabled       bool         `json:"enabled"`
	DisableReason string       `json:"disable_reason"`
	Capabilities  Capabilities `json:"capabilities"`
}

type Connector interface {
	Descriptor() Descriptor
	Search(SearchRequest) (SearchResult, error)
	Download(DownloadRequest) (RetrievalResult, error)
	Read(ReadRequest) (RetrievalResult, error)
}

type definition struct {
	id                 string
	capabilities       Capabilities
	gatedWhenMissing   func(config.Config) bool
	missingRequirement string
}

var definitions = []definition{
	{id: "acm", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityUnsupported, Read: CapabilityUnsupported}, gatedWhenMissing: func(cfg config.Config) bool { return strings.TrimSpace(cfg.ACMAPIKey) == "" }, missingRequirement: "PAPER_SEARCH_MCP_ACM_API_KEY"},
	{id: "arxiv", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "base", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "biorxiv", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "citeseerx", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "core", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "crossref", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityInformational, Read: CapabilityInformational}},
	{id: "dblp", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityUnsupported, Read: CapabilityUnsupported}},
	{id: "doaj", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "europepmc", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "google-scholar", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityInformational, Read: CapabilityInformational}},
	{id: "hal", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "iacr", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "ieee", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityUnsupported, Read: CapabilityUnsupported}, gatedWhenMissing: func(cfg config.Config) bool { return strings.TrimSpace(cfg.IEEEAPIKey) == "" }, missingRequirement: "PAPER_SEARCH_MCP_IEEE_API_KEY"},
	{id: "medrxiv", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "openalex", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityInformational, Read: CapabilityInformational}},
	{id: "openaire", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityUnsupported, Read: CapabilityUnsupported}},
	{id: "pmc", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilitySupported, Read: CapabilitySupported}},
	{id: "pubmed", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityInformational, Read: CapabilityInformational}},
	{id: "semantic", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "ssrn", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
	{id: "unpaywall", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityUnsupported, Read: CapabilityUnsupported}},
	{id: "zenodo", capabilities: Capabilities{Search: CapabilitySupported, Download: CapabilityRecordDependent, Read: CapabilityRecordDependent}},
}

func List(cfg config.Config) []Descriptor {
	descriptors := make([]Descriptor, 0, len(definitions))
	for _, item := range definitions {
		descriptor := Descriptor{
			ID:            item.id,
			Enabled:       true,
			DisableReason: "",
			Capabilities:  item.capabilities,
		}

		if item.gatedWhenMissing != nil && item.gatedWhenMissing(cfg) {
			descriptor.Enabled = false
			descriptor.DisableReason = "missing required credential: " + item.missingRequirement
			descriptor.Capabilities = Capabilities{
				Search:   CapabilityGated,
				Download: CapabilityGated,
				Read:     CapabilityGated,
			}
		}

		descriptors = append(descriptors, descriptor)
	}

	return descriptors
}

func ValidIDs() []string {
	ids := make([]string, 0, len(definitions))
	for _, item := range definitions {
		ids = append(ids, item.id)
	}
	return ids
}

func Select(cfg config.Config, requested []string) ([]Descriptor, []string) {
	if len(requested) == 0 {
		return List(cfg), nil
	}

	valid := map[string]struct{}{}
	for _, id := range ValidIDs() {
		valid[id] = struct{}{}
	}

	selected := map[string]struct{}{}
	invalid := make([]string, 0)
	seenInvalid := map[string]struct{}{}
	for _, raw := range requested {
		id := normalizeID(raw)
		if id == "" {
			continue
		}
		if _, ok := valid[id]; !ok {
			if _, seen := seenInvalid[id]; !seen {
				invalid = append(invalid, id)
				seenInvalid[id] = struct{}{}
			}
			continue
		}
		selected[id] = struct{}{}
	}

	if len(invalid) != 0 {
		return nil, invalid
	}

	all := List(cfg)
	filtered := make([]Descriptor, 0, len(selected))
	for _, source := range all {
		if _, ok := selected[source.ID]; ok {
			filtered = append(filtered, source)
		}
	}

	return filtered, nil
}

func normalizeID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidCapabilityStates() []CapabilityState {
	return []CapabilityState{
		CapabilitySupported,
		CapabilityRecordDependent,
		CapabilityInformational,
		CapabilityUnsupported,
		CapabilityGated,
	}
}

type StubConnector struct {
	DescriptorValue Descriptor
	SearchResults   []paper.Paper
	SearchError     error
	DownloadResult  *RetrievalResult
	DownloadError   error
	ReadResult      *RetrievalResult
	ReadError       error
}

func NewStubConnector(connector StubConnector) StubConnector {
	if connector.DescriptorValue.ID == "" {
		connector.DescriptorValue.ID = "stub"
	}
	return connector
}

func (s StubConnector) Descriptor() Descriptor {
	if s.DescriptorValue.ID == "" {
		s.DescriptorValue.ID = "stub"
	}
	return s.DescriptorValue
}

func (s StubConnector) Search(request SearchRequest) (SearchResult, error) {
	if s.SearchError != nil {
		return SearchResult{}, s.SearchError
	}

	papers := normalizeSearchPapers(s.SearchResults)
	if request.Limit > 0 && request.Limit < len(papers) {
		papers = papers[:request.Limit]
	}

	return SearchResult{
		Count:  len(papers),
		Papers: papers,
	}, nil
}

func (s StubConnector) Download(DownloadRequest) (RetrievalResult, error) {
	if s.DownloadError != nil {
		return RetrievalResult{}, s.DownloadError
	}
	if s.DownloadResult != nil {
		return *s.DownloadResult, nil
	}
	return RetrievalResult{
		State:   RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q does not implement download in the stub connector", s.Descriptor().ID),
	}, nil
}

func (s StubConnector) Read(ReadRequest) (RetrievalResult, error) {
	if s.ReadError != nil {
		return RetrievalResult{}, s.ReadError
	}
	if s.ReadResult != nil {
		return *s.ReadResult, nil
	}
	return RetrievalResult{
		State:   RetrievalStateUnsupported,
		Message: fmt.Sprintf("source %q does not implement read in the stub connector", s.Descriptor().ID),
	}, nil
}

func normalizeSearchPapers(input []paper.Paper) []paper.Paper {
	if len(input) == 0 {
		return []paper.Paper{}
	}

	result := make([]paper.Paper, 0, len(input))
	for _, item := range input {
		normalized := item.Normalized()
		result = append(result, normalized)
	}
	return result
}
