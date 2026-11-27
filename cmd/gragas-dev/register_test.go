package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// theRegister reads the register of the tree through its schema and its loader.
func theRegister(t *testing.T) *layers.Register {
	t.Helper()
	repo := os.DirFS("../..")
	data, err := os.ReadFile("../../" + layers.RegisterFile)
	if err != nil {
		t.Fatal(err)
	}
	set, err := jsonschema.Compile(repo, "rulebook/schema", layers.RegisterSchema+".schema.json")
	if err != nil {
		t.Fatal(err)
	}
	validate := func(doc any) []string {
		var out []string
		for _, p := range set.Validate(layers.RegisterSchema+".schema.json", doc) {
			out = append(out, fmt.Sprintf("line %d: %s", p.Line, p.Msg))
		}
		return out
	}
	r, err := layers.ParseRegister(data, validate)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestTheRegisterOfLibrariesHolds: the register of the tree reads through its schema and its loader, the feed holds
// only chosen rows, and no dropped name is a row's module.
//
// What it costs when this fails: a row with no source, an unripe pin or a ban that never compiles reaches the Go
// preset, and a repo's lint refuses a library nobody has proven.
func TestTheRegisterOfLibrariesHolds(t *testing.T) {
	t.Parallel()
	r := theRegister(t)
	if len(r.Rows) < 47 || len(r.Tools) < 17 || len(r.Dropped) < 8 {
		t.Fatalf("the register holds %d rows, %d tools and %d dropped, fewer than the draft's", len(r.Rows), len(r.Tools), len(r.Dropped))
	}
	fed := 0
	for _, f := range r.Feed() {
		fed++
		for _, l := range r.Rows {
			if l.Key == f.Key && l.State != layers.StateChosen {
				t.Errorf("row %s feeds the preset in state %s", l.Key, l.State)
			}
		}
	}
	if fed == 0 {
		t.Error("no row feeds the preset")
	}
	for _, d := range r.Dropped {
		for _, l := range r.Rows {
			if strings.EqualFold(l.Module, d.Name) {
				t.Errorf("%s is dropped and row %s calls it", d.Name, l.Key)
			}
		}
	}
}

// schemaProblems is what the schema named answers for the YAML text.
func schemaProblems(t *testing.T, schema, text string) []string {
	t.Helper()
	set, err := jsonschema.Compile(os.DirFS("../.."), "rulebook/schema", schema+".schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	validate := func(doc any) []string {
		for _, p := range set.Validate(schema+".schema.json", doc) {
			out = append(out, p.Msg)
		}
		return out
	}
	if schema == layers.UsesSchema {
		_, _ = layers.ParseUses([]byte(text), validate)
	} else {
		_, _ = layers.ParseLibrary([]byte(text), validate)
	}
	return out
}

// TestTheUsesAndLibrarySchemasHoldAValidFileAndRefuseAnInvalidOne: a small uses file and a small library file pass
// their schemas, and a pick with no register key, a finding with a state the schema does not know, a job that is not
// for the repo with no source, a package with no banned call and a key the schema does not know are each refused.
//
// What it costs when this fails: a file that breaks its shape reaches the loader unchecked by the schema, so the
// schema is no second check of the file a repo carries.
func TestTheUsesAndLibrarySchemasHoldAValidFileAndRefuseAnInvalidOne(t *testing.T) {
	t.Parallel()
	const uses = "repo: app\npicks:\n  - {item: x, register_key: yaml, job: j, scope: s, ban: b}\nfindings:\n  - {where: a.go, what: w, state: open}\n"
	const lib = "packages:\n  - {package: write, job: j, banned_calls: [os.CreateTemp]}\n"
	if got := schemaProblems(t, layers.UsesSchema, uses); len(got) != 0 {
		t.Errorf("a valid uses file: %q", got)
	}
	if got := schemaProblems(t, layers.LibrarySchema, lib); len(got) != 0 {
		t.Errorf("a valid library file: %q", got)
	}
	cases := []struct{ name, schema, text string }{
		{"a pick with no register key", layers.UsesSchema, "repo: app\npicks:\n  - {item: x, job: j, scope: s, ban: b}\n"},
		{"a finding state it does not know", layers.UsesSchema, strings.Replace(uses, "state: open", "state: pending", 1)},
		{"a job not for the repo with no source", layers.UsesSchema, uses + "not_for_this_repo:\n  - {item: x, why: w}\n"},
		{"a key it does not know", layers.UsesSchema, uses + "colour: blue\n"},
		{"a package with no banned call", layers.LibrarySchema, "packages:\n  - {package: write, job: j, banned_calls: []}\n"},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		if got := schemaProblems(t, tc.schema, tc.text); len(got) == 0 {
			t.Errorf("%s: the schema passed it", tc.name)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
}

// TestTheRegisterSchemaRequiresALinterOfARowThatBansAnImportOrACall: a row whose ban has deny or forbid and no
// linter fails the schema itself, and one that names its linter, or bans nothing, passes it.
//
// What it costs when this fails: the loader alone holds that a ban names the linter that enforces it, so another
// reader of the register, which has only the schema, takes a ban nothing enforces.
func TestTheRegisterSchemaRequiresALinterOfARowThatBansAnImportOrACall(t *testing.T) {
	t.Parallel()
	set, err := jsonschema.Compile(os.DirFS("../.."), "rulebook/schema", layers.RegisterSchema+".schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc := func(ban map[string]any) any {
		row := map[string]any{
			"key": "x", "job": "j", "scope": "s", "gap": "g", "source": "https://example.com/x", "state": "chosen",
			"use": map[string]any{"module": "example.com/x", "version": "v1.0.0", "package": "x"},
			"ban": ban,
		}
		return map[string]any{"part": "go", "libraries": []any{row}, "dropped": []any{}}
	}
	file := layers.RegisterSchema + ".schema.json"
	passes := map[string]map[string]any{
		"a ban that names its linter":    {"deny": []any{"example.com/y"}, "linter": "depguard"},
		"a ban that bans nothing":        {"note": "review only"},
		"a forbid that names its linter": {"forbid": []any{map[string]any{"pattern": "^x$"}}, "linter": "forbidigo"},
	}
	for name, ban := range passes {
		if got := set.Validate(file, doc(ban)); len(got) != 0 {
			t.Errorf("%s: the schema refused it: %v", name, got)
		}
	}
	fails := map[string]map[string]any{
		"a deny with no linter":   {"deny": []any{"example.com/y"}},
		"a forbid with no linter": {"forbid": []any{map[string]any{"pattern": "^x$"}}},
	}
	for name, ban := range fails {
		if got := set.Validate(file, doc(ban)); len(got) == 0 {
			t.Errorf("%s: the schema passed it", name)
		}
	}
}

// usesProblems reads a uses file through its schema.
func usesOf(t *testing.T, data []byte) (*layers.Uses, error) {
	t.Helper()
	set, err := jsonschema.Compile(os.DirFS("../.."), "rulebook/schema", layers.UsesSchema+".schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return layers.ParseUses(data, func(doc any) []string {
		var out []string
		for _, p := range set.Validate(layers.UsesSchema+".schema.json", doc) {
			out = append(out, fmt.Sprintf("line %d: %s", p.Line, p.Msg))
		}
		return out
	})
}

// TestARealMatrixReadsAsAUsesFileAndComesBackAsTheSameMatrix: the published matrix of one repo, with the picks whose
// banned calls were prose given patterns and the picks that review only given one, is converted to a uses file in
// testdata. It reads with no problem, holds to the register, and generates the matrix it came from.
//
// What it costs when this fails: the uses file cannot hold every fact of a published matrix, so the matrix cannot
// be generated from it, and the canonical form is only a promise.
func TestARealMatrixReadsAsAUsesFileAndComesBackAsTheSameMatrix(t *testing.T) {
	t.Parallel()
	file, err := os.ReadFile("testdata/uses/go-run.uses.yaml")
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := os.ReadFile("testdata/uses/go-run.matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	u, err := usesOf(t, file)
	if err != nil {
		t.Fatalf("the converted matrix does not read: %v", err)
	}
	if len(u.Picks) != 10 || len(u.NotFor) != 6 || len(u.Findings) != 4 || len(u.Blockers) == 0 {
		t.Errorf("read %d picks, %d not-for, %d findings and %d blockers, want 10, 6, 4 and some", len(u.Picks), len(u.NotFor), len(u.Findings), len(u.Blockers))
	}
	if err := u.Hold(theRegister(t)); err != nil {
		t.Errorf("the uses file over the register: %v", err)
	}
	if got := u.CompareMatrix(matrix); len(got) != 0 {
		t.Errorf("the matrix differs from its uses file: %q", got)
	}
	again, err := u.Matrix()
	if err != nil {
		t.Fatal(err)
	}
	if got := u.CompareMatrix(again); len(got) != 0 {
		t.Errorf("Matrix() is not the matrix the file came from: %q", got)
	}
}

// patternSamples are a few lines of code a pick's pattern must match and a few it must not, by the pick's item: the
// hand-written code the pick replaces, and the library's own use.
func patternSamples() map[string]struct{ match, miss []string } {
	return map[string]struct{ match, miss []string }{
		"golang.org/x/mod v0.42.0": {
			match: []string{`re := regexp.MustCompile(` + "`" + `^v?(\d+)\.(\d+)\.(\d+)` + "`" + `)`, `var semver = "[0-9]+\.[0-9]+\.[0-9]+"`, `b, err := os.ReadFile(filepath.Join(dir, "go.mod"))`},
			miss:  []string{`parts := strings.Split(v, ".")`, `module.IsPseudoVersion(v)`, `os.ReadFile("README.md")`},
		},
		"std runtime/debug": {
			match: []string{`exec.Command("git", "describe", "--tags")`, `os.ReadFile(".git/HEAD")`},
			miss:  []string{`if s.Key == "vcs.revision" { rev = s.Value }`, `info, ok := debug.ReadBuildInfo()`},
		},
		"std encoding/json/v2 and jsontext (Go 1.27)": {
			match: []string{`import "encoding/json"`},
			miss:  []string{`import "encoding/json/v2"`, `import "encoding/json/jsontext"`},
		},
		"golangci-lint v2.15.0": {
			match: []string{`p, err := build.ImportDir(dir, 0)`, `data, _ := os.ReadFile("lint-debt.txt")`},
			miss:  []string{`golangci-lint run ./...`, `fmt.Println("build ok")`},
		},
		"gitleaks v8.30.1": {
			match: []string{`regexp.MustCompile("(?i)secret[a-z]*")`, `leak := regexp.MustCompile(` + "`" + `token=\w+` + "`" + `)`},
			miss:  []string{`regexp.MustCompile("^v[0-9]+")`, `fmt.Println("token")`},
		},
		"go-task v3.54.0 (the repo's own Taskfile)": {
			match: []string{`exec.Command("make", "check")`, `os.WriteFile(".stamp", nil, 0o600)`, `tierPool := newPool(4)`},
			miss:  []string{`exec.Command("git", "status")`, `fmt.Println("stamped")`},
		},
		"changie v1.26.0": {
			match: []string{`git log --format=%s`, `parse conventional commit subjects into a changelog`},
			miss:  []string{`git status`, `a changelog entry file`},
		},
		"gotestsum v1.13.0": {
			match: []string{`exec.Command("go", "test", "-json", "./...")`, "Action string `json:\"Action\"`", `go tool test2json`},
			miss:  []string{`json.Marshal(v)`, `exec.Command("go", "vet")`},
		},
		"govulncheck v1.9.0": {
			match: []string{`http.Get("https://api.osv.dev/v1/query")`, `https://vuln.go.dev/index/db.json`},
			miss:  []string{`https://example.invalid/x`, `fmt.Println("vulnerable")`},
		},
		"go-licenses v2.0.1": {
			match: []string{`licenseFiles := walk(dir)`, `filepath.Walk(root, func(p string, i fs.FileInfo, e error) error { return find(p, "LICENSE") })`},
			miss:  []string{`fmt.Println("done")`, `os.ReadFile("NOTICE")`},
		},
	}
}

// TestEachPatternOfTheFixtureMatchesTheCodeItNamesAndNotTheCodeItDoesNot: every pick of the converted matrix that carries
// a pattern has a few lines of code the pattern matches and a few it does not, and the pattern does both, so a pattern
// that cannot match, or that matches the library's own use, is red.
//
// What it costs when this fails: a review pattern that never matches is a check that finds nothing, and one that
// matches the library itself flags the code it asks for.
func TestEachPatternOfTheFixtureMatchesTheCodeItNamesAndNotTheCodeItDoesNot(t *testing.T) {
	t.Parallel()
	file, err := os.ReadFile("testdata/uses/go-run.uses.yaml")
	if err != nil {
		t.Fatal(err)
	}
	u, err := usesOf(t, file)
	if err != nil {
		t.Fatal(err)
	}
	samples := patternSamples()
	checked := 0
	for _, pk := range u.Picks {
		if pk.Pattern == "" {
			continue
		}
		s, ok := samples[pk.Item]
		if !ok {
			t.Errorf("%s carries a pattern and has no samples", pk.Item)
			continue
		}
		re, err := regexp.Compile(pk.Pattern)
		if err != nil {
			t.Fatalf("%s: %v", pk.Item, err)
		}
		if len(s.match) == 0 || len(s.miss) == 0 {
			t.Errorf("%s needs code its pattern matches and code it does not", pk.Item)
		}
		for _, line := range s.match {
			if !re.MatchString(line) {
				t.Errorf("%s: the pattern %q does not match %q", pk.Item, pk.Pattern, line)
			}
		}
		for _, line := range s.miss {
			if re.MatchString(line) {
				t.Errorf("%s: the pattern %q matches %q, which it must not", pk.Item, pk.Pattern, line)
			}
		}
		checked++
	}
	if checked != len(samples) || checked != 10 {
		t.Errorf("checked %d patterns of %d samples, want 10 of 10", checked, len(samples))
	}
}
