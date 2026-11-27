package root

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestTheAnswersNextIsTheLineTheRunCarries proves the answer's next and its
// error's hint come from the line the ending or the failure carries, and are
// never read back out of the message: a message that holds a
// line in backticks and carries none gives an answer with no next.
//
// Risk: an agent reading --json is handed as its next whatever a message
// happens to quote, such as a path or a value in backticks from a deeper
// error, and runs it.
func TestTheAnswersNextIsTheLineTheRunCarries(t *testing.T) {
	t.Parallel()
	tree := (&session{rec: &recorder{}}).newRoot(t.Context())
	quoted := "the step stopped at `rm -rf work` and nothing was written"

	cases := []struct {
		name string
		s    *session
		want string
	}{
		{"an ending that carries no line", &session{rec: &recorder{}, ended: &ending{word: model.Failed, what: quoted}}, ""},
		{"an ending whose sentence says its line", &session{rec: &recorder{}, ended: &ending{word: model.Failed, what: quoted, said: "gragas gen --apply"}}, "gragas gen --apply"},
		{"an ending with a line under the word", &session{rec: &recorder{}, ended: &ending{word: model.Failed, what: quoted, next: "gragas check", said: "gragas gen --apply"}}, "gragas check"},
		{"a failure that carries no line", &session{rec: &recorder{}, fault: quoted}, ""},
		{"a failure that says its line", &session{rec: &recorder{}, fault: quoted, faultNext: "gragas --help"}, "gragas --help"},
	}
	for _, c := range cases {
		r := result(c.s, tree, nil, nil, exit.Failed)
		if r.Next != c.want || r.Error == nil || r.Error.Hint != c.want {
			t.Errorf("%s: next %q, error %+v; want next and hint %q", c.name, r.Next, r.Error, c.want)
		}
	}
}

// TestAMistypedLineCarriesTheLineItSays proves each usage message carries the
// one line it says in backticks, so the answer's next is the line a person
// reads on stderr.
//
// Risk: the stderr line and the --json next drift apart, and an agent is
// handed a different command than a person is.
func TestAMistypedLineCarriesTheLineItSays(t *testing.T) {
	t.Parallel()
	tree := (&session{rec: &recorder{}}).newRoot(t.Context())
	cases := []struct {
		args []string
		err  string
		want string
	}{
		{[]string{"chek"}, `unknown command "chek" for "gragas"`, "gragas check"},
		{[]string{"zzzzzz"}, `unknown command "zzzzzz" for "gragas"`, "gragas --help"},
		{[]string{"check", "--jsn"}, "unknown flag: --jsn", "gragas check --json"},
		{[]string{"check", "--qqqqqqq"}, "unknown flag: --qqqqqqq", "gragas check --help"},
	}
	for _, c := range cases {
		msg, next := usageMessage(tree, c.args, errors.New(c.err))
		if next != c.want || !strings.HasSuffix(strings.TrimSuffix(msg, "?"), "`"+c.want+"`") {
			t.Errorf("%v: %q, next %q; want the message to end on `%s` and next %q", c.args, msg, next, c.want, c.want)
		}
	}
}
