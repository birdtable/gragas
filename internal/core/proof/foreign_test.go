package proof_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// foreignPaths is the cases of testdata/foreign_paths.txt: one a line, a
// comment starting with #, and \\, \n and \t read as escapes.
func foreignPaths(tb testing.TB) []string {
	tb.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), "foreign_paths.txt")
	if err != nil {
		tb.Fatal(err)
	}
	var out []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\t`, "\t").Replace(line))
	}
	return out
}
