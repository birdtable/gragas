package canon_test

import (
	"encoding/json/jsontext"
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// TestCanonBytesAreJsontextCanonicalizeBytes (guard: the one source of the
// canonical bytes is jsontext.Value.Canonicalize): for text both read,
// canon's Canonical bytes are Canonicalize's, byte for byte, at the number
// boundaries, the escapes and the key orders where RFC 8785 and byte order
// part. Where canon refuses, it is a number past 2^53 - 1 only.
func TestCanonBytesAreJsontextCanonicalizeBytes(t *testing.T) {
	t.Parallel()
	same := []string{
		`0`, `-0`, `0.0`, `1.0`, `1e-5`, `1e-6`, `1e-7`, `1.5e-7`, `100`, `1e2`, `1E2`, `1e+2`, `5e-324`, `0.1`,
		`0.30000000000000004`, `0.000001`, `0.0000001`, `1e15`, `4.35`, `9007199254740991`, `-9007199254740991`,
		`" "`, `"\u007f"`, `"😀"`, `"é"`, `"\/"`, `"\u0000"`, `"\b\f\n\r\t"`, `"\u001f"`,
		`"<>&'"`, `"\"\\"`,
		`{"b":1,"a":2}`, `{"":1,"a":2}`, `{"é":1,"e":2,"f":3}`, `{"A":1,"a":2,"\u0000":3}`,
		`{"😀":1,"דּ":2}`, `{"":1,"𐀀":2,"￿":3,"퟿":4}`,
		` [ 1 , 2 ] `, `[]`, `{}`, `[1,[2,{"a":[]}]]`, `null`, `true`,
	}
	for _, in := range same {
		want := jsontext.Value(in)
		if err := want.Canonicalize(); err != nil {
			t.Fatalf("Canonicalize(%q): %v", in, err)
		}
		v, err := canon.Decode([]byte(in))
		if err != nil {
			t.Errorf("Decode(%q): %v", in, err)
			continue
		}
		got, err := canon.Encode(v, canon.Canonical)
		if err != nil {
			t.Errorf("Encode(%q): %v", in, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%q: canon %q, Canonicalize %q", in, got, want)
		}
	}
	// Numbers Canonicalize writes or rounds and canon refuses (the one kind of
	// difference): a whole number past 2^53 - 1 is never rounded.
	for _, in := range []string{`1e16`, `1e21`, `9007199254740992`, `9007199254740993`, `123456789012345678`, `1e400`} {
		_, err := canon.Decode([]byte(in))
		var ce *canon.Error
		if !errors.As(err, &ce) || ce.Kind != canon.TooLarge {
			t.Errorf("Decode(%q) = %v, want TooLarge", in, err)
		}
	}
}
