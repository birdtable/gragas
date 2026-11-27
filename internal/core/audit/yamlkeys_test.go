package audit_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/core/audit"
)

func releaseSources() []audit.Source {
	return []audit.Source{
		{Tool: "goreleaser", Files: []string{".goreleaser.yaml", ".goreleaser.yml"}, Kind: audit.KindYAMLKeys},
		{Tool: "changie", Files: []string{".changie.yaml", ".changie.yml"}, Kind: audit.KindYAMLKeys},
	}
}

func releaseBindings() []audit.Binding {
	return []audit.Binding{
		{"supply.licences.6", "goreleaser", "archives"},
		{"supply.release.5", "goreleaser", "changelog.disable"},
		{"supply.release.5", "changie", "unreleasedDir"},
	}
}

func checkRelease(t *testing.T, repo fstest.MapFS) audit.Result {
	t.Helper()
	return audit.Check(t.Context(), audit.Input{Sources: releaseSources(), Repo: repo, Bindings: releaseBindings()})
}

const cleanGoreleaser = "version: 2\narchives:\n  - formats: [tar.gz]\n    files:\n      - LICENSE\n      - src: NOTICE\n"

func TestACleanReleaseConfigAndARepoWithNoneListNoFinding(t *testing.T) {
	t.Parallel()
	clean := fstest.MapFS{
		".goreleaser.yaml":                  {Data: []byte(cleanGoreleaser)},
		".changie.yaml":                     {Data: []byte("changesDir: .changes\n")},
		".changes/unreleased/12-added.yaml": {Data: []byte("kind: Added\n")},
		".changes/unreleased/.gitkeep":      {},
	}
	for name, repo := range map[string]fstest.MapFS{"clean": clean, "no files": {}} {
		if got := checkRelease(t, repo); len(got.Findings)+len(got.Exempt)+len(got.NotChecked) != 0 {
			t.Errorf("%s: reports %+v, want nothing", name, got)
		}
	}
}

func TestAnArchiveWithoutNoticeIsOneFinding(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{".goreleaser.yaml": {Data: []byte("archives:\n  - formats: [tar.gz]\n    files: [LICENSE]\n")}}
	got := checkRelease(t, repo)
	want := ".goreleaser.yaml: supply.licences.6 (goreleaser archives): archive 1 of .goreleaser.yaml does not list NOTICE in files"
	if len(got.Findings) != 1 || got.Findings[0].String() != want {
		t.Fatalf("findings = %v, want %q", got.Findings, want)
	}
}

func TestAConfigWithNoArchivesIsAFindingAndABinaryArchiveIsNot(t *testing.T) {
	t.Parallel()
	none := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte("version: 2\n")}})
	if len(none.Findings) != 1 || !strings.Contains(none.Findings[0].Missing, "has no archives") {
		t.Errorf("no archives: findings = %v", none.Findings)
	}
	bin := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte("archives:\n  - formats: [binary]\n")}})
	if len(bin.Findings) != 0 {
		t.Errorf("a binary archive: findings = %v", bin.Findings)
	}
}

func TestChangelogDisabledIsOneFindingAndATemplateIsLeftToTheTool(t *testing.T) {
	t.Parallel()
	off := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte(cleanGoreleaser + "changelog:\n  disable: true\n")}})
	want := ".goreleaser.yaml: supply.release.5 (goreleaser changelog.disable): .goreleaser.yaml sets changelog.disable: the entry a person wrote would be dropped"
	if len(off.Findings) != 1 || off.Findings[0].String() != want {
		t.Fatalf("findings = %v, want %q", off.Findings, want)
	}
	tmpl := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte(cleanGoreleaser + "changelog:\n  disable: \"{{ .Env.OFF }}\"\n")}})
	if len(tmpl.Findings) != 0 {
		t.Errorf("a template: findings = %v", tmpl.Findings)
	}
	for _, v := range []string{"true", "True", "TRUE"} {
		quoted := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte(cleanGoreleaser + "changelog:\n  disable: \"" + v + "\"\n")}})
		if len(quoted.Findings) != 1 || quoted.Findings[0].String() != want {
			t.Errorf("a quoted %s: findings = %v, want %q", v, quoted.Findings, want)
		}
	}
}

// The presets are the reader's clean control: a repository that copies one
// has no finding, and a change file named the way the changie preset's
// fragmentFileFormat names it meets supply.release.5.
func TestThePresetsAreCleanToTheirOwnRules(t *testing.T) {
	t.Parallel()
	gr, err := os.ReadFile("../../../presets/goreleaser/base.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := os.ReadFile("../../../presets/changie/base.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// <issue>-<kind>, as "{{.Custom.Issue}}-{{.Kind}}" renders with kind keys.
	repo := fstest.MapFS{
		".goreleaser.yaml":                      {Data: gr},
		".changie.yaml":                         {Data: ch},
		".changes/unreleased/931-added.yaml":    {},
		".changes/unreleased/931-fixed.yaml":    {},
		".changes/unreleased/1037-changed.yaml": {},
	}
	if got := checkRelease(t, repo); len(got.Findings)+len(got.Exempt)+len(got.NotChecked) != 0 {
		t.Errorf("the presets report %+v, want nothing", got)
	}
	for _, want := range []string{"versionExt:", "fragmentFileFormat:"} {
		if !strings.Contains(string(ch), want) {
			t.Errorf("the changie preset does not set %s", want)
		}
	}
}

func TestAMisnamedChangeFileIsOneFinding(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{
		".changie.yaml":                           {Data: []byte("changesDir: .changes\n")},
		".changes/unreleased/12-added.yaml":       {},
		".changes/unreleased/Added-20260101.yaml": {},
	}
	got := checkRelease(t, repo)
	want := ".changie.yaml: supply.release.5 (changie unreleasedDir): the change file .changes/unreleased/Added-20260101.yaml is not named <issue>-<kind>"
	if len(got.Findings) != 1 || got.Findings[0].String() != want {
		t.Fatalf("findings = %v, want %q", got.Findings, want)
	}
}

func TestAChangeFolderOutsideTheRepositoryIsNeverRead(t *testing.T) {
	t.Parallel()
	for _, cfg := range []string{"changesDir: ..\nunreleasedDir: x\n", "changesDir: /work/scratch\n", "changesDir: a/../../b\n", "changesDir: " + table(t, "backslash_paths.txt")[0] + "\n"} {
		repo := fstest.MapFS{
			".changie.yaml":          {Data: []byte(cfg)},
			"x/bad.txt":              {},
			"etc/unreleased/bad.txt": {},
			"a/b/unreleased/bad.txt": {},
			table(t, "backslash_paths.txt")[0] + "/unreleased/bad.txt": {},
		}
		if got := checkRelease(t, repo); len(got.Findings) != 0 {
			t.Errorf("%q: findings = %v, want none", cfg, got.Findings)
		}
	}
}

func TestAReleaseConfigThatIsNotAMappingIsNotChecked(t *testing.T) {
	t.Parallel()
	got := checkRelease(t, fstest.MapFS{".goreleaser.yaml": {Data: []byte("- a\n- b\n")}})
	if len(got.Findings) != 0 || len(got.NotChecked) != 1 || got.NotChecked[0].Tool != "goreleaser" {
		t.Fatalf("result = %+v, want goreleaser not checked", got)
	}
}

func TestAKeyTheYAMLReaderDoesNotKnowIsAFinding(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{".goreleaser.yaml": {Data: []byte(cleanGoreleaser)}}
	got := audit.Check(t.Context(), audit.Input{Sources: releaseSources(), Repo: repo, Bindings: []audit.Binding{{"x.y.1", "goreleaser", "snapshot"}}})
	if len(got.Findings) != 1 || !strings.Contains(got.Findings[0].Missing, "does not read the key snapshot") {
		t.Fatalf("findings = %v", got.Findings)
	}
}
