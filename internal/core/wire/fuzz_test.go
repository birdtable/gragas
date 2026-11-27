package wire

import (
	"bytes"
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// answerOf is the canonical bytes of an accepted body as the ok of a response:
// what the engine digests and compares of an answer.
func answerOf(t *testing.T, body canon.Value) []byte {
	t.Helper()
	b, err := Canonical(canon.Object{"ok": body})
	if err != nil {
		t.Fatalf("Canonical of an accepted answer: %v", err)
	}
	return b
}

// refusedAs is whether err is one of the three errors a decode answers with; any
// other, or a panic, is a hole.
func refusedAs(err error) bool {
	_, remote := errors.AsType[*plugin.CallError](err)
	_, answer := errors.AsType[*plugin.AnswerError](err)
	_, limit := errors.AsType[*plugin.LimitError](err)
	return remote || answer || limit
}

// FuzzDecodeResponse (tests.generated.1): any bytes read as the
// response to any one of the five operations either are refused with the
// codec's own errors, or give an answer whose canonical bytes are the same
// after the answer is read again from them, and no accepted answer is past a
// cap: the response, an opaque value in canonical form.
func FuzzDecodeResponse(f *testing.F) {
	// each seed is the operation it is an answer to: 0 plan, 1 entries,
	// 2 findings, 3 rank, 4 preset; the same rows are in testdata/fuzz.
	seeds := []struct {
		op   uint8
		data []byte
	}{
		{0, run(``)},
		{0, ok(`{"final":true,"steps":[{"kind":"read","glob":"a/**/b"}]}`)},
		{0, ok(`{"final":false,"steps":[]}`)},
		{1, entries("a", "b")},
		{1, entries("b", "a")},
		{1, ok(`{"entries":[{"key":"a","on":true,"value":[null,{"x":1.5}]}]}`)},
		{2, ok(`{"findings":[{"path":"a","line":1,"column":2,"key":"k","message":"m"}]}`)},
		{3, ok(`{"words":["x","y"]}`)},
		{4, ok(`{"files":[{"path":"a","text":"# mark\n"},{"path":"b","text":"# mark\n"}]}`)},
		{1, []byte(`{"error":{"code":"bug","message":"x"}}`)},
		{0, []byte(`{"ok":` + nested(33) + `}`)},
		{0, []byte("\xef\xbb\xbf{}")},
		{0, []byte(``)},
	}
	for _, s := range seeds {
		f.Add(s.op, s.data)
	}
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		c := codec(t)
		var (
			body canon.Value
			err  error
		)
		// reread reads canonical bytes once more as the response of the same
		// operation, and answers what it would digest.
		var reread func([]byte) ([]byte, error)
		switch which % 5 {
		case 0:
			var r plugin.PlanResponse
			r, err = c.DecodePlan(data, int(which)/5%3, grantsOf())
			body = r.ToCanon()
			reread = func(b []byte) ([]byte, error) {
				x, err := c.DecodePlan(b, int(which)/5%3, grantsOf())
				return answerOf(t, x.ToCanon()), err
			}
		case 1:
			var r plugin.EntriesResponse
			r, err = c.DecodeEntries(data)
			body = r.ToCanon()
			for i := 1; err == nil && i < len(r.Entries); i++ {
				if r.Entries[i-1].Key >= r.Entries[i].Key {
					t.Fatalf("accepted entries out of order or twice: %q then %q", r.Entries[i-1].Key, r.Entries[i].Key)
				}
			}
			reread = func(b []byte) ([]byte, error) {
				x, err := c.DecodeEntries(b)
				return answerOf(t, x.ToCanon()), err
			}
		case 2:
			var r plugin.FindingsResponse
			r, err = c.DecodeFindings(data)
			body = r.ToCanon()
			reread = func(b []byte) ([]byte, error) {
				x, err := c.DecodeFindings(b)
				return answerOf(t, x.ToCanon()), err
			}
		case 3:
			var r plugin.RankResponse
			r, err = c.DecodeRank(data, 2)
			body = r.ToCanon()
			reread = func(b []byte) ([]byte, error) {
				x, err := c.DecodeRank(b, 2)
				return answerOf(t, x.ToCanon()), err
			}
		default:
			var r plugin.PresetResponse
			r, err = c.DecodePreset(data, "mark")
			body = r.ToCanon()
			reread = func(b []byte) ([]byte, error) {
				x, err := c.DecodePreset(b, "mark")
				return answerOf(t, x.ToCanon()), err
			}
		}
		if err != nil {
			if !refusedAs(err) {
				t.Fatalf("a refusal that is none of the codec's: %T %v", err, err)
			}
			return
		}
		if len(data) > MaxResponse {
			t.Fatalf("an answer of %d bytes, over %d", len(data), MaxResponse)
		}
		first := answerOf(t, body)
		second, err := reread(first)
		if err != nil {
			t.Fatalf("an accepted answer is refused read again from its canonical form: %v\n%s", err, first)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("an answer's canonical form changes when read again:\n%s\n%s", first, second)
		}
	})
}

// FuzzCanonicalForm (tests.generated.1): any JSON the engine reads
// has one canonical form, and Canonical of it is the same bytes again after it
// is read: the bytes a digest and a comparison are of.
func FuzzCanonicalForm(f *testing.F) {
	for _, s := range []string{
		`{"b":1,"a":[1.0,1e0,-0,"é"]}`,
		`{"a":{"b":{"c":null}}}`,
		`[1e300,0.1,1e-7,"😀"]`,
		`{"a":1,"a":2}`,
		`9007199254740993`,
		` [ ] `,
		`"\ud800"`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := canon.Decode(data)
		if err != nil {
			return
		}
		first, err := Canonical(v)
		if err != nil {
			return
		}
		w, err := canon.Decode(first)
		if err != nil {
			t.Fatalf("the canonical form is not read: %v\n%s", err, first)
		}
		second, err := Canonical(w)
		if err != nil {
			t.Fatalf("the canonical form is not written again: %v\n%s", err, first)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("canonical form is not one form:\n%s\n%s", first, second)
		}
	})
}
