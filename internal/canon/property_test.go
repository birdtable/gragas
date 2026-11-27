package canon_test

import (
	"encoding/binary"
	"encoding/json/jsontext"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
)

// entropy hands out bytes: from b, then from more when b is spent, or zeros
// when there is no more. The property tests draw it from SHA-256 over a
// counter and the fuzz targets from the fuzzer's input, so no test here reads
// math/rand.
type entropy struct {
	b    []byte
	i    int
	more func() []byte
}

func (e *entropy) byte() byte {
	if e.i >= len(e.b) {
		if e.more == nil {
			return 0
		}
		e.b, e.i = e.more(), 0
	}
	c := e.b[e.i]
	e.i++
	return c
}

func (e *entropy) u64() uint64 {
	var b [8]byte
	for i := range b {
		b[i] = e.byte()
	}
	return binary.LittleEndian.Uint64(b[:])
}

// hashed is entropy from SHA-256 over seed and a counter.
func hashed(seed string) *entropy {
	n := uint64(0)
	return &entropy{more: func() []byte {
		n++
		s := digest.Sum(binary.LittleEndian.AppendUint64([]byte(seed), n))
		return s[:]
	}}
}

// pieces are the strings generated text is made of: the characters an
// encoder could get wrong.
func pieces() []string {
	return []string{
		"a", "Z", "0", " ", "~", "/", "\"", "\\", "\n", "\t", "\x00", "\x01", "\x1f", "\x7f",
		"é", "é", " ", "ﬁ", "￿", "\U0001f600", "<", "&",
	}
}

func text(e *entropy) string {
	ps := pieces()
	n := int(e.byte() % 6)
	s := ""
	for range n {
		s += ps[int(e.byte())%len(ps)]
	}
	return s
}

// gen is a value drawn from e, nested at most six deep below depth.
func gen(e *entropy, depth int) canon.Value {
	k := e.byte() % 8
	if depth >= 6 {
		k %= 6
	}
	switch k {
	case 0:
		return canon.Null{}
	case 1:
		return canon.Bool(e.byte()&1 == 1)
	case 2:
		if e.byte()&1 == 0 {
			return canon.Int(int64(int8(e.byte())))
		}
		return canon.Int(int64(e.u64()%(2*canon.MaxSafe+1)) - canon.MaxSafe)
	case 3:
		f := math.Float64frombits(e.u64())
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > canon.MaxSafe {
			f = float64(int8(e.byte())) / 4
		}
		return canon.Float(f)
	case 4, 5:
		return canon.String(text(e))
	case 6:
		a := canon.Array{}
		for range e.byte() % 5 {
			a = append(a, gen(e, depth+1))
		}
		return a
	default:
		o := canon.Object{}
		for range e.byte() % 5 {
			o[text(e)] = gen(e, depth+1)
		}
		return o
	}
}

// layouts is every layout, as the round trips run them.
func layouts() []canon.Layout { return []canon.Layout{canon.Document, canon.Line, canon.Canonical} }

// normal is v as it reads back: RFC 8785 writes a whole Float as its Int and
// negative zero as 0, so Decode reads either as an Int.
func normal(v canon.Value) canon.Value {
	switch x := v.(type) {
	case canon.Float:
		if f := float64(x); f == math.Trunc(f) {
			return canon.Int(int64(f))
		}
	case canon.Array:
		out := make(canon.Array, len(x))
		for i, e := range x {
			out[i] = normal(e)
		}
		return out
	case canon.Object:
		out := make(canon.Object, len(x))
		for k, e := range x {
			out[k] = normal(e)
		}
		return out
	}
	return v
}

// roundTrip asserts the properties every value has: it reads back from every
// layout as itself, its numbers as normal gives them; encoding what was read
// gives the same bytes; its Canonical bytes are what jsontext, the standard
// library's RFC 8785, makes of its Document; and its digest is the Sum of its
// Canonical bytes.
func roundTrip(t *testing.T, v canon.Value) {
	t.Helper()
	enc := map[canon.Layout][]byte{}
	for _, layout := range layouts() {
		b, err := canon.Encode(v, layout)
		if err != nil {
			t.Fatalf("Encode(%#v): %v", v, err)
		}
		back, err := canon.Decode(b)
		if err != nil {
			t.Fatalf("Decode(%q): %v", b, err)
		}
		if want := normal(v); !reflect.DeepEqual(back, want) {
			t.Fatalf("Decode(Encode(v)) = %#v, want %#v", back, want)
		}
		again, err := canon.Encode(back, layout)
		if err != nil || string(again) != string(b) {
			t.Fatalf("Encode(Decode(b)) = %q, %v; want %q", again, err, b)
		}
		enc[layout] = b
	}
	jcs := jsontext.Value(slices.Clone(enc[canon.Document]))
	if err := jcs.Canonicalize(); err != nil || string(jcs) != string(enc[canon.Canonical]) {
		t.Fatalf("jsontext canonicalizes the document to %q, %v; canon wrote %q", jcs, err, enc[canon.Canonical])
	}
	d, err := canon.DigestOf(v)
	if want := sha(enc[canon.Canonical]); err != nil || d.String() != want {
		t.Fatalf("DigestOf = %s, %v; want the SHA-256 of the canonical bytes %s", d, err, want)
	}
}

func TestEveryValueReadsBackAsItselfFromEveryLayout(t *testing.T) {
	t.Parallel()
	e := hashed("round trip")
	n := 0
	for range 3000 {
		roundTrip(t, gen(e, 0))
		n++
	}
	if n != 3000 {
		t.Fatalf("%d values, want 3000", n)
	}
}

// shuffled is keys in an order drawn from e (Fisher and Yates).
func shuffled(e *entropy, keys []string) []string {
	s := append([]string(nil), keys...)
	for i := len(s) - 1; i > 0; i-- {
		j := int(e.u64() % uint64(i+1))
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// build is an Object with keys inserted in the order given, each holding an
// Object of the same keys inserted in reverse.
func build(order []string) canon.Object {
	o := canon.Object{}
	for _, k := range order {
		inner := canon.Object{}
		for _, ik := range slices.Backward(order) {
			inner[ik] = canon.String(ik)
		}
		o[k] = inner
	}
	return o
}

func TestAnObjectBuiltFromTwoOrdersEncodesToTheSameBytes(t *testing.T) {
	t.Parallel()
	e := hashed("order")
	keys := make([]string, 0, 64)
	seen := map[string]bool{}
	for len(keys) < 64 {
		if k := text(e) + string(rune('a'+len(keys)%26)); !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	first := shuffled(e, keys)
	second := shuffled(e, keys)
	if reflect.DeepEqual(first, second) {
		t.Fatal("the two orders are one order")
	}
	want, err := canon.Encode(build(first), canon.Document)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for i := range 20 {
		got, err := canon.Encode(build(second), canon.Document)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("run %d: the second order gave other bytes", i)
		}
	}
}

func TestAGoldenBuiltInShuffledOrderHasItsLiteralBytes(t *testing.T) {
	t.Parallel()
	e := hashed("golden order")
	keys := []string{"\U0001f600", "ﬁ", "é", "z", "Z"}
	const want = "{\"Z\":0,\"z\":1,\"é\":2,\"\U0001f600\":4,\"ﬁ\":3}\n"
	values := map[string]canon.Int{"Z": 0, "z": 1, "é": 2, "ﬁ": 3, "\U0001f600": 4}
	runs := 0
	for range 50 {
		o := canon.Object{}
		for _, k := range shuffled(e, keys) {
			o[k] = values[k]
		}
		got, err := canon.Encode(o, canon.Line)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		runs++
	}
	if runs != 50 {
		t.Fatalf("%d runs, want 50", runs)
	}
}
