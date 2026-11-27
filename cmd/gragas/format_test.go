package main

import (
	"encoding/json"
	"github.com/birdtable/gragas/internal/config"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"

	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The format schemas of v3 (M1-01) as main hands them to the Schema port: read
// through the Assets port, never from the file system. The planted breaks run
// in internal/host/jsonschema, which may write JSON; here the valid examples
// are checked as the bytes they are.

// assetsDir is the folder host/assets embeds from, from this package's folder.
const assetsDir = "../../internal/host/assets"

// schemaRoot is the folder of the Assets port that holds the format schemas.
const schemaRoot = "format/schema"

// examples is the folder of the goldens, <kind>.valid.json each.
const examples = "../../internal/testdata/format-v3"

// kinds are the file kinds of section 7.1 and the schema of each.
func kinds() map[string]string {
	return map[string]string{
		"checker":      "v3/checker.schema.json",
		"identity":     "v3/identity.schema.json",
		"structure":    "v3/structure.schema.json",
		"terms":        "v3/terms.schema.json",
		"rules":        "v3/rules.schema.json",
		"block":        "v3/block.schema.json",
		"sources":      "v3/sources.schema.json",
		"decisions":    "v3/decisions.schema.json",
		"exclusions":   "v3/exclusions.schema.json",
		"proposals":    "v3/proposals.schema.json",
		"translations": "v3/translations.schema.json",
		"history":      "v3/history.schema.json",
	}
}

// schemaNames are the schemas the Schema port is handed: the file kinds, the
// rule they share and the shapes they all use.
func schemaNames() []string {
	out := []string{"v3/common.schema.json", "v3/rule.schema.json"}
	for _, n := range kinds() {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func kindNames() []string {
	var out []string
	for k := range kinds() {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// valid is the documents of a kind's valid example file, each as its bytes.
func valid(t *testing.T, kind string) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(path.Join(examples, kind+".valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out []json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil || len(out) == 0 {
		t.Fatalf("%s.valid.json holds a non-empty array of documents: %v", kind, err)
	}
	return out
}

// TestMainsSchemaPortChecksEveryValidExampleAndRejectsAnUnknownSchema: the
// Schema port main fills compiles every embedded format schema, and a schema
// that does not compile would fail the first Check, so a pass here shows all
// of them compiled from the Assets port.
func TestMainsSchemaPortChecksEveryValidExampleAndRejectsAnUnknownSchema(t *testing.T) {
	t.Parallel()
	s := world(nil, config.Config{}).Schema
	if s == nil {
		t.Fatal("main leaves the Schema port empty")
	}
	for _, kind := range kindNames() {
		for i, d := range valid(t, kind) {
			got, err := s.Check(kinds()[kind], d)
			if err != nil || len(got) != 0 {
				t.Errorf("Check(%s) document %d = %v, %v, want none", kind, i, got, err)
			}
		}
	}
	// The guard can fail: a document the schema forbids is told, not passed.
	if got, err := s.Check(kinds()["identity"], []byte(`{"format": 2}`)); err != nil || len(got) == 0 {
		t.Errorf("Check(identity, format 2) = %v, %v, want problems", got, err)
	}
	if got, err := s.Check("v4/identity.schema.json", []byte(`{}`)); err != nil || len(got) != 1 {
		t.Errorf("Check of a schema that was not compiled = %v, %v, want one problem", got, err)
	}
}

// TestTheEmbeddedFormatSchemasAreTheFilesBesideHostAssets: what the Assets
// port answers under format/schema is the folder on disk, file for file, and
// nothing else is there.
func TestTheEmbeddedFormatSchemasAreTheFilesBesideHostAssets(t *testing.T) {
	t.Parallel()
	a := world(nil, config.Config{}).Assets
	listed, err := a.List(schemaRoot)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	err = fs.WalkDir(os.DirFS(assetsDir), schemaRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			onDisk = append(onDisk, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(onDisk)
	if !slices.Equal(listed, onDisk) {
		t.Fatalf("the Assets port lists %v, the folder holds %v", listed, onDisk)
	}
	for _, p := range listed {
		got, err := a.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(path.Join(assetsDir, p))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s differs between the Assets port and the disk", p)
		}
	}
}

// TestTheFakeSchemaPortCompilesTheSameNamesAsTheAssetsHold: the fake Schema is
// seeded with the names the Assets port lists, read through the port.
func TestTheFakeSchemaPortCompilesTheSameNamesAsTheAssetsHold(t *testing.T) {
	t.Parallel()
	a := world(nil, config.Config{}).Assets
	listed, err := a.List(schemaRoot)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, p := range listed {
		b, err := a.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		files[p] = string(b)
	}
	w := porttest.NewWorld(t)
	w.SeedAssets(files)
	ports := w.Ports()
	fakeListed, err := ports.Assets.List(schemaRoot)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range fakeListed {
		names = append(names, strings.TrimPrefix(p, schemaRoot+"/"))
	}
	sort.Strings(names)
	if !slices.Equal(names, schemaNames()) {
		t.Fatalf("the Assets port lists the schemas %v, want %v", names, schemaNames())
	}
	w.SeedSchemas(names...)
	for _, kind := range kindNames() {
		for _, d := range valid(t, kind) {
			if got, err := ports.Schema.Check(kinds()[kind], d); err != nil || len(got) != 0 {
				t.Errorf("the fake Check(%s) = %v, %v, want none", kind, got, err)
			}
		}
	}
	if got, err := ports.Schema.Check("v4/identity.schema.json", []byte(`{}`)); err != nil || len(got) != 1 {
		t.Errorf("the fake Check of a schema that was not compiled = %v, %v, want one problem", got, err)
	}
}
