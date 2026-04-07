package connectors

import (
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
)

func TestConnectorFactoryFirstWave(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"arxiv",
		"biorxiv",
		"medrxiv",
		"pubmed",
		"iacr",
		"pmc",
		"europepmc",
		"core",
		"doaj",
		"base",
		"zenodo",
		"hal",
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
