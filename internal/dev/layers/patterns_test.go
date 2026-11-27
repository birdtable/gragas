package layers_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// withPatterns is small with an edge layer that holds a folder by pattern, a
// root that may import the pattern, and a library the pattern's packages may
// import beside its owner.
func withPatterns() string {
	s := strings.Replace(small, "layers:\n", "layers:\n"+
		"  - {layer: root, kind: edge, packages: [internal/root], holds: root, template: root, may: [internal/cmds/..., internal/port], why: \"root: no\"}\n"+
		"  - {layer: cmds, kind: edge, patterns: [internal/cmds/...], holds: commands, template: cmds, may: [internal/port, internal/port/testing], why: \"cmds: no\"}\n", 1)
	return strings.Replace(s, "- {path: example.org/lib, owner: internal/host/x}",
		"- {path: example.org/lib, owner: internal/host/x, within: [internal/cmds/...]}", 1)
}

// TestAPatternHoldsEveryPackageBelowItsFolder: a package below the folder is in
// the row without a line of its own, the folder itself is not, and a row may
// import the pattern as it imports a package (the layer table's patterns).
func TestAPatternHoldsEveryPackageBelowItsFolder(t *testing.T) {
	t.Parallel()
	tb := parse(t, withPatterns())
	for pkg, layer := range map[string]string{
		"internal/cmds/version":     "cmds",
		"internal/cmds/a/deep":      "cmds",
		"internal/cmds":             "",
		"internal/cmdsother/x":      "",
		"internal/root":             "root",
		"internal/core/a":           "core",
		"internal/cmds/version/sub": "cmds",
	} {
		got := ""
		if r := tb.RowOf(pkg); r != nil {
			got = r.Layer
		}
		if got != layer {
			t.Errorf("RowOf(%s) = %q, want %q", pkg, got, layer)
		}
	}
	for _, tc := range []struct {
		from, to string
		may      bool
	}{
		{"internal/root", "internal/cmds/version", true},
		{"internal/root", "internal/cmds/a/deep", true},
		{"internal/root", "internal/cmdsother/x", false},
		{"internal/cmds/version", "internal/cmds/version", true},
		{"internal/cmds/version", "internal/cmds/other", false},
		{"internal/cmds/version", "internal/root", false},
		{"internal/cmds/version", "internal/port", true},
	} {
		if got := tb.May(tc.from, tc.to, false); got != tc.may {
			t.Errorf("May(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.may)
		}
	}
}

// TestAPatternIsWrittenAsOneRuleForItsFolder: the lint block has one rule for
// the folder, which allows every package below it, and the owner rule of a
// library spares the folder.
func TestAPatternIsWrittenAsOneRuleForItsFolder(t *testing.T) {
	t.Parallel()
	block, err := parse(t, withPatterns()).Lint([]byte(layers.BeginMark + "\n" + layers.EndMark + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"layer-cmds:",
		`files: ["**/internal/cmds/**"]`,
		"- example.com/m/internal/cmds/\n",
		`"!**/internal/cmds/**/*.go"`,
	} {
		if !strings.Contains(string(block), want) {
			t.Errorf("the block lacks %s:\n%s", want, block)
		}
	}
	if n := strings.Count(string(block), "layer-cmds:"); n != 1 {
		t.Errorf("the folder has %d rules, want one", n)
	}
	if !strings.Contains(parse(t, withPatterns()).Doc(), "every package below `internal/cmds/...`") {
		t.Errorf("the document does not say the pattern")
	}
}

// TestAPatternThatContradictsTheTableIsRefused: a pattern of the wrong shape,
// one that overlaps another, one that holds a package a row lists, a pattern
// no layer holds named in may and in within.
func TestAPatternThatContradictsTheTableIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, from, to, says string }{
		{"no wildcard", "patterns: [internal/cmds/...]", "patterns: [internal/cmds]", "is not of the shape folder/..."},
		{"a wildcard in the middle", "patterns: [internal/cmds/...]", "patterns: [internal/.../cmds/...]", "is not of the shape folder/..."},
		{"a bare wildcard", "patterns: [internal/cmds/...]", "patterns: [/...]", "is not of the shape folder/..."},
		{"the same pattern in two layers", "packages: [internal/root], holds: root", "packages: [internal/root], patterns: [internal/cmds/...], holds: root", "is in layer"},
		{"two patterns that overlap", "patterns: [internal/cmds/...]", "patterns: [internal/cmds/..., internal/cmds/deep/...]", "hold the same packages"},
		{"an import of a pattern no layer has", "may: [internal/cmds/..., internal/port]", "may: [internal/nowhere/..., internal/port]", "internal/nowhere/..., which is in no layer"},
		{"a library within no pattern", "within: [internal/cmds/...]", "within: [internal/nowhere/...]", "internal/nowhere/..., which is no pattern of a layer"},
		{"a library within a package", "within: [internal/cmds/...]", "within: [internal/root]", "internal/root, which is no pattern of a layer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := withPatterns()
			bad := strings.Replace(s, tc.from, tc.to, 1)
			if bad == s {
				t.Fatalf("the plant %q is not in the table", tc.from)
			}
			_, err := layers.Parse([]byte(bad))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Parse = %v, want ErrTable saying %q", err, tc.says)
			}
		})
	}
}

// TestAPackageBelowAPatternIsAKnownNameInAMayAndAnOwner: an entry row, a layer's
// may list and a library's owner may name one package below a pattern, whether
// or not the package exists, without a line for it in the table; a package
// below no pattern is still in no layer.
func TestAPackageBelowAPatternIsAKnownNameInAMayAndAnOwner(t *testing.T) {
	t.Parallel()
	s := withPatterns()
	named := strings.Replace(s, "may: [internal/cmds/..., internal/port]", "may: [internal/cmds/version, internal/port]", 1)
	named = strings.Replace(named, "owner: internal/host/x, within", "owner: internal/cmds/lib, within", 1)
	if named == s {
		t.Fatal("the plant is not in the table")
	}
	if _, err := layers.Parse([]byte(named)); err != nil {
		t.Errorf("a package below a pattern was refused: %v", err)
	}
	for _, tc := range []struct{ name, from, to, says string }{
		{"a may beside the folder", "may: [internal/cmds/..., internal/port]", "may: [internal/cmdsother/x, internal/port]", "internal/cmdsother/x, which is in no layer"},
		{"a may on the folder itself", "may: [internal/cmds/..., internal/port]", "may: [internal/cmds, internal/port]", "internal/cmds, which is in no layer"},
		{"an owner beside the folder", "owner: internal/host/x", "owner: internal/cmdsother/x", "internal/cmdsother/x, which is in no layer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bad := strings.Replace(s, tc.from, tc.to, 1)
			if bad == s {
				t.Fatalf("the plant %q is not in the table", tc.from)
			}
			_, err := layers.Parse([]byte(bad))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Parse = %v, want ErrTable saying %q", err, tc.says)
			}
		})
	}
}

// TestARowThatListsAPackageBelowAPatternCarvesItOut: a package a row lists inside
// the folder of another row's pattern stays in its own row, with its own may; the
// rest of the folder is still the pattern's.
func TestARowThatListsAPackageBelowAPatternCarvesItOut(t *testing.T) {
	t.Parallel()
	s := strings.Replace(withPatterns(), "packages: [internal/root]", "packages: [internal/root, internal/cmds/version]", 1)
	s = strings.Replace(s, "patterns: [internal/cmds/...]", "patterns: [internal/cmds/...], carves: [internal/cmds/version]", 1)
	tb := parse(t, s)
	for pkg, layer := range map[string]string{
		"internal/cmds/version":     "root",
		"internal/cmds/other":       "cmds",
		"internal/cmds/other/deep":  "cmds",
		"internal/cmds/version/sub": "cmds",
	} {
		got := ""
		if r := tb.RowOf(pkg); r != nil {
			got = r.Layer
		}
		if got != layer {
			t.Errorf("RowOf(%s) = %q, want %q", pkg, got, layer)
		}
	}
	if !tb.May("internal/cmds/version", "internal/cmds/other", false) {
		t.Error("the carved package lost its own row's may")
	}
	if tb.May("internal/cmds/other", "internal/root", false) {
		t.Error("the pattern's package took the carved package's may")
	}
}

// lintOf is the lint file the table writes over a block of no other text.
func lintOf(t *testing.T, tb *layers.Table) string {
	t.Helper()
	got, err := tb.Lint([]byte("        " + layers.BeginMark + "\n        " + layers.EndMark + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// TestAPatternIsOneRuleForEachPackageItHoldsWhenTheTreeIsKnown: a package of a
// pattern may import its own package and its row's may, and no other package of
// the pattern; a package a row lists is no rule of the pattern; and a table with
// no tree writes the pattern as one rule for its folder.
func TestAPatternIsOneRuleForEachPackageItHoldsWhenTheTreeIsKnown(t *testing.T) {
	t.Parallel()
	tree := []string{"internal/cmds/version", "internal/cmds/other", "internal/cmds/other/deep", "internal/root", "internal/port"}
	s := strings.Replace(withPatterns(), "packages: [internal/root]", "packages: [internal/root, internal/cmds/version]", 1)
	s = strings.Replace(s, "patterns: [internal/cmds/...]", "patterns: [internal/cmds/...], carves: [internal/cmds/version]", 1)
	with := lintOf(t, parse(t, s).WithTree(tree))
	for _, want := range []string{"layer-cmds-other:", "layer-cmds-other-deep:", `files: ["**/internal/cmds/other/**", "!**/internal/cmds/other/deep/**"]`, "example.com/m/internal/cmds/other$"} {
		if !strings.Contains(with, want) {
			t.Errorf("the lint file lacks %q:\n%s", want, with)
		}
	}
	other := with[strings.Index(with, "layer-cmds-other:"):]
	other = other[:strings.Index(other, "layer-cmds-other-deep:")]
	for _, bad := range []string{"internal/cmds/version$", "internal/cmds/other/deep$", "internal/cmds/\n", "internal/cmds$"} {
		if strings.Contains(other, bad) {
			t.Errorf("a package of the pattern may import %q:\n%s", bad, other)
		}
	}
	if strings.Count(with, "layer-cmds-version:") != 1 {
		t.Errorf("the listed package has %d rules, want one:\n%s", strings.Count(with, "layer-cmds-version:"), with)
	}
	if without := lintOf(t, parse(t, s)); !strings.Contains(without, "layer-cmds:\n") || strings.Contains(without, "layer-cmds-other:") {
		t.Errorf("a table with no tree is not one rule for the folder:\n%s", without)
	}
}

// TestACarveOutIsStatedByTheRowThatGivesItUp: a package a row lists below another
// row's pattern is refused unless the pattern's row names it under carves; a
// carves line that names no listed package, or one no pattern of the row holds,
// is refused; and the carved packages are in the rendered table.
func TestACarveOutIsStatedByTheRowThatGivesItUp(t *testing.T) {
	t.Parallel()
	listed := strings.Replace(withPatterns(), "packages: [internal/root]", "packages: [internal/root, internal/cmds/version]", 1)
	stated := strings.Replace(listed, "patterns: [internal/cmds/...]", "patterns: [internal/cmds/...], carves: [internal/cmds/version]", 1)
	for _, tc := range []struct{ name, text, says string }{
		{"a package listed below a pattern and not stated", listed, "internal/cmds/version is in layer root, and pattern internal/cmds/... of layer cmds holds it"},
		{"a carves line that names no listed package", strings.Replace(stated, "carves: [internal/cmds/version]", "carves: [internal/cmds/version, internal/cmds/nobody]", 1), "carves internal/cmds/nobody, which no layer lists"},
		{"a carves line below no pattern of the row", strings.Replace(stated, "carves: [internal/cmds/version]", "carves: [internal/cmds/version, internal/root]", 1), "carves internal/root, which none of its patterns holds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := layers.Parse([]byte(tc.text))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Parse = %v, want ErrTable saying %q", err, tc.says)
			}
		})
	}
	t.Run("a stated carve-out parses and is in the table", func(t *testing.T) {
		tb := parse(t, stated)
		if doc := tb.Doc(); !strings.Contains(doc, "less `internal/cmds/version`, which their own rows hold") {
			t.Errorf("the rendered table does not list the carved package:\n%s", doc)
		}
	})
}

// TestTheListingRowCarvesItsPackageWhateverTheOrderOfTheRows: the pattern's row
// written before the row that lists the package holds it the same, so RowOf does
// not depend on which row comes first. Planted failure: RowOf's two loops made
// one.
func TestTheListingRowCarvesItsPackageWhateverTheOrderOfTheRows(t *testing.T) {
	t.Parallel()
	s := strings.Replace(withPatterns(), "packages: [internal/root]", "packages: [internal/root, internal/cmds/version]", 1)
	s = strings.Replace(s, "patterns: [internal/cmds/...]", "patterns: [internal/cmds/...], carves: [internal/cmds/version]", 1)
	rootLine := "  - {layer: root, kind: edge, packages: [internal/root, internal/cmds/version], holds: root, template: root, may: [internal/cmds/..., internal/port], why: \"root: no\"}\n"
	cmdsLine := "  - {layer: cmds, kind: edge, patterns: [internal/cmds/...], carves: [internal/cmds/version], holds: commands, template: cmds, may: [internal/port, internal/port/testing], why: \"cmds: no\"}\n"
	if !strings.Contains(s, rootLine+cmdsLine) {
		t.Fatal("the two rows are not where the test expects them")
	}
	swapped := strings.Replace(s, rootLine+cmdsLine, cmdsLine+rootLine, 1)
	if r := parse(t, swapped).RowOf("internal/cmds/version"); r == nil || r.Layer != "root" {
		t.Errorf("RowOf with the pattern's row first = %v, want root", r)
	}
}
