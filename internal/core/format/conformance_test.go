package format

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The fixture rulebooks are the conformance suite (S25): the files under
// internal/host/assets/fixture, embedded in the program and never for use. These
// tests read them from the source tree, each into a copy of its own in the
// in-memory world, so no test touches the folder (tests.isolation.1).

const fixtureDir = "../../host/assets/fixture"

// fixtureFilesOf is every file below fixture/<dir>, by path below it. It fails
// the test when it finds none, so a loop over it never passes by running nothing.
func fixtureFilesOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	base := os.DirFS(filepath.Join(fixtureDir, dir))
	err := fs.WalkDir(base, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(base, p)
		if err != nil {
			return err
		}
		out[p] = string(b)
		return nil
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("fixture/%s: %d files, %v", dir, len(out), err)
	}
	return out
}

// loadFixture is the fixture rulebook dir, read by Load from a copy of it.
func loadFixture(t *testing.T, dir string) Loaded {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", fixtureFilesOf(t, dir)))
	if err != nil {
		t.Fatalf("Load of fixture/%s: %v", dir, err)
	}
	return got
}

func TestTheBaseAndTheSecondFixtureLoadWithNoFinding(t *testing.T) {
	t.Parallel()
	for dir, rules := range map[string]int{"base": 15, "other": 3} {
		got := loadFixture(t, dir)
		if len(got.Findings) != 0 {
			t.Errorf("fixture/%s: %v", dir, got.Findings)
		}
		if len(got.Book.Rules) != rules {
			t.Errorf("fixture/%s holds %d rules, want %d", dir, len(got.Book.Rules), rules)
		}
	}
}

func TestEveryFixtureHistoryLineIsCanonicalAndNamesTheLineBeforeIt(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"base", "other"} {
		lines := strings.Split(strings.TrimSuffix(fixtureFilesOf(t, dir)["history.jsonl"], "\n"), "\n")
		var previous string
		for i, line := range lines {
			v, err := canon.Decode([]byte(line))
			if err != nil {
				t.Fatalf("fixture/%s history line %d: %v", dir, i+1, err)
			}
			enc, err := canon.Encode(v, canon.Canonical)
			if err != nil || string(enc) != line {
				t.Errorf("fixture/%s history line %d is not in the canonical form", dir, i+1)
			}
			var e model.HistoryEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatal(err)
			}
			if e.Number != i+1 || string(e.Previous) != previous {
				t.Errorf("fixture/%s history line %d: number %d, previous %q, want %d and %q", dir, i+1, e.Number, e.Previous, i+1, previous)
			}
			d, err := canon.DigestOf(v)
			if err != nil {
				t.Fatal(err)
			}
			previous = d.String()
		}
		if len(lines) < 4 {
			t.Errorf("fixture/%s: %d history lines", dir, len(lines))
		}
	}
}

// coveredTypes are the types of the rulebook, with the file each one's fields
// are written in. A field is a row of the coverage table by Type.json_tag.
func coveredTypes() []any {
	return []any{
		model.RulebookIdentity{}, model.Licences{}, model.Extends{}, model.Import{}, model.Governance{}, model.Tested{},
		model.Version{}, model.Budgets{}, model.Structure{}, model.Layer{}, model.Part{}, model.Concern{}, model.ProgramKind{},
		model.Rule{}, model.RuleScope{}, model.Exemption{}, model.Reasoning{}, model.Pair{}, model.Relations{}, model.Citation{},
		model.Binding{}, model.Term{}, model.Source{}, model.Decision{}, model.Cites{}, model.Exclusion{}, model.Lifted{},
		model.Translation{}, model.Proposal{}, model.HistoryEntry{}, model.Lifecycle{},
	}
}

// bookFields are the fields of Book that are lists or maps of the types above,
// named as the coverage table names them.
func bookFields() []string { return []string{"Book.anchors", "Book.binding_records"} }

// fieldTag is the name a field is written under, "" for one with no name.
func fieldTag(f reflect.StructField) string {
	tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if tag == "-" {
		return ""
	}
	return tag
}

// allFields is every Type.tag of the covered types, and of Book.
func allFields() []string {
	out := bookFields()
	for _, v := range coveredTypes() {
		typ := reflect.TypeOf(v)
		for field := range typ.Fields() {
			if tag := fieldTag(field); tag != "" {
				out = append(out, typ.Name()+"."+tag)
			}
		}
	}
	sort.Strings(out)
	return out
}

// exercised marks Type.tag for each field that is set in v or in anything v holds.
func exercised(v reflect.Value, seen map[string]bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			exercised(v.Elem(), seen)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			exercised(v.Index(i), seen)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			exercised(v.MapIndex(k), seen)
		}
	case reflect.Struct:
		typ := v.Type()
		for i := range typ.NumField() {
			if typ.PkgPath() != reflect.TypeFor[model.Rule]().PkgPath() {
				continue
			}
			f := v.Field(i)
			if f.IsZero() {
				continue
			}
			// a field with no name (Book's) holds values to look into, and is no row
			if tag := fieldTag(typ.Field(i)); tag != "" {
				seen[typ.Name()+"."+tag] = true
			}
			exercised(f, seen)
		}
	}
}

// exercisedBy is every field the books set, by Type.tag.
func exercisedBy(books []model.Book) map[string]bool {
	seen := map[string]bool{}
	for _, b := range books {
		exercised(reflect.ValueOf(b), seen)
		if len(b.Anchors) > 0 {
			seen["Book.anchors"] = true
		}
		if len(b.BindingRecords) > 0 {
			seen["Book.binding_records"] = true
		}
	}
	return seen
}

// gaps are the fields of the model's rulebook types that no value of books uses.
// It answers how many fields it looked at, so a caller can see that it ran.
func gaps(books []model.Book) (missing []string, looked int) {
	seen := exercisedBy(books)
	for _, f := range allFields() {
		looked++
		if !seen[f] {
			missing = append(missing, f)
		}
	}
	return missing, looked
}

func fixtureBooks(t *testing.T) []model.Book {
	t.Helper()
	return []model.Book{loadFixture(t, "base").Book, loadFixture(t, "other").Book}
}

func TestEveryFieldOfTheRulebookTypesIsExercisedByAFixtureRow(t *testing.T) {
	t.Parallel()
	missing, looked := gaps(fixtureBooks(t))
	if looked != len(allFields()) || looked < 100 {
		t.Fatalf("the coverage loop looked at %d fields, want all %d", looked, len(allFields()))
	}
	if len(missing) > 0 {
		t.Errorf("no fixture row exercises: %s", strings.Join(missing, ", "))
	}
}

// TestAFieldNoFixtureRowExercisesTurnsTheCoverageRed: the plant takes every use
// of one field out of the loaded books; the control is the books as loaded.
func TestAFieldNoFixtureRowExercisesTurnsTheCoverageRed(t *testing.T) {
	t.Parallel()
	books := fixtureBooks(t)
	if missing, _ := gaps(books); len(missing) != 0 {
		t.Fatalf("control: the fixtures leave gaps: %v", missing)
	}
	for i := range books {
		for j := range books[i].Rules {
			books[i].Rules[j].ReviewBy = ""
		}
		books[i].Identity.DocsURL = ""
	}
	missing, _ := gaps(books)
	if !slices.Equal(missing, []string{"RulebookIdentity.docs_url", "Rule.review_by"}) && !slices.Equal(missing, []string{"Rule.review_by", "RulebookIdentity.docs_url"}) {
		t.Errorf("the planted gaps were reported as %v", missing)
	}
}

// coverageRows is the table of the fixture README: Type.field to the file that
// carries it, below fixture/.
func coverageRows(t *testing.T, readme string) map[string]string {
	t.Helper()
	rows := map[string]string{}
	re := regexp.MustCompile("(?m)^\\| ([A-Z][A-Za-z]*\\.[a-z_]+) \\| `([^`]+)` \\|$")
	for _, m := range re.FindAllStringSubmatch(readme, -1) {
		rows[m[1]] = m[2]
	}
	return rows
}

// needleOf is how a field is written in the file that carries it.
func needleOf(field string) string {
	switch field {
	case "Book.anchors":
		return "anchor"
	case "Book.binding_records":
		return "subject"
	}
	_, tag, _ := strings.Cut(field, ".")
	return tag
}

// rowProblems are the faults of a coverage table: a field with no row, a row
// for no field, and a row whose file is missing or does not carry the field.
func rowProblems(rows map[string]string, files func(string) (string, bool)) []string {
	var out []string
	want := allFields()
	for _, f := range want {
		file, ok := rows[f]
		if !ok {
			out = append(out, f+" has no row")
			continue
		}
		body, ok := files(file)
		n := needleOf(f)
		if !ok {
			out = append(out, f+": the file "+file+" is not in the fixture")
		} else if !strings.Contains(body, n+":") && !strings.Contains(body, `"`+n+`":`) {
			out = append(out, f+": "+file+" does not carry "+n)
		}
	}
	for f := range rows {
		if !slices.Contains(want, f) {
			out = append(out, f+" is a row for no field of the model")
		}
	}
	sort.Strings(out)
	return out
}

func readmeAndFiles(t *testing.T) (string, func(string) (string, bool)) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b), func(p string) (string, bool) {
		c, err := os.ReadFile(filepath.Join(fixtureDir, filepath.FromSlash(p)))
		return string(c), err == nil
	}
}

func TestTheReadmeCoverageTableNamesTheFileOfEveryField(t *testing.T) {
	t.Parallel()
	readme, files := readmeAndFiles(t)
	rows := coverageRows(t, readme)
	if len(rows) != len(allFields()) {
		t.Errorf("the table has %d rows, the model %d fields", len(rows), len(allFields()))
	}
	if p := rowProblems(rows, files); len(p) > 0 {
		t.Errorf("the coverage table:\n%s", strings.Join(p, "\n"))
	}
}

// TestACoverageTableMissingARowOrNamingAWrongFileIsRefused: the plants are a
// table with one row taken out, one row for a field the model does not have,
// and one row that names a file which lacks the field.
func TestACoverageTableMissingARowOrNamingAWrongFileIsRefused(t *testing.T) {
	t.Parallel()
	readme, files := readmeAndFiles(t)
	rows := coverageRows(t, readme)
	if p := rowProblems(rows, files); len(p) != 0 {
		t.Fatalf("control: %v", p)
	}
	delete(rows, "Rule.review_by")
	rows["Rule.no_such_field"] = "base/rulebook.yaml"
	rows["Term.literal"] = "base/sources.yaml"
	got := strings.Join(rowProblems(rows, files), "\n")
	for _, want := range []string{"Rule.review_by has no row", "Rule.no_such_field is a row for no field", "Term.literal: base/sources.yaml does not carry literal"} {
		if !strings.Contains(got, want) {
			t.Errorf("the refusal lacks %q:\n%s", want, got)
		}
	}
}

// TestEveryMemberOfAClosedSetIsUsedButTheOnesTheReadmeNames: each set the model
// lists is used by a fixture value, but for the members no fixture can hold, and
// the README says which those are. A new member of a set is a decision there.
func TestEveryMemberOfAClosedSetIsUsedButTheOnesTheReadmeNames(t *testing.T) {
	t.Parallel()
	books := fixtureBooks(t)
	used := map[string]map[string]bool{}
	use := func(set, v string) {
		if used[set] == nil {
			used[set] = map[string]bool{}
		}
		used[set][v] = true
	}
	for _, b := range books {
		use("kind", string(b.Identity.Kind))
		use("contribution", string(b.Identity.Governance.Contribution))
		use("proof", string(b.Identity.Governance.CheckerProof))
		for _, r := range b.Rules {
			use("strength", string(r.Strength))
			use("status", string(r.Status))
			use("origin", string(r.Origin))
			for _, c := range r.Sources {
				use("backs", string(c.Backs))
			}
			for _, p := range r.Pairs {
				use("target", string(p.Target))
			}
			for _, bd := range r.Bindings {
				use("target", string(bd.Target))
				use("severity", string(bd.Severity))
				use("runpoint", string(bd.RunPoint))
			}
		}
		for _, s := range b.Sources {
			use("source", string(s.Kind))
		}
		for _, p := range b.Proposals {
			use("proposal", string(p.State))
		}
	}
	sets := []struct {
		name    string
		members []string
	}{
		{"kind", strs(model.IdentityKinds())}, {"contribution", strs(model.Contributions())}, {"proof", strs(model.ProofLevels())},
		{"strength", strs(model.Strengths())}, {"status", strs(model.Statuses())}, {"origin", strs(model.Origins())},
		{"backs", strs(model.Backings())}, {"target", strs(model.TargetTypes())}, {"severity", strs(model.Severities())},
		{"runpoint", strs(model.RunPoints())}, {"source", strs(model.SourceKinds())}, {"proposal", strs(model.ProposalStates())},
	}
	notHeld := map[string][]string{"kind": {"companion", "fork", "archived"}, "contribution": {"closed"}, "proof": {"isolated"}}
	readme, _ := readmeAndFiles(t)
	section := readme[strings.Index(readme, "## Not exercised"):]
	section = section[:strings.Index(section[1:], "\n## ")+1]
	ran := 0
	for _, s := range sets {
		var missing []string
		for _, m := range s.members {
			ran++
			if !used[s.name][m] {
				missing = append(missing, m)
			}
		}
		sort.Strings(missing)
		want := slices.Clone(notHeld[s.name])
		sort.Strings(want)
		if !slices.Equal(missing, want) {
			t.Errorf("%s: no fixture value uses %v, the README is held to %v", s.name, missing, want)
		}
		for _, m := range want {
			if !strings.Contains(section, "`"+m+"`") {
				t.Errorf("the README's not-exercised section does not name %s", m)
			}
		}
	}
	if ran < 40 {
		t.Errorf("the loop looked at %d members", ran)
	}
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

// standIns is gate/standins.txt: the names a fixture may use.
func standIns(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile("../../../gate/standins.txt")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for line := range strings.SplitSeq(string(b), "\n") {
		name, _, _ := strings.Cut(line, "#")
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("the stand-in list is empty")
	}
	return out
}

var (
	whoPattern    = regexp.MustCompile(`(?:\b(?:owner|who|attribution):\s*|"who":)"?([^,}"\s]+)`)
	ownersPattern = regexp.MustCompile(`\bowners:\s*\[([^\]]*)\]`)
	hostPattern   = regexp.MustCompile(`https?://([^/\s"']+)`)
	mailPattern   = regexp.MustCompile(`[\w.+-]+@([\w.-]+\.[a-z]+)`)
	ipPattern     = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	keyPattern    = regexp.MustCompile(`ssh-ed25519 \S+ ([^\s"]+)`)
)

// namesIn is every name-shaped value of a file: who a person is, an owner, a
// host, an address and the comment of a key.
func namesIn(body string) []string {
	var out []string
	for _, m := range whoPattern.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	for _, m := range ownersPattern.FindAllStringSubmatch(body, -1) {
		for o := range strings.SplitSeq(m[1], ",") {
			if o = strings.Trim(strings.TrimSpace(o), `"`); o != "" {
				out = append(out, o)
			}
		}
	}
	for _, re := range []*regexp.Regexp{hostPattern, mailPattern, keyPattern} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			out = append(out, m[1])
		}
	}
	return append(out, ipPattern.FindAllString(body, -1)...)
}

// strangers are the names in files that the stand-in list does not hold, by file.
func strangers(files map[string]string, list map[string]bool) []string {
	var out []string
	for p, body := range files {
		for _, n := range namesIn(body) {
			if !list[n] {
				out = append(out, p+": "+n)
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestEveryNameInTheFixturesIsOnTheStandInList(t *testing.T) {
	t.Parallel()
	list := standIns(t)
	files := map[string]string{}
	names := 0
	for _, dir := range []string{"base", "other", "projects/clean", "projects/weakened"} {
		for p, body := range fixtureFilesOf(t, dir) {
			files[dir+"/"+p] = body
			names += len(namesIn(body))
		}
	}
	if names < 40 {
		t.Fatalf("the scan found %d names in %d files", names, len(files))
	}
	if s := strangers(files, list); len(s) > 0 {
		t.Errorf("names not on gate/standins.txt:\n%s", strings.Join(s, "\n"))
	}
}

// TestANameOffTheStandInListIsFound: the plants put a person, an owner, a host
// and an address off the list into one file each; the control is the list's own.
func TestANameOffTheStandInListIsFound(t *testing.T) {
	t.Parallel()
	list := standIns(t)
	control := map[string]string{"a.yaml": "owners: [alex]\nwho: alex\nurl: https://example.com/x\nhost: 192.0.2.1\n"}
	if s := strangers(control, list); len(s) != 0 {
		t.Fatalf("control: %v", s)
	}
	plants := map[string]string{
		"who.yaml":    "who: maya\n",
		"owners.yaml": "owners: [alex, team-z]\n",
		"url.yaml":    "link: https://git.corp.test/x\n",
		"ip.yaml":     "host: 10.1.2.3\n",
		"key.yaml":    "ssh-ed25519 AAAA maya@laptop\n",
		"json.jsonl":  `{"who":"maya"}`,
	}
	got := strings.Join(strangers(plants, list), "\n")
	for _, want := range []string{"who.yaml: maya", "owners.yaml: team-z", "url.yaml: git.corp.test", "ip.yaml: 10.1.2.3", "key.yaml: maya@laptop", "json.jsonl: maya"} {
		if !strings.Contains(got, want) {
			t.Errorf("the scan missed %q:\n%s", want, got)
		}
	}
}

// TestEachFixtureProposalCarriesItsStatesContractDocument: the
// document of each of the base fixture's four proposals is the contract's
// golden proposal in that state, as core/contract encodes it, read as YAML.
// cmd/gragas holds those goldens to the real contract schema, so each
// fixture document is one contract validate takes. Planted failure: a
// fixture document that differs from its golden in one field.
func TestEachFixtureProposalCarriesItsStatesContractDocument(t *testing.T) {
	t.Parallel()
	l := loadFixture(t, "base")
	if len(l.Book.Proposals) != len(model.ProposalStates()) {
		t.Fatalf("the base fixture has %d proposals, want one per state", len(l.Book.Proposals))
	}
	for _, p := range l.Book.Proposals {
		b, err := os.ReadFile(filepath.Join("..", "contract", "testdata", "proposal-"+string(p.State)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want map[string]any
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.Document, want) {
			t.Errorf("proposals/%s.yaml's document is\n%v\nwant the %s golden\n%v", p.Slug, p.Document, p.State, want)
		}
	}
}
