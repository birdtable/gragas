package rulebook

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/repo/gatetest"
)

// No active rule names a tool the rulebook has let go: a tools.yaml row whose state is replaced, by its key, or a name the
// register of libraries drops. A rule that still names one tells a reader to
// run a tool nobody runs. Two kinds of text may keep the name: a rule's
// sources, which quote what was measured with it, and an environment variable
// spelled NAME=, which the guards refuse whatever runs the hooks.

// proseField is one field of a rule a person reads as its words: the rule's id,
// the field's place and its text.
type proseField struct{ id, field, text string }

// activeProse is every field of every active rule that says what the rule is or how it is held: the
// statement and the reasoning's definition, pros and cons. A rule's sources are left out.
func activeProse(rules []gate.Rule) []proseField {
	var out []proseField
	add := func(id, field, text string) {
		if text != "" {
			out = append(out, proseField{id, field, text})
		}
	}
	for _, r := range rules {
		if r.Status != "active" {
			continue
		}
		add(r.ID, "statement", r.Fields.Str("statement"))
		why := r.Fields.Sub("reasoning")
		if why == nil {
			continue
		}
		add(r.ID, "reasoning.definition", why.Str("definition"))
		for _, k := range []string{"pros", "cons"} {
			for i, s := range ordered.Strings(why.List(k)) {
				add(r.ID, "reasoning."+k+"."+strconv.Itoa(i), s)
			}
		}
	}
	return out
}

// namings is each field that names one of names as a word of its own, in any
// case, as "id field: names name". A name followed by = is an environment
// variable, not the tool.
func namings(fields []proseField, names []string) []string {
	var out []string
	for _, n := range names {
		word := regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])(` + regexp.QuoteMeta(n) + `)(?:$|[^a-z0-9_-])`)
		for _, f := range fields {
			for _, m := range word.FindAllStringSubmatchIndex(f.text, -1) {
				if end := m[3]; end < len(f.text) && f.text[end] == '=' {
					continue
				}
				out = append(out, f.id+" "+f.field+": names "+n)
				break
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// replacedTools is the key of every row whose state is replaced, in every
// tools.yaml under rulebook/ of fsys.
func replacedTools(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(fsys, "rulebook", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Base(p) != "tools.yaml" {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		v, err := ordered.ParseStrict(data)
		if err != nil {
			return err
		}
		doc, ok := v.(*ordered.Map)
		if !ok {
			t.Fatalf("%s is not a map", p)
		}
		for _, row := range ordered.Maps(doc.List("tools")) {
			if row.Str("state") == "replaced" {
				out = append(out, row.Str("key"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestNoActiveRuleNamesARetiredOrDroppedTool reads the rule files (the rulebook
// in the format, or a converted copy of it before the switch) against every
// replaced tool and every dropped name.
//
// What it costs when this fails: a rule tells a project to run a tool the
// rulebook replaced, and the project fetches it.
func TestNoActiveRuleNamesARetiredOrDroppedTool(t *testing.T) {
	t.Parallel()
	names := replacedTools(t, os.DirFS(root))
	for _, d := range theRegister(t).Dropped {
		names = append(names, d.Name)
	}
	for _, want := range []string{"lefthook", "osv-scanner", "lychee", "multi-gitter"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s is neither a replaced tool nor a dropped name, so this test would not see it", want)
		}
	}
	rules, err := gate.ReadRules(gatetest.Of(t, root).Rulebook)
	if err != nil {
		t.Fatal(err)
	}
	fields := activeProse(rules)
	if len(fields) < 1000 {
		t.Fatalf("read %d prose fields of active rules, fewer than the rulebook holds", len(fields))
	}
	for _, f := range namings(fields, names) {
		t.Errorf("%s, a tool the rulebook replaced or dropped; name what replaced it, or retire the rule", f)
	}
}

// TestARuleNamingADroppedToolIsFoundAndItsSourcesAndVariablesAreNot is the
// plant and the control of the test above.
func TestARuleNamingADroppedToolIsFoundAndItsSourcesAndVariablesAreNot(t *testing.T) {
	t.Parallel()
	doc := `format: 3
part: docs
concern: links
rules:
  - id: docs.links.9
    statement: Links resolve.
    strength: must
    status: active
    reasoning:
      definition: Lychee --offline over docs.
      pros:
        - a hook run was timed
    sources:
      - {ref: lychee-readme, says: "lychee --offline, 0.16 s"}
  - id: docs.hooks.9
    statement: No LEFTHOOK=0 and no --no-verify.
    strength: must
    status: active
    reasoning:
      definition: the staged run of lefthook took 1.9 s
  - id: docs.links.8
    statement: lychee checks links.
    strength: must
    status: retired
  - id: docs.links.7
    statement: Links resolve.
    strength: must
    status: active
    reasoning:
      cons:
        - a prevalent fault, checked by a vale-free test
`
	rules, err := gate.ReadRules(fstest.MapFS{"base/docs/rules/links.yaml": {Data: []byte(doc)}})
	if err != nil {
		t.Fatal(err)
	}
	got := namings(activeProse(rules), []string{"lychee", "lefthook", "vale"})
	want := []string{"docs.hooks.9 reasoning.definition: names lefthook", "docs.links.9 reasoning.definition: names lychee"}
	if !slices.Equal(got, want) {
		t.Errorf("namings:\n got  %q\n want %q", got, want)
	}
}
