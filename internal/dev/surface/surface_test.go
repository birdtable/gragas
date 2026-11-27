package surface_test

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/surface"
	"github.com/birdtable/gragas/internal/digest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// code is one package with every kind of export, and the things a consumer
// cannot name.
const code = "package p\n" + `
import "io"

// Clock tells the time. It is a port.
type Clock interface {
	io.Closer
	// Now answers the instant.
	Now() int64
	hidden()
}

// Row is one row.
type Row struct {
	Name string ` + "`json:\"name\"`" + `
	secret int
	io.Reader
}

// Pair holds two of a kind.
type Pair[T any] struct{ A, B T }

type hiddenType struct{}

// Open opens a clock.
func Open(n int) (Clock, error) { return nil, nil }

// Map maps.
func Map[T any](x T) T { return x }

// Len is the length of a row.
func (r *Row) Len() int { return 0 }

func (r *Row) quiet() {}

func (hiddenType) Seen() {}

// Max is the most.
const Max int = 3

// Colours.
const (
	Red = iota
	Blue
	hidden2
)

// ErrGone says it is gone.
var ErrGone error

var unexported int
`

func src(body string) []surface.Source {
	return []surface.Source{{Dir: "internal/p", Files: map[string][]byte{"p.go": []byte(body)}}}
}

func collect(t *testing.T, body string) []surface.Section {
	t.Helper()
	secs, err := surface.Collect(src(body))
	if err != nil {
		t.Fatal(err)
	}
	return secs
}

func names(secs []surface.Section) []string {
	var out []string
	for _, s := range secs {
		for _, e := range s.Exports {
			out = append(out, e.Kind+" "+e.Name)
		}
	}
	return out
}

// marksFile parses the marks file text.
func marksFile(t *testing.T, text string) *surface.MarkFile {
	t.Helper()
	mf, err := surface.ParseMarks([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return mf
}

// recorded is the marks file with rows (a "marks:" block) and, for the
// listing of body under those rows, a change record that says why.
func recorded(t *testing.T, rows, body string) *surface.MarkFile {
	t.Helper()
	secs := collect(t, body)
	surface.Mark(secs, marksFile(t, rows))
	return marksFile(t, rows+"changes:\n  - {digest: \""+surface.Digest(secs)+"\", reason: \"the fixture changed for a test\"}\n")
}

// render is the listing file of body under mf.
func render(t *testing.T, body string, mf *surface.MarkFile) []byte {
	t.Helper()
	secs := collect(t, body)
	surface.Mark(secs, mf)
	return surface.Render(secs, mf.Changes)
}

const noRows = "marks: []\n"

// TestEveryKindOfExportIsListedAndNothingUnexported: a type, an interface's
// methods, a struct's exported fields only, a generic, a function, a method,
// a constant, a variable. Planted: an unexported name of each kind is in the
// code and none is listed.
func TestEveryKindOfExportIsListedAndNothingUnexported(t *testing.T) {
	t.Parallel()
	got := names(collect(t, code))
	want := []string{
		"const Blue", "const Max", "const Red", "func Map", "func Open",
		"method Clock.Now", "method Row.Len", "type Clock", "type Pair", "type Row", "var ErrGone",
	}
	slices.Sort(got)
	if len(got) == 0 {
		t.Fatal("nothing was listed")
	}
	if !slices.Equal(got, want) {
		t.Errorf("listed\n%v\nwant\n%v", got, want)
	}
}

// TestASignatureIsWhatAConsumerCanName: the unexported field and method are
// out of it, the generics and the tag are in.
func TestASignatureIsWhatAConsumerCanName(t *testing.T) {
	t.Parallel()
	sigs := map[string]string{}
	for _, e := range collect(t, code)[0].Exports {
		sigs[e.Name] = e.Signature
	}
	for name, want := range map[string]string{
		"Row":   "struct{Name string `json:\"name\"`; io.Reader}",
		"Clock": "interface{io.Closer}",
		"Pair":  "[T any] struct{A, B T}",
		"Map":   "func[T any](x T) T",
		"Open":  "func(n int) (Clock, error)",
		"Max":   "int = 3",
	} {
		if got := sigs[name]; got != want {
			t.Errorf("%s is %q, want %q", name, got, want)
		}
	}
}

// TestTheFirstSentenceOfTheCommentIsQuoted: the summary stops at the first
// sentence; a name in a group takes the group's comment.
func TestTheFirstSentenceOfTheCommentIsQuoted(t *testing.T) {
	t.Parallel()
	sums := map[string]string{}
	for _, e := range collect(t, code)[0].Exports {
		sums[e.Name] = e.Summary
	}
	for name, want := range map[string]string{
		"Clock": "Clock tells the time.", "Clock.Now": "Now answers the instant.", "Blue": "Colours.",
	} {
		if got := sums[name]; got != want {
			t.Errorf("%s summary %q, want %q", name, got, want)
		}
	}
}

// TestACleanListingPasses is the control: the listing of the code under its
// marks file, with its change recorded, has no problem.
func TestACleanListingPasses(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	if p := surface.Problems(collect(t, code), mf, render(t, code, mf)); len(p) != 0 {
		t.Errorf("a clean listing has problems: %v", p)
	}
}

// TestANewExportFailsUntilTheListingAndAReasonChange: planted, an export the
// file does not hold. It fails as not listed and as unrecorded; the
// regenerated file with no reason still fails, and with one passes.
func TestANewExportFailsUntilTheListingAndAReasonChange(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	have := render(t, code, mf)
	grown := code + "\n// Extra is new.\nfunc Extra() {}\n"

	p := surface.Problems(collect(t, grown), mf, have)
	if !slices.Contains(p, "not listed: internal/p Extra") {
		t.Errorf("a new export was not seen: %v", p)
	}
	if !hasPrefix(p, "the surface or a mark changed") {
		t.Errorf("a new export with no reason was not seen: %v", p)
	}

	secs := collect(t, grown)
	surface.Mark(secs, mf)
	if !slices.ContainsFunc(secs[0].Exports, func(e surface.Export) bool { return e.Name == "Extra" && e.Mark == surface.Experimental }) {
		t.Error("a new export did not start as experimental")
	}
	p = surface.Problems(collect(t, grown), mf, render(t, grown, mf))
	if len(p) != 1 || !hasPrefix(p, "the surface or a mark changed") || !strings.Contains(p[0], surface.Digest(secs)) {
		t.Errorf("a regenerated listing with no reason should fail on the reason alone, naming the digest: %v", p)
	}

	mf = recorded(t, noRows, grown)
	if p := surface.Problems(collect(t, grown), mf, render(t, grown, mf)); len(p) != 0 {
		t.Errorf("a listing with its reason has problems: %v", p)
	}
}

// TestARemovedExportFails: planted, an export in the file the code no longer
// has.
func TestARemovedExportFails(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	shrunk := strings.Replace(code, "func Open(n int) (Clock, error) { return nil, nil }", "", 1)
	p := surface.Problems(collect(t, shrunk), mf, render(t, code, mf))
	if !slices.Contains(p, "listed but gone: internal/p Open") {
		t.Errorf("a removed export was not seen: %v", p)
	}
}

// TestAChangedSignatureFails: the same name with another signature.
func TestAChangedSignatureFails(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	changed := strings.Replace(code, "func Open(n int)", "func Open(n, m int)", 1)
	p := surface.Problems(collect(t, changed), mf, render(t, code, mf))
	if !slices.Contains(p, "changed: internal/p Open") {
		t.Errorf("a changed signature was not seen: %v", p)
	}
}

const stableClock = `marks:
  - {package: internal/p, name: Clock, mark: stable, reason: "the port is held to its contract"}
`

// TestAMarkChangeFailsUntilItsReasonIsRecorded: a mark added in the marks
// file changes the listing and its digest. The old listing is stale, the
// regenerated one fails on the change record alone, and with the change
// recorded it passes. The control is the same file before the mark.
func TestAMarkChangeFailsUntilItsReasonIsRecorded(t *testing.T) {
	t.Parallel()
	before := recorded(t, noRows, code)
	have := render(t, code, before)

	after := marksFile(t, stableClock+"changes:\n  - {digest: \""+before.Changes[0].Digest+"\", reason: \"the fixture changed for a test\"}\n")
	p := surface.Problems(collect(t, code), after, have)
	if !slices.Contains(p, "changed: internal/p Clock") || !hasPrefix(p, "the surface or a mark changed") {
		t.Errorf("a new mark should change the listing and need a reason: %v", p)
	}
	p = surface.Problems(collect(t, code), after, render(t, code, after))
	if len(p) != 1 || !hasPrefix(p, "the surface or a mark changed") {
		t.Errorf("a regenerated listing should fail on the change record alone: %v", p)
	}

	after = recorded(t, stableClock, code)
	if p := surface.Problems(collect(t, code), after, render(t, code, after)); len(p) != 0 {
		t.Errorf("a recorded mark change has problems: %v", p)
	}
}

// TestAHandEditedListingFails: the listing is generated, so a mark typed into
// it is not a mark. Planted: the edit; the control is the unedited file.
func TestAHandEditedListingFails(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	have := render(t, code, mf)
	edited := strings.Replace(string(have), "`Clock` (type) **experimental**", "`Clock` (type) **stable**", 1)
	if edited == string(have) {
		t.Fatal("the planted edit changed nothing")
	}
	if p := surface.Problems(collect(t, code), mf, []byte(edited)); !slices.Contains(p, "changed: internal/p Clock") {
		t.Errorf("a hand-edited mark was not seen: %v", p)
	}
	stale := strings.Replace(string(have), "Listing digest", "Listing  digest", 1)
	if p := surface.Problems(collect(t, code), mf, []byte(stale)); len(p) != 1 || !strings.Contains(p[0], "stale") {
		t.Errorf("an edit outside the export lines was not seen: %v", p)
	}
}

// TestAMarkOnAnExportThatIsNotThereFails: a row for a name the code does not
// export.
func TestAMarkOnAnExportThatIsNotThereFails(t *testing.T) {
	t.Parallel()
	rows := "marks:\n  - {package: internal/p, name: Nope, mark: stable, reason: \"it was meant to be\"}\n"
	mf := recorded(t, rows, code)
	p := surface.Problems(collect(t, code), mf, render(t, code, mf))
	if !slices.Contains(p, "marked but not exported: internal/p Nope") {
		t.Errorf("a mark on nothing was not seen: %v", p)
	}
}

// TestRewordingAContractCommentAsksForNoReason: the digest leaves comments
// out, so a regenerated listing with new words passes on the old record.
func TestRewordingAContractCommentAsksForNoReason(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	have := render(t, code, mf)
	reworded := strings.Replace(code, "Clock tells the time.", "Clock says what time it is.", 1)
	if p := surface.Problems(collect(t, reworded), mf, have); !slices.Contains(p, "changed: internal/p Clock") {
		t.Errorf("a stale quotation was not seen: %v", p)
	}
	if p := surface.Problems(collect(t, reworded), mf, render(t, reworded, mf)); len(p) != 0 {
		t.Errorf("the regenerated listing has problems: %v", p)
	}
}

// TestAMarksFileThatSaysNothingIsRefused: each way the authored file can be
// wrong, with a control that is read.
func TestAMarksFileThatSaysNothingIsRefused(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	good := "marks:\n  - {package: internal/p, name: Clock, mark: stable, reason: \"held to its contract\"}\nchanges:\n  - {digest: \"" + digest + "\", reason: \"the first listing\"}\n"
	if _, err := surface.ParseMarks([]byte(good)); err != nil {
		t.Fatalf("the control was refused: %v", err)
	}
	bad := map[string]string{
		"an unknown mark":      strings.Replace(good, "stable", "final", 1),
		"a one-word reason":    strings.Replace(good, "held to its contract", "needed", 1),
		"a row with no reason": strings.Replace(good, `, reason: "held to its contract"`, "", 1),
		"a change with no why": strings.Replace(good, "the first listing", "none", 1),
		"a digest that is not": strings.Replace(good, digest, "md5:"+strings.Repeat("a", 32), 1),
		"a row marked twice":   strings.Replace(good, "changes:", "  - {package: internal/p, name: Clock, mark: internal, reason: \"wired by main\"}\nchanges:", 1),
		"an unknown key":       good + "extra: 1\n",
		"not a mapping":        "- a\n",
	}
	for name, text := range bad {
		if text == good {
			t.Fatalf("%s: the planted edit changed nothing", name)
		}
		if _, err := surface.ParseMarks([]byte(text)); !errors.Is(err, surface.ErrMarks) {
			t.Errorf("%s was read as a marks file: %v", name, err)
		}
	}
}

// TestAMissingListingIsAProblem: no file is not a clean listing.
func TestAMissingListingIsAProblem(t *testing.T) {
	t.Parallel()
	mf := recorded(t, noRows, code)
	p := surface.Problems(collect(t, code), mf, nil)
	if !slices.ContainsFunc(p, func(s string) bool { return strings.Contains(s, "missing") }) {
		t.Errorf("a missing file: %v", p)
	}
}

// TestNoMarksFileLeavesEverythingUnrecorded: a nil marks file is an empty one.
func TestNoMarksFileLeavesEverythingUnrecorded(t *testing.T) {
	t.Parallel()
	if !surface.Unrecorded(collect(t, code), nil) {
		t.Error("a listing with no marks file was recorded")
	}
}

// TestCountIsTheExportedNamesPerPackage feeds the coupling report.
func TestCountIsTheExportedNamesPerPackage(t *testing.T) {
	t.Parallel()
	got := surface.Count(collect(t, code))
	if got["internal/p"] != 11 || len(got) != 1 {
		t.Errorf("count %v, want 11 names in internal/p", got)
	}
}

// TestASourceThatDoesNotParseIsRefused.
func TestASourceThatDoesNotParseIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := surface.Collect(src("package p\nfunc (")); err == nil {
		t.Error("a broken source was listed")
	}
}

func hasPrefix(problems []string, prefix string) bool {
	return slices.ContainsFunc(problems, func(p string) bool { return strings.HasPrefix(p, prefix) })
}

// marked is the listing of body under the marks file rows, its marks set.
func marked(t *testing.T, rows, body string) []surface.Section {
	t.Helper()
	secs := collect(t, body)
	surface.Mark(secs, marksFile(t, rows))
	return secs
}

// bigSum is Sum worked out apart, with math/big: each export's SHA-256 as a
// number, added modulo 2^256.
func bigSum(secs []surface.Section) *big.Int {
	total := new(big.Int)
	for _, s := range secs {
		for _, e := range s.Exports {
			h := digest.Sum([]byte(strings.Join([]string{s.Dir, e.Name, e.Kind, e.Signature, e.Mark}, "\x00")))
			total.Add(total, new(big.Int).SetBytes(h[:]))
		}
	}
	return total.Mod(total, new(big.Int).Lsh(big.NewInt(1), 256))
}

// hex64 is n modulo 2^256 in 64 hex digits.
func hex64(n *big.Int) string {
	m := new(big.Int).Mod(n, new(big.Int).Lsh(big.NewInt(1), 256))
	return fmt.Sprintf("%064x", m)
}

// step is the step from the listing of before to the listing of after, each
// under no rows, worked out with math/big.
func step(t *testing.T, before, after string) string {
	t.Helper()
	return hex64(new(big.Int).Sub(bigSum(marked(t, noRows, after)), bigSum(marked(t, noRows, before))))
}

// movedFile is a marks file under no rows whose record holds an old line, then
// the base for the listing of body, then each step.
func movedFile(t *testing.T, body string, steps ...string) *surface.MarkFile {
	t.Helper()
	text := noRows + "changes:\n  - {digest: \"sha256:" + strings.Repeat("a", 64) + "\", reason: \"an old line of the record\"}\n"
	text += "  - {base: \"" + hex64(bigSum(marked(t, noRows, body))) + "\", reason: \"the record moves to sums here\"}\n"
	for i, s := range steps {
		text += fmt.Sprintf("  - {step: %q, reason: \"change %d of the fixture\"}\n", s, i)
	}
	return marksFile(t, text)
}

// The two changes made apart: one adds Extra, the other changes Open's
// signature, and the code that holds both.
const (
	extra = "\n// Extra is new.\nfunc Extra() {}\n"
	openA = "func Open(n int)"
	openB = "func Open(n, m int)"
)

// TestTheSumIsEachExportsSHA256AddedModulo2To256: Sum against the same sum
// worked out with math/big, for the fixture and for a listing with no exports.
func TestTheSumIsEachExportsSHA256AddedModulo2To256(t *testing.T) {
	t.Parallel()
	ran := 0
	for _, body := range []string{code, code + extra, "package p\n"} {
		secs := marked(t, noRows, body)
		if got, want := surface.Sum(secs), hex64(bigSum(secs)); got != want {
			t.Errorf("Sum is %s, want %s", got, want)
		}
		ran++
	}
	if ran != 3 {
		t.Fatalf("summed %d of the 3 listings", ran)
	}
}

// TestTheSumHoldsWhatTheDigestHolds: a mark and a signature change it, a
// reworded comment does not, and neither does the order of the exports.
func TestTheSumHoldsWhatTheDigestHolds(t *testing.T) {
	t.Parallel()
	base := surface.Sum(marked(t, noRows, code))
	if surface.Sum(marked(t, stableClock, code)) == base {
		t.Error("a mark did not change the sum")
	}
	if surface.Sum(marked(t, noRows, strings.Replace(code, openA, openB, 1))) == base {
		t.Error("a signature did not change the sum")
	}
	if surface.Sum(marked(t, noRows, strings.Replace(code, "Clock tells the time.", "Clock says the time.", 1))) != base {
		t.Error("a reworded comment changed the sum")
	}
	secs := marked(t, noRows, code)
	slices.Reverse(secs[0].Exports)
	if surface.Sum(secs) != base {
		t.Error("the order of the exports changed the sum")
	}
}

// TestTwoChangesMadeApartAddUpWithNoLineForBoth is gragas#825: two pull
// requests each record the step their own change made, from the same main;
// the code that holds both is held by the base and the two steps, in either
// order, with no line for the two together. Planted: the record with one
// step of the two, which fails and names the other as the step to add.
func TestTwoChangesMadeApartAddUpWithNoLineForBoth(t *testing.T) {
	t.Parallel()
	a, b := code+extra, strings.Replace(code, openA, openB, 1)
	both := strings.Replace(code+extra, openA, openB, 1)
	stepA, stepB := step(t, code, a), step(t, code, b)

	ran := 0
	for _, order := range [][]string{{stepA, stepB}, {stepB, stepA}} {
		mf := movedFile(t, code, order...)
		if p := surface.Problems(collect(t, both), mf, render(t, both, mf)); len(p) != 0 {
			t.Errorf("the base and both steps, in the order %v, did not hold the listing of both: %v", order, p)
		}
		ran++
	}
	if ran != 2 {
		t.Fatalf("ran %d of the 2 orders", ran)
	}

	mf := movedFile(t, code, stepA)
	if !surface.Unrecorded(marked(t, noRows, both), mf) {
		t.Fatal("the listing of both was held with one step of the two")
	}
	if why := surface.Why(marked(t, noRows, both), mf); !strings.Contains(why, "the step "+stepB+" ") {
		t.Errorf("Why should name the other change's step %s: %s", stepB, why)
	}
}

// TestAChangeBothSidesMadeToOneExportDoesNotAddUp: two pull requests change
// the same export two ways, and the code keeps one; their steps do not add
// up, so a person writes the line. The control is each change on its own.
func TestAChangeBothSidesMadeToOneExportDoesNotAddUp(t *testing.T) {
	t.Parallel()
	a := strings.Replace(code, openA, "func Open(s string)", 1)
	b := strings.Replace(code, openA, openB, 1)
	stepA, stepB := step(t, code, a), step(t, code, b)
	if surface.Unrecorded(marked(t, noRows, a), movedFile(t, code, stepA)) || surface.Unrecorded(marked(t, noRows, b), movedFile(t, code, stepB)) {
		t.Fatal("a change on its own was not held by its step")
	}
	if !surface.Unrecorded(marked(t, noRows, b), movedFile(t, code, stepA, stepB)) {
		t.Error("two changes to one export were held by their two steps")
	}
}

// TestARemovedExportIsAStepToo: the step of a removal is held, and the record
// with no step is not.
func TestARemovedExportIsAStepToo(t *testing.T) {
	t.Parallel()
	shrunk := strings.Replace(code, "func Open(n int) (Clock, error) { return nil, nil }", "", 1)
	if !surface.Unrecorded(marked(t, noRows, shrunk), movedFile(t, code)) {
		t.Error("a removed export was held with no step")
	}
	if surface.Unrecorded(marked(t, noRows, shrunk), movedFile(t, code, step(t, code, shrunk))) {
		t.Error("a removed export was not held by its step")
	}
}

// TestTheRecordedFormIsTheOneTheFileHolds: with no base the old rule holds, so
// a base-less record whose newest digest is the listing's is held and one with
// steps refused; the listing renders each kind of line.
func TestTheRecordedFormIsTheOneTheFileHolds(t *testing.T) {
	t.Parallel()
	old := recorded(t, noRows, code)
	if surface.Unrecorded(marked(t, noRows, code), old) {
		t.Error("the old form no longer holds its listing")
	}
	mf := movedFile(t, code+extra, step(t, code+extra, code))
	if surface.Unrecorded(marked(t, noRows, code), mf) {
		t.Error("a base and a step did not hold their listing")
	}
	text := string(render(t, code, mf))
	for _, want := range []string{
		"\nListing sum: `" + surface.Sum(marked(t, noRows, code)) + "`\n",
		"\n- `sha256:" + strings.Repeat("a", 64) + "` an old line of the record\n",
		"\n- base `" + hex64(bigSum(marked(t, noRows, code+extra))) + "` the record moves to sums here\n",
		"\n- step `" + step(t, code+extra, code) + "` change 0 of the fixture\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the listing does not hold %q", want)
		}
	}
}

// TestAnExportRemovedAndAddedBackIsTheSameStepTwice: Extra is added, removed
// and added back as it was, so the record holds its step twice with the
// removal between, and is read and holds the listing. The controls: the same
// step twice with nothing between, a change counted twice, does not add up,
// and neither does the record without the last step.
func TestAnExportRemovedAndAddedBackIsTheSameStepTwice(t *testing.T) {
	t.Parallel()
	in, out := step(t, code, code+extra), step(t, code+extra, code)
	secs := marked(t, noRows, code+extra)
	if mf := movedFile(t, code, in, out, in); surface.Unrecorded(secs, mf) {
		t.Errorf("the record of an export added, removed and added back did not hold it: %s", surface.Why(secs, mf))
	}
	if !surface.Unrecorded(secs, movedFile(t, code, in, in)) {
		t.Error("a change counted twice held the listing")
	}
	if !surface.Unrecorded(secs, movedFile(t, code, in, out)) {
		t.Error("the record without the export's second step held the listing")
	}
}

// TestTheBaseIsKeptAsMainHasIt: once main's record has a base, a head that
// changes its number, drops it or never moved is refused, each naming main's
// base; a head that adds a step is held, so is a head that rewords the base's
// reason with its number as it was, and so is any head while main has not
// moved.
func TestTheBaseIsKeptAsMainHasIt(t *testing.T) {
	t.Parallel()
	stepA, stepB := step(t, code, code+extra), step(t, code, strings.Replace(code, openA, openB, 1))
	main := movedFile(t, code, stepA)
	mainBase := hex64(bigSum(marked(t, noRows, code)))
	if got := surface.BaseKept(main, movedFile(t, code, stepA, stepB)); got != "" {
		t.Fatalf("a head that adds a step was refused: %s", got)
	}
	reworded := noRows + "changes:\n  - {base: \"" + mainBase + "\", reason: \"the record moves to sums now\"}\n  - {step: \"" + stepA + "\", reason: \"change 0 of the fixture\"}\n"
	if got := surface.BaseKept(main, marksFile(t, reworded)); got != "" {
		t.Fatalf("a base with a new reason and its number as it was was refused: %s", got)
	}
	bad := map[string]*surface.MarkFile{
		"a base with a change taken into it": movedFile(t, code+extra),
		"a record with no base":              recorded(t, noRows, code),
		"no marks file":                      nil,
	}
	ran := 0
	for name, head := range bad {
		got := surface.BaseKept(main, head)
		if !strings.Contains(got, "main's has the base "+mainBase) && !strings.Contains(got, "not main's, the base "+mainBase) {
			t.Errorf("%s was not refused by main's base: %q", name, got)
		}
		ran++
	}
	if ran != len(bad) {
		t.Fatalf("ran %d of the %d plants", ran, len(bad))
	}
	for name, m := range map[string]*surface.MarkFile{"an old-form main": recorded(t, noRows, code), "no main record": nil} {
		if got := surface.BaseKept(m, movedFile(t, code+extra)); got != "" {
			t.Errorf("%s refused a head: %s", name, got)
		}
	}
}

// TestARecordOfSumsThatSaysNothingIsRefused: each way the new form can be
// wrong, with a control that is read.
func TestARecordOfSumsThatSaysNothingIsRefused(t *testing.T) {
	t.Parallel()
	d, b, s, z := "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("0", 64)
	line := func(kind, v string) string {
		return "  - {" + kind + ": \"" + v + "\", reason: \"a line of the record\"}\n"
	}
	good := "changes:\n" + line("digest", d) + line("base", b) + line("step", s)
	if _, err := surface.ParseMarks([]byte(good)); err != nil {
		t.Fatalf("the control was refused: %v", err)
	}
	bad := map[string]string{
		"a second base":          good + line("base", b),
		"a digest after a base":  good + line("digest", d),
		"a step before the base": "changes:\n" + line("step", s) + line("base", b),
		"a step of nothing":      good + line("step", z),
		"a step not hex":         good + line("step", strings.Repeat("g", 64)),
		"a step in upper case":   good + line("step", strings.Repeat("C", 64)),
		"a short base":           "changes:\n" + line("base", b[:63]),
		"a base with a prefix":   "changes:\n" + line("base", "sha256:"+b),
		"a step and a digest":    good + "  - {step: \"" + strings.Repeat("d", 64) + "\", digest: \"" + d + "\", reason: \"a line of the record\"}\n",
		"a line of no kind":      good + "  - {reason: \"a line of the record\"}\n",
		"a step with no reason":  good + "  - {step: \"" + strings.Repeat("d", 64) + "\"}\n",
		"a step with one word":   good + "  - {step: \"" + strings.Repeat("d", 64) + "\", reason: \"because\"}\n",
		"a step with a stranger": good + "  - {step: \"" + strings.Repeat("d", 64) + "\", reason: \"a line of the record\", by: me}\n",
	}
	ran := 0
	for name, text := range bad {
		if text == good {
			t.Fatalf("%s: the planted edit changed nothing", name)
		}
		if _, err := surface.ParseMarks([]byte(text)); !errors.Is(err, surface.ErrMarks) {
			t.Errorf("%s was read as a marks file: %v", name, err)
		}
		ran++
	}
	if ran != len(bad) {
		t.Fatalf("ran %d of the %d plants", ran, len(bad))
	}
	// A line of two kinds is refused as that, not only for a key its one kind
	// does not take.
	for _, name := range []string{"a step and a digest", "a line of no kind"} {
		_, err := surface.ParseMarks([]byte(bad[name]))
		if err == nil || !strings.Contains(err.Error(), "a line is one of digest, base or step") {
			t.Errorf("%s was not refused as a line of one kind: %v", name, err)
		}
	}
}

// TestAHandMadeRecordWithABadSumIsUnrecorded: a MarkFile built in code, not by
// ParseMarks, whose base is no number, holds nothing and says so.
func TestAHandMadeRecordWithABadSumIsUnrecorded(t *testing.T) {
	t.Parallel()
	mf := &surface.MarkFile{Changes: []surface.Entry{{Base: "not a number", Reason: "made by hand here"}}}
	secs := marked(t, noRows, code)
	if !surface.Unrecorded(secs, mf) {
		t.Error("a base that is no number held the listing")
	}
	if why := surface.Why(secs, mf); !strings.Contains(why, "not 64 hex digits") {
		t.Errorf("Why: %s", why)
	}
}

// TestAKeyWrittenTwiceIsRefused: git's union driver merges the marks file, and
// two PRs that remove one export leave one step with two reasons. The reader
// refuses that line rather than keep the last reason and drop the other.
func TestAKeyWrittenTwiceIsRefused(t *testing.T) {
	t.Parallel()
	b, s := strings.Repeat("b", 64), strings.Repeat("c", 64)
	merged := "changes:\n" +
		"  - base: \"" + b + "\"\n" +
		"    reason: \"the record moves to sums here\"\n" +
		"  - step: \"" + s + "\"\n" +
		"    reason: \"one change removes the export\"\n"
	if _, err := surface.ParseMarks([]byte(merged)); err != nil {
		t.Fatalf("the control was refused: %v", err)
	}
	bad := map[string]string{
		"line 6: the key reason is written again, first at line 5": merged + "    reason: \"another change removes it too\"\n",
		"line 3: the anchor &why": strings.Replace(merged, "reason: \"the record", "reason: &why \"the record", 1) +
			"  - step: \"" + strings.Repeat("d", 64) + "\"\n    reason: *why\n",
	}
	for want, text := range bad {
		_, err := surface.ParseMarks([]byte(text))
		if !errors.Is(err, surface.ErrMarks) || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseMarks error = %v, want %q", err, want)
		}
	}
}

// TestTheRepositorysMarksFileReads: the strict reader takes the record main
// holds.
func TestTheRepositorysMarksFileReads(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../" + surface.MarksFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := surface.ParseMarks(data); err != nil {
		t.Errorf("the repository's %s was refused: %v", surface.MarksFile, err)
	}
}
