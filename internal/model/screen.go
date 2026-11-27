package model

import (
	"errors"
	"fmt"
)

// A Screen is what a run shows a person: a list of blocks from a closed set,
// built only by NewScreen, so a screen that breaks the look's order cannot
// exist (cli.look.5, cli.look.6). The command tree builds Screens and hands
// them to the Terminal port, which draws each block; it never draws, and it
// never imports the adapter that does (A62).
type Screen struct {
	blocks []Block
}

// A Block is one part of a Screen. The set is closed: Heading, Reads, Table,
// List, Note, DryRunFooter and Closing, and nothing outside this package can
// add one.
type Block interface {
	block()
}

// Heading is a line that names the screen: the command and what it is about.
type Heading struct{ Text string }

// Reads is a line saying what the run read, such as the folder.
type Reads struct{ What string }

// Column is one column of a Table.
type Column struct {
	Title string
	// Priority orders what is dropped when the width is short: the lowest
	// goes first, the rightmost of equals first. The last one is never
	// dropped.
	Priority int
}

// Table is columns and rows of text. A cell is text from outside and is
// drawn with its control sequences removed.
type Table struct {
	Columns []Column
	Rows    [][]string
}

// List is lines under one another, one item each.
type List struct{ Items []string }

// Note is a supporting line, less loud than the rest.
type Note struct{ Text string }

// DryRunFooter ends a screen of a plan that was not applied. Flag is the flag
// that applies it, with its dashes, as `--apply`.
type DryRunFooter struct{ Flag string }

// Closing is the last block of a screen: the word, the sentence beside it,
// and the line to type when the word asks for one. Build it with NewClosing.
type Closing struct {
	word Word
	what string
	next string
}

func (Heading) block()      {}
func (Reads) block()        {}
func (Table) block()        {}
func (List) block()         {}
func (Note) block()         {}
func (DryRunFooter) block() {}
func (Closing) block()      {}

// Errors a Screen or a Closing is refused with, for a caller or a test to
// match.
var (
	// ErrUnknownWord is a closing built with a word outside the eight.
	ErrUnknownWord = errors.New("model: the word is not one of the eight")
	// ErrNoNext is a closing of a word that refuses, built without the line to
	// type.
	ErrNoNext = errors.New("model: a closing that refuses needs the line to type")
	// ErrNoClosing is a screen that does not end on one closing word.
	ErrNoClosing = errors.New("model: a screen ends on exactly one closing word")
	// ErrFooterPlace is a dry-run footer that is not the block just before the
	// closing word, or a second one.
	ErrFooterPlace = errors.New("model: the dry-run footer is one, and just before the closing word")
	// ErrNoFlag is a dry-run footer that names no flag.
	ErrNoFlag = errors.New("model: a dry-run footer names its flag")
	// ErrBadBlock is a block that is nil, or a table with no columns.
	ErrBadBlock = errors.New("model: a block is nil or empty")
)

// NewClosing builds the closing word of a screen. The word must be one of the
// eight; a word that refuses (usage, busy, not ready, refused, declined) must
// carry the line to type, as a Refusal does (cli.errors.2).
func NewClosing(w Word, what, next string) (Closing, error) {
	if !Known(w) {
		return Closing{}, fmt.Errorf("%w: %q", ErrUnknownWord, string(w))
	}
	if refusing(w) && next == "" {
		return Closing{}, fmt.Errorf("%w: %q", ErrNoNext, string(w))
	}
	return Closing{word: w, what: what, next: next}, nil
}

// Word is the closing word.
func (c Closing) Word() Word { return c.word }

// What is the sentence beside the word.
func (c Closing) What() string { return c.what }

// Next is the line to type, empty when the word has none.
func (c Closing) Next() string { return c.next }

// NewScreen builds a screen of blocks. It must end on exactly one Closing, and
// a DryRunFooter, when there is one, must be the one block before it. A
// screen of no block, a nil block, a table with no column or a footer with no
// flag is refused.
func NewScreen(blocks ...Block) (Screen, error) {
	var closings int
	for i, b := range blocks {
		switch x := b.(type) {
		case nil:
			return Screen{}, fmt.Errorf("%w: block %d", ErrBadBlock, i)
		case Table:
			if len(x.Columns) == 0 {
				return Screen{}, fmt.Errorf("%w: the table at block %d has no column", ErrBadBlock, i)
			}
		case DryRunFooter:
			if x.Flag == "" {
				return Screen{}, ErrNoFlag
			}
			if i != len(blocks)-2 {
				return Screen{}, ErrFooterPlace
			}
		case Closing:
			closings++
			if i != len(blocks)-1 {
				return Screen{}, ErrNoClosing
			}
		}
	}
	if closings != 1 {
		return Screen{}, ErrNoClosing
	}
	return Screen{blocks: append([]Block(nil), blocks...)}, nil
}

// Blocks is the screen's blocks in order, a copy; none for a Screen that
// NewScreen did not build.
func (s Screen) Blocks() []Block { return append([]Block(nil), s.blocks...) }
