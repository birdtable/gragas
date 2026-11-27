package canon_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
)

// sha is the text form of the SHA-256 of b, made without canon, for tests
// that hold canon's digests to the sum of the canonical bytes.
func sha(b []byte) string {
	return digest.Prefixed(b)
}

func TestADigestReadsBackFromItsTextForm(t *testing.T) {
	t.Parallel()
	d, err := canon.DigestOf(canon.Object{})
	if err != nil {
		t.Fatalf("DigestOf: %v", err)
	}
	const want = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if d.String() != want {
		t.Fatalf("String = %s, want %s", d, want)
	}
	got, err := canon.ParseDigest(want)
	if err != nil {
		t.Fatalf("ParseDigest: %v", err)
	}
	if got != d {
		t.Errorf("ParseDigest(String) = %s, want %s", got, d)
	}
	if got.IsZero() {
		t.Error("a parsed digest is zero")
	}
}

func TestTheZeroDigestIsNoDigest(t *testing.T) {
	t.Parallel()
	var d canon.Digest
	if !d.IsZero() || d.String() != "" {
		t.Errorf("zero Digest: IsZero %v, String %q; want true and empty", d.IsZero(), d.String())
	}
	empty, err := canon.DigestOf(canon.String(""))
	if err != nil {
		t.Fatalf("DigestOf: %v", err)
	}
	if empty.IsZero() || empty == d {
		t.Error("the digest of the empty string is the zero Digest; it is a digest")
	}
}

func TestADigestIsOfTheCanonicalBytesAndOfNoOtherLayout(t *testing.T) {
	t.Parallel()
	v := canon.Object{"a": canon.Array{canon.Int(1), canon.String("x")}}
	got, err := canon.DigestOf(v)
	if err != nil {
		t.Fatalf("DigestOf: %v", err)
	}
	for _, layout := range layouts() {
		b, err := canon.Encode(v, layout)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		same := digest.Prefixed(b) == got.String()
		if (layout == canon.Canonical) != same {
			t.Errorf("layout %d: digest of its bytes equals DigestOf = %v, want %v", layout, same, layout == canon.Canonical)
		}
	}
}

func TestParseDigestRefusesEveryOtherForm(t *testing.T) {
	t.Parallel()
	hex := strings.Repeat("ab", 32)
	tests := []struct {
		name   string
		in     string
		offset int
	}{
		{"nothing", "", 0},
		{"hex alone", hex, 0},
		{"another algorithm", "sha512:" + hex, 0},
		{"the prefix in capitals", "SHA256:" + hex, 0},
		{"upper-case hex", "sha256:" + strings.ToUpper(hex), 7},
		{"a space after", "sha256:" + hex + " ", 71},
		{"a space before", " sha256:" + hex, 0},
		{"one digit short", "sha256:" + hex[1:], 70},
		{"one digit over", "sha256:" + hex + "a", 71},
		{"the prefix alone", "sha256:", 7},
		{"a letter past f", "sha256:" + hex[:10] + "g" + hex[11:], 17},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := canon.ParseDigest(tc.in)
			if !d.IsZero() {
				t.Errorf("ParseDigest returned %s with its refusal", d)
			}
			ce := refusal(t, err, canon.BadDigest)
			if ce.Offset != tc.offset {
				t.Errorf("offset = %d, want %d", ce.Offset, tc.offset)
			}
		})
	}
}

func TestSortedKeysAreInUTF16Order(t *testing.T) {
	t.Parallel()
	emoji, dalet := string(rune(0x1f600)), string(rune(0xfb33))
	m := map[string]int{"b": 2, "a": 1, dalet: 6, "é": 4, "B": 3, "-": 0, emoji: 5}
	got := canon.SortedKeys(m)
	want := []string{"-", "B", "a", "b", "é", emoji, dalet}
	if !slices.Equal(got, want) {
		t.Errorf("SortedKeys = %q, want %q", got, want)
	}
	if got := canon.SortedKeys(map[string]int(nil)); got == nil || len(got) != 0 {
		t.Errorf("SortedKeys(nil) = %#v, want an empty slice", got)
	}
}

func TestCompareOrdersByUTF16CodeUnits(t *testing.T) {
	t.Parallel()
	emoji, dalet, private := string(rune(0x1f600)), string(rune(0xfb33)), string(rune(0xe000))
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"the empty string first", "", "a", -1},
		{"a shorter start first", "a", "aa", -1},
		{"by the first unit that differs", "ab", "aa", 1},
		{"one string is itself", "é" + emoji, "é" + emoji, 0},
		{"below U+10000 by code point", "ö", "€", -1},
		{"above U+FFFF before U+FB33, as its first unit is a surrogate", emoji, dalet, -1},
		{"above U+FFFF before U+E000", private, emoji, 1},
		{"above U+FFFF after U+D7FF", string(rune(0xd7ff)), emoji, -1},
		{"two above U+FFFF by code point", string(rune(0x1f601)), emoji, 1},
		{"two above U+FFFF with other first units", string(rune(0x10000)), string(rune(0x10400)), -1},
		{"bytes that are not UTF-8 are never equal to other bytes", "\xff", "\xfe", 1},
		{"bad bytes and the replacement character are two strings", "\xff", "�", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := canon.Compare(tc.a, tc.b); got != tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := canon.Compare(tc.b, tc.a); got != -tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.b, tc.a, got, -tc.want)
			}
		})
	}
}

type rule struct{ id, note string }

func TestSortByIDSortsByIDInUTF16Order(t *testing.T) {
	t.Parallel()
	s := []rule{{"go.style.2", "b"}, {"arch.x.1", "a"}, {"go.style.10", "c"}}
	if err := canon.SortByID(s, func(r rule) string { return r.id }); err != nil {
		t.Fatalf("SortByID: %v", err)
	}
	got := make([]string, 0, len(s))
	for _, r := range s {
		got = append(got, r.id)
	}
	if want := []string{"arch.x.1", "go.style.10", "go.style.2"}; !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}

func TestSortByIDRefusesAnIDTwice(t *testing.T) {
	t.Parallel()
	s := []rule{{"b", "1"}, {"a", "2"}, {"b", "3"}}
	err := canon.SortByID(s, func(r rule) string { return r.id })
	ce := refusal(t, err, canon.DuplicateID)
	if ce.Detail != `"b" twice` {
		t.Errorf("detail = %q, want the id named", ce.Detail)
	}
	if s[0].id != "a" {
		t.Errorf("s is not sorted after the refusal: %v", s)
	}
}
