package explorer

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The explorer boards are static pages under docs/explorer-boards/canvas,
// one per view per width, and explorer/core/parts.seed.yaml lists the parts they
// name. These checks hold the boards to the issue's Done-when: every part a
// board names is in the seed, the breakpoints are exactly three, and every
// board draws the five states in both themes plus the text-grown frame.

const (
	canvasDir = "docs/explorer-boards/canvas"
	seedFile  = "explorer/core/parts.seed.yaml"
)

var (
	canvasPart  = regexp.MustCompile(`data-part="([^"]*)"`)
	canvasWidth = regexp.MustCompile(`data-width="([^"]*)"`)
	canvasRow   = regexp.MustCompile(`data-board-row="([^"]*)"`)
	seedName    = regexp.MustCompile(`(?m)^\s*(?:- )?name: (\S+)\s*$`)
)

// ownParts are the two data-part names a board uses for its own notes, which
// are not parts of the explorer.
func ownParts() []string { return []string{"note", "parts-table"} }

// widthOfSuffix is the width each file name ends in.
func widthOfSuffix() map[string]string {
	return map[string]string{"Phone": "390", "Tablet": "900", "Desktop": "1440"}
}

func canvasStates() []string {
	return []string{"default", "loading", "empty", "error", "stale", "grown"}
}
func canvasThemes() []string { return []string{"light", "dark"} }

// readCanvases is every board by name, and the seed.
func readCanvases(t *testing.T) (map[string]string, string) {
	t.Helper()
	tree := os.DirFS(root)
	boards := map[string]string{}
	err := fs.WalkDir(tree, canvasDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".dc.html") {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		boards[path.Base(p)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := fs.ReadFile(tree, seedFile)
	if err != nil {
		t.Fatal(err)
	}
	return boards, string(seed)
}

// canvasBreaks says what is wrong with the boards: a part not in the seed, a
// width that is not the file's, a state or a theme not drawn, an em dash, and a
// view that lacks one of the three widths.
func canvasBreaks(boards map[string]string, seed string) []string {
	var out []string
	inSeed := map[string]bool{}
	for _, m := range seedName.FindAllStringSubmatch(seed, -1) {
		if inSeed[m[1]] {
			out = append(out, seedFile+": part "+m[1]+" is listed twice")
		}
		inSeed[m[1]] = true
	}
	views := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(boards)) {
		text := boards[name]
		view, suffix := splitCanvasName(name)
		views[view] = append(views[view], suffix)
		for _, m := range canvasPart.FindAllStringSubmatch(text, -1) {
			if !inSeed[m[1]] && !slices.Contains(ownParts(), m[1]) {
				out = append(out, name+": part "+m[1]+" is on the board and not in the seed")
				inSeed[m[1]] = true
			}
		}
		if !canvasWidth.MatchString(text) {
			out = append(out, name+": no data-width, so no breakpoint is drawn")
		}
		for _, m := range canvasWidth.FindAllStringSubmatch(text, -1) {
			if m[1] != widthOfSuffix()[suffix] {
				out = append(out, name+": data-width "+m[1]+", want "+widthOfSuffix()[suffix])
			}
		}
		rows := map[string]bool{}
		for _, m := range canvasRow.FindAllStringSubmatch(text, -1) {
			rows[m[1]] = true
		}
		for _, s := range canvasStates() {
			for _, th := range canvasThemes() {
				if !rows[s+"-"+th] {
					out = append(out, name+": no "+s+" frame in the "+th+" theme")
				}
			}
		}
		if strings.Contains(text, "—") {
			out = append(out, name+": an em dash")
		}
	}
	for _, view := range slices.Sorted(maps.Keys(views)) {
		for suffix := range widthOfSuffix() {
			if !slices.Contains(views[view], suffix) {
				out = append(out, view+": no board at "+widthOfSuffix()[suffix])
			}
		}
	}
	if strings.Contains(seed, "—") {
		out = append(out, seedFile+": an em dash")
	}
	return out
}

func splitCanvasName(name string) (view, suffix string) {
	base := strings.TrimSuffix(name, ".dc.html")
	for s := range widthOfSuffix() {
		if v, ok := strings.CutSuffix(base, s); ok {
			return v, s
		}
	}
	return base, ""
}

// TestTheExplorerBoardsHoldToTheirRules: every part a board names is in the parts
// seed, the breakpoints are the three of the file names, and every board
// draws five states and the grown frame in light and dark.
// Risk: a board that names a part the catalogue will not know, or a state that
// was never drawn, passing review by eye.
func TestTheExplorerBoardsHoldToTheirRules(t *testing.T) {
	t.Parallel()
	boards, seed := readCanvases(t)
	if len(boards) == 0 {
		t.Fatal("no board under " + canvasDir)
	}
	for _, p := range canvasBreaks(boards, seed) {
		t.Error(p)
	}
}

// TestTheExplorerBoardChecksFireOnAPlantedBreach: each break the checks name, planted
// in a copy of the boards, is found and said.
func TestTheExplorerBoardChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	boards, seed := readCanvases(t)
	if got := canvasBreaks(boards, seed); len(got) != 0 {
		t.Fatalf("the boards are not clean to plant in: %v", got)
	}
	name := "RulesPhone.dc.html"
	with := func(edit func(string) string) map[string]string {
		c := maps.Clone(boards)
		c[name] = edit(c[name])
		return c
	}
	without := func(drop string) map[string]string {
		c := map[string]string{}
		for k, v := range boards {
			if k != drop {
				c[k] = v
			}
		}
		return c
	}
	tests := []struct {
		name   string
		boards map[string]string
		seed   string
		says   string
	}{
		{"a part not in the seed", with(func(s string) string { return s + `<i data-part="no-such-part"></i>` }), seed, "part no-such-part is on the board and not in the seed"},
		{"a fourth breakpoint", with(func(s string) string { return strings.Replace(s, `data-width="390"`, `data-width="768"`, 1) }), seed, "data-width 768, want 390"},
		{"a state not drawn", with(func(s string) string {
			return strings.ReplaceAll(s, `data-board-row="error-dark"`, `data-board-row="x"`)
		}), seed, "no error frame in the dark theme"},
		{"the grown frame not drawn", with(func(s string) string {
			return strings.ReplaceAll(s, `data-board-row="grown-light"`, `data-board-row="x"`)
		}), seed, "no grown frame in the light theme"},
		{"an em dash in a board", with(func(s string) string { return s + "a — b" }), seed, "an em dash"},
		{"an em dash in the seed", boards, seed + "# a — b\n", seedFile + ": an em dash"},
		{"a board with no data-width", with(func(s string) string { return canvasWidth.ReplaceAllString(s, `data-w="x"`) }), seed, "no data-width, so no breakpoint is drawn"},
		{"a part listed twice in the seed", boards, seed + "\n      - name: header\n", "part header is listed twice"},
		{"a view missing a width", without("RulesTablet.dc.html"), seed, "Rules: no board at 900"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canvasBreaks(tc.boards, tc.seed)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("planted %s: %q not said in %q", tc.name, tc.says, got)
			}
		})
	}
}
