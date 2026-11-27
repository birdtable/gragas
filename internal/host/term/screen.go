package term

import (
	"errors"
	"strings"

	"github.com/birdtable/go-console/ui"

	"github.com/birdtable/gragas/internal/model"
)

// ErrNoBlocks is a Show of a Screen that model.NewScreen did not build.
var ErrNoBlocks = errors.New("term: the screen has no block; build one with model.NewScreen")

// Banner writes the wordmark and the version in the look of this terminal.
func (t *Terminal) Banner(version string) error {
	return t.Draw(t.Look().Banner(version))
}

// Show draws screen in the look of this terminal and writes it: colour when
// the output paints, plain when it does not, in the accessible form when that
// was asked. A screen with no block is ErrNoBlocks.
func (t *Terminal) Show(screen model.Screen) error {
	drawn, err := t.Look().Screen(screen)
	if err != nil {
		return err
	}
	return t.Draw(drawn)
}

// Screen is s drawn block by block. Every text a block carries goes through
// Strip, so a name from outside cannot move the cursor or say something the
// program did not (cli.errors.4).
func (l Look) Screen(screen model.Screen) (string, error) {
	blocks := screen.Blocks()
	if len(blocks) == 0 {
		return "", ErrNoBlocks
	}
	var b strings.Builder
	for _, blk := range blocks {
		switch x := blk.(type) {
		case model.Heading:
			blankBefore(&b) // a heading after other blocks, as Notices after a table, stands apart
			b.WriteString(l.draw(func(s *ui.Session) {
				s.Log("%s", s.Bold(Strip(x.Text)))
			}))
		case model.Reads:
			b.WriteString(l.draw(func(s *ui.Session) { s.Log("%s", s.Note("reading "+Strip(x.What))) }))
		case model.Table:
			cols := make([]Column, len(x.Columns))
			for i, c := range x.Columns {
				cols[i] = Column{Title: c.Title, Priority: c.Priority}
			}
			b.WriteString(l.Table(cols, x.Rows))
		case model.List:
			b.WriteString(l.draw(func(s *ui.Session) {
				for _, it := range x.Items {
					s.Info("%s", Strip(it))
				}
			}))
		case model.Note:
			b.WriteString(l.draw(func(s *ui.Session) { s.Log("%s", s.Note(Strip(x.Text))) }))
		case model.DryRunFooter:
			b.WriteString(l.draw(func(s *ui.Session) {
				s.Log("%s", s.Note("This was a dry run and nothing was changed. Add "+Strip(x.Flag)+" to make these changes."))
			}))
		case model.Closing:
			blankBefore(&b) // the gap before the word, as the kit draws it
			b.WriteString(l.Closing(Ending{Word: x.Word(), What: Strip(x.What()), Next: Strip(x.Next())}))
		}
	}
	return b.String(), nil
}

// blankBefore puts one blank line between what is drawn and the next block, unless
// there is nothing before it or a blank line already.
func blankBefore(b *strings.Builder) {
	if s := b.String(); s != "" && s != "\n" && !strings.HasSuffix(s, "\n\n") {
		b.WriteString("\n")
	}
}
