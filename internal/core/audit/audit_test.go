package audit_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/audit"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// toml is a stand-in reader: the core links no TOML library, so the test
// hands the check the document it would have parsed.
func toml(doc map[string]any) audit.TOML {
	return func([]byte) (map[string]any, error) { return doc, nil }
}

func TestNoConfigFilesGivesFindingsOnlyForTheBoundRules(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"supply.secrets.1", "gitleaks", "extend.useDefault"}, {"gates.shared.1", "task", "gotestsum"}}
	got := audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: fstest.MapFS{}, Bindings: b, TOML: toml(nil)})
	if len(got.Findings) != 2 || len(got.NotChecked) != 0 {
		t.Fatalf("%+v, want two findings and nothing else", got)
	}
	none := audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: fstest.MapFS{}, TOML: toml(nil)})
	if len(none.Findings) != 0 {
		t.Errorf("no binding gave %v", none.Findings)
	}
}

func TestGitleaksExtendsTheDefaultsAndAllowsNoPaths(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"supply.secrets.1", "gitleaks", "extend.useDefault"}, {"supply.secrets.4", "gitleaks", "allowlist.paths"}}
	repo := fstest.MapFS{".gitleaks.toml": {Data: []byte("x")}}
	run := func(doc map[string]any) []audit.Finding {
		return audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: repo, Bindings: b, TOML: toml(doc)}).Findings
	}
	good := map[string]any{"extend": map[string]any{"useDefault": true}}
	if f := run(good); len(f) != 0 {
		t.Errorf("good config: %v", f)
	}
	if f := run(map[string]any{}); len(f) != 1 || f[0].Rule != "supply.secrets.1" {
		t.Errorf("no extend: %v", f)
	}
	paths := map[string]any{"extend": map[string]any{"useDefault": true}, "allowlist": map[string]any{"paths": []any{"vendor/"}}}
	if f := run(paths); len(f) != 1 || f[0].Rule != "supply.secrets.4" {
		t.Errorf("allowlist paths: %v", f)
	}
	scoped := map[string]any{"extend": map[string]any{"useDefault": true}, "allowlists": []any{map[string]any{"paths": []any{"a"}, "targetRules": []any{"r"}}}}
	if f := run(scoped); len(f) != 0 {
		t.Errorf("a path scoped to a rule is not a default allowlist: %v", f)
	}
}

func TestTheTaskfileNeedsATaskForEachMember(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"gates.shared.1", "task", "vet"}, {"gates.shared.2", "task", "gotestsum"}}
	repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  vet: {cmds: [go vet ./...]}\n")}}
	got := audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: repo, Bindings: b}).Findings
	if len(got) != 1 || got[0].Key != "gotestsum" || got[0].File != "Taskfile.yml" {
		t.Errorf("%v, want one finding for gotestsum", got)
	}
}

// sources is the readers table as the layer above hands it in: the tools the
// check reads, their files and their kinds of read.
func sources() []audit.Source {
	return []audit.Source{
		{Tool: "standin", Files: standinFiles(), Kind: audit.KindEntries},
		{Tool: "gitleaks", Files: []string{".gitleaks.toml"}, Kind: audit.KindTOMLKeys},
		{Tool: "task", Files: []string{"Taskfile.yml", "Taskfile.yaml"}, Kind: audit.KindTaskNames},
	}
}

// TestASourceOfAKindNoReaderKnowsIsNotChecked: a row of the readers data that
// names an unknown kind is a reason, never a pass.
func TestASourceOfAKindNoReaderKnowsIsNotChecked(t *testing.T) {
	t.Parallel()
	got := audit.Check(t.Context(), audit.Input{
		Repo: fstest.MapFS{}, Bindings: []audit.Binding{{"x.y.1", "other", "k"}},
		Sources: []audit.Source{{Tool: "other", Files: []string{"other.cfg"}, Kind: "unheard-of"}},
	})
	if len(got.Findings) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].Why, "unheard-of") {
		t.Errorf("Check = %+v, want one not-checked naming the kind", got)
	}
}

// TestAGitleaksKeyInAnotherCaseIsReadAsTheToolReadsIt: gitleaks ignores the
// case of a key, so an [Allowlist] with paths is the finding, and an [Extend]
// with UseDefault is the extension. Two keys of one table that fold to one are
// not checked.
func TestAGitleaksKeyInAnotherCaseIsReadAsTheToolReadsIt(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"supply.secrets.1", "gitleaks", "extend.useDefault"}, {"supply.secrets.4", "gitleaks", "allowlist.paths"}}
	repo := fstest.MapFS{".gitleaks.toml": {Data: []byte("x")}}
	run := func(doc map[string]any) audit.Result {
		return audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: repo, Bindings: b, TOML: toml(doc)})
	}
	if f := run(map[string]any{"Extend": map[string]any{"UseDefault": true}, "Allowlist": map[string]any{"Paths": []any{"secret.txt"}}}).Findings; len(f) != 1 || f[0].Rule != "supply.secrets.4" {
		t.Errorf("[Allowlist] paths with [Extend] UseDefault: %v, want the one allowlist finding", f)
	}
	if f := run(map[string]any{"EXTEND": map[string]any{"USEDEFAULT": true}}).Findings; len(f) != 0 {
		t.Errorf("[EXTEND] USEDEFAULT: %v, want the extension read", f)
	}
	if f := run(map[string]any{"allowlists": []any{map[string]any{"Paths": []any{"a"}, "TargetRules": []any{"r"}}}, "extend": map[string]any{"useDefault": true}}).Findings; len(f) != 0 {
		t.Errorf("a scoped allowlist in another case: %v, want none", f)
	}
	dup := run(map[string]any{"allowlist": map[string]any{}, "Allowlist": map[string]any{"paths": []any{"a"}}})
	if len(dup.Findings) != 0 || len(dup.NotChecked) != 1 || !strings.Contains(dup.NotChecked[0].String(), "one key to the tool") {
		t.Errorf("two spellings of one table: %+v, want one not-checked", dup)
	}
}

// TestAKeyOutsideAsciiIsNotCheckedAndADottedKeyIsSplit: a key the tool might
// fold by a table this check does not keep is never passed, and a gitleaks key
// is split on a dot as the tool splits it.
func TestAKeyOutsideAsciiIsNotCheckedAndADottedKeyIsSplit(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"supply.secrets.4", "gitleaks", "allowlist.paths"}}
	repo := fstest.MapFS{".gitleaks.toml": {Data: []byte("x")}}
	paths := []any{"vendor/"}
	check := func(doc map[string]any) audit.Result {
		return audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: repo, Bindings: b, TOML: toml(doc)})
	}
	for name, doc := range map[string]map[string]any{
		"plain (control)":      {"allowlist": map[string]any{"paths": paths}},
		"dotted key":           {"allowlist.paths": paths},
		"dotted key, capitals": {"Allowlist.Paths": paths},
	} {
		if got := check(doc); len(got.Findings) != 1 || got.Findings[0].Rule != "supply.secrets.4" || len(got.NotChecked) != 0 {
			t.Errorf("%s: %+v, want the one allowlist finding", name, got)
		}
	}
	for name, doc := range map[string]map[string]any{
		"long s in the key":   {"allowlist": map[string]any{"pathſ": paths}},
		"long s in the table": {"allowliſt": map[string]any{"paths": paths}},
		"long s in the list":  {"allowliſts": []any{map[string]any{"paths": paths}}},
		"a dotted capital I":  {"allowlİst": map[string]any{"paths": paths}},
	} {
		got := check(doc)
		if len(got.Findings) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].String(), "is not ASCII") || !strings.Contains(got.NotChecked[0].String(), ".gitleaks.toml") {
			t.Errorf("%s: %+v, want one not-checked naming the file and the key", name, got)
		}
	}
	dup := check(map[string]any{"allowlist": map[string]any{"paths": paths}, "allowlist.paths": paths})
	if len(dup.Findings) != 0 || len(dup.NotChecked) != 1 || !strings.Contains(dup.NotChecked[0].String(), "one key to the tool") {
		t.Errorf("one path written twice: %+v, want one not-checked", dup)
	}
}

// TestAnAllowlistOfAnotherShapeIsNotCheckedNeverPassed: gitleaks reads one
// value where it expects a list and one table where it expects a list of
// tables, so each is a path allowlist to the tool; here each is not checked.
func TestAnAllowlistOfAnotherShapeIsNotCheckedNeverPassed(t *testing.T) {
	t.Parallel()
	b := []audit.Binding{{"supply.secrets.4", "gitleaks", "allowlist.paths"}}
	repo := fstest.MapFS{".gitleaks.toml": {Data: []byte("x")}}
	pattern := table(t, "escaped_patterns.txt")[0]
	str, list := pattern, []any{pattern}
	for name, doc := range map[string]map[string]any{
		"a string for paths":            {"allowlist": map[string]any{"paths": str}},
		"one table for allowlists":      {"allowlists": map[string]any{"paths": list}},
		"a string in a list of tables":  {"allowlists": []any{map[string]any{"paths": str}}},
		"a string for targetRules":      {"allowlists": []any{map[string]any{"paths": list, "targetRules": "other"}}},
		"a number in the list of paths": {"allowlist": map[string]any{"paths": []any{1}}},
		"allowlist as a list":           {"allowlist": list},
	} {
		got := audit.Check(t.Context(), audit.Input{Sources: sources(), Repo: repo, Bindings: b, TOML: toml(doc)})
		if len(got.Findings) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].String(), ".gitleaks.toml") {
			t.Errorf("%s: %+v, want one not-checked naming the file", name, got)
		}
	}
}
