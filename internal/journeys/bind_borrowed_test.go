package journeys

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// bindingFiles are the files of the overlay's bindings/, each with its text.
func bindingFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "bindings"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		out[e.Name()] = read(t, dir, filepath.Join("bindings", e.Name()))
	}
	return out
}

// TestABorrowedRuleIsTightenedAndNeverWeakened is the done-when of the borrowed-rule
// bind, on the binary, over fixture-base, an overlay of the parent stand-in, with an
// imported rule of its own. Without the parent in the cache a bind of its
// rule is not ready, naming the pin. With it, each way of weakening a
// borrowed binding ends refused, exit 5, naming the field, and writes
// nothing; a changed value of a tool no installed plugin serves is not ready,
// exit 4, and writes nothing; a bind that tightens one plans a record under the overlay's
// prefix with the standing before and after, writes it with --apply, is
// nothing to do again and is the overlay's entry in resolve. A rule outside
// the overlay's own parts is never written in place, and validate finds a
// record written by hand that weakens a borrowed binding, with SARIF.
// Risk: an overlay or an importer that loosens what it was lent.
func TestABorrowedRuleIsTightenedAndNeverWeakened(t *testing.T) {
	t.Parallel()
	dir, env := overlayRepo(t), ruleEnv(t)
	env["COLUMNS"] = "80"
	src := []string{"--tool", "golangci-lint", "--target", "source"}
	tighten := with([]string{"bind", "std.errors.4"}, append(src, "--key", "errcheck", "--value", "true", "--severity", "error", "--rung", "1", "--run-point", "commit")...)
	was, lines := bindingFiles(t, dir), hist(t, dir)
	unchanged := func(what string) {
		t.Helper()
		if got := bindingFiles(t, dir); !maps.Equal(got, was) || hist(t, dir) != lines {
			t.Fatalf("%s wrote: records %v, history %d lines, want %v and %d", what, got, hist(t, dir), was, lines)
		}
	}

	r := run(t, dir, env, append(tighten, "--apply")...)
	if r.code != 4 || !strings.Contains(flat(r.stdout), "not ready parent not fetched: fixture-parent 0.9.0 is not in the releases cache") || lastEnding(t, env) != "not ready" {
		t.Fatalf("not fetched: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	unchanged("a bind with no parent")
	fetchParent(t, env)

	for _, tc := range []struct {
		name, says string
		args       []string
	}{
		{"weaken", "std.errors.4 is the parent fixture-parent 0.9.0's rule, and the change is refused at its severity: the severity would go from error to warning",
			with([]string{"rebind", "std.errors.4"}, append(src, "--key", "errorlint", "--severity", "warning")...)},
		{"weaken a tightening", "std.errors.1 is the parent fixture-parent 0.9.0's rule, and the change is refused at its severity: the severity would go from error to warning",
			with([]string{"rebind", "std.errors.1"}, append(src, "--key", "wrapcheck", "--severity", "warning")...)},
		{"move", "refused at its entry: the entry would move from golangci-lint source errorlint to golangci-lint source errcheck",
			with([]string{"rebind", "std.errors.4"}, append(src, "--key", "errorlint", "--to-key", "errcheck")...)},
		{"rung", "refused at its rung: the rung would go from 1 to 2",
			with([]string{"bind", "std.errors.4"}, append(src, "--key", "errorlint", "--value", "true", "--severity", "error", "--rung", "2", "--run-point", "commit")...)},
		{"remedy", "refused at its remedy",
			with([]string{"rebind", "std.errors.4"}, append(src, "--key", "errorlint", "--remedy", "fix")...)},
		{"unbind", "unbinding it at golangci-lint source errorlint would weaken it",
			with([]string{"unbind", "std.errors.4"}, append(src, "--key", "errorlint")...)},
		{"rebind an import", "base.errors.1 is an import's rule, and the change is refused at its severity",
			[]string{"rebind", "base.errors.1", "--tool", "golangci-lint", "--target", "config", "--key", "errorlint", "--severity", "warning"}},
		{"edit a parent's rule", "std.errors is no concern of this overlay's own parts, and an overlay never writes its parent's rule in place",
			[]string{"rule", "edit", "std.errors.4", "--rule-owner", "bob"}},
		{"a new rule in a parent's part", "std.errors is no concern of this overlay's own parts",
			[]string{"rule", "new", "--concern", "std.errors", "--statement", "Wrap it.", "--strength", "must", "--own", "--decision-title", "Wrap", "--decision-text", "We wrap."}},
	} {
		r := run(t, dir, env, append(tc.args, "--apply")...)
		if r.code != 5 || !strings.Contains(flat(r.stdout), tc.says) {
			t.Errorf("%s: exit %d, want 5 and %q\n%s%s", tc.name, r.code, tc.says, r.stdout, r.stderr)
		}
		if got := lastEnding(t, env); got != "refused" {
			t.Errorf("%s: the run log ends %q", tc.name, got)
		}
		unchanged(tc.name)
		if tc.name == "weaken" {
			uitest.Golden(t, "bind-borrowed-weaken-80", transcript(tc.args, r))
		}
	}

	unrankedValue(t, dir, env, src)
	unchanged("a value no plugin ranks")

	r = run(t, dir, env, tighten...)
	if r.code != 6 || !strings.Contains(r.stdout, "Standing before and after") || !strings.Contains(r.stdout, "bindings/std-errors-4.yaml: writes a record of std.errors.4") {
		t.Fatalf("tighten, the plan: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	uitest.Golden(t, "bind-borrowed-tighten-80", transcript(tighten, r))
	unchanged("the plan")
	if r = run(t, dir, env, append(tighten, "--apply")...); r.code != 0 || lastEnding(t, env) != "done" {
		t.Fatalf("tighten --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := "subject: std.errors.4\ntool: golangci-lint\ntarget: source\nkey: errcheck\nvalue: true\nseverity: error\nrung: 1\nrun_point: commit\n"
	if got := read(t, dir, "bindings/std-errors-4.yaml"); got != want || hist(t, dir) != lines+1 {
		t.Fatalf("the record is\n%s\nhistory %d lines, want\n%s\nand %d", got, hist(t, dir), want, lines+1)
	}
	if r = run(t, dir, env, tighten...); r.code != 0 || !strings.Contains(r.stdout, "nothing to do") {
		t.Fatalf("tighten again: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r = run(t, dir, env, "resolve"); r.code != 0 || !slices.ContainsFunc(strings.Split(r.stdout, "\n"), func(l string) bool {
		return strings.Join(strings.Fields(l), " ") == "golangci-lint source: errcheck std.errors.4 fixture-base"
	}) {
		t.Fatalf("resolve: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}

	// a record written by hand that weakens the imported rule
	weak := "subject: base.errors.1\ntool: golangci-lint\ntarget: config\nkey: errorlint\nvalue: true\nseverity: warning\nrung: 0\nrun_point: edit\n"
	if err := os.WriteFile(filepath.Join(dir, "bindings", "base-errors-1.yaml"), []byte(weak), 0o600); err != nil {
		t.Fatal(err)
	}
	r = run(t, dir, env, "validate")
	if r.code != 1 || !strings.Contains(flat(r.stdout), "the record of base.errors.1 weakens the binding an import lends at golangci-lint config errorlint, at its severity") ||
		!strings.Contains(flat(r.stdout), `remove the record "bindings/base-errors-1.yaml" from the repository with git rm`) || strings.Contains(r.stdout, "this rulebook's own") {
		t.Fatalf("validate: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	uitest.Golden(t, "validate-borrowed-weakened-80", transcript([]string{"validate"}, r))
	r = run(t, dir, env, "validate", "--sarif")
	if r.code != 1 || !strings.Contains(r.stdout, `"text": "the record of base.errors.1 weakens`) || !strings.Contains(r.stdout, `"startLine": 6`) {
		t.Fatalf("validate --sarif: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	uitest.Golden(t, "validate-borrowed-weakened-sarif", r.stdout)
}

// unrankedValue rebinds std.errors.4's errorlint value from true to false in
// dir. A changed value is ranked by the plugin that serves the tool, and none
// is installed here: the rebind ends not ready, naming the tool and how to
// add its plugin; the caller holds that it wrote
// nothing. A value the plugin cannot rank is refused: link's
// TestTightenRefusesEachWayOfWeakeningAndNamesTheField.
func unrankedValue(t *testing.T, dir string, env map[string]string, src []string) {
	t.Helper()
	value := with([]string{"rebind", "std.errors.4"}, append(src, "--key", "errorlint", "--value", "false")...)
	r := run(t, dir, env, append(value, "--apply")...)
	out := flat(r.stdout)
	if r.code != 4 || !strings.Contains(out, "no plugin is installed for golangci-lint, so a value of it cannot be ranked") ||
		!strings.Contains(out, "gragas plugin add <folder>") || lastEnding(t, env) != "not ready" {
		t.Errorf("a value no plugin ranks: exit %d, want 4 naming golangci-lint and gragas plugin add\n%s%s", r.code, r.stdout, r.stderr)
	}
}
