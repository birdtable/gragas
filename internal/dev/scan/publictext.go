package scan

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The public-text scan holds the tracked tree to the repository's rules for
// what a public file says: no number from a tracker, no decision or plan id,
// no home folder of a person, no address of a person, and none of the words
// of the private list. It counts what it finds in each file against a
// checked-in count that only shrinks: a hit that is new, or a file that has
// more than its count, fails, and a count is lowered by the command's apply
// flag and never raised by it.

// PublicTextFile is the checked-in count, from the repository root.
const PublicTextFile = "gate/public-text.txt"

// The rules a hit is of.
const (
	RuleTracker  = "tracker-number"
	RuleDecision = "decision-id"
	RuleHome     = "home-path"
	RuleEmail    = "email"
	// RulePrivate is a word of the hashed private list. It is never counted
	// against a baseline and never printed: a hit is a file and a line.
	RulePrivate = "private-word"
)

// PublicTextRules lists the pattern rules in the order a summary prints them.
func PublicTextRules() []string {
	return []string{RuleTracker, RuleDecision, RuleHome, RuleEmail}
}

// textRule is one pattern rule: the cheap substrings one of which a file must
// hold before the expression is run on it, and the expression.
type textRule struct {
	name     string
	triggers []string
	re       *regexp.Regexp
	// keep says whether a match is a hit; nil keeps every match.
	keep func(match []byte) bool
}

// newTextRules compiles the pattern rules, once for each scan.
func newTextRules() []textRule {
	return []textRule{
		{
			name:     RuleTracker,
			triggers: []string{"#", "PR "},
			re: regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)?#[0-9]+` +
				`|\(#[0-9]+\)` +
				"|(?:^|[\\s(\\[,;:\"'`])#[0-9]{1,5}\\b" +
				`|\bPR [0-9]+\b`),
		},
		{
			name:     RuleDecision,
			triggers: []string{"GD-", "PD-", "DL-", "ecision "},
			re:       regexp.MustCompile(`\b(?:GD|PD|DL)-[0-9]+\b|\b[Dd]ecision [0-9]+\b`),
		},
		{
			name:     RuleHome,
			triggers: []string{"/" + "Users/", "/" + "home/", `\Users\`},
			re:       regexp.MustCompile(`[/]Users[/][A-Za-z0-9._-]+[/]|[/]home[/][a-z][a-z0-9._-]*[/]|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+\\`),
		},
		{
			name:     RuleEmail,
			triggers: []string{"@"},
			re:       regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`),
			keep:     func(m []byte) bool { return !mailAllowed(string(m)) },
		},
	}
}

// mailAllowed is whether addr is one a public file may hold: one at
// example.com, or a GitHub no-reply address.
func mailAllowed(addr string) bool {
	return strings.HasSuffix(strings.ToLower(addr), "@example.com") || NoReply(addr)
}

// TextHit is one hit of a rule: a file, a line from 1 and the rule.
type TextHit struct {
	Path string
	Line int
	Rule string
}

// binary is whether data is not text: a NUL in its first 8000 bytes, as git
// judges it.
func binary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0
}

// CountPublicText finds every hit of the pattern rules in files, in order of
// path and line. A file that is not text is not read.
func CountPublicText(files map[string][]byte) []TextHit {
	var hits []TextHit
	rules := newTextRules()
	for _, path := range sortedKeys(files) {
		data := files[path]
		if binary(data) {
			continue
		}
		hits = append(hits, fileHits(path, data, rules)...)
	}
	return hits
}

func sortedKeys(files map[string][]byte) []string {
	return slices.Sorted(maps.Keys(files))
}

// fileHits is the hits in one text file. A rule is run only when the file
// holds one of its substrings, which is almost never for most of the tree.
func fileHits(path string, data []byte, rules []textRule) []TextHit {
	var hits []TextHit
	var starts []int // where each line begins, made when a rule first needs it
	for _, r := range rules {
		lines := triggerLines(data, r.triggers, func() []int {
			if starts == nil {
				starts = lineStarts(data)
			}
			return starts
		})
		for _, n := range lines {
			end := len(data)
			if n+1 < len(starts) {
				end = starts[n+1]
			}
			line := data[starts[n]:end]
			for _, at := range r.re.FindAllIndex(line, -1) {
				if r.keep == nil || r.keep(line[at[0]:at[1]]) {
					hits = append(hits, TextHit{Path: path, Line: n + 1, Rule: r.name})
				}
			}
		}
	}
	slices.SortStableFunc(hits, func(a, b TextHit) int { return cmp.Compare(a.Line, b.Line) })
	return hits
}

// triggerLines is the lines of data, from 0 and each once, that hold one of
// the triggers: no other line is read by a rule's expression. starts gives
// the line offsets, made when the first trigger is found.
func triggerLines(data []byte, triggers []string, starts func() []int) []int {
	var lines []int
	for _, t := range triggers {
		for off := 0; ; {
			i := bytes.Index(data[off:], []byte(t))
			if i < 0 {
				break
			}
			at, _ := slices.BinarySearch(starts(), off+i+1)
			lines = append(lines, at-1)
			off += i + len(t)
		}
	}
	slices.Sort(lines)
	return slices.Compact(lines)
}

// lineStarts is the offset each line of data begins at.
func lineStarts(data []byte) []int {
	starts := []int{0}
	for off := 0; ; {
		i := bytes.IndexByte(data[off:], '\n')
		if i < 0 || off+i+1 >= len(data) {
			return starts
		}
		off += i + 1
		starts = append(starts, off)
	}
}

// TextSkip is a file, or a folder when Path ends in a slash, left out of the
// count, and why.
type TextSkip struct{ Path, Why string }

// TextBaseline is the checked-in count: what is left out by name, and how
// many hits each other file may hold.
type TextBaseline struct {
	Skips  []TextSkip
	Counts map[string]int
}

func (b TextBaseline) skipped(path string) bool {
	for _, s := range b.Skips {
		if path == s.Path || strings.HasSuffix(s.Path, "/") && strings.HasPrefix(path, s.Path) {
			return true
		}
	}
	return false
}

// ParseTextBaseline reads the count file: a line is blank, a comment that
// starts with #, "skip <path> <why>", or "count <path> <n>".
func ParseTextBaseline(data []byte) (TextBaseline, error) {
	b := TextBaseline{Counts: map[string]int{}}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		switch {
		case f[0] == "skip" && len(f) >= 3:
			b.Skips = append(b.Skips, TextSkip{Path: f[1], Why: strings.Join(f[2:], " ")})
		case f[0] == "count" && len(f) == 3:
			n, err := strconv.Atoi(f[2])
			if err != nil || n < 1 {
				return b, fmt.Errorf("%s line %d: the count is a whole number of 1 or more", PublicTextFile, i+1)
			}
			if _, twice := b.Counts[f[1]]; twice {
				return b, fmt.Errorf("%s line %d: %s is counted twice", PublicTextFile, i+1, f[1])
			}
			b.Counts[f[1]] = n
		default:
			return b, fmt.Errorf("%s line %d: a line is \"skip <path> <why>\" or \"count <path> <n>\"", PublicTextFile, i+1)
		}
	}
	return b, nil
}

// Format is the count file as it is written: the header, each skip by path
// and each count by path.
func (b TextBaseline) Format() []byte {
	var out strings.Builder
	out.WriteString("# The public-text count: what each tracked file may still hold of what a public file\n" +
		"# does not say. It only shrinks: a file over its count fails the scan, and the scan's apply\n" +
		"# flag lowers a count and never raises one.\n" +
		"#\n" +
		"# skip <path> <why>   a generated file or a golden copy, left out by name; a path ending in / is a folder\n" +
		"# count <path> <n>    the most hits the file may hold\n")
	skips := slices.Clone(b.Skips)
	slices.SortFunc(skips, func(a, c TextSkip) int { return cmp.Compare(a.Path, c.Path) })
	for _, s := range skips {
		fmt.Fprintf(&out, "skip %s %s\n", s.Path, s.Why)
	}
	for _, k := range slices.Sorted(maps.Keys(b.Counts)) {
		fmt.Fprintf(&out, "count %s %d\n", k, b.Counts[k])
	}
	return []byte(out.String())
}

// TextVerdict is what the count says of a tree.
type TextVerdict struct {
	// Findings is a line for each file over its count, each hit of it, and
	// each stale row.
	Findings []string
	// Lowered is the baseline with every count that has fallen lowered, and
	// a file with no hit left removed. It never holds a raise.
	Lowered TextBaseline
	// Fallen is how many files are under their count.
	Fallen int
	// Total is the hits counted, and ByRule and ByFolder split them.
	Total    int
	ByRule   map[string]int
	ByFolder map[string]int
	Files    int
	// Today is the hits each counted file holds now, and Hits every one.
	Today map[string]int
	Hits  []TextHit
}

// JudgePublicText counts hits against base. Files in base's skips are left
// out, and a skip that names nothing tracked is a finding, so the list does
// not outlive what it leaves out.
func JudgePublicText(files map[string][]byte, base TextBaseline) TextVerdict {
	v := TextVerdict{Lowered: TextBaseline{Skips: base.Skips, Counts: map[string]int{}}, ByRule: map[string]int{}, ByFolder: map[string]int{}}
	counted := map[string][]byte{}
	for p, d := range files {
		if !base.skipped(p) {
			counted[p] = d
		}
	}
	v.Findings = staleSkips(files, base)
	byFile := map[string][]TextHit{}
	v.Today = map[string]int{}
	v.Hits = CountPublicText(counted)
	for _, h := range v.Hits {
		byFile[h.Path] = append(byFile[h.Path], h)
		v.Today[h.Path]++
		v.ByRule[h.Rule]++
		v.ByFolder[topFolder(h.Path)]++
		v.Total++
	}
	v.Files = len(byFile)
	for _, p := range sortedKeys(counted) {
		v.judgeFile(p, byFile[p], base.Counts[p])
	}
	for p := range base.Counts {
		if _, tracked := files[p]; !tracked {
			v.Fallen++
		}
	}
	return v
}

// staleSkips is a finding for each skip row that names no tracked file.
func staleSkips(files map[string][]byte, base TextBaseline) []string {
	var out []string
	for _, s := range base.Skips {
		used := false
		for p := range files {
			if p == s.Path || strings.HasSuffix(s.Path, "/") && strings.HasPrefix(p, s.Path) {
				used = true
				break
			}
		}
		if !used {
			out = append(out, fmt.Sprintf("%s: the skip row names no tracked file: take it out", s.Path))
		}
	}
	return out
}

// judgeFile sets one file's hits against its count c: over it is a finding
// with each hit, under it is a count fallen, and the lowered count never
// holds a raise.
func (v *TextVerdict) judgeFile(p string, hits []TextHit, c int) {
	n := len(hits)
	switch {
	case n > c:
		v.Findings = append(v.Findings, fmt.Sprintf("%s: %d %s, over its count of %d", p, n, plural(n, "hit", "hits"), c))
		for _, h := range hits {
			v.Findings = append(v.Findings, fmt.Sprintf("  %s:%d: %s", p, h.Line, h.Rule))
		}
		if c > 0 {
			v.Lowered.Counts[p] = c
		}
	case n > 0:
		v.Lowered.Counts[p] = n
		if n < c {
			v.Fallen++
		}
	case c > 0:
		v.Fallen++
	}
}

// topFolder is the first part of a path, or "." for a file at the root.
func topFolder(path string) string {
	if top, _, ok := strings.Cut(path, "/"); ok {
		return top
	}
	return "."
}

// summaryLines is the totals a run prints: per rule, then per top folder.
func (v TextVerdict) summaryLines() []string {
	var rules []string
	for _, r := range PublicTextRules() {
		rules = append(rules, fmt.Sprintf("%s %d", r, v.ByRule[r]))
	}
	folders := make([]string, 0, len(v.ByFolder))
	for f := range v.ByFolder {
		folders = append(folders, f)
	}
	slices.SortFunc(folders, func(a, b string) int { return cmp.Or(cmp.Compare(v.ByFolder[b], v.ByFolder[a]), cmp.Compare(a, b)) })
	var parts []string
	for _, f := range folders {
		parts = append(parts, fmt.Sprintf("%s %d", f, v.ByFolder[f]))
	}
	return []string{
		fmt.Sprintf("public-text: %d %s in %d %s, by rule: %s", v.Total, plural(v.Total, "hit", "hits"), v.Files, plural(v.Files, "file", "files"), strings.Join(rules, ", ")),
		"public-text: by folder: " + strings.Join(parts, ", "),
	}
}

// PublicText is the scan: every tracked file against the count in base, and
// against the hashed private list when one is named. It returns the lines to
// print and the verdict; the error is a Refusal when a count is over, a word
// of the list is in a file, or a row is stale. Nothing it prints holds a word
// of the list.
func PublicText(ctx context.Context, m Machine, base TextBaseline) ([]string, TextVerdict, error) {
	files, err := m.TrackedFiles(ctx, "")
	if err != nil {
		return nil, TextVerdict{}, fmt.Errorf("public-text: %w", err)
	}
	if len(files) == 0 {
		return nil, TextVerdict{}, fmt.Errorf("public-text: %w: git tracks no file", ErrEmpty)
	}
	v := JudgePublicText(files, base)
	lines := v.summaryLines()
	found := slices.Clone(v.Findings)
	list, say, err := privateList(m)
	if err != nil {
		return lines, v, err
	}
	lines = append(lines, say)
	if list != nil {
		for _, p := range sortedKeys(files) {
			found = append(found, privateHits(list, p, files[p])...)
		}
	}
	if len(found) > 0 {
		return lines, v, &Refusal{Scan: "public-text", Findings: found, Home: PublicTextFile + ", and gragas-dev scan public-text",
			Fix: "say the thing in words without the number, id, path or address; a count is lowered with --apply and never raised"}
	}
	if v.Fallen > 0 {
		lines = append(lines, fmt.Sprintf("public-text: %d %s under the count: run with --apply to lower it", v.Fallen, plural(v.Fallen, "file is", "files are")))
	}
	return lines, v, nil
}

// privateList reads the hashed list when one is named, and says what was
// checked. With none it says so and the scan has checked patterns only.
func privateList(m Machine) (NamesList, string, error) {
	p, data, err := m.NamesList()
	switch {
	case err != nil:
		return nil, "", fmt.Errorf("public-text: the list %s names cannot be read: %w", NamesHashesVar, err)
	case p == "":
		return nil, "public-text: private words not checked, as " + NamesHashesVar + " is not set; patterns only", nil
	}
	if m.Within != nil && m.Within(strings.ToLower(m.Root), strings.ToLower(p)) {
		return nil, "", fmt.Errorf("public-text: the list %s names is inside the tree, and it is kept outside it", NamesHashesVar)
	}
	l, err := ReadNamesList(data)
	if err != nil {
		return nil, "", fmt.Errorf("public-text: the list %s names: %w", NamesHashesVar, err)
	}
	return l, fmt.Sprintf("public-text: private words checked against %d %s", len(l), plural(len(l), "hash", "hashes")), nil
}

// privateHits is a finding for each word of the list in a file's text, and
// one if its path holds one. A finding is the file and the line, the path
// masked, and never the word.
func privateHits(l NamesList, path string, data []byte) []string {
	var out []string
	shown := path
	if hs := l.hits(path); len(hs) > 0 {
		shown = masked(path, hs)
		out = append(out, shown+": its path holds a word of the private list")
	}
	if binary(data) {
		return out
	}
	text := string(data)
	for _, h := range l.hits(text) {
		out = append(out, fmt.Sprintf("%s:%d: a word of the private list", shown, lineOf(text, h.start)))
	}
	return out
}
