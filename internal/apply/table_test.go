package apply_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// backslashed is the values of testdata/backslash.txt by key: a line is a key,
// a space and the value, a comment starts with #, and \\, \n and \t are read
// as escapes.
func backslashed(tb testing.TB) map[string]string {
	tb.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata"), "backslash.txt")
	if err != nil {
		tb.Fatal(err)
	}
	out := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, _ := strings.Cut(line, " ")
		out[k] = strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\t`, "\t").Replace(v)
	}
	return out
}
