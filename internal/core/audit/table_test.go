package audit_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// table is the cases of testdata/<name>: one a line, a comment starting with
// #, and \\, \n and \t read as escapes.
func table(tb testing.TB, name string) []string {
	tb.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), name)
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
