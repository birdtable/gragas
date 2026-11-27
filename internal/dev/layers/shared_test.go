package layers_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// registry is a shared registry over the small table: a call row, an import
// row with a shared-module export, and a later row.
const registry = `shared:
  - capability: lock
    owner: internal/host/x
    export: Lock.Take
    primitives:
      - call: syscall.Flock
    status: local
    since: 0.1.0
  - capability: seal
    owner: internal/core/a
    export: Seal
    shared: {package: github.com/birdtable/go-run/runlog, export: Digest}
    primitives:
      - import: crypto/hmac
    status: shared
    since: 0.1.0
  - capability: digest
    owner: internal/core/b
    export: SumBytes
    primitives:
      - call: crypto/sha256.Sum256
    status: later
    until: "the copies are gone"
    since: 0.1.0
`

// noProblems is a schema check that finds none, for the tests of what the
// parse holds on its own.
func noProblems(any) []string { return nil }

func shared(t *testing.T, s string) *layers.Shared {
	t.Helper()
	sh, err := layers.ParseShared([]byte(s), noProblems)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

// TestTheSharedRegistryReadsItsRows: each field of a row is read, and a later
// row is not banned. Risk: a field dropped on the way to the bans.
func TestTheSharedRegistryReadsItsRows(t *testing.T) {
	t.Parallel()
	s := shared(t, registry)
	if len(s.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(s.Rows))
	}
	seal := s.Rows[1]
	if seal.Name != "seal" || seal.Owner != "internal/core/a" || seal.Export != "Seal" || seal.Module != "github.com/birdtable/go-run/runlog" ||
		seal.ModuleExport != "Digest" || seal.Status != layers.StatusShared || seal.Since != "0.1.0" || len(seal.Primitives) != 1 || seal.Primitives[0].Import != "crypto/hmac" {
		t.Errorf("the seal row read as %+v", seal)
	}
	if got := s.Rows[0].Primitives[0]; got.Path() != "syscall" || got.String() != "syscall.Flock" {
		t.Errorf("the call read as path %q, %q", got.Path(), got.String())
	}
	if !s.Rows[0].Banned() || s.Rows[2].Banned() {
		t.Error("a local row is not banned, or a later one is")
	}
}

// TestASharedRegistryOutOfFormIsRefused: each way a row can be wrong is
// refused on the parsed form with ErrShared, whatever the schema said; with
// no schema check nothing is read, and a schema problem is refused. Risk: a
// row that renders no ban, or a ban the owner cannot be found by.
func TestASharedRegistryOutOfFormIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, from, to, says string }{
		{"a capability twice", "capability: seal", "capability: lock", "lock is there twice"},
		{"a capability out of form", "capability: seal", "capability: Seal", "not lower-case words"},
		{"an owner outside the module", "owner: internal/core/a", "owner: github.com/x/y", "not a package of this module"},
		{"an export not exported", "export: Seal", "export: seal", "not an exported name"},
		{"a status out of the list", "status: local", "status: owned", "not one of local, shared, later"},
		{"a since that is no version", "since: 0.1.0\n  - capability: seal", "since: v1\n  - capability: seal", "is not a version"},
		{"a later row that waits on nothing", "    until: \"the copies are gone\"\n", "", "digest is later and names nothing that turns it on"},
		{"a later row that waits on an issue number", "until: \"the copies are gone\"", "until: \"#9\"", "until \"#9\" is not plain words with no issue number"},
		{"a later row whose words name an issue", "until: \"the copies are gone\"", "until: \"x/y#9 lands\"", "until \"x/y#9 lands\" is not plain words with no issue number"},
		{"a later row whose until is a line break", "until: \"the copies are gone\"", "until: \"the copies\\nare gone\"", "is not plain words with no issue number"},
		{"an until on a row that is not later", "    status: local\n    since: 0.1.0\n  - capability: seal", "    status: local\n    until: \"the copies are gone\"\n    since: 0.1.0\n  - capability: seal", "until names what turns a later row on, and this row is local"},
		{"a primitive that is both", "      - import: crypto/hmac", "      - {import: crypto/hmac, call: crypto/hmac.New}", "one call or one import"},
		{"a primitive that is neither", "      - import: crypto/hmac", "      - {}", "one call or one import"},
		{"a primitive that is no mapping", "      - import: crypto/hmac", "      - crypto/hmac", "not a mapping"},
		{"a call with no name", "call: syscall.Flock", "call: syscall", "not an import path, a dot and an exported name"},
		{"a call of an unexported name", "call: syscall.Flock", "call: syscall.flock", "not an import path, a dot and an exported name"},
		{"an import out of form", "import: crypto/hmac", "import: Crypto/hmac", "not an import path"},
		{"a primitive owned twice", "call: crypto/sha256.Sum256", "call: syscall.Flock", "owned by lock already"},
		{"a shared-module package outside the modules", "package: github.com/birdtable/go-run/runlog", "package: github.com/birdtable/go-runx/runlog", "is not a package of"},
		{"a package of a module gragas does not import", "package: github.com/birdtable/go-run/runlog", "package: github.com/birdtable/go-proc/runlog", "is not a package of"},
		{"a shared-module export not exported", "export: Digest", "export: digest", "the shared module's export"},
		{"a shared row with no package", "    shared: {package: github.com/birdtable/go-run/runlog, export: Digest}\n", "", "names no shared-module package"},
		{"an unknown key", "since: 0.1.0\n  - capability: seal", "since: 0.1.0\n    note: x\n  - capability: seal", "note is not a key"},
		{"an anchor", "export: Seal", "export: &a Seal", "anchor"},
		{"a key twice", "export: Seal", "export: Seal\n    export: Seal", "written again"},
		{"no rows", registry, "shared: []\n", "holds no row"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plant := strings.Replace(registry, tc.from, tc.to, 1)
			if plant == registry {
				t.Fatal("the plant changed nothing")
			}
			_, err := layers.ParseShared([]byte(plant), noProblems)
			if !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("ParseShared = %v, want ErrShared saying %q", err, tc.says)
			}
		})
	}
	if _, err := layers.ParseShared([]byte(registry), nil); !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), "no schema check") {
		t.Errorf("with no schema check: %v, want ErrShared", err)
	}
	if _, err := layers.ParseShared([]byte(registry), func(any) []string { return []string{"line 3: planted"} }); !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), "planted") {
		t.Errorf("with a schema problem: %v, want ErrShared naming it", err)
	}
}

// TestAnOwnerTheTableDoesNotHaveIsRefused: each owner is a package of a row
// of the layer table, later rows too. Risk: a ban that spares a folder no
// package lives in, so the owner it names is nowhere.
func TestAnOwnerTheTableDoesNotHaveIsRefused(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	if err := tb.Hold(shared(t, registry)); err != nil {
		t.Fatalf("the registry over the small table: %v", err)
	}
	for _, owner := range []string{"internal/host/x", "internal/core/b"} {
		plant := strings.Replace(registry, "owner: "+owner, "owner: internal/host/gone", 1)
		err := tb.Hold(shared(t, plant))
		if !errors.Is(err, layers.ErrShared) || !strings.Contains(err.Error(), "owner internal/host/gone is not a package of layers.yaml") {
			t.Errorf("owner %s moved away: Hold = %v, want ErrShared", owner, err)
		}
	}
}

// lintBase is a base in the lint file's own shape, with every slot the
// table and the registry fill.
const lintBase = `linters:
  settings:
    depguard:
      rules:
        # slot layer-rows begin
        # slot layer-rows end
        # slot shared-imports begin
        # slot shared-imports end
    forbidigo:
      forbid:
        - pattern: ^panic$
          msg: "go.errors.3: no"
        # slot shared-calls begin
        # slot shared-calls end
  exclusions:
    rules:
      # slot exclusions-generated begin
      # slot exclusions-generated end
`

// joined is the lint file the small table and the registry make.
func joined(t *testing.T, s *layers.Shared) string {
	t.Helper()
	tb := parse(t, small)
	tb.Shared = s
	got, err := tb.Join([]byte(lintBase), []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// TestTheBansRenderForTheRowsThatAreNotLater: a call is a forbidigo row with
// its package and the owner's exclusion, an import is a depguard rule that
// spares the owner's files, and a later row renders nothing. Risk: a ban
// that never fires, or one that fires on its owner.
func TestTheBansRenderForTheRowsThatAreNotLater(t *testing.T) {
	t.Parallel()
	s := joined(t, shared(t, registry))
	for _, want := range []string{
		"- pattern: '^syscall\\.Flock$'\n",
		"  pkg: '^syscall$'\n",
		"  msg: \"arch.shared.8: lock belongs to internal/host/x; use its Lock.Take\"\n",
		"shared-seal:\n",
		"  files: [\"$all\", \"!**/internal/core/a/*.go\"]\n",
		"    - pkg: crypto/hmac\n",
		"- path: '(^|/)internal/host/x/[^/]+\\.go$'\n",
		"  text: \"arch\\\\.shared\\\\.8: lock belongs to \"\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the lint file lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "sha256") || strings.Contains(s, "digest") {
		t.Errorf("the later row rendered a ban:\n%s", s)
	}
	if strings.Count(s, "internal/core/a/[^/]") != 0 {
		t.Errorf("an import-only row rendered a forbidigo exclusion:\n%s", s)
	}
	if empty := joined(t, nil); strings.Contains(empty, "arch.shared.8:") || !strings.Contains(empty, layers.CallsBeginMark) {
		t.Errorf("no registry rendered a ban, or lost its markers:\n%s", empty)
	}
}

// lockMsg is the last line of the lock row's rendered ban.
const lockMsg = "          msg: \"arch.shared.8: lock belongs to internal/host/x; use its Lock.Take\"\n"

// TestTheLintFileHoldsEachBanAndNoOther: the rendered file holds the
// registry; a banned primitive whose ban or owner's exclusion is gone fails,
// and so does a ban under the rule that no banned row renders, written in
// any block, a later row's included. A layer's ban whose why cites the rule
// further in is the layer's, and no finding. Risk: a ban lost, or one widened
// or added by hand, that the registry does not show; or a layer that cites
// the rule refused as a ban.
func TestTheLintFileHoldsEachBanAndNoOther(t *testing.T) {
	t.Parallel()
	reg := shared(t, registry)
	lint := joined(t, reg)
	if got := reg.HeldBy([]byte(lint)); len(got) != 0 {
		t.Fatalf("the rendered lint file does not hold: %q", got)
	}
	for _, tc := range []struct{ name, from, to, says string }{
		{"a call's ban gone", "- pattern: '^syscall\\.Flock$'\n", "- pattern: '^syscall\\.Flick$'\n", "lock: syscall.Flock has no ban"},
		{"an import's ban gone", "    - pkg: crypto/hmac\n", "    - pkg: crypto/hmax\n", "seal: crypto/hmac has no ban"},
		{"an import's files widened", "shared-seal:\n          files: [\"$all\", \"!**/internal/core/a/*.go\"]", "shared-seal:\n          files: [\"$all\", \"!**/internal/core/**\"]", "the depguard ban of crypto/hmac in shared-seal is no ban"},
		{"the owner's exclusion widened", "'(^|/)internal/host/x/[^/]+\\.go$'", "'(^|/)internal/host/'", "the exclusion of (^|/)internal/host/ is no ban"},
		{"the owner's exclusion gone", "\"arch\\\\.shared\\\\.8: lock belongs to \"", "\"arch\\\\.shared\\\\.9: lock belongs to \"", "its owner internal/host/x has no exclusion"},
		{"a ban with no row", lockMsg, lockMsg + "        - pattern: '^os\\.Lstat$'\n          pkg: '^os$'\n          msg: \"arch.shared.8: link-walk belongs to internal/host/x; use its Walk\"\n", "the forbidigo ban ^os\\.Lstat$ is no ban"},
		{"a later row's ban by hand", lockMsg, lockMsg + "        - pattern: '^sha256\\.Sum256$'\n          pkg: '^crypto/sha256$'\n          msg: \"arch.shared.8: digest belongs to internal/core/b; use its SumBytes\"\n", "the forbidigo ban ^sha256\\.Sum256$ is no ban"},
		{"an import ban with no row", "        shared-seal:\n", "        shared-hand:\n          files: [\"$all\"]\n          deny:\n            - pkg: unsafe\n              desc: \"arch.shared.8: by hand\"\n        shared-seal:\n", "the depguard ban of unsafe in shared-hand is no ban"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plant := strings.Replace(lint, tc.from, tc.to, 1)
			if plant == lint {
				t.Fatal("the plant changed nothing")
			}
			got := reg.HeldBy([]byte(plant))
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("HeldBy = %q, want one saying %q", got, tc.says)
			}
		})
	}
	cites := strings.Replace(lint, "        shared-seal:\n", "        layer-walk:\n          files: [\"**/internal/host/walk/**\"]\n          deny:\n            - pkg: github.com/birdtable/gragas\n              desc: \"trust-walk, arch.shared.8: one owner for the trust check\"\n        shared-seal:\n", 1)
	if cites == lint {
		t.Fatal("the layer's plant changed nothing")
	}
	if got := reg.HeldBy([]byte(cites)); len(got) != 0 {
		t.Errorf("a layer whose why cites the rule further in: %q, want no finding", got)
	}
	if got := reg.HeldBy([]byte("- a\n")); len(got) != 1 {
		t.Errorf("a lint file that is no mapping: %q", got)
	}
}

// TestAnExclusionThatCouldVoidABanIsRefused: an exclusion for forbidigo,
// depguard or every linter whose text is not confined to another rule's id
// and colon (the words of a ban's message or its call, a call as one file
// spells it, an alternation with such a branch, no text, a wildcard or case
// folding before the colon, the rule's id or its tail, a text that does not
// compile), or a path excluded from every linter, is refused by the files it
// names unless a banned row renders it; one for another linter, or confined
// to another rule's id, is not. Risk: a hand exclusion in lint/project.yml,
// which is not protected, that keeps the lint from reporting a ban while the
// registry still holds (review-sec-2's X1, X2 and Y3 on PR 631).
func TestAnExclusionThatCouldVoidABanIsRefused(t *testing.T) {
	t.Parallel()
	reg := shared(t, registry)
	lint := joined(t, reg)
	end := "  exclusions:\n    rules:\n"
	if !strings.Contains(lint, end) {
		t.Fatal("the rendered lint file has no exclusion rules")
	}
	for _, tc := range []struct{ name, rule, says string }{
		{"X1: the rest of a call ban's message", "      - path: ^cmd/gragas-dev/\n        linters: [forbidigo]\n        text: \"lock belongs to\"\n", "the exclusion of ^cmd/gragas-dev/ is no ban"},
		{"X2: an import ban's list", "      - path: ^internal/dev/measure/\n        linters: [depguard]\n        text: \"seal\"\n", "the exclusion of ^internal/dev/measure/ is no ban"},
		{"the call named", "      - path: ^a/\n        linters: [forbidigo]\n        text: \"syscall\\\\.Flock\"\n", "the exclusion of ^a/ is no ban"},
		{"no text", "      - path: ^b/\n        linters: [depguard]\n", "the exclusion of ^b/ is no ban"},
		{"every linter", "      - path: ^c/\n        text: \"Lock\\\\.Take\"\n", "the exclusion of ^c/ is no ban"},
		{"a text that does not compile", "      - path: ^d/\n        linters: [forbidigo]\n        text: \"(\"\n", "the exclusion of ^d/ is no ban"},
		{"every file but one", "      - path-except: ^e/\n        linters: [forbidigo]\n", "the exclusion of every file but ^e/ is no ban"},
		{"Y3: a call as one file spells it", "      - path: ^cmd/gragas-dev/\n        linters: [forbidigo]\n        text: \"`sc\\\\.Flock`\"\n", "the exclusion of ^cmd/gragas-dev/ is no ban"},
		{"an alternation with another rule", "      - path: ^i/\n        linters: [forbidigo]\n        text: \"go\\\\.errors\\\\.[34]:|sc\\\\.Flock\"\n", "the exclusion of ^i/ is no ban"},
		{"words of no rule", "      - path: ^j/\n        linters: [forbidigo]\n        text: \"zzz\"\n", "the exclusion of ^j/ is no ban"},
		{"a wildcard before the colon", "      - path: ^k/\n        linters: [forbidigo]\n        text: \"go.errors.3:\"\n", "the exclusion of ^k/ is no ban"},
		{"a repeat before the colon", "      - path: ^l/\n        linters: [depguard]\n        text: \"[a-z]+:\"\n", "the exclusion of ^l/ is no ban"},
		{"case folding", "      - path: ^m/\n        linters: [forbidigo]\n        text: \"(?i)go\\\\.errors\\\\.3:\"\n", "the exclusion of ^m/ is no ban"},
		{"the rule's id spelled by a class", "      - path: ^n/\n        linters: [forbidigo]\n        text: \"arch\\\\.shared\\\\.[8]:\"\n", "the exclusion of ^n/ is no ban"},
		{"the tail of the rule's id", "      - path: ^o/\n        linters: [forbidigo]\n        text: \"h\\\\.shared\\\\.8:\"\n", "the exclusion of ^o/ is no ban"},
		{"an optional group", "      - path: ^q/\n        linters: [forbidigo]\n        text: \"a(rch)?\\\\.shared\\\\.8:\"\n", "the exclusion of ^q/ is no ban"},
		{"a colon of a list name", "      - path: ^p/\n        linters: [depguard]\n        text: \"seal':\"\n", "the exclusion of ^p/ is no ban"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reg.HeldBy([]byte(strings.Replace(lint, end, end+tc.rule, 1)))
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("HeldBy = %q, want one saying %q", got, tc.says)
			}
		})
	}
	paths := strings.Replace(lint, "  exclusions:\n    rules:\n", "  exclusions:\n    paths:\n      - ^f/\n    rules:\n", 1)
	if paths == lint {
		t.Fatal("the paths plant changed nothing")
	}
	if got := reg.HeldBy([]byte(paths)); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "the excluded path ^f/ is no ban") }) {
		t.Errorf("a path excluded from every linter: HeldBy = %q, want it refused", got)
	}
	for _, rule := range []string{
		"      - path: ^g/\n        linters: [gosec]\n        text: \"lock belongs to\"\n",
		"      - path: ^h/\n        linters: [forbidigo]\n        text: \"go\\\\.errors\\\\.3:\"\n",
		"      - path: ^h/\n        linters: [forbidigo]\n        text: \"go\\\\.errors\\\\.[34]:\"\n",
		"      - path: ^h/\n        linters: [forbidigo]\n        text: \"cli\\\\.(config\\\\.3|errors\\\\.3):\"\n",
		"      - path: ^h/\n        text: \"tests\\\\.(admission\\\\.5|isolation\\\\.6):\"\n",
		"      - path: ^h/\n        linters: [depguard]\n        text: \"arch\\\\.ports\\\\.1: only the Clock adapter reads the clock\"\n",
	} {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			if got := reg.HeldBy([]byte(strings.Replace(lint, end, end+rule, 1))); len(got) != 0 {
				t.Errorf("%q: HeldBy = %q, want no finding", rule, got)
			}
		})
	}
}

// TestTheExportsOfASourceAreItsTopLevelNamesAndMethods: functions, types,
// variables, constants and methods on value, pointer and generic receivers
// are read; unexported ones and methods of unexported types are not. Missing
// names a banned row's owner export and every row's shared-module export,
// and spares a later row's owner. Risk: a ban that points at an export that
// is not there.
func TestTheExportsOfASourceAreItsTopLevelNamesAndMethods(t *testing.T) {
	t.Parallel()
	got, err := layers.Exported(map[string][]byte{
		"a.go": []byte("package a\n\ntype Lock struct{}\n\nfunc (l *Lock) Take() {}\n\nfunc (Lock) Free() {}\n\ntype List[T any] struct{}\n\nfunc (l *List[T]) Add() {}\n\ntype hidden struct{}\n\nfunc (hidden) Out() {}\n\nfunc Seal() {}\n\nfunc seal() {}\n"),
		"b.go": []byte("package a\n\nvar Max, min = 1, 2\n\nconst Prefix = \"p\"\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"List", "List.Add", "Lock", "Lock.Free", "Lock.Take", "Max", "Prefix", "Seal"}
	if !slices.Equal(got, want) {
		t.Errorf("Exported = %q, want %q", got, want)
	}
	if _, err := layers.Exported(map[string][]byte{"bad.go": []byte("package")}); err == nil {
		t.Error("a source that does not parse gave no error")
	}
	reg := shared(t, registry)
	if m := reg.Missing("internal/host/x", want); len(m) != 0 {
		t.Errorf("the owner's export is there: Missing = %q", m)
	}
	if m := reg.Missing("internal/host/x", []string{"Lock"}); len(m) != 1 || !strings.Contains(m[0], "has no export Lock.Take") {
		t.Errorf("the owner lacks Lock.Take: Missing = %q", m)
	}
	if m := reg.Missing("github.com/birdtable/go-run/runlog", []string{"Log"}); len(m) != 1 || !strings.Contains(m[0], "at the pinned tag has no export Digest") {
		t.Errorf("the shared module lacks Digest: Missing = %q", m)
	}
	if m := reg.Missing("internal/core/b", nil); len(m) != 0 {
		t.Errorf("a later row's owner export was asked for: %q", m)
	}
}

// TestTheInstructionBlockIsWrittenBetweenItsMarkers: the block between the
// markers is the registry's and the rest of the file is kept; a file with no
// block, or a marker twice, is refused. Risk: an instruction file rewritten
// past its block.
func TestTheInstructionBlockIsWrittenBetweenItsMarkers(t *testing.T) {
	t.Parallel()
	reg := shared(t, registry)
	file := "# Title\n\nkept\n" + layers.AgentsBeginMark + "\nplanted-old-block\n" + layers.AgentsEndMark + "\nafter\n"
	got, err := reg.Agents([]byte(file))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{"# Title\n\nkept\n", "after\n", "`docs/shared.md`"} {
		if !strings.Contains(s, want) {
			t.Errorf("the file lacks %q:\n%s", want, s)
		}
	}
	for _, row := range []string{"- lock:", "- seal:", "- digest:"} {
		if strings.Contains(s, row) {
			t.Errorf("a row %q is in AGENTS.md; the rows are in docs/shared.md:\n%s", row, s)
		}
	}
	page := reg.Page()
	for _, want := range []string{"- lock: use `Lock.Take` of `internal/host/x`; outside it the lint refuses syscall.Flock.\n",
		"- seal: use `Seal` of `internal/core/a` (runlog.Digest of go-run); outside it the lint refuses crypto/hmac.\n",
		"- digest: use `SumBytes` of `internal/core/b`; later, until the copies are gone, the lint will refuse crypto/sha256.Sum256.\n",
		"type: reference\n", "\nfor: "} {
		if !strings.Contains(page, want) {
			t.Errorf("the shared page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(s, "planted-old-block") {
		t.Errorf("the old block is left:\n%s", s)
	}
	if again, err := reg.Agents(got); err != nil || string(again) != s {
		t.Errorf("a second write differs: %v", err)
	}
	for _, bad := range []string{"no block\n", layers.AgentsEndMark + "\n" + layers.AgentsBeginMark + "\n", file + layers.AgentsBeginMark + "\n"} {
		if _, err := reg.Agents([]byte(bad)); !errors.Is(err, layers.ErrNoBlock) {
			t.Errorf("Agents(%q) = %v, want ErrNoBlock", bad, err)
		}
	}
}

// TestAnOwnerBelowAPatternIsAPackageOfItsRow: an owner a row holds by a pattern
// (internal/host/fs below internal/host/...) is a package of that row, as one
// the row lists is; one below no pattern and in no row is still refused.
// Planted failure: Hold asking the listed packages alone.
func TestAnOwnerBelowAPatternIsAPackageOfItsRow(t *testing.T) {
	t.Parallel()
	tb := parse(t, strings.Replace(small, "packages: [internal/host/x]", "patterns: [internal/host/...]", 1)).WithTree([]string{"internal/host/x"})
	if err := tb.Hold(shared(t, registry)); err != nil {
		t.Errorf("an owner below a pattern: %v", err)
	}
	for name, owner := range map[string]string{"below no pattern": "internal/elsewhere/x", "below a pattern and in no folder": "internal/host/gone"} {
		plant := strings.Replace(registry, "owner: internal/host/x", "owner: "+owner, 1)
		if err := tb.Hold(shared(t, plant)); !errors.Is(err, layers.ErrShared) {
			t.Errorf("an owner %s: Hold = %v, want ErrShared", name, err)
		}
	}
}

// TestTheInstructionBlockIsTheSameSizeForAnyNumberOfRows: AGENTS.md's block of
// the shared code does not grow with a row, since the rows are in
// docs/shared.md (agents.instructions.1 caps the file). Plant:
// three rows more leave the block as it was and add three lines to the page.
// Risk: a row that costs another change its line of AGENTS.md.
func TestTheInstructionBlockIsTheSameSizeForAnyNumberOfRows(t *testing.T) {
	t.Parallel()
	lines := func(s string) int { return strings.Count(s, "\n") }
	few := shared(t, registry)
	more := shared(t, registry+`  - capability: extra-one
    owner: internal/core/x1
    export: One
    primitives:
      - call: os.Getpid
    status: local
    since: 0.1.0
  - capability: extra-two
    owner: internal/core/x2
    export: Two
    primitives:
      - call: os.Getppid
    status: local
    since: 0.1.0
  - capability: extra-three
    owner: internal/core/x3
    export: Three
    primitives:
      - call: os.Getuid
    status: local
    since: 0.1.0
`)
	if len(more.Rows) != len(few.Rows)+3 {
		t.Fatalf("the plant has %d rows, want %d", len(more.Rows), len(few.Rows)+3)
	}
	if few.Instructions() != more.Instructions() {
		t.Errorf("the block depends on the rows:\n%s\nagainst\n%s", few.Instructions(), more.Instructions())
	}
	if got, want := lines(more.Page()), lines(few.Page())+3; got != want {
		t.Errorf("the page has %d lines for three rows more, want %d", got, want)
	}
}
