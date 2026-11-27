package tool_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// TestAVersionRunsFixIsALineForTheDialectOrTheStepInWords: the
// fix of a version run that exits non-zero is the run's line as shell.Quote
// writes it for the run's dialect, to paste; with no dialect, or an argv Quote
// refuses, it is the step in words, each word in Go's quoting, so a control
// rune of the argv never reaches the terminal raw and no line is offered that
// a shell would read as other words.
// Planted: runFix saying every fix in words (the dialect row red); runFix
// returning Quote's line whatever its error (the refused row red, an empty
// line where the step should be); runFix joining the words bare, as the old
// fix did (the refused row red, the escape raw).
func TestAVersionRunsFixIsALineForTheDialectOrTheStepInWords(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		d    shell.Dialect
		path string
		argv []string
		want string
	}{
		"a dialect and an argv it quotes": {shell.POSIX, "/a b/it's", []string{"--version"},
			`run '/a b/it'\''s' --version and fix what it says`},
		"no dialect": {0, "/tools/lint-a", []string{"--version"},
			`run the program at "/tools/lint-a" with the words "--version" and fix what it says`},
		"a dialect and an argv it refuses": {shell.POSIX, "/tools/lint-a", []string{"--ver\x1bsion"},
			`run the program at "/tools/lint-a" with the words "--ver\x1bsion" and fix what it says`},
		"no dialect and no words": {0, "/tools/lint-a", nil,
			`run the program at "/tools/lint-a" with no words and fix what it says`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tool.RunFix(c.d, c.path, c.argv); got != c.want {
				t.Errorf("RunFix(%d, %q, %q) = %q; want %q", c.d, c.path, c.argv, got, c.want)
			}
		})
	}
}

// TestBindsVersionRefusalCarriesTheFixOfTheConfigsDialect: a
// tool whose version ask exits 1 is refused with the fix Config.Dialect
// writes: the line, quoted for POSIX, or with the zero Dialect the step in
// words. Planted: versionBy passing the zero Dialect in place of t.cfg.Dialect
// (the POSIX row red).
func TestBindsVersionRefusalCarriesTheFixOfTheConfigsDialect(t *testing.T) {
	t.Parallel()
	for name, d := range map[string]shell.Dialect{"posix": shell.POSIX, "none": 0} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := private(t)
			prog := program(t, private(t)+"/lint-a", printing("lint-a 1.2.3\n", 1))
			tl := tool.New(tool.Config{Paths: map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, Path: pathFolder(), Lang: "C", Dialect: d})
			_, err := tl.Bind(t.Context(), stepScope(t, root, false))
			r, ok := errors.AsType[*tool.Refused](err)
			if !ok || !errors.Is(err, port.ErrRefused) {
				t.Fatalf("Bind = %v; want refused", err)
			}
			want := "run the program at " + strconv.Quote(prog) + " with the words "
			if d != 0 {
				line, qerr := shell.Quote(d, []string{prog})
				if qerr != nil {
					t.Fatalf("shell.Quote(%q) = %v", prog, qerr)
				}
				want = "run " + line + " "
			}
			if !strings.HasPrefix(r.Hint(), want) || !strings.HasSuffix(r.Hint(), " and fix what it says") {
				t.Errorf("the fix = %q; want it to start %q", r.Hint(), want)
			}
		})
	}
}
