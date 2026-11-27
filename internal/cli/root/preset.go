package root

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Preset writes every tool's preset file from the rulebook's bindings, or, with
// check, says which differ from what would be written and writes nothing. The
// words live in internal/cli/cmd/preset.
func (k kitSession) Preset(ctx context.Context, cmd kit.Cmd, check bool) int {
	s := k.s
	c, ok := cmd.(*cobra.Command)
	if !ok {
		return s.die("the preset command was run with %T, not a cobra command", cmd)
	}
	root, ok := s.folder()
	if !ok {
		return exit.Failed
	}
	rep, err := coreapi.PresetCheck(ctx, s.p.World, root)
	if err != nil {
		return s.presetNotReady(err)
	}
	s.data = rep
	if check {
		return s.presetChecked(rep)
	}
	s.applied = applyGiven(c)
	code := s.ruleChange(ctx, c, nil, func(w port.Ports, ask model.ChangeAsk) (*coreapi.Change, *model.Refusal, error) {
		return coreapi.Preset(ctx, w, ask)
	})
	if s.applied && code == exit.Done {
		// the document is made after the run ends: only a run that ended done
		// wrote its files
		for i, f := range rep.Files {
			if f.State == coreapi.PresetDiffers {
				rep.Files[i].State = coreapi.PresetWritten
			}
		}
		s.data = rep
	}
	return code
}

// presetChecked ends a --check: done with the count when every file is what the
// book makes, otherwise findings, each differing file named with where it
// first differs. Each tool of hand_written_presets is named as hand-written,
// and each file the project owns as the project's, for neither is compared
// nor counted.
func (s *session) presetChecked(rep coreapi.PresetReport) int {
	var lead []model.Block
	for _, tool := range rep.HandWritten {
		lead = append(lead, model.Note{Text: tool + ": hand-written (hand_written_presets), so not compared"})
	}
	for _, path := range rep.Owned {
		lead = append(lead, model.Note{Text: path + ": the project's (its mark removed), so not compared"})
	}
	n := 0
	for _, f := range rep.Files {
		if f.State == coreapi.PresetDiffers {
			n++
			lead = append(lead, model.Note{Text: f.Path + ": differs from what the rulebook makes: " + f.First})
		}
	}
	if n == 0 {
		return s.end(ending{word: model.Done, what: fmt.Sprintf("%s compared, none differs", model.Plural(rep.Compared, "preset file"))}, lead...)
	}
	return s.end(ending{word: model.Findings, what: fmt.Sprintf("%d of %s differ", n, model.Plural(rep.Compared, "preset file")), next: "gragas preset --apply"}, lead...)
}

// presetNotReady ends a run whose presets cannot be made: not ready for a
// folder with no rulebook, findings, a missing settings input or no file to
// compare, failed for any other.
func (s *session) presetNotReady(err error) int {
	var findings *coreapi.FindingsError
	switch {
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.end(ending{word: model.NotReady, what: err.Error() + ". preset writes a rulebook's presets", next: "gragas init"})
	case errors.Is(err, coreapi.ErrNoPlugin):
		return s.end(ending{word: model.NotReady, what: coreapi.Reason(err), next: "gragas plugin add <folder>"})
	case errors.As(err, &findings):
		return s.end(ending{word: model.NotReady, what: err.Error(), next: "gragas validate"})
	case errors.Is(err, coreapi.ErrNoPresets):
		return s.end(ending{word: model.NotReady, what: "the rulebook makes no preset file, so there is nothing to compare", next: "gragas bind"})
	case errors.Is(err, coreapi.ErrSettingsInputMissing), errors.Is(err, coreapi.ErrNotReady), coreapi.IsOpenRecord(err):
		return s.end(ending{word: model.NotReady, what: coreapi.Reason(err), next: "gragas validate"})
	}
	return s.end(ending{word: model.Failed, what: "the presets could not be made: " + err.Error()})
}
