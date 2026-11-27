package pluginwire

import (
	"maps"
	"os"
	"path"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// schemaDir is the contract's schemas, read here as files.
const schemaDir = "../../../../contract/plugin/v1"

func schemas(t *testing.T) map[string][]byte {
	t.Helper()
	in := map[string][]byte{}
	for _, f := range Files() {
		b, err := os.ReadFile(path.Join(schemaDir, f))
		if err != nil {
			t.Fatal(err)
		}
		in[f] = b
	}
	return in
}

// TestTheGeneratorRefusesWhatItDoesNotRead plants a change to the schemas in
// each row and holds that the generator stops with the words the row names:
// a schema keyword nobody taught it must fail the run and never be skipped,
// or the types would hold less than the schema says.
//
//   - a keyword it does not read, at the top and inside a member
//   - a member it has no name for (an inline type with no row in names)
//   - a $ref to a def that is not there
//   - a pattern Go does not read
//   - a file it is not given
func TestTheGeneratorRefusesWhatItDoesNotRead(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name, file, from, to, want string
	}{
		{"an unread keyword", "manifest.schema.json", `"type": "object",`, `"type": "object", "propertyNames": {},`, "propertyNames"},
		{"an unread keyword in a member", "wire.schema.json", `"type": "array"`, `"type": "array", "contains": {}`, "contains"},
		{"a ref to nothing", "wire.schema.json", `"#/$defs/key"`, `"#/$defs/nokey"`, "nokey"},
		{"a pattern Go does not read", "manifest.schema.json", `"^[a-z0-9]+(-[a-z0-9]+)*$"`, `"^(?=a)$"`, "pattern"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			in := schemas(t)
			text := string(in[r.file])
			if !strings.Contains(text, r.from) {
				t.Fatalf("%s no longer has %q; the row is stale", r.file, r.from)
			}
			in[r.file] = []byte(strings.Replace(text, r.from, r.to, 1))
			_, err := Generate(in)
			if err == nil || !strings.Contains(err.Error(), r.want) {
				t.Fatalf("got %v, want a refusal that says %q", err, r.want)
			}
		})
	}
	t.Run("a file it is not given", func(t *testing.T) {
		t.Parallel()
		in := schemas(t)
		delete(in, "manifest.schema.json")
		if _, err := Generate(in); err == nil {
			t.Fatal("generated with no manifest schema")
		}
	})
}

// TestTheOutputIsTheSameEachTime plants two runs over the same schemas and
// holds them byte for byte, and holds each file to the marker and gofmt's
// form (gragas-generate's check reads the first line).
func TestTheOutputIsTheSameEachTime(t *testing.T) {
	t.Parallel()
	a, err := Generate(schemas(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(schemas(t))
	if err != nil {
		t.Fatal(err)
	}
	if !maps.EqualFunc(a, b, func(x, y []byte) bool { return string(x) == string(y) }) {
		t.Error("two runs over the same schemas differ")
	}
	for name, data := range a {
		if !strings.HasPrefix(name, GoDir) || !strings.HasSuffix(name, ".gen.go") {
			t.Errorf("%s is outside %s", name, GoDir)
		}
		if !strings.HasPrefix(string(data), "// Written by gragas-generate") {
			t.Errorf("%s has no marker in its first line", name)
		}
	}
	if len(a) != 3 {
		t.Errorf("%d files, want wire, manifest and codec", len(a))
	}
}

// TestABranchTagInEitherSpellingMakesTheSameCode plants the tags of the two
// step kinds as one-value enums in place of consts and holds the output byte
// for byte: a union is told by its tag however the schema spells it, and the
// spelling that was left unread would make the types read a step by trying
// each branch. Red first: with the enum read as a plain string, the output
// differs.
func TestABranchTagInEitherSpellingMakesTheSameCode(t *testing.T) {
	t.Parallel()
	want, err := Generate(schemas(t))
	if err != nil {
		t.Fatal(err)
	}
	in := schemas(t)
	text := string(in["wire.schema.json"])
	for _, kind := range []string{"run", "read"} {
		from := `"kind":    {"const": "` + kind + `"}`
		if kind == "read" {
			from = `"kind": {"const": "read"}`
		}
		if !strings.Contains(text, from) {
			t.Fatalf("wire.schema.json no longer has %q; the row is stale", from)
		}
		text = strings.Replace(text, from, `"kind": {"enum": ["`+kind+`"]}`, 1)
	}
	in["wire.schema.json"] = []byte(text)
	got, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	for name := range want {
		if string(got[name]) != string(want[name]) {
			t.Errorf("%s differs when the tags are one-value enums", name)
		}
	}
}
