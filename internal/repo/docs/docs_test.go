package docs

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/go-proc/gitenv/gittest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/repo/gatetest"
)

// The documents of docs/ held to docs.freshness, docs.links, docs.voice and
// arch.decisions: each has typed front matter, every link, rule id, S or A number and path
// resolves, and the packages the architecture names are the packages
// layers.yaml holds or plans. Every check is a function of a docWorld read
// once from the tree, so each is also run against a world with a breach
// planted in it, to show that it fires. Offline: nothing here reads a
// network.

// docWorld is the documents and what they are held against.
type docWorld struct {
	// docs are the Markdown files under docs/, by path from the root.
	docs map[string]string
	// pages are the static pages under docs/explorer-boards/canvas (the
	// .dc.html files), by path from the root; their relative links are held
	// like a document's (pages.cycle.1).
	pages map[string]string
	// exists says whether a path from the root is a file or a folder.
	exists func(rel string) bool
	// rules are the ids of the rulebook; parts the short names that start one.
	rules map[string]bool
	parts []string
	// planned are the packages layers.yaml plans, with the milestone.
	planned map[string]string
	// prose are the other files whose words are held to docs.freshness.4:
	// AGENTS.md and CLAUDE.md whole, the comments of the Makefile and of
	// gate/*.yaml. stays are the lines of gate/prose-durations.txt.
	prose map[string]string
	stays []string
}

var (
	frontKey  = regexp.MustCompile(`^([a-z-]+): ?(.*)$`)
	asOfDate  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	mdLink    = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	backtick  = regexp.MustCompile("`([^`\n]+)`")
	pathLike  = regexp.MustCompile(`^[A-Za-z0-9_.\-/]+$`)
	pathExt   = regexp.MustCompile(`\.(md|yaml|yml|json|go|ts|mjs|html|txt|lock|pub|jsonl)$`)
	createdIn = regexp.MustCompile("^\\| `([^`]+)` \\| [^|]+ \\| (?:created in (M(?:0[ab]|[1-9]|10))|(retired)\\b[^|]*) \\|$")
	headingRe = regexp.MustCompile(`^(#{1,6}) +(.+?) *$`)
	specDef   = regexp.MustCompile(`(?m)^\*\*S(\d+) `)
	sRef      = regexp.MustCompile(`\bS(\d+)\b`)
)

// readDocs is every Markdown file under docs/, by path from the root.
func readDocs() (map[string]string, error) {
	out := map[string]string{}
	tree := os.DirFS(root)
	err := fs.WalkDir(tree, "docs", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		out[p] = string(b)
		return err
	})
	return out, err
}

// readDocWorld is the repository's docs as the checks read them.
func readDocWorld(t *testing.T) docWorld {
	t.Helper()
	docs, err := readDocs()
	if err != nil {
		t.Fatal(err)
	}
	rules, err := gate.ReadRules(gatetest.Of(t, root).Rulebook)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := readPages()
	if err != nil {
		t.Fatal(err)
	}
	w := docWorld{docs: docs, pages: pages, rules: map[string]bool{}, planned: map[string]string{}}
	for _, r := range rules {
		w.rules[r.ID] = true
		if !slices.Contains(w.parts, r.Part) {
			w.parts = append(w.parts, r.Part)
		}
	}
	y, err := os.ReadFile(filepath.Join(root, "layers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := layers.Parse(y)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range tb.Planned {
		for _, p := range pl.Packages {
			w.planned[p] = pl.Created
		}
	}
	w.prose, w.stays = readProse(t)
	w.exists = existsIn(t, root)
	return w
}

// existsIn says whether a path from dir is a file or a folder, or is output
// the repository's .gitignore keeps out of a clone (times/measure.json, which
// the measure member writes; tools/.node/node_modules, which the tool install
// makes), so a fresh clone or an export reads the docs as a used tree. An
// ignore line that covers a file git tracks excuses nothing: such a line,
// docs/ say, names no tool's output, and excusing it would let a change
// ignore its way past a deleted doc with no verdict, since .gitignore is not
// protected.
func existsIn(t *testing.T, dir string) func(rel string) bool {
	t.Helper()
	var ig ignores
	if b, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		ig = readIgnores(string(b))
	}
	// Listed here, on the asking test's own t, not inside a sync.OnceValue: a Fatalf
	// there is a Goexit the once turns into panic(nil), which aborts the whole package.
	clean := ig.untracked(trackedIn(t, dir))
	common := commonGitDir(t, dir)
	return func(rel string) bool {
		// .git is never part of a tree: an export has none, a clone has it.
		if rel == ".git" {
			return true
		}
		// In a worktree .git is a file, and what a path below it names is in the common git dir.
		if below, ok := strings.CutPrefix(rel, ".git/"); ok && common != "" {
			_, err := os.Stat(filepath.Join(common, filepath.FromSlash(below)))
			return err == nil
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			return true
		}
		return clean.covers(rel)
	}
}

// trackedIn is every file git tracks in dir, slash-separated, as git
// ls-files lists them, a file deleted from disk but not from the index
// included. A tree with no .git, an export, holds only tracked files, so
// there every file on disk is one, which can only refuse more. A listing that
// fails ends the test: the excuse is never granted on a guess.
func trackedIn(t *testing.T, dir string) []string {
	t.Helper()
	out, err := listTracked(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// listTracked is trackedIn without a test: the error is returned, so the caller fails
// on its own t.
func listTracked(ctx context.Context, dir string) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
		cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
		cmd.Dir = dir
		cmd.Env = gitenv.Env(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("list the files git tracks in %s: %w", dir, err)
		}
		return strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }), nil
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		out = append(out, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list the files of %s: %w", dir, err)
	}
	return out, nil
}

// ignores are the folders a .gitignore names whole: roots from the top of the
// tree (/times/, /dist/) and names at any depth (node_modules/), as git reads them.
type ignores struct{ roots, names []string }

// readIgnores keeps the lines that name one folder or file with no wildcard:
// a leading slash roots it, a bare name with a trailing slash matches at any
// depth. A wildcard, a negation, a comment and a pattern with a slash inside
// are not read, so they never make a missing path count as there.
func readIgnores(gitignore string) ignores {
	var out ignores
	for l := range strings.SplitSeq(gitignore, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.ContainsAny(l, "*?[!#\\") {
			continue
		}
		switch {
		case strings.HasPrefix(l, "/"):
			out.roots = append(out.roots, strings.Trim(l, "/"))
		case strings.HasSuffix(l, "/") && !strings.Contains(strings.TrimSuffix(l, "/"), "/"):
			out.names = append(out.names, strings.TrimSuffix(l, "/"))
		}
	}
	return out
}

// covers says whether rel is one of the roots or below one, or has one of the
// names as a folder on its way.
func (g ignores) covers(rel string) bool {
	for _, r := range g.roots {
		if r != "" && (rel == r || strings.HasPrefix(rel, r+"/")) {
			return true
		}
	}
	parts := strings.Split(rel, "/")
	return slices.ContainsFunc(parts[:len(parts)-1], func(p string) bool { return slices.Contains(g.names, p) }) ||
		(len(parts) > 0 && slices.Contains(g.names, parts[len(parts)-1]))
}

// untracked is g without each line that covers a file git tracks: such a
// line, docs/ say, names no tool's output, and keeping it would excuse any
// missing path it covers, at any of the places a cited path is looked for.
func (g ignores) untracked(tracked []string) ignores {
	var out ignores
	for _, r := range g.roots {
		if !slices.ContainsFunc(tracked, ignores{roots: []string{r}}.covers) {
			out.roots = append(out.roots, r)
		}
	}
	for _, n := range g.names {
		if !slices.ContainsFunc(tracked, ignores{names: []string{n}}.covers) {
			out.names = append(out.names, n)
		}
	}
	return out
}

// planted is w with the document at name replaced by text.
func (w docWorld) planted(name, text string) docWorld {
	w.docs = maps.Clone(w.docs)
	w.docs[name] = text
	return w
}

// without is w with the document at name taken away.
func (w docWorld) without(name string) docWorld {
	w.docs = maps.Clone(w.docs)
	delete(w.docs, name)
	return w
}

// names is the paths of w's documents, in order.
func (w docWorld) names() []string { return slices.Sorted(maps.Keys(w.docs)) }

// front splits a document into its front matter and its body. ok is false
// for a document that does not start with a front matter block.
func front(text string) (meta map[string]string, body string, ok bool) {
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return nil, text, false
	}
	head, body, found := strings.Cut(rest, "\n---\n")
	if !found {
		return nil, text, false
	}
	meta = map[string]string{}
	for l := range strings.SplitSeq(head, "\n") {
		if m := frontKey.FindStringSubmatch(l); m != nil {
			meta[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return meta, body, true
}

// generated is whether a document says a command made it: such a document
// takes its front matter from its generator.
func generated(text string) bool { return strings.HasPrefix(text, "<!-- generated by ") }

// frontMatter is docs.freshness.1: every document carries a type, a status
// and an as-of date in typed front matter. layers.md is generated and takes
// its front matter from its generator (M0a-04).
func frontMatter(w docWorld) []string {
	var out []string
	for _, name := range w.names() {
		if generated(w.docs[name]) {
			continue
		}
		meta, _, ok := front(w.docs[name])
		if !ok {
			out = append(out, name+": no front matter (type, status, as-of)")
			continue
		}
		for _, k := range []string{"type", "status", "as-of"} {
			if meta[k] == "" {
				out = append(out, name+": front matter has no "+k)
			}
		}
		if d := meta["as-of"]; d != "" && !asOfDate.MatchString(d) {
			out = append(out, name+": as-of "+d+" is not a date (yyyy-mm-dd)")
		}
	}
	return out
}

// noEmDash is docs.voice.1 for the documents of docs/.
func noEmDash(w docWorld) []string {
	var out []string
	for _, name := range w.names() {
		if strings.ContainsRune(w.docs[name], '\u2014') {
			out = append(out, name+": an em dash")
		}
	}
	return out
}

// slug is a heading's anchor, as GitHub makes it.
func slug(h string) string {
	h = strings.ToLower(strings.NewReplacer("`", "", "*", "").Replace(h))
	var b strings.Builder
	for _, r := range h {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// anchors are the anchors of a document's headings, the second of two equal
// ones with -1.
func anchors(text string) map[string]bool {
	out, seen := map[string]bool{}, map[string]int{}
	fenced := false
	for l := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(l, "```") {
			fenced = !fenced
		}
		if m := headingRe.FindStringSubmatch(l); m != nil && !fenced {
			s := slug(m[2])
			if n := seen[s]; n > 0 {
				out[fmt.Sprintf("%s-%d", s, n)] = true
			} else {
				out[s] = true
			}
			seen[s]++
		}
	}
	return out
}

// links is docs.links.1: every link of a document to a file or an anchor
// resolves. A link to another site is not followed (offline).
func links(w docWorld) []string {
	var out []string
	for _, name := range w.names() {
		for _, m := range mdLink.FindAllStringSubmatch(w.docs[name], -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, frag, _ := strings.Cut(target, "#")
			dest := name
			if file != "" {
				dest = path.Join(path.Dir(name), file)
				if _, ok := w.docs[dest]; !ok && !w.exists(dest) {
					out = append(out, name+": the link "+target+" goes to no file")
					continue
				}
			}
			if frag == "" {
				continue
			}
			text, ok := w.docs[dest]
			if !ok {
				out = append(out, name+": the link "+target+" names an anchor in a file that is not a document")
			} else if !anchors(text)[frag] {
				out = append(out, name+": the link "+target+" names no heading")
			}
		}
	}
	return out
}

// ruleIDs is docs.links.2 for rules: a rule id a document cites is in the
// rulebook.
func ruleIDs(w docWorld) []string {
	re := regexp.MustCompile(`\b(` + strings.Join(w.parts, "|") + `)\.[a-z0-9-]+\.\d+\b`)
	var out []string
	for _, name := range w.names() {
		for _, id := range re.FindAllString(w.docs[name], -1) {
			if !w.rules[id] {
				out = append(out, name+": cites "+id+", which is no rule of the rulebook")
			}
		}
	}
	return slices.Compact(out)
}

// numbers is docs.links.2 for decisions: an S number resolves to a decision
// of the spec.
func numbers(w docWorld) []string {
	var out []string
	spec := map[string]bool{}
	for _, m := range specDef.FindAllStringSubmatch(w.docs["docs/spec.md"], -1) {
		spec["S"+m[1]] = true
	}
	for _, name := range w.names() {
		_, body, _ := front(w.docs[name])
		for _, m := range sRef.FindAllString(body, -1) {
			if !spec[m] {
				out = append(out, name+": cites "+m+", which the spec does not define")
			}
		}
	}
	return slices.Compact(out)
}

// candidates are the places a document's path may be: beside the document,
// from the root, and under internal/ where the text leaves that out.
func candidates(doc, p string) []string {
	p = strings.TrimSuffix(p, "/")
	return []string{path.Join(path.Dir(doc), p), p, "internal/" + p, strings.TrimPrefix(p, "internal/")}
}

// registry is the table of paths that do not exist yet, each with the
// milestone that creates it or the ruling that retired it before any did,
// so the records it superseded may still name it (architecture.md section
// 3.3), read from every document: the one table serves them all.
func registry(w docWorld) map[string]string {
	out := map[string]string{}
	for _, name := range w.names() {
		for l := range strings.SplitSeq(w.docs[name], "\n") {
			if m := createdIn.FindStringSubmatch(l); m != nil {
				out[strings.TrimSuffix(m[1], "/")] = m[2] + m[3]
			}
		}
	}
	return out
}

// pathTokens are the backticked tokens of a document that name a path.
func pathTokens(text string) []string {
	var out []string
	for _, m := range backtick.FindAllStringSubmatch(text, -1) {
		t := m[1]
		if !pathLike.MatchString(t) || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "application/") || strings.HasPrefix(t, "text/") {
			continue
		}
		if strings.Contains(t, "/") || pathExt.MatchString(t) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// paths is docs.links.3: a backticked path of a document resolves, or the
// document's table says which milestone creates it. A path in that table
// that exists by now is stale (docs.freshness.4).
func paths(w docWorld) []string {
	var out []string
	reg := registry(w)
	for _, name := range w.names() {
		if generated(w.docs[name]) {
			continue
		}
		for _, p := range pathTokens(w.docs[name]) {
			found := slices.ContainsFunc(candidates(name, p), w.exists)
			if _, ok := w.docs[path.Join(path.Dir(name), p)]; ok {
				found = true
			}
			planned := slices.ContainsFunc(candidates(name, p), func(c string) bool { _, ok := reg[c]; return ok })
			switch {
			case found && planned:
				out = append(out, name+": "+p+" is listed as created later and exists")
			case !found && !planned:
				out = append(out, name+": the path "+p+" resolves to nothing and says no milestone creates it")
			}
		}
	}
	return out
}

// section is the text under the heading line head, up to the next heading
// of the same depth or a higher one.
func section(text, head string) string {
	_, rest, ok := strings.Cut(text, "\n"+head+"\n")
	if !ok {
		return ""
	}
	depth := strings.Index(head, " ")
	if loc := regexp.MustCompile(`(?m)^#{1,` + strconv.Itoa(depth) + `} `).FindStringIndex(rest); loc != nil {
		return rest[:loc[0]]
	}
	return rest
}

// treePackages are the cmd/ and internal/ folders the layout tree of section
// 3 draws, and the ones its text names in backticks.
func treePackages(text string) []string {
	sec := section(text, "## 3. Layout and the module graph")
	// the table of 3.3 lists paths of every kind, and is held by paths()
	sec, _, _ = strings.Cut(sec, "### 3.3 ")
	_, after, _ := strings.Cut(sec, "```\n")
	tree, _, _ := strings.Cut(after, "```")
	type level struct {
		indent int
		dir    string
	}
	var stack []level
	set := map[string]bool{}
	for l := range strings.SplitSeq(tree, "\n") {
		trim := strings.TrimLeft(l, " ")
		if trim == "" {
			continue
		}
		indent := len(l) - len(trim)
		head, _, _ := strings.Cut(trim, "  ")
		toks := strings.Fields(head)
		// a line of folders only; a description or a file list is not
		if len(toks) == 0 || slices.ContainsFunc(toks, func(s string) bool { return !strings.HasSuffix(s, "/") }) {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = stack[len(stack)-1].dir
		}
		for _, tk := range toks {
			dir := path.Join(parent, strings.TrimSuffix(tk, "/"))
			if indent == 0 {
				// the module's own folder
				dir = ""
			}
			if len(toks) == 1 {
				stack = append(stack, level{indent, dir})
			}
			if dir != "" {
				set[dir] = true
			}
		}
	}
	for _, m := range backtick.FindAllStringSubmatch(sec, -1) {
		if t := strings.TrimSuffix(m[1], "/"); !strings.Contains(t, "*") && pathLike.MatchString(t) && (strings.HasPrefix(t, "internal/") || strings.HasPrefix(t, "cmd/")) {
			set[t] = true
		}
	}
	var out []string
	for d := range set {
		if d == "cmd" || d == "internal" || strings.HasPrefix(d, "cmd/") || strings.HasPrefix(d, "internal/") {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// packageNames is the check that stays: the packages section 3 of
// the architecture names are the packages layers.yaml holds. Each exists, or
// layers.yaml plans it with its milestone; a planned package section 3 does
// not name, or that exists by now, is stale.
func packageNames(w docWorld) []string {
	named := treePackages(w.docs["docs/architecture.md"])
	if len(named) == 0 {
		return []string{"docs/architecture.md: section 3 names no package"}
	}
	var out []string
	for _, p := range named {
		_, planned := w.planned[p]
		switch {
		case w.exists(p) && planned:
			out = append(out, p+": layers.yaml plans it for "+w.planned[p]+" and it exists")
		case !w.exists(p) && !planned:
			out = append(out, p+": section 3 of the architecture names it; it does not exist and layers.yaml does not plan it")
		}
	}
	for _, p := range slices.Sorted(maps.Keys(w.planned)) {
		if !slices.Contains(named, p) {
			out = append(out, p+": layers.yaml plans it and section 3 of the architecture does not name it")
		}
	}
	return out
}

// docChecks are every check, by name.
func docChecks() map[string]func(docWorld) []string {
	return map[string]func(docWorld) []string{
		"front matter": frontMatter, "em dash": noEmDash, "links": links, "page links": pageLinks, "rule ids": ruleIDs,
		"numbers": numbers, "paths": paths, "package names": packageNames, "format": formatDocs, "durations": durations,
	}
}

func TestTheDocsHoldToTheirRules(t *testing.T) {
	t.Parallel() // readDocWorld reads a world of this test's own
	w := readDocWorld(t)
	if len(w.docs) == 0 {
		t.Fatal("no document under docs/")
	}
	checks := docChecks()
	for _, name := range slices.Sorted(maps.Keys(checks)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, p := range checks[name](w) {
				t.Error(p)
			}
		})
	}
}

// TestTheDocsChecksFireOnAPlantedBreach: each check is run once on the
// repository's docs with one breach planted, and says it.
func TestTheDocsChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel() // readDocWorld reads a world of this test's own, and each plant clones the map it changes
	w := readDocWorld(t)
	more := w
	more.planned = maps.Clone(w.planned)
	more.planned["internal/nowhere"] = "M1"
	none := w
	none.planned = map[string]string{}
	tests := []struct {
		name, check string
		world       docWorld
		says        string
	}{
		{"a document with no front matter", "front matter", w.planted("docs/x.md", "# x\n"), "no front matter"},
		{"a document with no status", "front matter", w.planted("docs/x.md", "---\ntype: t\nas-of: 2026-01-02\n---\n"), "has no status"},
		{"an as-of that is no date", "front matter", w.planted("docs/x.md", "---\ntype: t\nstatus: s\nas-of: soon\n---\n"), "is not a date"},
		{"an em dash", "em dash", w.planted("docs/x.md", "a \u2014 b\n"), "an em dash"},
		{"a link to no file", "links", w.planted("docs/x.md", "[a](nowhere.md)"), "goes to no file"},
		{"a board linking a file that is not there", "page links", w.plantedPage(canvasDirOfPages+"/X.dc.html", `<a href="../nowhere.md">x</a>`), "goes to no file"},
		{"a link to no heading", "links", w.planted("docs/x.md", "[a](architecture.md#no-such-heading)"), "names no heading"},
		{"a rule id that does not exist", "rule ids", w.planted("docs/x.md", "cli.nothing.99"), "no rule of the rulebook"},
		{"an S number the spec does not define", "numbers", w.planted("docs/x.md", "see S999"), "S999, which the spec does not define"},
		{"a path that resolves to nothing", "paths", w.planted("docs/x.md", "see `internal/nowhere/at/all`"), "resolves to nothing"},
		{"a created-later path that exists", "paths", w.planted("docs/x.md", "| `layers.yaml` | this repository | created in M1 |\n"), "is listed as created later and exists"},
		{"a retired path that exists", "paths", w.planted("docs/x.md", "| `layers.yaml` | this repository | retired: no milestone creates it |\n"), "is listed as created later and exists"},
		{"a package section 3 names that layers.yaml does not plan", "package names", none, "layers.yaml does not plan it"},
		{"a planned package section 3 does not name", "package names", more, "section 3 of the architecture does not name it"},
		{"a duration in prose", "durations", w.planted("docs/x.md", "This takes about 3 minutes.\n"), "x.md: a duration in prose: This takes about 3 minutes."},
		{"a baseline line the prose no longer has", "durations", withStays(w, "docs/x.md: It took 9 seconds."), "is not in the prose any more"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := docChecks()[tc.check](tc.world)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("%s said %q, want a problem saying %q", tc.check, got, tc.says)
			}
		})
	}
}

// TestAFreshCloneReadsTheDocsAsAUsedOne: a backticked path of output the
// .gitignore keeps out of a clone (times/measure.json) resolves with no run
// before it, and a path nobody ignores that is not there still does not.
func TestAFreshCloneReadsTheDocsAsAUsedOne(t *testing.T) {
	t.Parallel()
	fresh := func(gitignore string) docWorld {
		dir := t.TempDir()
		if gitignore != "" {
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		w := docWorld{docs: map[string]string{"docs/x.md": "see `times/measure.json` and `timesheet/x.json`\n"}, planned: map[string]string{}}
		w.exists = existsIn(t, dir)
		return w
	}
	got := paths(fresh("/times/\n"))
	if slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "times/measure.json") }) {
		t.Errorf("an ignored output folder did not resolve: %q", got)
	}
	if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "timesheet/x.json") }) {
		t.Errorf("a path beside an ignored folder resolved: %q", got)
	}
	if got := paths(fresh("")); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "times/measure.json") }) {
		t.Errorf("with nothing ignored the missing output did not fail: %q", got)
	}
	if got := paths(fresh("*/measure.json\n!/times/\n# /times/\n/out/*\nsub/times/\n")); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "times/measure.json") }) {
		t.Errorf("a wildcard, negated, commented or slashed line was read as an ignored folder: %q", got)
	}
}

// TestAnUnrootedIgnoreLineIsReadAsGitReadsIt: a bare name with a trailing
// slash and no wildcard is a folder ignored at any depth (node_modules/ keeps
// tools/.node/node_modules out of an export); a line with a wildcard, a
// negation, a comment or a slash inside it is not read at all.
func TestAnUnrootedIgnoreLineIsReadAsGitReadsIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, gitignore, rel string
		want                 bool
	}{
		{"unrooted folder at the root", "times/\n", "times/measure.json", true},
		{"unrooted folder deep", "node_modules/\n", "tools/.node/node_modules", true},
		{"unrooted folder on the way", "node_modules/\n", "a/node_modules/b/c.js", true},
		{"a longer name beside it", "node_modules/\n", "tools/.node/node_modules_x", false},
		{"rooted folder", "/times/\n", "times/measure.json", true},
		{"rooted folder is not deep", "/times/\n", "a/times/measure.json", false},
		{"wildcard", "node_*/\n", "tools/node_modules", false},
		{"negation", "!node_modules/\n", "tools/node_modules", false},
		{"comment", "# node_modules/\n", "tools/node_modules", false},
		{"slash inside", "tools/node_modules/\n", "tools/node_modules", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.gitignore), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := existsIn(t, dir)(tc.rel); got != tc.want {
				t.Errorf("%q with %q: got %v, want %v", tc.rel, tc.gitignore, got, tc.want)
			}
		})
	}
}

// TestAnExportedTreeHasNoGitFolderAndTheDocsStillCiteIt: the one name .git
// resolves with no folder there (a git archive holds none), and nothing
// beside it does: .gitx and a path below .git are looked up like any other.
func TestAnExportedTreeHasNoGitFolderAndTheDocsStillCiteIt(t *testing.T) {
	t.Parallel()
	exists := existsIn(t, t.TempDir())
	if !exists(".git") {
		t.Error(".git did not resolve in a tree with no .git folder")
	}
	for _, rel := range []string{".gitx", ".git/foo-not-special", ".github"} {
		if exists(rel) {
			t.Errorf("%s resolved in an empty tree, so .git is not the only name read as there", rel)
		}
	}
}

// TestAnIgnoreLineOverATrackedFolderExcusesNothing: an ignore line excuses a
// missing cited path only where nothing tracked is at or below what it
// matches. docs/ added to the .gitignore with a cited doc deleted is still a
// path that resolves to nothing, in a clone, the deletion staged or not, and
// in an export; node_modules/, which no tracked file is under, still excuses
// the tool install's folder. Risk: a change ignores its way past a
// deleted doc with no verdict, since .gitignore is not protected.
func TestAnIgnoreLineOverATrackedFolderExcusesNothing(t *testing.T) {
	t.Parallel()
	git := func(t *testing.T, dir string, cmd *exec.Cmd) {
		t.Helper()
		cmd.Dir = dir
		cmd.Env = append(configtest.Inherited(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", cmd.Args, err, out)
		}
	}
	cites := "see `docs/gone.md` and `tools/.node/node_modules/x/y.js`\n"
	tree := func(t *testing.T, form string) string {
		t.Helper()
		files := map[string]string{"docs/gone.md": "gone\n", "docs/kept.md": "kept\n", "tools/.node/package.json": "{}\n", ".gitignore": "/times/\n"}
		var dir string
		if form == "export" {
			dir = t.TempDir()
			for name, text := range files {
				p := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		} else {
			// The committed tree is one commit on the default branch: the
			// files above, then the ignore line widened and the doc deleted below.
			dir = gittest.Repo(t, files, nil)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/times/\ndocs/\nnode_modules/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		switch form {
		case "clone, the deletion staged":
			git(t, dir, exec.CommandContext(t.Context(), "git", "rm", "-q", "docs/gone.md"))
		default:
			if err := os.Remove(filepath.Join(dir, "docs", "gone.md")); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	for _, form := range []string{"export", "clone, the deletion staged", "clone, the deletion not staged"} {
		t.Run(form, func(t *testing.T) {
			t.Parallel()
			w := docWorld{docs: map[string]string{"x.md": cites}, planned: map[string]string{}}
			w.exists = existsIn(t, tree(t, form))
			got := paths(w)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "docs/gone.md") }) {
				t.Errorf("a deleted doc under an ignore line over a tracked folder was excused: %q", got)
			}
			if slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "node_modules") }) {
				t.Errorf("the tool install's folder, which nothing tracked is under, was not excused: %q", got)
			}
		})
	}
}

// A .git that points nowhere is a listing that fails, and the failure comes back as an error to
// the test that asked, not as a Fatalf inside a shared once-function, whose Goexit became
// panic(nil) and aborted every test of the package. A folder with no .git lists its files.
//
// What it costs when this fails: one broken checkout turns the docs check from one red test into a
// package panic, and the other tests of this package report nothing.
func TestABrokenGitListingIsAnErrorOfTheAskerAndNotAPackagePanic(t *testing.T) {
	t.Parallel()
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, ".git"), []byte("gitdir: /nowhere/at/all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := listTracked(t.Context(), broken); err == nil || !strings.Contains(err.Error(), "list the files git tracks") {
		t.Errorf("a .git that points nowhere listed %v with error %v, want the listing error", got, err)
	}
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := listTracked(t.Context(), plain)
	if err != nil || !slices.Equal(got, []string{"a.md"}) {
		t.Errorf("a folder with no .git listed %v with error %v, want [a.md]", got, err)
	}
}
