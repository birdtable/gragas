package internal

// The lint config's exclusions, held to the paths they are for: a rule the
// config switches off for a path, it switches off for that path alone.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/ordered"
)

// exclusion is one of linters.exclusions.rules.
type exclusion struct {
	path, pathExcept, text string
	linters                []string
}

// exclusions reads the exclusion rules of the lint config.
func exclusions(t *testing.T) []exclusion {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ordered.ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		t.Fatal("the lint config is not a mapping")
	}
	var out []exclusion
	for _, r := range ordered.Maps(doc.Sub("linters").Sub("exclusions").List("rules")) {
		out = append(out, exclusion{path: r.Str("path"), pathExcept: r.Str("path-except"), text: r.Str("text"), linters: ordered.Strings(r.List("linters"))})
	}
	if len(out) == 0 {
		t.Fatal("no exclusion rule read")
	}
	return out
}

// silenced is whether the findings whose text matches msg are switched off
// for the file at path.
func silenced(t *testing.T, rules []exclusion, path, msg string) bool {
	t.Helper()
	for _, r := range rules {
		if r.text == "" || !regexp.MustCompile(r.text).MatchString(msg) {
			continue
		}
		switch {
		case r.path != "" && regexp.MustCompile(r.path).MatchString(path):
			return true
		case r.pathExcept != "" && !regexp.MustCompile(r.pathExcept).MatchString(path):
			return true
		}
	}
	return false
}

// TestTheExclusionsHoldOnlyTheirPaths: the JSON encoder ban reaches the
// command tree and spares only the root's renderer (cli.output.2); the
// environment may be read in the config package's own files and main, and
// not in a package below config (cli.config.3).
func TestTheExclusionsHoldOnlyTheirPaths(t *testing.T) {
	t.Parallel()
	rules := exclusions(t)
	encoder, env := "cli.output.2: JSON output is written by the root command only", "cli.config.3: read the environment in the config package"
	for _, c := range []struct {
		path, msg string
		silenced  bool
	}{
		{"internal/cli/root/commands.go", encoder, false},
		{"internal/cli/other/x.go", encoder, false},
		{"cmd/gragas/main.go", encoder, false},
		{"internal/cli/root/answer.go", encoder, true},
		{"internal/core/api/api.go", encoder, true},
		{"internal/config/config.go", env, true},
		{"internal/config/sub/x.go", env, false},
		{"internal/core/config/x.go", env, false},
		{"cmd/gragas/main.go", env, true},
	} {
		if got := silenced(t, rules, c.path, c.msg); got != c.silenced {
			t.Errorf("%s: %q silenced = %v, want %v", c.path, c.msg, got, c.silenced)
		}
	}
}

// g204 is a finding of gosec's G204, as the lint run words it.
const g204 = "G204: Subprocess launched with variable"

// TestOnlyAProgramsAdapterIsSparedG204 (cli.process.1, arch.ports.6): the
// rules that can switch off gosec's G204 are exactly the generated ones, one
// per adapter layers.yaml names under programs, each at that adapter's
// non-test files. The adapter's tests and every other package stay held.
// Planted failure: a hand-written G204 rule for another package, or a rule
// at the adapter's every file.
func TestOnlyAProgramsAdapterIsSparedG204(t *testing.T) {
	t.Parallel()
	tb := table(t)
	adapters := tb.ProgramAdapters()
	if len(adapters) == 0 {
		t.Fatal("layers.yaml names no program adapter")
	}
	rules := exclusions(t)
	var spared, want []string
	for _, r := range rules {
		if (len(r.linters) == 0 || slices.Contains(r.linters, layers.ProgramLinter)) && (r.text == "" || regexp.MustCompile(r.text).MatchString(g204)) {
			spared = append(spared, "path "+r.path+", path-except "+r.pathExcept)
		}
	}
	for _, a := range adapters {
		want = append(want, "path "+layers.NonTestFiles(a)+", path-except ")
	}
	if !slices.Equal(spared, want) {
		t.Errorf("the rules that spare G204 are\n  %s\nwant one per program adapter\n  %s", strings.Join(spared, "\n  "), strings.Join(want, "\n  "))
	}
	checked := 0
	for _, r := range tb.Rows {
		for _, pkg := range r.Packages {
			adapter := slices.Contains(adapters, pkg)
			for file, want := range map[string]bool{pkg + "/run.go": adapter, pkg + "/run_test.go": false, pkg + "/sub/run.go": false} {
				if got := silenced(t, rules, file, g204); got != want {
					t.Errorf("%s: G204 silenced = %v, want %v", file, got, want)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no package checked")
	}
}

// requiredSettings are the settings of the lint config that decide what a
// finding may be silenced by and that both files hold: which linters run by
// default, gosec's settings (which read no suppression comment),
// nolintlint's (a nolint names its linter and its reason), forbidigo's skip
// of an Example (offSettings), the generated-file header and the exclusion presets (both read as nothing), the
// issue caps and whether tests are linted (cli.process.1,
// gates.exemptions.1, gates.plants.10). A project that copies
// presets/golangci-lint/.golangci.yml gets these, so the preset holds the repository's
// own, which the planted modules of contexts_test.go run.
func requiredSettings() []string {
	return []string{
		"linters.default",
		"linters.settings.gosec",
		"linters.settings.nolintlint",
		"linters.settings.forbidigo.exclude-godoc-examples",
		"linters.exclusions.generated",
		"linters.exclusions.presets",
		"issues",
		"run.tests",
	}
}

// absentSettings are the settings that would silence a finding and that
// neither file holds today: a linter turned off, paths left out of the lint,
// build tags that leave a file out, and the issue filters that report only
// what a revision, a merge base or a patch adds (gragas#1750). The preset
// holds each as the repository does, absent or not, and each is absent or
// set to nothing (TestNoSilencerSettingIsHeld). Last is warn-unused, which
// silences nothing: it is compared here, and gragas#1733 sets it.
func absentSettings() []string {
	return append(silencerSettings(), "linters.exclusions.warn-unused")
}

// silencerSettings are absentSettings but warn-unused: the keys of
// golangci-lint v2.15.0 that leave a finding out with no exclusion rule and
// no nolint to show for it.
func silencerSettings() []string {
	return []string{
		"linters.disable",
		"linters.exclusions.paths",
		"linters.exclusions.paths-except",
		"run.build-tags",
		"issues.new",
		"issues.new-from-rev",
		"issues.new-from-merge-base",
		"issues.new-from-patch",
		"issues.whole-files",
	}
}

// offSettings are the keys of golangci-lint v2.15.0 whose default is true
// and leaves a finding out with no exclusion rule and no nolint to show for
// it, so both files hold each as false: forbidigo's exclude-godoc-examples
// skips every ban inside an Example, and the whole of a test file whose one
// declaration of note is an Example (forbidigo v2.3.1, gragas#1750 C4).
func offSettings() []string {
	return []string{"linters.settings.forbidigo.exclude-godoc-examples"}
}

// offNotHeld is each off setting doc does not hold as false, absent
// included.
func offNotHeld(doc *ordered.Map) []string {
	var out []string
	for _, p := range offSettings() {
		if v, ok := setting(doc, p); !ok || v != "false" {
			out = append(out, p+" is not held as false: its default, true, leaves findings out with no exclusion rule or nolint to show for each; set it to false (gragas#1750)")
		}
	}
	return out
}

// silencersHeld is each silencer setting doc holds with a value: anything
// but false, an empty string, an empty list or null.
func silencersHeld(doc *ordered.Map) []string {
	var out []string
	for _, p := range silencerSettings() {
		v, ok := setting(doc, p)
		if ok && !slices.Contains([]string{"false", `""`, "[]", "null"}, v) {
			out = append(out, p+" is "+v+": it leaves findings out with no exclusion rule or nolint to show for each; delete it (gragas#1750)")
		}
	}
	return out
}

// TestNoSilencerSettingIsHeld: neither .golangci.yml nor the preset holds a
// silencer setting, and both hold each off setting as false (gragas#1719
// C4, gragas#1750). Planted: each silencer key with a value is refused; each
// with nothing, and a document with none, are not; an off setting left out
// or true is refused, and false is not.
func TestNoSilencerSettingIsHeld(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{".golangci.yml", "presets/golangci-lint/.golangci.yml"} {
		doc := lintDoc(t, rel)
		for _, p := range append(silencersHeld(doc), offNotHeld(doc)...) {
			t.Error(rel + ": " + p)
		}
	}
	plant := func(t *testing.T, y string) *ordered.Map {
		t.Helper()
		v, err := ordered.ParseYAML([]byte(y))
		m, ok := v.(*ordered.Map)
		if err != nil || !ok {
			t.Fatalf("a plant does not read: %v\n%s", err, y)
		}
		return m
	}
	for y, says := range map[string]string{
		"linters:\n  disable: [forbidigo]\n":                     "linters.disable is",
		"linters:\n  exclusions:\n    paths: [\"^internal/\"]\n": "linters.exclusions.paths is",
		"linters:\n  exclusions:\n    paths-except: [x]\n":       "linters.exclusions.paths-except is",
		"run:\n  build-tags: [legacy]\n":                         "run.build-tags is",
		"issues:\n  new: true\n":                                 "issues.new is true",
		"issues:\n  new-from-rev: HEAD~1\n":                      "issues.new-from-rev is",
		"issues:\n  new-from-merge-base: main\n":                 "issues.new-from-merge-base is",
		"issues:\n  new-from-patch: a.patch\n":                   "issues.new-from-patch is",
		"issues:\n  whole-files: true\n":                         "issues.whole-files is",
	} {
		if got := silencersHeld(plant(t, y)); len(got) != 1 || !strings.HasPrefix(got[0], says) {
			t.Errorf("%q: got %q, want one saying %q", y, got, says)
		}
	}
	for _, y := range []string{
		"run:\n  tests: true\n",
		"issues:\n  new: false\n  new-from-rev: \"\"\n  whole-files: false\n",
		"linters:\n  disable: []\n  exclusions:\n    warn-unused: true\n",
	} {
		if got := silencersHeld(plant(t, y)); len(got) != 0 {
			t.Errorf("%q: refused: %q", y, got)
		}
	}
	const godoc = "linters.settings.forbidigo.exclude-godoc-examples is not held as false"
	for y, refused := range map[string]bool{
		"linters:\n  settings:\n    forbidigo:\n      analyze-types: true\n":           true,
		"linters:\n  settings:\n    forbidigo:\n      exclude-godoc-examples: true\n":  true,
		"linters:\n  settings:\n    forbidigo:\n      exclude-godoc-examples: \"\"\n":  true,
		"linters:\n  settings:\n    forbidigo:\n      exclude-godoc-examples: false\n": false,
	} {
		got := offNotHeld(plant(t, y))
		if refused && (len(got) != 1 || !strings.HasPrefix(got[0], godoc)) {
			t.Errorf("%q: got %q, want one saying %q", y, got, godoc)
		}
		if !refused && len(got) != 0 {
			t.Errorf("%q: refused: %q", y, got)
		}
	}
}

// lintDoc parses the lint config at rel, a slash path under the root.
func lintDoc(t *testing.T, rel string) *ordered.Map {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ordered.ParseYAML(data)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		t.Fatalf("%s is not a mapping", rel)
	}
	return doc
}

// lookup is the value at the dotted path in doc, and whether it is there.
func lookup(doc *ordered.Map, path string) (any, bool) {
	keys := strings.Split(path, ".")
	for _, k := range keys[:len(keys)-1] {
		if doc = doc.Sub(k); doc == nil {
			return nil, false
		}
	}
	return doc.Get(keys[len(keys)-1])
}

// setting is the value at the dotted path in doc, as JSON, and whether it is
// there.
func setting(doc *ordered.Map, path string) (string, bool) {
	v, ok := lookup(doc, path)
	if !ok {
		return "", false
	}
	return ordered.JSON(v, 0), true
}

// listAt is the list at the dotted path in doc, or nil.
func listAt(doc *ordered.Map, path string) []any {
	v, _ := lookup(doc, path)
	l, _ := v.([]any)
	return l
}

// items is each item of the list at the dotted path in doc, as JSON.
func items(doc *ordered.Map, path string) []string {
	l := listAt(doc, path)
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, ordered.JSON(x, 0))
	}
	return out
}

// projectLinters are the linters a project enables beyond the preset's,
// where it imports database/sql (go.style.4; presets/golangci/README.md).
func projectLinters() []string { return []string{"rowserrcheck", "sqlclosecheck"} }

// enabledLinters is the linters doc's linters.enable list less the five of
// golangci-lint's standard set. The preset's list is generated and names the
// five; the repository's holds them through its linters.default, which
// presetDrift compares as a required setting, so a default that is not
// "standard" is still drift and the five are not compared twice.
func enabledLinters(doc *ordered.Map) []string {
	var on []string
	for _, l := range ordered.Strings(listAt(doc, "linters.enable")) {
		if !slices.Contains([]string{"errcheck", "govet", "ineffassign", "staticcheck", "unused"}, l) {
			on = append(on, l)
		}
	}
	return on
}

// presetDrift is each way base, the preset, silences a finding that repo, the
// repository's config, does not, or the other way round: a required setting
// missing from either or different; an absent one present in one only or
// different; a linter one enables and the other does not, but for a project
// linter the repository adds; and an exclusion rule of base that repo does
// not hold. repo may hold more rules: those are its project's own.
func presetDrift(base, repo *ordered.Map) []string {
	var out []string
	for _, p := range requiredSettings() {
		b, inBase := setting(base, p)
		r, inRepo := setting(repo, p)
		if !inBase || !inRepo || b != r {
			out = append(out, fmt.Sprintf("%s: preset %s, repository %s", p, b, r))
		}
	}
	for _, p := range absentSettings() {
		b, inBase := setting(base, p)
		r, inRepo := setting(repo, p)
		if inBase != inRepo || b != r {
			out = append(out, fmt.Sprintf("%s: preset %s, repository %s", p, b, r))
		}
	}
	baseOn, repoOn := enabledLinters(base), enabledLinters(repo)
	for _, l := range baseOn {
		if !slices.Contains(repoOn, l) {
			out = append(out, "linters.enable: the preset enables "+l+" and the repository does not")
		}
	}
	for _, l := range repoOn {
		if !slices.Contains(baseOn, l) && !slices.Contains(projectLinters(), l) {
			out = append(out, "linters.enable: the repository enables "+l+" and the preset does not")
		}
	}
	have := items(repo, "linters.exclusions.rules")
	for _, x := range items(base, "linters.exclusions.rules") {
		if !slices.Contains(have, x) {
			out = append(out, "linters.exclusions.rules: the preset's rule "+x+" is not in the repository's list")
		}
	}
	return out
}

// TestThePresetSilencesWhatTheRepositorySilences: presets/golangci-lint/.golangci.yml
// holds each silencer setting as .golangci.yml does, enables the same
// linters, and holds no exclusion rule it does not, so a project that copies
// it reads no gosec suppression comment and no generated-file header either.
func TestThePresetSilencesWhatTheRepositorySilences(t *testing.T) {
	t.Parallel()
	base, repo := lintDoc(t, "presets/golangci-lint/.golangci.yml"), lintDoc(t, ".golangci.yml")
	for _, p := range []string{"linters.enable", "linters.exclusions.rules"} {
		if len(items(base, p)) == 0 {
			t.Fatalf("the preset holds no %s, so nothing is compared", p)
		}
	}
	for _, d := range presetDrift(base, repo) {
		t.Error(d)
	}
}

// TestAPresetThatSilencesMoreIsDrift: a preset that turns a linter off, reads
// a suppression comment or a generated header, leaves a path out, or excludes
// a finding the repository does not, differs from the repository; the same
// document on both sides does not. A repository that drops a linter the
// preset enables, or enables one beyond the project linters, is drift; one
// that enables a project linter or holds more rules is not.
func TestAPresetThatSilencesMoreIsDrift(t *testing.T) {
	t.Parallel()
	parse := func(src string) *ordered.Map {
		t.Helper()
		v, err := ordered.ParseYAML([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		doc, ok := v.(*ordered.Map)
		if !ok {
			t.Fatal("not a mapping")
		}
		return doc
	}
	const repo = "run:\n  tests: true\nissues:\n  max-same-issues: 0\nlinters:\n  default: standard\n  enable:\n    - gosec\n    - noctx\n    - revive\n  settings:\n    gosec:\n      config:\n        global:\n          nosec: true\n    nolintlint:\n      require-specific: true\n    forbidigo:\n      exclude-godoc-examples: false\n  exclusions:\n    generated: disable\n    presets: []\n    rules:\n      - path: _test\\.go$\n        linters: [errcheck]\n"
	if d := presetDrift(parse(repo), parse(repo)); len(d) != 0 {
		t.Fatalf("the same document is drift: %q", d)
	}
	more := strings.Replace(strings.Replace(repo, "    - revive\n", "    - revive\n    - sqlclosecheck\n", 1), "        linters: [errcheck]\n", "        linters: [errcheck]\n      - path: ^internal/x/\n        linters: [forbidigo]\n", 1)
	if more == repo {
		t.Fatal("the repository's own additions changed nothing")
	}
	if d := presetDrift(parse(repo), parse(more)); len(d) != 0 {
		t.Errorf("a repository that enables a project linter and holds more rules is drift: %q", d)
	}
	if d := presetDrift(parse(repo), parse(strings.Replace(repo, "    - revive\n", "    - revive\n    - gocritic\n", 1))); len(d) != 1 {
		t.Errorf("a repository that enables a linter the preset does not, and no project linter: drift %q, want one", d)
	}
	if d := presetDrift(parse(repo), parse(strings.Replace(repo, "    - noctx\n", "", 1))); len(d) != 1 {
		t.Errorf("a repository that drops a linter the preset enables: drift %q, want one", d)
	}
	for name, preset := range map[string]string{
		"nosec read":              strings.Replace(repo, "nosec: true", "nosec: false", 1),
		"generated dropped":       strings.Replace(repo, "    generated: disable\n", "", 1),
		"generated lax":           strings.Replace(repo, "generated: disable", "generated: lax", 1),
		"a preset turned on":      strings.Replace(repo, "presets: []", "presets: [comments]", 1),
		"tests not linted":        strings.Replace(repo, "tests: true", "tests: false", 1),
		"no default linters":      strings.Replace(repo, "default: standard", "default: none", 1),
		"gosec not enabled":       strings.Replace(repo, "    - gosec\n", "", 1),
		"gosec disabled":          strings.Replace(repo, "  enable:\n", "  disable:\n    - gosec\n  enable:\n", 1),
		"every path left out":     strings.Replace(repo, "    presets: []\n", "    presets: []\n    paths: [\".\"]\n", 1),
		"only one path linted":    strings.Replace(repo, "    presets: []\n", "    presets: []\n    paths-except: [\"^cmd/\"]\n", 1),
		"a rule that drops gosec": strings.Replace(repo, "    rules:\n", "    rules:\n      - path: .*\n        linters: [gosec]\n", 1),
		"examples skipped":        strings.Replace(repo, "exclude-godoc-examples: false", "exclude-godoc-examples: true", 1),
		"examples left default":   strings.Replace(repo, "    forbidigo:\n      exclude-godoc-examples: false\n", "", 1),
	} {
		if preset == repo {
			t.Fatalf("%s: the plant changed nothing", name)
		}
		if d := presetDrift(parse(preset), parse(repo)); len(d) != 1 {
			t.Errorf("%s: drift %q, want one", name, d)
		}
	}
}
