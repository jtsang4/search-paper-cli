package paper

import (
	"reflect"
	"slices"
	"testing"
)

func TestPaperModel(t *testing.T) {
	t.Parallel()

	t.Run("normalized paper trims and canonicalizes fields", func(t *testing.T) {
		p := Paper{
			PaperID:       "  paper-123  ",
			Title:         "  A   Shared\nPaper Model  ",
			Authors:       []string{"  Alice   Smith  ", "", "Bob\tJones"},
			Abstract:      "  This\n is\t a test.  ",
			DOI:           " HTTPS://doi.org/10.1000/ABC-123  ",
			PublishedDate: " 2024-05-01 ",
			PDFURL:        " https://example.com/paper.pdf ",
			URL:           " https://example.com/paper ",
			Source:        " ArXiv ",
		}

		got := p.Normalized()
		want := Paper{
			PaperID:       "paper-123",
			Title:         "A Shared Paper Model",
			Authors:       []string{"Alice Smith", "Bob Jones"},
			Abstract:      "This is a test.",
			DOI:           "10.1000/abc-123",
			PublishedDate: "2024-05-01",
			PDFURL:        "https://example.com/paper.pdf",
			URL:           "https://example.com/paper",
			Source:        "arxiv",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Normalized() mismatch\nwant: %#v\ngot:  %#v", want, got)
		}
	})

	t.Run("dedupe prefers doi then normalized title authors then paper id", func(t *testing.T) {
		doiKey := Paper{
			DOI:     "doi:10.1000/Test",
			Title:   "Ignored",
			Authors: []string{"Ignored"},
			PaperID: "ignored-id",
		}.IdentityKey()
		if doiKey != "doi:10.1000/test" {
			t.Fatalf("expected doi identity key, got %q", doiKey)
		}

		titleAuthorsKey := Paper{
			Title:   "  Shared\tPaper ",
			Authors: []string{" Alice  Smith ", "Bob Jones"},
			PaperID: "ignored-id",
		}.IdentityKey()
		if titleAuthorsKey != "title-authors:shared paper|alice smith|bob jones" {
			t.Fatalf("expected title/authors identity key, got %q", titleAuthorsKey)
		}

		paperIDKey := Paper{PaperID: "  Mixed-Case-ID  "}.IdentityKey()
		if paperIDKey != "paper-id:mixed-case-id" {
			t.Fatalf("expected paper id identity key, got %q", paperIDKey)
		}
	})

	t.Run("dedupe keeps first normalized survivor in deterministic order", func(t *testing.T) {
		papers := []Paper{
			{
				PaperID: "first",
				Title:   "Shared Paper",
				Authors: []string{"Alice Smith", "Bob Jones"},
				DOI:     "10.1000/example",
				Source:  "semantic",
			},
			{
				PaperID: "second",
				Title:   "Shared Paper",
				Authors: []string{"Alice Smith", "Bob Jones"},
				DOI:     "https://doi.org/10.1000/example",
				Source:  "crossref",
			},
			{
				PaperID: "Third-ID",
				Title:   " ",
				Authors: nil,
				Source:  "arxiv",
			},
			{
				PaperID: " third-id ",
				Title:   " ",
				Authors: nil,
				Source:  "pmc",
			},
		}

		got := Dedupe(papers)
		if len(got) != 2 {
			t.Fatalf("expected 2 deduped papers, got %d: %#v", len(got), got)
		}

		if got[0].PaperID != "first" || got[0].Source != "semantic" || got[0].DOI != "10.1000/example" {
			t.Fatalf("expected first paper to survive doi duplicate, got %#v", got[0])
		}
		if got[1].PaperID != "Third-ID" || got[1].Source != "arxiv" {
			t.Fatalf("expected first paper-id duplicate to survive, got %#v", got[1])
		}

		if !slices.Equal(got[0].Authors, []string{"Alice Smith", "Bob Jones"}) {
			t.Fatalf("expected authors to remain normalized, got %#v", got[0].Authors)
		}
	})
}
