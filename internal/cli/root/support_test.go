package root

import (
	"context"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// screens is what the fake terminals took in a run: the standard output's and
// the standard error's, each scripted at no width and no colour.
type screens struct{ out, diag *porttest.World }

// attach gives p its two terminals. Every run in these tests is attached, so
// this is where a run with no argument list stops: a test says what it runs,
// [] for the bare command.
func attach(t *testing.T, p Process) (Process, screens) {
	t.Helper()
	if p.Args == nil {
		t.Fatal("Process.Args is nil: give the run its arguments, []string{} for the bare command")
	}
	s := screens{out: porttest.NewWorld(t), diag: porttest.NewWorld(t)}
	s.out.SeedTerminal(porttest.Term{})
	s.diag.SeedTerminal(porttest.Term{})
	// The working folder, holding no rulebook until a test seeds one: check
	// asks it for one before it falls back to the old check.
	s.out.Seed(".", nil)
	for i, a := range p.Args {
		if a == "-C" && i+1 < len(p.Args) {
			s.out.Seed(p.Args[i+1], nil)
		}
	}
	if len(p.Args) > 0 && p.Args[0] == "gen" {
		seedGenBook(s.out, "some dir")
	}
	p.World = s.out.Ports()
	p.Diagnostics = s.diag.Ports().Terminal
	return p, s
}

// said is a screen as text, a block to a line, the closing word as the look
// draws it: word, sentence, then the line to type.
func said(s model.Screen) string {
	var b strings.Builder
	for _, blk := range s.Blocks() {
		switch x := blk.(type) {
		case model.Heading:
			b.WriteString(x.Text + "\n")
		case model.Reads:
			b.WriteString("reading " + x.What + "\n")
		case model.Table:
			for _, r := range x.Rows {
				b.WriteString(strings.Join(r, " ") + "\n")
			}
		case model.List:
			b.WriteString(strings.Join(x.Items, "\n") + "\n")
		case model.Note:
			b.WriteString(x.Text + "\n")
		case model.DryRunFooter:
			b.WriteString("Add " + x.Flag + "\n")
		case model.Closing:
			b.WriteString(string(x.Word()) + " " + x.What() + "\n")
			if x.Next() != "" {
				b.WriteString(x.Next() + "\n")
			}
		}
	}
	return b.String()
}

// output is everything a person was shown on standard output: the lines said,
// then each screen.
func (s screens) output() string {
	var b strings.Builder
	for _, l := range s.out.Said {
		b.WriteString(l + "\n")
	}
	for _, sc := range s.out.Shown {
		b.WriteString(said(sc))
	}
	return b.String()
}

// closing is the word the last screen ended on, "" when none was shown.
func (s screens) closing() model.Word {
	if len(s.out.Shown) == 0 {
		return ""
	}
	bl := s.out.Shown[len(s.out.Shown)-1].Blocks()
	c, _ := bl[len(bl)-1].(model.Closing)
	return c.Word()
}

// execute runs p in process and returns its code and what it said on stdout.
func execute(ctx context.Context, t *testing.T, p Process) (code int, shown screens) {
	t.Helper()
	p, shown = attach(t, p)
	return Execute(ctx, p), shown
}

// executeSaid runs p in process and returns its code and everything it said,
// standard output first and then standard error.
func executeSaid(ctx context.Context, t *testing.T, p Process) (code int, said string) {
	t.Helper()
	code, sc := execute(ctx, t, p)
	return code, sc.output() + strings.Join(sc.diag.Said, "\n")
}
