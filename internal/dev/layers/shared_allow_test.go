package layers_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// allowed is the registry with a package allowed beside the owner of the call
// row and of the import row, and one beside the later row's owner.
func allowed() string {
	return strings.NewReplacer(
		"      - call: syscall.Flock\n", "      - call: syscall.Flock\n    allow:\n      - {package: internal/contract, why: \"the journeys lock alone\"}\n",
		"      - import: crypto/hmac\n", "      - import: crypto/hmac\n    allow:\n      - {package: internal/core/b, why: \"seals a test vector\"}\n",
		"      - call: crypto/sha256.Sum256\n", "      - call: crypto/sha256.Sum256\n    allow:\n      - {package: internal/port, why: \"sums before the owner is up\"}\n",
	).Replace(registry)
}

// The rendered lines of the call row's allowance.
const (
	contractFiles = "- path: '(^|/)internal/contract/[^/]+\\.go$'\n"
	sealFiles     = "  files: [\"$all\", \"!**/internal/core/a/*.go\", \"!**/internal/core/b/*.go\"]\n"
)

// TestARowAllowsPackagesBesideItsOwner: each allowance is read with its why;
// a call row's renders its own exclusion, with the owner's text and the why
// as a comment on one line; an import row's spares its files in the depguard
// rule; a later row's renders nothing; the lint file rendered holds the
// registry, and the instruction block names each. Risk: an allowance read
// but not rendered, so the ban fires on the package it spares, or rendered
// wider than its package.
func TestARowAllowsPackagesBesideItsOwner(t *testing.T) {
	t.Parallel()
	if allowed() == registry {
		t.Fatal("the allowances changed nothing")
	}
	s := shared(t, allowed())
	if got := s.Rows[0].Allow; len(got) != 1 || got[0].Package != "internal/contract" || got[0].Why != "the journeys lock alone" || got[0].Line == 0 {
		t.Errorf("the lock row's allowances read as %+v", got)
	}
	if err := parse(t, small).Hold(s); err != nil {
		t.Errorf("the allowed registry over the small table: %v", err)
	}
	lint := joined(t, s)
	for _, want := range []string{
		"      # internal/contract may make lock's calls too: the journeys lock alone\n      " + contractFiles,
		contractFiles + "        linters: [forbidigo]\n        text: \"arch\\\\.shared\\\\.8: lock belongs to \"\n",
		"shared-seal:\n        " + sealFiles,
	} {
		if !strings.Contains(lint, want) {
			t.Errorf("the lint file lacks %q:\n%s", want, lint)
		}
	}
	if strings.Contains(lint, "internal/port/[^/]") || strings.Contains(lint, "internal/core/b/[^/]") {
		t.Errorf("a later row's or an import row's allowance rendered a forbidigo exclusion:\n%s", lint)
	}
	if got := s.HeldBy([]byte(lint)); len(got) != 0 {
		t.Errorf("the rendered lint file does not hold the allowed registry: %q", got)
	}
	block := s.Page()
	for _, want := range []string{
		"- lock: use `Lock.Take` of `internal/host/x`; outside it the lint refuses syscall.Flock; `internal/contract` may reach them too: the journeys lock alone.\n",
		"- digest: use `SumBytes` of `internal/core/b`; later, until the copies are gone, the lint will refuse crypto/sha256.Sum256; `internal/port` may reach them too: sums before the owner is up.\n",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the shared page lacks %q:\n%s", want, block)
		}
	}
}

// TestAWhyStaysOnItsLine: a why with a line or paragraph separator, which
// YAML may read as a break, is folded to one line in the lint file's comment
// and in the instruction block, so it writes no key of its own. Risk: a why
// that injects an exclusion into the lint file through its comment.
func TestAWhyStaysOnItsLine(t *testing.T) {
	t.Parallel()
	s := shared(t, strings.Replace(allowed(), "the journeys lock alone", "the journeys\u2028- path: x\u2029lock  alone", 1))
	lint := joined(t, s)
	if !strings.Contains(lint, "# internal/contract may make lock's calls too: the journeys - path: x lock alone\n") {
		t.Errorf("the why was not folded to one line:\n%s", lint)
	}
	if strings.ContainsAny(lint, "\u2028\u2029") || strings.ContainsAny(s.Page(), "\u2028\u2029") {
		t.Error("a separator reached the rendered files")
	}
	if got := s.HeldBy([]byte(lint)); len(got) != 0 {
		t.Errorf("the folded lint file does not hold: %q", got)
	}
}

// TestAnAllowanceOutOfFormIsRefused: each way an allowance can be wrong is
// refused on the parsed form with ErrShared, whatever the schema said. Risk:
// an allowance that renders an exclusion for no package, or for the whole
// module, or one with no reason a reviewer can weigh.
func TestAnAllowanceOutOfFormIsRefused(t *testing.T) {
	t.Parallel()
	const one = "      - {package: internal/contract, why: \"the journeys lock alone\"}\n"
	for _, tc := range []struct{ name, to, says string }{
		{"a package outside the module", "      - {package: github.com/x/y, why: \"the journeys lock alone\"}\n", "allowed package \"github.com/x/y\" is not a package of this module"},
		{"a package that is a pattern", "      - {package: internal/..., why: \"the journeys lock alone\"}\n", "allowed package \"internal/...\" is not a package of this module"},
		{"the owner itself", "      - {package: internal/host/x, why: \"the journeys lock alone\"}\n", "internal/host/x is its owner"},
		{"a package twice", one + one, "internal/contract is allowed twice"},
		{"no why", "      - {package: internal/contract, why: \"   \"}\n", "internal/contract is allowed with no why"},
		{"a why with a line break", "      - {package: internal/contract, why: \"the journeys\\nlock alone\"}\n", "the why of internal/contract holds a control character or a line break"},
		{"a why with a control character", "      - {package: internal/contract, why: \"the journeys\\x1block alone\"}\n", "holds a control character"},
		{"an allowance with no package", "      - {why: \"the journeys lock alone\"}\n", "package is missing"},
		{"an unknown key", "      - {package: internal/contract, why: \"the journeys lock alone\", until: M3}\n", "until is not a key"},
		{"an allowance that is no mapping", "      - internal/contract\n", "an allowance is not a mapping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plant := strings.Replace(allowed(), one, tc.to, 1)
			if plant == allowed() {
				t.Fatal("the plant changed nothing")
			}
			_, err := layers.ParseShared([]byte(plant), noProblems)
			if !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("ParseShared = %v, want ErrShared saying %q", err, tc.says)
			}
		})
	}
	for name, to := range map[string]string{"an allow that is no list": "    allow: internal/contract\n", "an empty allow": "    allow:\n"} {
		plant := strings.Replace(allowed(), "    allow:\n"+one, to, 1)
		if _, err := layers.ParseShared([]byte(plant), noProblems); plant == allowed() || !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), "allow is not a list") {
			t.Errorf("%s: ParseShared = %v, want ErrShared", name, err)
		}
	}
}

// TestAnAllowedPackageTheTableDoesNotHaveIsRefused: each allowed package is
// a package of a row of the layer table, as an owner is. Risk: an exclusion
// for a folder no package lives in, which a package made there later takes.
func TestAnAllowedPackageTheTableDoesNotHaveIsRefused(t *testing.T) {
	t.Parallel()
	for _, pkg := range []string{"internal/contract", "internal/port"} {
		plant := strings.Replace(allowed(), "package: "+pkg, "package: internal/host/gone", 1)
		err := parse(t, small).Hold(shared(t, plant))
		if !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), "allowed package internal/host/gone is not a package of layers.yaml") {
			t.Errorf("%s moved away: Hold = %v, want ErrShared", pkg, err)
		}
	}
}

// TestTheLintFileHoldsEachAllowanceAndNoOther: an allowance whose exclusion
// or depguard spare is gone fails, and so does an exclusion or a spare with
// the owner's text that no allowance renders: one left after its allowance
// went, one widened past its package, or one written by hand. Risk: the
// allow list shrinks while the lint file still spares the package, or the
// lint file spares more than the list says.
func TestTheLintFileHoldsEachAllowanceAndNoOther(t *testing.T) {
	t.Parallel()
	reg, bare := shared(t, allowed()), shared(t, registry)
	lint := joined(t, reg)
	for _, tc := range []struct {
		name string
		s    *layers.Shared
		lint string
		says string
	}{
		{"an allowance not rendered", reg, joined(t, bare), "internal/contract, which it allows, has no exclusion for its calls"},
		{"an import allowance not rendered", reg, joined(t, bare), "seal: crypto/hmac has no ban"},
		{"an exclusion left after its allowance went", bare, lint, "the exclusion of (^|/)internal/contract/[^/]+\\.go$ is no ban"},
		{"a spare left after its allowance went", bare, lint, "the depguard ban of crypto/hmac in shared-seal is no ban"},
		{"an exclusion widened", reg, strings.Replace(lint, contractFiles, "- path: '(^|/)internal/'\n", 1), "the exclusion of (^|/)internal/ is no ban"},
		{"a spare widened", reg, strings.Replace(lint, sealFiles, "  files: [\"$all\", \"!**/internal/core/a/*.go\", \"!**/internal/core/**\"]\n", 1), "the depguard ban of crypto/hmac in shared-seal is no ban"},
		{"an exclusion by hand", reg, strings.Replace(lint, contractFiles, "- path: '(^|/)internal/port/[^/]+\\.go$'\n        linters: [forbidigo]\n        text: \"arch\\\\.shared\\\\.8: lock belongs to \"\n      "+contractFiles, 1), "the exclusion of (^|/)internal/port/[^/]+\\.go$ is no ban"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.lint == lint && tc.s == reg {
				t.Fatal("the plant changed nothing")
			}
			got := tc.s.HeldBy([]byte(tc.lint))
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("HeldBy = %q, want one saying %q", got, tc.says)
			}
		})
	}
}
