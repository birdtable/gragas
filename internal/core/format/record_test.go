package format

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func at(day, hour int) int64 { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC).Unix() }

const planA = model.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")

// recordWorld loads the fixture book with history as its history.jsonl; the
// record function is made from the world's own ports.
func recordWorld(t *testing.T, history string, e Entry) func(model.Yes) (model.Target, error) {
	t.Helper()
	files := bareBook()
	if history != "" {
		files[HistoryFile] = history
	}
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	return HistoryRecord(p.Tree, p.Schema, root, got, e)
}

func initEntry() Entry {
	return Entry{Kind: "init", Entity: "rule:go.errors.1", After: model.Lifecycle{Status: "proposed"}}
}

func promoteEntry() Entry {
	return Entry{Kind: "promote", Entity: "rule:go.errors.1", Before: &model.Lifecycle{Status: "proposed"},
		After: model.Lifecycle{Status: "active", ReviewBy: "2027-01-31"}, Decision: "wrap-errors"}
}

const (
	initLine    = `{"after":{"status":"proposed"},"entity":"rule:go.errors.1","format":3,"kind":"init","number":1,"plan":"sha256:2222222222222222222222222222222222222222222222222222222222222222","when":"2026-11-01T09:00:00Z","who":"team-a"}`
	promoteLine = `{"after":{"review_by":"2027-01-31","status":"active"},"before":{"status":"proposed"},"decision":"wrap-errors","entity":"rule:go.errors.1","format":3,"kind":"promote","number":2,"plan":"sha256:2222222222222222222222222222222222222222222222222222222222222222","previous":"PREVIOUS","when":"2026-11-02T09:00:00Z","who":"team-a"}`
)

func TestTheFirstLineOfARulebookHasNoPreviousDigest(t *testing.T) {
	t.Parallel()
	rec := recordWorld(t, "", initEntry())
	got, err := rec(model.Yes{Who: "team-a", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != initLine || !got.Append || got.Path != HistoryFile || got.Before != "" {
		t.Errorf("target = %+v\n%s", got, got.Data)
	}
}

func TestALineNamesTheDigestOfTheLineBeforeIt(t *testing.T) {
	t.Parallel()
	// the digest of the bytes of initLine, taken with shasum -a 256
	const prev = "sha256:10304e6ceed344911ae5a174e6db7e765fd04abf20528757248b37a30bf2a1a9"
	rec := recordWorld(t, initLine+"\n", promoteEntry())
	got, err := rec(model.Yes{Who: "team-a", At: at(2, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(promoteLine, "PREVIOUS", prev, 1); string(got.Data) != want || got.Before == "" {
		t.Errorf("line = %s\nwant   %s\nbefore = %q", got.Data, want, got.Before)
	}
}

// TestPreviousIsTheCanonicalDigestOfTheLineBeforeOverAChain builds a chain of
// lines, each from the file the one before left, and holds every previous to
// the digest of the canonical value of the line it names.
func TestPreviousIsTheCanonicalDigestOfTheLineBeforeOverAChain(t *testing.T) {
	t.Parallel()
	history := ""
	var lines []string
	for i := range 30 {
		entry := Entry{Kind: "promote", Entity: fmt.Sprintf("rule:go.errors.%d", i), After: model.Lifecycle{Status: "active"}}
		got, err := recordWorld(t, history, entry)(model.Yes{Who: "team-a", At: at(1+i%27, i%24), Plan: planA})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(got.Data))
		history += string(got.Data) + "\n"
	}
	checked := 0
	for i, line := range lines {
		v, err := canon.Decode([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		o := v.(canon.Object)
		if n := int(o["number"].(canon.Int)); n != i+1 {
			t.Errorf("line %d has number %d", i+1, n)
		}
		if i == 0 {
			if _, has := o["previous"]; has {
				t.Error("the first line has a previous digest")
			}
			continue
		}
		pv, _ := canon.Decode([]byte(lines[i-1]))
		d, _ := canon.DigestOf(pv)
		if o["previous"] != canon.String(d.String()) {
			t.Errorf("line %d names %v; the digest of the line before is %s", i+1, o["previous"], d)
		}
		checked++
	}
	if checked != 29 {
		t.Errorf("checked %d links; want 29", checked)
	}
}

func TestANumberSharedAcrossBranchesIsNotAFindingAndTheNextTakesTheHighestPlusOne(t *testing.T) {
	t.Parallel()
	two := strings.Replace(promoteLine, "PREVIOUS", "sha256:"+strings.Repeat("1", 64), 1)
	other := strings.Replace(two, "wrap-errors", "other", 1)
	history := initLine + "\n" + two + "\n" + other + "\n"
	got, err := recordWorld(t, history, promoteEntry())(model.Yes{Who: "team-a", At: at(3, 9), Plan: planA})
	if err != nil || !strings.Contains(string(got.Data), "\"number\":3,") {
		t.Errorf("line = %s, %v; want number 3", got.Data, err)
	}
}

func TestTwoRendersWithTheSameInputsAreTheSameBytesAndTheFirstCallNeedsNoPlanDigest(t *testing.T) {
	t.Parallel()
	rec := recordWorld(t, initLine+"\n", promoteEntry())
	y := model.Yes{Who: "team-a", At: at(2, 9), Plan: planA}
	a, err1 := rec(y)
	b, err2 := rec(y)
	if err1 != nil || err2 != nil || string(a.Data) != string(b.Data) {
		t.Fatalf("two renders differ: %s | %s (%v, %v)", a.Data, b.Data, err1, err2)
	}
	y.Plan = ""
	first, err := rec(y)
	if err != nil || first.Path != a.Path || first.Before != a.Before || first.Append != a.Append || !strings.Contains(string(first.Data), strings.Repeat("0", 64)) {
		t.Errorf("the first call = %+v, %v", first, err)
	}
}

func TestALineIsOneWriteWithNoRawNewlineInAnyField(t *testing.T) {
	t.Parallel()
	e := Entry{Kind: "init", Entity: "rule:a\nb\r.1", After: model.Lifecycle{Owner: "x\ny"}, Decision: "d"}
	got, err := recordWorld(t, "", e)(model.Yes{Who: "a\n\"b\"\r<c>", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(got.Data), "\r\n") {
		t.Fatalf("the line holds a raw newline: %q", got.Data)
	}
	v, err := canon.Decode(got.Data)
	if err != nil || v.(canon.Object)["who"] != canon.String("a\n\"b\"\r<c>") {
		t.Errorf("the line does not read back: %v, %v", v, err)
	}
}

func TestAnEntryThatCannotBeALineIsRefusedByWhatIsWrong(t *testing.T) {
	t.Parallel()
	good := model.Yes{Who: "team-a", At: at(1, 9), Plan: planA}
	cases := []struct {
		name, history string
		e             Entry
		y             model.Yes
		want          string
	}{
		{"a kind that is not a slug", "", Entry{Kind: "Promote it", Entity: "rule:x.y.1", After: model.Lifecycle{Status: "active"}}, good, "not a slug"},
		{"no lifecycle after", "", Entry{Kind: "init", Entity: "rule:x.y.1"}, good, "no lifecycle field"},
		{"no entity", "", Entry{Kind: "init", After: model.Lifecycle{Status: "active"}}, good, "names its entity"},
		{"nobody", "", initEntry(), model.Yes{At: 1, Plan: planA}, "who made it"},
		{"a last line that is not JSON", initLine + "\n{not json\n", initEntry(), good, "history.jsonl:2:"},
		{"a line with no number", "{\"kind\":\"x\"}\n", initEntry(), good, "history.jsonl:1: the line has no number"},
	}
	ran := 0
	for _, c := range cases {
		_, err := recordWorld(t, c.history, c.e)(c.y)
		ran++
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want one holding %q", c.name, err, c.want)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
}

// TestTheLineIsTheLastLineOfTheFileOnceAppended is the integration with the
// fake: the target is an append, so the file's last line is the plan's line,
// and the next record chains onto it.
func TestTheLineIsTheLastLineOfTheFileOnceAppended(t *testing.T) {
	t.Parallel()
	files := bareBook()
	files[HistoryFile] = initLine + "\n"
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	y := model.Yes{Who: "team-a", At: at(2, 9), Plan: planA}
	tg, err := HistoryRecord(p.Tree, p.Schema, root, got, promoteEntry())(y)
	if err != nil || !tg.Append {
		t.Fatalf("target = %+v, %v", tg, err)
	}
	if err := p.Writer.Append(root, tg.Path, tg.Data); err != nil {
		t.Fatal(err)
	}
	data, err := p.Tree.Read(root, HistoryFile)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(all) != 2 || all[1] != string(tg.Data) || !strings.HasSuffix(string(data), "\n") {
		t.Errorf("file = %q", data)
	}
	if tg.Before != digestOf([]byte(initLine+"\n")) {
		t.Errorf("before = %s", tg.Before)
	}
	reloaded, err := load(t, w, root)
	bad := 0
	for _, f := range reloaded.Findings {
		if f.File == HistoryFile {
			bad++
		}
	}
	if err != nil || bad != 0 || len(reloaded.Book.History) != 2 {
		t.Errorf("the book with the new line: %v, %v", reloaded.Findings, err)
	}
}

func FuzzHistoryRecord(f *testing.F) {
	f.Add(initLine+"\n", "team-a")
	f.Add("", "a\nb")
	f.Add("{\"number\":9007199254740993}\n", "x")
	f.Add("[1]\n\n  \n", "")
	f.Fuzz(func(t *testing.T, history, who string) {
		files := bareBook()
		files[HistoryFile] = history
		w := porttest.NewWorld(t)
		w.SeedSchemas(schemaNames()...)
		root := w.Seed("repo", files)
		p := w.Ports()
		got, err := Load(p.Tree, p.Schema, root)
		if err != nil {
			return
		}
		tg, err := HistoryRecord(p.Tree, p.Schema, root, got, initEntry())(model.Yes{Who: who, At: 0, Plan: planA})
		if err == nil && strings.ContainsAny(string(tg.Data), "\r\n") {
			t.Fatalf("a line with a newline in it: %q", tg.Data)
		}
	})
}

// bareBook is the identity and the structure alone: a comment after the
// structure keeps the world from adding the fixture's own files, history
// included.
func bareBook() map[string]string {
	files := fixtureBook()
	files[StructureFile] += "# bare\n"
	return files
}

func TestTheNextNumberIsTheHighestInTheFilePlusOneWhateverTheOrder(t *testing.T) {
	t.Parallel()
	two := strings.Replace(promoteLine, "PREVIOUS", "sha256:"+strings.Repeat("1", 64), 1)
	third := strings.Replace(strings.Replace(two, "\"number\":2,", "\"number\":3,", 1), "wrap-errors", "other", 1)
	again := strings.Replace(two, "wrap-errors", "again", 1)
	history := initLine + "\n" + third + "\n" + again + "\n"
	got, err := recordWorld(t, history, promoteEntry())(model.Yes{Who: "team-a", At: at(3, 9), Plan: planA})
	if err != nil || !strings.Contains(string(got.Data), "\"number\":4,") {
		t.Errorf("line = %s, %v; want number 4 after 1, 3, 2", got.Data, err)
	}
}

// TestSeveralEntriesAreOneTargetNumberedAfterTheFileAndChainedThroughEachOther:
// a plan with three entries makes one target of three lines, numbered one after
// another from the highest in the file, each naming the digest of the line before
// it, the first the file's last; written in one append the book loads with every
// line and no finding, so the chain verifies; the entries share the yes and its
// plan.
// Planted failure: the lines are all numbered and chained from the file's last, or
// each is a target of its own.
func TestSeveralEntriesAreOneTargetNumberedAfterTheFileAndChainedThroughEachOther(t *testing.T) {
	t.Parallel()
	files := bareBook()
	files[HistoryFile] = initLine + "\n"
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	y := model.Yes{Who: "team-a", At: at(2, 9), Plan: planA}
	entries := []Entry{
		{Kind: "edit", Entity: "source:effective-go", After: model.Lifecycle{Status: "active"}},
		{Kind: "reference", Entity: "rule:go.errors.1", After: model.Lifecycle{Status: "proposed"}},
		{Kind: "reference", Entity: "rule:go.errors.2", After: model.Lifecycle{Status: "active"}},
	}
	tg, err := HistoryRecord(p.Tree, p.Schema, root, got, entries...)(y)
	if err != nil || !tg.Append || tg.Path != HistoryFile || tg.Before != digestOf([]byte(initLine+"\n")) {
		t.Fatalf("target = %+v, %v", tg, err)
	}
	lines := strings.Split(string(tg.Data), "\n")
	if len(lines) != 3 {
		t.Fatalf("the target holds %d lines, want 3:\n%s", len(lines), tg.Data)
	}
	prev := initLine
	for i, l := range lines {
		v, err := canon.Decode([]byte(l))
		if err != nil {
			t.Fatal(err)
		}
		o := v.(canon.Object)
		d, err := canon.DigestOf(mustDecode(t, prev))
		if err != nil {
			t.Fatal(err)
		}
		if n := o["number"].(canon.Int); int(n) != i+2 || o["previous"] != canon.String(d.String()) || o["plan"] != canon.String(string(planA)) || o["who"] != canon.String("team-a") {
			t.Errorf("line %d = %s: want number %d, previous %s, the yes's plan and who", i+1, l, i+2, d)
		}
		prev = l
	}
	if err := p.Writer.Append(root, tg.Path, tg.Data); err != nil {
		t.Fatal(err)
	}
	reloaded, err := load(t, w, root)
	bad := 0
	for _, f := range reloaded.Findings {
		if f.File == HistoryFile {
			bad++
		}
	}
	if err != nil || bad != 0 || len(reloaded.Book.History) != 4 {
		t.Errorf("the book with the three lines: %v (%d history entries), %v", reloaded.Findings, len(reloaded.Book.History), err)
	}
}

func mustDecode(t *testing.T, line string) canon.Value {
	t.Helper()
	v, err := canon.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestARecordOfNoEntryIsAnError(t *testing.T) {
	t.Parallel()
	files := bareBook()
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	if _, err := HistoryRecord(p.Tree, p.Schema, root, got)(model.Yes{Who: "team-a", At: at(2, 9), Plan: planA}); err == nil {
		t.Error("a plan that records itself with no entry made a line")
	}
}
