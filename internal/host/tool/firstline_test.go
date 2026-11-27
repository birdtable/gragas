package tool_test

import (
	"path/filepath"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/host/tool"
)

// TestAFirstLineIsReadInsideTheProgramsFolder: the first line of a program,
// read for its interpreter after the walk judged it, is read inside the
// program's own folder, so a name that is a link out of that folder, as one
// changed since the walk would be, is refused and not read (supply.scans.3,
// gosec G304).
// Risk: a program swapped for a link after the walk has its interpreter read
// from a file nobody judged.
func TestAFirstLineIsReadInsideTheProgramsFolder(t *testing.T) {
	t.Parallel()
	bin, outside := t.TempDir(), t.TempDir()
	first := viaEnv(t, "sh")
	filestest.WriteProgram(t, filepath.Join(bin, "lint-a"), []byte(first+"\necho a\n"))
	if line, err := tool.FirstLine(filepath.Join(bin, "lint-a")); err != nil || line != first {
		t.Fatalf("a program in its folder = %q, %v; want its first line", line, err)
	}
	filestest.WriteProgram(t, filepath.Join(outside, "other"), []byte(viaEnv(t, "other")+"\n"))
	testenv.Link(t, filepath.Join(outside, "other"), filepath.Join(bin, "lint-b"))
	if line, err := tool.FirstLine(filepath.Join(bin, "lint-b")); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("a program that is a link out of its folder = %q, %v; want refused", line, err)
	}
}
