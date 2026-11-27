package scan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/scan"
)

// cat joins parts, so a plant is made at run time and this file holds none of
// what the scan looks for.
func cat(parts ...string) string { return strings.Join(parts, "") }

// plants is one line of each rule, by the rule it is.
func plants() map[string]string {
	return map[string]string{
		scan.RuleTracker:  cat("see owner/repo", "#", "12", " for it\n"),
		scan.RuleDecision: cat("ruled by GD", "-", "7", "\n"),
		scan.RuleHome:     cat("cd /", "Users/someone/", "work\n"),
		scan.RuleEmail:    cat("write to someone", "@", "corp.test\n"),
	}
}

func files(kv ...string) map[string][]byte {
	out := map[string][]byte{}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = []byte(kv[i+1])
	}
	return out
}

// TestPublicTextFindsOneHitOfEachRuleAndNothingInACleanTree: each rule's plant
// alone is red, naming its file and line, and a clean tree is green.
func TestPublicTextFindsOneHitOfEachRuleAndNothingInACleanTree(t *testing.T) {
	t.Parallel()
	for rule, plant := range plants() {
		v := scan.JudgePublicText(files("a.md", "one\n"+plant), scan.TextBaseline{})
		if v.Total != 1 || v.ByRule[rule] != 1 {
			t.Errorf("%s: the plant counted %d hits, %v: want one of its rule", rule, v.Total, v.ByRule)
		}
		if len(v.Findings) != 2 || v.Findings[1] != "  a.md:2: "+rule {
			t.Errorf("%s: findings %q: want the file over its count and its line", rule, v.Findings)
		}
	}
	clean := files("a.md", "plain words\nwrite to alex"+"@"+"example.com\nand 1+"+"alex"+"@"+"users.noreply.github.com\n", "b.go", "package b\n")
	if v := scan.JudgePublicText(clean, scan.TextBaseline{}); v.Total != 0 || len(v.Findings) != 0 {
		t.Errorf("a clean tree: %d hits, findings %q: want none", v.Total, v.Findings)
	}
}

// TestPublicTextCountsEachWayAReferenceIsWritten: the forms of a tracker
// number and of a decision are each one hit.
func TestPublicTextCountsEachWayAReferenceIsWritten(t *testing.T) {
	t.Parallel()
	forms := []string{
		cat("owner/repo", "#", "5"),
		cat("repo", "#", "5"),
		cat("(", "#", "5", ")"),
		cat("see ", "#", "55", " here"),
		cat("PR ", "5"),
		cat("GD", "-", "5"),
		cat("PD", "-", "5"),
		cat("DL", "-", "5"),
		cat("decision ", "5"),
		cat("Decision ", "5"),
		cat("C:", `\Users\`, `someone\`),
		cat("/", "home/someone/"),
	}
	for _, f := range forms {
		if v := scan.JudgePublicText(files("a.md", f+"\n"), scan.TextBaseline{}); v.Total != 1 {
			t.Errorf("%q counted %d hits: want 1", f, v.Total)
		}
	}
}

// TestPublicTextNeverReadsAFileThatIsNotText: a NUL in the head of a file
// makes it data, which is not read.
func TestPublicTextNeverReadsAFileThatIsNotText(t *testing.T) {
	t.Parallel()
	if v := scan.JudgePublicText(files("a.bin", "\x00"+plants()[scan.RuleTracker]), scan.TextBaseline{}); v.Total != 0 {
		t.Errorf("a binary file counted %d hits: want none", v.Total)
	}
}

// TestPublicTextHoldsACountThatOnlyShrinks: a file at its count is green, one
// over it is red, one under it is green and is lowered, and a file with no
// count that gains a hit is red.
func TestPublicTextHoldsACountThatOnlyShrinks(t *testing.T) {
	t.Parallel()
	two := plants()[scan.RuleTracker] + plants()[scan.RuleDecision]
	base := scan.TextBaseline{Counts: map[string]int{"a.md": 2, "b.md": 2}}
	if v := scan.JudgePublicText(files("a.md", two, "b.md", two), base); len(v.Findings) != 0 || v.Fallen != 0 {
		t.Errorf("files at their count: findings %q, fallen %d: want none", v.Findings, v.Fallen)
	}
	v := scan.JudgePublicText(files("a.md", two+plants()[scan.RuleEmail], "b.md", two), base)
	if len(v.Findings) == 0 || !strings.HasPrefix(v.Findings[0], "a.md: 3 hits, over its count of 2") {
		t.Errorf("a rising count: findings %q: want a.md over its count", v.Findings)
	}
	if v.Lowered.Counts["a.md"] != 2 {
		t.Errorf("a rising count was lowered to %d: it must stay at 2, never rise", v.Lowered.Counts["a.md"])
	}
	v = scan.JudgePublicText(files("a.md", plants()[scan.RuleTracker], "b.md", two, "c.md", plants()[scan.RuleHome]), base)
	if len(v.Findings) == 0 || !strings.HasPrefix(v.Findings[0], "c.md: 1 hit, over its count of 0") {
		t.Errorf("a new file with a hit: findings %q: want c.md over a count of 0", v.Findings)
	}
	v = scan.JudgePublicText(files("a.md", plants()[scan.RuleTracker], "b.md", "clean\n"), base)
	if len(v.Findings) != 0 || v.Fallen != 2 || v.Lowered.Counts["a.md"] != 1 || len(v.Lowered.Counts) != 1 {
		t.Errorf("counts that fell: findings %q, fallen %d, lowered %v: want a.md at 1 and b.md gone", v.Findings, v.Fallen, v.Lowered.Counts)
	}
}

// TestPublicTextLeavesOutASkipByNameAndRefusesAStaleOne: a skipped file, or
// a file under a skipped folder, is not counted; a skip that names nothing
// is a finding.
func TestPublicTextLeavesOutASkipByNameAndRefusesAStaleOne(t *testing.T) {
	t.Parallel()
	base := scan.TextBaseline{Skips: []scan.TextSkip{{Path: "gen.json", Why: "rendered"}, {Path: "golden/", Why: "copy"}}}
	tree := files("gen.json", plants()[scan.RuleTracker], "golden/x.md", plants()[scan.RuleHome], "golden/y/z.md", plants()[scan.RuleEmail])
	if v := scan.JudgePublicText(tree, base); v.Total != 0 || len(v.Findings) != 0 {
		t.Errorf("skipped files: %d hits, findings %q: want none", v.Total, v.Findings)
	}
	v := scan.JudgePublicText(files("a.md", "clean\n"), base)
	if len(v.Findings) != 2 || !strings.Contains(v.Findings[0], "names no tracked file") {
		t.Errorf("skips of nothing: findings %q: want one finding for each", v.Findings)
	}
}

// TestPublicTextBaselineIsReadAndWrittenAsItIs: the count file reads and
// writes back the same, and a line it cannot read is refused with its number.
func TestPublicTextBaselineIsReadAndWrittenAsItIs(t *testing.T) {
	t.Parallel()
	text := "# note\nskip golden/ a copy of it\ncount b.md 2\ncount a.md 1\n"
	b, err := scan.ParseTextBaseline([]byte(text))
	if err != nil || len(b.Skips) != 1 || b.Counts["a.md"] != 1 || b.Counts["b.md"] != 2 {
		t.Fatalf("read %+v, %v", b, err)
	}
	again, err := scan.ParseTextBaseline(b.Format())
	if err != nil || again.Counts["b.md"] != 2 || again.Skips[0].Why != "a copy of it" {
		t.Errorf("read back %+v, %v", again, err)
	}
	for _, bad := range []string{"count a.md\n", "count a.md 0\n", "count a.md x\n", "skip a.md\n", "other a.md 1\n", "count a.md 1\ncount a.md 2\n"} {
		if _, err := scan.ParseTextBaseline([]byte(bad)); err == nil || !strings.Contains(err.Error(), "line") {
			t.Errorf("%q: error %v: want one that names a line", bad, err)
		}
	}
}

// machine is a Machine over files, with the list at hashes when it is not "".
func machine(files map[string][]byte, hashes string) scan.Machine {
	return scan.Machine{
		TrackedFiles: func(context.Context, string) (map[string][]byte, error) { return files, nil },
		NamesList: func() (string, []byte, error) {
			if hashes == "" {
				return "", nil, nil
			}
			return "/outside/list", []byte(hashes), nil
		},
		Root: "/tree",
	}
}

// TestPublicTextWithNoListSaysSoAndChecksPatternsOnly: no list is not an
// error; the output says the private words were not checked.
func TestPublicTextWithNoListSaysSoAndChecksPatternsOnly(t *testing.T) {
	t.Parallel()
	lines, _, err := scan.PublicText(t.Context(), machine(files("a.md", "word\n"), ""), scan.TextBaseline{})
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "private words not checked") {
		t.Errorf("no list: lines %q, error %v: want a line that says so, and no error", lines, err)
	}
}

// TestPublicTextMatchesAWordOfThePrivateListAndNeverPrintsIt: a word of the
// list in a file's text, or in its path, is red whatever the count, is named
// by file and line, and is printed nowhere, in the findings, the lines or
// the error.
func TestPublicTextMatchesAWordOfThePrivateListAndNeverPrintsIt(t *testing.T) {
	t.Parallel()
	word := cat("zorble", "flax")
	hashes, err := scan.NameHashes(word)
	if err != nil {
		t.Fatal(err)
	}
	list := strings.Join(hashes, "\n") + "\n"
	tree := files("notes.md", "fine\nthe "+word+" is here\n", word+"/x.md", "fine\n")
	lines, _, err := scan.PublicText(t.Context(), machine(tree, list), scan.TextBaseline{})
	var refusal *scan.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("a word of the list: error %v, want a refusal", err)
	}
	all := strings.Join(append(lines, refusal.Findings...), "\n") + err.Error()
	if strings.Contains(strings.ToLower(all), word) {
		t.Errorf("the word is printed: %s", all)
	}
	if !strings.Contains(all, "notes.md:2: a word of the private list") || !strings.Contains(all, "<name>/x.md: its path holds a word") {
		t.Errorf("findings %q: want the line of the text hit and the masked path", refusal.Findings)
	}
	if _, _, err := scan.PublicText(t.Context(), machine(files("notes.md", "fine\n"), list), scan.TextBaseline{}); err != nil {
		t.Errorf("a tree without the word: %v", err)
	}
}

// TestPublicTextRefusesAListInsideTheTreeAndAnEmptyTree.
func TestPublicTextRefusesAListInsideTheTreeAndAnEmptyTree(t *testing.T) {
	t.Parallel()
	if _, _, err := scan.PublicText(t.Context(), machine(files(), ""), scan.TextBaseline{}); !errors.Is(err, scan.ErrEmpty) {
		t.Errorf("an empty tree: %v, want ErrEmpty", err)
	}
	inside := machine(files("a.md", "word\n"), "anything\n")
	inside.Within = func(string, string) bool { return true }
	if _, _, err := scan.PublicText(t.Context(), inside, scan.TextBaseline{}); err == nil || !strings.Contains(err.Error(), "inside the tree") {
		t.Errorf("a list inside the tree: %v, want the refusal that says so", err)
	}
}
