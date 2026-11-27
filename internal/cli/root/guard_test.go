package root

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The hook's input is read whole, its lines joined as they came; an input of
// exactly the cap is read, one byte more is an error and nothing is kept, so
// the guard refuses it rather than judge a part.
// The cap itself is held at its edge by core/api's test of ParseHookEdit.
func TestHookInputReadsEveryLineAndNoMoreThanTheCap(t *testing.T) {
	t.Parallel()
	read := func(lines ...string) ([]byte, error) {
		w := porttest.NewWorld(t)
		w.SeedTerminal(porttest.Term{Input: lines})
		return hookInput(t.Context(), w.Ports().Terminal)
	}
	if got, err := read(`{"tool_name":`, `"Write"}`); err != nil || string(got) != "{\"tool_name\":\n\"Write\"}" {
		t.Fatalf("got %q, %v", got, err)
	}
	half := strings.Repeat("x", coreapi.MaxHookInput/2)
	if got, err := read(half, half[1:]); err != nil || len(got) != coreapi.MaxHookInput {
		t.Fatalf("an input of the cap read %d bytes, %v", len(got), err)
	}
	if got, err := read(half, half); err == nil || got != nil {
		t.Fatalf("an input a byte past the cap read %d bytes, %v", len(got), err)
	}
	if got, err := read(); err != nil || len(got) != 0 {
		t.Fatalf("an empty input: %q, %v", got, err)
	}
}

// hintProblems checks each guard hint against the commands a tree registers
// : a hint names a command the tree has, or it says "(not built yet)"
// of one it lacks, and never the marker of a command it has.
func hintProblems(root *cobra.Command, hints []string) []string {
	const marker = " (not built yet)"
	var bad []string
	for _, hint := range hints {
		if hint == "" { // a field no command changes
			continue
		}
		marked := strings.HasSuffix(hint, marker)
		// " or gragas " joins two commands; a bare " or " joins two flags.
		for one := range strings.SplitSeq(strings.TrimSuffix(hint, marker), " or gragas ") {
			if !strings.HasPrefix(one, "gragas ") {
				one = "gragas " + one
			}
			words := hintWords(one)
			switch have := hasCommand(root, words); {
			case len(words) == 0:
				bad = append(bad, fmt.Sprintf("%q names no command", hint))
			case !have && !marked:
				bad = append(bad, fmt.Sprintf("%q names %q, which the root does not register", hint, strings.Join(words, " ")))
			case have && marked:
				bad = append(bad, fmt.Sprintf("%q still says not built of %q, which the root registers", hint, strings.Join(words, " ")))
			}
		}
	}
	return bad
}

// hintWords is the command a hint line names: the words after "gragas" up to
// the first rule id or flag.
func hintWords(line string) []string {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "gragas" {
		return nil
	}
	var words []string
	for _, w := range fields[1:] {
		if strings.HasPrefix(w, "-") || strings.HasPrefix(w, "<") || strings.Contains(w, ".") {
			break
		}
		words = append(words, w)
	}
	return words
}

func hasCommand(c *cobra.Command, words []string) bool {
	for _, w := range words {
		i := slices.IndexFunc(c.Commands(), func(k *cobra.Command) bool { return k.Name() == w })
		if i < 0 {
			return false
		}
		c = c.Commands()[i]
	}
	return len(words) > 0
}

// TestEveryGuardHintNamesACommandTheRootRegisters: the hints the guard gives
// (S59) each name a registered command, and a hint says "not built yet" only
// of one the root lacks. archive is planned, so the plants use it as the
// command that is still not built.
func TestEveryGuardHintNamesACommandTheRootRegisters(t *testing.T) {
	t.Parallel()
	root := tree(t)
	hints := coreapi.GuardCommands()
	if bad := hintProblems(root, hints); len(bad) > 0 {
		t.Errorf("the guard's hints and the root disagree:\n%s", strings.Join(bad, "\n"))
	}
	// the lists of sources and terms are in the set the check reaches
	for _, want := range []string{"gragas source edit <id> --title", "gragas term retire <id>"} {
		if !slices.Contains(hints, want) {
			t.Errorf("GuardCommands lacks the hint %q", want)
		}
	}
	for _, c := range []struct{ name, hint, want string }{
		{"a built command marked as not built", "gragas rule promote a.b.1 (not built yet)", "still says not built"},
		{"a command the root lacks", "gragas rule frobnicate a.b.1", "does not register"},
		{"a command marked built that is not", "gragas archive a.b.1", "does not register"},
	} {
		if bad := hintProblems(root, []string{c.hint}); len(bad) != 1 || !strings.Contains(bad[0], c.want) {
			t.Errorf("%s: the check said %v, want one problem with %q", c.name, bad, c.want)
		}
	}
	if bad := hintProblems(root, []string{"gragas archive a.b.1 (not built yet)"}); len(bad) != 0 {
		t.Errorf("a command the root lacks, marked as not built: the check said %v, want none", bad)
	}
}

// drainTerm is a terminal whose every Line is refused, as a line past the
// cap is, and whose Drain reports that it ran and then waits for release,
// as the read of a blocking pipe whose writer never closes does: it does
// not end with its ctx.
type drainTerm struct {
	port.Terminal
	ran     chan struct{}
	release chan struct{}
}

func (d drainTerm) Line(context.Context) (string, error) {
	return "", errors.New("a line of input is longer than the cap")
}

func (d drainTerm) Drain(context.Context) error {
	close(d.ran)
	<-d.release
	return nil
}

// TestARefusedHookInputIsDrainedForNoLongerThanHookDrain: the hook's input
// refused, the guard drains it before it answers, and answers when the drain
// ends; a drain that never ends holds the guard for coreapi.HookDrain and no
// longer, and the input is refused all the same, the deadline named.
// The clock is synctest's, so no test
// waits ten seconds.
// Risk: the guard refuses with the host still writing, or a stuck pipe
// holds the refusal without end.
func TestARefusedHookInputIsDrainedForNoLongerThanHookDrain(t *testing.T) {
	t.Parallel()
	type answer struct {
		in  []byte
		err error
	}
	for _, stuck := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			d := drainTerm{Terminal: porttest.NewWorld(t).Ports().Terminal, ran: make(chan struct{}), release: make(chan struct{})}
			if !stuck {
				close(d.release)
			}
			answered := make(chan answer, 1)
			go func() {
				in, err := hookInput(t.Context(), d)
				answered <- answer{in, err}
			}()
			if stuck {
				<-time.After(coreapi.HookDrain - time.Nanosecond)
			}
			synctest.Wait()
			select {
			case <-d.ran:
			default:
				t.Fatalf("stuck %v: the refused input was not drained", stuck)
			}
			if stuck {
				select {
				case <-answered:
					t.Fatalf("the guard answered before the drain's deadline")
				default:
				}
			}
			var got answer
			select {
			case got = <-answered:
			case <-time.After(2 * time.Nanosecond): // past HookDrain when stuck
				t.Fatalf("stuck %v: the guard had not answered by its deadline", stuck)
			}
			if got.err == nil || got.in != nil {
				t.Errorf("hookInput = %q, %v, want the input refused", got.in, got.err)
			}
			if deadline := errors.Is(got.err, context.DeadlineExceeded); deadline != stuck {
				t.Errorf("stuck %v: the refusal %v names the drain's deadline: %v", stuck, got.err, deadline)
			}
			if stuck {
				close(d.release)
			}
		})
	}
}

// TestAHookInputReadWholeIsNotDrained: an input read to its end is not
// drained: there is nothing left to read.
func TestAHookInputReadWholeIsNotDrained(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedTerminal(porttest.Term{Input: []string{`{"tool_name":"Write"}`}})
	d := drainTerm{Terminal: w.Ports().Terminal, ran: make(chan struct{}), release: make(chan struct{})}
	if _, err := hookInput(t.Context(), struct {
		port.Terminal
		drainer
	}{w.Ports().Terminal, d}); err != nil {
		t.Fatalf("hookInput = %v", err)
	}
	select {
	case <-d.ran:
		t.Errorf("an input read whole was drained")
	default:
	}
}
