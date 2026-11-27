package layers_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// joinBase is a base with every slot the table fills and two a project may
// change.
const joinBase = `rules:
  # slot flags begin
  files: ["$all"]
  # slot flags end
  deny:
    # slot layer-rows begin
    # default comment
    # slot layer-rows end
    # slot shared-imports begin
    # slot shared-imports end
    # slot owner begin
    files: ["$all", "!**/old/**"]
    # slot owner end
  forbid:
    # slot shared-calls begin
    # slot shared-calls end
  exclusions:
    rules:
      # slot exclusions-generated begin
      # slot exclusions-generated end
`

const joinProject = `flags: |
  files: ["$all", "!**/cli/**"]

  more: 1
owner: |
  files: ["$all", {{programs}}]
layer-rows: |
  # rows below are rendered
`

// TestTheProjectTextTakesTheSlotsPlaceAndTheBaseKeepsTheRest: a named slot is
// the project's text at the slot's indent, an unnamed one keeps the base's
// default, the slot lines are not written, and the table's blocks stand in
// their markers. Risk: a project override that is dropped or a default that is
// lost.
func TestTheProjectTextTakesTheSlotsPlaceAndTheBaseKeepsTheRest(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	got, err := tb.Join([]byte(joinBase), []byte(joinProject))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		"  files: [\"$all\", \"!**/cli/**\"]\n",
		"    # rows below are rendered\n    " + layers.BeginMark + "\n",
		"    files: [\"$all\", \"!**/internal/host/x/**\"]\n",
		layers.ProgramsBeginMark,
		"    " + layers.ImportsBeginMark + "\n    " + layers.ImportsEndMark + "\n",
		"    " + layers.CallsBeginMark + "\n    " + layers.CallsEndMark + "\n",
		layers.OwnersBeginMark,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the lint file lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "# slot") || strings.Contains(s, "default comment") || strings.Contains(s, "!**/old/**") {
		t.Errorf("a slot line or a replaced default is left:\n%s", s)
	}
	if !strings.Contains(s, "\"!**/cli/**\"]\n\n  more: 1\n") {
		t.Errorf("a blank line in the project text was indented or dropped:\n%s", s)
	}
	kept, err := tb.Join([]byte(joinBase), []byte("{}\n"))
	if err != nil || !strings.Contains(string(kept), "    # default comment\n") || !strings.Contains(string(kept), "!**/old/**") || !strings.Contains(string(kept), "  files: [\"$all\"]\n") {
		t.Errorf("an unnamed slot lost its default: %v\n%s", err, kept)
	}
	again, err := tb.Join([]byte(joinBase), []byte(joinProject))
	if err != nil || string(again) != s {
		t.Errorf("a second join differs: %v", err)
	}
}

// TestABaseAndAProjectThatDoNotFitAreRefused: each way they can fail to fit is
// ErrSlots and says which slot.
func TestABaseAndAProjectThatDoNotFitAreRefused(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	for _, tc := range []struct {
		name, base, project, says string
	}{
		{"a slot the base lacks", joinBase, joinProject + "nowhere: |\n  x\n", "slot nowhere"},
		{"a slot never ended", strings.Replace(joinBase, "  # slot flags end\n", "", 1), "{}\n", "slot flags is never ended"},
		{"a slot twice", joinBase + "  # slot flags begin\n  # slot flags end\n", "{}\n", "slot flags is there twice"},
		{"an end with no begin", "  # slot extra end\n" + joinBase, "{}\n", "ends a slot that was not begun"},
		{"text for the generated exclusions", joinBase, "exclusions-generated: |\n  x\n", "takes no text"},
		{"no layer-rows slot", strings.Replace(joinBase, "layer-rows", "rows", 2), "{}\n", "no slot layer-rows"},
		{"no exclusions slot", strings.ReplaceAll(joinBase, "exclusions-generated", "other"), "{}\n", "no slot exclusions-generated"},
		{"text for the shared calls", joinBase, "shared-calls: |\n  x\n", "takes no text"},
		{"text for the shared imports", joinBase, "shared-imports: |\n  x\n", "takes no text"},
		{"no shared-calls slot", strings.ReplaceAll(joinBase, "shared-calls", "other-calls"), "{}\n", "no slot shared-calls"},
		{"no shared-imports slot", strings.ReplaceAll(joinBase, "shared-imports", "other-imports"), "{}\n", "no slot shared-imports"},
		{"a project that is no mapping", joinBase, "- a\n- b\n", "not a mapping"},
		{"a slot that is no text", joinBase, "flags: [a]\n", "slot flags is not text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tb.Join([]byte(tc.base), []byte(tc.project))
			if !errors.Is(err, layers.ErrSlots) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Join = %v, want ErrSlots saying %q", err, tc.says)
			}
		})
	}
}

// TestTheProgramGlobsNameTheAdapterOrItsOneFile: a program row with a file
// spares that file alone, one without spares the package, and a file that is
// no plain .go file is refused. Risk: the os/exec exclusion wider than the
// table says.
func TestTheProgramGlobsNameTheAdapterOrItsOneFile(t *testing.T) {
	t.Parallel()
	if got, want := parse(t, small).ProgramGlobs(), `"!**/internal/host/x/**"`; got != want {
		t.Errorf("ProgramGlobs = %s, want %s", got, want)
	}
	withFile := strings.Replace(small, "{program: xtool, adapter: internal/host/x}", "{program: xtool, adapter: internal/host/x, file: host_x.go}", 1)
	if got, want := parse(t, withFile).ProgramGlobs(), `"!**/internal/host/x/host_x.go"`; got != want {
		t.Errorf("ProgramGlobs with a file = %s, want %s", got, want)
	}
	twice := strings.Replace(small, "  - {program: xtool, adapter: internal/host/x}\n", "  - {program: xtool, adapter: internal/host/x}\n  - {program: ytool, adapter: internal/host/x}\n", 1)
	if twice == small {
		t.Fatal("the plant added no program")
	}
	if got, want := parse(t, twice).ProgramGlobs(), `"!**/internal/host/x/**"`; got != want {
		t.Errorf("two programs on one adapter: ProgramGlobs = %s, want one glob, %s", got, want)
	}
	for _, bad := range []string{"../x.go", "a/b.go", "x.txt", "x_test.go", "*.go", "a?.go", "[a].go", "a\"b.go", ".hidden.go", "a{b,c}.go", "a,b.go", "A.go", ".go"} {
		plant := strings.Replace(small, "{program: xtool, adapter: internal/host/x}", "{program: xtool, adapter: internal/host/x, file: '"+bad+"'}", 1)
		if _, err := layers.Parse([]byte(plant)); !errors.Is(err, layers.ErrTable) {
			t.Errorf("file %q: Parse = %v, want ErrTable", bad, err)
		}
	}
}

// TestThePresetIsTheBaseAndTheLintFileHasNoSlotLines: the shared preset's
// settings file is lint/base.yml byte for byte, but for the switch list: the
// base's linters.enable entries are the rulebook's bindings' to write, so the
// settings file holds the two markers in their place (gragas#1743). The lint
// file written from the base holds no slot line. Risk: a preset that drifts
// from the base projects copy.
func TestThePresetIsTheBaseAndTheLintFileHasNoSlotLines(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..")
	base, err := os.ReadFile(filepath.Join(root, "lint", "base.yml"))
	if err != nil {
		t.Fatal(err)
	}
	preset, err := os.ReadFile(filepath.Join(root, "presets", "golangci-lint", "settings.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var want, got []string
	entries, inList := 0, false
	for l := range strings.SplitSeq(string(base), "\n") {
		if inList && strings.HasPrefix(l, "    - ") {
			entries++
			continue
		}
		inList = l == "  enable:"
		want = append(want, l)
	}
	markers := 0
	for l := range strings.SplitSeq(string(preset), "\n") {
		if l == "    # generated:linters:begin" || l == "    # generated:linters:end" {
			markers++
			continue
		}
		got = append(got, l)
	}
	if entries == 0 || markers != 2 {
		t.Fatalf("lint/base.yml holds %d linters.enable entries and the settings file %d marker lines; want some and 2", entries, markers)
	}
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Error("presets/golangci-lint/settings.yml is not lint/base.yml with the markers in place of its linters.enable entries")
	}
	lint, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for l := range strings.SplitSeq(string(lint), "\n") {
		if tr := strings.TrimSpace(l); strings.HasPrefix(tr, "# slot ") {
			t.Errorf(".golangci.yml holds a slot line: %q", l)
		}
	}
}
