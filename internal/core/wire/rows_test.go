package wire

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model/plugin"
)

// fnd is one finding of the given fields, as JSON.
func fnd(path string, line, col int, key, msg string) string {
	return `{"key":"` + key + `","message":"` + msg + `","path":"` + path + `","line":` + itoa(line) + `,"column":` + itoa(col) + `}`
}

func itoa(n int) string { return strconv.Itoa(n) }

func isAnswer(err error) bool {
	_, ok := errors.AsType[*plugin.AnswerError](err)
	return ok
}

func isRemoteErr(err error) bool {
	_, ok := errors.AsType[*plugin.CallError](err)
	return ok
}

// TestEachOperationAnswersItsOwnCodes plants every cell of the answers table
// that is a refusal, and the cells that pass. Risk: a plugin that answers a
// code the engine has no case for.
func TestEachOperationAnswersItsOwnCodes(t *testing.T) {
	t.Parallel()
	c := codec(t)
	errOf := func(code string) []byte { return []byte(`{"error":{"code":"` + code + `","message":"m"}}`) }
	rows := []struct {
		name string
		do   func() error
		want bool // a *plugin.CallError, else an *plugin.AnswerError
	}{
		{"entries refused", func() error { _, err := c.DecodeEntries(errOf("refused")); return err }, false},
		{"findings refused", func() error { _, err := c.DecodeFindings(errOf("refused")); return err }, false},
		{"rank unreadable", func() error { _, err := c.DecodeRank(errOf("unreadable"), 0); return err }, false},
		{"preset unreadable", func() error { _, err := c.DecodePreset(errOf("unreadable"), ""); return err }, false},
		{"a plan's refused", func() error { _, err := c.DecodePlan(errOf("refused"), 0, grantsOf()); return err }, true},
		{"entries unreadable", func() error { _, err := c.DecodeEntries(errOf("unreadable")); return err }, true},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			err := r.do()
			if r.want && !isRemoteErr(err) || !r.want && !isAnswer(err) {
				t.Errorf("got %T %v", err, err)
			}
		})
	}
}

// TestTheOrderAndNumbersOfAnAnswerAreHeld plants a row for each field of the
// finding order, the words of a rank, a repeated preset path and a pointer's
// tilde. Risk: an answer with two spellings, or a rank that is misread.
func TestTheOrderAndNumbersOfAnAnswerAreHeld(t *testing.T) {
	t.Parallel()
	c := codec(t)
	findings := func(fs ...string) []byte { return ok(`{"ran":true,"findings":[` + strings.Join(fs, ",") + `]}`) }
	rows := []struct {
		name string
		do   func() error
		at   string
	}{
		{"findings out of order by line", func() error {
			_, err := c.DecodeFindings(findings(fnd("a", 2, 1, "k", "m"), fnd("a", 1, 1, "k", "m")))
			return err
		}, "/ok/findings/1"},
		{"findings out of order by column", func() error {
			_, err := c.DecodeFindings(findings(fnd("a", 1, 2, "k", "m"), fnd("a", 1, 1, "k", "m")))
			return err
		}, "/ok/findings/1"},
		{"findings out of order by key", func() error {
			_, err := c.DecodeFindings(findings(fnd("a", 1, 1, "b", "m"), fnd("a", 1, 1, "a", "m")))
			return err
		}, "/ok/findings/1"},
		{"findings out of order by message", func() error {
			_, err := c.DecodeFindings(findings(fnd("a", 1, 1, "k", "b"), fnd("a", 1, 1, "k", "a")))
			return err
		}, "/ok/findings/1"},
		{"more words than pairs", func() error {
			_, err := c.DecodeRank(ok(`{"words":["same","same"]}`), 1)
			return err
		}, "/ok/words"},
		{"two preset files of one path", func() error {
			_, err := c.DecodePreset(ok(`{"files":[{"path":"a","text":"x"},{"path":"a","text":"x"}]}`), "")
			return err
		}, "/ok/files/1/path"},
		{"an error path with a tilde that is neither ~0 nor ~1", func() error {
			_, err := c.DecodePlan([]byte(`{"error":{"code":"bug","message":"m","path":"/a~2"}}`), 0, grantsOf())
			return err
		}, "/error/path"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			err := r.do()
			a, isA := errors.AsType[*plugin.AnswerError](err)
			if !isA || a.Path != r.at {
				t.Errorf("got %T %v, want an *plugin.AnswerError at %s", err, err, r.at)
			}
		})
	}
	if _, err := c.DecodeFindings(findings(fnd("a", 1, 1, "k", "a"), fnd("a", 1, 1, "k", "b"))); err != nil {
		t.Errorf("the twin, in order: %v", err)
	}
}

// TestBase64HasOneSpelling plants the spellings of one byte that the standard
// decoder lets by: a line end inside or after, and trailing bits that are not
// zero; and a plan's stdin and a placed file are held to the bytes form, as a
// result of a read is. Risk: one answer with many spellings and digests.
func TestBase64HasOneSpelling(t *testing.T) {
	t.Parallel()
	c := codec(t)
	stdin := func(b64 string) error {
		_, err := c.DecodePlan(run(`,"stdin":{"base64":"`+b64+`"}`), 0, grantsOf())
		return err
	}
	for name, b64 := range map[string]string{
		"a line end inside":               `/w=\n=`,
		"a carriage return after":         `/w==\r`,
		"trailing bits that are not zero": `/x==`,
		"base64 of valid UTF-8":           `YQ==`,
	} {
		if err := stdin(b64); !isAnswer(err) {
			t.Errorf("stdin %s: %T %v, want an *plugin.AnswerError", name, err, err)
		}
	}
	if err := stdin(`/w==`); err != nil {
		t.Errorf("the twin, ff in base64: %v", err)
	}
	// 2 MiB of line ends are no bytes against the cap, and no base64.
	if err := stdin(strings.Repeat(`\n`, 2*MaxStdin)); !isAnswer(err) {
		t.Errorf("stdin of line ends: %T %v, want an *plugin.AnswerError", err, err)
	}
	_, err := c.DecodePlan(run(`,"place":[{"path":"a","data":{"base64":"YQ=="}}]`), 0, grantsOf())
	if !isAnswer(err) {
		t.Errorf("a placed file of base64 of valid UTF-8: %T %v, want an *plugin.AnswerError", err, err)
	}
}
