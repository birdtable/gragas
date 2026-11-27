package contracttest

import (
	"embed"
	"strings"
	"testing"
)

// tables holds the suites' lists of hostile paths, one case a line, kept as
// data so no suite's source spells a path of one system.
//
//go:embed testdata/*.txt
var tables embed.FS

// table is the cases of testdata/<name>.txt: each line that is not empty and
// does not start with #, with \\, \n and \t read as a backslash, a newline and
// a tab. A table that cannot be read, or holds no case, fails t: a suite run
// with no cases would pass and prove nothing.
func table(t testing.TB, name string) []string {
	t.Helper()
	data, err := tables.ReadFile("testdata/" + name + ".txt")
	if err != nil {
		t.Fatalf("contracttest: the table %s is not embedded: %v", name, err)
	}
	var cases []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cases = append(cases, unescape(line))
	}
	if len(cases) == 0 {
		t.Fatalf("contracttest: the table %s holds no case", name)
	}
	return cases
}

// unescape reads \\, \n and \t in line; a backslash before any other
// character is kept as it is.
func unescape(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] != '\\' || i+1 == len(line) {
			b.WriteByte(line[i])
			continue
		}
		switch line[i+1] {
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(line[i])
			continue
		}
		i++
	}
	return b.String()
}
