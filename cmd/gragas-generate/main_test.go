package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/generate"
)

// copySchemas is the contract schemas, the consumers' lists and the fixture's
// documents in a folder of the test's own, which generate then fills.
func copySchemas(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, folder := range []string{generate.SchemaDir, generate.ConsumersDir, generate.FixtureDir} {
		from := filepath.Join("..", "..", filepath.FromSlash(folder))
		err := filepath.WalkDir(from, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(from, p)
			if err != nil {
				return err
			}
			to := filepath.Join(dir, filepath.FromSlash(folder), rel)
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			return os.WriteFile(to, data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestTheRepositorysGeneratedFilesAreCurrent: the gate itself, through the
// program. Risk: make gen-check passing on a program that checks nothing.
func TestTheRepositorysGeneratedFilesAreCurrent(t *testing.T) {
	t.Parallel()
	code, out, errs := do("--check", "-C", filepath.Join("..", ".."))
	if code != exitDone || !strings.Contains(out, "current") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	// It ends on the count the gate runner reads (#695): the files compared.
	if !regexp.MustCompile(`\ncount: [1-9][0-9]* files\n$`).MatchString(out) {
		t.Errorf("the check does not end on a count of the files it compared:\n%s", out)
	}
}

// TestGeneratingLeavesNoDiffAndAHandEditFails: a clean run is followed by a
// check that passes; a planted hand edit of a Go file, of a zod file, a
// deleted file, a file left over and a changed schema each fail the check,
// name the file and write nothing (gates.own-rules.3, supply.provenance.4).
func TestGeneratingLeavesNoDiffAndAHandEditFails(t *testing.T) {
	t.Parallel()
	dir := copySchemas(t)
	if code, _, errs := do("-C", dir); code != exitDone {
		t.Fatalf("generate: exit %d\n%s", code, errs)
	}
	if code, out, errs := do("--check", "-C", dir); code != exitDone || !strings.Contains(out, "current") {
		t.Fatalf("the control: exit %d\n%s%s", code, out, errs)
	}
	goFile := generate.GoDir + "entity_finding.gen.go"
	tsFile := generate.TSDir + "entity-finding.gen.ts"
	for _, tc := range []struct {
		name, file, edit, want string
	}{
		{"a hand edit of a Go file", goFile, "package contract", goFile},
		{"a hand edit of a zod file", tsFile, "import ", tsFile},
		{"a deleted file", tsFile, "", tsFile},
		{"a file left over", generate.TSDir + "gone.gen.ts", "x", "leftover " + generate.TSDir + "gone.gen.ts"},
		{"a schema changed", generate.SchemaDir + "entity-finding.schema.json", `"fix"`, "stale " + goFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			do("-C", dir)
			path := filepath.Join(dir, filepath.FromSlash(tc.file))
			switch tc.name {
			case "a deleted file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "a file left over":
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			default:
				old := read(t, dir, tc.file)
				repl := tc.edit + " // by hand"
				if strings.HasSuffix(tc.file, ".schema.json") {
					repl = `"fix_renamed"`
				}
				if err := os.WriteFile(path, []byte(strings.Replace(old, tc.edit, repl, 1)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := read2(path)
			code, out, _ := do("--check", "-C", dir)
			if code != exitFailed || !strings.Contains(out, tc.want) {
				t.Errorf("exit %d, output %q, want a failure naming %q", code, out, tc.want)
			}
			if read2(path) != before {
				t.Error("--check wrote a file")
			}
		})
	}
}

func read2(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// TestGeneratingRepairsAHandEditAndRemovesALeftover: without --check the
// command puts the files right.
func TestGeneratingRepairsAHandEditAndRemovesALeftover(t *testing.T) {
	t.Parallel()
	dir := copySchemas(t)
	do("-C", dir)
	edited := filepath.Join(dir, filepath.FromSlash(generate.GoDir), "common.gen.go")
	left := filepath.Join(dir, filepath.FromSlash(generate.TSDir), "gone.gen.ts")
	for p, b := range map[string]string{edited: "package model // edited", left: "x"} {
		if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, errs := do("-C", dir); code != exitDone {
		t.Fatalf("exit %d\n%s", code, errs)
	}
	if strings.Contains(read2(edited), "edited") {
		t.Error("the hand edit stayed")
	}
	if _, err := os.Stat(left); err == nil {
		t.Error("the leftover stayed")
	}
}

// TestAHandFileInTheModelFolderFailsGenerate: the review's plant. A
// hand method on a contract type, in the model's folder, reads a display-only
// field in a condition; the decides walk does not walk that package, so the
// generator refuses the file in both modes, names it, writes nothing and
// leaves it in place. A test file there, a generated name the schemas no
// longer make, and a file that is not Go are the controls. Risk: a hand file
// that carries a display-only field to a decision past the walk.
func TestAHandFileInTheModelFolderFailsGenerate(t *testing.T) {
	t.Parallel()
	const plant = "package contract\n\nfunc (v Health) Plant() bool {\n\tif v.Resolved {\n\t\treturn true\n\t}\n\treturn false\n}\n"
	for _, args := range [][]string{{"--check"}, {}} {
		t.Run("args "+strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			do("-C", dir)
			planted := filepath.Join(dir, filepath.FromSlash(generate.GoDir), "zz_plant.go")
			if err := os.WriteFile(planted, []byte(plant), 0o644); err != nil {
				t.Fatal(err)
			}
			edited := filepath.Join(dir, filepath.FromSlash(generate.GoDir), "common.gen.go")
			if err := os.WriteFile(edited, []byte("package contract // edited"), 0o644); err != nil {
				t.Fatal(err)
			}
			code, out, _ := do(append(args, "-C", dir)...)
			if code != exitFailed || !strings.Contains(out, "hand-written "+generate.GoDir+"zz_plant.go") {
				t.Errorf("exit %d, output %q, want a refusal naming the planted file", code, out)
			}
			if read2(planted) != plant {
				t.Error("the planted file was changed or removed")
			}
			if read2(edited) != "package contract // edited" {
				t.Error("a refused run wrote a generated file")
			}
		})
	}
	t.Run("controls", func(t *testing.T) {
		t.Parallel()
		dir := copySchemas(t)
		do("-C", dir)
		for name, body := range map[string]string{"zz_plant_test.go": "package contract\n", "NOTES.md": "x"} {
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(generate.GoDir), name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if code, out, errs := do("--check", "-C", dir); code != exitDone || !strings.Contains(out, "current") {
			t.Errorf("a test file and a file that is not Go: exit %d\n%s%s", code, out, errs)
		}
		gone := generate.GoDir + "gone.gen.go"
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(gone)), []byte("package contract\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, out, _ := do("--check", "-C", dir); code != exitFailed || !strings.Contains(out, "leftover "+gone) || strings.Contains(out, "hand-written") {
			t.Errorf("a generated name no schema makes is a leftover, not a hand file: exit %d, output %q", code, out)
		}
	})
}

// TestAConsumerReadingAMissingFieldFailsGenerate: the command refuses a
// consumer list the schemas do not hold, and writes nothing.
func TestAConsumerReadingAMissingFieldFailsGenerate(t *testing.T) {
	t.Parallel()
	dir := copySchemas(t)
	p := filepath.Join(dir, filepath.FromSlash(generate.ConsumersDir), "explorer.json")
	if err := os.WriteFile(p, []byte(`{"consumer":"explorer","reads":[{"document":"bundle","fields":["rules[].colour"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := do("-C", dir)
	if code != exitFailed || !strings.Contains(errs, "rules[].colour") {
		t.Errorf("exit %d, stderr %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "model")); err == nil {
		t.Error("a failed run wrote files")
	}
}

// A flag that is not one, and a stray argument, are usage.
func TestGenerateWithAStrayArgumentIsUsage(t *testing.T) {
	t.Parallel()
	if code, _, _ := do("extra"); code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
}

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func do(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func read(t *testing.T, dir, f string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, f))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
