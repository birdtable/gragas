package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/proc"
	testenv "github.com/birdtable/go-testkit"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// private is the mode of a 700 folder, spelt in hex so no octal literal is in the file.
const private = 0x1c0

// TestTheCommandMakesItsFolderUnderTheBaseTheCallerGivesAndRefusesTheThreeWays: the
// namespace-caps task hands the command a base as TMPDIR and uses the folder it prints, with no mkdir of its own,
// so the refusals the walk makes are the task's. Planted, each with the real walk and not a stand-in: a base a
// group can write, a link in the way of the account's folder, and a base inside the work tree;
// and a good base that prints a folder under it, at mode 0700.
func TestTheCommandMakesItsFolderUnderTheBaseTheCallerGivesAndRefusesTheThreeWays(t *testing.T) {
	// not parallel: it points the temp folder at each base through the environment (one variable is set
	// here first so the test is read as one that sets it, then every one go-proc names, under each base).
	t.Setenv(split(proc.HomeEnv(t.TempDir())[0]))
	// baseUnder points every home and temp variable go-proc names at a folder of its own under root,
	// so no system's variable is named here, and makes and answers the base the walk then reads,
	// os.TempDir.
	baseUnder := func(root string) string {
		for _, kv := range proc.HomeEnv(root) {
			t.Setenv(split(kv))
		}
		base := os.TempDir()
		if err := os.MkdirAll(base, private); err != nil {
			t.Fatal(err)
		}
		return base
	}
	try := func(apart ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(&out, &errb, func() (string, error) { return hosttrust.StableTemp(apart) })
		return code, out.String(), errb.String()
	}
	work := t.TempDir()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	good := baseUnder(root)
	code, out, errs := try(work)
	folder := strings.TrimSpace(out)
	if code != 0 || errs != "" || !strings.HasPrefix(folder, good) {
		t.Fatalf("a good base: exit %d, stdout %q, stderr %q: want a folder under %s", code, out, errs, good)
	}
	// The folder keeps no access for a group or others: its mode on Unix, its access list on Windows.
	trusttest.Perm(t, folder, private)

	// A link where the account's folder goes.
	linkBase := baseUnder(t.TempDir())
	elsewhere := t.TempDir()
	// A link is made in its one place, go-testkit (tests.admission.5).
	testenv.Link(t, elsewhere, filepath.Join(linkBase, filepath.Base(filepath.Dir(folder))))
	if code, out, errs := try(work); code != 1 || out != "" || !strings.Contains(errs, "not trusted") {
		t.Errorf("a link in the way: exit %d, stdout %q, stderr %q: want 1, none, the walk's refusal", code, out, errs)
	}

	// A base others can write: the mode bit on Unix, an allow entry for Everyone on Windows.
	open := baseUnder(t.TempDir())
	trusttest.Loosen(t, open, trusttest.OthersWrite)
	if code, out, errs := try(work); code != 1 || out != "" || !strings.Contains(errs, "not trusted") {
		t.Errorf("a base others can write: exit %d, stdout %q, stderr %q: want 1, none, the walk's refusal", code, out, errs)
	}

	// A base inside the work tree.
	baseUnder(filepath.Join(work, "cache"))
	if code, out, errs := try(work); code != 1 || out != "" || !strings.Contains(errs, "not trusted") {
		t.Errorf("a base inside the work tree: exit %d, stdout %q, stderr %q: want 1, none, the walk's refusal", code, out, errs)
	}
}

// split is NAME=value as its name and its value, for t.Setenv.
func split(kv string) (string, string) {
	k, v, _ := strings.Cut(kv, "=")
	return k, v
}
