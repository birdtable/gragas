package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// identityAnswers is the part of rulebook/identity-answers.yaml that migrate
// takes as flags. The rest of the file (the spelling variant, the governance
// keys) has no flag.
type identityAnswers struct {
	name, version, language, rules, presets, docsURL string
	owners                                           []string
}

// readAnswers reads the file with no YAML library, which the journeys may not
// import: the file is flat, a key at the left, a nested key under licences, a
// list under owners. A line it does not know fails the test.
func readAnswers(t *testing.T, b []byte) identityAnswers {
	t.Helper()
	var a identityAnswers
	section := ""
	for line := range strings.SplitSeq(string(b), "\n") {
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, val, isKey := strings.Cut(strings.TrimSpace(line), ":")
		val = strings.TrimSpace(val)
		switch {
		case !strings.HasPrefix(line, " "):
			section = key
			switch key {
			case "short_name":
				a.name = val
			case "version":
				a.version = val
			case "language":
				a.language = val
			case "docs_url":
				a.docsURL = val
			case "spelling", "licences", "owners", "governance":
			default:
				t.Fatalf("identity-answers.yaml: an unknown key %q", key)
			}
		case section == "licences" && isKey && key == "rules":
			a.rules = val
		case section == "licences" && isKey && key == "presets":
			a.presets = val
		case section == "owners" && strings.HasPrefix(strings.TrimSpace(line), "- "):
			a.owners = append(a.owners, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")))
		case section == "governance":
		default:
			t.Fatalf("identity-answers.yaml: a line this reader does not know: %q", line)
		}
	}
	return a
}

func (a identityAnswers) flags() []string {
	f := []string{"--name", a.name, "--version", a.version, "--language", a.language,
		"--licence", a.rules, "--presets-licence", a.presets}
	for _, o := range a.owners {
		f = append(f, "--owner", o)
	}
	if a.docsURL != "" {
		f = append(f, "--docs-url", a.docsURL)
	}
	return f
}

// TestTheOwnersIdentityAnswersMigrateACopyWithNoIdentityFinding: the owner's
// recorded answers, given to migrate as flags over a copy of the rulebook, are
// planned and written, the identity file holds them, and validate over the
// result names no finding in the identity file. The base URL is the owner's, so
// the file holds none and the result holds none. Risk: an answer the format
// refuses, found at the first release and not now.
func TestTheOwnersIdentityAnswersMigrateACopyWithNoIdentityFinding(t *testing.T) {
	t.Parallel()
	live := oldBook() // the owner's format 2 rulebook, frozen: the live one is format 3 now
	b, err := os.ReadFile(filepath.Join(live, "identity-answers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a := readAnswers(t, b)
	for k, v := range map[string]string{"name": a.name, "version": a.version, "language": a.language, "rules licence": a.rules, "presets licence": a.presets} {
		if v == "" {
			t.Fatalf("identity-answers.yaml holds no %s", k)
		}
	}
	if len(a.owners) == 0 {
		t.Fatal("identity-answers.yaml holds no owner")
	}
	if a.docsURL != "" {
		t.Errorf("the base URL is the owner's to give: the file holds %q", a.docsURL)
	}
	before := checksum(t, live)
	dir := newFolder(t)
	copyTree(t, live, dir)
	copied := checksum(t, dir)

	if plan := migrateRun(t, dir, "80", a.flags()...); plan.code != 6 {
		t.Fatalf("dry run: exit %d, want 6\n%s%s", plan.code, plan.stdout, plan.stderr)
	}
	if checksum(t, dir) != copied {
		t.Fatal("the dry run changed the copy")
	}
	if r := migrateRun(t, dir, "80", append(a.flags(), "--apply")...); r.code != 0 {
		t.Fatalf("migrate --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	id, err := os.ReadFile(filepath.Join(dir, "rulebook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{a.name, a.version, a.rules, a.presets}, a.owners...)
	for _, w := range want {
		if !strings.Contains(string(id), w) {
			t.Errorf("rulebook.yaml lacks %q\n%s", w, id)
		}
	}
	if strings.Contains(string(id), "docs_url") {
		t.Errorf("rulebook.yaml holds a base URL nobody gave\n%s", id)
	}
	// validate over the whole copy holds every migrated rule from the prose lint
	//, so it is held to a clean exit and a held line that counts
	// rules: it loaded the book.
	v := validateRun(t, dir, "200")
	if held := heldLine.FindStringSubmatch(v.stdout); v.code != 0 || held == nil || held[1] == "0" {
		t.Fatalf("validate did not load the migrated copy: exit %d, want 0 with a held line\n%s%s", v.code, v.stdout, v.stderr)
	}
	if strings.Contains(v.stdout+v.stderr, "rulebook.yaml") {
		t.Errorf("validate names a finding in the identity file\n%s%s", v.stdout, v.stderr)
	}
	if checksum(t, live) != before {
		t.Error("the live rulebook changed")
	}

	// The plant: a version the schema refuses, in the migrated copy, is a finding
	// in the identity file, so the check above is seen to check something.
	bad := strings.Replace(string(id), "version: "+a.version, "version: 1.0", 1)
	if bad == string(id) {
		t.Fatalf("the plant found no version line to change\n%s", id)
	}
	if err := os.WriteFile(filepath.Join(dir, "rulebook.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := validateRun(t, dir, "200"); v.code == 0 || !strings.Contains(v.stdout+v.stderr, "rulebook.yaml") {
		t.Errorf("a refused version did not fail validate in the identity file: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
}
