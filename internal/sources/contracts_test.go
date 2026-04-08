package sources

import (
	"slices"
	"testing"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/paper"
)

func TestCapabilityEnums(t *testing.T) {
	t.Parallel()

	want := []CapabilityState{
		CapabilitySupported,
		CapabilityRecordDependent,
		CapabilityInformational,
		CapabilityUnsupported,
		CapabilityGated,
	}
	got := ValidCapabilityStates()
	if !slices.Equal(got, want) {
		t.Fatalf("ValidCapabilityStates() mismatch\nwant: %v\ngot:  %v", want, got)
	}
}

func TestConnectorInterfaces(t *testing.T) {
	t.Parallel()

	t.Run("search results expose normalized papers", func(t *testing.T) {
		helper := NewStubConnector(StubConnector{
			DescriptorValue: Descriptor{
				ID:      "stub",
				Enabled: true,
				Capabilities: Capabilities{
					Search:   CapabilitySupported,
					Download: CapabilityUnsupported,
					Read:     CapabilityUnsupported,
				},
			},
			SearchResults: []paper.Paper{
				{
					PaperID: "  Paper-1 ",
					Title:   "  Shared   Contract ",
					Authors: []string{" Alice  Smith "},
					Source:  " Stub ",
				},
			},
		})

		var connector Connector = helper
		result, err := connector.Search(SearchRequest{Query: "contract", Limit: 5})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}

		if result.Count != 1 {
			t.Fatalf("expected count 1, got %d", result.Count)
		}
		if len(result.Papers) != 1 {
			t.Fatalf("expected one paper, got %#v", result.Papers)
		}
		if result.Papers[0].PaperID != "Paper-1" || result.Papers[0].Title != "Shared Contract" || result.Papers[0].Source != "stub" {
			t.Fatalf("expected normalized paper output, got %#v", result.Papers[0])
		}
		if result.Papers[0].Authors == nil {
			t.Fatalf("expected normalized paper authors to be a non-nil slice, got %#v", result.Papers[0])
		}
	})

	t.Run("empty search results use empty paper slices", func(t *testing.T) {
		helper := NewStubConnector(StubConnector{
			DescriptorValue: Descriptor{
				ID:      "stub",
				Enabled: true,
				Capabilities: Capabilities{
					Search:   CapabilitySupported,
					Download: CapabilityUnsupported,
					Read:     CapabilityUnsupported,
				},
			},
		})

		result, err := helper.Search(SearchRequest{Query: "none", Limit: 5})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if result.Count != 0 {
			t.Fatalf("expected count 0, got %d", result.Count)
		}
		if result.Papers == nil {
			t.Fatalf("expected empty papers slice, got nil")
		}
	})

	t.Run("descriptor enablement follows capability gating config", func(t *testing.T) {
		cfg := config.Config{}
		descriptors := List(cfg)
		ieee := findDescriptor(t, descriptors, "ieee")
		if ieee.Enabled {
			t.Fatalf("expected ieee disabled without key, got %#v", ieee)
		}
		if ieee.Capabilities.Search != CapabilityGated {
			t.Fatalf("expected ieee gated capability, got %#v", ieee)
		}

		cfg.IEEEAPIKey = "configured"
		descriptors = List(cfg)
		ieee = findDescriptor(t, descriptors, "ieee")
		if !ieee.Enabled {
			t.Fatalf("expected ieee enabled with key, got %#v", ieee)
		}
		if ieee.Capabilities.Search != CapabilitySupported {
			t.Fatalf("expected ieee search supported with key, got %#v", ieee)
		}

		scihub := findDescriptor(t, descriptors, "scihub")
		if !scihub.Enabled || scihub.Capabilities.Download != CapabilitySupported || scihub.Capabilities.Search != CapabilityUnsupported {
			t.Fatalf("expected scihub descriptor to remain enabled download-only, got %#v", scihub)
		}
	})

	t.Run("test helper provides unsupported defaults for retrieval", func(t *testing.T) {
		helper := NewStubConnector(StubConnector{
			DescriptorValue: Descriptor{
				ID:      "stub",
				Enabled: true,
				Capabilities: Capabilities{
					Search:   CapabilitySupported,
					Download: CapabilityUnsupported,
					Read:     CapabilityUnsupported,
				},
			},
		})

		downloadResult, err := helper.Download(DownloadRequest{Paper: paper.Paper{PaperID: "p-1"}})
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}
		if downloadResult.State != RetrievalStateUnsupported {
			t.Fatalf("expected unsupported download state, got %#v", downloadResult)
		}

		readResult, err := helper.Read(ReadRequest{Paper: paper.Paper{PaperID: "p-1"}})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if readResult.State != RetrievalStateUnsupported {
			t.Fatalf("expected unsupported read state, got %#v", readResult)
		}
	})
}

func findDescriptor(t *testing.T, descriptors []Descriptor, id string) Descriptor {
	t.Helper()

	for _, descriptor := range descriptors {
		if descriptor.ID == id {
			return descriptor
		}
	}

	t.Fatalf("expected descriptor %q in %#v", id, descriptors)
	return Descriptor{}
}
