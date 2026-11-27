package canon_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// TestNestingIsHeldAtMaxDepthByTheEncoderAndTheDecoder (boundary: MaxDepth):
// a value MaxDepth levels deep is taken by Encode and by Decode, in arrays and
// in objects, and one level deeper is refused by each with TooDeep, so the
// refusal is the cap's. The longest text is about 2.6 KB.
func TestNestingIsHeldAtMaxDepthByTheEncoderAndTheDecoder(t *testing.T) {
	t.Parallel()
	arrays := func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }
	objects := func(n int) string { return strings.Repeat(`{"a":`, n-1) + "{}" + strings.Repeat("}", n-1) }
	for name, doc := range map[string]func(int) string{"arrays": arrays, "objects": objects} {
		if _, err := canon.Decode([]byte(doc(canon.MaxDepth))); err != nil {
			t.Errorf("Decode of %s at MaxDepth: %v", name, err)
		}
		_, err := canon.Decode([]byte(doc(canon.MaxDepth + 1)))
		refusal(t, err, canon.TooDeep)
	}
	if _, err := canon.Encode(nested(canon.MaxDepth), canon.Line); err != nil {
		t.Errorf("Encode at MaxDepth: %v", err)
	}
	_, err := canon.Encode(nested(canon.MaxDepth+1), canon.Line)
	refusal(t, err, canon.TooDeep)
}

// TestWholeNumbersAreHeldAtMaxSafeByTheEncoderAndTheDecoder (boundary: MaxSafe):
// MaxSafe and -MaxSafe are taken by Encode and by Decode, as an Int
// and as a Float, and a magnitude one more is refused by each with TooLarge.
func TestWholeNumbersAreHeldAtMaxSafeByTheEncoderAndTheDecoder(t *testing.T) {
	t.Parallel()
	at, over := strconv.FormatInt(canon.MaxSafe, 10), strconv.FormatInt(canon.MaxSafe+1, 10)
	for _, sign := range []string{"", "-"} {
		if _, err := canon.Decode([]byte(sign + at)); err != nil {
			t.Errorf("Decode of %s%s: %v", sign, at, err)
		}
		_, err := canon.Decode([]byte(sign + over))
		refusal(t, err, canon.TooLarge)
	}
	for _, v := range []canon.Value{canon.Int(canon.MaxSafe), canon.Int(-canon.MaxSafe), canon.Float(canon.MaxSafe), canon.Float(-canon.MaxSafe)} {
		if _, err := canon.Encode(v, canon.Line); err != nil {
			t.Errorf("Encode of %v at MaxSafe: %v", v, err)
		}
	}
	for _, v := range []canon.Value{canon.Int(canon.MaxSafe + 1), canon.Int(-canon.MaxSafe - 1), canon.Float(canon.MaxSafe + 1), canon.Float(-canon.MaxSafe - 1)} {
		_, err := canon.Encode(v, canon.Line)
		refusal(t, err, canon.TooLarge)
	}
}
