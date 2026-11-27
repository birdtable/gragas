package plugin_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// A plugin's text with ESC in it is printed with the escape, never the
// character: the sequence that clears the screen is shown as the text it is.
func TestShownWritesESCAsItsEscape(t *testing.T) {
	t.Parallel()
	got := plugin.Shown("no-unused\x1b[2J\x1b[Hall clear")
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("ESC is printed as itself: %q", got)
	}
	if want := `no-unused\x1b[2J\x1b[Hall clear`; got != want {
		t.Fatalf("Shown gave %q, want %q", got, want)
	}
}

func TestShownEscapesWhatAScreenDoesNotShowAsWritten(t *testing.T) {
	t.Parallel()
	for name, row := range map[string]struct{ in, want string }{
		"plain":                    {"an unused variable", "an unused variable"},
		"a newline":                {"line one\nline two", `line one\nline two`},
		"a carriage return":        {"good\rbad ", `good\rbad `},
		"a tab":                    {"a \tb", `a \tb`},
		"a NUL":                    {"a \x00b", `a \x00b`},
		"DEL":                      {"a \x7fb", `a \x7fb`},
		"a backspace":              {"ok \b\bno", `ok \b\bno`},
		"the one-character CSI":    {"a \u009b2Jb", `a \u009b2Jb`},
		"a C1 next line":           {"a \u0085b", `a \u0085b`},
		"a right-to-left override": {"it is safe\u202eexe.txt", `it is safe\u202eexe.txt`},
		"an isolate":               {"a \u2066b\u2069", `a \u2066b\u2069`},
		"a zero-width space":       {"a \u200bb", `a \u200bb`},
		"a byte order mark":        {"\ufeffkey", `\ufeffkey`},
		"a soft hyphen":            {"a \u00adb", `a \u00adb`},
		"a line separator":         {"a \u2028b", `a \u2028b`},
		"a paragraph separator":    {"a \u2029b", `a \u2029b`},
		"a grapheme joiner":        {"a \u034fb", `a \u034fb`},
		"a tag character":          {"a \U000e0041b", `a \U000e0041b`},
		"a variation selector":     {"a \ufe0fb", `a \ufe0fb`},
		// A backslash is two, so a text that spells an escape is not
		// printed like the character.
		"a spelled escape":     {`a \x1b[2Jb`, `a \\x1b[2Jb`},
		"a spelled newline":    {`a \n b`, `a \\n b`},
		"backslashes in words": {`one\two three\four`, `one\\two three\\four`},
		"a backslash and ESC":  {"\\\x1b", `\\\x1b`},
		// Bytes that are not UTF-8, each on its own.
		"a lone continuation byte": {"a \x80b", `a \x80b`},
		"the CSI byte of Latin-1":  {"a \x9b2Jb", `a \x9b2Jb`},
		"a cut-off character":      {"un caf\xc3", `un caf\xc3`},
		"an overlong slash":        {"a \xc0\xafb", `a \xc0\xafb`},
		"a surrogate half":         {"a \xed\xa0\x80b", `a \xed\xa0\x80b`},
		"0xff":                     {"\xff", `\xff`},
		// The replacement character someone really sent is a character.
		"a real U+FFFD": {"a\ufffdb", "a\ufffdb"},
		// Letters of every script are left alone.
		"Greek":      {"μεταβλητή", "μεταβλητή"},
		"Japanese":   {"未使用の変数", "未使用の変数"},
		"Arabic":     {"متغير غير مستخدم", "متغير غير مستخدم"},
		"an accent":  {"cafe\u0301", "cafe\u0301"},
		"an emoji":   {"done 🎉", "done 🎉"},
		"the empty":  {"", ""},
		"a quote":    {`say "hi" to 'them'`, `say "hi" to 'them'`},
		"wide space": {"a\u3000b", "a\u3000b"},
	} {
		if got := plugin.Shown(row.in); got != row.want {
			t.Errorf("%s: Shown(%q) is %q, want %q", name, row.in, got, row.want)
		}
	}
}

// What Shown writes holds no character the manifest schema's "shown" class
// refuses (control, format, line and paragraph separator), none the terminal
// would escape again, and is UTF-8, for every character there is and every
// byte.
func TestShownLeavesNothingAScreenDoesNotShow(t *testing.T) {
	t.Parallel()
	check := func(in string) {
		t.Helper()
		got := plugin.Shown(in)
		if !utf8.ValidString(got) {
			t.Fatalf("Shown(%q) is not UTF-8: %q", in, got)
		}
		for _, r := range got {
			if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp) || model.Unshowable(r) {
				t.Fatalf("Shown(%q) holds %U", in, r)
			}
		}
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		check("a" + string(r) + "b")
	}
	for b := range 256 {
		check("a" + string([]byte{byte(b)}) + "b")
		check(string([]byte{byte(b)}))
	}
}

// Two texts that differ are printed differently: the escape can be read back
// to the bytes that were sent, so no text is printed as another's.
func TestShownPrintsTwoTextsDifferently(t *testing.T) {
	t.Parallel()
	seen := map[string]string{}
	add := func(in string) {
		t.Helper()
		got := plugin.Shown(in)
		if other, ok := seen[got]; ok && other != in {
			t.Fatalf("%q and %q are both printed as %q", other, in, got)
		}
		seen[got] = in
	}
	parts := []string{"a", `\`, "\x1b", `x1b`, "\n", "n", "\x9b", "\u009b", `x9b`, `u009b`, "\u200b", `u200b`, "\xc3", "\ufffd", `\\`, "x", "\x00", "00"}
	for _, p := range parts {
		for _, q := range parts {
			for _, r := range parts {
				add(p + q + r)
			}
		}
	}
}

// A plugin's error is printed with its own text escaped, once: an ESC, a C1
// CSI and a bidi override in its code, path or message reach no terminal,
// and the fields stay as the plugin sent them for a screen that lays them out
// itself.
func TestACallErrorPrintsThePluginsTextShown(t *testing.T) {
	t.Parallel()
	const paint = "\x1b]0;owned\x07\x1b[2J\u009b31m\u202efdp.txt"
	for _, e := range []*plugin.CallError{
		{Code: plugin.CodeRefused, Message: "no" + paint, Path: "/config" + paint},
		{Code: plugin.CodeBug, Message: "no" + paint},
		{Code: plugin.Code("bug" + paint), Message: "its code"},
		{Code: plugin.Code("bug" + paint), Message: "its code", Path: "/config/0"},
	} {
		got := e.Error()
		for _, r := range []rune{0x1b, 0x07, 0x9b, 0x202e} {
			if strings.ContainsRune(got, r) {
				t.Fatalf("the error prints %U raw: %q", r, got)
			}
		}
		if !strings.Contains(got, plugin.Shown(e.Message)) || !strings.Contains(got, plugin.Shown(string(e.Code))) || !strings.Contains(got, plugin.Shown(e.Path)) {
			t.Fatalf("the error does not say its fields shown: %q", got)
		}
	}
	e := &plugin.CallError{Code: plugin.CodeRefused, Message: `a \x1b`, Path: "/config/0"}
	if got, want := e.Error(), `the plugin answered refused at /config/0: a \\x1b`; got != want {
		t.Fatalf("the error is %q, want %q: the plugin's backslash is doubled once", got, want)
	}
}

// An answer refused at a member the plugin named is printed with its place
// and its why escaped, once: the place holds the plugin's own key,
// and the why can carry an error's words about the answer's
// text, so an ESC, a C1 CSI and a bidi override in either reach no terminal,
// and the fields stay as they were made for a screen that lays them out.
func TestAnAnswerErrorPrintsThePluginsKeyShown(t *testing.T) {
	t.Parallel()
	const paint = "\x1b]0;owned\x07\x1b[2J\u009b31m\u202efdp.txt"
	for _, e := range []*plugin.AnswerError{
		{Path: "/ok/" + paint, Why: "is not a member of this message"},
		{Path: "/ok/words/0", Why: "has no canonical form: " + paint},
		{Why: "is not JSON: " + paint},
	} {
		got := e.Error()
		for _, r := range []rune{0x1b, 0x07, 0x9b, 0x202e} {
			if strings.ContainsRune(got, r) {
				t.Fatalf("the error prints %U raw: %q", r, got)
			}
		}
		if !strings.Contains(got, plugin.Shown(e.Path)) || !strings.Contains(got, plugin.Shown(e.Why)) {
			t.Fatalf("the error does not say its fields shown: %q", got)
		}
	}
	e := &plugin.AnswerError{Path: "/ok/a b", Why: `names "a \x1b"`}
	if got, want := e.Error(), `the plugin's answer is not one at /ok/a b: names "a \\x1b"`; got != want {
		t.Fatalf("the error is %q, want %q: a backslash is doubled once", got, want)
	}
	if got, want := (&plugin.AnswerError{Why: "is not UTF-8"}).Error(), "the plugin's answer is not one: is not UTF-8"; got != want {
		t.Fatalf("the error of the whole answer is %q, want %q", got, want)
	}
}

// A message the generated codec refuses at a member the plugin named prints
// that place escaped, once: the place holds the plugin's own
// key, so the Fault a FromCanon returns is safe to print as it is, and its
// At stays the key as sent for a caller that lays the place out itself.
func TestAFaultPrintsThePluginsKeyShown(t *testing.T) {
	t.Parallel()
	const paint = "\x1b]0;owned\x07\x1b[2J\u009b31m\u202efdp"
	key, err := json.Marshal(paint)
	if err != nil {
		t.Fatal(err)
	}
	v, err := canon.Decode([]byte(`{"program":"p","argv":["a"],` + string(key) + `:1}`))
	if err != nil {
		t.Fatal(err)
	}
	var run plugin.Run
	f, isFault := errors.AsType[*plugin.Fault](run.FromCanon(v))
	if !isFault {
		t.Fatalf("a run with a member no message names reads, or is refused without a Fault")
	}
	if f.At != "/"+paint {
		t.Fatalf("refused at %q (%s), want the planted key as sent", f.At, f.Msg)
	}
	got := f.Error()
	for _, r := range []rune{0x1b, 0x07, 0x9b, 0x202e} {
		if strings.ContainsRune(got, r) {
			t.Fatalf("the fault prints %U raw: %q", r, got)
		}
	}
	if want := plugin.Shown(f.At) + " " + f.Msg; got != want {
		t.Fatalf("the fault is %q, want %q", got, want)
	}
	if got, want := (&plugin.Fault{At: `/a\b`, Msg: "m"}).Error(), `/a\\b m`; got != want {
		t.Fatalf("the fault is %q, want %q: a backslash is doubled once", got, want)
	}
	if got, want := (&plugin.Fault{Msg: "is not UTF-8"}).Error(), "the message is not UTF-8"; got != want {
		t.Fatalf("the fault of the whole message is %q, want %q", got, want)
	}
}
