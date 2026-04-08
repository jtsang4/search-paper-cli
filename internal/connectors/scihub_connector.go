package connectors

import "github.com/jtsang4/search-paper-cli/internal/sources"

type SciHub struct{}

func NewSciHub() *SciHub {
	return &SciHub{}
}

func (c *SciHub) Descriptor() sources.Descriptor {
	return sources.Descriptor{
		ID:      "scihub",
		Enabled: true,
		Capabilities: sources.Capabilities{
			Search:   sources.CapabilityUnsupported,
			Download: sources.CapabilitySupported,
			Read:     sources.CapabilityUnsupported,
		},
	}
}

func (c *SciHub) Search(sources.SearchRequest) (sources.SearchResult, error) {
	return sources.SearchResult{}, nil
}

func (c *SciHub) Download(request sources.DownloadRequest) (sources.RetrievalResult, error) {
	return DownloadSciHub(firstNonEmpty(request.Paper.DOI, request.Paper.Title, request.Paper.PaperID, request.Paper.URL), request.SaveDir, "https://sci-hub.se")
}

func (c *SciHub) Read(sources.ReadRequest) (sources.RetrievalResult, error) {
	return unsupportedRead("scihub")
}
