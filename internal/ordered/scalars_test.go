package ordered_test

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// scalarOf is the one scalar of a document of a key and its value, the value's.
func valueOf(t *testing.T, doc string) ordered.Scalar {
	t.Helper()
	got, err := ordered.Scalars([]byte(doc))
	if err != nil || len(got) != 2 || !got[0].Key || got[1].Key {
		t.Fatalf("Scalars(%q) = %+v, %v, want a key and a value", doc, got, err)
	}
	return got[1]
}

// TestScalarsGiveBothReadingsOfAPlainScalar: the 1.1 tables and the 1.2 core
// schema part on each row below, and agree on the last ones.
func TestScalarsGiveBothReadingsOfAPlainScalar(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, text string
		r11, r12   any
	}{
		{"yes is a 1.1 boolean and 1.2 text", "yes", true, "yes"},
		{"On is a 1.1 boolean and 1.2 text", "On", true, "On"},
		{"no is a 1.1 boolean and 1.2 text", "no", false, "no"},
		{"a leading zero is 1.1 octal and 1.2 decimal", "010", int64(8), int64(10)},
		{"0o is 1.2 octal and 1.1 text", "0o10", "0o10", int64(8)},
		{"a big 0o is a 1.2 float and 1.1 text", "0o7777777777777777777777", "0o7777777777777777777777", float64(1 << 66)},
		{"an underscore is 1.1 grouping and 1.2 text", "1_000", int64(1000), "1_000"},
		{"0b is 1.1 binary and 1.2 text", "0b11", int64(3), "0b11"},
		{"an exponent with no dot is 1.2 float and 1.1 text", "1e3", "1e3", float64(1000)},
		{"a date is a 1.1 timestamp and 1.2 text", "2026-01-02", ordered.Timestamp("2026-01-02"), "2026-01-02"},
		{"a colon is 1.1 base 60 and 1.2 text", "1:30", int64(90), "1:30"},
		{"hex is the same in both", "0x1F", int64(31), int64(31)},
		{"true is the same in both", "true", true, true},
		{"False is the same in both", "False", false, false},
		{"a decimal is the same in both", "12", int64(12), int64(12)},
		{"a negative decimal is the same in both", "-7", int64(-7), int64(-7)},
		{"a float is the same in both", "1.5", 1.5, 1.5},
		{"inf is the same in both", ".inf", math.Inf(1), math.Inf(1)},
		{"negative inf is the same in both", "-.Inf", math.Inf(-1), math.Inf(-1)},
		{"a tilde is null in both", "~", nil, nil},
		{"null is null in both", "null", nil, nil},
		{"a word is text in both", "text", "text", "text"},
		{"an integer int64 cannot hold is refused by the gate", "9223372036854775808", nil, nil},
	} {
		doc := "k: " + c.text + "\n"
		if c.text == "9223372036854775808" {
			// ParseStrict refuses an integer int64 cannot hold: the gate is one.
			if _, err := ordered.Scalars([]byte(doc)); !errors.Is(err, ordered.ErrSyntax) {
				t.Errorf("Scalars(%q) error = %v, want what ParseStrict says", doc, err)
			}
			continue
		}
		s := valueOf(t, doc)
		if !s.Plain || s.Text != c.text || s.Line != 1 || s.Tag != "" {
			t.Errorf("%s: %+v, want a plain scalar of that text at line 1 with no tag", c.name, s)
		}
		if !reflect.DeepEqual(s.Read11, c.r11) || !reflect.DeepEqual(s.Read12, c.r12) {
			t.Errorf("%s: Read11 %#v, Read12 %#v, want %#v and %#v", c.name, s.Read11, s.Read12, c.r11, c.r12)
		}
	}
}

// TestScalarsReadQuotedAndBlockAsText: only a plain scalar is resolved.
func TestScalarsReadQuotedAndBlockAsText(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{
		"k: 'yes'\n",
		"k: \"yes\"\n",
		"k: |\n  yes\n",
		"k: >\n  yes\n",
	} {
		got := valueOf(t, doc)
		want := "yes"
		if strings.Contains(doc, "|") || strings.Contains(doc, ">") {
			want = "yes\n"
		}
		if got.Plain || got.Read11 != want || got.Read12 != want || got.Text != want {
			t.Errorf("%q: %+v, want text %q under both readings and not plain", doc, got, want)
		}
	}
}

// TestScalarsComeInDocumentOrderWithTheirLines: keys and values, nested and
// in a list, each at the line it starts on; a key is told from a value.
func TestScalarsComeInDocumentOrderWithTheirLines(t *testing.T) {
	t.Parallel()
	got, err := ordered.Scalars([]byte("a: 1\nb:\n  - x\n  - 0o7: y\n\nc: 'z'\n"))
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, s := range got {
		kind := "v"
		if s.Key {
			kind = "k"
		}
		seen = append(seen, kind+":"+s.Text+"@"+string(rune('0'+s.Line)))
	}
	want := "k:a@1 v:1@1 k:b@2 v:x@3 k:0o7@4 v:y@4 k:c@6 v:z@6"
	if g := strings.Join(seen, " "); g != want {
		t.Errorf("Scalars = %s, want %s", g, want)
	}
}

// TestScalarsRefuseWhatParseStrictRefuses: one gate, so the error is
// ParseStrict's, word for word, for a tag, an anchor, an alias, a merge key, a
// repeated key, a key that is not text, a second document and a broken one.
func TestScalarsRefuseWhatParseStrictRefuses(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{
		"a: !!str 1\n",
		"a: !x yes\n",
		"a: &x 1\nb: *x\n",
		"a:\n  <<: {b: 1}\n",
		"a: 1\na: 2\n",
		"yes: 1\n",
		"a: 1\n---\nb: 2\n",
		"a: [\n",
	} {
		_, want := ordered.ParseStrict([]byte(doc))
		got, err := ordered.Scalars([]byte(doc))
		if want == nil {
			t.Fatalf("%q: ParseStrict reads it, the case does not test a refusal", doc)
		}
		if got != nil || err == nil || err.Error() != want.Error() || !errors.Is(err, ordered.ErrSyntax) {
			t.Errorf("%q: Scalars = %v, %v, want ParseStrict's error %v", doc, got, err, want)
		}
	}
}

// TestScalarsFindTheNonSpecificTagTheNodeDrops: ParseStrict reads "! yes" as
// it reads "yes", and the parser keeps no trace of the tag on the node, so the
// scalar is tagged "!" from the source, wherever the tag sits: after a key, in
// a list, on a key, before a quote, on the line before its value, after a
// byte order mark, past a line break of each kind. Its 1.2 reading is its text.
func TestScalarsFindTheNonSpecificTagTheNodeDrops(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		doc  string
		text string
		key  bool
	}{
		{"a: ! yes\n", "yes", false},
		{"a: !  010\n", "010", false},
		{"a:\n  - ! true\n", "true", false},
		{"! a: 1\n", "a", true},
		{"a: ! 'x'\n", "x", false},
		{"a: !\n  yes\n", "yes", false},
		{"\ufeffa: ! yes\n", "yes", false},
		{"b: 1\r\na: ! yes\r\n", "yes", false},
		{"b: 1\ra: ! yes\r", "yes", false},
		{"b: 1\u2028a: ! yes\n", "yes", false},
		{"é: ! yes\n", "yes", false},
	} {
		got, err := ordered.Scalars([]byte(c.doc))
		if err != nil {
			t.Fatalf("%q: %v", c.doc, err)
		}
		var tagged []ordered.Scalar
		for _, s := range got {
			if s.Tag != "" {
				tagged = append(tagged, s)
			}
		}
		if len(tagged) != 1 || tagged[0].Tag != "!" || tagged[0].Text != c.text || tagged[0].Key != c.key || tagged[0].Read12 != c.text {
			t.Errorf("%q: tagged %+v, want one scalar %q tagged ! reading as its text", c.doc, tagged, c.text)
		}
	}
	// What the tag is not: the same text untagged keeps its two readings.
	if s := valueOf(t, "a: yes\n"); s.Tag != "" || s.Read11 != true || s.Read12 != "yes" {
		t.Errorf("an untagged yes: %+v", s)
	}
}

// TestScalarsOfNothingAreNothing: an empty document, a comment and a null
// document have no scalar to judge.
func TestScalarsOfNothingAreNothing(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{"", "# only a comment\n", "~\n"} {
		if got, err := ordered.Scalars([]byte(doc)); err != nil || len(got) != 0 {
			t.Errorf("Scalars(%q) = %+v, %v, want none", doc, got, err)
		}
	}
}

// FuzzScalars: Scalars refuses what ParseStrict refuses with its error, and
// what it reads it reads whole: every scalar has a line, a plain scalar's
// Read11 is the value ParseStrict would give it, and an unplain scalar reads as
// its text both ways.
func FuzzScalars(f *testing.F) {
	for _, seed := range []string{
		"k: yes\n",
		"k: 010\nj: 0o10\n",
		"k: 1_000\nj: 0b11\nh: 1e3\n",
		"d: 2026-01-02\nt: 2026-01-02 03:04:05\n",
		"k: 'yes'\nj: \"no\"\nh: |\n  on\n",
		"a: ! yes\n",
		"a: &x 1\nb: *x\n",
		"k: .inf\nj: -.Inf\nh: .NaN\n",
		"k: 9223372036854775808\n",
		"- - - [\n",
		"? [a, b]\n: c\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, strictErr := ordered.ParseStrict(data)
		got, err := ordered.Scalars(data)
		if strictErr != nil {
			if err == nil || err.Error() != strictErr.Error() {
				t.Fatalf("ParseStrict refuses with %v, Scalars says %v", strictErr, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Scalars refuses what ParseStrict reads: %v", err)
		}
		for _, s := range got {
			if s.Line < 1 || (s.Tag != "" && s.Tag != "!") {
				t.Fatalf("scalar %+v has no line, or a tag ParseStrict would have refused", s)
			}
			if s.Tag == "!" && s.Read12 != s.Text {
				t.Fatalf("scalar %+v is tagged ! and does not read as its text in 1.2", s)
			}
			if !s.Plain && (s.Read11 != s.Text || s.Read12 != s.Text) {
				t.Fatalf("scalar %+v is not plain and does not read as its text", s)
			}
			switch s.Read12.(type) {
			case nil, bool, int64, float64, string:
			default:
				t.Fatalf("scalar %+v has a 1.2 reading of a type outside nil, bool, int64, float64 and string", s)
			}
		}
	})
}

// TestAnEncodingTheSourceFinderCannotFollowIsRefused: the parser decodes UTF-16
// and misreads a second byte order mark, so a document in either is refused by
// the one decode that ParseStrict, ParseYAML, ParseTextKeys and Scalars share,
// before any tag is looked for; one leading UTF-8 mark is still read.
func TestAnEncodingTheSourceFinderCannotFollowIsRefused(t *testing.T) {
	t.Parallel()
	utf16 := func(s string, le bool) []byte {
		out := []byte{0xFE, 0xFF}
		if le {
			out = []byte{0xFF, 0xFE}
		}
		for _, r := range s {
			if le {
				out = append(out, byte(r), 0)
			} else {
				out = append(out, 0, byte(r))
			}
		}
		return out
	}
	bom := "\xEF\xBB\xBF"
	for _, c := range []struct {
		name string
		doc  []byte
	}{
		{"UTF-16 little-endian with its mark", utf16("opts:\n  a: ! true\n", true)},
		{"UTF-16 big-endian with its mark", utf16("opts:\n  a: ! true\n", false)},
		{"UTF-16 with NUL bytes and no mark", []byte("a\x00:\x00 \x001\x00\n\x00")},
		{"a doubled mark at the start", []byte(bom + bom + "\n#opts:\n#  a: true\n")},
		{"a doubled mark before a tag", []byte(bom + bom + "\n\nopts: {a: ! true}\n")},
		{"a mark in the middle of the file", []byte("a: 1\n" + bom + "b: 2\n")},
		{"bytes that are not UTF-8", []byte("a: \xff\n")},
	} {
		for name, read := range map[string]func([]byte) (any, error){
			"ParseStrict":   ordered.ParseStrict,
			"ParseYAML":     ordered.ParseYAML,
			"ParseTextKeys": ordered.ParseTextKeys,
			"Scalars":       func(d []byte) (any, error) { return ordered.Scalars(d) },
		} {
			if v, err := read(c.doc); err == nil || !errors.Is(err, ordered.ErrSyntax) {
				t.Errorf("%s, %s: %v, %v, want a refusal", c.name, name, v, err)
			}
		}
	}
	if s := valueOf(t, bom+"a: ! yes\n"); s.Tag != "!" {
		t.Errorf("one leading UTF-8 mark: %+v, want it read and the tag found", s)
	}
}
