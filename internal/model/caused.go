package model

import (
	"errors"
	"fmt"
)

// Caused is a Failure with what was being done and the error behind it: the
// body of an adapter's Failed, which embeds it so that each adapter's type
// stays its own to errors.As (code.duplication.1: git's and the signer's were
// one copy).
type Caused struct {
	Failure
	// What says what was being done.
	What string
	// Err is the cause.
	Err error
}

func (c Caused) Error() string { return fmt.Sprintf("%s (run %s): %v", c.What, c.Run(), c.Err) }

func (c Caused) Unwrap() error { return c.Err }

// NewCaused is a Caused of kind in run. Both are required, as NewFailure
// asks; when either is missing the error still answers what and the cause,
// joined to why the failure could not be built.
func NewCaused(kind Kind, run, what string, err error) (Caused, error) {
	f, ferr := NewFailure(kind, run)
	if ferr != nil {
		return Caused{}, errors.Join(fmt.Errorf("%s: %w", what, err), ferr)
	}
	return Caused{Failure: f, What: what, Err: err}, nil
}
