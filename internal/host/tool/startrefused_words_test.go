package tool_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"

	"github.com/birdtable/go-proc/proc"
)

// TestEachRefusalOfGoProcsIsSaidInThisAdaptersWords: go-proc starts no
// batch file on Windows and no file that is not a program file, and says so
// by proc.ErrBatch and proc.ErrNotProgram; no program whose
// path holds = on any system. Each is said in this adapter's words, with the
// path and a fix that works: name the program file itself, or a folder with
// no =. Any other refusal is reported as go-proc's, with no fix of the
// tool's. The errors are planted as go-proc wraps them, so the mapping holds
// on every system, not only where go-proc returns them; the two npm shapes
// are a .cmd shim and the extensionless file beside it.
// Risk: a .cmd tool refused with go-proc's text and no fix, or one refused
// with a fix that cannot fix it ("run it at the sandboxed level").
func TestEachRefusalOfGoProcsIsSaidInThisAdaptersWords(t *testing.T) {
	t.Parallel()
	const programFix = "name the program file of lint-a itself,"
	for name, c := range map[string]struct {
		path      string
		err       error
		says, fix string
		named     bool
	}{
		"a .cmd shim":        {"/tools/lint-a-shim", fmt.Errorf("%w: /tools/lint-a-shim", proc.ErrBatch), "is a batch file, which Windows runs through", programFix, true},
		"the file beside it": {"/tools/lint-a", fmt.Errorf("%w: /tools/lint-a", proc.ErrNotProgram), "is not a program file", programFix, true},
		"a path holding =":   {"/tools=1/lint-a", fmt.Errorf("%w: Path holds =", proc.ErrRefused), "holds =, and go-proc starts no program whose path holds =", "install lint-a in a folder whose path holds no =,", true},
		"any other refusal":  {"/tools/lint-a", fmt.Errorf("%w: the set-up", proc.ErrRefused), "go-proc refused to start lint-a: ", "report the fault with this line", false},
	} {
		// No subtest: t.Run reads as the adapter's Run, which signals, and
		// this test starts nothing.
		err := tool.StartRefused("lint-a", c.path, c.err)
		var r *tool.Refused
		if !errors.As(err, &r) || !errors.Is(err, port.ErrRefused) {
			t.Errorf("%s: %v gives %v, want refused", name, c.err, err)
			continue
		}
		if o := r.Observed(); !strings.Contains(o, c.says) || c.named && !strings.Contains(o, filepath.FromSlash(c.path)) {
			t.Errorf("%s: the refusal says %q; want the path %s and %q", name, o, filepath.FromSlash(c.path), c.says)
		}
		if h := r.Hint(); !strings.HasPrefix(h, c.fix) || strings.Contains(h, "sandboxed") {
			t.Errorf("%s: the fix is %q; want %q", name, h, c.fix)
		}
	}
}
