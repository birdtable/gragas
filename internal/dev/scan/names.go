package scan

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
)

// NamesHashesVar names the forbidden-names list the names scan reads: a file
// outside the tree, one SHA-256 in hex a line, which gragas-dev names hash
// makes from a person's words (supply.secrets.4). The engine's names check
// reads GRAGAS_NAMES_FILE, regular expressions, which no hash can hold.
const NamesHashesVar = "GRAGAS_NAMES_HASHES"

// NamesBase is the ref a push is judged past: the commits HEAD holds that it
// does not are the ones a push publishes.
const NamesBase = "origin/main"

// MaxRun is the most words a name of the list holds, and the longest run of
// words the scan hashes.
const MaxRun = 4

// NamesList is the forbidden-names list: the SHA-256 of each name, its words
// folded to lower case and joined by one space.
type NamesList map[[digest.Size]byte]struct{}

// ReadNamesList reads the list: one SHA-256 in hex a line, and a blank line
// or one that starts with # passed over. Any other line is refused by its
// number alone, since it may hold a name in the clear, and a list of no hash
// is ErrEmpty.
func ReadNamesList(data []byte) (NamesList, error) {
	l := NamesList{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		b, err := hex.DecodeString(line)
		if err != nil || len(b) != digest.Size {
			return nil, fmt.Errorf("line %d of the list is not a SHA-256 in hex: make the list with gragas-dev names hash", i+1)
		}
		l[[digest.Size]byte(b)] = struct{}{}
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("%w: the list holds no hash", ErrEmpty)
	}
	return l, nil
}

// NameHashes is the list lines of one name as a person writes it: the
// SHA-256 of its words, folded and joined by one space, and, when it has more
// than one, of its words run together, so the scan finds the name written
// either way. A word is a run of letters, marks and digits, split where its case
// turns, as the scan splits text. A name of no word, or of more than MaxRun,
// is refused, and the refusal never holds the name.
func NameHashes(name string) ([]string, error) {
	ps, _ := words(name)
	switch {
	case len(ps) == 0:
		return nil, errors.New("a name holds no word")
	case len(ps) > MaxRun:
		return nil, fmt.Errorf("a name of %d words: the scan hashes runs of 1 to %d", len(ps), MaxRun)
	}
	folded := make([]string, len(ps))
	for i, p := range ps {
		folded[i] = p.word
	}
	out := []string{hashOf(strings.Join(folded, " "))}
	if len(ps) > 1 {
		out = append(out, hashOf(strings.Join(folded, "")))
	}
	return out, nil
}

// hashOf is the SHA-256 of s in lower-case hex.
func hashOf(s string) string {
	sum := digest.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// span is one word of a text, folded, and the bytes it lies on.
type span struct {
	word       string
	start, end int
}

// The patterns words splits text by: a run of letters, marks and digits;
// within it, a part, upper-case letters and then the rest; and a part's
// upper-case head, and the lower-case letter that may follow it.
var (
	wordRun   = regexp.MustCompile(`[\p{L}\p{M}\p{Nd}]+`)
	casePart  = regexp.MustCompile(`[\p{Lu}\p{Lt}]*[^\p{Lu}\p{Lt}]*`)
	upperHead = regexp.MustCompile(`^[\p{Lu}\p{Lt}]+`)
	lowerNext = regexp.MustCompile(`^\p{Ll}`)
)

// words is text's words, each folded to lower case: a word is a run of
// letters, marks and digits, split where its case turns (parts). A run that
// splits into more than one is also in wholes, so a name written run
// together is found as one word.
func words(text string) (ps, wholes []span) {
	for _, run := range wordRun.FindAllStringIndex(text, -1) {
		seg := parts(text, run[0], run[1])
		ps = append(ps, seg...)
		if len(seg) > 1 {
			wholes = append(wholes, span{strings.ToLower(text[run[0]:run[1]]), run[0], run[1]})
		}
	}
	return ps, wholes
}

// parts splits the run of letters, marks and digits at text[start:end]
// where its case turns: before an upper-case letter that follows a
// lower-case one or a digit, and before the last of two or more upper-case
// letters that a lower-case one follows, so HTTPServer is HTTP and Server.
func parts(text string, start, end int) []span {
	var out []span
	add := func(s, e int) { out = append(out, span{strings.ToLower(text[s:e]), s, e}) }
	for _, m := range casePart.FindAllStringIndex(text[start:end], -1) {
		s, e := start+m[0], start+m[1]
		if s == e {
			continue
		}
		head := upperHead.FindString(text[s:e])
		if rs := []rune(head); len(rs) > 1 && lowerNext.MatchString(text[s+len(head):e]) {
			at := s + len(head) - len(string(rs[len(rs)-1]))
			add(s, at)
			add(at, e)
			continue
		}
		add(s, e)
	}
	return out
}

// hit is one name of the list in a text: the bytes it lies on and how many
// words it holds.
type hit struct{ start, end, words int }

// holds is whether the list holds the hash of s.
func (l NamesList) holds(s string) bool {
	_, in := l[digest.Sum([]byte(s))]
	return in
}

// hits is each run of 1 to MaxRun words of text, and each word written run
// together, whose hash the list holds, each once.
func (l NamesList) hits(text string) []hit {
	ps, wholes := words(text)
	var out []hit
	for i := range ps {
		var run strings.Builder
		for k := 0; k < MaxRun && i+k < len(ps); k++ {
			if k > 0 {
				run.WriteByte(' ')
			}
			run.WriteString(ps[i+k].word)
			if l.holds(run.String()) {
				out = append(out, hit{ps[i].start, ps[i+k].end, k + 1})
			}
		}
	}
	for _, w := range wholes {
		if l.holds(w.word) {
			out = append(out, hit{w.start, w.end, 1})
		}
	}
	slices.SortFunc(out, func(a, b hit) int { return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.end, b.end)) })
	return slices.CompactFunc(out, func(a, b hit) bool { return a.start == b.start && a.end == b.end })
}

// masked is text with each hit written as <name>, so a finding says where a
// name is and never the name.
func masked(text string, hs []hit) string {
	var b strings.Builder
	at := 0
	for _, h := range hs {
		if h.start < at {
			at = max(at, h.end)
			continue
		}
		b.WriteString(text[at:h.start] + "<name>")
		at = h.end
	}
	b.WriteString(text[at:])
	return b.String()
}

// lineOf is the line, from 1, that the byte at offset lies on.
func lineOf(text string, offset int) int { return strings.Count(text[:offset], "\n") + 1 }

// noReply is the addresses a commit may carry: a GitHub account's no-reply
// address, with or without its id, a bot's, and the address GitHub's web
// merges commit with.
var noReply = regexp.MustCompile(`^(?:[0-9]+\+)?[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\[bot\])?@users\.noreply\.github\.com$|^noreply@github\.com$`)

// NoReply is whether addr is a no-reply address a public repository's
// commits may carry.
func NoReply(addr string) bool { return noReply.MatchString(addr) }

// Pushed is what a push of HEAD publishes past NamesBase, as the names scan
// reads it.
type Pushed struct {
	// Files is every file git tracks, as it is on disk, by its path.
	Files map[string][]byte
	// Branch is the branch HEAD is on, "" when it is on none.
	Branch string
	// Commits are the commits past NamesBase, newest first: none when
	// Rewrites holds a finding, as the history git would show is not the
	// one a push sends.
	Commits []Commit
	// Rewrites is a finding for each replace ref and grafts file of the
	// repository, and for a shallow clone, which make git show a history
	// other than its own.
	Rewrites []string
}

// Commit is one commit a push publishes.
type Commit struct {
	// ID is its hash; Author and Committer are their addresses.
	ID, Author, Committer string
	// Message is its whole message.
	Message string
	// Patch is what it changes, as git log -p prints it with patchArgs: a
	// name it adds and a later commit of the push removes is published all
	// the same, and is read here.
	Patch string
}

// JudgeNames is a finding for each rewrite of the history, for each
// name of the list in a tracked file, its path, the branch or a commit's
// message, and for each commit whose author or committer address is not a
// no-reply one. A finding never holds the name: a path that holds one is
// printed with it masked, and an address is never printed.
func JudgeNames(l NamesList, p Pushed) []string {
	found := append(slices.Clone(p.Rewrites), judgeFiles(l, p.Files)...)
	if len(l.hits(p.Branch)) > 0 {
		found = append(found, "the branch name holds a name on the list")
	}
	return append(found, judgeCommits(l, p.Commits)...)
}

// judgeFiles is the findings in the tracked files and their paths.
func judgeFiles(l NamesList, files map[string][]byte) []string {
	var found []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		inPath := l.hits(name)
		shown := masked(name, inPath)
		if len(inPath) > 0 {
			found = append(found, shown+": its path holds a name on the list")
		}
		body := string(files[name])
		for _, h := range l.hits(body) {
			found = append(found, fmt.Sprintf("%s:%d: a name of %d %s on the list", shown, lineOf(body, h.start), h.words, plural(h.words, "word", "words")))
		}
	}
	return found
}

// judgeCommits is the findings in the commits' messages, the lines and
// paths they add, and their addresses.
func judgeCommits(l NamesList, commits []Commit) []string {
	var found []string
	for _, c := range commits {
		id := c.ID[:min(len(c.ID), 12)]
		for _, h := range l.hits(c.Message) {
			found = append(found, fmt.Sprintf("commit %s: its message, line %d, holds a name of %d %s on the list", id, lineOf(c.Message, h.start), h.words, plural(h.words, "word", "words")))
		}
		found = append(found, judgeAdded(l, id, c.Patch)...)
		if !NoReply(c.Author) {
			found = append(found, fmt.Sprintf("commit %s: its author address is not a no-reply address", id))
		}
		if !NoReply(c.Committer) {
			found = append(found, fmt.Sprintf("commit %s: its committer address is not a no-reply address", id))
		}
	}
	return found
}

// judgeAdded is the findings in the paths and lines one commit's patch
// adds, masked as the files' are: a path it writes and a run of lines it
// adds, read across their line breaks, with the line in the file each lies
// on.
func judgeAdded(l NamesList, id, patch string) []string {
	paths, runs := parsePatch(patch)
	var found []string
	for _, p := range paths {
		if hs := l.hits(p); len(hs) > 0 {
			found = append(found, fmt.Sprintf("commit %s: it writes %s, whose path holds a name on the list", id, masked(p, hs)))
		}
	}
	for _, r := range runs {
		shown := masked(r.path, l.hits(r.path))
		for _, h := range l.hits(r.text) {
			found = append(found, fmt.Sprintf("commit %s: %s:%d, a line it adds, holds a name of %d %s on the list", id, shown, r.line+lineOf(r.text, h.start)-1, h.words, plural(h.words, "word", "words")))
		}
	}
	return found
}

// patchArgs are git's arguments for the patch of the commit id: what it
// adds, as text whatever the attributes say, with no textconv, external
// diff, rename, colour or relative path, against its first parent when it
// is a merge and against nothing when it is a root commit, with every
// submodule entry shown, and with the prefixes and the quoting of a path
// fixed, so no config of the repository's changes what the scan reads.
func patchArgs(id string) []string {
	return []string{"-c", "core.quotePath=false", "log", "-1", "-p", "--format=", "--text", "--no-textconv", "--no-ext-diff", "--no-renames", "--no-relative", "--no-color", "--diff-merges=first-parent", "--root", "--ignore-submodules=none", "--unified=0", "--src-prefix=a/", "--dst-prefix=b/", id}
}

// addedRun is one run of lines a patch adds to a file: the file's path, the
// line of the file the run starts on, and the lines.
type addedRun struct {
	path string
	line int
	text string
}

// hunkHead is a hunk's header, and the line of the new file it starts on.
var hunkHead = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

// parsePatch reads a patch patchArgs printed: the path each file it writes
// has, and each run of lines it adds. A file's header runs from its diff
// --git line to its first hunk, so a line it adds that looks like a header
// is read as a line.
func parsePatch(patch string) ([]string, []addedRun) {
	var r patchReader
	for line := range strings.SplitSeq(patch, "\n") {
		r.read(line)
	}
	r.flush()
	return r.paths, r.runs
}

// patchReader is parsePatch part way: what it has read, the file and the
// run it is in, whether it is in a file's header, and the line of the file
// the next added line lies on.
type patchReader struct {
	paths  []string
	runs   []addedRun
	cur    addedRun
	b      strings.Builder
	header bool
	next   int
}

// read reads one line of the patch.
func (r *patchReader) read(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		r.flush()
		r.header, r.cur = true, addedRun{}
	case strings.HasPrefix(line, "@@"):
		r.hunk(line)
	case r.header:
		if p, ok := patchPath(line); ok {
			r.cur.path = p
			r.paths = append(r.paths, p)
		}
	case strings.HasPrefix(line, "+"):
		r.add(line[1:])
	default:
		r.flush()
		if strings.HasPrefix(line, " ") {
			r.next++
		}
	}
}

// hunk starts a hunk at the line of the new file its header names.
func (r *patchReader) hunk(line string) {
	r.flush()
	r.header = false
	if m := hunkHead.FindStringSubmatch(line); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			r.next = n
		}
	}
}

// add adds one line to the run of added lines.
func (r *patchReader) add(text string) {
	if r.b.Len() == 0 {
		r.cur.line = r.next
	} else {
		r.b.WriteByte('\n')
	}
	r.b.WriteString(text)
	r.next++
}

// flush ends the run of added lines, if one is open.
func (r *patchReader) flush() {
	if r.b.Len() > 0 {
		r.cur.text = r.b.String()
		r.runs = append(r.runs, r.cur)
		r.b.Reset()
	}
}

// patchPath is the path a header line of a file names as the one written,
// unquoted when git quoted it: false for any other line, and for a file the
// commit removes.
func patchPath(line string) (string, bool) {
	p, ok := strings.CutPrefix(line, "+++ ")
	if !ok || p == "/dev/null" {
		return "", false
	}
	if strings.HasPrefix(p, `"`) {
		if u, err := strconv.Unquote(p); err == nil {
			p = u
		}
	}
	return strings.TrimPrefix(p, "b/"), true
}

// namesScan is the git a names scan runs.
type namesScan struct {
	m   Machine
	git string
}

// gitIn runs git in dir, the tree when "", reading no config of the host's
// and no replaced object, and answers its result.
func (s namesScan) gitIn(ctx context.Context, dir string, args ...string) (Result, error) {
	return s.m.Run(ctx, dir, s.m.GitEnv, s.git, args...)
}

// out runs git in dir and fails on a non-zero exit.
func (s namesScan) out(ctx context.Context, dir string, args ...string) (string, error) {
	r, err := s.gitIn(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	if r.Code != 0 {
		return "", fmt.Errorf("git %s exited %d: %s", subcommand(args), r.Code, firstLine(r.Stderr))
	}
	return string(r.Stdout), nil
}

// subcommand is git's command in args, past any -c setting before it.
func subcommand(args []string) string {
	for len(args) > 2 && args[0] == "-c" {
		args = args[2:]
	}
	return args[0]
}

// pushed reads what a push of HEAD in dir, the tree when "", publishes past
// NamesBase. It refuses a clone without NamesBase, and a log whose commits
// do not match git's own count of them. A replace ref, a grafts file or a
// shallow clone is a finding, and no commit is read.
func (s namesScan) pushed(ctx context.Context, dir string) (Pushed, error) {
	r, err := s.gitIn(ctx, dir, "rev-parse", "-q", "--verify", NamesBase+"^{commit}")
	if err != nil {
		return Pushed{}, err
	}
	if r.Code != 0 {
		return Pushed{}, fmt.Errorf("%s is not in this clone, so what a push publishes cannot be told: run git fetch origin", NamesBase)
	}
	var p Pushed
	if p.Branch, err = s.branch(ctx, dir); err != nil {
		return Pushed{}, err
	}
	if p.Rewrites, err = rewrites(ctx, s.m, s.git, dir); err != nil {
		return Pushed{}, err
	}
	if len(p.Rewrites) > 0 {
		p.Files, err = s.m.TrackedFiles(ctx, dir)
		return p, err
	}
	past := NamesBase + "..HEAD"
	count, err := s.out(ctx, dir, "rev-list", "--count", past)
	if err != nil {
		return Pushed{}, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(count))
	if err != nil {
		return Pushed{}, errors.New("git rev-list printed what is not a count of commits")
	}
	log, err := s.out(ctx, dir, "log", "-z", "--no-show-signature", "--no-color", "--format=%H%x1f%ae%x1f%ce%x1f%B", past)
	if err != nil {
		return Pushed{}, err
	}
	if p.Commits, err = parseLog(log); err != nil {
		return Pushed{}, err
	}
	if len(p.Commits) != n {
		return Pushed{}, fmt.Errorf("git log printed %d commits past %s, and git rev-list counts %d", len(p.Commits), NamesBase, n)
	}
	for i := range p.Commits {
		if p.Commits[i].Patch, err = s.out(ctx, dir, patchArgs(p.Commits[i].ID)...); err != nil {
			return Pushed{}, err
		}
	}
	if p.Files, err = s.m.TrackedFiles(ctx, dir); err != nil {
		return Pushed{}, err
	}
	return p, nil
}

// branch is the branch HEAD is on in dir, "" when it is on none.
func (s namesScan) branch(ctx context.Context, dir string) (string, error) {
	r, err := s.gitIn(ctx, dir, "symbolic-ref", "-q", "--short", "HEAD")
	switch {
	case err != nil:
		return "", err
	case r.Code == 1:
		return "", nil
	case r.Code != 0:
		return "", fmt.Errorf("git symbolic-ref exited %d: %s", r.Code, firstLine(r.Stderr))
	}
	return strings.TrimSpace(string(r.Stdout)), nil
}

// parseLog reads git log -z with the format pushed asks for: each commit's
// hash, author and committer addresses and message, the fields split by the
// unit separator and each commit ended by a NUL. A record it cannot read is
// refused without its text, which may hold a name.
func parseLog(out string) ([]Commit, error) {
	var cs []Commit
	for i, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\x1f", 4)
		if len(f) != 4 || (len(f[0]) != 40 && len(f[0]) != 64) || strings.Trim(f[0], "0123456789abcdef") != "" {
			return nil, fmt.Errorf("git log printed record %d in a shape the scan cannot read", i+1)
		}
		cs = append(cs, Commit{ID: f[0], Author: f[1], Committer: f[2], Message: f[3]})
	}
	return cs, nil
}

// readNamesInput reads the list NamesHashesVar names, which must be set and
// lie outside the tree. No message prints where the list lies: its path may
// hold a login, and a login may be on the list.
func readNamesInput(m Machine) (NamesList, error) {
	p, data, err := m.NamesList()
	switch {
	case err != nil:
		return nil, fmt.Errorf("names: the list %s names cannot be read: %w", NamesHashesVar, err)
	case p == "":
		return nil, fmt.Errorf("names: %s is not set, so no name can be checked: make the list with gragas-dev names hash, keep it outside the tree, and set %s to its path", NamesHashesVar, NamesHashesVar)
	}
	if m.Within != nil && m.Within(strings.ToLower(m.Root), strings.ToLower(p)) {
		return nil, fmt.Errorf("names: the list %s names is inside the tree, and it is kept outside it", NamesHashesVar)
	}
	l, err := ReadNamesList(data)
	if err != nil {
		return nil, fmt.Errorf("names: the list %s names: %w", NamesHashesVar, err)
	}
	return l, nil
}

// Names is the forbidden-name scan: every file git tracks and its path, the
// branch name, each commit message past NamesBase, and the lines and paths
// those commits add, against the hashed list NamesHashesVar names, and each
// of those commits' addresses against the no-reply ones (supply.secrets.4). It proves itself first on plant
// repositories, and fails when the list cannot be read. Nothing it prints
// holds a name, or the path of the list.
func Names(ctx context.Context, m Machine) ([]string, error) {
	l, err := readNamesInput(m)
	if err != nil {
		return nil, err
	}
	s := namesScan{m: m}
	if s.git, err = m.Git(); err != nil {
		return nil, fmt.Errorf("names: git is not on the PATH: %w", err)
	}
	lines := []string{fmt.Sprintf("names: %d %s in the list %s names", len(l), plural(len(l), "hash", "hashes"), NamesHashesVar)}
	proof, err := s.prove(ctx)
	lines = append(lines, proof...)
	if err != nil {
		return lines, err
	}
	pushed, err := s.pushed(ctx, "")
	if err != nil {
		return lines, fmt.Errorf("names: %w", err)
	}
	lines = append(lines, fmt.Sprintf("names: %d tracked %s, the branch, and %d %s past %s", len(pushed.Files), plural(len(pushed.Files), "file", "files"), len(pushed.Commits), plural(len(pushed.Commits), "commit", "commits"), NamesBase))
	if len(pushed.Files) == 0 {
		return lines, fmt.Errorf("names: %w: git tracks no file", ErrEmpty)
	}
	if found := JudgeNames(l, pushed); len(found) > 0 {
		return lines, &Refusal{Scan: "names", Findings: found, Home: NamesHashesVar + ", the list outside the tree that gragas-dev names hash makes",
			Fix: "take the name out of the file or its path, rename the branch, or reword the commit before it is pushed; for an address, set git config user.email to your no-reply address (<id>+<login>@users.noreply.github.com) and make the commit again"}
	}
	return append(lines, "names: no name on the list, and every address is a no-reply one"), nil
}

// The plants' stand-ins, from gate/standins.txt: the one person and the one
// host. The plants' list is made from them at run time, so no file holds a
// hash of anyone's name.
const (
	plantName    = "Alex"
	plantTwoWord = "Alex Box"
	plantNoReply = "1+alex@users.noreply.github.com"
	plantAddress = "alex@example.com"
)

// namesPlant is one plant repository on its branch: an empty first commit
// that NamesBase points at, or, when orphanBase is set, an empty commit off
// the branch, then one commit past it, a second when later is set, and
// the steps of then.
type namesPlant struct {
	// baseMessage is the first commit's message, on NamesBase.
	baseMessage string
	// orphanBase, when set, makes NamesBase a commit the branch does not
	// hold, so the branch's first commit is a root commit past it.
	orphanBase bool
	// branch, file, body, message and address make the commit past it.
	branch, file, body, message, address string
	// later, when set, is file's body in a second commit past NamesBase.
	later string
	// attributes, when set, is a .gitattributes the first commit past
	// NamesBase adds with file.
	attributes string
	// then is what is done last, in order.
	then []plantStep
}

// plantStep is one step of a plant: body written to file, a path under the
// plant's folder, or else git run with args. {HEAD} in body or an arg is
// HEAD's id.
type plantStep struct {
	file, body string
	args       []string
}

// gitStep is a plant step that runs git with args.
func gitStep(args ...string) plantStep { return plantStep{args: args} }

// plant makes p's repository in a new temp folder and returns the folder.
func (s namesScan) plant(ctx context.Context, p namesPlant) (string, error) {
	dir, err := s.m.Temp(ctx)
	if err != nil {
		return "", err
	}
	steps := []plantStep{gitStep("init", "--quiet", "--initial-branch="+p.branch, ".")}
	if p.orphanBase {
		if err := s.do(ctx, dir, steps[0]); err != nil {
			return "", err
		}
		if steps, err = s.orphan(ctx, dir, p.baseMessage); err != nil {
			return "", err
		}
	} else {
		steps = append(steps, gitStep("commit", "--quiet", "--no-verify", "--allow-empty", "-m", p.baseMessage), gitStep("update-ref", "refs/remotes/"+NamesBase, "HEAD"))
	}
	steps = append(steps, plantStep{file: p.file, body: p.body}, gitStep("add", p.file))
	if p.attributes != "" {
		steps = append(steps, plantStep{file: ".gitattributes", body: p.attributes}, gitStep("add", ".gitattributes"))
	}
	steps = append(steps, gitStep("-c", "user.email="+p.address, "commit", "--quiet", "--no-verify", "-m", p.message))
	if p.later != "" {
		steps = append(steps, plantStep{file: p.file, body: p.later}, gitStep("add", p.file), gitStep("commit", "--quiet", "--no-verify", "-m", "tidy the note"))
	}
	for _, step := range append(steps, p.then...) {
		if err := s.do(ctx, dir, step); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// orphan makes NamesBase in dir an empty commit of message that no branch
// holds, and answers no step more.
func (s namesScan) orphan(ctx context.Context, dir, message string) ([]plantStep, error) {
	tree, err := s.out(ctx, dir, "write-tree")
	if err != nil {
		return nil, err
	}
	base, err := s.out(ctx, dir, plantGit("-c", "user.email="+plantNoReply, "commit-tree", strings.TrimSpace(tree), "-m", message)...)
	if err != nil {
		return nil, err
	}
	return nil, s.do(ctx, dir, gitStep("update-ref", "refs/remotes/"+NamesBase, strings.TrimSpace(base)))
}

// do is one plant step in dir, as the stand-in with a no-reply address.
func (s namesScan) do(ctx context.Context, dir string, step plantStep) error {
	head := func(v string) (string, error) {
		if !strings.Contains(v, "{HEAD}") {
			return v, nil
		}
		id, err := s.out(ctx, dir, "rev-parse", "HEAD")
		return strings.ReplaceAll(v, "{HEAD}", strings.TrimSpace(id)), err
	}
	if step.file != "" {
		body, err := head(step.body)
		if err != nil {
			return err
		}
		return s.m.Write(path.Join(dir, step.file), []byte(body))
	}
	args := slices.Clone(step.args)
	for i := range args {
		var err error
		if args[i], err = head(args[i]); err != nil {
			return err
		}
	}
	_, err := s.out(ctx, dir, plantGit(append([]string{"-c", "user.email=" + plantNoReply}, args...)...)...)
	return err
}

// prove runs the plants and the clean control, each read as the tree is,
// against a list made from the stand-ins. The control must read its one
// file, its branch and its one commit, so a host or a git that reads
// nothing fails the proof.
func (s namesScan) prove(ctx context.Context) ([]string, error) {
	var hashes []string
	for _, name := range []string{plantName, plantTwoWord} {
		hs, err := NameHashes(name)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, hs...)
	}
	l, err := ReadNamesList([]byte(strings.Join(hashes, "\n")))
	if err != nil {
		return nil, err
	}
	clean := namesPlant{baseMessage: "start", branch: "work", file: "notes.txt", body: "a clean note\n", message: "add a note", address: plantNoReply}
	with := func(edit func(*namesPlant)) namesPlant {
		p := clean
		edit(&p)
		return p
	}
	named := "met " + plantName + " today\n"
	judge := func(p namesPlant, control bool) func(context.Context) (bool, error) {
		return func(ctx context.Context) (bool, error) {
			dir, err := s.plant(ctx, p)
			if err != nil {
				return false, err
			}
			pushed, err := s.pushed(ctx, dir)
			if err != nil {
				return false, err
			}
			if control && (len(pushed.Files) != 1 || pushed.Branch != p.branch || len(pushed.Commits) != 1) {
				return false, fmt.Errorf("the control read %d files, branch %q and %d commits, and it holds 1, %q and 1", len(pushed.Files), pushed.Branch, len(pushed.Commits), p.branch)
			}
			return len(JudgeNames(l, pushed)) > 0, nil
		}
	}
	return Prove(ctx, "names",
		Case{Name: "a tracked file that holds the planted name is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.body = "met " + plantName + " today\n" }), false)},
		Case{Name: "a tracked file whose two-word name breaks across a line is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.body = "met Alex\nBox today\n" }), false)},
		Case{Name: "a tracked file whose path holds the name run together is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.file = "AlexBox.txt" }), false)},
		Case{Name: "a branch named for the planted name is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.branch = "fix-alex" }), false)},
		Case{Name: "a commit message past " + NamesBase + " that holds the planted name is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.message = "thanks " + plantName }), false)},
		Case{Name: "a name a commit past " + NamesBase + " adds and a later one removes is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.body, p.later = named, clean.body }), false)},
		Case{Name: "that name in a file its attributes mark binary is red", Red: true, Judge: judge(with(func(p *namesPlant) {
			p.body, p.later, p.attributes = named, clean.body, "*.txt binary\n"
		}), false)},
		Case{Name: "that name in a file a textconv driver of its attributes would print as nothing is red", Red: true, Judge: judge(with(func(p *namesPlant) {
			p.body, p.later, p.attributes = named, clean.body, "*.txt diff=plant\n"
			p.then = []plantStep{gitStep("config", "diff.plant.textconv", "true")}
		}), false)},
		Case{Name: "that name in a root commit past " + NamesBase + ", though the repository's config sets log.showRoot false, is red", Red: true, Judge: judge(with(func(p *namesPlant) {
			p.body, p.later, p.orphanBase = named, clean.body, true
			p.then = []plantStep{gitStep("config", "log.showRoot", "false")}
		}), false)},
		Case{Name: "that name only a merge's own lines add is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.then = evilMergeSteps(named) }), false)},
		Case{Name: "that name as the path of a submodule entry a commit adds and a later one removes, though the repository's config sets diff.ignoreSubmodules all, is red", Red: true, Judge: judge(with(func(p *namesPlant) {
			p.then = []plantStep{
				gitStep("config", "diff.ignoreSubmodules", "all"),
				gitStep("update-index", "--add", "--cacheinfo", "160000,{HEAD},AlexBox"), gitStep("commit", "--quiet", "--no-verify", "-m", "add a module"),
				gitStep("update-index", "--force-remove", "AlexBox"), gitStep("commit", "--quiet", "--no-verify", "-m", "drop the module"),
			}
		}), false)},
		Case{Name: "a commit past " + NamesBase + " whose address is not a no-reply one is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.address = plantAddress }), false)},
		Case{Name: "a clean repository that holds a replace ref is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.then = []plantStep{gitStep("replace", "HEAD", "HEAD~1")} }), false)},
		Case{Name: "a clean repository whose git folder holds a grafts file is red", Red: true, Judge: judge(with(func(p *namesPlant) { p.then = []plantStep{{file: ".git/info/grafts", body: "{HEAD}\n"}} }), false)},
		Case{Name: "a name a commit past " + NamesBase + " adds and a later one removes, in a clone made shallow at the later one, is red", Red: true, Judge: judge(with(func(p *namesPlant) {
			p.body, p.later = named, clean.body
			p.then = []plantStep{{file: ".git/shallow", body: "{HEAD}\n"}}
		}), false)},
		Case{Name: "the same name in a message already on " + NamesBase + " is green", Judge: judge(with(func(p *namesPlant) { p.baseMessage = "thanks " + plantName }), false)},
		Case{Name: "a clean repository, read whole, is green", Judge: judge(clean, true)},
	)
}

// evilMergeSteps is the steps of a plant whose name only a merge's own lines
// add: neither side holds it, the merge resolves it in, and a later commit
// takes it out.
func evilMergeSteps(named string) []plantStep {
	commit := func(m string) plantStep { return gitStep("commit", "--quiet", "--no-verify", "-m", m) }
	return []plantStep{
		gitStep("checkout", "--quiet", "-b", "side"), {file: "side.txt", body: "a side\n"}, gitStep("add", "side.txt"), commit("side"),
		gitStep("checkout", "--quiet", "-"), {file: "main.txt", body: "a main\n"}, gitStep("add", "main.txt"), commit("main"),
		gitStep("merge", "--quiet", "--no-ff", "--no-commit", "side"), {file: "merged.txt", body: named}, gitStep("add", "merged.txt"), commit("merge"),
		gitStep("rm", "--quiet", "merged.txt"), commit("tidy"),
	}
}
