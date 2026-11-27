package link_test

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// checkerSchema is the Schema port with both of link's schemas compiled and
// every document valid, so each refusal below is the reader's own.
func checkerSchema(t *testing.T) port.Schema {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(link.CheckerSchemaName)
	return w.Ports().Schema
}

const (
	ownDecl = "format: 3\nname: own-plugin\ntool: lint-a\npublisher: alex\ntests: testdata\nlicence: Apache-2.0\n"
	refDecl = "format: 3\nname: shared-analyzer\ntool: lint-a\npublisher: sam\nfrom: shared-go\n"
)

// cleanCheckers is a rulebook, an overlay that imports shared-go, with its own
// artifact (its code and tests in its folder), an import's declared by
// reference, a binding that names its own, and the parent's and the import's
// checker code handed in. Every finding below is one change of it.
func cleanCheckers() link.CheckerCode {
	return link.CheckerCode{
		Listing: []string{
			"checkers/own-plugin/checker.yaml", "checkers/own-plugin/plugin.so", "checkers/own-plugin/testdata/case.txt",
			"checkers/shared-analyzer/checker.yaml",
		},
		Files: []model.File{
			{Path: "checkers/own-plugin/checker.yaml", Data: []byte(ownDecl)},
			{Path: "checkers/shared-analyzer/checker.yaml", Data: []byte(refDecl)},
		},
		Book: model.Book{
			Identity: model.RulebookIdentity{
				Name: "acme-go", Extends: &model.Extends{Rulebook: "base-go", Version: "1.0.0"},
				Imports: []model.Import{{Rulebook: "shared-go", Version: "2.0.0"}},
			},
			Rules: []model.Rule{{ID: "acme.errors.1", Bindings: []model.Binding{{Tool: "lint-a", Key: "own", Checker: "own-plugin"}}}},
		},
		Parent:  &link.Imported{Rulebook: "base-go", Code: []model.CheckerArtifact{{Name: "parent-plugin", Publisher: "pat", Tool: "lint-a"}}},
		Imports: []link.Imported{{Rulebook: "shared-go", Code: []model.CheckerArtifact{{Name: "shared-analyzer", Publisher: "sam", Tool: "lint-a"}}}},
	}
}

// file replaces or adds the declaration at path.
func file(in *link.CheckerCode, path, data string) {
	in.Files = slices.DeleteFunc(in.Files, func(f model.File) bool { return f.Path == path })
	in.Files = append(in.Files, model.File{Path: path, Data: []byte(data)})
	if !slices.Contains(in.Listing, path) {
		in.Listing = append(in.Listing, path)
	}
}

// TestTheCheckerCodeFindingsFireOnEachPlantAndNotOnTheCleanControl
// (gates.plants.1): the clean control has no finding and both declarations;
// each plant turns on its finding, and only it.
// Risk: checker code a consumer's tools load that the rulebook never
// declared, or an import's brought in silently.
func TestTheCheckerCodeFindingsFireOnEachPlantAndNotOnTheCleanControl(t *testing.T) {
	t.Parallel()
	s := checkerSchema(t)
	decls, found := link.Checkers(s, cleanCheckers())
	if len(found) != 0 {
		t.Fatalf("the clean control has findings: %+v", found)
	}
	if len(decls) != 2 || decls[0].Name != "own-plugin" || decls[1].From != "shared-go" || decls[1].Publisher != "sam" {
		t.Fatalf("the clean control reads %+v", decls)
	}
	for name, c := range map[string]struct {
		plant func(in *link.CheckerCode)
		says  []string
	}{
		"code in a folder with no declaration": {func(in *link.CheckerCode) {
			in.Listing = append(in.Listing, "checkers/stray/stray.so", "checkers/stray/more.so")
		}, []string{"checker code in checkers/stray/ has no declaration"}},
		"code straight under checkers/": {func(in *link.CheckerCode) {
			in.Listing = append(in.Listing, "checkers/loose.so")
		}, []string{"stands straight under checkers/"}},
		"a declaration that is refused": {func(in *link.CheckerCode) {
			file(in, "checkers/own-plugin/checker.yaml", ownDecl+"digest: sha256:00\n")
		}, []string{"key: digest is not a field of a checker declaration", "names the checker code own-plugin, which the rulebook does not declare"}},
		"a name that is not its folder": {func(in *link.CheckerCode) {
			file(in, "checkers/own-plugin/checker.yaml", strings.Replace(ownDecl, "name: own-plugin", "name: other-plugin", 1))
		}, []string{"place: the name is other-plugin, and the file stands in the folder own-plugin", "which the rulebook does not declare"}},
		"an own artifact with no tests": {func(in *link.CheckerCode) {
			file(in, "checkers/own-plugin/checker.yaml", strings.Replace(ownDecl, "tests: testdata\n", "", 1))
		}, []string{"own-plugin declares no tests"}},
		"tests that hold nothing": {func(in *link.CheckerCode) {
			file(in, "checkers/own-plugin/checker.yaml", strings.Replace(ownDecl, "tests: testdata", "tests: spec", 1))
		}, []string{"own-plugin's tests are at spec, which holds nothing"}},
		"an import's artifact declared by reference that holds code": {func(in *link.CheckerCode) {
			in.Listing = append(in.Listing, "checkers/shared-analyzer/analyzer.so")
		}, []string{"holds code of its own, checkers/shared-analyzer/analyzer.so"}},
		"an import's artifact the rulebook does not declare": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "shared-policy", Publisher: "sam", Tool: "lint-c"})
		}, []string{"the import shared-go brings the checker code shared-policy by sam, and this rulebook does not declare it"}},
		"a declaration from a rulebook it does not import": {func(in *link.CheckerCode) {
			in.Book.Identity.Imports = nil
		}, []string{"shared-analyzer is declared from shared-go, which the rulebook neither extends nor imports"}},
		"an import's artifact by another publisher": {func(in *link.CheckerCode) {
			file(in, "checkers/shared-analyzer/checker.yaml", strings.Replace(refDecl, "publisher: sam", "publisher: kim", 1))
		}, []string{"brings the checker code shared-analyzer by sam, and this rulebook does not declare it", "shared-analyzer by kim is declared from shared-go, which carries no such artifact"}},
		"an overlay's own artifact by its parent's name": {func(in *link.CheckerCode) {
			file(in, "checkers/parent-plugin/checker.yaml", strings.Replace(ownDecl, "name: own-plugin", "name: parent-plugin", 1))
			in.Listing = append(in.Listing, "checkers/parent-plugin/testdata/case.txt")
		}, []string{"parent-plugin is the parent base-go's checker code, and an overlay holds a name its parent carries only by reference from the parent"}},
		"a declaration by reference from an import under the parent's name": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "parent-plugin", Publisher: "sam", Tool: "lint-a"})
			file(in, "checkers/parent-plugin/checker.yaml", strings.Replace(refDecl, "name: shared-analyzer", "name: parent-plugin", 1))
			in.Book.Rules[0].Bindings[0].Checker = "parent-plugin"
		}, []string{"parent-plugin is the parent base-go's checker code, and an overlay holds a name its parent carries only by reference",
			"parent-plugin is carried by the parent base-go and by shared-go; a rulebook can hold one"}},
		"a name the parent and an import both carry": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "parent-plugin", Publisher: "sam", Tool: "lint-a"})
		}, []string{"parent-plugin is carried by the parent base-go and by shared-go; a rulebook can hold one"}},
		"the rulebook's own artifact by an import's name": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "own-plugin", Publisher: "sam", Tool: "lint-a"})
		}, []string{"checkers/own-plugin/checker.yaml: own-plugin is this rulebook's own and also shared-go's; a rulebook can hold one"}},
		"code with no declaration under an import's name": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "shared-policy", Publisher: "sam", Tool: "lint-a"})
			in.Listing = append(in.Listing, "checkers/shared-policy/policy.so")
		}, []string{"checkers/shared-policy/ holds this rulebook's code with no declaration, and shared-policy is also shared-go's; a rulebook can hold one"}},
		"a refused declaration under an import's name": {func(in *link.CheckerCode) {
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "own-plugin", Publisher: "sam", Tool: "lint-a"})
			file(in, "checkers/own-plugin/checker.yaml", ownDecl+"digest: sha256:00\n")
		}, []string{"key: digest is not a field of a checker declaration", "names the checker code own-plugin, which the rulebook does not declare",
			"checkers/own-plugin/ holds this rulebook's code, whose declaration is refused, and own-plugin is also shared-go's; a rulebook can hold one"}},
		"code with no declaration under the parent's name": {func(in *link.CheckerCode) {
			in.Listing = append(in.Listing, "checkers/parent-plugin/p.so")
		}, []string{"checker code in checkers/parent-plugin/ has no declaration, so a consumer's tools would load code the rulebook does not declare, under the name of the parent base-go's checker code"}},
		"a name two imports carry, declared from one": {func(in *link.CheckerCode) {
			in.Book.Identity.Imports = append(in.Book.Identity.Imports, model.Import{Rulebook: "other-go", Version: "1.0.0"})
			in.Imports = append(in.Imports, link.Imported{Rulebook: "other-go", Code: []model.CheckerArtifact{{Name: "shared-analyzer", Publisher: "kim", Tool: "lint-a"}}})
		}, []string{"shared-analyzer is declared from shared-go and is also other-go's artifact; a rulebook can hold one"}},
		"a name two imports carry, undeclared": {func(in *link.CheckerCode) {
			in.Book.Identity.Imports = append(in.Book.Identity.Imports, model.Import{Rulebook: "other-go", Version: "1.0.0"})
			in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "shared-policy", Publisher: "sam", Tool: "lint-a"})
			in.Imports = append(in.Imports, link.Imported{Rulebook: "other-go", Code: []model.CheckerArtifact{{Name: "shared-policy", Publisher: "kim", Tool: "lint-a"}}})
		}, []string{"shared-policy is carried by shared-go and other-go; a rulebook can hold one"}},
		"a declaration from the parent by another publisher": {func(in *link.CheckerCode) {
			file(in, "checkers/parent-plugin/checker.yaml", parentDecl("kim"))
		}, []string{"parent-plugin is the parent base-go's checker code, and an overlay holds a name its parent carries only by reference",
			"parent-plugin by kim is declared from base-go, which carries no such artifact by that publisher"}},
		"a binding that names no declaration": {func(in *link.CheckerCode) {
			in.Book.Rules[0].Bindings[0].Checker = "ghost-plugin"
		}, []string{"acme.errors.1's lint-a binding names the checker code ghost-plugin, which the rulebook does not declare"}},
		"a binding of another tool": {func(in *link.CheckerCode) {
			in.Book.Rules[0].Bindings[0].Tool = "lint-b"
		}, []string{"names the checker code own-plugin, which is declared for lint-a"}},
	} {
		in := cleanCheckers()
		c.plant(&in)
		_, found := link.Checkers(s, in)
		var msgs []string
		for _, f := range found {
			msgs = append(msgs, f.Entity+": "+f.Msg)
		}
		got := strings.Join(msgs, "\n")
		if len(found) != len(c.says) {
			t.Errorf("%s: %d findings, want %d:\n%s", name, len(found), len(c.says), got)
		}
		for _, want := range c.says {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the findings do not say %q:\n%s", name, want, got)
			}
		}
	}
}

// TestNothingFetchedChecksNoImportAndNoParent: with no parent and no import
// handed in, as on a live run until a fetch hands them in, a declaration from a pinned import
// is read and listed, and only the import's own half is not checked.
func TestNothingFetchedChecksNoImportAndNoParent(t *testing.T) {
	t.Parallel()
	in := cleanCheckers()
	in.Parent, in.Imports = nil, nil
	decls, found := link.Checkers(checkerSchema(t), in)
	if len(found) != 0 || len(decls) != 2 {
		t.Errorf("with nothing fetched: %d declarations, findings %+v", len(decls), found)
	}
}

// TestASkippedImportCheckTurnsTheImportPlantGreen is the plant of the import
// check: the same input with the import's checker code dropped from what is
// handed in reads as clean, so the test above that hands it in is the one
// that holds S3.
func TestASkippedImportCheckTurnsTheImportPlantGreen(t *testing.T) {
	t.Parallel()
	in := cleanCheckers()
	in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "shared-policy", Publisher: "sam", Tool: "lint-c"})
	if _, found := link.Checkers(checkerSchema(t), in); len(found) != 1 {
		t.Fatalf("an import's undeclared artifact gave %d findings, want 1", len(found))
	}
	in.Imports = nil
	if _, found := link.Checkers(checkerSchema(t), in); len(found) != 0 {
		t.Errorf("with the import not handed in, %d findings", len(found))
	}
}

// FuzzParseDeclaration (tests.generated.1): ReadDeclaration over any bytes,
// at a declaration's place, answers a declaration or its refusals and never
// both, at most twenty, each a check of the set at that path; a declaration it
// answers has the folder's name, a publisher and tests below the folder. A
// rulebook's declaration is less-trusted input, so nothing in it is guessed.
func FuzzParseDeclaration(f *testing.F) {
	f.Add([]byte(ownDecl))
	f.Add([]byte(refDecl))
	// The seeds of a review, inline, since the checked-in seed folder is a
	// protected path: the empty publisher it found, an empty
	// licence, and a control character in the publisher, the licence and the
	// tool.
	f.Add([]byte(emptyPublisher))
	f.Add([]byte(strings.Replace(ownDecl, "licence: Apache-2.0", "licence: \"\"", 1)))
	f.Add([]byte(strings.Replace(ownDecl, "publisher: alex", "publisher: \"alex\\nghost-plugin\"", 1)))
	f.Add([]byte(strings.Replace(ownDecl, "licence: Apache-2.0", "licence: \"Apache\\e[31m\"", 1)))
	f.Add([]byte(strings.Replace(ownDecl, "tool: lint-a", "tool: \"lint\\u0001a\"", 1)))
	// Inline too: characters that print as nothing, turn the text around or
	// start a line, a seed each.
	for _, s := range unseenSeeds() {
		f.Add([]byte(s.doc))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		w := porttest.NewWorld(t)
		w.SeedSchemas(link.CheckerSchemaName)
		const at = "checkers/own-plugin/checker.yaml"
		c, refused := link.ReadDeclaration(w.Ports().Schema, at, data)
		if len(refused) > 20 {
			t.Fatalf("%d refusals", len(refused))
		}
		for _, r := range refused {
			if !slices.Contains(checks(), r.Check) || r.Path != at {
				t.Fatalf("refusal %v names no check of the set, or another path", r)
			}
		}
		if len(refused) > 0 {
			if c != (model.Checker{}) {
				t.Fatalf("refused and still answered %+v", c)
			}
			return
		}
		if c.Name != "own-plugin" || c.Format != model.FormatVersion || !strings.ContainsFunc(c.Publisher, shows) || c.Tool == "" ||
			strings.ContainsFunc(c.Publisher+c.Licence+c.Tool, unseen) {
			t.Fatalf("read %+v from %s", c, at)
		}
		for step := range strings.SplitSeq(c.Tests, "/") {
			if c.Tests != "" && (step == "" || step == "." || step == "..") {
				t.Fatalf("the tests path %q leaves or blurs the folder", c.Tests)
			}
		}
	})
}

// parentDecl is a declaration by reference from the parent of the clean
// rulebook, of the parent's parent-plugin, by publisher.
func parentDecl(publisher string) string {
	return "format: 3\nname: parent-plugin\ntool: lint-a\npublisher: " + publisher + "\nfrom: base-go\n"
}

// TestADeclarationFromTheParentIsTheParentsArtifactAndNoShadow: an overlay that declares its parent's
// artifact by reference from the parent, by the parent's publisher, and binds
// a rule to it, has no finding, even when an import carries the same name.
// Risk: an overlay rule its parent's checker code could never check.
func TestADeclarationFromTheParentIsTheParentsArtifactAndNoShadow(t *testing.T) {
	t.Parallel()
	in := cleanCheckers()
	file(&in, "checkers/parent-plugin/checker.yaml", parentDecl("pat"))
	in.Book.Rules[0].Bindings[0].Checker = "parent-plugin"
	in.Imports[0].Code = append(in.Imports[0].Code, model.CheckerArtifact{Name: "parent-plugin", Publisher: "sam", Tool: "lint-a"})
	decls, found := link.Checkers(checkerSchema(t), in)
	if len(found) != 0 || !slices.ContainsFunc(decls, func(c model.Checker) bool { return c.Name == "parent-plugin" && c.From == "base-go" }) {
		t.Errorf("a declaration from the parent: %d declarations, findings %+v", len(decls), found)
	}
}

// TestEveryCheckerFindingsFixLeavesNoFinding: each finding, planted on
// the clean rulebook, is followed down every way out its fix line gives, each
// from the plant afresh, and each leaves no finding, so no fix sends the
// author to another. The plants hold each name in each place it can collide:
// the parent's, an import's, two imports' and the rulebook's own.
// Risk: a finding whose fix is another finding, which an author can never
// clear.
func TestEveryCheckerFindingsFixLeavesNoFinding(t *testing.T) {
	t.Parallel()
	s := checkerSchema(t)
	withTests := func(in *link.CheckerCode, name string) {
		file(in, "checkers/"+name+"/checker.yaml", strings.Replace(ownDecl, "name: own-plugin", "name: "+name, 1))
		in.Listing = append(in.Listing, "checkers/"+name+"/testdata/case.txt")
	}
	drop := func(in *link.CheckerCode, prefix string) {
		in.Listing = slices.DeleteFunc(in.Listing, func(p string) bool { return strings.HasPrefix(p, prefix) })
		in.Files = slices.DeleteFunc(in.Files, func(f model.File) bool { return strings.HasPrefix(f.Path, prefix) })
	}
	carry := func(in *link.CheckerCode, i int, name, publisher string) {
		in.Imports[i].Code = append(in.Imports[i].Code, model.CheckerArtifact{Name: name, Publisher: publisher, Tool: "lint-a"})
	}
	unCarry := func(in *link.CheckerCode, i int, name string) {
		in.Imports[i].Code = slices.DeleteFunc(in.Imports[i].Code, func(a model.CheckerArtifact) bool { return a.Name == name })
	}
	// secondImport is other-go, imported and handed in beside shared-go.
	secondImport := func(in *link.CheckerCode) {
		in.Book.Identity.Imports = append(in.Book.Identity.Imports, model.Import{Rulebook: "other-go", Version: "1.0.0"})
		in.Imports = append(in.Imports, link.Imported{Rulebook: "other-go"})
	}
	byRef := func(in *link.CheckerCode, name, from, publisher string) {
		file(in, "checkers/"+name+"/checker.yaml", "format: 3\nname: "+name+"\ntool: lint-a\npublisher: "+publisher+"\nfrom: "+from+"\n")
	}
	fromParent := func(in *link.CheckerCode) {
		drop(in, "checkers/parent-plugin/")
		file(in, "checkers/parent-plugin/checker.yaml", parentDecl("pat"))
	}
	// renameOwn moves the rulebook's own-plugin, code, tests and bindings, to
	// my-plugin, and declares shared-go's own-plugin by reference in the
	// folder that is now free.
	renameOwn := func(in *link.CheckerCode) {
		drop(in, "checkers/own-plugin/")
		in.Listing = append(in.Listing, "checkers/my-plugin/plugin.so")
		withTests(in, "my-plugin")
		in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
		byRef(in, "own-plugin", "shared-go", "sam")
	}
	type fixes = map[string]func(in *link.CheckerCode)
	for name, c := range map[string]struct {
		plant func(in *link.CheckerCode)
		ways  fixes
	}{
		"code with no declaration": {
			func(in *link.CheckerCode) { in.Listing = append(in.Listing, "checkers/stray-plugin/stray.so") },
			fixes{"declare it with its tests": func(in *link.CheckerCode) { withTests(in, "stray-plugin") }}},
		"code straight under checkers/": {
			func(in *link.CheckerCode) { in.Listing = append(in.Listing, "checkers/loose.so") },
			fixes{"move it beside a declaration": func(in *link.CheckerCode) {
				drop(in, "checkers/loose.so")
				in.Listing = append(in.Listing, "checkers/loose-plugin/loose.so")
				withTests(in, "loose-plugin")
			}}},
		"a refused declaration": {
			func(in *link.CheckerCode) {
				file(in, "checkers/own-plugin/checker.yaml", ownDecl+"digest: sha256:00\n")
			},
			fixes{"write it as the page says": func(in *link.CheckerCode) { file(in, "checkers/own-plugin/checker.yaml", ownDecl) }}},
		"no tests": {
			func(in *link.CheckerCode) {
				file(in, "checkers/own-plugin/checker.yaml", strings.Replace(ownDecl, "tests: testdata\n", "", 1))
			},
			fixes{"name them": func(in *link.CheckerCode) { file(in, "checkers/own-plugin/checker.yaml", ownDecl) }}},
		"tests that hold nothing": {
			func(in *link.CheckerCode) {
				file(in, "checkers/own-plugin/checker.yaml", strings.Replace(ownDecl, "tests: testdata", "tests: spec", 1))
			},
			fixes{
				"put its tests there": func(in *link.CheckerCode) {
					in.Listing = append(in.Listing, "checkers/own-plugin/spec/case.txt")
				},
				"name where they are": func(in *link.CheckerCode) { file(in, "checkers/own-plugin/checker.yaml", ownDecl) },
			}},
		"a reference that holds code": {
			func(in *link.CheckerCode) { in.Listing = append(in.Listing, "checkers/shared-analyzer/analyzer.so") },
			fixes{
				"keep only its declaration": func(in *link.CheckerCode) { drop(in, "checkers/shared-analyzer/analyzer.so") },
				"move the code to a name of its own": func(in *link.CheckerCode) {
					drop(in, "checkers/shared-analyzer/analyzer.so")
					in.Listing = append(in.Listing, "checkers/my-analyzer/analyzer.so")
					withTests(in, "my-analyzer")
				},
			}},
		// The import carries the name of the rulebook's own artifact, and
		// the author declared theirs by reference over the folder that
		// holds the rulebook's code.
		"a reference over the rulebook's own code": {
			func(in *link.CheckerCode) {
				carry(in, 0, "own-plugin", "sam")
				byRef(in, "own-plugin", "shared-go", "sam")
			},
			fixes{
				"keep only its declaration": func(in *link.CheckerCode) {
					drop(in, "checkers/own-plugin/plugin.so")
					drop(in, "checkers/own-plugin/testdata/")
				},
				"move the code to a name of its own": func(in *link.CheckerCode) {
					drop(in, "checkers/own-plugin/plugin.so")
					drop(in, "checkers/own-plugin/testdata/")
					in.Listing = append(in.Listing, "checkers/my-plugin/plugin.so")
					withTests(in, "my-plugin")
					in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
				},
			}},
		"the rulebook's own artifact by an import's name": {
			func(in *link.CheckerCode) { carry(in, 0, "own-plugin", "sam") },
			fixes{
				"rename yours":      renameOwn,
				"drop the import's": func(in *link.CheckerCode) { unCarry(in, 0, "own-plugin") },
			}},
		"an import's undeclared artifact": {
			func(in *link.CheckerCode) { carry(in, 0, "shared-policy", "sam") },
			fixes{"declare it by reference": func(in *link.CheckerCode) { byRef(in, "shared-policy", "shared-go", "sam") }}},
		"a name two imports carry, undeclared": {
			func(in *link.CheckerCode) {
				secondImport(in)
				carry(in, 0, "shared-policy", "sam")
				carry(in, 1, "shared-policy", "kim")
			},
			fixes{
				"declare one and drop the other's": func(in *link.CheckerCode) {
					byRef(in, "shared-policy", "other-go", "kim")
					unCarry(in, 0, "shared-policy")
				},
			}},
		"a name two imports carry, declared from one": {
			func(in *link.CheckerCode) {
				secondImport(in)
				carry(in, 1, "shared-analyzer", "kim")
			},
			fixes{"drop the other's": func(in *link.CheckerCode) { unCarry(in, 1, "shared-analyzer") }}},
		"a declaration from no import": {
			func(in *link.CheckerCode) { in.Book.Identity.Imports = nil },
			fixes{
				"import it": func(in *link.CheckerCode) {
					in.Book.Identity.Imports = []model.Import{{Rulebook: "shared-go", Version: "2.0.0"}}
				},
				// Nothing fetches a rulebook the book does not import.
				"declare it as the rulebook's own": func(in *link.CheckerCode) {
					in.Imports = nil
					drop(in, "checkers/shared-analyzer/")
					in.Listing = append(in.Listing, "checkers/shared-analyzer/analyzer.so")
					withTests(in, "shared-analyzer")
				},
			}},
		"a declaration from no import, of a name an import carries": {
			func(in *link.CheckerCode) {
				carry(in, 0, "shared-policy", "sam")
				byRef(in, "shared-policy", "ghost-go", "sam")
			},
			fixes{"declare it from the import that carries it": func(in *link.CheckerCode) {
				byRef(in, "shared-policy", "shared-go", "sam")
			}}},
		"an import's artifact by another publisher": {
			func(in *link.CheckerCode) {
				file(in, "checkers/shared-analyzer/checker.yaml", strings.Replace(refDecl, "publisher: sam", "publisher: kim", 1))
			},
			fixes{"declare it by its own": func(in *link.CheckerCode) { file(in, "checkers/shared-analyzer/checker.yaml", refDecl) }}},
		// Code of the rulebook's own under a
		// name another carries, with no declaration read, is renamed or the
		// others' are dropped, and nothing is declared by reference over it.
		"code with no declaration under an import's name": {
			func(in *link.CheckerCode) {
				carry(in, 0, "shared-policy", "sam")
				in.Listing = append(in.Listing, "checkers/shared-policy/policy.so")
			},
			fixes{
				"rename yours and declare theirs there": func(in *link.CheckerCode) {
					drop(in, "checkers/shared-policy/")
					in.Listing = append(in.Listing, "checkers/my-policy/policy.so")
					withTests(in, "my-policy")
					byRef(in, "shared-policy", "shared-go", "sam")
				},
				"drop theirs and declare yours": func(in *link.CheckerCode) {
					unCarry(in, 0, "shared-policy")
					withTests(in, "shared-policy")
				},
			}},
		"a refused declaration under an import's name": {
			func(in *link.CheckerCode) {
				carry(in, 0, "own-plugin", "sam")
				file(in, "checkers/own-plugin/checker.yaml", ownDecl+"digest: sha256:00\n")
			},
			fixes{
				"write it, then rename yours and declare theirs there": renameOwn,
				"write it, then drop theirs": func(in *link.CheckerCode) {
					file(in, "checkers/own-plugin/checker.yaml", ownDecl)
					unCarry(in, 0, "own-plugin")
				},
			}},
		"code with no declaration under two imports' name": {
			func(in *link.CheckerCode) {
				secondImport(in)
				carry(in, 0, "shared-policy", "sam")
				carry(in, 1, "shared-policy", "kim")
				in.Listing = append(in.Listing, "checkers/shared-policy/policy.so")
			},
			fixes{
				"rename yours, declare one of theirs there and drop the other's": func(in *link.CheckerCode) {
					drop(in, "checkers/shared-policy/")
					in.Listing = append(in.Listing, "checkers/my-policy/policy.so")
					withTests(in, "my-policy")
					byRef(in, "shared-policy", "other-go", "kim")
					unCarry(in, 0, "shared-policy")
				},
				"drop theirs and declare yours": func(in *link.CheckerCode) {
					unCarry(in, 0, "shared-policy")
					unCarry(in, 1, "shared-policy")
					withTests(in, "shared-policy")
				},
			}},
		// The binding's finding follows the
		// own-code rule too, so each of these binds the rule to the name.
		"bound code with no declaration under the parent's name": {
			func(in *link.CheckerCode) {
				in.Listing = append(in.Listing, "checkers/parent-plugin/p.so")
				in.Book.Rules[0].Bindings[0].Checker = "parent-plugin"
			},
			fixes{"rename yours": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				in.Listing = append(in.Listing, "checkers/my-plugin/p.so")
				withTests(in, "my-plugin")
				in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
			}}},
		"a bound overlay's own artifact by the parent's name, its declaration refused": {
			func(in *link.CheckerCode) {
				withTests(in, "parent-plugin")
				file(in, "checkers/parent-plugin/checker.yaml", strings.Replace(ownDecl, "name: own-plugin", "name: parent-plugin", 1)+"digest: sha256:00\n")
				in.Book.Rules[0].Bindings[0].Checker = "parent-plugin"
			},
			fixes{"rename yours": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				withTests(in, "my-plugin")
				in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
			}}},
		"the bound own artifact under a name the parent carries, its declaration refused": {
			func(in *link.CheckerCode) {
				in.Parent.Code = append(in.Parent.Code, model.CheckerArtifact{Name: "own-plugin", Publisher: "pat", Tool: "lint-a"})
				file(in, "checkers/own-plugin/checker.yaml", ownDecl+"digest: sha256:00\n")
			},
			fixes{"rename yours": func(in *link.CheckerCode) {
				drop(in, "checkers/own-plugin/")
				in.Listing = append(in.Listing, "checkers/my-plugin/plugin.so")
				withTests(in, "my-plugin")
				in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
			}}},
		"bound code with no declaration under a name the parent and an import carry": {
			func(in *link.CheckerCode) {
				in.Listing = append(in.Listing, "checkers/parent-plugin/p.so")
				carry(in, 0, "parent-plugin", "sam")
				in.Book.Rules[0].Bindings[0].Checker = "parent-plugin"
			},
			fixes{"rename yours, declare the parent's there and drop the import's": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				in.Listing = append(in.Listing, "checkers/my-plugin/p.so")
				withTests(in, "my-plugin")
				in.Book.Rules[0].Bindings[0].Checker = "my-plugin"
				fromParent(in)
				unCarry(in, 0, "parent-plugin")
			}}},
		"code with no declaration under the parent's name": {
			func(in *link.CheckerCode) { in.Listing = append(in.Listing, "checkers/parent-plugin/p.so") },
			fixes{"give yours a name of its own": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				in.Listing = append(in.Listing, "checkers/my-plugin/p.so")
				withTests(in, "my-plugin")
			}}},
		"an overlay's own artifact under a name the parent and an import carry": {
			func(in *link.CheckerCode) {
				withTests(in, "parent-plugin")
				carry(in, 0, "parent-plugin", "sam")
			},
			fixes{"rename yours, declare the parent's there and drop the import's": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				withTests(in, "my-plugin")
				fromParent(in)
				unCarry(in, 0, "parent-plugin")
			}}},
		"an overlay's own artifact by its parent's name": {
			func(in *link.CheckerCode) { withTests(in, "parent-plugin") },
			fixes{"give yours a name of its own": func(in *link.CheckerCode) {
				drop(in, "checkers/parent-plugin/")
				withTests(in, "my-plugin")
			}}},
		"a reference from an import under the parent's name": {
			func(in *link.CheckerCode) {
				carry(in, 0, "parent-plugin", "sam")
				byRef(in, "parent-plugin", "shared-go", "sam")
			},
			fixes{
				"declare it from the parent": fromParent,
				"drop it and the import's": func(in *link.CheckerCode) {
					drop(in, "checkers/parent-plugin/")
					unCarry(in, 0, "parent-plugin")
				},
			}},
		"a name the parent and an import both carry": {
			func(in *link.CheckerCode) { carry(in, 0, "parent-plugin", "sam") },
			fixes{
				"declare it from the parent": fromParent,
				"drop the import's":          func(in *link.CheckerCode) { unCarry(in, 0, "parent-plugin") },
			}},
		"a declaration from the parent by another publisher": {
			func(in *link.CheckerCode) { file(in, "checkers/parent-plugin/checker.yaml", parentDecl("kim")) },
			fixes{"declare it by the parent's": fromParent}},
		"a binding that names no declaration": {
			func(in *link.CheckerCode) { in.Book.Rules[0].Bindings[0].Checker = "ghost-plugin" },
			fixes{
				"declare it":                 func(in *link.CheckerCode) { withTests(in, "ghost-plugin") },
				"bind to a tool's own entry": func(in *link.CheckerCode) { in.Book.Rules[0].Bindings[0].Checker = "" },
			}},
		"a binding that names the parent's artifact undeclared": {
			func(in *link.CheckerCode) { in.Book.Rules[0].Bindings[0].Checker = "parent-plugin" },
			fixes{"declare it from the parent": fromParent}},
		"a binding of another tool": {
			func(in *link.CheckerCode) { in.Book.Rules[0].Bindings[0].Tool = "lint-b" },
			fixes{
				"bind it with the declaration's tool": func(in *link.CheckerCode) { in.Book.Rules[0].Bindings[0].Tool = "lint-a" },
				"name an artifact of its tool": func(in *link.CheckerCode) {
					withTests(in, "b-plugin")
					file(in, "checkers/b-plugin/checker.yaml", strings.Replace(strings.Replace(ownDecl, "name: own-plugin", "name: b-plugin", 1), "tool: lint-a", "tool: lint-b", 1))
					in.Book.Rules[0].Bindings[0].Checker = "b-plugin"
				},
			}},
	} {
		planted := cleanCheckers()
		c.plant(&planted)
		_, found := link.Checkers(s, planted)
		if len(found) == 0 {
			t.Errorf("%s: the plant gives no finding", name)
			continue
		}
		for _, f := range found {
			// No fix declares by reference, from an import or from the
			// parent, where the rulebook's own code sits, unless it moves
			// that code to a name of its own first.
			for _, n := range ownCode(planted) {
				there := f.File == "checkers/"+n+"/checker.yaml" || f.Entity == "checkers/"+n || strings.Contains(f.Fix, "checkers/"+n+"/checker.yaml")
				byRef := strings.Contains(f.Fix, "by reference") || strings.Contains(f.Fix, "from the parent")
				if there && byRef && !strings.Contains(f.Fix, "rename yours (") {
					t.Errorf("%s: a fix declares by reference over the rulebook's own code in checkers/%s/: %q", name, n, f.Fix)
				}
			}
			// Every way out of a plant's one finding is one this test
			// follows. Where a plant gives two, a way out of one need not
			// clear the other, so the ways are those that clear both.
			if ways := strings.Count(f.Fix, ", or ") + 1; len(found) == 1 && ways > len(c.ways) {
				t.Errorf("%s: the fix %q gives %d ways out, and the test follows %d", name, f.Fix, ways, len(c.ways))
			}
		}
		for way, fix := range c.ways {
			in := cleanCheckers()
			c.plant(&in)
			fix(&in)
			if _, found := link.Checkers(s, in); len(found) != 0 {
				t.Errorf("%s, %s: after the fix, findings remain: %+v", name, way, found)
			}
		}
	}
}

// TestTheDeclarationReaderRefusesEmptyAndControlText: the Go reader holds
// what the schema holds, whatever the Schema port does: an empty publisher or
// licence, and a control character in the publisher, the licence or the tool,
// are each refused, and no declaration is answered; so is a format character
// or a line or paragraph separator.
// Risk: a declaration that names no one, or prints as another row.
func TestTheDeclarationReaderRefusesEmptyAndControlText(t *testing.T) {
	t.Parallel()
	s := checkerSchema(t)
	cases := map[string]struct{ doc, says string }{
		"an empty publisher":              {emptyPublisher, "publisher is empty"},
		"an empty licence":                {strings.Replace(ownDecl, "licence: Apache-2.0", "licence: \"\"", 1), "licence is empty"},
		"a blank publisher":               {strings.Replace(ownDecl, "publisher: alex", "publisher: \"  \"", 1), "publisher is empty"},
		"an escape in the publisher":      {strings.Replace(ownDecl, "publisher: alex", "publisher: \"alex\\e[31m\"", 1), "publisher holds a control character"},
		"a newline in the publisher":      {strings.Replace(ownDecl, "publisher: alex", "publisher: \"alex\\nghost-plugin\"", 1), "publisher holds a control character"},
		"a bell in the licence":           {strings.Replace(ownDecl, "licence: Apache-2.0", "licence: \"Apache\\a\"", 1), "licence holds a control character"},
		"a control character in the tool": {strings.Replace(ownDecl, "tool: lint-a", "tool: \"lint\\u0001a\"", 1), "tool"},
	}
	for _, s := range unseenSeeds() {
		cases[s.name] = struct{ doc, says string }{s.doc, s.says}
	}
	for name, c := range cases {
		got, refused := link.ReadDeclaration(s, "checkers/own-plugin/checker.yaml", []byte(c.doc))
		if len(refused) == 0 || got != (model.Checker{}) || !strings.Contains(strings.Join(strs(refused), "\n"), c.says) {
			t.Errorf("%s: read %+v, refused %v; want a refusal saying %q", name, got, strs(refused), c.says)
		}
	}
}

// TestTheDocumentCapIsHeldAtItsBoundary: a declaration of exactly MaxDocument
// bytes is read, and one byte more is refused for its size, unread, so the
// cap is neither a byte short nor a byte long. Both are the clean declaration
// with a comment as long as it takes. The test that held the cap read an
// adapter, and went with the adapter reader; the cap
// stays, for the declaration and record readers. boundary: MaxDocument.
func TestTheDocumentCapIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	s := checkerSchema(t)
	padded := func(size int) []byte {
		pad := size - len(ownDecl) - len("# \n")
		if pad < 0 {
			t.Fatalf("the clean declaration is %d bytes, over %d", len(ownDecl), size)
		}
		return []byte(ownDecl + "# " + strings.Repeat("x", pad) + "\n")
	}
	exact, over := padded(link.MaxDocument), padded(link.MaxDocument+1)
	if len(exact) != link.MaxDocument || len(over) != link.MaxDocument+1 {
		t.Fatalf("the two declarations are %d and %d bytes, want %d and %d", len(exact), len(over), link.MaxDocument, link.MaxDocument+1)
	}
	if got, refused := link.ReadDeclaration(s, "checkers/own-plugin/checker.yaml", exact); len(refused) > 0 || got.Name != "own-plugin" {
		t.Errorf("a declaration of the cap: read %+v, refused %v", got, strs(refused))
	}
	got, refused := link.ReadDeclaration(s, "checkers/own-plugin/checker.yaml", over)
	if len(refused) != 1 || refused[0].Check != link.CheckSize || got != (model.Checker{}) {
		t.Errorf("a declaration one byte over the cap: read %+v, refused %v", got, strs(refused))
	}
}

// emptyPublisher is a review's declaration, which the reader once accepted
// with an empty publisher.
const emptyPublisher = "format: 3\nname: own-plugin\ntool: lint-a\npublisher: \"\"\ntests: testdata\n"

// unseen is the test's own reading of a character a listing cannot show as
// written: a control or format character, or a line or paragraph separator.
func unseen(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029
}

// unseenSeeds are a review's seeds: a publisher that is one
// zero-width space, a licence that is one word joiner, a right-to-left
// override and a line separator in a publisher, a paragraph separator and a
// byte order mark in a licence, and a zero-width space in a tool, each
// written as a YAML escape of its code point, so this file holds none of them.
func unseenSeeds() []struct{ name, doc, says string } {
	esc := func(hex string) string { return `\` + "u" + hex }
	pub := func(v string) string { return strings.Replace(ownDecl, "publisher: alex", "publisher: \""+v+"\"", 1) }
	lic := func(v string) string { return strings.Replace(ownDecl, "licence: Apache-2.0", "licence: \""+v+"\"", 1) }
	return []struct{ name, doc, says string }{
		{"a publisher of one zero-width space", pub(esc("200B")), "publisher holds a control character"},
		{"a licence of one word joiner", lic(esc("2060")), "licence holds a control character"},
		{"a right-to-left override in the publisher", pub("al" + esc("202E") + "ex"), "publisher holds a control character"},
		{"a line separator in the publisher", pub("alex" + esc("2028") + "ghost-plugin"), "publisher holds a control character"},
		{"a paragraph separator in the licence", lic("Apache" + esc("2029") + "2.0"), "licence holds a control character"},
		{"a byte order mark in the licence", lic(esc("FEFF") + "Apache-2.0"), "licence holds a control character"},
		{"a zero-width space in the tool", strings.Replace(ownDecl, "tool: lint-a", "tool: \"lint"+esc("200B")+"a\"", 1), "tool"},
		// Text that prints as nothing.
		{"a publisher of one Hangul filler", pub(esc("3164")), "publisher is empty or prints as nothing"},
		{"a publisher of one halfwidth Hangul filler", pub(esc("FFA0")), "publisher is empty or prints as nothing"},
		{"a publisher of one blank braille pattern", pub(esc("2800")), "publisher is empty or prints as nothing"},
		{"a publisher of one combining grapheme joiner", pub(esc("034F")), "publisher is empty or prints as nothing"},
		{"a licence of one Hangul choseong filler", lic(esc("115F")), "licence is empty or prints as nothing"},
	}
}

// ownCode is each name whose folder in the checkers folder holds code beside
// its declaration, where that declaration is not one by reference: none
// handed in, the rulebook's own, or one that a reader may refuse.
func ownCode(in link.CheckerCode) []string {
	var out []string
	for _, p := range in.Listing {
		rest, ok := strings.CutPrefix(p, "checkers/")
		name, file, inFolder := strings.Cut(rest, "/")
		if !ok || !inFolder || file == "checker.yaml" || slices.Contains(out, name) {
			continue
		}
		i := slices.IndexFunc(in.Files, func(f model.File) bool { return f.Path == "checkers/"+name+"/checker.yaml" })
		if i < 0 || !strings.Contains(string(in.Files[i].Data), "\nfrom: ") {
			out = append(out, name)
		}
	}
	return out
}

// shows is the test's own reading of a character a listing shows as
// something: graphic, no space, no combining mark, and no Hangul filler or
// blank braille pattern.
func shows(r rune) bool {
	switch r {
	case 0x115F, 0x1160, 0x3164, 0xFFA0, 0x2800:
		return false
	}
	return unicode.IsGraphic(r) && !unicode.IsSpace(r) && !unicode.In(r, unicode.Mn, unicode.Me)
}

// checks is the closed set of checks, for the fuzz target.
func checks() []link.Check {
	return []link.Check{link.CheckSize, link.CheckPlace, link.CheckYAML, link.CheckFormat, link.CheckKey, link.CheckConditional,
		link.CheckReference, link.CheckExpression, link.CheckShape, link.CheckSchema}
}

// strs is each refusal as a person reads it.
func strs(rs []link.Refusal) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.String()
	}
	return out
}
