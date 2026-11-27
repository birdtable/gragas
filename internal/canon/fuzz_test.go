package canon_test

import (
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// FuzzDecode holds Decode, the reader of JSON the program did not write, to
// its contract on any input: it never panics, it refuses with one of its
// kinds at an offset inside the input, it refuses whatever the standard
// library's validator refuses, and a value it reads encodes in every layout
// to bytes that read back as the same value and encode again to the same
// bytes. Its Canonical bytes are what jsontext, the standard library's RFC
// 8785, makes of the input, so canon reads JSON as the RFC does. The seeds are in testdata/fuzz/FuzzDecode as well as below.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		`{"b":[1,2.5,-0,1e400],"a":{"":null}}`,
		u(`"U+d83dU+de00U+00e9U+00E9"`),
		u(`{"a":1,"U+0061":2}`),
		"[9007199254740991,9007199254740992]",
		"[1e-400,0e-400,5e-324]",
		"\xef\xbb\xbf[]",
		`"\ud800A"`,
		strings.Repeat("[", 600),
	} {
		f.Add([]byte(seed))
	}
	decodeKinds := []canon.Kind{canon.BadSyntax, canon.BadUTF8, canon.DuplicateKey, canon.TooLarge, canon.Underflow, canon.TooDeep}
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := canon.Decode(b)
		if err != nil {
			var ce *canon.Error
			if !errors.As(err, &ce) || !slices.Contains(decodeKinds, ce.Kind) {
				t.Fatalf("Decode refused with %v, not one of its kinds", err)
			}
			if ce.Offset < 0 || ce.Offset > len(b) {
				t.Fatalf("offset %d is outside the input of %d bytes", ce.Offset, len(b))
			}
			if json.Valid(b) && ce.Kind == canon.BadSyntax {
				t.Fatalf("Decode calls valid JSON bad syntax: %v", err)
			}
			return
		}
		if !json.Valid(b) {
			t.Fatalf("Decode read %q, which is not JSON", b)
		}
		canonical, err := canon.Encode(v, canon.Canonical)
		if err != nil {
			t.Fatalf("Encode refused a decoded value: %v", err)
		}
		jcs := jsontext.Value(slices.Clone(b))
		if err := jcs.Canonicalize(); err != nil || string(jcs) != string(canonical) {
			t.Fatalf("jsontext canonicalizes %q to %q, %v; canon wrote %q", b, jcs, err, canonical)
		}
		for _, layout := range layouts() {
			enc, err := canon.Encode(v, layout)
			if err != nil {
				t.Fatalf("Encode refused a decoded value: %v", err)
			}
			back, err := canon.Decode(enc)
			if err != nil {
				t.Fatalf("Decode refused canon's own bytes %q: %v", enc, err)
			}
			if !reflect.DeepEqual(back, v) {
				t.Fatalf("the encoding of a decoded value read back as another value")
			}
			again, err := canon.Encode(back, layout)
			if err != nil || string(again) != string(enc) {
				t.Fatalf("encoding twice gave other bytes")
			}
		}
	})
}

// FuzzRoundTrip draws values from the fuzzer's bytes and holds every one to
// the round-trip properties of roundTrip.
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "\x07\x03\x07\x02", "\x06\x04\x06\x04\x06\x04\x06\x04\x06\x04\x06\x04\x06\x04\x03", "\x07\x04\x05\x13\x14\x15\x16\x17\x05"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		roundTrip(t, gen(&entropy{b: b}, 0))
	})
}

// FuzzParseDigest holds ParseDigest, the reader of digests in files the
// program did not write, to its one form: what it reads it writes back
// byte for byte, and what it refuses it refuses as BadDigest inside the input.
func FuzzParseDigest(f *testing.F) {
	for _, seed := range []string{
		"sha256:ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356",
		"sha256:CA3D163BAB055381827226140568F3BEF7EAAC187CEBD76878E0B63E9E442356",
		"sha256:", "", "sha512:00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := canon.ParseDigest(s)
		if err != nil {
			var ce *canon.Error
			if !errors.As(err, &ce) || ce.Kind != canon.BadDigest || ce.Offset < 0 || ce.Offset > len(s) {
				t.Fatalf("ParseDigest(%q) refused with %v", s, err)
			}
			return
		}
		if d.String() != s {
			t.Fatalf("ParseDigest(%q).String() = %q", s, d.String())
		}
	})
}

// sumsThrough is the one gragas package canon may import: the leaf that owns
// SHA-256 (arch.shared.8), which imports nothing of gragas's.
const sumsThrough = "github.com/birdtable/gragas/internal/digest"

// gragasImports is the imports of a Go source file that name a gragas
// package other than sumsThrough.
func gragasImports(t *testing.T, name string, src []byte) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if (p == "github.com/birdtable/gragas" || strings.HasPrefix(p, "github.com/birdtable/gragas/")) && p != sumsThrough {
			out = append(out, p)
		}
	}
	return out
}

func TestNoNonTestFileImportsAGragasPackage(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, en := range entries {
		name := en.Name()
		if en.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if imps := gragasImports(t, name, src); len(imps) > 0 {
			t.Errorf("%s imports %q: canon is a leaf and imports nothing of gragas's but internal/digest", name, imps)
		}
		read++
	}
	if read == 0 {
		t.Fatal("no non-test Go files read")
	}
}

func TestTheLeafCheckSeesAPlantedImport(t *testing.T) {
	t.Parallel()
	plant := []byte("package canon\n\nimport (\n\t\"strings\"\n\t\"github.com/birdtable/gragas/internal/digest\"\n\t\"github.com/birdtable/gragas/internal/model\"\n)\n")
	if got := gragasImports(t, "plant.go", plant); !slices.Equal(got, []string{"github.com/birdtable/gragas/internal/model"}) {
		t.Errorf("the planted import was not seen: %q", got)
	}
	clean := []byte("package canon\n\nimport \"github.com/birdtable/gragasx/y\"\n")
	if got := gragasImports(t, "clean.go", clean); len(got) != 0 {
		t.Errorf("a look-alike module was taken for gragas: %q", got)
	}
}
