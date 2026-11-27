package audit_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// TestEachFormThatCannotFailIsFound: the members go-task 3.54.0 ends green, each
// with the form that does it. go-task runs a member with errexit on and no
// pipefail, so a failure ends it unless one of these forms swallows it. Each row
// is its own plant: take the form out of the check and the row turns red. The
// rows are held to the pinned go-task itself in internal (TestCannotFailAgrees
// WithTheGoTaskThatRunsTheMember).
func TestEachFormThatCannotFailIsFound(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, text, why string }{
		{"|| true", "go vet ./... || true", "|| true"},
		{"|| :", "go vet ./... || :", "|| :"},
		{"|| echo", "go vet ./... || echo failed", "|| echo"},
		{"|| exit 0", "go vet ./... || exit 0", "|| exit 0"},
		{"a chain ending in || true", "a || b || true", "|| true"},
		{"a pipe to cat", "go vet ./... | cat", "| cat"},
		{"a pipe to tee", "go vet ./... | tee vet.log", "| tee"},
		{"a longer pipeline ending in tee", "go vet ./... | grep x | tee vet.log", "| tee"},
		{"set -n", "set -n; go vet ./...", "set -n"},
		{"set -o noexec", "set -o noexec\ngo vet ./...", "set -n"},
		{"exit 0 alone", "exit 0", "exit 0"},
		{"exit 0 after a statement that cannot fail", "go vet ./... || true; exit 0", "|| true"},
		{"set +e and a last statement that cannot fail", "set +e; go vet ./...; true", "set +e"},
		{"set +o errexit and a last echo", "set +o errexit; go vet ./...; echo done", "set +e"},
		{"every statement swallowed", "go build ./... || true\ngo vet ./... || true", "|| true"},
		{"a later || true after one that cannot fail", "true; false || true", "|| true"},
	} {
		if v := audit.CannotFail(c.text, false); v.Why != c.why || v.Unjudged != "" {
			t.Errorf("%s: %+v, want the form %q", c.name, v, c.why)
		}
	}
}

// TestAMemberThatCanFailIsNotFound: the members go-task ends red, which the old
// reading called unable to fail, and the plain ones. go-task runs a member with
// errexit on, so `false; exit 0` exits 201 and the statement that fails ends it.
func TestAMemberThatCanFailIsNotFound(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, text string
		pipefail   bool
	}{
		{"a plain member", "go vet ./...", false},
		{"a pipeline to a command that can fail", "go vet ./... | grep x", false},
		{"a pipe to tee with pipefail", "go vet ./... | tee vet.log", true},
		{"set -e", "set -e; go vet ./...", false},
		{"|| false", "go vet ./... || false", false},
		{"|| a command that can fail", "go vet ./... || go build ./...", false},
		{"exit 1", "go vet ./...; exit 1", false},
		{"exit with a variable", "go vet ./...; exit $code", false},
		{"exit 0 after a statement that can fail", "go vet ./...; exit 0", false},
		{"|| true then a statement that fails", "false || true; false", false},
		{"the early-exit guard then a statement that fails", `P=x; [ -n "$P" ] || exit 0; false`, false},
		{"set +e and a last statement that fails", "set +e; go vet ./...", false},
		{"the empty-run guard", `grep -q -e EMPTY -e "DONE 0 tests" out.txt && exit 1 || true`, false},
	} {
		if v := audit.CannotFail(c.text, c.pipefail); v != (audit.Verdict{}) {
			t.Errorf("%s: %+v, want nothing found and nothing unjudged", c.name, v)
		}
	}
}

// TestAFormThatCannotBeJudgedIsUnjudgedNeverPassed: text that does not parse,
// and a form that would end a member green inside a condition, a loop, a
// function, a subshell or a group, where whether it runs depends on something
// the check cannot see.
func TestAFormThatCannotBeJudgedIsUnjudgedNeverPassed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, text string }{
		{"text that does not parse", "go vet ( ./..."},
		{"exit 0 in an else", "if go vet ./...; then :; else exit 0; fi"},
		{"|| true in a loop", "for p in a b; do go vet $p || true; done"},
		{"|| true in a subshell", "(go vet ./... || true)"},
		{"|| true in a group", "{ go vet ./... || true; }"},
		{"set +e in a function", "f() { set +e; }; f"},
		{"a combined flag that stops the shell", "set -ne; go vet ./..."},
		{"a positional word before an option", "set x -n; go vet ./..."},
		{"an if with a plain body and nothing else that fails", "if test -f x; then go vet ./...; fi"},
		{"a negated call", "! go vet ./..."},
		{"an if with no else that ends green", "if ! go vet ./...; then echo failed; fi"},
		{"a redirect to a file on the right of ||", "go vet ./... || echo failed > out.log"},
	} {
		if v := audit.CannotFail(c.text, false); v.Why != "" || v.Unjudged == "" {
			t.Errorf("%s: %+v, want it unjudged and nothing found", c.name, v)
		}
	}
}
