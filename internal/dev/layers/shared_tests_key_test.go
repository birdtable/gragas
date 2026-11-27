package layers_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// sparing is the registry with the lock row's test files spared.
func sparing() string {
	return strings.Replace(registry, "      - call: syscall.Flock\n",
		"      - call: syscall.Flock\n    tests:\n      spared: \"the tests lock until the move\"\n      until: \"the tests take the module lock\"\n", 1)
}

// TestARowSparesTestFilesUntilAnIssue: a row's tests key renders one exclusion
// for every _test.go file with the owner's text, the lint file held to it
// refuses one without it, and a ban in a test file is refused without the key
// (the lint file rendered from the registry with no key holds no such
// exclusion). Risk: a reprieve read and not rendered, or rendered for a row
// that did not ask.
func TestARowSparesTestFilesUntilAnIssue(t *testing.T) {
	t.Parallel()
	if sparing() == registry {
		t.Fatal("the reprieve changed nothing")
	}
	s := shared(t, sparing())
	if c := s.Rows[0].Tests; c == nil || c.Until != "the tests take the module lock" || c.Why == "" {
		t.Fatalf("the lock row's reprieve reads as %+v", c)
	}
	lint := joined(t, s)
	want := "- path: '" + layers.TestFiles + "'\n        linters: [forbidigo]\n        text: \"arch\\\\.shared\\\\.8: lock belongs to \"\n"
	if !strings.Contains(lint, want) {
		t.Errorf("the lint file lacks the test exclusion %q:\n%s", want, lint)
	}
	if got := s.HeldBy([]byte(lint)); len(got) != 0 {
		t.Errorf("the rendered lint file does not hold the registry: %q", got)
	}
	plain := shared(t, registry)
	bare := joined(t, plain)
	if strings.Contains(bare, layers.TestFiles) {
		t.Errorf("a row with no reprieve rendered a test exclusion:\n%s", bare)
	}
	if got := s.HeldBy([]byte(bare)); len(got) == 0 {
		t.Error("a lint file with no test exclusion was held to a registry that spares test files")
	}
	if got := plain.HeldBy([]byte(lint)); len(got) == 0 {
		t.Error("a test exclusion no row spares was let through")
	}
	if !strings.Contains(s.Page(), "test files are spared until the tests take the module lock: the tests lock until the move") {
		t.Errorf("the shared page does not name the reprieve:\n%s", s.Page())
	}
}

// TestAReprieveOutOfFormIsRefused: a reprieve with no why, with no end, or
// with an end that names an issue by its number is refused. Risk: a stop-gap
// with no end, or one that points at a number the public tree cannot read.
func TestAReprieveOutOfFormIsRefused(t *testing.T) {
	t.Parallel()
	until := "\"the tests take the module lock\""
	for name, bad := range map[string]string{
		"no why":              strings.Replace(sparing(), "the tests lock until the move", "", 1),
		"no end":              strings.Replace(sparing(), until, "\"\"", 1),
		"an issue number":     strings.Replace(sparing(), until, "\"#1751\"", 1),
		"words with an issue": strings.Replace(sparing(), until, "\"go-files#374 lands\"", 1),
	} {
		if _, err := layers.ParseShared([]byte(bad), noProblems); err == nil {
			t.Errorf("%s: read with no error", name)
		}
	}
}
