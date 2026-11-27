package source

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// newCmd is gragas source new.
func newCmd(s kit.Session) *cobra.Command {
	var slug, kind, title, locator, attribution string
	c := &cobra.Command{
		Use:   "new",
		Short: "Record a source",
		Long: `Record a source: its slug, its kind (standard, guide, study,
incident, decision-record, vendor-document or own-decision), its
title, where it is found and, if there is one, who wrote it. At a
terminal the missing answers are asked, each with the flag that
gives it in a script. A source that is recorded already, with the
same words, ends "nothing to do".

  gragas source new
  gragas source new --source-slug rfc-9110 --source-kind standard \
    --title "HTTP Semantics" --locator "RFC 9110" --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		req := model.NewSource{Slug: slug, Kind: model.SourceKind(kind), Title: title, Locator: locator, Attribution: attribution}
		return s.Exit(s.SourceNew(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&slug, "source-slug", "", "The source, as lower-case words joined by hyphens")
	f.StringVar(&kind, "source-kind", "", "What the source is: one of the kinds named above")
	f.StringVar(&title, "title", "", "What it is called")
	f.StringVar(&locator, "locator", "", "A URL, an ISBN, a DOI, an RFC or a decision slug")
	f.StringVar(&attribution, "attribution", "", "Who wrote it")
	f.Bool("apply", false, "Write the files")
	return c
}
