package journeys

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// tableOf is the cases of testdata/name: one case per line, its fields
// separated by a tab, # for a comment, and the escapes \\ \n and \t (the
// convention of #1708). Cases that name a system's path or variable live
// there, so no test file holds the literal.
func tableOf(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	var cases [][]string
	for line := range strings.SplitSeq(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		for i, f := range fields {
			fields[i] = unescaped(f)
		}
		cases = append(cases, fields)
	}
	return cases
}

func unescaped(s string) string {
	return strings.ReplaceAll(strings.NewReplacer(`\\`, "\x00", `\n`, "\n", `\t`, "\t").Replace(s), "\x00", `\`)
}
