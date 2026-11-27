package internal

// The two depguard rows of gragas's own (gragas#917, P2): no package imports a
// Charm package, since the terminal and its look are go-console's, and nothing
// under cmd/ imports os/signal, since the stop signal is ui.Execute's. The rows
// are in lint/project.yml's layer-rows text, rendered into .golangci.yml.

import (
	"maps"
	"os/exec"
	"strings"
	"testing"
)

// charmRowPlants is a module of the plants of the two rows: each import the
// rows name, in a package where it is refused, and the same imports where
// they are not. The imported modules are stand-ins that replace directives
// point at, so the plants build with no network.
func charmRowPlants(config string) map[string]string {
	stubs := map[string]string{
		"charm.land/lipgloss/v2":           "lipgloss",
		"github.com/charmbracelet/x/ansi":  "ansi",
		"github.com/charmbracelet/bubbles": "bubbles",
	}
	var requires, replaces strings.Builder
	files := map[string]string{".golangci.yml": config}
	for path, pkg := range stubs {
		dir := "stub/" + strings.ReplaceAll(path, "/", "_")
		files[dir+"/go.mod"] = "module " + path + "\n\ngo 1.23.0\n"
		files[dir+"/p.go"] = "// Package " + pkg + " is a stand-in.\npackage " + pkg + "\n\n// Name is a stand-in.\nconst Name = \"x\"\n"
		version := "v0.0.0"
		if strings.HasSuffix(path, "/v2") {
			version = "v2.0.0"
		}
		requires.WriteString("\t" + path + " " + version + "\n")
		replaces.WriteString("\t" + path + " => ./" + dir + "\n")
	}
	files["go.mod"] = "module example.com/planted\n\ngo 1.23.0\n\nrequire (\n" + requires.String() + ")\n\nreplace (\n" + replaces.String() + ")\n"
	use := func(pkg, imp, name string) string {
		return "// Package " + pkg + " is a plant.\npackage " + pkg + "\n\nimport \"" + imp + "\"\n\n// Use reads the stand-in.\nfunc Use() string { return " + name + ".Name }\n"
	}
	maps.Copy(files, map[string]string{
		"plantlip/x.go":  use("plantlip", "charm.land/lipgloss/v2", "lipgloss"),
		"plantansi/x.go": use("plantansi", "github.com/charmbracelet/x/ansi", "ansi"),
		"plantbub/x.go":  use("plantbub", "github.com/charmbracelet/bubbles", "bubbles"),
		"tools/cmd/sig/main.go": "// Command sig is a plant: a command that reads the signal itself.\npackage main\n\n" +
			"import (\n\t\"os\"\n\t\"os/signal\"\n)\n\nfunc main() { signal.Ignore(os.Interrupt) }\n",
		"plantsig/x.go": "// Package plantsig is a control: os/signal outside cmd/ is these rows' to leave.\npackage plantsig\n\n" +
			"import (\n\t\"os\"\n\t\"os/signal\"\n)\n\n// Quiet ignores the interrupt.\nfunc Quiet() { signal.Ignore(os.Interrupt) }\n",
		"plain/x.go": "// Package plain is a control: it names no Charm package and no signal.\npackage plain\n\n// Is names none.\nfunc Is() string { return \"none\" }\n",
	})
	return files
}

// TestTheLintShowsACharmImportAndASignalUnderCmdRed runs the pinned
// golangci-lint with the repository's own .golangci.yml over the plants of
// charmRowPlants: an import of charm.land, of github.com/charmbracelet and of
// os/signal under cmd/ is each reported by its row, and os/signal elsewhere and
// a package that imports neither are reported by neither.
// Planted failure: lint/project.yml drops the no-charm row or one of its two
// prefixes, or the no-signal-in-cmd row, or widens its files line to all of
// the tree.
func TestTheLintShowsACharmImportAndASignalUnderCmdRed(t *testing.T) {
	bin := pinnedLint(t)
	dir := writeModule(t, charmRowPlants(lintConfig(t)))
	out, ok := lint(t, bin, dir, exec.CommandContext(t.Context(), "golangci-lint", "run",
		"--enable-only=depguard", "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "--allow-parallel-runners", "./..."))
	if ok {
		t.Fatalf("the plants are not reported:\n%s", out)
	}
	for _, want := range []string{"plantlip/x.go", "plantansi/x.go", "plantbub/x.go", "tools/cmd/sig/main.go"} {
		var found bool
		for line := range strings.SplitSeq(out, "\n") {
			found = found || strings.HasPrefix(line, want) && strings.Contains(line, "gragas#917") && strings.Contains(line, "(depguard)")
		}
		if !found {
			t.Errorf("the lint does not name %s by a gragas#917 row:\n%s", want, out)
		}
	}
	for line := range strings.SplitSeq(out, "\n") {
		for _, spared := range []string{"plantsig/", "plain/"} {
			if strings.HasPrefix(line, spared) {
				t.Errorf("a control is reported: %s", line)
			}
		}
	}
}
