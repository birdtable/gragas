package root

// How a run ends when its settings cannot be used: before the command runs,
// with the word the problem's kind is. What a person chose is stripped of
// control sequences where the Terminal port draws it.

import (
	"errors"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/model"
)

// endSettings draws the ending of the settings problem err and returns its
// exit code. A refusal carries the problem's own next line where config
// gives one, as for a variable that names no value it takes (unset it, or
// set it to one), and else "edit the settings file", since the fix is in
// the file.
func (s *session) endSettings(err error) int {
	p, ok := errors.AsType[*config.Problem](err)
	if !ok {
		return s.end(ending{word: model.Failed, what: "the settings could not be read: " + err.Error()})
	}
	switch p.Kind {
	case config.NotReady:
		return s.end(ending{word: model.NotReady, what: p.What, next: p.Next})
	case config.Failed:
		return s.end(ending{word: model.Failed, what: p.What})
	case config.Refused:
		// below, with any kind this build does not know: refused
	}
	next := p.Next
	if next == "" {
		next = "edit the settings file"
	}
	return s.end(ending{word: model.Refused, what: p.What, next: next})
}
