package coreapi_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// TestTheShippedLinterRowCountsWhatTheCountCheckCountedBefore:
// the readers file's one counted row gives the count check the ten exclusion
// lists and the directive word the audit held as a table before it, over the
// row's own files, so a repository's counts are what they were, and the
// presets list and the generated scalar's loosening value.
// The row holds them until the tool's plugin gives them in its manifest's
// counted.
// Risk: an exclusion list or the directive drops out of the count unseen, and
// a new exclusion lands with nothing to say so.
func TestTheShippedLinterRowCountsWhatTheCountCheckCountedBefore(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../host/assets/audit/readers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r := readersFrom(t, string(data))
	if r.Refused != "" {
		t.Fatalf("the shipped readers file is refused: %s", r.Refused)
	}
	got := coreapi.Countings(r.Rows)
	want := linterCounting()[0]
	sameLoosening := func(a, b coreapi.Loosening) bool {
		return slices.Equal(a.At, b.At) && slices.Equal(a.Values, b.Values) && (a.Unless == nil) == (b.Unless == nil) && (a.Unless == nil || *a.Unless == *b.Unless)
	}
	if len(got) != 1 || !slices.EqualFunc(got[0].Exclusions, want.Exclusions, slices.Equal) || got[0].Directive != want.Directive ||
		!slices.EqualFunc(got[0].Loosening, want.Loosening, sameLoosening) {
		t.Fatalf("Countings = %+v, want one with %+v", got, want)
	}
	if !got[0].FoldKeys {
		t.Errorf("the shipped row reads keys as written; its tool lower-cases them, so fold_keys is true")
	}
	if i := slices.IndexFunc(r.Rows, func(row coreapi.Reader) bool { return row.Directive != "" }); !slices.Equal(got[0].Configs, r.Rows[i].Files) {
		t.Errorf("the counted configs are %v, want the row's own files %v", got[0].Configs, r.Rows[i].Files)
	}
}

// TestARowsCountedIsHeldToTheManifestsShape: counted is a mapping of
// exclusions and directive alone, in the manifest's limits; anything else
// refuses the file whole, naming the row.
func TestARowsCountedIsHeldToTheManifestsShape(t *testing.T) {
	t.Parallel()
	row := func(counted string) string {
		return "readers:\n  - tool: a\n    files: [a.yml]\n    kind: entries\n    counted:" + counted + "\n"
	}
	ok := readersFrom(t, row("\n      exclusions: [[issues, exclude-dirs], [run, skip_files]]\n      loosening: [{at: [a, b], values: [lax, loose-2]}]\n      directive: no-lint2"))
	if ok.Refused != "" || len(ok.Rows) != 1 || ok.Rows[0].Directive != "no-lint2" || len(ok.Rows[0].Exclusions) != 2 ||
		len(ok.Rows[0].Loosening) != 1 || !slices.Equal(ok.Rows[0].Loosening[0].Values, []string{"lax", "loose-2"}) {
		t.Fatalf("a whole counted: %+v", ok)
	}
	many := "[" + strings.Repeat("{at: [a], values: [x]}, ", 16) + "{at: [b], values: [x]}]"
	long := "[" + strings.Repeat("[a], ", 64) + "[b]]"
	for name, tc := range map[string]struct{ counted, want string }{
		"not a mapping":                {" [x]", "counted is not a mapping"},
		"empty":                        {" {}", "counted is not a mapping"},
		"another key":                  {"\n      exclusions: [[a]]\n      paths: [[b]]", `the key "paths"`},
		"a capital in a key":           {"\n      exclusions: [[Issues, exclude]]", "has a key that is not"},
		"a key that is a path":         {"\n      exclusions: [[issues/exclude]]", "has a key that is not"},
		"a path that is a word":        {"\n      exclusions: [issues]", "is not a list of one to 16 keys"},
		"no paths":                     {"\n      exclusions: []", "holds 0 paths"},
		"65 paths":                     {"\n      exclusions: " + long, "holds 65 paths"},
		"a path twice":                 {"\n      exclusions: [[a, b], [a, b]]", "is a.b a second time"},
		"a directive of two":           {"\n      directive: no lint", "counted.directive is not a word"},
		"a directive in caps":          {"\n      directive: NoLint", "counted.directive is not a word"},
		"a numeric directive":          {"\n      directive: 7", "counted.directive is not a word"},
		"no loosening":                 {"\n      loosening: []", "counted.loosening holds 0 scalars"},
		"17 loosenings":                {"\n      loosening: " + many, "counted.loosening holds 17 scalars"},
		"a loosening of a path":        {"\n      loosening: [[a, b]]", "counted.loosening[0] is not a mapping of at and one of values and unless"},
		"a loosening, no values":       {"\n      loosening: [{at: [a]}]", "counted.loosening[0] is not a mapping of at and one of values and unless"},
		"a loosening, a third key":     {"\n      loosening: [{at: [a], values: [x], why: y}]", "counted.loosening[0] is not a mapping of at and one of values and unless"},
		"a capital in at":              {"\n      loosening: [{at: [A], values: [x]}]", "counted.loosening[0].at has a key that is not"},
		"no values":                    {"\n      loosening: [{at: [a], values: []}]", "values holds 0 words"},
		"a value twice":                {"\n      loosening: [{at: [a], values: [x, x]}]", "a value that is not a word given once"},
		"a value of two words":         {"\n      loosening: [{at: [a], values: [lax mode]}]", "a value that is not a word given once"},
		"fold_keys that is no boolean": {"\n      exclusions: [[a]]\n      fold_keys: yes-please", "counted.fold_keys is not true or false"},
		"fold_keys alone":              {"\n      fold_keys: true", "counted holds none of exclusions, loosening and directive, so it counts nothing"},
		"values and unless":            {"\n      loosening: [{at: [a], values: [x], unless: \"\"}]", "is not a mapping of at and one of values and unless"},
		"an unless that is a list":     {"\n      loosening: [{at: [a], unless: []}]", "counted.loosening[0].unless is not a default's text"},
		"an unless of capitals":        {"\n      loosening: [{at: [a], unless: None}]", "counted.loosening[0].unless is not a default's text"},
		"an unless of a spaced []":     {"\n      loosening: [{at: [a], unless: \"[ ]\"}]", "counted.loosening[0].unless is not a default's text"},
		"a value that is a boolean":    {"\n      loosening: [{at: [a], values: [false]}]", "a value that is not a word given once"},
		"a loosening path twice":       {"\n      loosening: [{at: [a], values: [x]}, {at: [a], values: [y]}]", "counted.loosening[1] is a a second time"},
	} {
		r := readersFrom(t, row(tc.counted))
		if len(r.Rows) != 0 || !strings.Contains(r.Refused, "row 1: ") || !strings.Contains(r.Refused, tc.want) {
			t.Errorf("%s: %+v, want the file refused naming %q", name, r, tc.want)
		}
	}
}

// TestARowsLooseningReadsBothForms: values of texts, false and 0
// among them, and unless with a text, empty, or [] for an empty list.
func TestARowsLooseningReadsBothForms(t *testing.T) {
	t.Parallel()
	r := readersFrom(t, "readers:\n  - tool: a\n    files: [a.yml]\n    kind: entries\n    counted:\n      loosening:\n"+
		"        - {at: [run, tests], values: [\"false\"]}\n"+
		"        - {at: [run, code], values: [\"0\"]}\n"+
		"        - {at: [issues, rev], unless: \"\"}\n"+
		"        - {at: [keep], unless: \"[]\"}\n"+
		"        - {at: [mode], unless: strict}\n")
	if r.Refused != "" || len(r.Rows) != 1 {
		t.Fatalf("both forms: %+v", r)
	}
	l := r.Rows[0].Loosening
	if len(l) != 5 || l[0].Values[0] != "false" || l[1].Values[0] != "0" || l[0].Unless != nil ||
		l[2].Unless == nil || *l[2].Unless != "" || *l[3].Unless != "[]" || *l[4].Unless != "strict" || l[4].Values != nil {
		t.Errorf("Loosening = %+v", l)
	}
}
