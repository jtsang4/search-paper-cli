package connectors

import (
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
)

func TestConnectorFactoryFirstWave(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"acm",
		"arxiv",
		"biorxiv",
		"citeseerx",
		"medrxiv",
		"core",
		"crossref",
		"dblp",
		"doaj",
		"europepmc",
		"google-scholar",
		"base",
		"hal",
		"iacr",
		"ieee",
		"openalex",
		"openaire",
		"pmc",
		"pubmed",
		"semantic",
		"scihub",
		"ssrn",
		"unpaywall",
		"zenodo",
	} {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			connector, err := New(id, config.Config{})
			if err != nil {
				t.Fatalf("New(%q) error = %v", id, err)
			}
			if connector.Descriptor().ID != id {
				t.Fatalf("expected descriptor id %q, got %#v", id, connector.Descriptor())
			}
		})
	}
}
