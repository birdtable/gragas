// Package exit maps a closing word to the exit code a script reads, and is
// gragas's table of endings: each kind with its closing word and its code,
// checked once by go-run/exit, which declares no code of its own (go-run
// v0.5.0). The codes are declared here and nowhere else (cli.exit.2): no other
// package writes an exit code (internal/repo/gocode/exits_test.go,
// TestOnlyMainExitsAndOnlyExitDeclaresACode), and only main exits (go.errors.4).
//
// The closing words stay gragas's own (internal/model): a kind's name is its
// word, so a name the kit is handed (host/term's EndKinds) is a word of the
// closed set.
package exit

import (
	kit "github.com/birdtable/go-run/exit"

	"github.com/birdtable/gragas/internal/model"
)

// The seven codes, gragas's own. failed and findings share 1 by ruling (A39):
// a gate fails on either, and the word and the JSON kind tell them apart.
// go-run/exit lets two kinds share a code.
const (
	Done     = 0
	Failed   = 1
	Findings = 1
	Usage    = 2
	Busy     = 3
	NotReady = 4
	Refused  = 5
	Declined = 6
)

// BlockedWord is the first argument that starts gragas-dev or gragas-release
// as the blocked child: the program after "--" runs behind go-proc's sandbox
// (go-proc/sandbox). The word is gragas's, so it is
// declared once here, where both mains read their codes, and a usage fault of
// the child ends on Usage, any other on Failed.
const BlockedWord = "network-blocked"

// Table is a checked table of endings, as go-run/exit builds it. It is named
// here so that host/term hands the kit gragas's table without importing
// go-run/exit, which this package alone imports (layers.yaml).
type Table = kit.Table

// NewTable is gragas's table of endings: one kind per closing word, each
// named by its word and given its code. The kinds are this file's and never
// change at run time, so an error is a fault of this file, never of a run:
// main checks it before anything runs and stops on it, a test holds it on
// every run (TestTheTableIsTheClosedSetWithItsCodes), and nothing panics on
// it in an init.
func NewTable() (Table, error) {
	words := model.Words()
	kinds := make([]kit.Kind, len(words))
	for i, w := range words {
		code, _ := Code(w)
		kinds[i] = kit.Kind{Name: string(w), Word: string(w), Code: code}
	}
	return kit.New(kinds...)
}

// Code is the exit code of a word. A word outside the closed set is not a
// code to guess at: it gets Failed, the safe direction, and ok is false.
func Code(w model.Word) (code int, ok bool) {
	switch w {
	case model.Done:
		return Done, true
	case model.Failed:
		return Failed, true
	case model.Findings:
		return Findings, true
	case model.Usage:
		return Usage, true
	case model.Busy:
		return Busy, true
	case model.NotReady:
		return NotReady, true
	case model.Refused:
		return Refused, true
	case model.Declined:
		return Declined, true
	}
	return Failed, false
}
