package term_test

import (
	"os"
	"testing"

	"github.com/birdtable/gragas/internal/host/term"
)

// TestAnOutputThatIsNotATerminalHasNoWidth: a pipe and a character device such
// as /dev/null are no terminal, so Measure answers none and no width; the
// answer is the system's own for the descriptor (the kit's).
func TestAnOutputThatIsNotATerminalHasNoWidth(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = null.Close() })
	for name, f := range map[string]*os.File{"a pipe": w, os.DevNull: null} {
		if tty, width := term.Measure(f); tty || width != 0 {
			t.Errorf("Measure(%s) = %v, %d, want false, 0", name, tty, width)
		}
	}
}
