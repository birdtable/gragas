package join_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/join"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func tree(files ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, f := range files {
		fsys[f] = &fstest.MapFile{Data: []byte("package x\n")}
	}
	return fsys
}

// TestNamesAreTheFoldersThatHoldACommand: sorted, a folder of tests alone holds
// none, a file beside the folders is no command, and no folder at all is none.
func TestNamesAreTheFoldersThatHoldACommand(t *testing.T) {
	t.Parallel()
	fsys := tree(join.CmdDir+"/version/command.go", join.CmdDir+"/check/command.go", join.CmdDir+"/check/command_test.go",
		join.CmdDir+"/onlytests/x_test.go", join.CmdDir+"/README.md", join.CmdDir+"/empty/.keep")
	got, err := join.Names(fsys)
	if err != nil || !slices.Equal(got, []string{"check", "version"}) {
		t.Errorf("Names = %v, %v, want [check version]", got, err)
	}
	if got, err := join.Names(fstest.MapFS{}); err != nil || got != nil {
		t.Errorf("no folder: Names = %v, %v, want none", got, err)
	}
}

// TestAFolderThatIsNoWordIsRefused: its name is its package name, which the
// join imports it by.
func TestAFolderThatIsNoWordIsRefused(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"Check", "my-cmd", "a_b", "1st"} {
		_, err := join.Names(tree(join.CmdDir + "/" + bad + "/command.go"))
		if !errors.Is(err, join.ErrJoin) || !strings.Contains(err.Error(), bad) {
			t.Errorf("%s: Names = %v, want ErrJoin naming it", bad, err)
		}
	}
}

// TestTheJoinNamesEveryPackageInOrder, and a tree with none is a join with an
// empty list that still compiles.
func TestTheJoinNamesEveryPackageInOrder(t *testing.T) {
	t.Parallel()
	got, err := join.Render("example.com/m", []string{"check", "version"})
	if err != nil {
		t.Fatal(err)
	}
	want := `// Written by gragas-dev gen commands from internal/cli/cmd, which task gen runs; edit no line of it.

package root

import (
	checkcmd "example.com/m/internal/cli/cmd/check"
	versioncmd "example.com/m/internal/cli/cmd/version"
)

// built are the command packages below internal/cli/cmd, by name: the row each
// declares and the cobra command it makes.
func built() []builtCommand {
	return []builtCommand{
		{Row: checkcmd.Row(), New: checkcmd.New},
		{Row: versioncmd.Row(), New: versioncmd.New},
	}
}
`
	if string(got) != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
	empty, err := join.Render("example.com/m", nil)
	if err != nil || !strings.Contains(string(empty), "return []builtCommand{}") || strings.Contains(string(empty), "import") {
		t.Errorf("an empty join = %q, %v", empty, err)
	}
}
