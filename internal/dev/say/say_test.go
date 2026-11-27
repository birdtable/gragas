package say_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/say"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// failing fails every write after its first n bytes are taken.
type failing struct {
	b strings.Builder
	n int
}

var errClosed = errors.New("the pipe is closed")

func (f *failing) Write(p []byte) (int, error) {
	if f.b.Len()+len(p) > f.n {
		return 0, errClosed
	}
	return f.b.Write(p)
}

// TestASpeakerKeepsTheFirstFailureAndWritesNothingAfterIt: text is written
// until a write fails; that failure is kept in Err, and no later text is
// written, though the writer would take it again. Risk: a closed pipe that
// a dev tool drops, so it reports done on output nobody read. Planted: Text
// writing whatever Err holds (the third line written, red here).
func TestASpeakerKeepsTheFirstFailureAndWritesNothingAfterIt(t *testing.T) {
	t.Parallel()
	w := &failing{n: 6}
	s := &say.Speaker{W: w}
	s.Text("one\n")
	if s.Err != nil || w.b.String() != "one\n" {
		t.Fatalf("the control: wrote %q, Err %v; want one line and no error", w.b.String(), s.Err)
	}
	s.Text("two\n")
	if !errors.Is(s.Err, errClosed) {
		t.Fatalf("a write that fails: Err %v; want %v", s.Err, errClosed)
	}
	w.n = 100
	s.Text("3\n")
	if w.b.String() != "one\n" || !errors.Is(s.Err, errClosed) {
		t.Errorf("after the failure: wrote %q, Err %v; want nothing more and the first failure kept", w.b.String(), s.Err)
	}
}
