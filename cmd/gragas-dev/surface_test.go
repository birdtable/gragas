package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/dev/surface"
)

// fixtureFiles is a repository of the two surfaced packages, one export each.
func fixtureFiles() map[string]string {
	return map[string]string{
		"internal/port/port.go":               "package port\n\n// Clock tells the time.\ntype Clock interface{ Now() int64 }\n",
		"internal/model/model.go":             "package model\n\n// Word is a closing word.\ntype Word string\n",
		"internal/model/contract/contract.go": "package contract\n\n// Slug is a slug.\ntype Slug string\n",
		// gragas-generate writes the list of the TypeScript contract package.
		"explorer/packages/contract/exports.gen.json": `[{"name":"Slug","kind":"const","signature":"zod schema","summary":"A slug"}]`,
		// A test file is never listed.
		"internal/model/model_test.go": "package model\n\nfunc TestHidden() {}\n\n// Seen is in a test.\ntype Seen int\n",
	}
}

// fixture writes the files into a folder of the test's own.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

var digestInAdvice = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

// marksWith is the marks file text for rows (a "marks:" block) and a change
// record of one line: digest, when it is not empty.
func marksWith(rows, digest string) string {
	text := rows
	if digest != "" {
		text += "changes:\n  - {digest: \"" + digest + "\", reason: \"the fixture changed for a test\"}\n"
	}
	return text
}

// record writes the marks file with the digest the generator asks for, as an
// author does: run it, read what it asks for, add that line.
func record(t *testing.T, dir, rows string) {
	t.Helper()
	write(t, dir, surface.MarksFile, marksWith(rows, ""))
	code, _, errs := do("surface", "-C", dir)
	digest := digestInAdvice.FindString(errs)
	if code != exitFailed || digest == "" {
		t.Fatalf("the generator named no digest: exit %d, %q", code, errs)
	}
	write(t, dir, surface.MarksFile, marksWith(rows, digest))
	if code, _, errs := do("surface", "-C", dir); code != exitDone {
		t.Fatalf("recorded: exit %d, %q", code, errs)
	}
}

const noRows = "marks: []\n"

// listed is a fixture with its listing written and its change recorded.
func listed(t *testing.T) string {
	t.Helper()
	dir := fixture(t, fixtureFiles())
	record(t, dir, noRows)
	return dir
}

// TestSurfaceWritesTheListingAndCheckPasses is the clean control: a listing
// with its change recorded passes --check, writes nothing under it, and leaves
// test files out.
func TestSurfaceWritesTheListingAndCheckPasses(t *testing.T) {
	t.Parallel()
	dir := listed(t)
	text := read(t, dir, surface.File)
	for _, want := range []string{"## internal/port", "## internal/model", "## internal/model/contract", "## explorer/packages/contract", "`Slug` (const) **experimental**", "`Clock.Now` (method) **experimental**", "`Word` (type) **experimental**", "the fixture changed for a test"} {
		if !strings.Contains(text, want) {
			t.Errorf("the listing lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Seen") || strings.Contains(text, "TestHidden") {
		t.Errorf("a test file was listed:\n%s", text)
	}
	if code, out, errs := do("surface", "--check", "-C", dir); code != exitDone || !strings.Contains(out, "current") {
		t.Errorf("a clean listing: exit %d, %q, %q", code, out, errs)
	}
	if code, _, _ := do("surface", "-C", dir); code != exitDone {
		t.Errorf("regenerating a current listing: exit %d", code)
	}
	if read(t, dir, surface.File) != text {
		t.Error("regenerating a current listing changed it")
	}
}

// TestAPlantedNewExportFailsUntilTheListingAndAReasonChange: the code gains an
// export. --check names it; regenerating without a change record writes the
// listing but fails naming the digest; then --check still fails on the record
// alone; recording it passes.
func TestAPlantedNewExportFailsUntilTheListingAndAReasonChange(t *testing.T) {
	t.Parallel()
	dir := listed(t)
	write(t, dir, "internal/port/port.go", fixtureFiles()["internal/port/port.go"]+"\n// Extra is planted.\nfunc Extra() {}\n")

	code, out, _ := do("surface", "--check", "-C", dir)
	if code != exitFailed || !strings.Contains(out, "not listed: internal/port Extra") {
		t.Fatalf("a planted export: exit %d, %q", code, out)
	}
	if code, _, errs := do("surface", "-C", dir); code != exitFailed || !digestInAdvice.MatchString(errs) {
		t.Errorf("regenerating with no record: exit %d, %q", code, errs)
	}
	if !strings.Contains(read(t, dir, surface.File), "`Extra` (func) **experimental**") {
		t.Error("the new export was not written as experimental")
	}
	code, out, _ = do("surface", "--check", "-C", dir)
	if code != exitFailed || !strings.Contains(out, "does not say why") || strings.Contains(out, "not listed") {
		t.Errorf("listed but unrecorded: exit %d, %q", code, out)
	}
	if strings.Contains(read(t, dir, surface.MarksFile), "Extra") {
		t.Error("the generator wrote the authored marks file")
	}
	record(t, dir, noRows)
	if code, out, errs := do("surface", "--check", "-C", dir); code != exitDone {
		t.Errorf("listed and recorded: exit %d, %q, %q", code, out, errs)
	}
}

// TestAPlantedRemovedExportFails: the code loses one.
func TestAPlantedRemovedExportFails(t *testing.T) {
	t.Parallel()
	dir := listed(t)
	write(t, dir, "internal/model/model.go", "package model\n")
	code, out, _ := do("surface", "--check", "-C", dir)
	if code != exitFailed || !strings.Contains(out, "listed but gone: internal/model Word") {
		t.Errorf("a removed export: exit %d, %q", code, out)
	}
}

const stableWord = "marks:\n  - {package: internal/model, name: Word, mark: stable, reason: \"every run ends on a closing word\"}\n"

// TestAPlantedMarkChangeFailsUntilItsReasonIsRecorded: a mark added in the
// marks file; the listing follows it, and the change record must name the new
// digest before --check passes.
func TestAPlantedMarkChangeFailsUntilItsReasonIsRecorded(t *testing.T) {
	t.Parallel()
	dir := listed(t)
	before := read(t, dir, surface.MarksFile)
	write(t, dir, surface.MarksFile, strings.Replace(before, noRows, stableWord, 1))
	if read(t, dir, surface.MarksFile) == before {
		t.Fatal("the planted mark changed nothing")
	}
	if code, out, _ := do("surface", "--check", "-C", dir); code != exitFailed || !strings.Contains(out, "does not say why") {
		t.Fatalf("a mark with no recorded change passed: exit %d, %q", code, out)
	}
	if code, _, _ := do("surface", "-C", dir); code != exitFailed {
		t.Fatal("regenerating a mark with no recorded change succeeded")
	}
	if !strings.Contains(read(t, dir, surface.File), "`Word` (type) **stable**") {
		t.Error("the listing did not follow the marks file")
	}
	record(t, dir, stableWord)
	if code, out, errs := do("surface", "--check", "-C", dir); code != exitDone {
		t.Errorf("a recorded mark change: exit %d, %q, %q", code, out, errs)
	}
}

// TestAHandEditedListingFails: the listing is generated, so a mark typed into
// it is a stale file.
func TestAHandEditedListingFails(t *testing.T) {
	t.Parallel()
	dir := listed(t)
	text := read(t, dir, surface.File)
	edited := strings.Replace(text, "`Word` (type) **experimental**", "`Word` (type) **stable**", 1)
	if edited == text {
		t.Fatal("the planted edit changed nothing")
	}
	write(t, dir, surface.File, edited)
	if code, out, _ := do("surface", "--check", "-C", dir); code != exitFailed || !strings.Contains(out, "changed: internal/model Word") {
		t.Errorf("a hand-edited listing: exit %d, %q", code, out)
	}
	if code, _, _ := do("surface", "-C", dir); code != exitDone {
		t.Error("regenerating did not put the listing right")
	}
	if read(t, dir, surface.File) != text {
		t.Error("regenerating kept the hand edit")
	}
}

// TestAMissingListingFails: no file is not a clean listing.
func TestAMissingListingFails(t *testing.T) {
	t.Parallel()
	dir := fixture(t, fixtureFiles())
	code, out, _ := do("surface", "--check", "-C", dir)
	if code != exitFailed || !strings.Contains(out, "missing") {
		t.Errorf("a missing listing: exit %d, %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, surface.File)); err == nil {
		t.Error("--check wrote the file")
	}
}

// TestASurfaceMarksFileThatIsWrongIsRefused: a mark with no reason is an error
// the command reports, and no listing is written.
func TestASurfaceMarksFileThatIsWrongIsRefused(t *testing.T) {
	t.Parallel()
	dir := fixture(t, fixtureFiles())
	write(t, dir, surface.MarksFile, "marks:\n  - {package: internal/model, name: Word, mark: stable}\n")
	code, _, errs := do("surface", "-C", dir)
	if code != exitFailed || !strings.Contains(errs, "no reason") {
		t.Errorf("a mark with no reason: exit %d, %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, surface.File)); err == nil {
		t.Error("a refused marks file wrote the listing")
	}
}

// TestSurfaceTakesNoReasonFlagAndNoStrayArgument: the marks file is the only
// place a reason is written.
func TestSurfaceTakesNoReasonFlagAndNoStrayArgument(t *testing.T) {
	t.Parallel()
	dir := fixture(t, fixtureFiles())
	if code, _, _ := do("surface", "--reason", "a b c", "-C", dir); code != exitUsage {
		t.Errorf("--reason: exit %d, want usage", code)
	}
	if code, _, _ := do("surface", "stray", "-C", dir); code != exitUsage {
		t.Errorf("a stray argument: exit %d, want usage", code)
	}
}

// TestEveryPortAndTheModelAreInTheListingWithMarks: the ports are the layer
// table's, the model leaf is the model's. Each appears in the committed
// listing as a type with a mark, and the model has exports to appear. The
// count is the table's alone (gragas#2248): that the table names every field
// of port.Ports is held by arch.ports.15 and internal/port's own test, so
// this row asks only that it names one.
func TestEveryPortAndTheModelAreInTheListingWithMarks(t *testing.T) {
	t.Parallel()
	table, err := os.ReadFile(filepath.Join("..", "..", layers.File))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := layers.Parse(table)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := os.ReadFile(filepath.Join("..", "..", surface.File))
	if err != nil {
		t.Fatal(err)
	}
	text := string(listing)
	if len(tb.Ports.List) == 0 {
		t.Fatalf("%s names no port", layers.File)
	}
	for _, p := range tb.Ports.List {
		if !regexp.MustCompile("(?m)^- `" + p.Name + "` \\(type\\) \\*\\*(stable|experimental|internal)\\*\\*").MatchString(text) {
			t.Errorf("port %s is not in %s with a mark", p.Name, surface.File)
		}
	}
	if !strings.Contains(text, "## internal/model\n") {
		t.Fatal("the model leaf has no section")
	}
	model := text[strings.Index(text, "## internal/model\n"):]
	if n := strings.Count(model, "\n- `"); n == 0 {
		t.Error("the model section lists no export")
	}
}

// TestTheCommittedListingIsCurrent: the listing in the repository matches its
// code, recorded. This is what make gen-check holds on every push.
func TestTheCommittedListingIsCurrent(t *testing.T) {
	t.Parallel()
	code, out, errs := do("surface", "--check", "-C", filepath.Join("..", ".."))
	if code != exitDone {
		t.Errorf("exit %d: %s %s", code, out, errs)
	}
	// The count the gate runner reads: the listing compared.
	if _, n := counted(t, out, "files"); n != 1 {
		t.Errorf("a count of %d files compared, want the listing\n%s", n, out)
	}
}

// FuzzTheContractExportsList: whatever bytes the exports list holds, reading
// it returns sections or an error and never panics.
func FuzzTheContractExportsList(f *testing.F) {
	f.Add([]byte(fixtureFiles()[contractExports]))
	f.Add([]byte(`[{"name":1}]`))
	f.Add([]byte(`{`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, filepath.FromSlash(contractExports))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		_, _ = withContractPackage(root, nil)
	})
}
