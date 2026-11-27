package format

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// faulty is a statement the prose lint makes one finding of: it does not
// start with a verb.
const faulty = "Every error wraps its cause."

// migratedHistory is a history of one migrate entry recording statements,
// written by the record function as migrate's apply writes it.
func migratedHistory(t *testing.T, statements map[model.RuleID]model.Digest) string {
	t.Helper()
	got, err := recordWorld(t, "", MigrateEntry(migrateForm(), statements))(model.Yes{Who: "team-a", At: at(1, 9), Plan: planA})
	if err != nil {
		t.Fatal(err)
	}
	return string(got.Data) + "\n"
}

// holdCase loads the fixture with go.errors.1's statement and origin as given,
// history as its history.jsonl ("-" for none) and the digests handed in.
func holdCase(t *testing.T, statement string, origin model.Origin, history string, given map[model.RuleID]model.Digest) Loaded {
	t.Helper()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = "format: 3\npart: go\nconcern: errors\nrules:\n  - id: go.errors.1\n    statement: " + statement +
		"\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: " + string(origin) + "\n"
	files["language/go/rules/layout.yaml"] = "format: 3\npart: go\nconcern: layout\nrules:\n  - id: go.layout.1\n    statement: Keep a folder to one subject.\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n"
	for _, f := range []string{"exclusions.yaml", "translations/fr/errors.yaml", "decisions.yaml", "proposals.yaml"} {
		delete(files, f)
	}
	if history == "-" {
		delete(files, HistoryFile)
	} else {
		files[HistoryFile] = history
	}
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	p := w.Ports()
	got, err := LoadWith(p.Tree, p.Schema, w.Seed("repo", files), Options{Names: standInNames(t), Statements: given})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// proseFindings are the findings of l the prose lint made.
func proseFindings(l Loaded) []Finding {
	var out []Finding
	for _, f := range l.Findings {
		if f.Prose {
			out = append(out, f)
		}
	}
	return out
}

// TestProseHoldsAMigratedRuleWithTheRecordedDigest is the control: a migrated
// rule whose statement still has the digest the migrate entry recorded gets
// no prose finding, read from the history or handed in, and is counted held;
// planted beside it, the same fault on a reworded statement is a finding.
// Risk: the owner's 1,129 migrated rules held to a lint they never wrote for.
func TestProseHoldsAMigratedRuleWithTheRecordedDigest(t *testing.T) {
	t.Parallel()
	recorded := map[model.RuleID]model.Digest{"go.errors.1": StatementDigest(faulty)}
	cases := []struct {
		name, statement, history string
		given                    map[model.RuleID]model.Digest
		prose, held              int
	}{
		{"control: read from the history", faulty, migratedHistory(t, recorded), nil, 0, 1},
		{"control: handed in, as migrate's read-back does", faulty, "-", recorded, 0, 1},
		{"planted: reworded by hand, read from the history", "Every error wraps its cause, always.", migratedHistory(t, recorded), nil, 1, 0},
		{"planted: reworded, handed in", "Every error wraps its cause, always.", "-", recorded, 1, 0},
	}
	for _, c := range cases {
		got := holdCase(t, c.statement, model.Migrated, c.history, c.given)
		if p := proseFindings(got); len(p) != c.prose || got.Held != c.held {
			t.Errorf("%s: prose findings %v, held %d; want %d and %d", c.name, p, got.Held, c.prose, c.held)
		}
		if len(got.Findings) != c.prose {
			t.Errorf("%s: findings %v, want only the %d of the prose lint", c.name, got.Findings, c.prose)
		}
	}
}

// TestProseLintsARewordedMigratedRule: once the statement differs from what
// the conversion recorded, the rule is linted like every other; the reword is
// no finding of its own, and the origin stays migrated. Risk: a hold that
// outlives the text it was given for, so a hand edit skips the lint.
func TestProseLintsARewordedMigratedRule(t *testing.T) {
	t.Parallel()
	history := migratedHistory(t, map[model.RuleID]model.Digest{"go.errors.1": StatementDigest("Wrap an error with what was being done.")})
	got := holdCase(t, faulty, model.Migrated, history, nil)
	p := proseFindings(got)
	if len(p) != 1 || !strings.Contains(p[0].Msg, "which is not the imperative") || got.Held != 0 {
		t.Fatalf("prose findings %v, held %d; want the one finding of the fault and none held", p, got.Held)
	}
	if len(got.Findings) != 1 {
		t.Errorf("findings %v: the reword itself is a finding", got.Findings)
	}
	if r := got.Book.Rules[0]; r.ID != "go.errors.1" || r.Origin != model.Migrated {
		t.Errorf("rule %s has origin %q, want migrated", r.ID, r.Origin)
	}
	clean := holdCase(t, "Wrap an error with the step that failed.", model.Migrated, history, nil)
	if len(clean.Findings) != 0 || clean.Held != 0 {
		t.Errorf("a clean reword: findings %v, held %d; want none of either", clean.Findings, clean.Held)
	}
}

// TestProseHoldsNothingWithoutARecordedDigest: no history, a history with no
// migrate entry, a migrate entry with no digest for the rule or an empty
// one, and a rule whose origin is not migrated each leave the rule linted.
// Risk: a hold given on nothing read.
func TestProseHoldsNothingWithoutARecordedDigest(t *testing.T) {
	t.Parallel()
	recorded := map[model.RuleID]model.Digest{"go.errors.1": StatementDigest(faulty)}
	cases := []struct {
		name, history string
		origin        model.Origin
		given         map[model.RuleID]model.Digest
	}{
		{"no history", "-", model.Migrated, nil},
		{"a history with no migrate entry", fullBookHistory(t), model.Migrated, nil},
		{"a migrate entry that recorded no statement", migratedHistory(t, map[model.RuleID]model.Digest{}), model.Migrated, nil},
		{"a migrate entry with no digest for the rule", migratedHistory(t, map[model.RuleID]model.Digest{"go.layout.1": StatementDigest("Keep a folder to one subject.")}), model.Migrated, nil},
		{"an empty digest handed in", "-", model.Migrated, map[model.RuleID]model.Digest{"go.errors.1": ""}},
		{"an empty map handed in", migratedHistory(t, recorded), model.Migrated, map[model.RuleID]model.Digest{}},
		{"an authored rule with the digest recorded", migratedHistory(t, recorded), model.Authored, nil},
		{"an imported rule with the digest handed in", "-", model.Imported, recorded},
	}
	for _, c := range cases {
		got := holdCase(t, faulty, c.origin, c.history, c.given)
		if p := proseFindings(got); len(p) != 1 || got.Held != 0 {
			t.Errorf("%s: prose findings %v, held %d; want the fault found and none held", c.name, p, got.Held)
		}
	}
	// The schema refuses an origin missing, empty or of another value before
	// the lint is reached, so the hold's own test of the origin is read here.
	for claim, origin := range map[string]model.Origin{
		"no origin":                          "",
		"an origin the format does not have": "copied",
		"authored":                           model.Authored,
		"imported":                           model.Imported,
	} {
		p := proseItem{statement: true, text: faulty, rule: "go.errors.1", origin: origin}
		if held(p, recorded) {
			t.Errorf("%s (origin %q) is held", claim, origin)
		}
	}
	if held(proseItem{statement: true, text: faulty, origin: model.Migrated}, map[model.RuleID]model.Digest{model.RuleID(""): StatementDigest(faulty)}) {
		t.Error("a statement with no rule id is held")
	}
	if !held(proseItem{statement: true, text: faulty, rule: "go.errors.1", origin: model.Migrated}, recorded) {
		t.Error("the control is not held")
	}
}

// TestUndefinedTermIsCheckedOnAMigratedRule: the hold is the prose lint's
// alone, and the undefined-term check still reads a held statement, as does
// the names check. Risk: a migrated rule naming a term no one defined.
func TestUndefinedTermIsCheckedOnAMigratedRule(t *testing.T) {
	t.Parallel()
	statement := "Every error names {lint}, as alex asked."
	got := holdCase(t, statement, model.Migrated, "-", map[model.RuleID]model.Digest{"go.errors.1": StatementDigest(statement)})
	if got.Held != 1 || len(proseFindings(got)) != 0 {
		t.Fatalf("held %d, prose findings %v; want the rule held", got.Held, proseFindings(got))
	}
	var term, name bool
	for _, f := range got.Findings {
		term = term || strings.Contains(f.Msg, "the term {lint} is not defined")
		name = name || f.Msg == NamesMsg
	}
	if !term || !name || len(got.Findings) != 2 {
		t.Errorf("findings %v; want the undefined term and the name, and nothing else", got.Findings)
	}
}

// TestRecordedTakesTheLaterMigrateEntrysDigest: a rulebook migrated twice
// holds by the later entry's digest for a rule, and an entry of another kind
// records nothing. Risk: a hold read from a conversion that was replaced.
func TestRecordedTakesTheLaterMigrateEntrysDigest(t *testing.T) {
	t.Parallel()
	got := Recorded([]model.HistoryEntry{
		{Kind: "migrate", Statements: map[model.RuleID]model.Digest{"a.b.1": "sha256:1", "a.b.2": "sha256:2"}},
		{Kind: "promote", Statements: map[model.RuleID]model.Digest{"a.b.1": "sha256:9"}},
		{Kind: "migrate", Statements: map[model.RuleID]model.Digest{"a.b.1": "sha256:3"}},
	})
	if len(got) != 2 || got["a.b.1"] != "sha256:3" || got["a.b.2"] != "sha256:2" {
		t.Errorf("Recorded = %v", got)
	}
	if got := Recorded(nil); got == nil || len(got) != 0 {
		t.Errorf("Recorded(nil) = %#v, want an empty map", got)
	}
}

// fullBookHistory is the fixture's own history, which holds no migrate entry.
func fullBookHistory(t *testing.T) string {
	t.Helper()
	h, ok := fullBook(t)[HistoryFile]
	if !ok || strings.Contains(h, `"kind":"migrate"`) {
		t.Fatalf("the fixture's history is not one without a migrate entry: %q", h)
	}
	return h
}
