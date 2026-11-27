package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// realSchemas is the folder the Assets adapter embeds the contract schemas
// from, read here as files only to seed the fake Assets port with them.
const realSchemas = "../../host/assets/contract/schema/v1"

// golden is the minimal document of kind k, by hand: the fewest fields its
// schema of contract 1.0 takes, in canon's Document layout. The fixture
// rulebook's documents (M1-20) replace them when they land.
func golden(t testing.TB, k Kind) []byte {
	t.Helper()
	b, err := os.ReadFile(path.Join("testdata", string(k)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// schemaFiles is the real contract schemas of v1, by their path in the
// Assets port.
func schemaFiles(t testing.TB) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(realSchemas)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(path.Join(realSchemas, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[SchemaDir+"/v1/"+e.Name()] = string(b)
	}
	if len(out) < len(Kinds()) {
		t.Fatalf("%s holds %d schemas, fewer than the documents", realSchemas, len(out))
	}
	return out
}

// world is a fake whose Schema port compiled every document's schema of v1
// and passes what no Plant names, and whose Assets port holds files.
func world(t testing.TB, files map[string]string) (*porttest.World, Schemas) {
	t.Helper()
	w := porttest.NewWorld(t)
	for _, k := range Kinds() {
		w.SeedSchemas(k.schema(1))
	}
	w.SeedAssets(files)
	p := w.Ports()
	return w, Schemas{Check: p.Schema, Files: p.Assets}
}

// newOf is a pointer to the zero model value of a document of kind k.
func newOf(k Kind) any {
	switch k {
	case Bundle:
		return &mc.BundleDocument{}
	case Report:
		return &mc.ReportDocument{}
	case Changelog:
		return &mc.ChangelogDocument{}
	case SARIF:
		return &mc.SarifDocument{}
	case Plan:
		return &mc.PlanDocument{}
	case Proposal:
		return &mc.ProposalDocument{}
	case Grid:
		return &mc.GridDocument{}
	}
	return nil
}

// TestCurrentIsTheModelsContractVersion: Current answers the zero Version
// rather than panic on a model.ContractVersion it cannot read, so this holds
// the constant to one it can.
func TestCurrentIsTheModelsContractVersion(t *testing.T) {
	t.Parallel()
	if got := Current(); got.String() != model.ContractVersion || got.Major < 1 {
		t.Errorf("Current() = %s, want model.ContractVersion %q", got, model.ContractVersion)
	}
}

// TestEncodeOfEachMinimalDocumentIsItsGoldenBytes: the model value of each
// minimal document encodes to the bytes it was read from, so the encoder
// writes every document of the contract in one layout (A20, tests.generated.5).
// TestTheTwoBumpEnumsAreTheSame: the changelog entry's bump takes the shared
// $defs/bump, and the changelog release's bump (entity-release.schema.json)
// keeps its own inline enum, because a $ref would rename a generated type and
// a minor cannot. Risk: one of the two gains a value the other
// lacks, and a bump reads in one document and fails in the other.
func TestTheTwoBumpEnumsAreTheSame(t *testing.T) {
	t.Parallel()
	read := func(file string) map[string]any {
		b, err := os.ReadFile(path.Join(realSchemas, file))
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	shared, ok := read("common.schema.json")["$defs"].(map[string]any)["bump"].(map[string]any)
	if !ok {
		t.Fatal("common.schema.json has no $defs/bump")
	}
	release, ok := read("entity-release.schema.json")["properties"].(map[string]any)["bump"].(map[string]any)
	if !ok {
		t.Fatal("entity-release.schema.json has no bump")
	}
	if a, b := shared["enum"], release["enum"]; a == nil || !slices.Equal(toStrings(a), toStrings(b)) {
		t.Errorf("$defs/bump is %v and the release's bump is %v, want the same values", a, b)
	}
}

// toStrings is the strings of a decoded JSON array.
func toStrings(v any) []string {
	var out []string
	items, _ := v.([]any)
	for _, i := range items {
		s, _ := i.(string)
		out = append(out, s)
	}
	return out
}

func TestEncodeOfEachMinimalDocumentIsItsGoldenBytes(t *testing.T) {
	t.Parallel()
	for _, k := range Kinds() {
		want := golden(t, k)
		v := newOf(k)
		dec := json.NewDecoder(bytes.NewReader(want))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			t.Fatalf("decode the %s golden: %v", k, err)
		}
		got, err := Encode(k, v)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("Encode(%s) = %v\n%s\nwant\n%s", k, err, got, want)
		}
	}
	// An empty contract field is filled with the contract this engine writes.
	var plan mc.PlanDocument
	if err := json.Unmarshal(golden(t, Plan), &plan); err != nil {
		t.Fatal(err)
	}
	plan.Contract = ""
	if got, err := Encode(Plan, plan); err != nil || !bytes.Equal(got, golden(t, Plan)) {
		t.Errorf("Encode(plan with no contract) = %v\n%s\nwant\n%s", err, got, golden(t, Plan))
	}
}

// TestEncodeRefusesAValueThatIsNotTheDocument: a value of another document,
// one that is no document, an unknown kind and a value naming another
// contract are each refused, never written.
func TestEncodeRefusesAValueThatIsNotTheDocument(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		k    Kind
		v    any
		want string
	}{
		{"another document", Bundle, &mc.PlanDocument{}, "encode a bundle: the value is a plan"},
		{"no document", Plan, map[string]any{"contract": "1.0"}, "the value is not a document of the contract"},
		{"an unknown kind", "memo", &mc.PlanDocument{}, `"memo" is not a document of the contract`},
		{"another contract", Plan, &mc.PlanDocument{Contract: "2.0"}, `it names contract "2.0", and this engine writes contract 1.3`},
		{"a nil pointer", Plan, (*mc.PlanDocument)(nil), "the value is not a JSON object"},
	}
	for _, c := range cases {
		got, err := Encode(c.k, c.v)
		if err == nil || !strings.Contains(err.Error(), c.want) || got != nil {
			t.Errorf("%s: Encode = %q, %v, want an error with %q", c.name, got, err, c.want)
		}
	}
}

// TestValidateNamesBothVersionsOfANewerMajorAndEndsNotReady: a document in a
// contract major this engine does not read is named, not guessed at (S25):
// the error names the version it was written in and the newest this engine
// reads, and ends "not ready".
func TestValidateNamesBothVersionsOfANewerMajorAndEndsNotReady(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	for _, k := range Kinds() {
		for _, found := range []string{"2.0", "10.3"} {
			doc := []byte(`{"contract": "` + found + `", "anything": ["a later major may hold"]}`)
			got, err := Validate(k, doc, s)
			var nr *NotReady
			if !errors.As(err, &nr) {
				t.Fatalf("Validate(%s, %s) = %+v, %v, want not ready", k, found, got, err)
			}
			msg := err.Error()
			if nr.Found.String() != found || nr.Reads != Current() || !strings.Contains(msg, "contract "+found) ||
				!strings.Contains(msg, "up to contract 1.3") || nr.Word() != model.NotReady || nr.Kind != k {
				t.Errorf("Validate(%s, %s): %+v, %q, word %q", k, found, nr, msg, nr.Word())
			}
		}
	}
}

// TestValidateFindsWhatIsWrongWithADocument: each way a document from
// outside is wrong is one finding, and a valid one has none.
func TestValidateFindsWhatIsWrongWithADocument(t *testing.T) {
	t.Parallel()
	w, s := world(t, schemaFiles(t))
	plan := golden(t, Plan)
	planted, err := canon.Decode(plan)
	if err != nil {
		t.Fatal(err)
	}
	body, err := canon.Encode(planted, canon.Document)
	if err != nil {
		t.Fatal(err)
	}
	w.Plant(Plan.schema(1), body, model.Problem{Line: 3, Msg: "schema at /plan: planted"})
	cases := []struct {
		name string
		k    Kind
		doc  []byte
		want string
	}{
		{"a repeated key", Changelog, []byte(`{"contract": "1.0", "contract": "1.0", "releases": [], "rulebook": "x"}`), "not one JSON value every reader reads alike (duplicate"},
		{"not an object", Changelog, []byte(`["contract", "1.0"]`), "the changelog is not a JSON object"},
		{"no contract", Changelog, []byte(`{"releases": [], "rulebook": "x"}`), "it has no contract field"},
		{"a contract that is a number", Changelog, []byte(`{"contract": 1.0}`), "contract field is not a string"},
		{"a contract with a leading zero", Changelog, []byte(`{"contract": "1.01"}`), `"1.01" is no contract version`},
		{"a contract older than the previous major", Changelog, []byte(`{"contract": "0.9"}`), "older than 1.0, the oldest major"},
		{"what the schema says", Plan, plan, "schema at /plan: planted"},
	}
	for _, c := range cases {
		got, err := Validate(c.k, c.doc, s)
		if err != nil || len(got.Findings) != 1 || !strings.Contains(got.Findings[0].Msg, c.want) || got.Findings[0].Entity != string(c.k) {
			t.Errorf("%s: Validate = %+v, %v, want one finding with %q", c.name, got, err, c.want)
		}
	}
	for _, k := range Kinds() {
		if k == Plan {
			continue // the schema was planted to refuse it, above
		}
		if got, err := Validate(k, golden(t, k), s); err != nil || len(got.Findings) != 0 || got.Contract != Current() || got.Kind != k {
			t.Errorf("Validate(minimal %s) = %+v, %v, want valid", k, got, err)
		}
	}
	if _, err := Validate("memo", plan, s); err == nil {
		t.Error("Validate of an unknown kind: no error")
	}
}

// deprecatedFiles is a contract of v1 whose plan schema marks a shape
// deprecated at each place the walk reaches one: a property, an item through
// a reference into another file, and a member of a map through allOf, a
// reference that comes back to itself, and anyOf. The root marks old twice,
// once in its properties and once through allOf, and it is one finding.
func deprecatedFiles() map[string]string {
	return map[string]string{
		SchemaDir + "/v1/document-plan.schema.json": `{
  "allOf": [{"properties": {"old": {"x-deprecated": {"since": "1.0", "removed_in": "2.0"}}}}],
  "properties": {
    "contract": {},
    "old": {"x-deprecated": {"since": "1.0", "removed_in": "2.0"}},
    "list": {"items": {"$ref": "common.schema.json#/$defs/thing"}},
    "map": {"additionalProperties": {"allOf": [{"$ref": "#/$defs/loop"}]}}
  },
  "$defs": {
    "loop": {"$ref": "#/$defs/again"},
    "again": {"anyOf": [{"$ref": "#/$defs/loop"}, {"properties": {"gone": {"x-deprecated": {"since": "1.0", "removed_in": "2.0"}}}}]}
  }
}`,
		SchemaDir + "/v1/common.schema.json": `{"$defs": {"thing": {"properties": {"was": {"$ref": "#/$defs/marked"}}}, "marked": {"x-deprecated": {"since": "1.1", "removed_in": "2.0"}}}}`,
	}
}

// TestADeprecatedShapePastItsWindowIsAFinding: a shape deprecated since 1.x
// is emitted beside the new one for two minors; in a document naming a later
// minor it is a finding, named by its place, and inside its window it is none.
func TestADeprecatedShapePastItsWindowIsAFinding(t *testing.T) {
	t.Parallel()
	_, s := world(t, deprecatedFiles())
	cases := []struct {
		doc  string
		want []string
	}{
		{`{"contract": "1.1", "old": 1}`, nil},
		{`{"contract": "1.2", "old": 1}`, []string{"plan /old"}},
		{`{"contract": "1.2", "list": [{"was": 1}]}`, nil},
		{`{"contract": "1.3", "list": [{"was": 1}, {"was": 2}, {}]}`, []string{"plan /list/0/was", "plan /list/1/was"}},
		{`{"contract": "1.2", "map": {"a/b": {"gone": 1}, "c": {}}}`, []string{"plan /map/a~1b/gone"}},
		{`{"contract": "1.2", "old": 1, "map": {"x": {"gone": 1}}}`, []string{"plan /map/x/gone", "plan /old"}},
	}
	for _, c := range cases {
		got, err := Validate(Plan, []byte(c.doc), s)
		var places []string
		for _, f := range got.Findings {
			places = append(places, f.Entity)
		}
		if err != nil || strings.Join(places, ", ") != strings.Join(c.want, ", ") {
			t.Errorf("Validate(%s) = %+v, %v, want findings at %q", c.doc, got.Findings, err, c.want)
		}
	}
	got, _ := Validate(Plan, []byte(`{"contract": "1.2", "old": 1}`), s)
	if len(got.Findings) == 1 && (!strings.Contains(got.Findings[0].Msg, "deprecated since contract 1.0, and its window of 2 minors ended before 1.2") ||
		!strings.Contains(got.Findings[0].Fix, "contract 2.0 removes this one")) {
		t.Errorf("the finding reads %+v", got.Findings[0])
	}
}

// TestAShapeIsPastItsWindowTwoMinorsOnOrAtTheMajorThatRemovesIt pins the
// window itself.
func TestAShapeIsPastItsWindowTwoMinorsOnOrAtTheMajorThatRemovesIt(t *testing.T) {
	t.Parallel()
	v := func(s string) Version {
		got, err := ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	cases := []struct {
		found, since, gone string
		want               bool
	}{
		{"1.0", "1.0", "2.0", false}, {"1.1", "1.0", "2.0", false}, {"1.2", "1.0", "2.0", true},
		{"1.4", "1.3", "2.0", false}, {"1.5", "1.3", "2.0", true}, {"0.9", "1.0", "2.0", false},
		{"2.0", "1.9", "2.0", true}, {"3.1", "1.9", "2.0", true},
	}
	for _, c := range cases {
		if got := past(v(c.found), v(c.since), v(c.gone)); got != c.want {
			t.Errorf("past(found %s, since %s, removed in %s) = %v, want %v", c.found, c.since, c.gone, got, c.want)
		}
	}
}

// TestTheDeprecationWalkFailsOnASchemaItCannotFollow: a schema the walk
// cannot read all of is an error, never a walk that quietly misses a mark.
func TestTheDeprecationWalkFailsOnASchemaItCannotFollow(t *testing.T) {
	t.Parallel()
	file := SchemaDir + "/v1/document-plan.schema.json"
	cases := []struct {
		name, schema, want string
	}{
		{"a keyword it does not follow", `{"properties": {"x": {"if": {}}}}`, "uses if, which the deprecation walk does not follow"},
		{"a reference out of the folder", `{"properties": {"x": {"$ref": "../v2/common.schema.json"}}}`, `refers to "../v2/common.schema.json", which is not a file beside it`},
		{"a reference to another site", `{"properties": {"x": {"$ref": "https://example.org/a.json"}}}`, "which is not a file beside it"},
		{"a reference to nothing", `{"properties": {"x": {"$ref": "#/$defs/none"}}}`, `refers to "#/$defs/none", which names nothing`},
		{"a reference to a missing file", `{"properties": {"x": {"$ref": "gone.schema.json"}}}`, "read the schema gone.schema.json"},
		{"a mark with no window", `{"properties": {"x": {"x-deprecated": {"since": "1.0"}}}}`, "x-deprecated that is not {since, removed_in}"},
		{"a mark with a bad version", `{"properties": {"x": {"x-deprecated": {"since": "one", "removed_in": "2.0"}}}}`, `"one" is no contract version`},
	}
	for _, c := range cases {
		_, s := world(t, map[string]string{file: c.schema})
		_, err := Validate(Plan, []byte(`{"contract": "1.0", "x": {}}`), s)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Validate = %v, want an error with %q", c.name, err, c.want)
		}
	}
	_, s := world(t, map[string]string{})
	if _, err := Validate(Plan, golden(t, Plan), s); err == nil || !strings.Contains(err.Error(), "read the schema document-plan.schema.json") {
		t.Errorf("Validate with no schema file = %v, want an error naming it", err)
	}
}

// TestReadBundleReadsTheMinimalBundle: a valid bundle comes back as its
// model value.
func TestReadBundleReadsTheMinimalBundle(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	got, err := ReadBundle(golden(t, Bundle), s)
	if err != nil || got.Contract != "1.3" || got.Identity.Name != "example" || len(got.Health.Numbers) != 7 || got.Rules == nil {
		t.Fatalf("ReadBundle(minimal) = %+v, %v", got, err)
	}
	again, err := Encode(Bundle, got)
	if err != nil || !bytes.Equal(again, golden(t, Bundle)) {
		t.Errorf("Encode(ReadBundle(minimal)) = %v\n%s", err, again)
	}
}

// with is the minimal bundle with its text old replaced by new, once.
func with(t *testing.T, old, new string) []byte {
	t.Helper()
	b := golden(t, Bundle)
	if bytes.Count(b, []byte(old)) != 1 {
		t.Fatalf("the minimal bundle holds %q %d times", old, bytes.Count(b, []byte(old)))
	}
	return bytes.Replace(b, []byte(old), []byte(new), 1)
}

// TestReadBundleRefusesEachPlantedInputAtItsStep: each step of ReadBundle
// refuses the input planted for it, with the schema passing everything but
// the input planted for it, so each step is held alone.
func TestReadBundleRefusesEachPlantedInputAtItsStep(t *testing.T) {
	t.Parallel()
	w, s := world(t, schemaFiles(t))
	schemaBad := with(t, `"name": "example"`, `"name": "planted"`)
	v, err := canon.Decode(schemaBad)
	if err != nil {
		t.Fatal(err)
	}
	body, err := canon.Encode(v, canon.Document)
	if err != nil {
		t.Fatal(err)
	}
	w.Plant(Bundle.schema(1), body, model.Problem{Msg: "schema at /identity/name: planted"})
	cases := []struct {
		name, step string
		doc        []byte
		want       string
	}{
		{"a duplicate key", "2 canon", with(t, `"released": false,`, `"released": false, "released": true,`), "(duplicate key at byte"},
		{"a schema finding", "3 the schema", schemaBad, "fails its contract (schema at /identity/name: planted)"},
		{"an unknown field", "4 the decode", with(t, `"released": false,`, `"released": false, "trusted": true,`), `its fields are not the model's: the document has no field "trusted"`},
		{"a key that differs only in case, beside its field", "4 the decode", with(t, `"released": false,`, `"released": false, "Rules": [],`), `the document has no field "Rules"`},
		{"a key that differs only in case, for its field", "4 the decode", with(t, `"identity": {`, `"Identity": {`), `the document has no field "Identity"`},
		{"a nested key that differs only in case", "4 the decode", with(t, `"review_days": 1,`, `"review_days": 1, "Review_Days": 9,`), `/identity/governance has no field "Review_Days"`},
		// encoding/json read this one into the exclusions field, and the drift
		// check could not tell it from the field's own empty value.
		{"a lone empty key that differs only in case", "4 the decode", with(t, `"released": false,`, `"released": false, "Exclusions": [],`), `the document has no field "Exclusions"`},
		{"a member of another JSON type than its field", "4 the decode", with(t, `"released": false,`, `"released": "no",`), `/released: want a boolean, found a string`},
	}
	for _, c := range cases {
		got, err := ReadBundle(c.doc, s)
		var r *Refused
		if !errors.As(err, &r) || !strings.Contains(err.Error(), c.want) || r.Word() != model.Refused || got.Contract != "" {
			t.Errorf("%s (step %s): ReadBundle = %v, want refused with %q", c.name, c.step, err, c.want)
		}
	}
	for _, found := range []string{"2.0", "1.4"} {
		var nr *NotReady
		_, err := ReadBundle(with(t, `"contract": "1.3",
  "decisions"`, `"contract": "`+found+`",
  "decisions"`), s)
		if !errors.As(err, &nr) || nr.Found.String() != found || nr.Reads != Current() {
			t.Errorf("ReadBundle of contract %s = %v, want not ready naming both versions", found, err)
		}
	}
	// A later minor's new field fails the 1.3 schema, which closes every
	// object; the bundle is still not ready, naming both versions, and not
	// refused for the field it adds.
	later := with(t, `"contract": "1.3",
  "decisions"`, `"contract": "1.4",
  "added_in_1_4": true,
  "decisions"`)
	lv, err := canon.Decode(later)
	if err != nil {
		t.Fatal(err)
	}
	lb, err := canon.Encode(lv, canon.Document)
	if err != nil {
		t.Fatal(err)
	}
	w.Plant(Bundle.schema(1), lb, model.Problem{Msg: "schema at /added_in_1_4: planted"})
	if got, err := Validate(Bundle, later, s); err != nil || len(got.Findings) == 0 {
		t.Fatalf("Validate of the later minor's bundle = %+v, %v, want the planted finding", got, err)
	}
	var nr *NotReady
	if _, err := ReadBundle(later, s); !errors.As(err, &nr) || nr.Found.String() != "1.4" || nr.Reads != Current() || !strings.Contains(err.Error(), "contract 1.4") || !strings.Contains(err.Error(), "contract 1.3") {
		t.Errorf("ReadBundle of a 1.4 bundle with a field 1.3 lacks = %v, want not ready naming 1.4 and 1.3", err)
	}
	// Validate reads a newer minor against its major's schema.
	if got, err := Validate(Changelog, []byte(`{"contract": "1.4", "releases": [], "rulebook": "example"}`), s); err != nil || len(got.Findings) != 0 || got.Contract.String() != "1.4" {
		t.Errorf("Validate of a changelog in contract 1.4 =%+v, %v, want valid", got, err)
	}
}

// FuzzReadBundle: anything ReadBundle accepts encodes to bytes that read back
// to a value that encodes to the same bytes (tests.generated.1).
func FuzzReadBundle(f *testing.F) {
	files := schemaFiles(f)
	f.Add(golden(f, Bundle))
	f.Add([]byte(`{"contract": "1.3", "rules": [], "Rules": [{}]}`))
	f.Add([]byte(`{"contract": "1.3", "identity": {"name": "a", "Name": "b"}}`))
	f.Add([]byte(`{"contract": "2.0"}`))
	f.Add([]byte(`{"contract": "1.3", "health": {"numbers": [{"name": "a", "value": 1.5e300}]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, s := world(t, files)
		first, err := ReadBundle(data, s)
		if err != nil {
			var nr *NotReady
			var r *Refused
			if !errors.As(err, &nr) && !errors.As(err, &r) {
				t.Fatalf("ReadBundle failed: %v", err)
			}
			return
		}
		b1, err := Encode(Bundle, first)
		if err != nil {
			t.Fatalf("Encode of an accepted bundle: %v", err)
		}
		second, err := ReadBundle(b1, s)
		if err != nil {
			t.Fatalf("ReadBundle of its own encoding: %v\n%s", err, b1)
		}
		b2, err := Encode(Bundle, second)
		if err != nil || !bytes.Equal(b1, b2) {
			t.Fatalf("the encoding is not stable: %v\n%s\n%s", err, b1, b2)
		}
	})
}

// FuzzValidate: Validate of any bytes as any document answers findings or
// not ready, and not ready only for a newer major.
func FuzzValidate(f *testing.F) {
	files := schemaFiles(f)
	for i, k := range Kinds() {
		f.Add(uint8(i), golden(f, k))
	}
	f.Add(uint8(0), []byte(`{"contract": "1.9", "rules": [{"Id": 1}]}`))
	f.Add(uint8(4), []byte(`{"contract": "3.0"}`))
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		_, s := world(t, files)
		k := Kinds()[int(which)%len(Kinds())]
		got, err := Validate(k, data, s)
		var nr *NotReady
		switch {
		case errors.As(err, &nr):
			if nr.Found.Major <= Current().Major {
				t.Fatalf("not ready for contract %s", nr.Found)
			}
		case err != nil:
			t.Fatalf("Validate(%s) failed: %v", k, err)
		default:
			for _, f := range got.Findings {
				if f.Entity == "" || f.Msg == "" {
					t.Fatalf("a finding with no entity or message: %+v", f)
				}
			}
		}
	})
}

// TestTheSizeCapIsHeldAtItsBoundary (tests.generated.7): the document of
// exactly the cap is not refused for its size, and one byte over is, so the
// cap is neither a byte short nor a byte long. The cap is the reader's Cap,
// set to 16, so no test writes MaxDocument bytes. boundary: MaxDocument.
func TestTheSizeCapIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	s.Cap = 16
	at := []byte(`{"a":"xxxxxxxx"}`) // 16 bytes of one JSON value
	if len(at) != s.Cap {
		t.Fatalf("the document at the cap is %d bytes, want %d", len(at), s.Cap)
	}
	_, err := ReadBundle(at, s)
	var r *Refused
	if !errors.As(err, &r) || strings.Contains(err.Error(), "more than the") {
		t.Errorf("a document of the cap: %v, want a refusal for something other than its size", err)
	}
	over := append(slices.Clone(at), ' ')
	_, err = ReadBundle(over, s)
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "the bundle is 17 bytes, more than the 16 a document may hold") {
		t.Errorf("a document one byte over the cap: %v, want it refused for its size", err)
	}
	if MaxDocument != 64<<20 {
		t.Errorf("MaxDocument = %d, want 64 MiB: the cap above is a stand-in for it", MaxDocument)
	}
}

// TestACapOnlyNarrowsTheLimit: with no Cap the limit is MaxDocument, and a
// Cap over it is MaxDocument too, so the default is held with no document of
// 64 MiB written; a Cap under it is the Cap.
func TestACapOnlyNarrowsTheLimit(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ cap, want int }{
		"none":      {0, MaxDocument},
		"one over":  {MaxDocument + 1, MaxDocument},
		"far over":  {1 << 62, MaxDocument},
		"negative":  {-1, MaxDocument},
		"at":        {MaxDocument, MaxDocument},
		"one under": {MaxDocument - 1, MaxDocument - 1},
		"small":     {16, 16},
	} {
		if got := limit(Schemas{Cap: c.cap}); got != c.want {
			t.Errorf("%s: limit(Cap %d) = %d, want %d", name, c.cap, got, c.want)
		}
	}
}
