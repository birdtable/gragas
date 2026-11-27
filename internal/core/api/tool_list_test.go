package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// boundRules is a rules file of three proposed rules: two bind the same entry of
// lint-a, one binds a second entry of lint-a and one of lint-z, a tool no
// plugin serves.
const boundRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
    bindings:
      - {tool: lint-a, target: source, key: wrap, severity: error, rung: 1, run_point: edit}
  - id: go.errors.2
    statement: Name the cause.
    strength: should
    status: proposed
    origin: authored
    bindings:
      - {tool: lint-a, target: source, key: wrap, severity: error, rung: 1, run_point: edit}
      - {tool: lint-a, target: config, key: cause, severity: warning, rung: 1, run_point: commit}
      - {tool: lint-z, target: source, key: zed, severity: error, rung: 1, run_point: edit}
  - id: go.errors.3
    statement: Say why.
    strength: may
    status: proposed
    origin: authored
`

func boundBook(tested string) map[string]string {
	files := goodBook()
	files["language/go/rules/errors.yaml"] = boundRules
	if tested != "" {
		files["rulebook.yaml"] = bookIdentity + "tested:\n  tools:\n" + tested + "  min_engine: 0.1.0\n"
	}
	return files
}

// servedBy installs at root a stand-in plugin named name at version whose
// manifest names tool, and answers it.
func servedBy(w *porttest.World, root model.Root, name, version, tool string) *porttest.StandinPlugin {
	p := porttest.NewStandinPlugin()
	p.Facts.Name, p.Facts.Version, p.Facts.Tool = name, version, tool
	w.PluginsOf().Install(string(root), model.PluginRef{Name: name, Version: version}, p)
	return p
}

// TestListToolsListsEachEntryWithTheRulesThatHoldItAndTheTestedVersion: the
// tools by name, each entry by target and key with every rule that has a binding
// on it, the rules that hold any binding counted, the tested version and the
// installed plugin that serves the tool, by the name and version its manifest
// says: the first in List's order where two serve it, each
// opened once and closed. A tool no plugin serves has none, and is Unserved.
func TestListToolsListsEachEntryWithTheRulesThatHoldItAndTheTestedVersion(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, boundBook("    - {name: lint-a, version: 2.5.0}\n"))
	first, second := servedBy(w, root, "a-lint", "1.2.0", "lint-a"), servedBy(w, root, "b-lint", "3.0.0", "lint-a")
	other := servedBy(w, root, "c-lint", "1.0.0", "lint-c")
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	if err != nil || l.Rules != 2 || len(l.Rows) != 2 {
		t.Fatalf("ListTools = %+v, %v; want two rules that hold a binding and two tools", l, err)
	}
	a := l.Rows[0]
	if a.Tool != "lint-a" || a.Tested != "2.5.0" || a.Plugin == nil || *a.Plugin != (model.ToolPlugin{Name: "a-lint", Version: "1.2.0"}) || len(a.Entries) != 2 ||
		a.Entries[0].Target != model.TargetConfig || a.Entries[0].Key != "cause" ||
		a.Entries[1].Key != "wrap" || len(a.Entries[1].Rules) != 2 || a.Entries[1].Rules[0] != "go.errors.1" || a.Entries[1].Rules[1] != "go.errors.2" {
		t.Errorf("lint-a = %+v", a)
	}
	if z := l.Rows[1]; z.Tool != "lint-z" || z.Tested != "" || z.Plugin != nil || len(z.Entries) != 1 {
		t.Errorf("lint-z = %+v; want no tested version and no plugin", z)
	}
	if u := coreapi.Unserved(l); len(u) != 1 || u[0] != "lint-z" {
		t.Errorf("Unserved = %v; want lint-z", u)
	}
	if d := coreapi.ToolsDocument(l); d.Word != string(model.NotReady) || d.Schema != 2 || len(d.Unserved) != 1 || d.Unserved[0] != "lint-z" {
		t.Errorf("document = %+v; want not ready, schema 2, lint-z unserved", d)
	}
	for name, p := range map[string]*porttest.StandinPlugin{"a-lint": first, "b-lint": second, "c-lint": other} {
		if opened, closed := p.Opened(); opened != 1 || closed != 1 || len(p.Calls()) != 0 {
			t.Errorf("%s: opened %d, closed %d, called %v; want each opened once, closed and never called", name, opened, closed, p.Calls())
		}
	}
}

// TestAListingWhoseBoundToolsAllHaveAPluginIsDone: with lint-a and lint-z
// served, nothing is Unserved and the document's word is done, with an empty
// list. The control of the row above.
func TestAListingWhoseBoundToolsAllHaveAPluginIsDone(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, boundBook(""))
	servedBy(w, root, "a-lint", "1.2.0", "lint-a")
	servedBy(w, root, "z-lint", "0.1.0", "lint-z")
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	if err != nil || len(coreapi.Unserved(l)) != 0 {
		t.Fatalf("ListTools = %+v, %v; want every bound tool served", l, err)
	}
	if d := coreapi.ToolsDocument(l); d.Word != string(model.Done) || d.Unserved == nil || len(d.Unserved) != 0 {
		t.Errorf("document = %+v; want done and an empty unserved list", d)
	}
}

// TestAPluginThatIsRefusedEndsTheListing: a refused plugin of any tool gives
// no listing and no row in its place (fail closed), and the
// error names the plugin and matches the refusal; a list that fails is an
// error too. Risk: a tampered plugin of a bound tool that the listing hides
// by naming the tool as served by none, or by another.
func TestAPluginThatIsRefusedEndsTheListing(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, boundBook(""))
	servedBy(w, root, "a-lint", "1.2.0", "lint-a")
	servedBy(w, root, "b-lint", "1.0.0", "lint-z").OpenErr = port.ErrNotApproved
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	var un *coreapi.UnapprovedPlugin
	if !errors.Is(err, port.ErrNotApproved) || !errors.As(err, &un) || un.Ref != (model.PluginRef{Name: "b-lint", Version: "1.0.0"}) ||
		!strings.Contains(err.Error(), "b-lint 1.0.0") || len(l.Rows) != 0 {
		t.Errorf("an unapproved plugin: %+v, %v; want an UnapprovedPlugin naming b-lint 1.0.0 and no rows", l, err)
	}
	w, root = listWorld(t, boundBook(""))
	servedBy(w, root, "b-lint", "1.0.0", "lint-z").OpenErr = port.ErrDigest
	if _, err := coreapi.ListTools(context.Background(), w.Ports(), root); !errors.Is(err, port.ErrDigest) || errors.As(err, &un) {
		t.Errorf("a plugin refused for its digest: %v; want ErrDigest and no UnapprovedPlugin", err)
	}
	w, root = listWorld(t, boundBook(""))
	w.PluginsOf().ListErr = errors.New("disk")
	if _, err := coreapi.ListTools(context.Background(), w.Ports(), root); err == nil || !strings.Contains(err.Error(), "could not be listed") {
		t.Errorf("a failed list: %v; want it said", err)
	}
}

// TestAToolTheTestedSetNamesAndNoRuleBindsIsListedWithNoEntry: and, with no
// plugin, it is not Unserved, since no rule binds it.
func TestAToolTheTestedSetNamesAndNoRuleBindsIsListedWithNoEntry(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, boundBook("    - {name: lint-a, version: 9.0.0}\n    - {name: lint-q, version: 1.0.0}\n"))
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	if err != nil || len(l.Rows) != 3 || l.Rows[0].Plugin != nil || l.Rows[1].Tool != "lint-q" || len(l.Rows[1].Entries) != 0 || l.Rows[1].Tested != "1.0.0" || l.Rows[1].Plugin != nil {
		t.Fatalf("ListTools = %+v, %v; want lint-a with no plugin and lint-q with no entry", l, err)
	}
	if u := coreapi.Unserved(l); strings.Join(u, ",") != "lint-a,lint-z" {
		t.Errorf("Unserved = %v; want lint-a and lint-z, not lint-q", u)
	}
}

// TestARulebookWithNoBindingListsNoTool: the empty state is no row, and the
// document says 0 rules and 0 entries.
func TestARulebookWithNoBindingListsNoTool(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, goodBook())
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	if err != nil || len(l.Rows) != 0 || l.Rules != 0 {
		t.Fatalf("ListTools = %+v, %v; want nothing", l, err)
	}
	if d := coreapi.ToolsDocument(l); d.Entries != 0 || d.Rules != 0 || d.Command != "tools" || d.Contract != model.ContractVersion ||
		d.Word != "done" || d.Unexplained == nil || len(d.Unexplained) != 0 || d.Tools == nil {
		t.Errorf("document = %+v", d)
	}
}

// TestListToolsRefusesAFolderWithNoRulebookAndOneWithFindings: no rulebook is
// ErrNoBook, a rulebook with a finding is a FindingsError counting them, and
// neither lists a thing.
func TestListToolsRefusesAFolderWithNoRulebookAndOneWithFindings(t *testing.T) {
	t.Parallel()
	w, root := listWorld(t, map[string]string{"README.md": "x"})
	if l, err := coreapi.ListTools(context.Background(), w.Ports(), root); !errors.Is(err, coreapi.ErrNoBook) || len(l.Rows) != 0 {
		t.Errorf("no rulebook: %+v, %v; want ErrNoBook", l, err)
	}
	files := boundBook("")
	files["rulebook.yaml"] = "format: 3\nname: a\nname: b\n"
	w, root = listWorld(t, files)
	l, err := coreapi.ListTools(context.Background(), w.Ports(), root)
	var f *coreapi.FindingsError
	if !errors.As(err, &f) || f.Count < 1 || len(l.Rows) != 0 || !strings.Contains(err.Error(), "findings") {
		t.Errorf("a finding: %+v, %v; want a FindingsError", l, err)
	}
}

// TestListToolsEndsNotReadyOnAnOpenRun, as the other listings do.
func TestListToolsEndsNotReadyOnAnOpenRun(t *testing.T) {
	t.Parallel()
	j := newJournalWorld(t, boundBook(""))
	j.w.SeedSchemas(validateSchemas()...)
	j.copied("run-open", "rulebook.yaml", "x", true, now)
	if _, err := coreapi.ListTools(context.Background(), j.ports, j.root); !coreapi.IsOpenRecord(err) || !errors.Is(err, coreapi.ErrNotReady) {
		t.Errorf("an open run: %v; want the open record, not ready", err)
	}
}
