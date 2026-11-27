package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/surface"
)

// marksRecord is a marks file whose change record moved to sums at base,
// with each step after it.
func marksRecord(base string, steps ...string) string {
	text := "changes:\n  - {digest: \"sha256:" + strings.Repeat("a", 64) + "\", reason: \"an old line of the record\"}\n"
	text += "  - {base: \"" + base + "\", reason: \"the record moves to sums here\"}\n"
	for _, s := range steps {
		text += "  - {step: \"" + s + "\", reason: \"a change of the fixture\"}\n"
	}
	return text
}

// TestTheGateHoldsTheMarksBaseToTheBase is gragas#825's guard on the record:
// through the gate check, once the base's change record has a base line, a
// head whose line differs, whose record has none, whose marks file is gone or
// does not parse fails the gate, each saying why; a base read that fails, or a
// base marks file that does not parse, fails it too. A head that adds a step
// passes, and so does any head while the base's record has not moved or the
// base has no marks file.
func TestTheGateHoldsTheMarksBaseToTheBase(t *testing.T) {
	t.Parallel()
	b, b2, s, s2 := strings.Repeat("b", 64), strings.Repeat("e", 64), strings.Repeat("c", 64), strings.Repeat("d", 64)
	old := "changes:\n  - {digest: \"sha256:" + strings.Repeat("a", 64) + "\", reason: \"an old line of the record\"}\n"
	base := func(text string, err error) func(context.Context, string, string) ([]byte, error) {
		return func(context.Context, string, string) ([]byte, error) {
			if text == "" {
				return nil, err
			}
			return []byte(text), err
		}
	}
	gateWith := func(t *testing.T, head string, read func(context.Context, string, string) ([]byte, error)) (int, string) {
		t.Helper()
		dir := copyGate(t)
		if head != "" {
			p := filepath.Join(dir, filepath.FromSlash(surface.MarksFile))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(head), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		m := fakeMachine(t, dir)
		m.baseMarks = read
		code, out, errs := doGate(m, dir)
		return code, out + errs
	}
	for name, c := range map[string]struct {
		head string
		read func(context.Context, string, string) ([]byte, error)
	}{
		"the same record":                 {marksRecord(b, s), base(marksRecord(b, s), nil)},
		"a head that adds a step":         {marksRecord(b, s, s2), base(marksRecord(b, s), nil)},
		"a base line with a new reason":   {strings.Replace(marksRecord(b, s), "moves to sums here", "moves to sums now", 1), base(marksRecord(b, s), nil)},
		"a base whose record never moved": {marksRecord(b2), base(old, nil)},
		"a base with no marks file":       {marksRecord(b2), base("", nil)},
	} {
		t.Run(name+" passes", func(t *testing.T) {
			t.Parallel()
			if code, out := gateWith(t, c.head, c.read); code != exitDone {
				t.Errorf("exit %d, want %d\n%s", code, exitDone, out)
			}
		})
	}
	for name, c := range map[string]struct {
		head, want string
		read       func(context.Context, string, string) ([]byte, error)
	}{
		"a base line edited":        {marksRecord(b2, s), "the change record's base is not main's, the base " + b, base(marksRecord(b, s), nil)},
		"a record moved back":       {old, "the change record has no base, and main's has the base " + b, base(marksRecord(b), nil)},
		"no marks file at the head": {"", "the change record has no base, and main's has the base " + b, base(marksRecord(b), nil)},
		"a head marks file broken":  {marksRecord(b) + "  - {base: \"" + b2 + "\", reason: \"a second base here\"}\n", "a second base", base(marksRecord(b), nil)},
		"a base read that fails":    {marksRecord(b), "the base cannot be read", base("", errors.New("the base cannot be read"))},
		"a base marks file broken":  {marksRecord(b), "the base's " + surface.MarksFile, base("changes: [{step: x}]\n", nil)},
	} {
		t.Run(name+" fails", func(t *testing.T) {
			t.Parallel()
			code, out := gateWith(t, c.head, c.read)
			if code != exitFailed || !strings.Contains(out, c.want) {
				t.Errorf("exit %d\n%s\nwant a failure saying %q", code, out, c.want)
			}
		})
	}
}
