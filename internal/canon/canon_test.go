package canon_test

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// golden is a value with its Document and Canonical encodings and its digest,
// every one a literal. Line is the canonical bytes and an LF. The digests were
// made by sha256sum over the canonical bytes written with printf, not by this
// package.
type golden struct {
	name      string
	v         canon.Value
	document  string
	canonical string
	digest    string
}

func goldens() []golden {
	return []golden{
		{
			name: "empty object", v: canon.Object{},
			document: "{}\n", canonical: "{}",
			digest: "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		},
		{
			name: "nil array", v: canon.Array(nil),
			document: "[]\n", canonical: "[]",
			digest: "sha256:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945",
		},
		{
			name:      "empty containers inside",
			v:         canon.Object{"c": canon.Array{canon.Object(nil)}, "a": canon.Array{}, "b": canon.Object{}},
			document:  "{\n  \"a\": [],\n  \"b\": {},\n  \"c\": [\n    {}\n  ]\n}\n",
			canonical: "{\"a\":[],\"b\":{},\"c\":[{}]}",
			digest:    "sha256:bab09cc6eaf6b4ecb7304da6011ce5f786e3dc29ef8dd255e99365c41e23eb32",
		},
		{
			// A whole Float is written as its Int is, and negative zero as 0.
			name: "whole numbers to 2^53 - 1",
			v: canon.Array{
				canon.Int(canon.MaxSafe), canon.Int(-canon.MaxSafe), canon.Int(0),
				canon.Float(1), canon.Float(math.Copysign(0, -1)), canon.Float(1e2),
			},
			document:  "[\n  9007199254740991,\n  -9007199254740991,\n  0,\n  1,\n  0,\n  100\n]\n",
			canonical: "[9007199254740991,-9007199254740991,0,1,0,100]",
			digest:    "sha256:3b920a8066efa1875c83eab625bb1d4e9b9d8e43fb3103e07d424f2343a70518",
		},
		{
			// Every sample of RFC 8785 Appendix B within MaxSafe, by its bits,
			// with the RFC's text for it.
			name: "float forms of RFC 8785 Appendix B",
			v: canon.Array{
				canon.Float(0.5), canon.Float(-2.5),
				bits(0x0000000000000001), bits(0x8000000000000001),
				bits(0x3eb0c6f7a0b5ed8c), bits(0x3eb0c6f7a0b5ed8d), canon.Float(1.5e-7),
				bits(0x41b3de4355555553), bits(0x41b3de4355555554), bits(0x41b3de4355555555),
				bits(0x41b3de4355555556), bits(0x41b3de4355555557),
				bits(0xbecbf647612f3696), bits(0x43143ff3c1cb0959),
			},
			document: "[\n  0.5,\n  -2.5,\n  5e-324,\n  -5e-324,\n  9.999999999999997e-7,\n  0.000001,\n  1.5e-7,\n" +
				"  333333333.3333332,\n  333333333.33333325,\n  333333333.3333333,\n  333333333.3333334,\n" +
				"  333333333.33333343,\n  -0.0000033333333333333333,\n  1424953923781206.2\n]\n",
			canonical: "[0.5,-2.5,5e-324,-5e-324,9.999999999999997e-7,0.000001,1.5e-7,333333333.3333332," +
				"333333333.33333325,333333333.3333333,333333333.3333334,333333333.33333343," +
				"-0.0000033333333333333333,1424953923781206.2]",
			digest: "sha256:35d11e09a988e30bbc967cc3b3c350167b7ee8b36ef671e2730304f2503c4621",
		},
		{
			// RFC 8785 section 3.2.3's sorting test: U+1F600 sorts before
			// U+FB33 by its UTF-16 code units, where byte order would put it
			// after.
			name: "keys in UTF-16 order, RFC 8785 section 3.2.3",
			v: canon.Object{
				"\u20ac": canon.String("Euro Sign"), "\r": canon.String("Carriage Return"),
				"\ufb33": canon.String("Hebrew Letter Dalet With Dagesh"), "1": canon.String("One"),
				"\U0001f600": canon.String("Emoji: Grinning Face"), "\u0080": canon.String("Control"),
				"\u00f6": canon.String("Latin Small Letter O With Diaeresis"),
			},
			document: "{\n  \"\\r\": \"Carriage Return\",\n  \"1\": \"One\",\n  \"\u0080\": \"Control\",\n" +
				"  \"ö\": \"Latin Small Letter O With Diaeresis\",\n  \"€\": \"Euro Sign\",\n" +
				"  \"\U0001f600\": \"Emoji: Grinning Face\",\n  \"\ufb33\": \"Hebrew Letter Dalet With Dagesh\"\n}\n",
			canonical: "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\"," +
				"\"ö\":\"Latin Small Letter O With Diaeresis\",\"€\":\"Euro Sign\"," +
				"\"\U0001f600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}",
			digest: "sha256:5e321556d22018a9656991a9e94f77ec175fa193e52a2429d312f8419ec8b08c",
		},
		{
			// The short escapes, \u00xx for the other controls, and nothing
			// else escaped: not DEL, the slash, U+2028 or the HTML characters.
			name:      "escapes",
			v:         canon.String("q\" b\\ n\n t\t r\r bs\b ff\f c\x01 us\x1f del\x7f / \u2028 <>&"),
			document:  "\"q\\\" b\\\\ n\\n t\\t r\\r bs\\b ff\\f c\\u0001 us\\u001f del\x7f / \u2028 <>&\"\n",
			canonical: "\"q\\\" b\\\\ n\\n t\\t r\\r bs\\b ff\\f c\\u0001 us\\u001f del\x7f / \u2028 <>&\"",
			digest:    "sha256:14bc293a0cc48587a2f11c3c7bbe8f0c5bf044e00735c0d9ae3979c00f656001",
		},
		{
			name: "composed accent", v: canon.String("\u00e9"),
			document: "\"\u00e9\"\n", canonical: "\"\u00e9\"",
			digest: "sha256:f2886017e9c7abacf804b54d64787dce2b611c9544ba21f3affdd126a6e50086",
		},
		{
			name: "decomposed accent, not normalised", v: canon.String("e\u0301"),
			document: "\"e\u0301\"\n", canonical: "\"e\u0301\"",
			digest: "sha256:3d68ce21f2899a475713cdbe7562ba9bdb6b1dfde8af1f221bdff4a0935b53b2",
		},
		{
			name: "a document",
			v: canon.Object{
				"b": canon.Int(1),
				"a": canon.Object{"d": canon.Bool(true), "c": canon.Null{}},
				"A": canon.Array{canon.String("x"), canon.Bool(false)},
			},
			document:  "{\n  \"A\": [\n    \"x\",\n    false\n  ],\n  \"a\": {\n    \"c\": null,\n    \"d\": true\n  },\n  \"b\": 1\n}\n",
			canonical: "{\"A\":[\"x\",false],\"a\":{\"c\":null,\"d\":true},\"b\":1}",
			digest:    "sha256:b1b76fc104f745d6cd2154f6f483a7c925aa82bd6e1f5a7cbbd4679d4a6875d2",
		},
		{
			// RFC 8785 section 3.2.2's sample, whose bytes section 3.2.4 gives,
			// less its 1e+30: a whole number past 2^53 - 1, which canon refuses.
			name: "the sample of RFC 8785 section 3.2.2, less 1e+30",
			v: canon.Object{
				"numbers": canon.Array{
					canon.Float(333333333.33333329), canon.Float(4.50), canon.Float(2e-3),
					canon.Float(0.000000000000000000000000001),
				},
				"string":   canon.String("\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"/"),
				"literals": canon.Array{canon.Null{}, canon.Bool(true), canon.Bool(false)},
			},
			document: "{\n  \"literals\": [\n    null,\n    true,\n    false\n  ],\n  \"numbers\": [\n" +
				"    333333333.3333333,\n    4.5,\n    0.002,\n    1e-27\n  ],\n" +
				"  \"string\": \"€$\\u000f\\nA'B\\\"\\\\\\\\\\\"/\"\n}\n",
			canonical: "{\"literals\":[null,true,false],\"numbers\":[333333333.3333333,4.5,0.002,1e-27]," +
				"\"string\":\"€$\\u000f\\nA'B\\\"\\\\\\\\\\\"/\"}",
			digest: "sha256:670b2058ab2793b25083f61d252e6e830859c658656569de160897232b6b69cc",
		},
		{
			name: "empty string", v: canon.String(""),
			document: "\"\"\n", canonical: "\"\"",
			digest: "sha256:12ae32cb1ec02d01eda3581b127c1fee3b0dc53572ed6baf239721a03d82e126",
		},
		{
			name: "null", v: canon.Null{},
			document: "null\n", canonical: "null",
			digest: "sha256:74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b",
		},
	}
}

// bits is the Float with the IEEE 754 bits b, as RFC 8785 Appendix B names
// its samples.
func bits(b uint64) canon.Float { return canon.Float(math.Float64frombits(b)) }

func TestTheGoldenValuesEncodeToTheirLiteralBytes(t *testing.T) {
	t.Parallel()
	gs := goldens()
	if len(gs) == 0 {
		t.Fatal("no goldens")
	}
	for _, g := range gs {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			for _, c := range []struct {
				layout canon.Layout
				want   string
			}{{canon.Document, g.document}, {canon.Line, g.canonical + "\n"}, {canon.Canonical, g.canonical}} {
				got, err := canon.Encode(g.v, c.layout)
				if err != nil {
					t.Fatalf("Encode(layout %d): %v", c.layout, err)
				}
				if string(got) != c.want {
					t.Errorf("Encode(layout %d)\n got %q\nwant %q", c.layout, got, c.want)
				}
			}
		})
	}
}

func TestTheGoldenDigestsEqualTheirLiteralStrings(t *testing.T) {
	t.Parallel()
	gs := goldens()
	if len(gs) == 0 {
		t.Fatal("no goldens")
	}
	for _, g := range gs {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			d, err := canon.DigestOf(g.v)
			if err != nil {
				t.Fatalf("DigestOf: %v", err)
			}
			if d.String() != g.digest {
				t.Errorf("DigestOf = %s, want %s", d, g.digest)
			}
			if s := sha([]byte(g.canonical)); s != g.digest {
				t.Errorf("SHA-256 of the canonical bytes = %s, want %s", s, g.digest)
			}
		})
	}
}

func TestAComposedAndADecomposedAccentHaveTwoDigests(t *testing.T) {
	t.Parallel()
	a, errA := canon.DigestOf(canon.String("é"))
	b, errB := canon.DigestOf(canon.String("é"))
	if errA != nil || errB != nil {
		t.Fatalf("DigestOf: %v, %v", errA, errB)
	}
	if a == b {
		t.Errorf("both digests are %s; canon must not normalise Unicode", a)
	}
}

// refusal asserts err is a canon refusal of kind k.
func refusal(t *testing.T, err error, k canon.Kind) *canon.Error {
	t.Helper()
	var ce *canon.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a *canon.Error of kind %q", err, k)
	}
	if ce.Kind != k {
		t.Fatalf("kind = %q (%v), want %q", ce.Kind, err, k)
	}
	return ce
}

// embedded implements canon.Value by embedding a canon type, from outside
// the package.
type embedded struct {
	canon.Null
	secret string
}

// nested is depth arrays, one inside the next, around null.
func nested(depth int) canon.Value {
	var v canon.Value = canon.Null{}
	for range depth {
		v = canon.Array{v}
	}
	return v
}

func TestTheEncoderRefusesWhatHasNoCanonicalForm(t *testing.T) {
	t.Parallel()
	selfArray := canon.Array{nil}
	selfArray[0] = selfArray
	selfObject := canon.Object{}
	selfObject["me"] = selfObject
	tests := []struct {
		name string
		v    canon.Value
		kind canon.Kind
		path string
	}{
		{"a nil value at the top", nil, canon.NilValue, ""},
		{"a nil value in an array", canon.Array{canon.Int(1), nil}, canon.NilValue, "/1"},
		{"a nil value in an object", canon.Object{"a/b~": nil}, canon.NilValue, "/a~1b~0"},
		{"a string that is not UTF-8", canon.Object{"k": canon.String("\xff")}, canon.BadUTF8, "/k"},
		{"a key that is not UTF-8", canon.Object{"\xc0\x80": canon.Null{}}, canon.BadUTF8, ""},
		{"NaN", canon.Array{canon.Float(math.NaN())}, canon.NotFinite, "/0"},
		{"plus infinity", canon.Float(math.Inf(1)), canon.NotFinite, ""},
		{"minus infinity", canon.Float(math.Inf(-1)), canon.NotFinite, ""},
		{"an Int one past 2^53 - 1", canon.Object{"n": canon.Int(canon.MaxSafe + 1)}, canon.TooLarge, "/n"},
		{"an Int one below -(2^53 - 1)", canon.Int(-canon.MaxSafe - 1), canon.TooLarge, ""},
		{"the largest Int", canon.Int(math.MaxInt64), canon.TooLarge, ""},
		{"the smallest Int", canon.Int(math.MinInt64), canon.TooLarge, ""},
		{"a Float of 2^53", canon.Float(1 << 53), canon.TooLarge, ""},
		{"a Float of -2^53", canon.Float(-(1 << 53)), canon.TooLarge, ""},
		{"the 1e+30 of RFC 8785's sample", canon.Array{canon.Float(4.5), canon.Float(1e30)}, canon.TooLarge, "/1"},
		{"the largest Float", canon.Float(math.MaxFloat64), canon.TooLarge, ""},
		{"one level past MaxDepth", nested(canon.MaxDepth + 1), canon.TooDeep, ""},
		{"an array that holds itself", selfArray, canon.TooDeep, ""},
		{"an object that holds itself", selfObject, canon.TooDeep, ""},
		{"a type that embeds a canon type", canon.Array{embedded{secret: "x"}}, canon.Misuse, "/0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := canon.Encode(tc.v, canon.Document)
			if got != nil {
				t.Errorf("Encode returned %d bytes with its refusal", len(got))
			}
			ce := refusal(t, err, tc.kind)
			if tc.kind != canon.TooDeep && ce.Path != tc.path {
				t.Errorf("path = %q, want %q", ce.Path, tc.path)
			}
			if ce.Offset != -1 {
				t.Errorf("offset = %d, want -1 for an encoder refusal", ce.Offset)
			}
			if _, err := canon.DigestOf(tc.v); err == nil {
				t.Error("DigestOf took what Encode refused")
			}
		})
	}
}

func TestAValueAtMaxDepthIsEncoded(t *testing.T) {
	t.Parallel()
	got, err := canon.Encode(nested(canon.MaxDepth), canon.Line)
	if err != nil {
		t.Fatalf("Encode at MaxDepth: %v", err)
	}
	if len(got) != 2*canon.MaxDepth+len("null\n") {
		t.Errorf("len = %d", len(got))
	}
}

func TestAnUnknownLayoutIsRefused(t *testing.T) {
	t.Parallel()
	_, err := canon.Encode(canon.Null{}, canon.Layout(7))
	refusal(t, err, canon.Misuse)
}

func TestTheErrorSaysItsKindAndWhere(t *testing.T) {
	t.Parallel()
	_, encErr := canon.Encode(canon.Array{nil}, canon.Line)
	_, decErr := canon.Decode([]byte(`{"a":1,"a":2}`))
	for _, c := range []struct {
		err  error
		want string
	}{
		{encErr, `canon: nil value at "/0": a nil Value: write Null{} for null, or leave the member out`},
		{decErr, `canon: duplicate key at byte 7: "a" twice`},
	} {
		if c.err == nil {
			t.Errorf("no refusal, want %q", c.want)
		} else if c.err.Error() != c.want {
			t.Errorf("Error() = %q, want %q", c.err.Error(), c.want)
		}
	}
}

func TestEveryKindHasAWord(t *testing.T) {
	t.Parallel()
	kinds := []canon.Kind{
		canon.BadSyntax, canon.BadUTF8, canon.DuplicateKey, canon.TooLarge, canon.NotFinite,
		canon.Underflow, canon.TooDeep, canon.NilValue, canon.Misuse, canon.BadDigest, canon.DuplicateID,
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		w := k.String()
		if w == "" || seen[w] {
			t.Errorf("kind %d has the word %q, empty or taken", int(k), w)
		}
		seen[w] = true
	}
	if len(seen) != 11 {
		t.Fatalf("%d words, want 11", len(seen))
	}
	if got := canon.Kind(0).String(); got != "kind 0" {
		t.Errorf("Kind(0) = %q, want kind 0", got)
	}
}
