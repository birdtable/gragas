package root

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// word is the value testdata/words.txt holds under key: a word a quoting test
// holds to, whose bytes name a system's variable or path. Each line is a key
// and a value, tab-separated, with the escapes \\ and \t (the convention of
// #1708).
func word(t *testing.T, key string) string {
	t.Helper()
	b, err := fs.ReadFile(os.DirFS("testdata"), "words.txt")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		k, v, ok := strings.Cut(line, "\t")
		if ok && k == key {
			return strings.ReplaceAll(strings.NewReplacer(`\\`, "\x00", `\t`, "\t").Replace(v), "\x00", `\`)
		}
	}
	t.Fatalf("testdata/words.txt holds no %q", key)
	return ""
}
