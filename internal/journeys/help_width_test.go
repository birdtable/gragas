package journeys

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// helpWidth is the width help is held to piped: 80 columns must work (cli.environment.2).
const helpWidth = 80

// TestEveryHelpFitsEightyColumnsPiped: the help of the root and of every
// command, piped at COLUMNS=80, has no line wider than 80. Since go-console
// v0.1.3 help wraps its rows to the width, piped as on a
// terminal; this holds it there at 80, and pty_narrow_test.go holds help on a
// terminal at 60 and 40 like every other screen. Width is counted in runes:
// piped help is plain and holds no wide character, which the test also
// asserts.
func TestEveryHelpFitsEightyColumnsPiped(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	env := map[string]string{"COLUMNS": "80"}
	paths := commandPaths(t, repo, env, nil)
	if len(paths) < 10 {
		t.Fatalf("found %d commands, want the whole tree: %q", len(paths), paths)
	}
	for _, p := range paths {
		r := run(t, repo, env, append(append([]string{}, p...), "--help")...)
		name := strings.Join(append([]string{"gragas"}, p...), " ")
		if r.code != 0 {
			t.Errorf("%s --help: exit %d\n%s%s", name, r.code, r.stdout, r.stderr)
			continue
		}
		for i, line := range strings.Split(r.stdout, "\n") {
			n := utf8.RuneCountInString(line)
			if n > helpWidth {
				t.Errorf("%s --help, line %d is %d wide, %d past 80: %q", name, i+1, n, n-helpWidth, line)
			}
			for _, c := range line {
				if (c >= 0x1100 && c <= 0x115f) || c >= 0x2e80 { // Hangul Jamo, CJK and on, emoji
					t.Errorf("%s --help, line %d holds %q, which may be two cells wide: count cells, not runes", name, i+1, c)
				}
			}
		}
	}
}

// commandPaths is the root, as an empty path, and every command below
// parent, read from each one's help document.
func commandPaths(t *testing.T, repo string, env map[string]string, parent []string) [][]string {
	t.Helper()
	r := run(t, repo, env, append(append([]string{}, parent...), "--help", "--json")...)
	var doc struct {
		Commands []struct {
			Name string `json:"name"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("%q --help --json is not one document: %v\n%s%s", parent, err, r.stdout, r.stderr)
	}
	out := [][]string{parent}
	for _, c := range doc.Commands {
		out = append(out, commandPaths(t, repo, env, append(append([]string{}, parent...), c.Name))...)
	}
	return out
}
