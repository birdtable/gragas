package rulebook_test

import (
	"context"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/rulebook"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// fixture builds the tree for one case: testdata/<name>, plus the shared
// testdata/parts.yaml, products.yaml and sources.yaml where the case has
// none of its own. Inputs read: testdata/.
func fixture(t *testing.T, name string) fstest.MapFS {
	t.Helper()
	tree := fstest.MapFS{}
	copyTree(t, tree, os.DirFS(path.Join("testdata", name)), "")
	for _, f := range []string{"parts.yaml", "products.yaml", "sources.yaml"} {
		if _, ok := tree["rulebook/"+f]; ok {
			continue
		}
		data, err := os.ReadFile(path.Join("testdata", f))
		if err != nil {
			t.Fatalf("read shared fixture: %v", err)
		}
		tree["rulebook/"+f] = &fstest.MapFile{Data: data}
	}
	return tree
}

func copyTree(t *testing.T, dst fstest.MapFS, src fs.FS, prefix string) {
	t.Helper()
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		dst[prefix+p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
}

func findings(b *rulebook.Book) []string {
	out := make([]string, len(b.Findings))
	for i, f := range b.Findings {
		out[i] = f.String()
	}
	return out
}

func TestLoad(t *testing.T) {
	t.Parallel()
	const layers = "rulebook/base/arch/rules/layers.yaml"
	tests := []struct {
		name string
		want []string
	}{
		{"valid", nil},
		{"duplicate-id", []string{
			layers + ":19: id arch.layers.1 used twice",
		}},
		{"retired-no-replacement", []string{
			layers + ":6: arch.layers.1: retired needs replaced_by or a note saying why",
		}},
		{"broken-replaced-by", []string{
			layers + ":6: arch.layers.1: replaced_by arch.layers.9 is not another rule",
		}},
		{"binding-no-check", []string{
			layers + ":6: arch.layers.1: strength Answered needs at least one check",
		}},
		{"unknown-folder", []string{
			"rulebook/base/archs: folder is not a part of layer base in parts.yaml",
		}},
		{"part-mismatch", []string{
			"rulebook/base/arch/rules/layer.yaml:1: part is 'go', folder says 'arch'",
			"rulebook/base/arch/rules/layer.yaml:2: concern is 'layers', file says 'layer'",
			"rulebook/base/arch/rules/layer.yaml:6: go.layers.1: a rule's id starts with its part (arch); a moved rule is retired with replaced_by",
		}},
		{"unsourced", []string{
			layers + ":6: arch.layers.1: an active rule cites at least one source; with none it is proposed",
			layers + ":16: arch.layers.2: source no-such-source is not in sources.yaml",
			layers + ":16: arch.layers.2: kind web-app is not in products.yaml",
			layers + ":16: arch.layers.2: facet nothing-names-this is named by no part",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := rulebook.Load(context.Background(), fixture(t, tc.name), valid{}, rulebook.Options{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := findings(b); !slices.Equal(got, tc.want) {
				t.Errorf("findings:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

func TestLoadValidBook(t *testing.T) {
	t.Parallel()
	b, err := rulebook.Load(context.Background(), fixture(t, "valid"), valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(b.Rules), 6; got != want {
		t.Errorf("rules = %d, want %d", got, want)
	}
	if got, want := len(b.Files), 2; got != want {
		t.Errorf("files = %d, want %d", got, want)
	}
	if !b.HasGuide(rulebook.Part{Short: "arch"}) || b.HasGuide(rulebook.Part{Short: "go"}) {
		t.Errorf("guides = %v, want arch only", b.Guides)
	}
}

func TestBinds(t *testing.T) {
	t.Parallel()
	b, err := rulebook.Load(context.Background(), fixture(t, "valid"), valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		id   string
		want bool
	}{
		{"arch.layers.1", true},  // Hard, active
		{"arch.layers.2", false}, // a measure
		{"arch.layers.3", false}, // Hard but retired
		{"arch.layers.4", false}, // an instruction
		{"arch.layers.5", false}, // Lens, not promoted
		{"go.errors.1", true},    // Record, promoted
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			r, ok := b.Rules[tc.id]
			if !ok {
				t.Fatalf("no rule %s", tc.id)
			}
			if got := rulebook.Binds(r.Data); got != tc.want {
				t.Errorf("Binds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNames(t *testing.T) {
	t.Parallel()
	names, err := rulebook.ParseNames(strings.NewReader("# made-up names\n\nacme-corp\n\\bwidgetco\\b\n"))
	if err != nil {
		t.Fatalf("ParseNames: %v", err)
	}
	tree := fixture(t, "valid")
	f := tree["rulebook/base/arch/rules/layers.yaml"]
	f.Data = []byte(strings.Replace(string(f.Data), "An upward import makes", "At WidgetCo an upward import makes", 1))
	b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{Names: names})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"rulebook/base/arch/rules/layers.yaml:7: arch.layers.1 names 'WidgetCo': public text must stay generic"}
	if got := findings(b); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
}

func TestParseNamesRefusesBadPattern(t *testing.T) {
	t.Parallel()
	_, err := rulebook.ParseNames(strings.NewReader("ok\n(unclosed\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("ParseNames error = %v, want one naming line 2", err)
	}
}

func TestLoadNoParts(t *testing.T) {
	t.Parallel()
	tree := fixture(t, "valid")
	delete(tree, "rulebook/parts.yaml")
	if _, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{}); err == nil {
		t.Fatal("Load with no parts.yaml: want an error")
	}
}

func TestLoadStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rulebook.Load(ctx, fixture(t, "valid"), valid{}, rulebook.Options{}); err == nil {
		t.Fatal("Load with a cancelled context: want an error")
	}
}

// valid is the schemas as a test of the cross-file checks wants them: every
// document passes. The schemas themselves are the jsonschema adapter's tests.
type valid struct{}

func (valid) Validate(string, any) []legacy.Problem { return nil }

// failing is the schemas when one document fails: every rules file has the
// one problem.
type failing struct{ schema string }

func (f failing) Validate(schema string, _ any) []legacy.Problem {
	if schema != f.schema {
		return nil
	}
	return []legacy.Problem{{Line: 6, Msg: "schema at rules/0: made up"}}
}

func TestASchemaProblemIsAFindingAtItsFileAndLine(t *testing.T) {
	t.Parallel()
	b, err := rulebook.Load(context.Background(), fixture(t, "valid"), failing{"rules.schema.json"}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{
		"rulebook/base/arch/rules/layers.yaml:6: schema at rules/0: made up",
		"rulebook/lang/go/rules/errors.yaml:6: schema at rules/0: made up",
	}
	if got := findings(b); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
	if len(b.Rules) != 0 {
		t.Errorf("rules = %d, want none: a file that fails its schema is not read further", len(b.Rules))
	}
}

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestStandingIsDerived: standing follows from status, strength and kind,
// and from whether a check's tool and key resolve in the tool's preset.
// Risk: a rule reported as enforced that nothing holds.
func TestStandingIsDerived(t *testing.T) {
	t.Parallel()
	b, err := rulebook.Load(context.Background(), fixture(t, "valid"), valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for id, want := range map[string]string{
		"arch.layers.1": rulebook.Enforced, // Hard, its key in the preset's enable list
		"arch.layers.2": rulebook.Advisory, // a measure
		"arch.layers.3": rulebook.Retired,
		"arch.layers.4": rulebook.Advisory, // an instruction
		"arch.layers.5": rulebook.Advisory, // Lens, not promoted
		"go.errors.1":   rulebook.Enforced, // Record, promoted, its key a setting's dotted path
	} {
		if got := b.Standing(id); got != want {
			t.Errorf("%s: standing %q, want %q", id, got, want)
		}
	}
	tree := fixture(t, "valid")
	tree["presets/layers.yml"] = &fstest.MapFile{Data: []byte("linters:\n  enable: [other]\n")}
	b, err = rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := b.Standing("arch.layers.1"); got != rulebook.Planned {
		t.Errorf("with its key gone from the preset: standing %q, want %q", got, rulebook.Planned)
	}
}

// TestABaselineMayOnlyFall: a count above its baseline fails, and so does
// one below it, so the commit that closes a gap lowers the number. Risk: a
// gap that grows unseen, or a baseline left high after a fix.
func TestABaselineMayOnlyFall(t *testing.T) {
	t.Parallel()
	const file = "rulebook/baseline.json"
	tests := []struct {
		name, from, to string
		want           []string
	}{
		{"held", "", "", nil},
		{"rose", `"checks-unresolved": 3`, `"checks-unresolved": 2`,
			[]string{file + ":6: checks-unresolved rose to 3, above its baseline of 2: a baseline may only fall"}},
		{"fell", `"checks-unresolved": 3`, `"checks-unresolved": 4`,
			[]string{file + ":6: checks-unresolved fell to 3: lower its baseline from 4 to 3"}},
		{"missing", `,` + "\n" + `  "advisory-with-checks": 0`, "",
			[]string{file + ": advisory-with-checks has no baseline; it counts 0 now"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tree := fixture(t, "valid")
			f := tree[file]
			f.Data = []byte(strings.Replace(string(f.Data), tc.from, tc.to, 1))
			b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if err := b.CheckBaseline(); err != nil {
				t.Fatalf("CheckBaseline: %v", err)
			}
			if got := findings(b); !slices.Equal(got, tc.want) {
				t.Errorf("findings:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

// TestALockedIDStays: an id in ids.lock that is no longer a rule fails, and
// the lock keeps every id ever issued. Risk: an id removed and
// later reused for a different rule.
func TestALockedIDStays(t *testing.T) {
	t.Parallel()
	tree := fixture(t, "valid")
	tree["rulebook/ids.lock"] = &fstest.MapFile{Data: []byte("# the lock\narch.layers.1\narch.layers.7\n")}
	b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"rulebook/ids.lock:3: arch.layers.7 was issued and is gone: an id is retired with replaced_by, never removed"}
	if got := findings(b); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
	if got := b.LockIDs(); !slices.Contains(got, "arch.layers.7") || !slices.Contains(got, "go.errors.1") {
		t.Errorf("LockIDs = %q, want the lock and the rulebook's ids together", got)
	}
}

// An empty pairs/<id>/1/ folder, which A56 allows until the old check goes at
// M6, leaves the load with the findings it had without the folder, and the
// check reads nothing in it: a file planted inside is no finding either.
func TestAnEmptyPairsFolderIsNoFinding(t *testing.T) {
	t.Parallel()
	load := func(tree fstest.MapFS) []string {
		b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return findings(b)
	}
	without := load(fixture(t, "valid"))
	withDir := fixture(t, "valid")
	withDir["rulebook/pairs/arch.layers.1/1"] = &fstest.MapFile{Mode: fs.ModeDir}
	if got := load(withDir); !slices.Equal(got, without) {
		t.Errorf("an empty pairs folder changed the findings: %v, want %v", got, without)
	}
	withFile := fixture(t, "valid")
	withFile["rulebook/pairs/arch.layers.1/1/good.go"] = &fstest.MapFile{Data: []byte("not read")}
	if got := load(withFile); !slices.Equal(got, without) {
		t.Errorf("a file in a pair changed the findings: %v, want %v", got, without)
	}
}
