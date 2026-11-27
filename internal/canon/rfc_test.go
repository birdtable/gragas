package canon_test

import (
	"encoding/hex"
	"encoding/json/jsontext"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// The vectors of RFC 8785 (A59), copied from the RFC. canon's strings and
// numbers come from jsontext, so the tests below show first that jsontext
// passes them, and then that canon does inside its profile, refusing by name
// what is outside it: a whole number past 2^53 - 1 and what is not finite.

// sample is RFC 8785 section 3.2.2's input, with u for its \u escapes.
func sample() string {
	return u(`{
  "numbers": [333333333.33333329, 1E30, 4.50, 2e-3, 0.000000000000000000000000001],
  "string": "U+20ac$U+000FU+000aA'U+0042U+0022U+005c\\\"\/",
  "literals": [null, true, false]
}`)
}

// sampleBytes is section 3.2.4's hex of the sample's canonical form, as the
// RFC prints it.
const sampleBytes = "7b 22 6c 69 74 65 72 61 6c 73 22 3a 5b 6e 75 6c 6c 2c 74 72 " +
	"75 65 2c 66 61 6c 73 65 5d 2c 22 6e 75 6d 62 65 72 73 22 3a " +
	"5b 33 33 33 33 33 33 33 33 33 2e 33 33 33 33 33 33 33 2c 31 " +
	"65 2b 33 30 2c 34 2e 35 2c 30 2e 30 30 32 2c 31 65 2d 32 37 " +
	"5d 2c 22 73 74 72 69 6e 67 22 3a 22 e2 82 ac 24 5c 75 30 30 " +
	"30 66 5c 6e 41 27 42 5c 22 5c 5c 5c 5c 5c 22 2f 22 7d "

// sorting is section 3.2.3's input.
func sorting() string {
	return u(`{
  "U+20ac": "Euro Sign",
  "\r": "Carriage Return",
  "U+fb33": "Hebrew Letter Dalet With Dagesh",
  "1": "One",
  "U+d83dU+de00": "Emoji: Grinning Face",
  "U+0080": "Control",
  "U+00f6": "Latin Small Letter O With Diaeresis"
}`)
}

// sorted is the values of sorting in the order the RFC gives after sorting.
func sorted() []string {
	return []string{
		"Carriage Return", "One", "Control", "Latin Small Letter O With Diaeresis",
		"Euro Sign", "Emoji: Grinning Face", "Hebrew Letter Dalet With Dagesh",
	}
}

// numberRow is one row of Appendix B: the IEEE 754 bits and the JSON the RFC
// writes for them, "" for NaN and Infinity, which it refuses.
type numberRow struct {
	bits uint64
	want string
}

func appendixB() []numberRow {
	return []numberRow{
		{0x0000000000000000, "0"},
		{0x8000000000000000, "0"},
		{0x0000000000000001, "5e-324"},
		{0x8000000000000001, "-5e-324"},
		{0x7fefffffffffffff, "1.7976931348623157e+308"},
		{0xffefffffffffffff, "-1.7976931348623157e+308"},
		{0x4340000000000000, "9007199254740992"},
		{0xc340000000000000, "-9007199254740992"},
		{0x4430000000000000, "295147905179352830000"},
		{0x7fffffffffffffff, ""},
		{0x7ff0000000000000, ""},
		{0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{0x44b52d02c7e14af6, "1e+23"},
		{0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{0x444b1ae4d6e2ef4e, "999999999999999700000"},
		{0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{0x444b1ae4d6e2ef50, "1e+21"},
		{0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{0x3eb0c6f7a0b5ed8d, "0.000001"},
		{0x41b3de4355555553, "333333333.3333332"},
		{0x41b3de4355555554, "333333333.33333325"},
		{0x41b3de4355555555, "333333333.3333333"},
		{0x41b3de4355555556, "333333333.3333334"},
		{0x41b3de4355555557, "333333333.33333343"},
		{0xbecbf647612f3696, "-0.0000033333333333333333"},
		{0x43143ff3c1cb0959, "1424953923781206.2"},
	}
}

func rfcSample(t *testing.T) string {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(sampleBytes, " ", ""))
	if err != nil {
		t.Fatalf("the RFC's hex: %v", err)
	}
	return string(b)
}

// inOrder asserts the values sorted gives appear in s in that order.
func inOrder(t *testing.T, s string) {
	t.Helper()
	at, n := -1, 0
	for _, v := range sorted() {
		i := strings.Index(s, strconv.Quote(v))
		if i <= at {
			t.Fatalf("%q is not after the value before it in %q", v, s)
		}
		at = i
		n++
	}
	if n != 7 {
		t.Fatalf("%d values, want 7", n)
	}
}

func TestJsontextPassesTheVectorsOfRFC8785(t *testing.T) {
	t.Parallel()
	t.Run("the numbers of Appendix B", func(t *testing.T) {
		t.Parallel()
		n := 0
		for _, r := range appendixB() {
			f := math.Float64frombits(r.bits)
			if r.want == "" {
				continue
			}
			v := jsontext.Value(strconv.FormatFloat(f, 'g', -1, 64))
			if err := v.Canonicalize(); err != nil || string(v) != r.want {
				t.Errorf("%016x: jsontext writes %s, %v; the RFC %s", r.bits, v, err, r.want)
			}
			n++
		}
		if n != 24 {
			t.Fatalf("%d rows, want 24", n)
		}
	})
	t.Run("the sorting of section 3.2.3", func(t *testing.T) {
		t.Parallel()
		v := jsontext.Value(sorting())
		if err := v.Canonicalize(); err != nil {
			t.Fatalf("Canonicalize: %v", err)
		}
		inOrder(t, string(v))
	})
	t.Run("the sample of section 3.2.2 has the bytes of section 3.2.4", func(t *testing.T) {
		t.Parallel()
		v := jsontext.Value(sample())
		if err := v.Canonicalize(); err != nil || string(v) != rfcSample(t) {
			t.Errorf("jsontext writes %q, %v; the RFC %q", v, err, rfcSample(t))
		}
	})
}

func TestCanonPassesTheVectorsOfRFC8785InItsProfile(t *testing.T) {
	t.Parallel()
	t.Run("the numbers of Appendix B", func(t *testing.T) {
		t.Parallel()
		n := 0
		for _, r := range appendixB() {
			f := math.Float64frombits(r.bits)
			got, err := canon.Encode(canon.Float(f), canon.Canonical)
			switch {
			case r.want == "":
				refusal(t, err, canon.NotFinite)
			case math.Abs(f) > canon.MaxSafe:
				refusal(t, err, canon.TooLarge)
			case err != nil || string(got) != r.want:
				t.Errorf("%016x: canon writes %s, %v; the RFC %s", r.bits, got, err, r.want)
			}
			n++
		}
		if n != 26 {
			t.Fatalf("%d rows, want 26", n)
		}
	})
	t.Run("the sorting of section 3.2.3", func(t *testing.T) {
		t.Parallel()
		v, err := canon.Decode([]byte(sorting()))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		got, err := canon.Encode(v, canon.Canonical)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		inOrder(t, string(got))
	})
	t.Run("the sample of section 3.2.2 is refused at its 1e+30", func(t *testing.T) {
		t.Parallel()
		_, err := canon.Decode([]byte(sample()))
		ce := refusal(t, err, canon.TooLarge)
		if want := strings.Index(sample(), "1E30"); ce.Offset != want {
			t.Errorf("offset = %d, want %d", ce.Offset, want)
		}
	})
	t.Run("the sample less its 1e+30 has the bytes of section 3.2.4 less 1e+30", func(t *testing.T) {
		t.Parallel()
		v, err := canon.Decode([]byte(strings.Replace(sample(), " 1E30,", "", 1)))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		got, err := canon.Encode(v, canon.Canonical)
		want := strings.Replace(rfcSample(t), "1e+30,", "", 1)
		if err != nil || string(got) != want {
			t.Errorf("canon writes %q, %v; want %q", got, err, want)
		}
	})
}
