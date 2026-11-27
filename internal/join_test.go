package internal

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/dev/join"
)

// TestTheJoinNamesEveryCommandPackage: internal/cli/root/join_gen.go is what
// gragas-dev gen commands writes from the folders below internal/cli/cmd, so a
// command package that is not joined, or a join of one that is gone, fails here
// as well as at make gen-check (arch.ports.12: the root is the only joiner).
func TestTheJoinNamesEveryCommandPackage(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(root)
	names, err := join.Names(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no command package is below " + join.CmdDir + ", so the join proves nothing")
	}
	check := func(fsys fstest.MapFS, names []string) []string {
		want, err := join.Render(table(t).Module, names)
		if err != nil {
			t.Fatal(err)
		}
		if have := string(fsys[join.File].Data); have != string(want) {
			return []string{join.File + " is not what the folders below " + join.CmdDir + " give: " + strings.Join(names, ", ")}
		}
		return nil
	}
	have, err := os.ReadFile(root + "/" + join.File)
	if err != nil {
		t.Fatal(err)
	}
	joined := fstest.MapFS{join.File: {Data: have}}
	for _, f := range check(joined, names) {
		t.Error(f)
	}
	if got := check(joined, append(names, "planted")); len(got) != 1 {
		t.Errorf("a command package the join lacks was not told: %v", got)
	}
}
