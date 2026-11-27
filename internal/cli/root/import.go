package root

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Import asks for what the request leaves out, then plans the import of the
// guide and the configs it names. A guide needs the place its rules go and its
// source; configs need the tool and the target they are of.
func (k kitSession) Import(ctx context.Context, cmd kit.Cmd, in model.ImportInput) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the import command was run with %T, not a cobra command", cmd)
	}
	s.applied = applyGiven(c)
	target := string(in.Target)
	noGuide := func() bool { return in.GuideDir == "" }
	noConfig := func() bool { return len(in.Configs) == 0 }
	qs := []prompted{
		{flag: "--into-part", ask: "Which part are the rules drafted into?", to: &in.Part, skip: noGuide},
		{flag: "--into-concern", ask: "Which concern of that part?", to: &in.Concern, skip: noGuide},
		{flag: "--guide-slug", ask: "What is the guide's source, as lower-case words joined by hyphens?", to: &in.Guide.Slug, skip: noGuide},
		{flag: "--guide-title", ask: "What is the guide called?", to: &in.Guide.Title, skip: noGuide},
		{flag: "--guide-url", ask: "Where is the guide found?", to: &in.Guide.URL, skip: noGuide},
		{flag: "--guide-tag", ask: "Which release of the guide is imported?", to: &in.Guide.Tag, skip: noGuide},
		{flag: "--tool", ask: "Which tool are the configs of?", to: &in.Tool, skip: noConfig},
		{flag: "--target", ask: "What are the configs about (a target type)?", to: &target, skip: noConfig},
	}
	if code, ok := s.asked(ctx, c, qs); !ok {
		return code
	}
	in.Target = model.TargetType(target)
	return s.ruleChange(ctx, c, guideArgs(in), func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.Import(ctx, w, ask, in)
	})
}

// guideArgs are the words typed after the command's name: the guide's folder, if given.
func guideArgs(in model.ImportInput) []string {
	if in.GuideDir == "" {
		return nil
	}
	return []string{in.GuideDir}
}
