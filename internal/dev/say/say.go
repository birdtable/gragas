// Package say is the dev tools' shared first-error writer (gragas#1897): the
// one Speaker the cmd/gragas-* tools write their output and diagnostics
// through, in place of a copy in each. Not product.
package say

import "io"

// Speaker writes text to W and keeps the first failure to write in Err, so a
// closed pipe is an error the run reports and not one it drops.
type Speaker struct {
	W   io.Writer
	Err error
}

// Text writes t, unless a write has failed already.
func (s *Speaker) Text(t string) {
	if s.Err == nil {
		_, s.Err = io.WriteString(s.W, t)
	}
}
