package audit_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// rules is the rule each key is checked under, as a rulebook would say.
func rules() audit.RepoRules {
	return audit.RepoRules{
		"link": "gates.tracked.1", "fold": "gates.tracked.2", "names": "supply.secrets.4",
		"replace": "supply.age.3", "debt": "gates.ratchets.1", "directive": "gates.exemptions.2",
	}
}

// configs are the file names of the linter config.
// configs is the count's data of one linter, as its plugin gives it
// the test hands it in, and the audit names no tool's paths.
func configs() []audit.Counting {
	return []audit.Counting{{
		Configs: []string{".golangci.yml", ".golangci.yaml"},
		Exclusions: [][]string{
			{"linters", "exclusions", "rules"}, {"linters", "exclusions", "paths"}, {"linters", "exclusions", "paths-except"},
			{"formatters", "exclusions", "paths"},
			{"issues", "exclude-rules"}, {"issues", "exclude"}, {"issues", "exclude-dirs"}, {"issues", "exclude-files"},
			{"run", "skip-dirs"}, {"run", "skip-files"},
		},
		Directive: "nolint",
	}}
}

// kindDirective is the kind the linter's directive is counted as.
func kindDirective() string { return audit.DirectiveKind("nolint") }

// words is a stand-in names list: a text holds a name when it holds one of its
// words. The real list is private; no test reads it.
type words []string

func (w words) Find(text string) bool {
	for _, s := range w {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func TestNamesFindsAPlantInAFileItsNameACommitAndAGivenTextAndNeverSaysTheName(t *testing.T) {
	t.Parallel()
	list := words{"zorbquux"}
	in := audit.NamesInput{
		Rules:    rules(),
		List:     list,
		Files:    []audit.Doc{{Path: "a.txt", Text: "one\ntwo zorbquux\n"}, {Path: "b.txt", Text: "clean\n"}},
		Paths:    []string{"docs/zorbquux.md", "docs/clean.md"},
		Messages: []audit.Message{{Commit: "abc", Text: "subject\n\nzorbquux in the body"}, {Commit: "def", Text: "fine"}},
		Texts:    []audit.Doc{{Path: "pr-body.txt", Text: "zorbquux"}},
	}
	got, none := audit.CheckNames(in)
	if none != nil {
		t.Fatalf("the check did not run: %v", none)
	}
	want := []string{
		"a.txt:2: supply.secrets.4 (gragas names): holds a name from the private names list",
		"commit abc:3: supply.secrets.4 (gragas names): message holds a name from the private names list",
		"docs/zorbquux.md: supply.secrets.4 (gragas names): has a file name that holds a name from the private names list",
		"pr-body.txt:1: supply.secrets.4 (gragas names): holds a name from the private names list",
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
	for i, f := range got {
		if f.String() != want[i] {
			t.Errorf("finding %d = %q, want %q", i, f.String(), want[i])
		}
		// The core says the path as it is; CheckRepo masks it (the api tests).
		if strings.Contains(f.Missing, "zorbquux") {
			t.Errorf("finding %d says the name in what it says: %q", i, f)
		}
	}
}

func TestNamesControlPassesCleanTextsAndSkipsBinary(t *testing.T) {
	t.Parallel()
	got, none := audit.CheckNames(audit.NamesInput{
		Rules: rules(),
		List:  words{"zorbquux"},
		Files: []audit.Doc{{Path: "a.txt", Text: "clean"}, {Path: "blob.bin", Text: "zorbquux\x00"}},
		Paths: []string{"a.txt"},
	})
	if len(got) != 0 || none != nil {
		t.Fatalf("got %v, %v; want nothing", got, none)
	}
}

func TestNamesWithNoListDidNotRunAndIsNotAPass(t *testing.T) {
	t.Parallel()
	got, none := audit.CheckNames(audit.NamesInput{Rules: rules(), Files: []audit.Doc{{Path: "a.txt", Text: "zorbquux"}}})
	if len(got) != 0 || none == nil || !strings.Contains(none.String(), "no names list was given") {
		t.Fatalf("got %v, %v; want the check not run", got, none)
	}
}

// keyed is a path with the key a caller gives it: here the path in lower case, as a disk that folds case reads it,
// since the check holds no table of its own and takes each key from its caller.
func keyed(p string, link bool) audit.Tracked {
	return audit.Tracked{Path: p, Link: link, Key: strings.ToLower(p)}
}

func TestATrackedLinkAndTwoPathsThatDifferByCaseAreFindings(t *testing.T) {
	t.Parallel()
	got, unchecked := audit.CheckTracked(rules(), []audit.Tracked{
		keyed("README.md", false), keyed("Readme.md", false), keyed("docs/link", true), keyed("docs/real.md", false), keyed("\u00dcn\u00ef.txt", false), keyed("\u00fcn\u00ef.txt", false),
	})
	want := []string{
		"README.md: gates.tracked.2 (gragas fold): differs only by case from Readme.md, which the macOS and Windows file systems hold as one path",
		"docs/link: gates.tracked.1 (gragas link): is a tracked symbolic link, which Git for Windows checks out as a file holding the link's text",
		"\u00dcn\u00ef.txt: gates.tracked.2 (gragas fold): differs only by case from \u00fcn\u00ef.txt, which the macOS and Windows file systems hold as one path",
	}
	if len(got) != len(want) || len(unchecked) != 0 {
		t.Fatalf("findings = %v, not checked = %v", got, unchecked)
	}
	seen := map[string]bool{}
	for _, f := range got {
		seen[f.String()] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing %q in %v", w, got)
		}
	}
	if clean, none := audit.CheckTracked(rules(), []audit.Tracked{keyed("a/b.go", false), keyed("a/c.go", false), keyed("a/B/c.go", false)}); len(clean) != 0 || len(none) != 0 {
		t.Fatalf("control: %v, %v", clean, none)
	}
}

// TestATrackedNameNoSystemCanHoldIsNotCheckedAndNeverAFinding: a path whose name the port
// refuses, or that came with no key, is a not-checked row naming the path and the reason: it is never dropped,
// never a clean answer and never a finding, and the paths beside it are still grouped.
// Risk: a tracked name no disk can hold vanishes from the run, or makes it say clean for a path it never judged.
func TestATrackedNameNoSystemCanHoldIsNotCheckedAndNeverAFinding(t *testing.T) {
	t.Parallel()
	got, unchecked := audit.CheckTracked(rules(), []audit.Tracked{
		keyed("a.md", false), keyed("A.md", false),
		{Path: "dir/v1.", Refused: `"v1." is not a portable name: it ends in a dot or a space, which Windows strips`},
		{Path: "dir/loose.md"},
	})
	if len(got) != 1 || !strings.Contains(got[0].String(), "differs only by case from A.md") && !strings.Contains(got[0].String(), "differs only by case from a.md") {
		t.Fatalf("findings = %v, want the one case pair", got)
	}
	if len(unchecked) != 2 {
		t.Fatalf("not checked = %v, want two rows", unchecked)
	}
	joined := unchecked[0].String() + "\n" + unchecked[1].String()
	for _, want := range []string{"dir/v1. was not checked", "it ends in a dot or a space", "dir/loose.md was not checked", "it came with no key"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %q in %s", want, joined)
		}
	}
}

// TestAPairWithAKeyAndARefusedNameIsGroupedAndStillNotChecked: a path the caller keyed by the names it
// could compare, and marked Refused for the one it could not, is grouped on its key, so the pair is one finding, and is
// a not-checked row besides: the refused name is never a clean answer. A refused path with no key is a row alone.
func TestAPairWithAKeyAndARefusedNameIsGroupedAndStillNotChecked(t *testing.T) {
	t.Parallel()
	why := `"aux" is not a portable name: Windows keeps it for a device`
	got, unchecked := audit.CheckTracked(rules(), []audit.Tracked{
		{Path: "aux/Readme.md", Key: "\x00aux/readme.md", Refused: why},
		{Path: "aux/README.md", Key: "\x00aux/readme.md", Refused: why},
		{Path: "v1.", Refused: "it ends in a dot"},
	})
	if len(got) != 1 || !strings.Contains(got[0].String(), "differs only by case from aux/Readme.md") {
		t.Fatalf("findings = %v, want the one pair", got)
	}
	if len(unchecked) != 3 {
		t.Fatalf("not checked = %v, want a row for each of the three paths", unchecked)
	}
}

// notCompared is the two paths the case-twin check cannot compare, a name the
// port refuses and a path that came with no key, beside a link and a case pair.
func notCompared() []audit.Tracked {
	return []audit.Tracked{
		keyed("a.md", false), keyed("A.md", false), keyed("notes.lnk", true),
		{Path: "dir/v1.", Refused: "it ends in a dot"},
		{Path: "dir/loose.md"},
	}
}

// TestAPathNotComparedIsNoRowWhenNoRuleBindsTheCaseTwinCheck: with
// no rule on the fold key, nobody asked for the case-twin check, so a path it
// could not compare is no not-checked row: not with the link rule alone, which
// still finds its link, and not with no rule at all.
// Risk: every screen of a project that pins no such rule carries a row about a
// check its book does not hold.
func TestAPathNotComparedIsNoRowWhenNoRuleBindsTheCaseTwinCheck(t *testing.T) {
	t.Parallel()
	got, unchecked := audit.CheckTracked(audit.RepoRules{"link": "gates.tracked.1"}, notCompared())
	if len(got) != 1 || !strings.Contains(got[0].String(), "notes.lnk: gates.tracked.1") {
		t.Fatalf("findings = %v, want the link alone: the control, the check ran", got)
	}
	if len(unchecked) != 0 {
		t.Errorf("not checked = %v, want none: no rule binds the case-twin check", unchecked)
	}
	if got, unchecked := audit.CheckTracked(audit.RepoRules{}, notCompared()); len(got) != 0 || len(unchecked) != 0 {
		t.Errorf("with no rule: findings = %v, not checked = %v, want neither", got, unchecked)
	}
}

// TestAPathNotComparedIsARowWhenARuleBindsTheCaseTwinCheck is the
// other half: with the fold rule, and it alone, each of the two paths is its
// row, worded as before, and the pair is still the finding.
// Risk: the row goes with the noise, and a bound check says clean for a path
// it never compared.
func TestAPathNotComparedIsARowWhenARuleBindsTheCaseTwinCheck(t *testing.T) {
	t.Parallel()
	got, unchecked := audit.CheckTracked(audit.RepoRules{"fold": "gates.tracked.2"}, notCompared())
	if len(got) != 1 || !strings.Contains(got[0].String(), "differs only by case from") {
		t.Fatalf("findings = %v, want the case pair alone", got)
	}
	want := []string{
		"dir/loose.md was not checked for a twin that differs only by case: it came with no key",
		"dir/v1. was not checked for a twin that differs only by case: it ends in a dot",
	}
	if len(unchecked) != len(want) {
		t.Fatalf("not checked = %v, want %d rows", unchecked, len(want))
	}
	for i, w := range want {
		if unchecked[i].Why != w || unchecked[i].Tool != audit.Unjudged(audit.KeyTracked, w).Tool {
			t.Errorf("row %d = %+v, want the tracked check's %q", i, unchecked[i], w)
		}
	}
}

func TestReplaceFailsAReplaceToAFolderAndAWorkspaceByName(t *testing.T) {
	t.Parallel()
	got := audit.CheckReplace(audit.ReplaceInput{Rules: rules(), Workspace: "go.work", Modules: []audit.Module{
		{Path: "example.test/lib", Replace: "../lib", Local: true},
	}})
	want := []string{
		"go.mod: supply.age.3 (gragas replace): module example.test/lib is replaced by the folder ../lib, so its age is not judged",
		"go.work: supply.age.3 (gragas replace): a go workspace builds its modules from their folders, so the age of what the build uses is not judged",
	}
	if len(got) != 2 || got[0].String() != want[0] || got[1].String() != want[1] {
		t.Fatalf("findings = %v", got)
	}
}

// TestReplacePassesAModuleReplacedByAnotherAndAPlainOne is the control: only a
// folder and a workspace are findings.
func TestReplacePassesAModuleReplacedByAnotherAndAPlainOne(t *testing.T) {
	t.Parallel()
	got := audit.CheckReplace(audit.ReplaceInput{Rules: rules(), Modules: []audit.Module{
		{Path: "example.test/fork/lib", Replace: "example.test/org/lib"},
		{Path: "example.test/plain"},
	}})
	if len(got) != 0 {
		t.Fatalf("findings = %v, want none", got)
	}
}

func lines(kind string, in ...audit.Counted) []audit.Counted {
	for i := range in {
		in[i].Kind = kind
	}
	return in
}

func TestCountOfReadsDirectivesSkipsAndExclusionRows(t *testing.T) {
	t.Parallel()
	// the planted text is built here, so this file holds none of the three
	nolint := "//" + "nolint:errcheck // a reason"
	skip := "t.Ski" + "p(\"later\")"
	got := audit.CountOf(audit.Doc{Path: "a/x_test.go", Text: "package a\n\nfunc F() { _ = g() } " + nolint + "\nfunc T(t *testing.T) {\n\t" + skip + "\n\t// t.Ski" + "p(\"in a comment\")\n}\n"}, configs())
	want := []audit.Counted{
		{Kind: kindDirective(), File: "a/x_test.go", Line: 3, Text: "func F() { _ = g() } " + nolint},
		{Kind: audit.KindSkip, File: "a/x_test.go", Line: 5, Text: skip},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := audit.CountOf(audit.Doc{Path: "a/x.go", Text: "package a\nvar _ = \"t.Ski" + "p(\"\n"}, configs()); len(got) != 0 {
		t.Fatalf("a Skip call counts only in a test file: %v", got)
	}
	cfg := "version: \"2\"\nlinters:\n  enable:\n    - errcheck\n  exclusions:\n    presets: []\n    rules:\n      - path: foo\n        linters:\n          - gosec\n      - text: bar\n    paths:\n      - vendor\nrun:\n  skip-dirs:\n    - old\n"
	rows := audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: cfg}, configs())
	var text []string
	for _, r := range rows {
		text = append(text, fmt.Sprintf("%d:%s:%s", r.Line, r.Kind, r.Text))
	}
	if fmt.Sprint(text) != "[8:linter exclusion:{'path': 'foo', 'linters': ['gosec']} 11:linter exclusion:{'text': 'bar'} 13:linter exclusion:vendor 16:linter exclusion:old]" {
		t.Fatalf("rows = %v", text)
	}
}

func TestCountsAreAFindingOnlyWhenTheyRiseAndShowTheNewLine(t *testing.T) {
	t.Parallel()
	base := lines(kindDirective(), audit.Counted{File: "a.go", Line: 3, Text: "x() //nolint:errcheck // old"})
	moved := lines(kindDirective(), audit.Counted{File: "b.go", Line: 40, Text: "x() //nolint:errcheck // old"})
	if got := audit.CheckCounts(rules(), base, moved); len(got) != 0 {
		t.Fatalf("a moved line is not a new one: %v", got)
	}
	if got := audit.CheckCounts(rules(), base, nil); len(got) != 0 {
		t.Fatalf("a removed line passes: %v", got)
	}
	grown := append(moved, lines(kindDirective(), audit.Counted{File: "c.go", Line: 7, Text: "y() //nolint:gosec // new"})...)
	got := audit.CheckCounts(rules(), base, grown)
	if len(got) != 1 || got[0].String() != "c.go:7: gates.exemptions.2 (gragas directive): is a new nolint directive (2 against 1 at the merge base): y() //nolint:gosec // new" {
		t.Fatalf("findings = %v", got)
	}
	// a swap of one for another keeps the count, so it is not a rise
	swapped := lines(audit.KindSkip, audit.Counted{File: "d_test.go", Line: 1, Text: "t.Skip(\"b\")"})
	if got := audit.CheckCounts(rules(), lines(audit.KindSkip, audit.Counted{File: "d_test.go", Line: 1, Text: "t.Skip(\"a\")"}), swapped); len(got) != 0 {
		t.Fatalf("an equal count passes: %v", got)
	}
	ex := audit.CheckCounts(rules(), nil, lines(audit.KindExclusion, audit.Counted{File: ".golangci.yml", Line: 9, Text: "path: foo"}))
	if len(ex) != 1 || !strings.HasPrefix(ex[0].String(), ".golangci.yml:9: gates.ratchets.1 (gragas debt): is a new linter exclusion") {
		t.Fatalf("findings = %v", ex)
	}
}

// TestAMentionOfADirectiveOrASkipCountsForNothing: the words nolint and
// .Skip( in a string, or in the middle of a comment, are not a directive or a
// skip; a comment that starts with nolint and a Skip call are. Planted
// failure: the count reads lines, so the three mentions below count.
func TestAMentionOfADirectiveOrASkipCountsForNothing(t *testing.T) {
	t.Parallel()
	text := "package p\n\n" +
		"// KindDirective is a //nolint directive.\n" +
		"var s = \"x() //nolint:errcheck\"\n" +
		"var r = `t.Skip(\"b\")`\n" +
		"func f() { g() //nolint:errcheck // real\n" +
		"\tt.Skip(\"why\")\n}\n"
	if got := audit.CountOf(audit.Doc{Path: "a.go", Text: text}, configs()); len(got) != 1 || got[0].Kind != kindDirective() || got[0].Line != 6 {
		t.Errorf("a non-test file counts %+v, want the one directive on line 6", got)
	}
	got := audit.CountOf(audit.Doc{Path: "a_test.go", Text: text}, configs())
	if len(got) != 2 || got[1].Kind != audit.KindSkip || got[1].Line != 7 {
		t.Errorf("a test file counts %+v, want the directive and the skip on line 7", got)
	}
}

// TestAnExclusionInFlowStyleIsRead plants an exclusion written in flow style,
// which the config's own reader takes as the block row it spells and the old
// reading by indentation did not see at all.
func TestAnExclusionInFlowStyleIsRead(t *testing.T) {
	t.Parallel()
	flow := audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: "version: \"2\"\nlinters:\n  exclusions:\n    paths: [vendor, gen]\n    rules: [{path: foo, linters: [gosec]}]\n"}, configs())
	block := audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: "version: \"2\"\nlinters:\n  exclusions:\n    paths:\n      - vendor\n      - gen\n    rules:\n      - path: foo\n        linters:\n          - gosec\n"}, configs())
	if len(flow) != 3 || len(block) != 3 {
		t.Fatalf("flow style gives %d rows and block style %d, want 3 each: %v %v", len(flow), len(block), flow, block)
	}
	texts := func(rows []audit.Counted) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.Text)
		}
		slices.Sort(out)
		return out
	}
	if !slices.Equal(texts(flow), texts(block)) {
		t.Errorf("flow style reads %q and block style %q", texts(flow), texts(block))
	}
}

// TestTwoEqualExclusionPathsEachHaveTheirOwnLine holds the lines of scalar
// items: two equal paths entries are two rows, each at its own line.
func TestTwoEqualExclusionPathsEachHaveTheirOwnLine(t *testing.T) {
	t.Parallel()
	rows := audit.CountOf(audit.Doc{Path: ".golangci.yml", Text: "version: \"2\"\nlinters:\n  exclusions:\n    paths:\n      - vendor\n      - vendor\n"}, configs())
	if len(rows) != 2 || rows[0].Line != 5 || rows[1].Line != 6 {
		t.Errorf("rows %v, want two at lines 5 and 6", rows)
	}
}
