package connectors

import (
	"fmt"
	"strings"

	"github.com/jtsang4/search-paper-cli/internal/config"
	"github.com/jtsang4/search-paper-cli/internal/sources"
)

func New(id string, cfg config.Config) (sources.Connector, error) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "arxiv":
		return NewArxiv(), nil
	case "biorxiv":
		return NewBioRxiv(), nil
	case "medrxiv":
		return NewMedRxiv(), nil
	case "pubmed":
		return NewPubMed(), nil
	case "iacr":
		return NewIACR(), nil
	case "pmc":
		return NewPMC(), nil
	case "europepmc":
		return NewEuropePMC(), nil
	case "core":
		return NewCORE(cfg), nil
	case "doaj":
		return NewDOAJ(), nil
	case "base":
		return NewBASE(), nil
	case "zenodo":
		return NewZenodo(), nil
	case "hal":
		return NewHAL(), nil
	default:
		return nil, fmt.Errorf("connector %q is not implemented", id)
	}
}
