package layers_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

// usesFile is the uses file of a repo with two picks, one job that is not for it and two findings.
const usesFile = `repo: app
picks:
  - item: the YAML reader
    register_key: yaml
    job: read and write YAML
    scope: anywhere
    ban: "depguard: deny pkg gopkg.in/yaml.v3"
    banned_calls: [gopkg.in/yaml.v3]
    sources: ["https://github.com/yaml/go-yaml"]
  - item: exit codes
    register_key: first-party:lib/exit
    job: exit codes and closing words
    scope: import lib/exit; no os.Exit anywhere
    ban: "none: review only (a test of the program: no os.Exit in any Go file)"
    pattern: 'os\.Exit\('
must_use: [yaml]
not_for_this_repo:
  - item: path-lock
    why: the program writes one file and takes no lock
    sources: ["https://github.com/gofrs/flock"]
findings:
  - where: main.go:167
    what: exit codes by hand
    lines: 21
    replaced_by: [first-party:lib/exit]
    removed_by: ["app-7"]
    pattern: 'os\.Exit\('
    state: open
  - where: render.go
    what: a home-made progress meter
    cut: true
    carried: true
    symbol: Meter
    state: later
`

func noUsesProblems(any) []string { return nil }

func uses(t *testing.T, text string) (*layers.Uses, error) {
	t.Helper()
	return layers.ParseUses([]byte(text), noUsesProblems)
}

// TestAUsesFileReadsItsPicksFindingsAndNotForJobs: picks, the jobs that are not for the repo and findings read with
// their fields, and the file holds to the register: a pick names a chosen row, a replacing library has a row.
//
// What it costs when this fails: a repo's picks read wrongly make the lint ban a library the repo chose, or let a
// hand-written copy stay with no ticket.
func TestAUsesFileReadsItsPicksFindingsAndNotForJobs(t *testing.T) {
	t.Parallel()
	u, err := uses(t, usesFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Picks) != 2 || len(u.NotFor) != 1 || len(u.Findings) != 2 || len(u.MustUse) != 1 {
		t.Fatalf("read %d picks, %d not-for, %d findings and %d must_use, want 2, 1, 2 and 1", len(u.Picks), len(u.NotFor), len(u.Findings), len(u.MustUse))
	}
	if f := u.Findings[1]; !f.Cut || !f.Carried || f.Symbol != "Meter" || f.State != layers.FindingLater {
		t.Errorf("the cut finding read as %+v", f)
	}
	r, err := register(t, library)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Hold(r); err != nil {
		t.Errorf("the uses file over the register: %v", err)
	}
	bad := strings.NewReplacer("register_key: yaml\n", "register_key: toml-read\n", "must_use: [yaml]", "must_use: [toml-read]").Replace(usesFile)
	other, err := uses(t, bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Hold(r); err == nil || !strings.Contains(err.Error(), "register key toml-read, which has no row") {
		t.Errorf("a pick of a key with no row: %v", err)
	}
	spike := strings.NewReplacer("register_key: yaml\n", "register_key: path-lock\n", "must_use: [yaml]", "must_use: [path-lock]").Replace(usesFile)
	s, err := uses(t, strings.Replace(spike, "not_for_this_repo:\n  - item: path-lock\n    why: the program writes one file and takes no lock\n    sources: [\"https://github.com/gofrs/flock\"]\n", "", 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Hold(r); err == nil || !strings.Contains(err.Error(), "which is needs-spike, not chosen") {
		t.Errorf("a pick of a row that needs a spike: %v", err)
	}
}

// TestAUsesFileRefusesWhatDoesNotHold: a pick with no register key and no first-party path, a job with no banned
// call and no review-only line, a job with no import and no pattern, a finding with no ticket and no carried mark,
// one with neither a symbol nor a pattern, one with both, a cut finding that names a library, a finding that names
// none and is not cut, a state it does not know, a job that is not for the repo with no source, a must_use that is
// no pick, a pick that is also not for the repo, a pattern that does not compile, and a twice-written pick each fail
// with ErrUses and say what.
//
// What it costs when this fails: a finding with no ticket stays forever, a repo's own picks go unchecked, and
// "not used here" passes with no source.
func TestAUsesFileRefusesWhatDoesNotHold(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, text, says string }{
		{"a pick with no register key", strings.Replace(usesFile, "    register_key: yaml\n", "", 1), "register_key is missing"},
		{"a pick of a key that is no key", strings.Replace(usesFile, "register_key: yaml", "register_key: Not A Key", 1), "names no register key and no first-party path"},
		{"a job with no banned call and no review-only line", strings.Replace(usesFile, "    banned_calls: [gopkg.in/yaml.v3]\n", "", 1), "says `none: review only` in ban"},
		{"a job with no import and no pattern", strings.Replace(usesFile, "    pattern: 'os\\.Exit\\('\nmust_use", "must_use", 1), "carries a pattern"},
		{"a finding with no ticket and no carried mark", strings.Replace(usesFile, "    removed_by: [\"app-7\"]\n", "", 1), "names the ticket that removes it, or is marked carried"},
		{"a finding with neither a symbol nor a pattern", strings.Replace(usesFile, "    symbol: Meter\n", "", 1), "a symbol or a pattern, or both"},
		{"a finding marked later with another state", strings.Replace(usesFile, "state: later", "later: true\n    state: closed", 1), "it is marked later and its state is closed"},
		{"a finding lines that is a list", strings.Replace(usesFile, "    lines: 21\n", "    lines: [1]\n", 1), "lines is a count, a text or nothing"},
		{"a finding lines that is negative", strings.Replace(usesFile, "    lines: 21\n", "    lines: -1\n", 1), "finding at main.go:167: lines is a count"},
		{"a cut finding that names a library", strings.Replace(usesFile, "    cut: true\n", "    cut: true\n    replaced_by: [yaml]\n", 1), "a cut finding names no replacing library"},
		{"a finding that names no library and is not cut", strings.Replace(usesFile, "    cut: true\n", "", 1), "names the library that replaces it, or is cut"},
		{"a state it does not know", strings.Replace(usesFile, "state: open", "state: pending", 1), "is not one of open, later, closed"},
		{"a job that is not for the repo with no source", strings.Replace(usesFile, "    sources: [\"https://github.com/gofrs/flock\"]\n", "", 1), "sources is missing"},
		{"a must_use that is no pick", strings.Replace(usesFile, "must_use: [yaml]", "must_use: [toml-read]", 1), "must_use toml-read is no pick of the file"},
		{"a pick that is also not for the repo", strings.Replace(usesFile, "item: path-lock", "item: yaml", 1), "is a pick and a job that is not for this repo"},
		{"a pattern that does not compile", strings.Replace(usesFile, "pattern: 'os\\.Exit\\('\nmust_use", "pattern: 'os\\.(Exit'\nmust_use", 1), "does not compile as a Go regexp"},
		{"a pick written twice", strings.Replace(usesFile, "  - item: exit codes\n", "  - item: the YAML reader\n", 1), "is there twice"},
		{"a key it does not know", strings.Replace(usesFile, "repo: app\n", "repo: app\ncolour: blue\n", 1), "colour is not a key"},
		{"an adopted_by item that is no one-line string", strings.Replace(usesFile, "    sources: [\"https://github.com/yaml/go-yaml\"]\n", "    sources: [\"https://github.com/yaml/go-yaml\"]\n    adopted_by: [' ']\n", 1), "adopted_by holds an item that is not a one-line string"},
		{"a source written as an empty text", strings.Replace(usesFile, "    sources: [\"https://github.com/yaml/go-yaml\"]\n", "    sources: ''\n", 1), "sources is empty"},
		{"a blocker with no why", strings.Replace(usesFile, "findings:\n", "blockers:\n  - {ticket: t-1}\nfindings:\n", 1), "why is missing"},
		{"a blocker with no ticket", strings.Replace(usesFile, "findings:\n", "blockers:\n  - {why: w}\nfindings:\n", 1), "ticket is missing"},
		{"a blocker with a key it does not know", strings.Replace(usesFile, "findings:\n", "blockers:\n  - {ticket: t-1, why: w, colour: blue}\nfindings:\n", 1), "colour is not a key"},
		{"a repo that is no repository name", strings.Replace(usesFile, "repo: app\n", "repo: App!\n", 1), `repo "App!" is not a repository name`},
		{"no pick at all", "repo: app\npicks: []\n", "picks holds no pick"},
		{"a must_use that is no key", strings.Replace(usesFile, "must_use: [yaml]", "must_use: [Bad Key]", 1), `must_use "Bad Key" is a register key or a first-party path`},
		{"a source that is no one-line string", strings.Replace(usesFile, "    sources: [\"https://github.com/yaml/go-yaml\"]\n", "    sources: [' ']\n", 1), "sources holds an item that is not a text"},
		{"a banned call that is no call", strings.Replace(usesFile, "banned_calls: [gopkg.in/yaml.v3]", "banned_calls: ['not a call!']", 1), `banned call "not a call!"`},
		{"a not-for job that names no job", strings.Replace(usesFile, "  - item: path-lock\n", "  - item: ' '\n", 1), "names the job and says why"},
		{"a not-for job with an empty source list", strings.Replace(usesFile, "    sources: [\"https://github.com/gofrs/flock\"]\n", "    sources: []\n", 1), "has no source: `not used here` is allowed only with one"},
		{"a finding pattern that does not compile", strings.Replace(usesFile, "    pattern: 'os\\.Exit\\('\n    state: open", "    pattern: 'os\\.(Exit'\n    state: open", 1), "finding at main.go:167: pattern"},
		{"an unexpanded placeholder", strings.Replace(usesFile, "register_key: yaml", "register_key: $k", 1), `register_key holds an unexpanded placeholder: "$k"`},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		if tc.text == usesFile {
			t.Errorf("%s: the plant changed nothing", tc.name)
			continue
		}
		_, err := uses(t, tc.text)
		if !errors.Is(err, layers.ErrUses) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: ParseUses = %v, want ErrUses saying %q", tc.name, err, tc.says)
		}
	}
	if ran != len(cases) || ran < 32 {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	if _, err := layers.ParseUses([]byte(usesFile), nil); !errors.Is(err, layers.ErrUses) {
		t.Errorf("with no schema check ParseUses = %v, want ErrUses", err)
	}
	if _, err := uses(t, usesFile); err != nil {
		t.Errorf("the unplanted file: %v", err)
	}
}

const libraryFile = `packages:
  - package: write
    job: replace a file whole
    banned_calls: [os.CreateTemp, github.com/google/renameio]
  - package: lock
    job: one holder per path
    banned_calls: [syscall.Flock]
`

// TestALibraryFileReadsItsPackagesAndRefusesWhatDoesNotHold: each package, its job and its banned calls read, and a
// package with no banned call, with no job, written twice or with a path that is no path each fail.
//
// What it costs when this fails: a library package that names no banned call bans nothing, so the program keeps the
// hand-written copy the library was made to replace.
func TestALibraryFileReadsItsPackagesAndRefusesWhatDoesNotHold(t *testing.T) {
	t.Parallel()
	l, err := layers.ParseLibrary([]byte(libraryFile), noUsesProblems)
	if err != nil || len(l.Packages) != 2 || len(l.Packages[0].BannedCalls) != 2 {
		t.Fatalf("ParseLibrary = %+v, %v", l, err)
	}
	cases := []struct{ name, text, says string }{
		{"no banned call", strings.Replace(libraryFile, "    banned_calls: [syscall.Flock]\n", "    banned_calls: []\n", 1), "names no banned call"},
		{"no job", strings.Replace(libraryFile, "    job: one holder per path\n", "", 1), "job is missing"},
		{"a package written twice", strings.Replace(libraryFile, "package: lock", "package: write", 1), "is there twice"},
		{"a path that is no path", strings.Replace(libraryFile, "package: lock", "package: ../lock", 1), "is a path in the library's module"},
		{"a banned call that is no call", strings.Replace(libraryFile, "syscall.Flock", "not a call!", 1), "is an import path or a package and a name"},
		{"a package with no job", strings.Replace(libraryFile, "    job: one holder per path\n", "    job: ' '\n", 1), "package lock names no job"},
		{"no package at all", "packages: []\n", "packages holds no package"},
		{"an unexpanded placeholder", strings.Replace(libraryFile, "job: replace a file whole", "job: $j", 1), `job holds an unexpanded placeholder: "$j"`},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		_, err := layers.ParseLibrary([]byte(tc.text), noUsesProblems)
		if !errors.Is(err, layers.ErrUses) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: ParseLibrary = %v, want ErrUses saying %q", tc.name, err, tc.says)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	if _, err := layers.ParseLibrary([]byte(libraryFile), nil); !errors.Is(err, layers.ErrUses) {
		t.Errorf("with no schema check ParseLibrary = %v, want ErrUses", err)
	}
}

// TestThePublishedMatrixIsComparedWithTheUsesFileAsData: the matrix the file generates compares clean with itself;
// a matrix with a pick changed, a finding missing or a job that is not for the repo added is told apart, and the
// fields the file does not hold (blockers, notes) are left alone.
//
// What it costs when this fails: the published matrix and the file a repo carries drift apart and nobody sees it,
// so a ticket is filed from a matrix the repo no longer holds.
func TestThePublishedMatrixIsComparedWithTheUsesFileAsData(t *testing.T) {
	t.Parallel()
	u, err := uses(t, usesFile)
	if err != nil {
		t.Fatal(err)
	}
	m, err := u.Matrix()
	if err != nil {
		t.Fatal(err)
	}
	if got := u.CompareMatrix(m); len(got) != 0 {
		t.Fatalf("the generated matrix differs from its own file: %q", got)
	}
	if got := u.CompareMatrix([]byte(strings.Replace(string(m), `"repo": "app",`, `"repo": "app", "plan_key_prefix": "app",`, 1))); len(got) != 1 || !strings.Contains(got[0], "plan_key_prefix differs") {
		t.Errorf("a plan key the file lacks: %q", got)
	}
	if got := u.CompareMatrix([]byte(strings.Replace(string(m), `"repo": "app",`, `"repo": "app", "colour": "blue",`, 1))); len(got) != 1 || !strings.Contains(got[0], "does not read as the uses file's form") {
		t.Errorf("a field the uses file has no place for: %q", got)
	}
	asText := strings.Replace(string(m), "\"sources\": [\n    \"https://github.com/yaml/go-yaml\"\n   ]", "\"sources\": \"https://github.com/yaml/go-yaml\"", 1)
	if asText == string(m) {
		t.Fatal("the plant of a source written as a text changed nothing")
	}
	if got := u.CompareMatrix([]byte(asText)); len(got) != 0 {
		t.Errorf("a source written as a text where the file has a list of one made a difference: %q", got)
	}
	cases := []struct{ name, text, says string }{
		{"a pick changed", strings.Replace(string(m), `"job": "read and write YAML"`, `"job": "read YAML"`, 1), `pick "the YAML reader" differs`},
		{"a finding missing", strings.Replace(string(m), `"where": "render.go"`, `"where": "other.go"`, 1), `finding "render.go|Meter" is in the uses file and not in the matrix`},
		{"a job that is not for the repo added", strings.Replace(string(m), `"not_for_this_repo": [`, `"not_for_this_repo": [{"item": "extra", "why": "w", "sources": ["s"]},`, 1), `not_for_this_repo "extra" is in the matrix and not in the uses file`},
		{"another repo", strings.Replace(string(m), `"repo": "app"`, `"repo": "other"`, 1), "the matrix is for other"},
		{"not JSON", "{", "does not read as the uses file's form"},
	}
	ran := 0
	for _, tc := range cases {
		ran++
		got := u.CompareMatrix([]byte(tc.text))
		if len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), tc.says) {
			t.Errorf("%s: CompareMatrix = %q, want a line saying %q", tc.name, got, tc.says)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
}

// FuzzTheReadersOfTheRegisterAndTheUsesFiles: whatever bytes the readers are handed, none panics, and a file that
// reads once reads again to the same matrix.
//
// What it costs when this fails: a repo's malformed file crashes the checker that reads it, instead of being
// refused with a line.
func FuzzTheReadersOfTheRegisterAndTheUsesFiles(f *testing.F) {
	for _, seed := range []string{usesFile, libraryFile, library, "", "{", "repo: [", "picks: 1\n"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = layers.ParseRegister(data, noRegisterProblems)
		_, _ = layers.ParseLibrary(data, noUsesProblems)
		u, err := layers.ParseUses(data, noUsesProblems)
		if err != nil {
			return
		}
		first, err := u.Matrix()
		if err != nil {
			t.Fatalf("a file that read has no matrix: %v", err)
		}
		if got := u.CompareMatrix(first); len(got) != 0 {
			t.Errorf("a file that read differs from its own matrix: %q", got)
		}
	})
}

// TestTheUsesFileTakesTheSpellingsOfThePublishedMatrix: a source or a note written as one text, a lines count that is a
// number, a text or nothing, a pick key of the forms tool: and std:, a replacing library written as a path, a version
// or a sentence, a finding that carries both a symbol and a pattern, and the later mark each read, and the file
// that carries them generates a matrix with them.
//
// What it costs when this fails: a published matrix is refused for how it spells a fact, so each repo has to
// rewrite what the matrix already says before its file reads.
func TestTheUsesFileTakesTheSpellingsOfThePublishedMatrix(t *testing.T) {
	t.Parallel()
	text := `repo: app
notes: one text
plan_key_prefix: app
related: [other-1]
picks:
  - {item: lint, register_key: "tool:golangci-lint", job: j, scope: s, ban: "none: the tool itself", pattern: 'layers_test', sources: one source}
  - {item: json, register_key: "std:encoding/json/v2", job: j, scope: s, ban: "none: review only", pattern: '"encoding/json"'}
  - {item: reg, register_key: yaml, job: j, scope: s, ban: b, banned_calls: [gopkg.in/yaml.v3], conditional: only if x, adopted_by: [t-2]}
findings:
  - {where: a.go, what: w, lines: 12, symbol: S, pattern: 'x', replaced_by: ["os.Root (Go 1.27)", "first-party:lib/exit", "mvdan.cc/sh/v3 v3.14.1"], removed_by: [t-1], sources: a text, note: n, kept_issue: "x#1", carried: true}
  - {where: b.go, what: w, lines: "about 60 sites", symbol: S, removed_by: [t-1], replaced_by: [yaml]}
  - {where: c.go, what: w, lines: null, symbol: S, removed_by: [t-1], replaced_by: [yaml], later: true}
`
	u, err := uses(t, text)
	if err != nil {
		t.Fatal(err)
	}
	if u.Findings[0].Lines != int64(12) || u.Findings[1].Lines != "about 60 sites" || u.Findings[2].Lines != nil || u.Findings[2].State != layers.FindingLater || u.Findings[0].State != layers.FindingOpen {
		t.Errorf("lines or states read wrongly: %+v", u.Findings)
	}
	if len(u.Picks[0].Sources) != 1 || len(u.Notes) != 1 {
		t.Errorf("a source or a note written as a text read as %v and %v", u.Picks[0].Sources, u.Notes)
	}
	m, err := u.Matrix()
	if err != nil {
		t.Fatal(err)
	}
	if got := u.CompareMatrix(m); len(got) != 0 {
		t.Errorf("a file with every spelling differs from its own matrix: %q", got)
	}
	for _, want := range []string{`"carried": true`, `"note": "n"`, `"conditional": "only if x"`, `"adopted_by": [`, `"plan_key_prefix"`, `"later": true`, `"lines": null`, `"lines": "about 60 sites"`, `"kept_issue": "x#1"`, `"related": [`} {
		if !strings.Contains(string(m), want) {
			t.Errorf("the matrix lacks %s:\n%s", want, m)
		}
	}
}

// TestHoldRefusesWhatTheRegisterDoesNotKnow: a pick that names a register key with no row, a row that is not
// chosen, a tool: the register does not point to, and a finding replaced by a bare register key with no row each
// fail Hold with ErrUses and say which; a std: pick, a tool: the register points to, a first-party pick and a
// replacing path read against nothing pass, a pick of a conditional row passes when it says it is conditional and fails when
// it does not, and a pick of a spike row fails whatever it says.
//
// What it costs when this fails: a repo's picks name rows that are not there, so the lint bans a library the register
// does not hold, or a finding is replaced by a library nobody chose.
func TestHoldRefusesWhatTheRegisterDoesNotKnow(t *testing.T) {
	t.Parallel()
	r, err := register(t, library)
	if err != nil {
		t.Fatal(err)
	}
	head := "repo: app\npicks:\n  - {item: p, register_key: %s, job: j, scope: s, ban: b, banned_calls: [x.y/z]%s}\n"
	finding := "findings:\n  - {where: a.go, what: w, symbol: S, removed_by: [t-1], replaced_by: [%s]}\n"
	cases := []struct{ name, key, extra, finding, says string }{
		{"a key with no row", "toml-read", "", "yaml", "names register key toml-read, which has no row"},
		{"a row that is not chosen", "path-lock", "", "yaml", "names row path-lock, which is needs-spike, not chosen"},
		{"a tool the register does not point to", `"tool:nothing"`, "", "yaml", "names tool nothing, which the register does not point to"},
		{"a finding replaced by a key with no row", "yaml", "", "toml-read", "is replaced by toml-read, which has no row"},
		{"a std pick", `"std:testing"`, "", "yaml", ""},
		{"a tool the register points to", `"tool:govulncheck"`, "", "yaml", ""},
		{"a first-party pick", `"first-party:lib/exit"`, "", "os.Root (Go 1.27)", ""},
		{"a spike row said to be conditional", "path-lock", ", conditional: only if the spike passes", "mvdan.cc/sh/v3 v3.14.1", "names row path-lock, which is needs-spike, not chosen"},
		{"a conditional row said to be conditional", "path-lock", ", conditional: only if the proof passes", "mvdan.cc/sh/v3 v3.14.1", ""},
		{"a conditional row not said to be conditional", "path-lock", "", "yaml", "names row path-lock, which is conditional, and the pick does not say it is conditional"},
	}
	conditional, err := register(t, strings.Replace(library, "state: needs-spike", "state: conditional\n    proof: the spike passes", 1))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, tc := range cases {
		ran++
		u, err := uses(t, fmt.Sprintf(head, tc.key, tc.extra)+fmt.Sprintf(finding, tc.finding))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		reg := r
		if strings.Contains(tc.name, "conditional row") {
			reg = conditional
		}
		err = u.Hold(reg)
		switch {
		case tc.says == "" && err != nil:
			t.Errorf("%s: Hold = %v, want no refusal", tc.name, err)
		case tc.says != "" && (!errors.Is(err, layers.ErrUses) || !strings.Contains(err.Error(), tc.says)):
			t.Errorf("%s: Hold = %v, want ErrUses saying %q", tc.name, err, tc.says)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
}
