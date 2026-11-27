package internal

// gragas carries no operating-system-specific code, and the
// commit tier refuses a change that adds any (gragas#1447). A Go file carries
// some when it has any of these:
//
//   - a name that ends in a system (_linux, _darwin, _windows, _unix, _other,
//     any system go build knows, and a system and an architecture), with or
//     without _test;
//   - a //go:build line that names a system, negated or not;
//   - runtime.GOOS;
//   - a string that holds a /proc/ path;
//   - a call to sysctl, or the program of that name.
//
// The files that already do are listed by path in gate/os-code-allowlist.txt,
// each with the row of gragas#1446 that deletes it. The list only shrinks, and
// its base is git, not a number (gragas#1494): a listed path that the list at
// the merge base with origin/main does not hold is red, so an addition, a swap
// and a rename are refused, and a removal always passes. A listed file that no
// longer matches is red until its line is taken off, so no slack is left for a
// path to be swapped into. With no merge base to read, a shallow clone or a
// tree outside git, the check is red and says why. The check reads files as
// text; it carries no system's code itself.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/core/audit"
)

// osCodeAllowlist is the one file the check and its test read.
const osCodeAllowlist = "gate/os-code-allowlist.txt"

// osCodeBase is the allowlist as it stands at the merge base of HEAD in dir (during a merge, the newer of the merge bases of HEAD and MERGE_HEAD, gragas#1538)
// with origin/main, or why it cannot be read: dir is not in git (a tarball),
// the clone is shallow, there is no origin/main or no history in common with
// it, or the list is not at that commit. git looks no higher than dir's
// parent, so a tree outside git is not read as a repository above it.
func osCodeBase(t *testing.T, dir string) (string, string) {
	t.Helper()
	list, why, _ := osListBase(t, dir, osCodeAllowlist)
	return list, why
}

// osListBase is osCodeBase for any list file: the list at the merge base, or why it cannot be read, and
// whether the one reason is that the list is not at the merge base (the kinds list of gragas#1591 is held to a
// base only once main has it).
//
// also is the other branches of origin the list is held to as well (gragas#2038: a list a release branch
// holds before main does): the merge base with each that resolves is a base too, and a base that is an
// ancestor of another is dropped, so the newest holds the list. Two that are not in line both stand, and
// the list is the places either holds (linesInAny): a place is new only where every base lacks it, so a
// merge of main into the release, which brings main's lines, is green. A branch that does not resolve, or shares no history,
// adds no base.
func osListBase(t *testing.T, dir, file string, also ...string) (string, string, bool) {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "the merge base cannot be read: " + err.Error(), false
	}
	ceiling := []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(abs)}
	shallow, err := gitTry(t, abs, ceiling, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", "the tree is not a git checkout, so the list has no merge base to be held to: " + err.Error(), false
	}
	if shallow != "false" {
		return "", "the clone is shallow, so its merge base with origin/main is not the one main has: fetch the full history", false
	}
	base, err := gitTry(t, abs, ceiling, "merge-base", "HEAD", "refs/remotes/origin/main")
	if err != nil {
		return "", "HEAD has no merge base with origin/main (no origin/main, or no history in common), so the list has none to be held to: " + err.Error(), false
	}
	// During a merge, pre-commit runs before the merge commit exists, so HEAD is still the branch's old tip and
	// its merge base is the old fork point, where a list that main brought in later is not (gragas#1538). The
	// commit being made has two parents, so its base is the newer of the two merge bases: the one the other is
	// an ancestor of. A list missing at that base is still red, and outside a merge nothing changes.
	ancestor := func(a, b string) bool {
		_, err := gitTry(t, abs, ceiling, "merge-base", "--is-ancestor", a, b)
		return err == nil
	}
	newer := func(base, ref string) string {
		if _, err := gitTry(t, abs, ceiling, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
			if other, err := gitTry(t, abs, ceiling, "merge-base", "MERGE_HEAD", ref); err == nil && other != base && ancestor(base, other) {
				return other
			}
		}
		return base
	}
	bases, names := []string{newer(base, "refs/remotes/origin/main")}, []string{"origin/main"}
	for _, ref := range also {
		if b, err := gitTry(t, abs, ceiling, "merge-base", "HEAD", ref); err == nil {
			bases, names = append(bases, newer(b, ref)), append(names, strings.TrimPrefix(ref, "refs/remotes/"))
		}
	}
	var lists, missing []string
	for i, b := range bases {
		if slices.Contains(bases[:i], b) || slices.ContainsFunc(bases, func(o string) bool { return o != b && ancestor(b, o) }) {
			continue
		}
		list, err := gitTry(t, abs, ceiling, "show", b+":"+file)
		if err != nil {
			missing = append(missing, file+" is not at the merge base "+b+" with "+names[i]+": "+err.Error())
			continue
		}
		lists = append(lists, list+"\n")
	}
	if len(lists) == 0 {
		return "", strings.Join(missing, "; "), true
	}
	return linesInAny(lists), "", false
}

// linesInAny is the lines any list holds, each place as often as the most hold it, in the order the lists
// first hold them: the first list whole, then what each later one adds. A place is a line's entry as
// parseListed reads it (listedPlace), so two bases that word one place's reason differently hold it once,
// and a list cannot take one more of it by a reworded reason.
func linesInAny(lists []string) string {
	var out strings.Builder
	most := map[string]int{}
	for _, l := range lists {
		here := map[string]int{}
		for line := range strings.Lines(l) {
			place := listedPlace(line)
			if here[place]++; here[place] > most[place] {
				most[place]++
				out.WriteString(line)
			}
		}
	}
	return out.String()
}

// listedPlace is what linesInAny counts a line by: its entry as parseListed reads it, the text before its
// # with the spaces made single, or, for a line with none (a comment, a blank), the line itself, kept
// apart from every entry.
func listedPlace(line string) string {
	entry, _, _ := strings.Cut(line, "#")
	if entry = strings.Join(strings.Fields(entry), " "); entry != "" {
		return entry
	}
	return "\x00" + line
}

// procDir is the folder a /proc/ path starts with, joined at run time: the check
// folds a constant "a" + "b", so a constant spelling would be red in this file,
// which the check reads too.
func procDir() string { return strings.Join([]string{"/pro", "c"}, "") }

// sysctlName is a call or a program named sysctl, in any case, with or
// without a by-name suffix.
var sysctlName = regexp.MustCompile(`(?i)(^|/)sysctl(byname)?$`)

// runtimeNames is the names the file gives the runtime package: its own name or
// the one it was imported as, and whether a dot import brings its names in bare.
func runtimeNames(file *ast.File) (names map[string]bool, dot bool) {
	names = map[string]bool{}
	for _, im := range file.Imports {
		if p, err := strconv.Unquote(im.Path.Value); err != nil || p != "runtime" {
			continue
		}
		switch {
		case im.Name == nil:
			names["runtime"] = true
		case im.Name.Name == ".":
			dot = true
		case im.Name.Name != "_":
			names[im.Name.Name] = true
		}
	}
	return names, dot
}

// foldString is the value of a constant string expression: a string literal, a
// parenthesised one, or a + of such, so "/pro" + "c" reads as "/proc".
func foldString(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(x.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return foldString(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, lok := foldString(x.X)
		r, rok := foldString(x.Y)
		return l + r, lok && rok
	}
	return "", false
}

// osCodeReasons is why a file counts as carrying operating-system code, each
// reason once, sorted: none when it carries none. A file that does not parse
// is a reason, never a pass.
func osCodeReasons(name string, body []byte) []string {
	var why []string
	if named, err := audit.NamesAnOS(name, body); err != nil {
		why = append(why, "it is not read as far as its package clause: "+err.Error())
	} else if named {
		why = append(why, "its name or its //go:build line names a system")
	}
	file, err := parser.ParseFile(token.NewFileSet(), name, body, parser.SkipObjectResolution)
	if err != nil {
		return append(why, "it does not parse: "+err.Error())
	}
	rt, dot := runtimeNames(file)
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if dot && x.Name == "GOOS" {
				why = append(why, "it reads runtime.GOOS")
			}
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && rt[id.Name] && x.Sel.Name == "GOOS" {
				why = append(why, "it reads runtime.GOOS")
			}
		case *ast.CallExpr:
			callee := ""
			switch f := x.Fun.(type) {
			case *ast.Ident:
				callee = f.Name
			case *ast.SelectorExpr:
				callee = f.Sel.Name
			}
			if sysctlName.MatchString(callee) {
				why = append(why, "it calls sysctl")
			}
		case *ast.BasicLit, *ast.BinaryExpr:
			if s, ok := foldString(x.(ast.Expr)); ok {
				if proc := procDir(); strings.Contains(s, proc+"/") || s == proc {
					why = append(why, "it holds a path under "+proc)
				}
				if sysctlName.MatchString(strings.TrimSpace(s)) {
					why = append(why, "it names the sysctl program")
				}
			}
		}
		return true
	})
	slices.Sort(why)
	return slices.Compact(why)
}

// osCodeFiles is each Go file of repo that carries operating-system code, by
// slash path, with why. Hidden folders, testdata, vendor and node_modules,
// and files whose names start with a dot or _, are not read, as go build
// reads none of them.
func osCodeFiles(repo fs.FS) (map[string][]string, error) { return osFilesBy(repo, osCodeReasons) }

// osFilesBy is each Go file of repo that why gives a reason for, by slash path, under the walk osCodeFiles
// describes; the kinds check of gragas#1591 walks the same way.
func osFilesBy(repo fs.FS, reasons func(name string, body []byte) []string) (map[string][]string, error) {
	found := map[string][]string{}
	err := fs.WalkDir(repo, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n := d.Name()
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || n == "testdata" || n == "vendor" || n == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(n, ".go") || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
			return nil
		}
		body, err := fs.ReadFile(repo, p)
		if err != nil {
			return err
		}
		if why := reasons(n, body); len(why) > 0 {
			found[p] = why
		}
		return nil
	})
	return found, err
}

// parseOsCodeAllowlist is the paths of the list text, each with its row: a
// line is "<path> <row of gragas#1446>", and a line that starts with # or is
// empty is a comment. A line with no row, or a path twice, is a problem.
func parseOsCodeAllowlist(text string) (paths []string, problems []string) {
	seen := map[string]bool{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, row, _ := strings.Cut(line, " ")
		switch {
		case strings.TrimSpace(row) == "":
			problems = append(problems, "line "+strconv.Itoa(i+1)+": "+p+" names no row of gragas#1446")
		case seen[p]:
			problems = append(problems, "line "+strconv.Itoa(i+1)+": "+p+" is listed twice")
		default:
			paths = append(paths, p)
		}
		seen[p] = true
	}
	return paths, problems
}

// osCodeProblems is what the check says of repo and its list, against base,
// the list at the merge base: each listed path base does not hold, each file
// that carries operating-system code and is not listed, each listed path that
// is gone or no longer carries any, and each line with no row. None when the
// tree is green.
func osCodeProblems(repo fs.FS, list, base string) []string {
	found, err := osCodeFiles(repo)
	if err != nil {
		return []string{"the Go files could not be read: " + err.Error()}
	}
	listed, problems := parseOsCodeAllowlist(list)
	before, _ := parseOsCodeAllowlist(base)
	for _, p := range listed {
		if !slices.Contains(before, p) {
			problems = append(problems, p+" is listed in "+osCodeAllowlist+" and not in the list at the merge base with origin/main: the list takes no new path, and a rename or a move of a listed file is an addition too. The route is an ASK on the platform side for the need, never a new line")
		}
	}
	for _, p := range slices.Sorted(maps.Keys(found)) {
		if !slices.Contains(listed, p) {
			problems = append(problems, p+" carries operating-system code ("+strings.Join(found[p], "; ")+"): gragas has none; ask for the need on the platform side, never write it here")
		}
	}
	for _, p := range listed {
		if _, ok := found[p]; !ok {
			problems = append(problems, p+" is listed in "+osCodeAllowlist+" and no longer carries operating-system code, or is gone: take the line off")
		}
	}
	return problems
}

// TestNoNewOsCode is the check the commit tier runs: the tree, with the
// allowlist held to the allowlist at its merge base with origin/main, is green
// (gragas#1447, gragas#1494). With no merge base it is red and says why.
func TestNoNewOsCode(t *testing.T) {
	t.Parallel()
	list, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(osCodeAllowlist)))
	if err != nil {
		t.Fatal(err)
	}
	base, why := osCodeBase(t, root)
	if why != "" {
		t.Fatal(why)
	}
	for _, p := range osCodeProblems(os.DirFS(root), string(list), base) {
		t.Error(p)
	}
}

// osPlants is one file of each kind the check refuses, and the controls that
// it does not: a plain file, a system word in a comment, and a build line
// that names a feature.
func osPlants() map[string]string {
	pkg := "package x\n"
	return map[string]string{
		"p/a_linux.go":         pkg,
		"p/b_darwin.go":        pkg,
		"p/c_windows.go":       pkg,
		"p/d_unix.go":          pkg,
		"p/e_other.go":         pkg,
		"p/f_linux_test.go":    pkg,
		"p/g_freebsd_amd64.go": pkg,
		"p/h.go":               "//go:build linux\n\n" + pkg,
		"p/i.go":               "//go:build !windows\n\n" + pkg,
		"p/j.go":               "//go:build unix && race\n\n" + pkg,
		"p/k.go":               pkg + "\nimport \"runtime\"\n\nvar K = runtime.GOOS\n",
		"p/l.go":               pkg + "\nvar L = \"/pro\" + \"c/self/exe\"\n",
		"p/m.go":               pkg + "\nfunc sysctl(string) int { return 0 }\n\nvar M = sysctl(\"x\")\n",
		"p/n.go":               pkg + "\nvar N = []string{\"/us\" + \"r/sbin/sys\" + \"ctl\", \"-n\"}\n",
		"p/o.go":               pkg + "\nfunc f(u struct{ Sysctl func(string) int }) int { return u.Sysctl(\"x\") }\n",
		"p/q.go":               pkg + "\nimport rt \"runtime\"\n\nvar Q = rt.GOOS\n",
		"p/r.go":               pkg + "\nimport . \"runtime\"\n\nvar R = GOOS\n",
		"p/s.go":               pkg + "\nvar S = (\"/pro\" + (\"c/\" + \"self\"))\n",
		"plain/ok.go":          pkg + "\n// Linux, darwin and runtime.GOOS in a comment name no system to the check.\nvar Plain = 1\n",
		"plain/tag.go":         "//go:build race\n\n" + pkg,
		"plain/linuxish.go":    pkg + "\nvar Linuxish = \"linux\"\n",
		"plain/win_test.go":    pkg,
		"plain/local.go":       pkg + "\nvar runtime = struct{ GOOS string }{}\n\nvar Local = runtime.GOOS\n",
		"plain/join.go":        pkg + "\nvar Join = \"/pro\" + string(rune(99+1))\n",
	}
}

// TestTheOsCodeCheckSeesEachKindRed plants one file of each kind and a
// control of each near miss: each plant is red, each by its own name, and no
// control is reported.
// Planted failure: a kind the check does not read, or a control it reports.
func TestTheOsCodeCheckSeesEachKindRed(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{}
	for p, body := range osPlants() {
		repo[p] = &fstest.MapFile{Data: []byte(body)}
	}
	problems := strings.Join(osCodeProblems(repo, "", ""), "\n")
	for p := range osPlants() {
		reported := strings.Contains(problems, p+" carries operating-system code")
		if strings.HasPrefix(p, "p/") && !reported {
			t.Errorf("the plant %s is not red:\n%s", p, problems)
		}
		if strings.HasPrefix(p, "plain/") && reported {
			t.Errorf("the control %s is red:\n%s", p, problems)
		}
	}
	if bad := osCodeProblems(fstest.MapFS{"p/x.go": &fstest.MapFile{Data: []byte("package x\nfunc (")}}, "", ""); len(bad) == 0 {
		t.Error("a file that does not parse is green")
	}
}

// TestTheOsCodeAllowlistOnlyShrinks plants the list's failures against the
// list at the merge base (gragas#1494), each red for its own reason: an added
// line, a swapped line, a rename of a listed file (an addition, and the
// message says so), a listed file with no system's code left, a line with no
// row, a path twice and a listed file that is gone. Each message of an
// addition names the route, an ASK on the platform side. A
// removed line with its file cleaned is green, and so is the tree with its
// list. Then the base itself, in a repository made here: read at the merge
// base with origin/main, and red, saying why, outside git, with no
// origin/main, in a shallow clone, and with the list not at the merge base.
// Planted failure: a list that takes a new path, keeps a file that no longer
// matches, or passes with no merge base.
func TestTheOsCodeAllowlistOnlyShrinks(t *testing.T) {
	t.Parallel()
	const a, b, c = "host/a_linux.go #1446 row 1a\n", "host/b_linux.go #1446 row 1b\n", "host/c_linux.go #1446 row 1c\n"
	tree := func(files ...string) fstest.MapFS {
		repo := fstest.MapFS{}
		for _, f := range files {
			repo[f] = &fstest.MapFile{Data: []byte("package host\n")}
		}
		return repo
	}
	added := "is listed in " + osCodeAllowlist + " and not in the list at the merge base"
	for _, tc := range []struct {
		name       string
		repo       fstest.MapFS
		list, base string
		want       []string // each a problem says; none when green
	}{
		{"the tree with its list", tree("host/a_linux.go", "host/b.go"), a, a, nil},
		{"a removed line with its file cleaned", tree("host/a_linux.go", "host/b.go"), a, a + b, nil},
		{"an added line", tree("host/a_linux.go", "host/c_linux.go"), a + c, a, []string{"host/c_linux.go " + added}},
		{"a swapped line", tree("host/a_linux.go", "host/c_linux.go"), a + c, a + b, []string{"host/c_linux.go " + added}},
		{"a renamed file", tree("host/z_linux.go"), "host/z_linux.go #1446 row 1a\n", a, []string{"host/z_linux.go " + added, "a rename or a move of a listed file is an addition too"}},
		{"an added line names its rule and route", tree("host/a_linux.go", "host/c_linux.go"), a + c, a, []string{"an ASK on the platform side for the need, never a new line"}},
		{"a removed line with its file left", tree("host/a_linux.go", "host/b_linux.go"), a, a + b, []string{"host/b_linux.go carries operating-system code"}},
		{"a listed file with no system's code left", tree("host/a_linux.go", "host/b.go"), a + "host/b.go #1446 row 1b\n", a + "host/b.go #1446 row 1b\n", []string{"host/b.go is listed in " + osCodeAllowlist + " and no longer carries"}},
		{"a listed file that is gone", tree("host/a_linux.go"), a + b, a + b, []string{"host/b_linux.go is listed in " + osCodeAllowlist + " and no longer carries"}},
		{"a line with no row", tree("host/a_linux.go"), "host/a_linux.go\n", a, []string{"names no row of gragas#1446"}},
		{"a path listed twice", tree("host/a_linux.go"), a + a, a, []string{"is listed twice"}},
	} {
		got := osCodeProblems(tc.repo, tc.list, tc.base)
		if tc.want == nil && len(got) != 0 {
			t.Errorf("%s: red: %q", tc.name, got)
		}
		for _, w := range tc.want {
			if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, w) }) {
				t.Errorf("%s: want a problem saying %q, got %q", tc.name, w, got)
			}
		}
	}

	// The base, in a repository of one commit that holds the list.
	repo := filepath.Join(t.TempDir(), "repo")
	put(t, repo, osCodeAllowlist, []byte(a))
	put(t, repo, "README", []byte("x\n"))
	wideGit(t, repo, "init", "-q")
	wideGit(t, repo, "add", "-A")
	wideGit(t, repo, "commit", "-q", "-m", "base")
	baseIs := func(name, dir, want string) {
		t.Helper()
		got, why := osCodeBase(t, dir)
		switch {
		case want == "" && (why != "" || got != a):
			t.Errorf("%s: the base is %q (%s), want the list %q", name, got, why, a)
		case want != "" && !strings.Contains(why, want):
			t.Errorf("%s: want red saying %q, got the base %q and %q", name, want, got, why)
		}
	}
	baseIs("no origin/main", repo, "HEAD has no merge base with origin/main")
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	baseIs("origin/main at HEAD", repo, "")
	// A branch past origin/main that adds a line is still held to the base.
	put(t, repo, osCodeAllowlist, []byte(a+c))
	wideGit(t, repo, "commit", "-q", "-am", "add")
	baseIs("a branch past origin/main", repo, "")
	baseIs("outside git", t.TempDir(), "the tree is not a git checkout")
	shallow := filepath.Join(t.TempDir(), "shallow")
	wideGit(t, filepath.Dir(shallow), "clone", "-q", "--depth", "1", "file://"+filepath.ToSlash(repo), shallow)
	wideGit(t, shallow, "update-ref", "refs/remotes/origin/main", "HEAD")
	baseIs("a shallow clone", shallow, "the clone is shallow")
	empty := filepath.Join(t.TempDir(), "empty")
	put(t, empty, "README", []byte("x\n"))
	wideGit(t, empty, "init", "-q")
	wideGit(t, empty, "add", "-A")
	wideGit(t, empty, "commit", "-q", "-m", "no list")
	wideGit(t, empty, "update-ref", "refs/remotes/origin/main", "HEAD")
	baseIs("the list not at the merge base", empty, osCodeAllowlist+" is not at the merge base")
}

// TestTheOsCodeAllowlistNamesItsRows holds each line of the list to a row of
// gragas#1446 (a number or "row" and its number), and the list to its order.
func TestTheOsCodeAllowlistNamesItsRows(t *testing.T) {
	t.Parallel()
	text, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(osCodeAllowlist)))
	if err != nil {
		t.Fatal(err)
	}
	paths, problems := parseOsCodeAllowlist(string(text))
	for _, p := range problems {
		t.Error(p)
	}
	if !slices.IsSorted(paths) {
		t.Error(osCodeAllowlist + " is not in order")
	}
	for _, p := range paths {
		if path.Clean(p) != p || strings.Contains(p, "\\") {
			t.Errorf("%s is not a slash path", p)
		}
	}
}

// TestTheOsCodeBaseOfAMergeIsTheNewerOfItsTwoParentsBases holds gragas#1538: a branch cut before the
// list existed can merge main without a bypass, a merge that adds a row is still red, and so is a plain
// commit that adds one.
func TestTheOsCodeBaseOfAMergeIsTheNewerOfItsTwoParentsBases(t *testing.T) {
	t.Parallel()
	const a, c = "host/a_linux.go #1446 row 1a\n", "host/c_linux.go #1446 row 1c\n"
	repo := filepath.Join(t.TempDir(), "repo")
	put(t, repo, "README", []byte("x\n"))
	wideGit(t, repo, "init", "-q", "-b", "main")
	wideGit(t, repo, "add", "README")
	wideGit(t, repo, "commit", "-q", "-m", "before the list")
	wideGit(t, repo, "checkout", "-q", "-b", "old")
	put(t, repo, "old.txt", []byte("branch work\n"))
	wideGit(t, repo, "add", "old.txt")
	wideGit(t, repo, "commit", "-q", "-m", "the branch's own commit")
	wideGit(t, repo, "checkout", "-q", "main")
	put(t, repo, osCodeAllowlist, []byte(a))
	wideGit(t, repo, "add", osCodeAllowlist)
	wideGit(t, repo, "commit", "-q", "-m", "main brings the list")
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, repo, "checkout", "-q", "old")

	// The branch before the merge: the list is not at its base, which stays red.
	if got, why := osCodeBase(t, repo); got != "" || !strings.Contains(why, osCodeAllowlist+" is not at the merge base") {
		t.Errorf("a pre-list branch outside a merge: base %q, %q, want red", got, why)
	}
	// (a) a merge of main into it: the base is main's tip, which holds the list.
	wideGit(t, repo, "merge", "--no-commit", "--no-ff", "refs/remotes/origin/main")
	got, why := osCodeBase(t, repo)
	if why != "" || got != a {
		t.Fatalf("a merge into a pre-list branch: base %q, %q, want the list %q", got, why, a)
	}
	repoFS := fstest.MapFS{"host/a_linux.go": &fstest.MapFile{Data: []byte("package host\n")}}
	if p := osCodeProblems(repoFS, a, got); len(p) != 0 {
		t.Errorf("a merge into a pre-list branch: red: %q", p)
	}
	// (b) the same merge with an added row is red.
	repoFS["host/c_linux.go"] = &fstest.MapFile{Data: []byte("package host\n")}
	if p := osCodeProblems(repoFS, a+c, got); !slices.ContainsFunc(p, func(s string) bool { return strings.Contains(s, "not in the list at the merge base") }) {
		t.Errorf("a merge that adds a row: want red, got %q", p)
	}
	// (c) a plain commit on a branch that has the list adds a row: the base is the fork point's list.
	wideGit(t, repo, "commit", "-q", "-m", "merge main")
	put(t, repo, osCodeAllowlist, []byte(a+c))
	wideGit(t, repo, "commit", "-q", "-am", "add a row")
	got, why = osCodeBase(t, repo)
	if why != "" || got != a {
		t.Fatalf("a plain commit past the merge: base %q, %q, want the list %q", got, why, a)
	}
	if p := osCodeProblems(repoFS, a+c, got); len(p) == 0 {
		t.Error("a plain commit that adds a row: want red, got green")
	}
}

// TestAListIsHeldToTheNewestBaseOfMainAndARelease holds the also of osListBase (gragas#2038): a list a
// release branch brought in, which main lacks, is held to the merge base with that branch; a base that is
// an ancestor of the other is dropped, so the newer list holds, though the older holds fewer lines; a branch
// that does not resolve leaves main's base alone; two bases not in line hold the lines either has; and a
// place both hold, its reason worded differently in each, is held once.
func TestAListIsHeldToTheNewestBaseOfMainAndARelease(t *testing.T) {
	t.Parallel()
	const list, a, c, d = "gate/a-list.txt", "a # one\n", "c # two\n", "d # three\n"
	const rel = "refs/remotes/origin/release-1"
	repo := filepath.Join(t.TempDir(), "repo")
	commit := func(name, file, body string) {
		t.Helper()
		put(t, repo, file, []byte(body))
		wideGit(t, repo, "add", file)
		wideGit(t, repo, "commit", "-q", "-m", name)
	}
	put(t, repo, "README", []byte("x\n"))
	wideGit(t, repo, "init", "-q", "-b", "main")
	commit("main, with no list", "README", "x\n")
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, repo, "checkout", "-q", "-b", "release")
	commit("the release brings the list", list, a)
	wideGit(t, repo, "update-ref", rel, "HEAD")
	wideGit(t, repo, "checkout", "-q", "-b", "work")
	commit("the work's own commit", "README", "work\n")
	is := func(name, wantList string, wantMissing bool, also ...string) {
		t.Helper()
		got, why, missing := osListBase(t, repo, list, also...)
		if got != wantList || missing != wantMissing || (why == "") == wantMissing {
			t.Errorf("%s: the base is %q (%q, missing %v), want %q (missing %v)", name, got, why, missing, wantList, wantMissing)
		}
	}
	is("main's base alone, which lacks the list", "", true)
	is("main's and the release's, main's an ancestor", a, false, rel)
	is("a release that does not resolve", "", true, "refs/remotes/origin/release-9")
	// Main takes the release's first list in; the release moves on with a line, and the work takes it in.
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", rel)
	wideGit(t, repo, "checkout", "-q", "release")
	commit("the release adds a line", list, a+c)
	wideGit(t, repo, "update-ref", rel, "HEAD")
	wideGit(t, repo, "checkout", "-q", "work")
	wideGit(t, repo, "merge", "-q", "--no-edit", "release")
	is("main's base alone, an older list", a, false)
	is("main's base an ancestor of the release's, dropped", a+c, false, rel)
	// Two bases not in line: main moves on from the first list with a line of its own, and the work takes
	// main in without its tree.
	wideGit(t, repo, "checkout", "-q", "-b", "newmain", "refs/remotes/origin/main")
	commit("main adds a line of its own", list, a+d)
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, repo, "checkout", "-q", "work")
	wideGit(t, repo, "merge", "-q", "-s", "ours", "--no-edit", "newmain")
	is("two bases not in line, the lines either holds", a+d+c, false, rel)
	is("main's base alone, not in line with the release", a+d, false)
	// The issue's two rows. Main is merged into the release, as pre-commit sees it, with MERGE_HEAD set and
	// the list holding both sides: green, since each line is held by one base. Then a piece cut from the
	// release past that merge adds a line neither list holds: red, since every base lacks it.
	held := func(name, list, base string, want bool) {
		t.Helper()
		lines, _ := parseListed(list)
		was, _ := parseListed(base)
		if got := len(difference(lines, was)) == 0; got != want {
			t.Errorf("%s: the list %q against the base %q is held %v, want %v", name, list, base, got, want)
		}
	}
	wideGit(t, repo, "checkout", "-q", "release")
	wideGit(t, repo, "merge", "-q", "--no-commit", "-s", "ours", "newmain")
	put(t, repo, list, []byte(a+c+d))
	base, why, _ := osListBase(t, repo, list, rel)
	if why != "" {
		t.Fatalf("a merge of main into the release: %s", why)
	}
	held("a merge of main into the release", a+c+d, base, true)
	wideGit(t, repo, "commit", "-q", "-am", "merge main into the release")
	wideGit(t, repo, "update-ref", rel, "HEAD")
	wideGit(t, repo, "checkout", "-q", "-b", "piece")
	const e = "e # four\n"
	commit("a release piece adds a line", list, a+c+d+e)
	base, why, _ = osListBase(t, repo, list, rel)
	if why != "" {
		t.Fatalf("a release piece: %s", why)
	}
	held("a release piece, a line release-1's list lacks", a+c+d+e, base, false)
	held("a release piece, the list as release-1 holds it", a+c+d, base, true)
	// a review case: main rewords a's reason, and a branch of the release takes main in
	// without its tree, so the two bases are not in line and each holds a, worded differently. The union
	// holds the place a once: a list that holds it twice is red, and the list with main's wording is green.
	const aReworded = "a # one, worded again\n"
	wideGit(t, repo, "checkout", "-q", "newmain")
	commit("main rewords a's reason", list, aReworded+d)
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, repo, "checkout", "-q", "-b", "reworded", "release")
	wideGit(t, repo, "merge", "-q", "-s", "ours", "--no-edit", "newmain")
	base, why, _ = osListBase(t, repo, list, rel)
	if why != "" {
		t.Fatalf("a reason reworded on main: %s", why)
	}
	held("two bases that word a's reason differently, the list holding a twice", a+a+c+d, base, false)
	held("two bases that word a's reason differently, the list holding a once", aReworded+c+d, base, true)
}

// TestTheOsCodeBaseOfAMergeKeepsTheNewerOfTwoBases holds the choice of gragas#1538 that the base of a merge is
// the newer of its two parents' bases: a branch that has merged a newer main and then merges a branch cut from
// an older main is held to the newer list, so a row main has since removed cannot come back through the merge.
func TestTheOsCodeBaseOfAMergeKeepsTheNewerOfTwoBases(t *testing.T) {
	t.Parallel()
	const a, b = "host/a_linux.go #1446 row 1a\n", "host/b_linux.go #1446 row 1b\n"
	repo := filepath.Join(t.TempDir(), "repo")
	put(t, repo, osCodeAllowlist, []byte(a+b))
	put(t, repo, "README", []byte("x\n"))
	wideGit(t, repo, "init", "-q", "-b", "main")
	wideGit(t, repo, "add", "-A")
	wideGit(t, repo, "commit", "-q", "-m", "main has rows a and b")
	older := func(name string) {
		wideGit(t, repo, "checkout", "-q", "-b", name, "main")
		put(t, repo, name+".txt", []byte("work\n"))
		wideGit(t, repo, "add", name+".txt")
		wideGit(t, repo, "commit", "-q", "-m", name+" work")
		wideGit(t, repo, "checkout", "-q", "main")
	}
	older("first")
	older("second") // cut from the older main, which still has row b
	put(t, repo, osCodeAllowlist, []byte(a))
	wideGit(t, repo, "commit", "-q", "-am", "main removes row b")
	wideGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, repo, "checkout", "-q", "first")
	wideGit(t, repo, "merge", "-q", "--no-ff", "-m", "merge the newer main", "refs/remotes/origin/main")
	// (d) the branch that merged the newer main merges the one cut from the older: the base is the newer list.
	wideGit(t, repo, "merge", "--no-commit", "--no-ff", "second")
	got, why := osCodeBase(t, repo)
	if why != "" || got != a {
		t.Fatalf("a merge of an older branch into one that has the newer main: base %q, %q, want the newer list %q", got, why, a)
	}
	repoFS := fstest.MapFS{
		"host/a_linux.go": &fstest.MapFile{Data: []byte("package host\n")},
		"host/b_linux.go": &fstest.MapFile{Data: []byte("package host\n")},
	}
	if p := osCodeProblems(repoFS, a+b, got); !slices.ContainsFunc(p, func(s string) bool { return strings.Contains(s, "not in the list at the merge base") }) {
		t.Errorf("a merge that brings back a row main removed: want red, got %q", p)
	}
}
