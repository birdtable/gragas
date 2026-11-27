package internal

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/ordered"
)

// devTools returns the closed list of the dev tools: the packages of the dev tools at the day
// it was closed, cmd/gragas-* and internal/dev. A package of those folders not on it needs a row with
// home: or product: that names it, as a shared package does. The list is only shortened: a line that
// names a package the tree no longer has is red, so the list falls with the tree.
func devTools() []string {
	return []string{
		"cmd/gragas-crash",
		"cmd/gragas-dev",
		"cmd/gragas-docs",
		"cmd/gragas-generate",
		"cmd/gragas-names",
		"cmd/gragas-new",
		"cmd/gragas-release",
		"cmd/gragas-subset",
		"cmd/gragas-tmp",
		"internal/dev/census",
		"internal/dev/gate",
		"internal/dev/generate",
		"internal/dev/join",
		"internal/dev/layers",
		"internal/dev/newcommand",
		"internal/dev/say",
		"internal/dev/scan",
		"internal/dev/surface",
		"internal/dev/templates/command",
		"internal/dev/tokens",
	}
}

// linesRow is the budget row of the dev tools' size (gate/budgets.yaml, kind: lines).
const linesRow = "dev-tool-lines"

// sayPackage is the dev tools' shared first-error writer, which their ceiling counts with cmd/gragas-*, so
// the copies it took the place of stay in the count.
const sayPackage = "internal/dev/say"

// devPackage says whether rel is a dev tool: a command package of cmd/gragas-* (cmd/gragas is the
// product) or a package of internal/dev.
func devPackage(rel string) bool {
	return strings.HasPrefix(rel, "cmd/gragas-") || under(rel, "internal/dev")
}

// devToolsOf are the dev tools of the world, read from go list.
func devToolsOf(w world) ([]string, error) {
	var out []string
	for _, p := range w.sorted() {
		if devPackage(p.path) {
			out = append(out, p.path)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("could not run: go list found no dev tool, and the tree has " + strconv.Itoa(len(devTools())))
	}
	return out, nil
}

// devClosed is every finding of line 2: a dev package off the closed list that is not named by a row with
// home: or product:, and a name on the list that the tree no longer has.
func devClosed(tb *layers.Table, w world, list []string) ([]string, error) {
	dev, err := devToolsOf(w)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rel := range dev {
		if slices.Contains(list, rel) {
			continue
		}
		// Off the list, a package is let in by its row alone: reported only when the row lacks the key or
		// does not name it, so the list never has to grow to let a tool in.
		if bad := homeProblems(tb, rel); len(bad) != 0 {
			out = append(out, rel+": a dev tool not on the closed list of dev_closed_test.go; "+
				"a new one needs its row in "+layers.File+" to carry home: or product: and name it in its why")
			out = append(out, bad...)
		}
	}
	for _, rel := range list {
		if !slices.Contains(dev, rel) {
			out = append(out, rel+": on the closed list of dev tools and no package of the tree: delete the line")
		}
	}
	return out, nil
}

// linesLimits are the budget rows of kind: lines, by id, the limit of each. A file with no such row, a row
// with no limit or one that is no positive whole number, a row with no id or two with one are refused: a
// ceiling that is not there is not a pass.
func linesLimits(data []byte) (map[string]int, error) {
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return nil, err
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, errors.New("budgets.yaml is not a mapping")
	}
	out := map[string]int{}
	for _, m := range ordered.Maps(doc.List("budgets")) {
		if m.Str("kind") != "lines" {
			continue
		}
		id := m.Str("id")
		if id == "" {
			return nil, fmt.Errorf("line %d: a row of kind lines has no id", m.Line)
		}
		if _, dup := out[id]; dup {
			return nil, fmt.Errorf("line %d: two rows of kind lines are %s", m.Line, id)
		}
		n, ok := m.Val("limit").(int64)
		if !ok || n <= 0 {
			return nil, fmt.Errorf("line %d: the limit of %s is no positive whole number", m.Line, id)
		}
		if m.Str("unit") != "lines" {
			return nil, fmt.Errorf("line %d: the unit of %s is not lines", m.Line, id)
		}
		out[id] = int(n)
	}
	if len(out) == 0 {
		return nil, errors.New("budgets.yaml has no row of kind lines")
	}
	return out, nil
}

// devLines is the non-test Go lines of cmd/gragas-*, counted as wc -l counts: the files go list names for
// each of those packages, the ones another system builds among them.
func devLines(w world, read func(rel string) ([]byte, error)) (int, error) {
	n, files := 0, 0
	for _, p := range w.sorted() {
		if !strings.HasPrefix(p.path, "cmd/gragas-") && p.path != sayPackage {
			continue
		}
		for _, f := range p.files {
			data, err := read(path.Join(p.path, f))
			if err != nil {
				return 0, err
			}
			files++
			n += strings.Count(string(data), "\n")
		}
	}
	if files == 0 {
		return 0, errors.New("could not run: go list names no file of cmd/gragas-*")
	}
	return n, nil
}

// devLinesProblems holds the dev tools to their ceiling: over it is red, with the count and the way out.
func devLinesProblems(w world, data []byte, read func(rel string) ([]byte, error)) ([]string, error) {
	limits, err := linesLimits(data)
	if err != nil {
		return nil, err
	}
	limit, ok := limits[linesRow]
	if !ok {
		return nil, errors.New("gate/budgets.yaml has no row " + linesRow + " of kind lines")
	}
	n, err := devLines(w, read)
	if err != nil {
		return nil, err
	}
	if n > limit {
		return []string{fmt.Sprintf("cmd/gragas-* and "+sayPackage+" hold %d non-test lines, over the %d of %s in gate/budgets.yaml: "+
			"the dev tools only shrink, and a ceiling raised is a reviewed line that says why", n, limit, linesRow)}, nil
	}
	return nil, nil
}

func fileReader(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
}

// TestDevIsClosed holds that the dev tools, cmd/gragas-* and internal/dev,
// are a closed list, and the lines of cmd/gragas-* have a ceiling in gate/budgets.yaml. Risk: gate tooling
// that grows by a package or by lines nobody asked whether platform's hatch has.
func TestDevIsClosed(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)
	problems, err := devClosed(tb, w, devTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
	lines, err := devLinesProblems(w, []byte(read(t, "gate/budgets.yaml")), fileReader)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range lines {
		t.Error(p)
	}
}

// TestDevIsClosedPlanted plants each way lines 2 and 3 are broken: a dev package off the list, with and
// without a row that carries the key and names it; a name on the list that the tree lost; a dev tool, or the
// shared writer internal/dev/say, grown past the ceiling; and each missing subject (no dev package, a
// budgets file with no lines row, a row with no limit, a tree with no cmd/gragas-* file), which is refused
// and never passes.
func TestDevIsClosedPlanted(t *testing.T) {
	t.Parallel()
	tb, w := tree(t)

	fresh := w.clone()
	fresh.pkgs["cmd/gragas-fresh"] = &pkg{path: "cmd/gragas-fresh", name: "main", files: []string{"main.go"}}
	got, err := devClosed(table(t), fresh, devTools())
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "cmd/gragas-fresh: a dev tool not on the closed list")
	fires(t, got, "cmd/gragas-fresh: in no layer")

	fresh.pkgs["internal/dev/fresh"] = &pkg{path: "internal/dev/fresh", name: "fresh", files: []string{"f.go"}}
	got, err = devClosed(table(t), fresh, devTools())
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "internal/dev/fresh: a dev tool not on the closed list")
	// internal/dev/... is a pattern of the dev row, which names no home and no product: a package under it is no more welcome.
	fires(t, got, "internal/dev/fresh: layer dev carries neither home:")

	// A new dev tool whose own row carries the key and names it is let in.
	named := table(t)
	named.Rows = append(named.Rows, layers.Row{Layer: "fresh-entry", Kind: layers.KindEdge, Packages: []string{"cmd/gragas-fresh"}, Product: "a reason", Why: "cmd/gragas-fresh is the product's"})
	fresh2 := w.clone()
	fresh2.pkgs["cmd/gragas-fresh"] = fresh.pkgs["cmd/gragas-fresh"]
	got, err = devClosed(named, fresh2, devTools())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a dev tool whose row carries a product and names it was refused: %q", got)
	}

	// A name on the list that the tree lost.
	short := w.clone()
	delete(short.pkgs, "cmd/gragas-tmp")
	got, err = devClosed(tb, short, devTools())
	if err != nil {
		t.Fatal(err)
	}
	fires(t, got, "cmd/gragas-tmp: on the closed list of dev tools and no package of the tree")

	// The real tree is clean.
	if got, err = devClosed(tb, w, devTools()); err != nil || len(got) != 0 {
		t.Errorf("the real tree is red: %q %v", got, err)
	}

	// The ceiling: a file grown by one line over it.
	data := []byte(read(t, "gate/budgets.yaml"))
	limits, err := linesLimits(data)
	if err != nil {
		t.Fatal(err)
	}
	// The plant is built from the ceiling alone (gragas#1905): the one file holds exactly the lines asked and every
	// other file of cmd/gragas-* holds none, so the tree is over by one, at, or under the ceiling whatever the real
	// tree holds, and the real tree may itself be over it without the plant going negative.
	target := path.Join("cmd/gragas-dev", w.pkgs["cmd/gragas-dev"].files[0])
	only := func(lines int) func(string) ([]byte, error) {
		return func(rel string) ([]byte, error) {
			if rel == target {
				return []byte(strings.Repeat("// planted\n", lines)), nil
			}
			return nil, nil
		}
	}
	limit := limits[linesRow]
	probs, err := devLinesProblems(w, data, only(limit+1))
	if err != nil {
		t.Fatal(err)
	}
	fires(t, probs, "hold "+strconv.Itoa(limit+1)+" non-test lines, over the "+strconv.Itoa(limit)+" of "+linesRow)
	for name, lines := range map[string]int{"at the ceiling": limit, "under the ceiling": limit - 1, "nothing": 1} {
		if probs, err = devLinesProblems(w, data, only(lines)); err != nil || len(probs) != 0 {
			t.Errorf("a tree %s is red: %q %v", name, probs, err)
		}
	}
	// The shared writer counts with the dev tools (gragas#1897): the same plant in internal/dev/say alone fires.
	sayFile := path.Join(sayPackage, w.pkgs[sayPackage].files[0])
	probs, err = devLinesProblems(w, data, func(rel string) ([]byte, error) {
		if rel == sayFile {
			return []byte(strings.Repeat("// planted\n", limit+1)), nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fires(t, probs, "hold "+strconv.Itoa(limit+1)+" non-test lines, over the "+strconv.Itoa(limit)+" of "+linesRow)

	// The subject missing.
	if _, err := devClosed(tb, world{module: tb.Module, pkgs: map[string]*pkg{}}, devTools()); err == nil {
		t.Error("a go list with no dev tool passed")
	}
	if _, err := devLinesProblems(w, []byte("budgets:\n  - id: tier-x\n    kind: tier\n    unit: seconds\n    limit: 1\n"), fileReader); err == nil {
		t.Error("a budgets file with no lines row passed")
	}
	if _, err := devLinesProblems(w, []byte("budgets:\n  - id: other\n    kind: lines\n    unit: lines\n    limit: 5\n"), fileReader); err == nil {
		t.Error("a budgets file with no " + linesRow + " row passed")
	}
	for name, text := range map[string]string{
		"no limit":      "budgets:\n  - id: " + linesRow + "\n    kind: lines\n    unit: lines\n",
		"a zero limit":  "budgets:\n  - id: " + linesRow + "\n    kind: lines\n    unit: lines\n    limit: 0\n",
		"a text limit":  "budgets:\n  - id: " + linesRow + "\n    kind: lines\n    unit: lines\n    limit: many\n",
		"a wrong unit":  "budgets:\n  - id: " + linesRow + "\n    kind: lines\n    unit: bytes\n    limit: 5\n",
		"a repeated id": "budgets:\n  - id: " + linesRow + "\n    kind: lines\n    unit: lines\n    limit: 5\n  - id: " + linesRow + "\n    kind: lines\n    unit: lines\n    limit: 6\n",
	} {
		if _, err := linesLimits([]byte(text)); err == nil {
			t.Errorf("%s: a budgets file was read", name)
		}
	}
	none := world{module: tb.Module, pkgs: map[string]*pkg{"cmd/gragas": {path: "cmd/gragas", name: "main", files: []string{"main.go"}}}}
	if _, err := devLinesProblems(none, data, fileReader); err == nil {
		t.Error("a tree with no cmd/gragas-* file passed")
	}
}
