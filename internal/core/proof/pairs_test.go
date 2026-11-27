package proof_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// stubTree is a Tree that lists and reads what a test says, so the reader is
// held to foreign names a real folder could not spell.
type stubTree struct {
	port.Tree
	entries []model.Entry
	files   map[string]string
	reads   []string
}

func (s *stubTree) List(_ model.Root, dir string, _ model.Scope) ([]model.Entry, error) {
	if s.entries == nil && s.files == nil {
		return nil, fs.ErrNotExist
	}
	return s.entries, nil
}

func (s *stubTree) Read(_ model.Root, p string) ([]byte, error) {
	s.reads = append(s.reads, p)
	data, ok := s.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(data), nil
}

func treeOf(files map[string]string) *stubTree {
	s := &stubTree{files: files, entries: []model.Entry{}}
	for p, data := range files {
		s.entries = append(s.entries, model.Entry{Path: p, Size: int64(len(data))})
	}
	return s
}

func rule(id string, pair int, bindings ...model.Binding) model.Rule {
	r := model.Rule{ID: model.RuleID(id), Bindings: bindings}
	if pair != 0 {
		r.Pairs = []model.Pair{{Number: pair, Target: model.TargetSource}}
	}
	return r
}

func bind(key string, pair int, at string) model.Binding {
	return model.Binding{Key: key, Pair: pair, PairPath: at}
}

func msgs(l proof.Loaded) []string {
	var out []string
	for _, f := range l.Findings {
		out = append(out, f.String())
	}
	return out
}

func TestAPairIsLoadedWithItsTargetAndADigestOfBothFiles(t *testing.T) {
	t.Parallel()
	files := map[string]string{"pairs/a.b.1/1/good.go": "package a\n", "pairs/a.b.1/1/bad.go": "package b\n"}
	got, err := proof.LoadPairs(treeOf(files), "r", []model.Rule{rule("a.b.1", 1, bind("k", 1, "src/x.go"))})
	if err != nil || len(got.Findings) != 0 || len(got.Pairs) != 1 {
		t.Fatalf("LoadPairs = %+v, %v; want one pair and no finding", got, err)
	}
	p := got.Pairs[0]
	if p.Rule != "a.b.1" || p.Number != 1 || p.Target != model.TargetSource || string(p.Good.Data) != "package a\n" || p.Bad.Name != "bad.go" {
		t.Errorf("pair = %+v", p)
	}
	if !strings.HasPrefix(string(p.Digest), "sha256:") || len(p.Digest) != len("sha256:")+64 {
		t.Errorf("digest = %q", p.Digest)
	}
	files["pairs/a.b.1/1/bad.go"] = "package c\n"
	again, _ := proof.LoadPairs(treeOf(files), "r", []model.Rule{rule("a.b.1", 1, bind("k", 1, "src/x.go"))})
	if again.Pairs[0].Digest == p.Digest {
		t.Error("changing the bad file left the pair's digest as it was")
	}
	swapped := map[string]string{"pairs/a.b.1/1/good.go": "package b\n", "pairs/a.b.1/1/bad.go": "package a\n"}
	sw, _ := proof.LoadPairs(treeOf(swapped), "r", []model.Rule{rule("a.b.1", 1)})
	if sw.Pairs[0].Digest == p.Digest {
		t.Error("swapping good and bad left the pair's digest as it was")
	}
}

func TestAnAbsentPairsFolderIsNoPairs(t *testing.T) {
	t.Parallel()
	got, err := proof.LoadPairs(&stubTree{}, "r", []model.Rule{rule("a.b.1", 0)})
	if err != nil || len(got.Pairs) != 0 || len(got.Findings) != 0 {
		t.Errorf("LoadPairs of no folder = %+v, %v", got, err)
	}
}

// Each plant is one thing wrong in a pairs folder, with the finding it must
// draw, and none of them may stop the whole pair beside it from loading.
func TestEachFaultOfAPairsFolderIsAFindingAndTheRestLoads(t *testing.T) {
	t.Parallel()
	whole := map[string]string{"pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b"}
	plants := []struct {
		name  string
		extra map[string]string
		rules []model.Rule
		want  string
	}{
		{"a number with a leading zero", map[string]string{"pairs/a.b.1/01/good.go": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"a number of zero", map[string]string{"pairs/a.b.1/0/good.go": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"a number that is a climb", map[string]string{"pairs/a.b.1/../good.go": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"an absolute spelling", map[string]string{"/work/scratch/x/1/good.go": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"a file with no extension", map[string]string{"pairs/a.b.1/2/good": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"a file of another name", map[string]string{"pairs/a.b.1/2/other.go": "g"}, nil, "is not pairs/<rule id>/<n>/good.* or bad.*"},
		{"a pair of one side", map[string]string{"pairs/a.b.1/2/good.go": "g"}, nil, "a.b.1 pair 2: a pair holds one good.* and one bad.*"},
		{"a pair the rule does not list", map[string]string{"pairs/a.b.1/3/good.go": "g", "pairs/a.b.1/3/bad.go": "b"}, nil, "a.b.1 pair 3: the rule does not list this pair"},
		{"a pair with two good files", map[string]string{"pairs/a.b.1/1/good.py": "g"}, nil, "a.b.1 pair 1: a pair holds one good.* and one bad.*, and this holds more"},
		{"a pair for a rule the book lacks", map[string]string{"pairs/no.such.1/1/good.go": "g"}, nil, "no.such.1: a pairs folder for a rule the book does not hold"},
	}
	for _, p := range plants {
		files := map[string]string{}
		maps.Copy(files, whole)
		maps.Copy(files, p.extra)
		rules := p.rules
		if rules == nil {
			rules = []model.Rule{{ID: "a.b.1", Pairs: []model.Pair{{Number: 1, Target: model.TargetSource}, {Number: 2, Target: model.TargetSource}}}}
		}
		got, err := proof.LoadPairs(treeOf(files), "r", rules)
		if err != nil {
			t.Errorf("%s: %v", p.name, err)
			continue
		}
		if !strings.Contains(strings.Join(msgs(got), "\n"), p.want) {
			t.Errorf("%s: findings %q lack %q", p.name, msgs(got), p.want)
		}
		if p.name != "a pair with two good files" && (len(got.Pairs) != 1 || got.Pairs[0].Number != 1) {
			t.Errorf("%s: the whole pair beside it did not load: %+v", p.name, got.Pairs)
		}
		if p.name == "a pair with two good files" && len(got.Pairs) != 0 {
			t.Errorf("%s: a pair of two good files loaded", p.name)
		}
	}
}

func TestAnOverLargeFileIsAFindingAndIsNotRead(t *testing.T) {
	t.Parallel()
	s := treeOf(map[string]string{"pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b"})
	for i := range s.entries {
		if s.entries[i].Path == "pairs/a.b.1/1/bad.go" {
			s.entries[i].Size = proof.MostExample + 1
		}
	}
	got, err := proof.LoadPairs(s, "r", []model.Rule{rule("a.b.1", 1)})
	if err != nil || len(got.Pairs) != 0 || !strings.Contains(strings.Join(msgs(got), "\n"), "is over") {
		t.Fatalf("LoadPairs = %+v, %v", got, err)
	}
	if len(s.reads) != 0 {
		t.Errorf("an over-large pair was read: %v", s.reads)
	}
}

func TestABindingWithNoPairIsAFinding(t *testing.T) {
	t.Parallel()
	files := map[string]string{"pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b"}
	rules := []model.Rule{rule("a.b.1", 1, bind("ok", 1, "x.go"), bind("none", 0, ""), bind("gone", 2, "y.go"))}
	got, _ := proof.LoadPairs(treeOf(files), "r", rules)
	want := []string{"a.b.1: a binding of none names no pair", "a.b.1 pair 2: a binding of gone names this pair, which is not there whole"}
	if strings.Join(msgs(got), "|") != strings.Join(want, "|") {
		t.Errorf("findings = %q, want %q", msgs(got), want)
	}
}

func TestAPairIsReadOnlyBelowPairsAndOnlyByAListedName(t *testing.T) {
	t.Parallel()
	s := treeOf(map[string]string{
		"pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b",
		"pairs/a.b.1/1/notes.txt": "n", "rulebook.yaml": "secret", "pairs/a.b.1/1/sub/good.go": "x",
	})
	_, _ = proof.LoadPairs(s, "r", []model.Rule{rule("a.b.1", 1)})
	for _, p := range s.reads {
		if p != "pairs/a.b.1/1/good.go" && p != "pairs/a.b.1/1/bad.go" {
			t.Errorf("read %q, which is no pair file", p)
		}
	}
}

func TestATreeFailureIsReturnedAndNotAFinding(t *testing.T) {
	t.Parallel()
	s := treeOf(map[string]string{"pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b"})
	delete(s.files, "pairs/a.b.1/1/bad.go") // listed but unreadable
	if _, err := proof.LoadPairs(s, "r", []model.Rule{rule("a.b.1", 1)}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one matching fs.ErrNotExist", err)
	}
}

// show is a placement as text, for the goldens.
func show(pl proof.Placement) string {
	var b strings.Builder
	fmt.Fprintf(&b, "not run: %q\n", pl.NotRun)
	for _, side := range []struct {
		name  string
		files []model.File
	}{{"good", pl.Good}, {"bad", pl.Bad}} {
		fmt.Fprintf(&b, "%s:\n", side.name)
		for _, f := range side.files {
			fmt.Fprintf(&b, "  %s = %q\n", f.Path, f.Data)
		}
	}
	return b.String()
}

func pairOne() proof.Pair {
	return proof.Pair{Rule: "a.b.1", Number: 1, Target: model.TargetSource,
		Good: proof.Example{Name: "good.go", Data: []byte("package ok\n")}, Bad: proof.Example{Name: "bad.go", Data: []byte("package bad\n")}}
}

func scoped() []model.File {
	return []model.File{{Path: "src/legacy/.lintrc", Data: []byte("strict\n")}, {Path: ".lintrc", Data: []byte("base\n")}}
}

func TestAPairWithAPathScopedConfigPlacesAtTheDeclaredPath(t *testing.T) {
	t.Parallel()
	pl, err := proof.Place(pairOne(), bind("k", 1, "src/legacy/x.go"), scoped())
	if err != nil {
		t.Fatal(err)
	}
	configtest.Golden(t, "place-declared.golden", []byte(show(pl)))
}

func TestTheSamePairAtAWrongPathIsShownToDiffer(t *testing.T) {
	t.Parallel()
	right, _ := proof.Place(pairOne(), bind("k", 1, "src/legacy/x.go"), scoped())
	wrong, err := proof.Place(pairOne(), bind("k", 1, "docs/x.go"), scoped())
	if err != nil {
		t.Fatal(err)
	}
	configtest.Golden(t, "place-wrong-path.golden", []byte(show(wrong)))
	if show(right) == show(wrong) {
		t.Error("the same pair at two paths placed the same")
	}
}

func TestABindingWithNoDeclaredPathIsNotRunForNoPair(t *testing.T) {
	t.Parallel()
	pl, err := proof.Place(pairOne(), bind("k", 1, ""), scoped())
	if err != nil || pl.NotRun != model.ReasonNoPair || len(pl.Good)+len(pl.Bad) != 0 {
		t.Errorf("Place = %+v, %v; want not run, no pair, and nothing placed", pl, err)
	}
	configtest.Golden(t, "place-no-path.golden", []byte(show(pl)))
}

func TestAPathThatLeavesTheScratchFolderIsRefused(t *testing.T) {
	t.Parallel()
	bad := []string{"../x.go", "src/../../x.go", "/work/scratch/x.go", "src/./x.go", "src//x.go", "src/x.go/", "src/x\x00.go", "..", ".", strings.Repeat("a/", 3000) + "x"}
	bad = append(bad, foreignPaths(t)...)
	for _, p := range bad {
		if _, err := proof.Place(pairOne(), bind("k", 1, p), nil); !errors.Is(err, proof.ErrPath) {
			t.Errorf("a declared path %q: err = %v, want ErrPath", p, err)
		}
		if _, err := proof.Place(pairOne(), bind("k", 1, "x.go"), []model.File{{Path: p}}); !errors.Is(err, proof.ErrPath) {
			t.Errorf("a config path %q: err = %v, want ErrPath", p, err)
		}
	}
	for _, p := range []string{"x.go", "src/legacy/x.go", ".config/x", "a b/c d.go", "..x/y", "a..b"} {
		if err := proof.Confine(p); err != nil {
			t.Errorf("Confine(%q) = %v, want nil", p, err)
		}
	}
}

func TestAConfigFileOnTheExampleOrTwiceIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := proof.Place(pairOne(), bind("k", 1, "x.go"), []model.File{{Path: "x.go"}}); err == nil {
		t.Error("a config file where the example stands was accepted")
	}
	if _, err := proof.Place(pairOne(), bind("k", 1, "x.go"), []model.File{{Path: "c"}, {Path: "c"}}); err == nil {
		t.Error("a config file twice was accepted")
	}
	if _, err := proof.Place(pairOne(), bind("k", 2, "x.go"), nil); err == nil {
		t.Error("a binding of another pair was placed")
	}
}

// FuzzLoadPairs (tests.generated.1): file names from a repository are foreign
// input. Whatever a listing holds, LoadPairs does not panic, reads only a path
// below pairs/ spelled pairs/<rule id>/<n>/good.* or bad.*, and any pair it
// loads is of a rule in the book with a number from 1.
func FuzzLoadPairs(f *testing.F) {
	for _, s := range []string{"pairs/a.b.1/1/good.go", "pairs/a.b.1/1/bad.go", "pairs/a.b.1/01/good.go", "pairs/a.b.1/../x", "/work/scratch/passwd", "pairs/a.b.1/1/good.", "pairs//1/good.go", "pairs/a.b.1/1/good.go/x", "pairs/a.b.1/-1/good.go", "pairs/a.b.1/99999999999999999999/good.go", "", "\x00", "pairs/a.b.1/1/GOOD.go"} {
		f.Add(s, int64(1), false)
	}
	f.Add("pairs/a.b.1/1/good.go", int64(-1), true)
	f.Add("pairs/a.b.1/1/bad.go", int64(proof.MostExample+1), false)
	f.Fuzz(func(t *testing.T, name string, size int64, dir bool) {
		s := &stubTree{entries: []model.Entry{
			{Path: name, Size: size, Dir: dir},
			{Path: "pairs/a.b.1/1/good.go", Size: 1}, {Path: "pairs/a.b.1/1/bad.go", Size: 1},
		}, files: map[string]string{name: "x", "pairs/a.b.1/1/good.go": "g", "pairs/a.b.1/1/bad.go": "b"}}
		got, err := proof.LoadPairs(s, "r", []model.Rule{rule("a.b.1", 1, bind("k", 1, "x.go"))})
		if err != nil {
			return
		}
		for _, p := range s.reads {
			if !strings.HasPrefix(p, "pairs/a.b.1/") || strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
				t.Fatalf("read %q", p)
			}
		}
		for _, p := range got.Pairs {
			if p.Rule != "a.b.1" || p.Number < 1 || !strings.HasPrefix(p.Good.Name, "good.") || !strings.HasPrefix(p.Bad.Name, "bad.") {
				t.Fatalf("loaded %+v", p)
			}
		}
	})
}

// FuzzConfine: whatever a path says, Confine does not panic, and a path it
// accepts is one Place can put inside a folder: relative, with no climb.
func FuzzConfine(f *testing.F) {
	for _, s := range []string{"x.go", "../x", "/x", "a/../b", "", ".", "a//b", "a/./b", "\x00", "a/b/", "é/ü.go", strings.Repeat("a/", 3000)} {
		f.Add(s)
	}
	for _, s := range foreignPaths(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		if proof.Confine(p) != nil {
			return
		}
		if strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.Contains(p, "\x00") {
			t.Fatalf("Confine accepted %q", p)
		}
		for part := range strings.SplitSeq(p, "/") {
			if part == ".." || part == "." || part == "" {
				t.Fatalf("Confine accepted %q", p)
			}
		}
	})
}
