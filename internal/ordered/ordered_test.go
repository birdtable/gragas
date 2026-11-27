package ordered_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/ordered"
)

func TestResolveYAML11(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want any
	}{
		{"v: plain", "plain"},
		{"v: yes", true},
		{"v: Off", false},
		{"v: ~", nil},
		{"v:", nil},
		{"v: 010", int64(8)},
		{"v: 0x1f", int64(31)},
		{"v: 1_000", int64(1000)},
		{"v: 1:30", int64(90)},
		{"v: 1.5", 1.5},
		{"v: 1e5", "1e5"},
		{"v: 0.24.0", "0.24.0"},
		{`v: "yes"`, "yes"},
		{"v: '010'", "010"},
		{"v: !!str 12", "12"},
		{"v: 2026-01-02", ordered.Timestamp("2026-01-02")},
		{"v: |\n  two\n  lines\n", "two\nlines\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			v, err := ordered.ParseYAML([]byte(tc.in))
			if err != nil {
				t.Fatalf("ParseYAML: %v", err)
			}
			m, ok := v.(*ordered.Map)
			if !ok {
				t.Fatalf("got %T, want a map", v)
			}
			got, _ := m.Get("v")
			if got != tc.want {
				t.Errorf("v = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseYAMLRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{"boolean key", "on: 2026-01-02\n"},
		{"merge key", "a: &x {b: 1}\nc:\n  <<: *x\n"},
		{"two documents", "a: 1\n---\nb: 2\n"},
		{"bad indent", "a:\n  - b\n c: d\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ordered.ParseYAML([]byte(tc.in))
			if !errors.Is(err, ordered.ErrSyntax) {
				t.Errorf("ParseYAML error = %v, want ErrSyntax", err)
			}
		})
	}
}

// TestParseTextKeysReadsAKeyAsGitHubReadsAWorkflows: a plain key is its
// text, so a workflow's on: is read, where ParseYAML refuses it as true; a
// value is read as ParseYAML reads it, a tagged key is still refused, and an
// alias bomb is refused within the same budget.
func TestParseTextKeysReadsAKeyAsGitHubReadsAWorkflows(t *testing.T) {
	t.Parallel()
	v, err := ordered.ParseTextKeys([]byte("on:\n  push:\nyes: on\n10: x\nnull: 1\n"))
	if err != nil {
		t.Fatalf("ParseTextKeys: %v", err)
	}
	if got, want := ordered.JSON(v, -1), `{"on": {"push": null}, "yes": true, "10": "x", "null": 1}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
	for name, doc := range map[string]string{
		"a tagged key":  "!!bool on: x\n",
		"a merge key":   "a: &x {b: 1}\nc:\n  <<: *x\n",
		"an alias bomb": laughs(9),
	} {
		if _, err := ordered.ParseTextKeys([]byte(doc)); !errors.Is(err, ordered.ErrSyntax) {
			t.Errorf("%s: err %v, want ErrSyntax", name, err)
		}
	}
}

func TestKeyOrderAndLines(t *testing.T) {
	t.Parallel()
	v, err := ordered.ParseYAML([]byte("b: 1\na: 2\nlist:\n  - x: 1\n    y: 2\n  - z: 3\nb: 9\n"))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		t.Fatalf("got %T, want a map", v)
	}
	if got, want := ordered.JSON(m, -1), `{"b": 9, "a": 2, "list": [{"x": 1, "y": 2}, {"z": 3}]}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
	if got := ordered.LineAt(m, []string{"list", "1", "z"}); got != 6 {
		t.Errorf("LineAt list/1/z = %d, want 6", got)
	}
}

// TestTheLinesRenderWillUse holds the line-position API the package comment
// documents: a mapping's line, a key's line, the last line of a key written
// twice, and line 0 for a key that is not there.
func TestTheLinesRenderWillUse(t *testing.T) {
	t.Parallel()
	v, err := ordered.ParseYAML([]byte("\n\nid: a\nrule:\n  note: x\nid: b\n"))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	m := v.(*ordered.Map)
	if m.Line != 3 {
		t.Errorf("the mapping starts on line %d, want 3", m.Line)
	}
	if got := m.KeyLine("rule"); got != 4 {
		t.Errorf("KeyLine(rule) = %d, want 4", got)
	}
	if got := m.KeyLine("id"); got != 6 {
		t.Errorf("KeyLine(id) = %d, want 6, the line of its last write", got)
	}
	if got := m.KeyLine("absent"); got != 0 {
		t.Errorf("KeyLine(absent) = %d, want 0", got)
	}
	if keys := m.Keys(); len(keys) != 2 || keys[0] != "id" {
		t.Errorf("Keys() = %q, want id first", keys)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	m := ordered.NewMap(0)
	m.Set("s", "é <&> \u2028 \"q\" \\ \n\t\x01", 0)
	m.Set("empty", []any{}, 0)
	m.Set("obj", ordered.NewMap(0), 0)
	m.Set("l", []any{true, nil, int64(-3), 2.5}, 0)
	want := "{\n \"s\": \"é <&> \u2028 \\\"q\\\" \\\\ \\n\\t\\u0001\",\n \"empty\": [],\n \"obj\": {},\n \"l\": [\n  true,\n  null,\n  -3,\n  2.5\n ]\n}"
	if got := ordered.JSON(m, 1); got != want {
		t.Errorf("JSON:\n got  %q\n want %q", got, want)
	}
}

func TestFloatRepr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float64
		want string
	}{
		{1, "1.0"},
		{1.5, "1.5"},
		{0.1, "0.1"},
		{1e6, "1000000.0"},
		{1e15, "1000000000000000.0"},
		{1e16, "1e+16"},
		{1.5e16, "1.5e+16"},
		{0.0001, "0.0001"},
		{0.00001, "1e-05"},
		{-2.25, "-2.25"},
		{1e100, "1e+100"},
		{math.Inf(1), "inf"},
	}
	for _, tc := range tests {
		if got := ordered.FloatRepr(tc.in); got != tc.want {
			t.Errorf("FloatRepr(%v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestRepr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   any
		want string
	}{
		{"plain", "'plain'"},
		{"it's", `"it's"`},
		{`it's "x"`, `'it\'s "x"'`},
		{"a\\b\n", `'a\\b\n'`},
		{"\x1b", `'\x1b'`},
		{[]any{"a", int64(1), nil, true}, "['a', 1, None, True]"},
	}
	for _, tc := range tests {
		if got := ordered.Repr(tc.in); got != tc.want {
			t.Errorf("Repr(%#v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// FuzzParseYAML holds the loader to two promises on any input: it never
// panics, and a document it accepts encodes to valid JSON.
func FuzzParseYAML(f *testing.F) {
	for _, seed := range []string{
		"part: arch\nrules:\n  - id: arch.x.1\n",
		"a: &x [1, 2]\nb: *x\n",
		"on: yes\n",
		"v: 1:2:3.5\n",
		"- - - [\n",
		"a: !!binary aGk=\n",
		"v: [.nan, .inf, -.inf]\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := ordered.ParseYAML(data)
		if err != nil {
			return
		}
		for _, indent := range []int{-1, 1} {
			if out := ordered.JSON(v, indent); !json.Valid([]byte(out)) {
				t.Fatalf("JSON(indent %d) is not valid JSON:\n%s", indent, out)
			}
		}
		if _, err := json.Marshal(ordered.Plain(v)); err != nil {
			t.Fatalf("Plain does not marshal: %v", err)
		}
	})
}

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestAKeyWrittenTwiceIsReportedWhereItRepeats: the map keeps the last value,
// and Repeats says the key, the line before and the line again, so a reader
// that refuses a repeat can name both.
func TestAKeyWrittenTwiceIsReportedWhereItRepeats(t *testing.T) {
	t.Parallel()
	v, err := ordered.ParseYAML([]byte("a: 1\nb:\n  c: 1\n  c: 2\na: 3\na: 4\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := v.(*ordered.Map)
	if got, want := m.Repeats(), []ordered.Repeat{{Key: "a", First: 1, Again: 5}, {Key: "a", First: 5, Again: 6}}; !slices.Equal(got, want) {
		t.Errorf("top: %v, want %v", got, want)
	}
	if got, want := m.Sub("b").Repeats(), []ordered.Repeat{{Key: "c", First: 3, Again: 4}}; !slices.Equal(got, want) {
		t.Errorf("b: %v, want %v", got, want)
	}
	if got, _ := m.Get("a"); got != int64(4) {
		t.Errorf("a = %v, want its last value", got)
	}
	once, err := ordered.ParseYAML([]byte("a: 1\nb: 2\n"))
	if err != nil || len(once.(*ordered.Map).Repeats()) != 0 {
		t.Errorf("each key once: %v, %v; want no repeat", once, err)
	}
}
