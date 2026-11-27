package format

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// leaf is one value of an old file below its keys: the key it is held at,
// list positions skipped, its text, and whether it is raw: a value of a check
// with no tool or key, which is parked whole with its old words.
type leaf struct {
	file string
	at   []string
	text string
	raw  bool
}

func (l leaf) key() string { return l.at[len(l.at)-1] }

// leavesOf is every scalar of v below at, a list item at the key of its list;
// below raw, each is raw.
func leavesOf(file string, at []string, v any, raw bool, out *[]leaf) {
	switch x := v.(type) {
	case *ordered.Map:
		for _, k := range x.Keys() {
			leavesOf(file, append(slices.Clone(at), k), x.Val(k), raw, out)
		}
	case []any:
		for _, e := range x {
			c, isMap := e.(*ordered.Map)
			whole := isMap && len(at) > 0 && at[len(at)-1] == "checks" && (c.Str("tool") == "" || c.Str("key") == "")
			leavesOf(file, at, e, raw || whole, out)
		}
	case nil:
	default:
		*out = append(*out, leaf{file: file, at: at, text: leafText(x), raw: raw})
	}
}

// leafText is a scalar as text, a string without the white space around it,
// since the conversion trims every text it carries.
func leafText(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ordered.Repr(v)
}

// written is how many times each text is a value in the files m writes, the
// parked file with them.
func written(t *testing.T, m Migration) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tg := range m.Targets {
		if !strings.HasSuffix(tg.Path, ".yaml") {
			continue
		}
		v, err := ordered.ParseYAML(tg.Data)
		if err != nil {
			t.Fatalf("%s: %v", tg.Path, err)
		}
		var ls []leaf
		leavesOf(tg.Path, []string{tg.Path}, v, false, &ls)
		for _, l := range ls {
			out[l.text]++
		}
	}
	return out
}

// renamed is each old word the conversion writes as another, by the key it is
// held at: the named mappings. A word that maps to "" is written as the shape
// of the result, not as a value: a conflict is the relation its id is listed
// under.
func renamed() map[string]map[string]string {
	rung := map[string]string{}
	for k, v := range rungs() {
		rung[k] = strconv.Itoa(v)
	}
	return map[string]map[string]string{
		"strength": strengths(),
		"status":   {"later": "proposed"},
		"class":    rung,
		"tier":     runPoints(),
		"binding":  targets(),
		"kind":     sourceKinds(),
		"how":      {"conflicts": ""},
	}
}

// notCarriedLeaf is whether l is below an item the migration lists as not
// carried: a kind's instances, a top-level key of the parts file, or a source
// whose kind has none at format 3.
func notCarriedLeaf(l leaf, m Migration) bool {
	var item string
	switch {
	case l.file == "products.yaml" && len(l.at) > 2 && l.at[1] == "instances":
		return slices.ContainsFunc(m.NotCarried, func(n NotCarried) bool { return strings.HasSuffix(n.Item, " instances") })
	case l.file == "parts.yaml" && l.at[0] != "layers" && l.at[0] != "parts":
		item = "parts.yaml " + l.at[0]
	case l.file == "sources.yaml" && len(l.at) > 1 && l.at[0] == "sources":
		item = "source " + l.at[1]
	default:
		return false
	}
	return slices.ContainsFunc(m.NotCarried, func(n NotCarried) bool { return n.Item == item })
}

// unaccounted is each leaf of the old files of v that m writes nowhere: not as
// a value of a file it writes, the parked file with them, nor by a named
// mapping, nor below an item it lists as not carried. Each written value is
// spent once, so a word written fewer times than the old files held it is
// found.
func unaccounted(t *testing.T, v V2, m Migration) []string {
	t.Helper()
	have := written(t, m)
	var old []leaf
	for _, p := range sortedKeys(v) {
		if !strings.HasSuffix(p, ".yaml") {
			continue
		}
		doc, err := ordered.ParseYAML(v[p])
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		leavesOf(p, nil, doc, false, &old)
	}
	if len(old) == 0 {
		t.Fatal("the old files hold no value")
	}
	// A kind of a part's facets is written once however many parts name it.
	kinds := map[string]bool{}
	var plain, mapped []leaf
	for _, l := range old {
		if notCarriedLeaf(l, m) || shape(l) {
			continue
		}
		if l.file == "parts.yaml" && l.key() == "facets" {
			if kinds[l.text] {
				continue
			}
			kinds[l.text] = true
		}
		if _, ok := renamed()[l.key()][l.text]; ok && !l.raw {
			mapped = append(mapped, l)
		} else {
			plain = append(plain, l)
		}
	}
	// The words written as themselves first, then the mapped ones, each its
	// mapped word if one is left, else itself, as a check parked whole keeps
	// its old words: so no word is spent by one written as another.
	var lost []string
	for i, l := range append(plain, mapped...) {
		text := l.text
		if i >= len(plain) {
			to := renamed()[l.key()][l.text]
			if to == "" {
				continue
			}
			if left(have, to) {
				text = to
			}
		}
		if !left(have, text) {
			lost = append(lost, l.file+": "+strings.Join(l.at, "/")+": "+l.text)
			continue
		}
		have[text]--
	}
	return lost
}

func left(have map[string]int, text string) bool { return text != "" && have[text] > 0 }

// shape is whether l is written as the shape of the structure, not as a
// value: a rule file's order is the place of its concern among its part's,
// and a part's layer is the layer the part is listed under.
func shape(l leaf) bool {
	switch {
	case strings.Contains(l.file, "/rules/"):
		return slices.Equal(l.at, []string{"order"})
	case l.file == "parts.yaml":
		return slices.Equal(l.at, []string{"parts", "layer"})
	}
	return false
}

// withRelations is the fixture with arch.layers.1 relating to arch.layers.2
// in each way the owner's rulebook does: see, refines, and a conflict with a
// winner.
func withRelations() V2 {
	v := v2Fixture()
	v["base/arch/rules/layers.yaml"] = []byte(strings.Replace(v2Rules, "    status: active\n", `    status: active
    relates:
      - id: arch.layers.2
        how: see
      - id: arch.gone.1
        how: refines
      - id: arch.other.1
        how: conflicts
        wins: this
`, 1))
	return v
}

// ownersRulebook is the old files of the owner's rulebook, as migrate reads
// them from its folder: the frozen format 2 book of the journeys' testdata,
// rulebook/ as it stood before the switch to format 3 converted the live one.
func ownersRulebook(t *testing.T) V2 {
	t.Helper()
	root := filepath.Join("..", "..", "journeys", "testdata", "rulebook-v2")
	var all []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		all = append(all, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	v := V2{}
	for _, p := range V2Files(all) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		v[p] = b
	}
	if !IsV2(sortedKeys(v)) {
		t.Fatalf("%s is not a rulebook of the old layout", root)
	}
	return v
}

// TestMigrateWritesEveryLeafOfTheOldFilesSomewhere: every value of the old
// files is a value of a file the migration writes, the parked file with them,
// or is written by a named mapping, or is below an item the plan lists as not
// carried, in the fixture with relations of each kind and in the owner's
// rulebook. Planted: the fixture's relations with how and wins dropped, as
// they once were, are found. Risk: a value
// the plan never names, gone when the old files are.
func TestMigrateWritesEveryLeafOfTheOldFilesSomewhere(t *testing.T) {
	t.Parallel()
	for name, v := range map[string]V2{"the fixture": withRelations(), "the owner's rulebook": ownersRulebook(t)} {
		m, err := MigrateV2(v, migrateForm())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if lost := unaccounted(t, v, m); len(lost) > 0 {
			t.Errorf("%s: %d values written nowhere, the first %q", name, len(lost), lost[:min(len(lost), 5)])
		}
	}
	v := withRelations()
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	for i, tg := range m.Targets {
		if path.Base(tg.Path) == ParkedFile {
			doc, err := ordered.ParseYAML(tg.Data)
			if err != nil {
				t.Fatal(err)
			}
			parked := doc.(*ordered.Map)
			was, cut := parked.Sub("arch.layers.1"), ordered.NewMap(0)
			for _, k := range was.Keys() {
				if k != "relates" {
					cut.Set(k, was.Val(k), 0)
				}
			}
			parked.Set("arch.layers.1", cut, 0)
			m.Targets[i].Data = []byte(ordered.YAML(doc))
		}
	}
	lost := unaccounted(t, v, m)
	want := []string{"base/arch/rules/layers.yaml: rules/relates/how: see", "base/arch/rules/layers.yaml: rules/relates/how: refines", "base/arch/rules/layers.yaml: rules/relates/wins: this"}
	if !slices.Equal(lost, want) {
		t.Errorf("planted: with the parked relations dropped, the values found are %q, want %q", lost, want)
	}
}
