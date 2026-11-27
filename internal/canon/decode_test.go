package canon_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

func TestDecodeReadsJSONAnyoneWrote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want canon.Value
	}{
		{"white space anywhere JSON allows", " \t\r\n{ \"b\" : [ 1 , 2 ] ,\n\"a\":null } \n", canon.Object{"a": canon.Null{}, "b": canon.Array{canon.Int(1), canon.Int(2)}}},
		{"minus zero is the whole number 0", "-0", canon.Int(0)},
		{"a whole number with an exponent is an Int", "1E2", canon.Int(100)},
		{"a whole number with a point is an Int", "[-0.0,1.0]", canon.Array{canon.Int(0), canon.Int(1)}},
		{"a number that is not whole is a Float", "0.5", canon.Float(0.5)},
		{"2^53 - 1 at both ends", "[9007199254740991,-9007199254740991]", canon.Array{canon.Int(canon.MaxSafe), canon.Int(-canon.MaxSafe)}},
		{"the smallest binary64 is no underflow", "5e-324", canon.Float(5e-324)},
		{"a number nearer the smallest binary64 than 0 reads as it", "3e-324", canon.Float(5e-324)},
		{"zero written small is zero, not an underflow", "[0e-400,0.000e5]", canon.Array{canon.Int(0), canon.Int(0)}},
		{"every short escape", `"\" \\ \/ \b \f \n \r \t"`, canon.String("\" \\ / \b \f \n \r \t")},
		{"a \\u escape in either case", u(`"U+00e9U+00E9"`), canon.String("\u00e9\u00e9")},
		{"a surrogate pair", u(`"U+d83dU+de00"`), canon.String("\U0001f600")},
		{"raw UTF-8", "\"é\U0001f600 \"", canon.String("é\U0001f600 ")},
		{"an escaped NUL", `"\u0000"`, canon.String("\x00")},
		{"the literals", "[true,false,null]", canon.Array{canon.Bool(true), canon.Bool(false), canon.Null{}}},
		{"empty containers are not nil", `{"a":[],"o":{}}`, canon.Object{"a": canon.Array{}, "o": canon.Object{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := canon.Decode([]byte(tc.in))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Decode = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestDecodeRefusesWhatTwoReadersCouldReadTwoWays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     string
		kind   canon.Kind
		offset int
	}{
		{"nothing", "", canon.BadSyntax, 0},
		{"only white space", " \n", canon.BadSyntax, 2},
		{"a byte order mark", "\xef\xbb\xbf{}", canon.BadSyntax, 0},
		{"a second value", "{} {}", canon.BadSyntax, 3},
		{"a leading zero", "01", canon.BadSyntax, 1},
		{"a plus", "+1", canon.BadSyntax, 0},
		{"a minus alone", "-", canon.BadSyntax, 1},
		{"a point with no digits", "1.", canon.BadSyntax, 2},
		{"a point first", ".5", canon.BadSyntax, 0},
		{"an exponent with no digits", "1e+", canon.BadSyntax, 3},
		{"NaN", "NaN", canon.BadSyntax, 0},
		{"Infinity", "-Infinity", canon.BadSyntax, 1},
		{"a trailing comma in an array", "[1,]", canon.BadSyntax, 3},
		{"a trailing comma in an object", `{"a":1,}`, canon.BadSyntax, 7},
		{"a key with no value", `{"a"}`, canon.BadSyntax, 4},
		{"a key that is not a string", `{a:1}`, canon.BadSyntax, 1},
		{"a missing comma", "[1 2]", canon.BadSyntax, 3},
		{"an unclosed array", "[1", canon.BadSyntax, 2},
		{"a word cut short", "tru", canon.BadSyntax, 0},
		{"single quotes", "'a'", canon.BadSyntax, 0},
		{"an unterminated string", `"abc`, canon.BadSyntax, 4},
		{"a raw control in a string", "\"a\tb\"", canon.BadSyntax, 2},
		{"an escape JSON does not have", `"\x41"`, canon.BadSyntax, 1},
		{"a \\u escape cut short", `"\u12"`, canon.BadSyntax, 1},
		{"a \\u escape that is not hex", `"\u12G4"`, canon.BadSyntax, 1},
		{"a raw byte that is not UTF-8", "\"a\xffb\"", canon.BadUTF8, 2},
		{"an overlong encoding", "\"\xc0\x80\"", canon.BadUTF8, 1},
		{"a surrogate written as UTF-8", "\"\xed\xa0\x80\"", canon.BadUTF8, 1},
		{"a first half alone at the end", `"\ud800"`, canon.BadUTF8, 1},
		{"a first half before a letter", `"\ud800A"`, canon.BadUTF8, 1},
		{"a first half before another first half", `"x\ud800\ud800"`, canon.BadUTF8, 2},
		{"a second half alone", `"\udc00"`, canon.BadUTF8, 1},
		{"a key twice", `{"a":1,"a":2}`, canon.DuplicateKey, 7},
		{"a key twice, once escaped", u(`{"a":1,"U+0061":2}`), canon.DuplicateKey, 7},
		{"a key twice, deep inside", `[{"x":{"k":1,"j":2,"k":3}}]`, canon.DuplicateKey, 19},
		{"a whole number one past 2^53 - 1", "9007199254740992", canon.TooLarge, 0},
		{"a whole number one before -(2^53 - 1)", "[-9007199254740992]", canon.TooLarge, 1},
		{"a whole number past int64", "9223372036854775808", canon.TooLarge, 0},
		{"a whole number past 2^53 written with an exponent", "1e16", canon.TooLarge, 0},
		{"a fraction that rounds past 2^53 - 1", "9007199254740991.9", canon.TooLarge, 0},
		{"a number too large for a binary64", "1e400", canon.TooLarge, 0},
		{"a number too large for a binary64, negative", "-1.8e308", canon.TooLarge, 0},
		{"a number too small for a binary64", "1e-400", canon.Underflow, 0},
		{"a number too small, negative, in an object", `{"a":-2e-324}`, canon.Underflow, 5},
		{"a number too small with digits after its point", "0.0001e-400", canon.Underflow, 0},
		{"arrays one level past MaxDepth", strings.Repeat("[", canon.MaxDepth+1) + strings.Repeat("]", canon.MaxDepth+1), canon.TooDeep, canon.MaxDepth},
		{"objects one level past MaxDepth", strings.Repeat(`{"a":`, canon.MaxDepth) + "{}" + strings.Repeat("}", canon.MaxDepth), canon.TooDeep, 5 * canon.MaxDepth},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v, err := canon.Decode([]byte(tc.in))
			if v != nil {
				t.Errorf("Decode returned %#v with its refusal", v)
			}
			ce := refusal(t, err, tc.kind)
			if ce.Offset != tc.offset {
				t.Errorf("offset = %d, want %d (%v)", ce.Offset, tc.offset, err)
			}
			if ce.Path != "" {
				t.Errorf("path = %q, want none from Decode", ce.Path)
			}
		})
	}
}

func TestDecodeTakesAValueAtMaxDepth(t *testing.T) {
	t.Parallel()
	in := strings.Repeat("[", canon.MaxDepth) + strings.Repeat("]", canon.MaxDepth)
	if _, err := canon.Decode([]byte(in)); err != nil {
		t.Fatalf("Decode at MaxDepth: %v", err)
	}
	obj := strings.Repeat(`{"a":`, canon.MaxDepth-1) + "{}" + strings.Repeat("}", canon.MaxDepth-1)
	if _, err := canon.Decode([]byte(obj)); err != nil {
		t.Fatalf("Decode objects at MaxDepth: %v", err)
	}
}

func TestADecodedNumberEncodesInTheFormOfRFC8785(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"a whole number", "1", "1"},
		{"a point on a whole number goes", "1.0", "1"},
		{"an exponent on a whole number goes", "1e0", "1"},
		{"a capital exponent", "100E-2", "1"},
		{"minus zero loses its sign", "-0", "0"},
		{"a float minus zero loses its sign", "-0.0", "0"},
		{"a trailing zero goes", "4.50", "4.5"},
		{"a small number is written with a point", "2e-3", "0.002"},
		{"one below 1e-6 is written with an exponent", "0.0000001", "1e-7"},
		{"an exponent has no plus and no leading zero", "123e-09", "1.23e-7"},
		{"a float keeps the digits a binary64 holds", "12345678901234567890e-10", "1234567890.1234567"},
		{"the first number of RFC 8785's sample", "333333333.33333329", "333333333.3333333"},
		{"the last number of RFC 8785's sample", "0.000000000000000000000000001", "1e-27"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v, err := canon.Decode([]byte(tc.in))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			got, err := canon.Encode(v, canon.Canonical)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Encode(Decode(%s)) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// u is s with each "U+" made a backslash and a u, so a test input holds JSON
// \u escapes written plainly: u(`"U+0061"`) is the JSON string "a" escaped.
func u(s string) string { return strings.ReplaceAll(s, "U+", `\u`) }
