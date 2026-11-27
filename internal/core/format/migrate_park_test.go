package format

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// parkedAt is the value the parked file of m holds for subject at path, nil
// when it holds none. It fails the test unless m writes the parked file once,
// headed by its first line.
func parkedAt(t *testing.T, m Migration, subject string, path ...string) any {
	t.Helper()
	var text []string
	for _, tg := range m.Targets {
		if tg.Path == ParkedFile {
			text = append(text, string(tg.Data))
		}
	}
	if len(text) != 1 {
		t.Fatalf("the migration writes %s %d times, want once", ParkedFile, len(text))
	}
	head, body, _ := strings.Cut(text[0], "\n")
	if head != parkedHead {
		t.Fatalf("the parked file starts %q, want %q", head, parkedHead)
	}
	v, err := ordered.ParseYAML([]byte(body))
	if err != nil {
		t.Fatalf("the parked file does not read back: %v\n%s", err, text[0])
	}
	m2, _ := v.(*ordered.Map)
	var at any = m2.Sub(subject)
	for _, k := range path {
		sub, ok := at.(*ordered.Map)
		if !ok || sub == nil {
			return nil
		}
		at = sub.Val(k)
	}
	if sub, ok := at.(*ordered.Map); ok && sub == nil {
		return nil
	}
	return at
}

// compact is v as JSON on one line, for a comparison.
func compact(v any) string {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(ordered.JSON(v, 0))); err != nil {
		return ordered.JSON(v, 0)
	}
	return b.String()
}

// TestMigrateParksTheFixWithItsToolAndKey: the old check's fix is an enum
// (auto, suggest or never) that names no fixer program, so the binding gets no
// remedy and the value is parked beside the check's tool and key.
func TestMigrateParksTheFixWithItsToolAndKey(t *testing.T) {
	t.Parallel()
	for _, fix := range []string{"auto", "suggest", "never"} {
		t.Run(fix, func(t *testing.T) {
			t.Parallel()
			rule := strings.Replace(string(oneRule("Hard", "active", "go", "fact", "1")), `        tier: "1"`+"\n", `        tier: "1"`+"\n        fix: "+fix+"\n", 1)
			m, err := migrateRule(t, []byte(rule))
			if err != nil {
				t.Fatal(err)
			}
			if out := string(m.Targets[0].Data); strings.Contains(out, "remedy") || !strings.Contains(out, "key: depguard") {
				t.Errorf("the binding is not written without a remedy:\n%s", out)
			}
			if got := compact(parkedAt(t, m, "arch.layers.1", "checks", "1")); got != `{"tool":"golangci-lint","key":"depguard","fix":"`+fix+`"}` {
				t.Errorf("parked check = %s, want its tool, key and fix", got)
			}
		})
	}
}

// TestMigrateWritesOneParkedFileInTheOldOrder: one file, in the rulebook's
// folder at the name the loader does not read, its first line saying what it
// is and that no program reads it, its line count in the plan, and each
// subject's fields in the order of the old file.
func TestMigrateWritesOneParkedFileInTheOldOrder(t *testing.T) {
	t.Parallel()
	m, err := MigrateV2(v2Fixture(), migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(parkedHead, "No program reads this file.") || !strings.HasPrefix(parkedHead, "# ") {
		t.Errorf("the first line is %q, want a comment ending in that no program reads it", parkedHead)
	}
	text := string(m.Targets[len(m.Targets)-1].Data)
	if m.Parked.Path != ParkedFile || m.Parked.Lines != strings.Count(text, "\n") || m.Parked.Lines == 0 {
		t.Errorf("Parked = %+v, want %s and its %d lines", m.Parked, ParkedFile, strings.Count(text, "\n"))
	}
	// The loader reads no file of the parked file's name: one that is not
	// YAML at all is no finding beside a full book, where the same text in a
	// file it reads is one (the planted control).
	for name, want := range map[string]bool{ParkedFile: false, TermsFile: true} {
		files := fullBook(t)
		files[name] = "a: [\n"
		w := porttest.NewWorld(t)
		w.SeedSchemas(schemaNames()...)
		p := w.Ports()
		got, err := LoadWith(p.Tree, p.Schema, w.Seed("repo", files), Options{Names: standInNames(t)})
		if err != nil {
			t.Fatal(err)
		}
		if found := slices.ContainsFunc(got.Findings, func(f Finding) bool { return f.File == name }); found != want {
			t.Errorf("%s not YAML: a finding on it is %v, want %v (%v)", name, found, want, got.Findings)
		}
	}
	rule := strings.Replace(v2Rules, "    slug: no-cycles\n    since: 0.1.0\n", "    review_question: q?\n    since: 0.1.0\n    slug: no-cycles\n", 1)
	v := v2Fixture()
	v["base/arch/rules/layers.yaml"] = []byte(rule)
	m, err = MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := parkedAt(t, m, "arch.layers.1").(*ordered.Map)
	if sub == nil || !slices.Equal(sub.Keys(), []string{"review_question", "since", "slug"}) {
		t.Errorf("arch.layers.1 is parked as %s, want its fields in the old file's order", compact(sub))
	}
}

// TestMigrateParksTheOldStrengthWordWhereSeveralFoldIntoOne: Hard alone maps
// to must and is not parked; three words fold into should and two into may, so
// each of those is parked beside the rule id. The counts per strength are in
// the migration, for the plan's one line.
func TestMigrateParksTheOldStrengthWordWhereSeveralFoldIntoOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		old    string
		parked any
	}{{"Hard", nil}, {"Answered", "Answered"}, {"Session", "Session"}, {"Record", "Record"}, {"Lens", "Lens"}, {"Review", "Review"}} {
		t.Run(c.old, func(t *testing.T) {
			t.Parallel()
			m, err := migrateRule(t, oneRule(c.old, "active", "go", "fact", "1"))
			if err != nil {
				t.Fatal(err)
			}
			if got := parkedAt(t, m, "arch.layers.1", "strength"); got != c.parked {
				t.Errorf("parked strength = %v, want %v", got, c.parked)
			}
		})
	}
	m, err := MigrateV2(v2Fixture(), migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range m.Strengths {
		got = append(got, strings.Join(s.From, "+")+">"+s.To+"="+strings.Repeat("|", s.Rules))
	}
	if strings.Join(got, " ") != "Hard>must=| Answered+Session+Record>should=| Lens+Review>may=" {
		t.Errorf("Strengths = %v, want Hard to must for 1 rule, Record's fold to should for 1, none to may", got)
	}
}
