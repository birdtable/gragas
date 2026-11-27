package git

import (
	"strings"
	"testing"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
)

// TestTheReaderReadsWhatGitPrints: the reader's answers for what git config
// -z prints, literal for literal. Planted failure: the first value of a key
// set twice kept.
func TestTheReaderReadsWhatGitPrints(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, out, missing string
		want               model.Identity
	}{
		{"both", "user.name\nAda Example\x00user.email\nada@example.com\x00", "", model.Identity{Name: "Ada Example", Email: "ada@example.com"}},
		{"the last of a key set twice", "user.name\nAda\x00user.email\na@example.com\x00user.name\nGrace\x00", "", model.Identity{Name: "Grace", Email: "a@example.com"}},
		{"nothing", "", "user.name and user.email", model.Identity{}},
		{"a name with no value", "user.name\x00user.email\na@example.com\x00", "user.name", model.Identity{}},
		{"a name of spaces", "user.name\n   \x00user.email\na@example.com\x00", "user.name", model.Identity{}},
		{"a newline in a value", "user.name\nAda\nExample\x00user.email\na@example.com\x00", "", model.Identity{Name: "AdaExample", Email: "a@example.com"}},
	}
	for _, c := range cases {
		got, missing, err := parseIdentity([]byte(c.out))
		if err != nil || got != c.want || missing != c.missing {
			t.Errorf("%s: %+v, missing %q, %v; want %+v, missing %q", c.name, got, missing, err, c.want, c.missing)
		}
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, bad := range []string{"user.name\nAda", "core.editor\nvi\x00", "user.name\n\xff\x00"} {
		if _, _, err := parseIdentity([]byte(bad)); err == nil {
			t.Errorf("%q read with no error", bad)
		}
	}
}

// FuzzParseIdentity (tests.generated.1): whatever git prints, the reader
// does not panic, and an identity it answers is two non-empty lines with no
// control character.
func FuzzParseIdentity(f *testing.F) {
	for _, seed := range []string{
		"user.name\nAda Example\x00user.email\nada@example.com\x00",
		"user.name\nAda\x00user.email\na@example.com\x00user.name\nGrace\x00",
		"",
		"user.name\x00",
		"user.name\n\x1b[31mAda\x1b[0m\x00user.email\na@\x1b]0;t\x07example.com\x00",
		"user.name\nAda\r\nuser.email\nx\x00user.email\ny\x00",
		"core.editor\nvi\x00",
		"user.name\nAda",
		"\x00\x00\x00",
		"user.name\n\xff\xfe\x00",
		"user.name\n\u202eadA\x00user.email\na@example.com\x00",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		id, missing, err := parseIdentity(out)
		if err != nil || missing != "" {
			if id != (model.Identity{}) {
				t.Fatalf("an identity %+v beside missing %q or %v", id, missing, err)
			}
			return
		}
		for _, v := range []string{id.Name, id.Email} {
			if v == "" || v != strings.TrimSpace(v) || strings.ContainsFunc(v, unicode.IsControl) {
				t.Fatalf("an identity half %q is empty, padded or holds a control character", v)
			}
		}
	})
}
