package wire

import (
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// grantError is the refusal of a step that names a program the manifest does
// not grant. It is the AnswerError, found by errors.As with its path, and it
// is also port.ErrGrant, so a caller's errors.Is(err, port.ErrGrant) is true
// as the port says. Its text is the AnswerError's, byte for byte.
type grantError struct{ answer *plugin.AnswerError }

func (e *grantError) Error() string { return e.answer.Error() }

// Unwrap gives the AnswerError first and the port's sentinel second.
func (e *grantError) Unwrap() []error { return []error{e.answer, port.ErrGrant} }
