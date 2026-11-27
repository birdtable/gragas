package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheLicencesListIsTheCommaJoinedIdsAndRedOnNone holds gragas#1228's replacement of the
// grep and paste of licences-system: comments and blanks are skipped, the ids are joined with
// commas, and a file with no id, or no file, is red.
func TestTheLicencesListIsTheCommaJoinedIdsAndRedOnNone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var out, errs bytes.Buffer
	if code := runLicences(t.Context(), []string{"list", write("ok.txt", "# a comment\nMIT\n\nBSD-3-Clause\n  Apache-2.0  \n")}, &out, &errs); code != exitDone || out.String() != "MIT,BSD-3-Clause,Apache-2.0\n" {
		t.Errorf("a list: exit %d, %q, %q; want the three ids joined", code, out.String(), errs.String())
	}
	out.Reset()
	errs.Reset()
	if code := runLicences(t.Context(), []string{"list", write("none.txt", "# only a comment\n\n")}, &out, &errs); code != exitFailed || out.String() != "" || !strings.Contains(errs.String(), "names no licence") {
		t.Errorf("no id: exit %d, %q, %q; want red", code, out.String(), errs.String())
	}
	if code := runLicences(t.Context(), []string{"list", filepath.Join(dir, "gone.txt")}, &out, &errs); code != exitFailed {
		t.Errorf("no file: exit %d, want red", code)
	}
}

// TestTheLicencesIgnoreListsTheModulesOfTheSourcesAndRefusesAWildcard holds gragas#1265: the
// member passes over only the modules a row names, comma-joined for go-licenses --ignore; a row
// that is not six fields, or names a wildcard, is red.
func TestTheLicencesIgnoreListsTheModulesOfTheSourcesAndRefusesAWildcard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	list := write("l.txt", "MIT\nApache-2.0\n")
	const row = "example.org/a/x v1.0.0 abc Apache-2.0 deadbeef x#1"
	var out, errs bytes.Buffer
	if code := runLicences(t.Context(), []string{"ignore", write("s.txt", "# c\n"+row+"\nexample.org/a/y v1.0.0 abc Apache-2.0 deadbeef x#1\n"), list}, &out, &errs); code != exitDone || out.String() != "example.org/a/x,example.org/a/y\n" {
		t.Errorf("two rows: exit %d, %q, %q", code, out.String(), errs.String())
	}
	for name, body := range map[string]string{"wildcard": "example.org/a/* v1.0.0 abc Apache-2.0 deadbeef x#1\n", "short": "example.org/a/x v1.0.0\n"} {
		out.Reset()
		if code := runLicences(t.Context(), []string{"ignore", write("s.txt", body), list}, &out, &errs); code != exitFailed || out.String() != "" {
			t.Errorf("%s: exit %d, %q; want red and nothing on stdout", name, code, out.String())
		}
	}
	out.Reset()
	if code := runLicences(t.Context(), []string{"ignore", write("s.txt", row+"\n")}, &out, &errs); code != exitUsage || out.String() != "" {
		t.Errorf("ignore with no allowlist: exit %d, %q; want usage and nothing on stdout", code, out.String())
	}
}

// TestTheLicencesIgnorePassesOverAModuleTheListTakesOneSideOf holds gragas#1455: a
// "<module> takes <id>" row of the allowlist puts its module on go-licenses --ignore, after the
// sources' modules, only when <id> is a plain id of the same list. A row taking an id the list
// does not allow, a row of another shape, or a wildcard is red. With no row in either file the
// value is empty, never a lone comma, whose empty entry would be a prefix of every path.
func TestTheLicencesIgnorePassesOverAModuleTheListTakesOneSideOf(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	sources := write("s.txt", "example.org/a/x v1.0.0 abc MIT deadbeef x#1\n")
	empty := write("e.txt", "# no row\n")
	var out, errs bytes.Buffer
	for _, r := range []struct {
		claim, sources, list, want string
	}{
		{"a source and a takes row", sources, "MIT\nBSD-3-Clause\nexample.org/psx takes BSD-3-Clause\n", "example.org/a/x,example.org/psx\n"},
		{"a takes row alone", empty, "BSD-3-Clause\n# c\nexample.org/psx takes BSD-3-Clause\n", "example.org/psx\n"},
		{"the other forms are not takes rows", empty, "MIT\npublic-domain example.org/sql\nexample.org/c adds ICU\n", "\n"},
		{"no row anywhere", empty, "MIT\n", "\n"},
	} {
		out.Reset()
		errs.Reset()
		if code := runLicences(t.Context(), []string{"ignore", r.sources, write("l.txt", r.list)}, &out, &errs); code != exitDone || out.String() != r.want {
			t.Errorf("%s: exit %d, %q (%q); want %q", r.claim, code, out.String(), errs.String(), r.want)
		}
	}
	for claim, list := range map[string]string{
		"the side taken is not on the list": "MIT\nexample.org/psx takes GPL-2.0-only\n",
		"a wildcard module":                 "BSD-3-Clause\nexample.org/* takes BSD-3-Clause\n",
		"a takes row of four words":         "BSD-3-Clause\nexample.org/psx takes BSD-3-Clause now\n",
		"a takes row with no id":            "BSD-3-Clause\nexample.org/psx takes\n",
	} {
		out.Reset()
		errs.Reset()
		if code := runLicences(t.Context(), []string{"ignore", empty, write("l.txt", list)}, &out, &errs); code != exitFailed || out.String() != "" || !strings.Contains(errs.String(), "line ") {
			t.Errorf("%s: exit %d, %q, %q; want red naming the line, nothing on stdout", claim, code, out.String(), errs.String())
		}
	}
}
