package root

import (
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// TestALineSplitsBackIntoTheWordsItWasQuotedFrom: every value applies and the
// yes quote with go-console's shell.Quote, a quote, a space, a dollar, a shell operator
// or nothing at all among them, splits back into the same words, so guard
// staged runs the command the line names and no other.
// Risk: a value with a quote or a space replays as other flags.
func TestALineSplitsBackIntoTheWordsItWasQuotedFrom(t *testing.T) {
	t.Parallel()
	words := []string{"gragas", "rule", "promote", "go.errors.2", "--decision-text", "It's the team's call.", "--decision-title", "Try it", "--note", "", "--x", word(t, "dollar-space"), "--y", word(t, "backslash-name"),
		"--z", "a;b", "--w", "x|y&z", "--v", "`id`", "--u", "~/(n)", "--t", "=ls"}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = oneWord(t, w)
	}
	got, err := splitLine(strings.Join(parts, " "))
	if err != nil || !slices.Equal(got, words) {
		t.Fatalf("split %q:\n got %q, %v\nwant %q", strings.Join(parts, " "), got, err, words)
	}
}

// TestAnApplyLineHoldsEachArgumentAsOneWord: the line a rule command applies
// with quotes its arguments as it quotes its flags' values, so an argument
// with a space or a shell operator in it is one word again when guard staged
// splits it, and one word to a shell it is pasted into.
// Risk: an argument like `a; rm -rf x` pastes as a second command.
func TestAnApplyLineHoldsEachArgumentAsOneWord(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "gragas"}
	promote := &cobra.Command{Use: "promote", Run: func(*cobra.Command, []string) {}}
	promote.Flags().String("note", "", "")
	root.AddCommand(promote)
	if err := promote.Flags().Set("note", word(t, "note-home")); err != nil {
		t.Fatal(err)
	}
	line, err := applies(shell.POSIX, promote, []string{"a; rm -rf x", "go.errors.2"})
	if want := word(t, "apply-line-home"); err != nil || line != want {
		t.Fatalf("applies = %s\nwant %s", line, want)
	}
	got, err := splitLine(line)
	if want := []string{"gragas", "promote", "a; rm -rf x", "go.errors.2", "--note", word(t, "note-home"), "--apply"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("split %q: got %q, %v, want %q", line, got, err, want)
	}
}

// TestALineWithAnOpenQuoteIsNotSplit: a quote left open is an error, never a
// last word that runs to the line's end.
func TestALineWithAnOpenQuoteIsNotSplit(t *testing.T) {
	t.Parallel()
	if got, err := splitLine("gragas rule promote 'go.errors.2"); err == nil {
		t.Fatalf("an open quote split into %q", got)
	}
}

// readsOf is the old Files port, recording every folder and file it is asked
// for, in order.
type readsOf struct {
	legacy.Files
	asked *[]string
}

func (r readsOf) Repo(dir string) (legacy.Source, error) {
	*r.asked = append(*r.asked, dir)
	return r.Files.Repo(dir)
}

func (r readsOf) Open(name string) (io.ReadCloser, error) {
	*r.asked = append(*r.asked, name)
	return r.Files.Open(name)
}

// TestAReplayedLineRunsOnlyAPlanCommandAndReadsNothingOnTheHost: a history
// line that names gen -C with an absolute folder, another command that is not
// a plan, or a plan command with -C or a root flag such as --config, is
// refused before it runs, and the old ports, which read the host's disk, are
// asked nothing. The plant is gen -C run as a command, whose read of the same
// folder the recorder sees. Risk: a branch's history line makes check read a
// folder on the checking host, and run the names check over it.
func TestAReplayedLineRunsOnlyAPlanCommandAndReadsNothingOnTheHost(t *testing.T) {
	t.Parallel()
	dir, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var asked []string
	f := world(t)
	ports := f.Ports()
	ports.Files = readsOf{Files: ports.Files, asked: &asked}
	s := &session{p: Process{Ports: ports, Config: config.Config{StateDir: t.TempDir()}}}
	replay := s.replayer()
	const notAPlan = "is not a command that plans a change to the rulebook"
	for _, c := range []struct{ line, says string }{
		{"gragas gen -C " + oneWord(t, dir), "gragas gen " + notAPlan},
		{"gragas gen --dir=" + oneWord(t, dir), "gragas gen " + notAPlan},
		{"gragas gen", "gragas gen " + notAPlan},
		{"gragas check -C " + oneWord(t, dir), "gragas check " + notAPlan},
		{"gragas contract fixture --out " + oneWord(t, dir), "gragas contract fixture " + notAPlan},
		{"gragas cache clear", "gragas cache clear " + notAPlan},
		{"gragas journal recover", "gragas journal recover " + notAPlan},
		{"gragas rule promote go.errors.2 -C " + oneWord(t, dir), "-C"},
		{"gragas rule promote go.errors.2 --config " + oneWord(t, filepath.Join(dir, "settings.toml")), "it sets --config"},
		{"gragas rule promote go.errors.2 --json", "it sets --json"},
	} {
		ch, ref, err := replay(t.Context(), port.Ports{}, c.line)
		if err == nil || ch != nil || ref != nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: change %v, refusal %v, error %v; want an error saying %q", c.line, ch, ref, err, c.says)
		}
	}
	if len(asked) != 0 {
		t.Errorf("the replays read %q on the host", asked)
	}
	p, _ := process(t, f, "gen", "-C", dir)
	p.Ports = ports
	execute(t.Context(), t, p)
	if !slices.Contains(asked, dir) {
		t.Errorf("plant: gen -C %s run as a command read %q, so the recorder proves nothing", dir, asked)
	}
}

// TestALineOfAPlanCommandWithItsOwnFlagsIsReplayable is the control: a plan
// command's line, as commandLine writes it, passes.
func TestALineOfAPlanCommandWithItsOwnFlagsIsReplayable(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"rule", "promote", "go.errors.2", "--decision-text", "It's the team's call.", "--decision-title", "Try it"},
		{"term", "new", "--slug", "seam", "--term-definition", "Where two layers meet."},
		{"migrate", "--name", "rb", "--owner", "me"},
	} {
		if err := replayable(t.Context(), args); err != nil {
			t.Errorf("%q is refused: %v", args, err)
		}
	}
}

// TestTheReplaysAreTheCommandsThatWriteButFourThatReachTheHost: every command
// replays names is in the tree and carries writes, and every command that
// carries writes is in replays or is one of the four that read or change the
// host, so a command added to the tree is a choice made here. Risk: a new
// command that writes the host is made replayable by its annotation alone,
// or a renamed plan command is refused by every replay.
func TestTheReplaysAreTheCommandsThatWriteButFourThatReachTheHost(t *testing.T) {
	t.Parallel()
	host := []string{"gragas cache clear", "gragas contract fixture", "gragas gen", "gragas journal recover", "gragas plugin add", "gragas plugin approve", "gragas plugin record", "gragas plugin upgrade", "gragas preset"}
	var write []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if _, ok := c.Annotations[writes]; ok {
			write = append(write, c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk((&session{}).newRoot(t.Context()))
	want := slices.Sorted(slices.Values(append(replays(), host...)))
	slices.Sort(write)
	if !slices.Equal(write, want) {
		t.Errorf("the commands that carry writes are\n%q\nand replays with the four that reach the host are\n%q", write, want)
	}
}

// oneWord is s as one word in the middle of a POSIX line, through go-console's
// shell.Quote, for the tables that build a line by hand.
func oneWord(t *testing.T, s string) string {
	t.Helper()
	q, err := shell.Quote(shell.POSIX, []string{"x", s})
	if err != nil {
		t.Fatalf("%q: %v", s, err)
	}
	return strings.TrimPrefix(q, "x ")
}
