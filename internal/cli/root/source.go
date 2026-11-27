package root

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The source family's two plans reach the core through the one form every
// rule change ends in (ruleChange): the questions, the dry run, the question
// at a terminal and the apply. The words live in internal/cli/cmd/source.

// SourceNew asks for what the request leaves out, then plans the record of the
// source.
func (k kitSession) SourceNew(ctx context.Context, cmd kit.Cmd, req model.NewSource) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the source command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	slug, kind := req.Slug, string(req.Kind)
	qs := []prompted{
		{flag: "--source-slug", ask: "What is the source, as lower-case words joined by hyphens?", to: &slug},
		{flag: "--source-kind", ask: "What kind is it (" + strings.Join(kindWords(), ", ") + ")?", to: &kind},
		{flag: "--title", ask: "What is it called?", to: &req.Title},
		{flag: "--locator", ask: "Where is it found: a URL, an ISBN, a DOI, an RFC or a decision slug?", to: &req.Locator},
	}
	if code, ok := s.asked(ctx, c, qs); !ok {
		return code
	}
	req.Slug, req.Kind = slug, model.SourceKind(kind)
	return s.ruleChange(ctx, c, nil, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.SourceNew(ctx, w, ask, req)
	})
}

// SourceEdit plans a change of the source the request names.
func (k kitSession) SourceEdit(ctx context.Context, cmd kit.Cmd, req model.EditSource) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the source command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	return s.ruleChange(ctx, c, []string{req.Slug}, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.SourceEdit(ctx, w, ask, req)
	})
}

// applyGiven is whether the command was given --apply.
func applyGiven(c *cobra.Command) bool {
	on, err := c.Flags().GetBool("apply")
	return err == nil && on
}

func kindWords() []string {
	var out []string
	for _, k := range model.SourceKinds() {
		out = append(out, fmt.Sprint(k))
	}
	return out
}
