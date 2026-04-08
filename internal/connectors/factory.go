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
		return NewArxiv(cfg), nil
	case "biorxiv":
		return NewBioRxiv(), nil
	case "medrxiv":
		return NewMedRxiv(), nil
	case "pubmed":
		return NewPubMed(), nil
	case "iacr":
		return NewIACR(), nil
	case "pmc":
		return NewPMC(cfg), nil
	case "europepmc":
		return NewEuropePMC(cfg), nil
	case "core":
		return NewCORE(cfg), nil
	case "semantic":
		return NewSemantic(cfg), nil
	case "crossref":
		return NewCrossref(), nil
	case "openalex":
		return NewOpenAlex(), nil
	case "google-scholar":
		return NewGoogleScholar(cfg), nil
	case "dblp":
		return NewDBLP(), nil
	case "openaire":
		return NewOpenAIRE(cfg), nil
	case "citeseerx":
		return NewCiteSeerX(), nil
	case "ssrn":
		return NewSSRN(), nil
	case "unpaywall":
		return NewUnpaywall(cfg), nil
	case "ieee":
		return NewIEEE(cfg), nil
	case "acm":
		return NewACM(cfg), nil
	case "doaj":
		return NewDOAJ(), nil
	case "base":
		return NewBASE(), nil
	case "zenodo":
		return NewZenodo(), nil
	case "hal":
		return NewHAL(), nil
	case "scihub":
		return NewSciHub(), nil
	default:
		return nil, fmt.Errorf("connector %q is not implemented", id)
	}
}
