package root

// The one place gragas writes JSON (cli.output.2): the answer of a run under
// --json. A command returns its data; this file renders it, as the result
// types internal/core/api declares.

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// The kinds of error, as the answer spells them.
const (
	kindRefused  = "refused"
	kindFailed   = "failed"
	kindUsage    = "usage"
	kindBusy     = "busy"
	kindNotReady = "not-ready"
)

// result is the Result of a run that printed no document of its own. Its
// next is the line the run's ending or failure carries; it is never read back
// out of the message.
func result(s *session, tree *cobra.Command, args []string, err error, code int) coreapi.Result {
	c, _, ferr := tree.Find(args)
	if ferr != nil || c == nil {
		c = tree
	}
	r := coreapi.Result{Schema: coreapi.ResultSchema, Command: strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), tree.Name())),
		Changes: s.changes, Run: s.rec.id(), Data: s.data}
	if c.Flags().Lookup("apply") != nil {
		apply := s.applied
		r.Apply = &apply
	}
	switch e := s.ended; {
	case e != nil:
		r.Ended, r.Msg, r.Next = string(e.word), e.what, cmp.Or(e.next, e.said)
	case code == exit.Usage:
		r.Ended = string(model.Usage)
	case code != exit.Done:
		r.Ended = string(model.Failed)
	default:
		r.Ended = string(model.Done)
	}
	// A declined dry run is a plan, not an error: the document holds no error.
	if code == exit.Done || code == exit.Declined {
		return r
	}
	if r.Msg == "" {
		r.Msg, r.Next = s.fault, cmp.Or(r.Next, s.faultNext)
	}
	if r.Msg == "" && err != nil {
		r.Msg = err.Error()
	}
	if r.Msg == "" {
		r.Msg = fmt.Sprintf("exit status %d", code)
	}
	r.Error = &coreapi.Error{Kind: kindOf(code, r.Ended), Msg: r.Msg, Hint: r.Next}
	return r
}

// kindOf is an Error's kind from the exit code it ends with.
func kindOf(code int, ended string) string {
	switch code {
	case exit.Usage:
		return kindUsage
	case exit.Busy:
		return kindBusy
	case exit.NotReady:
		return kindNotReady
	}
	if ended == string(model.Failed) || ended == string(model.Findings) {
		return kindFailed
	}
	return kindRefused
}

// answer writes r as the run's one JSON document.
func (s *session) answer(r coreapi.Result) { s.document(r) }

// document writes v, a command's own document, as the run's one answer.
func (s *session) document(v any) {
	s.printed = true
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.die("the answer could not be rendered: %v", err)
		return
	}
	if _, err := fmt.Fprintln(s.p.Stdout, string(b)); err != nil {
		s.die("the answer could not be written: %v", err)
	}
}

// encoded writes doc, a document internal/core/contract encoded (A20), as
// the run's one answer, byte for byte: the contract's encoder is the one
// encoder of its documents, so nothing here renders it again.
func (s *session) encoded(doc []byte) {
	s.printed = true
	if _, err := s.p.Stdout.Write(doc); err != nil {
		s.die("the answer could not be written: %v", err)
	}
}

// renderJSON is v as an indented document with a closing newline: the form of
// the committed command-tree file. It is here with the answer's encoder, the
// one place gragas writes JSON.
func renderJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("the document could not be rendered: %w", err)
	}
	return append(b, '\n'), nil
}
