package internal

// No gragas Go package, tests included, may know that an
// operating system exists; only the platform libraries may. Besides what
// oscode_test.go refuses (a system in a name or a build line, runtime.GOOS,
// /proc/ and sysctl), a file carries operating-system knowledge when it has any
// of these, read off the syntax tree, so a comment or a string that merely
// mentions one is no hit:
//
//   - kind A: an import of syscall, golang.org/x/sys/... or os/signal, under any
//     import name, the blank one included;
//   - kind B: file-mode knowledge: os.ModePerm or fs.ModePerm, a .Mode().Perm(),
//     a .Mode() masked with & or &^, and an octal integer literal wherever it
//     stands but as the permission argument of a call that takes one (a const,
//     a comparison, a field, any argument of a helper). The permission a call
//     of permFuncs or permMethods is given and the type fs.FileMode or
//     os.FileMode are the portable std calls the ban allows, and count for
//     nothing;
//   - kind C: separators and volumes: filepath.Separator, os.PathSeparator and
//     filepath.VolumeName, a string of names joined by backslashes, a share (two
//     backslashes and a host) and a drive letter anywhere in a string (a % verb
//     before the colon is no letter, but %c:\ and %c:/ are a drive's root;
//     only a scheme's %s:// is spared). filepath.ToSlash, filepath.FromSlash,
//     filepath.SplitList, filepath.ListSeparator, os.PathListSeparator and a
//     literal that is one backslash count for nothing;
//   - kind D: a string literal that holds a rooted path of a system (/home/,
//     /tmp, /opt/, /etc/, /usr/, /var/, /proc/, /dev/, /private/, /Volumes/ and
//     /c/ among them), after a quote, a space, a comma, a shebang's !, an = or a
//     : . The literal "/dev/null" alone, as an operand of == or != or in a case,
//     is foreign text and counts for nothing. Only literals are read, never a
//     constant a + joins, and URL paths are
//     left out because a rooted path is read only where it starts a word: a / that
//     follows a letter, as in a host's name, starts none. Golden text under
//     testdata is left out because testdata is never walked, and that is where
//     text in the shape of a system's path goes when it is a test's input or
//     want and nothing more, as what a tool printed is: a file there, never a
//     literal or a join in a Go file;
//   - kind E: a string literal that is the whole name of a variable a system
//     sets (HOME, USERPROFILE, APPDATA, TMPDIR, PATHEXT, SHELL, XDG_*);
//   - kind F: a string literal that ends in the suffix .exe.
//
// There are no exceptions: a hit is on the list or it is red, and the list only
// shrinks against its merge base with origin/main (once main has it).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// importNames is the names the file gives the package at importPath: its own
// name or the one it was imported as, and whether a dot import brings its names
// in bare.
func importNames(file *ast.File, importPath, own string) (names map[string]bool, dot bool) {
	names = map[string]bool{}
	for _, im := range file.Imports {
		if p, err := strconv.Unquote(im.Path.Value); err != nil || p != importPath {
			continue
		}
		switch {
		case im.Name == nil:
			names[own] = true
		case im.Name.Name == ".":
			dot = true
		case im.Name.Name != "_":
			names[im.Name.Name] = true
		}
	}
	return names, dot
}

// systemImport is whether an import path is one gragas may not name: the
// system's calls and its signals (kind A).
func systemImport(p string) bool {
	return p == "syscall" || p == "os/signal" || p == "golang.org/x/sys" || strings.HasPrefix(p, "golang.org/x/sys/")
}

// octalLiteral is whether an integer literal is written in octal: 0o755 or the
// older 0755. A lone 0 is not.
func octalLiteral(v string) bool {
	if strings.HasPrefix(v, "0o") || strings.HasPrefix(v, "0O") {
		return true
	}
	return len(v) > 1 && v[0] == '0' && strings.Trim(v[1:], "01234567_") == ""
}

// rootedPath is a string that holds the start of a path of a system: a rooted
// folder that starts a word, so after anything but a letter, a digit, a dot, a
// hyphen or a slash: a URL's /tmp after a host's name starts none, and a shebang,
// a comma and a quote each leave one.
var rootedPath = regexp.MustCompile(`(^|[^A-Za-z0-9_.\-/])/(home|tmp|opt|etc|usr|var|proc|dev|bin|sbin|root|mnt|run|srv|lib|nix|sys|snap|private|Volumes|Applications|System|Users|Library|c)(/|$|[^A-Za-z0-9_.\-])`)

// backslash is the one separator of a Windows path, written as its code so this file holds no literal of it.
const backslash = 0x5c

// driveLetter is a string that holds a drive's root: a letter that starts a
// word, a colon and a separator. A % verb before the colon is no letter, but
// one followed by a backslash or a lone slash is a drive's root all the same
// (formatDrive), and only the :// of a URL's scheme is spared.
var (
	driveLetter = regexp.MustCompile(`(^|[^A-Za-z0-9%])[A-Za-z]:[\\/]`)
	formatDrive = regexp.MustCompile(`%[A-Za-z]:(\\|/([^/]|$))`)
)

// uncPath is a string that starts a share of the network: two backslashes and a host.
var uncPath = regexp.MustCompile(`^\\\\[^\\]`)

// backslashPath is a string made of names joined by backslashes, so a regular
// expression's \d is none.
var backslashPath = regexp.MustCompile(`^[A-Za-z0-9_.-]+(\\[A-Za-z0-9_.-]+)+$`)

// envNames is the variables a system sets for the folders and the shell of a
// person; envName is a string that is one of them whole, before an =, as a $NAME
// or ${NAME}, or as %NAME% (kind E).
const envNames = `(HOME|USERPROFILE|HOMEDRIVE|HOMEPATH|APPDATA|LOCALAPPDATA|PROGRAMDATA|TMPDIR|TEMP|TMP|PATHEXT|SHELL|XDG_[A-Z_]+)`

var envName = regexp.MustCompile(`^` + envNames + `($|=)|\$\{?` + envNames + `\b|%` + envNames + `%`)

// exeSuffix is a string that ends in the suffix of a Windows program, in any case (kind F).
var exeSuffix = regexp.MustCompile(`(?i)\.(exe|bat|cmd)$`)

// kindsLine is a line of the list: path, kinds, and in plain words the change
// that empties the file, naming no issue by its number.
var kindsLine = regexp.MustCompile(`^\S+ [A-F]+ [a-z][^#\x00-\x1f\x7f]*$`)

// modeCall is whether e is a call of a method named Mode with no argument.
func modeCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Mode" && len(call.Args) == 0
}

// separatorName is whether name is one of filepath's names of a separator or a volume.
func separatorName(name string) bool {
	switch name {
	case "Separator", "VolumeName":
		return true
	}
	return false
}

// permAt is where a call takes its permission: the index of the argument, of
// a call of so many.
type permAt struct{ arg, of int }

// permFuncs is the functions and conversions that take a permission, by
// import path and name: std's, and the platform libraries' mode calls. A call
// of anything else, a helper of gragas's own included, gives its octal no
// pass.
func permFuncs() map[string]map[string]permAt {
	return map[string]map[string]permAt{
		"os": {
			"Chmod": {1, 2}, "Mkdir": {1, 2}, "MkdirAll": {1, 2},
			"WriteFile": {2, 3}, "OpenFile": {2, 3}, "FileMode": {0, 1},
		},
		"io/fs":                                         {"FileMode": {0, 1}},
		"github.com/birdtable/go-files/write":           {"Mode": {0, 1}, "LogMode": {0, 1}},
		"github.com/birdtable/go-files/confine":         {"Mkdir": {2, 3}, "Open": {3, 4}},
		"github.com/birdtable/go-files/trust/trusttest": {"Perm": {2, 3}},
		"github.com/birdtable/go-socket/listen":         {"SocketMode": {0, 1}},
	}
}

// permMethods is the methods that take a permission, of *os.File, *os.Root and
// *zip.FileHeader, read by name and count of arguments: the receiver's type is
// not on the syntax tree.
func permMethods() map[string][]permAt {
	return map[string][]permAt{
		"Chmod":     {{0, 1}, {1, 2}},
		"Mkdir":     {{1, 2}},
		"MkdirAll":  {{1, 2}},
		"WriteFile": {{2, 3}},
		"OpenFile":  {{2, 3}},
		"SetMode":   {{0, 1}},
	}
}

// kindReader reads one file for kinds B, C and D, by the names the file gives
// the packages it asks about.
type kindReader struct {
	os, fs, fp          map[string]bool
	osDot, fsDot, fpDot bool
	perm                map[string]map[string]permAt // the file's names of permFuncs' packages
	skip                map[*ast.BasicLit]bool       // literals that count for nothing where they stand
	why                 []string
}

// permArg is the argument where call takes a permission, and whether it takes
// one there: a function or conversion of permFuncs by the name the file gives
// its package, or a method of permMethods on anything that is not a package.
func (r *kindReader) permArg(call *ast.CallExpr) (int, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		if funcs, ok := r.perm[id.Name]; ok {
			at, ok := funcs[sel.Sel.Name]
			return at.arg, ok && at.of == len(call.Args)
		}
	}
	for _, at := range permMethods()[sel.Sel.Name] {
		if at.of == len(call.Args) {
			return at.arg, true
		}
	}
	return 0, false
}

func (r *kindReader) add(reason string) { r.why = append(r.why, reason) }

// ident reads a bare name, which a dot import brings in.
func (r *kindReader) ident(x *ast.Ident) {
	switch {
	case r.osDot && x.Name == "PathSeparator":
		r.add("kind C: it names os." + x.Name)
	case (r.osDot || r.fsDot) && x.Name == "ModePerm":
		r.add("kind B: it names ModePerm")
	case r.fpDot && separatorName(x.Name):
		r.add("kind C: it names filepath." + x.Name)
	}
}

// selector reads pkg.Name and .Mode().Perm().
func (r *kindReader) selector(x *ast.SelectorExpr) {
	if id, ok := x.X.(*ast.Ident); ok {
		switch {
		case r.os[id.Name] && x.Sel.Name == "PathSeparator":
			r.add("kind C: it names os." + x.Sel.Name)
		case (r.os[id.Name] || r.fs[id.Name]) && x.Sel.Name == "ModePerm":
			r.add("kind B: it names " + id.Name + ".ModePerm")
		case r.fp[id.Name] && separatorName(x.Sel.Name):
			r.add("kind C: it names filepath." + x.Sel.Name)
		}
	}
	if x.Sel.Name == "Perm" && modeCall(x.X) {
		r.add("kind B: it reads .Mode().Perm()")
	}
}

// devNull marks the literal "/dev/null" alone, an operand of == or != or a case,
// as foreign text: a shell word, git's +++ header. Any other rooted
// literal stays counted in a comparison.
func (r *kindReader) devNull(e ast.Expr) {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		if s, err := strconv.Unquote(lit.Value); err == nil && s == "/dev/null" {
			r.skip[lit] = true
		}
	}
}

// literal reads an integer or a string: an octal, a drive, a share, a path of
// backslashes, a rooted path, a variable's name, the .exe suffix.
func (r *kindReader) literal(x *ast.BasicLit) {
	if r.skip[x] {
		return
	}
	switch x.Kind {
	case token.INT:
		if octalLiteral(x.Value) {
			r.add("kind B: it writes an octal literal, a mode")
		}
	case token.STRING:
		s, err := strconv.Unquote(x.Value)
		if err != nil {
			return
		}
		switch {
		case driveLetter.MatchString(s), formatDrive.MatchString(s):
			r.add("kind C: it holds a drive letter in a string")
		case uncPath.MatchString(s):
			r.add("kind C: it starts a string with a share")
		case backslashPath.MatchString(s):
			r.add("kind C: it holds a path joined by backslashes")
		}
		if rootedPath.MatchString(s) {
			r.add("kind D: it holds a rooted path in a string")
		}
		if envName.MatchString(s) {
			r.add("kind E: it names an environment variable of a system")
		}
		if exeSuffix.MatchString(s) {
			r.add("kind F: it holds the .exe suffix")
		}
	}
}

// knowledgeReasons is why the file knows a system exists, by kind, each once.
func knowledgeReasons(file *ast.File) []string {
	r := &kindReader{skip: map[*ast.BasicLit]bool{}}
	for _, im := range file.Imports {
		if p, err := strconv.Unquote(im.Path.Value); err == nil && systemImport(p) {
			r.add("kind A: it imports " + p)
		}
	}
	r.os, r.osDot = importNames(file, "os", "os")
	r.fs, r.fsDot = importNames(file, "io/fs", "fs")
	r.fp, r.fpDot = importNames(file, "path/filepath", "filepath")
	r.perm = map[string]map[string]permAt{}
	for p, funcs := range permFuncs() {
		names, _ := importNames(file, p, p[strings.LastIndex(p, "/")+1:])
		for n := range names {
			r.perm[n] = funcs
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			r.ident(x)
		case *ast.SelectorExpr:
			r.selector(x)
		case *ast.CallExpr:
			if i, ok := r.permArg(x); ok {
				if lit, ok := x.Args[i].(*ast.BasicLit); ok && lit.Kind == token.INT && octalLiteral(lit.Value) {
					r.skip[lit] = true // the permission a call is given
				}
			}
		case *ast.CaseClause:
			for _, e := range x.List {
				r.devNull(e)
			}
		case *ast.BinaryExpr:
			if x.Op == token.EQL || x.Op == token.NEQ {
				r.devNull(x.X)
				r.devNull(x.Y)
			}
			if (x.Op == token.AND || x.Op == token.AND_NOT) && (modeCall(x.X) || modeCall(x.Y)) {
				r.add("kind B: it masks a .Mode()")
			}
		case *ast.BasicLit:
			r.literal(x)
		}
		return true
	})
	slices.Sort(r.why)
	return slices.Compact(r.why)
}

// osKnowledgeList is the one file of the kinds check: the shrink-only list of
// the files that know a system exists today.
const osKnowledgeList = "gate/os-knowledge-allowlist.txt"

// knowledgeKinds is the kinds, as sorted letters, of a file's reasons.
func knowledgeKinds(why []string) string {
	seen := map[string]bool{}
	for _, w := range why {
		if k, _, ok := strings.Cut(strings.TrimPrefix(w, "kind "), ":"); ok && strings.HasPrefix(w, "kind ") {
			seen[k] = true
		}
	}
	return strings.Join(slices.Sorted(maps.Keys(seen)), "")
}

// knowledgeFiles is each Go file of repo that knows a system exists, by slash
// path, with its kinds. The walk is the os-code check's own.
func knowledgeFiles(repo fs.FS) (map[string]string, error) {
	found := map[string]string{}
	files, err := osFilesBy(repo, func(name string, body []byte) []string {
		file, err := parser.ParseFile(token.NewFileSet(), name, body, parser.SkipObjectResolution)
		if err != nil {
			return []string{"kind ?: it does not parse: " + err.Error()}
		}
		return knowledgeReasons(file)
	})
	for p, why := range files {
		found[p] = knowledgeKinds(why)
	}
	return found, err
}

// parseKnowledgeList is a list's lines by path, in the order read, with what is
// wrong with the lines themselves.
func parseKnowledgeList(list string) (listed map[string]string, order, problems []string) {
	listed = map[string]string{}
	for i, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		at := "line " + strconv.Itoa(i+1) + ": "
		if !kindsLine.MatchString(line) {
			problems = append(problems, at+line+" is not <path> <kinds A-F> <what empties it, in words with no issue number>")
			continue
		}
		if !slices.IsSorted(strings.Split(f[1], "")) {
			problems = append(problems, at+f[0]+": the kinds "+f[1]+" are not in order")
		}
		if _, dup := listed[f[0]]; dup {
			problems = append(problems, at+f[0]+" is listed twice")
			continue
		}
		listed[f[0]] = f[1]
		order = append(order, f[0])
	}
	if !slices.IsSorted(order) {
		problems = append(problems, osKnowledgeList+" is not in path order")
	}
	return listed, order, problems
}

// baseKnowledge is the kinds of each path of the list at the merge base. Only
// the path and the kinds are held to it, so its lines are read whatever their
// words: a base from before the lines took words ends each on an issue number.
func baseKnowledge(base string) map[string]string {
	held := map[string]string{}
	for line := range strings.Lines(base) {
		if f := strings.Fields(line); len(f) >= 2 && !strings.HasPrefix(f[0], "#") {
			held[f[0]] = f[1]
		}
	}
	return held
}

// subsetOf is whether every kind letter of a is in b.
func subsetOf(a, b string) bool {
	for _, k := range a {
		if !strings.ContainsRune(b, k) {
			return false
		}
	}
	return true
}

// knowledgeProblems is what the kinds check says of repo and its list. The
// list is exact: a line is "<path> <kinds> <what empties it>", in path order; every file
// that knows a system has a line with its kinds, and every line matches a file
// with those kinds, so a dead line, a missing one and a kind that moved are red.
// The list only shrinks: base is the list at the merge base with origin/main,
// held when haveBase (it is skipped while main has no list), and a path that is
// not in it, or a kind that its line does not have, is red.
// With the list gone, every finding is unlisted.
func knowledgeProblems(repo fs.FS, list, base string, haveBase bool) []string {
	found, err := knowledgeFiles(repo)
	if err != nil {
		return []string{"the Go files could not be read: " + err.Error()}
	}
	listed, order, problems := parseKnowledgeList(list)
	if haveBase {
		held := baseKnowledge(base)
		for _, p := range order {
			was, ok := held[p]
			switch {
			case !ok:
				problems = append(problems, p+" is a line of "+osKnowledgeList+" that the merge base with origin/main does not have: the list only shrinks (an addition, a swap or a rename is red)")
			case !subsetOf(listed[p], was):
				problems = append(problems, p+" is listed with kinds "+listed[p]+" and the merge base has "+was+": the kinds only shrink")
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(found)) {
		kinds, ok := listed[p]
		switch {
		case !ok:
			problems = append(problems, p+" knows that a system exists (kinds "+found[p]+") and is not in "+osKnowledgeList+": gragas has no such code; ask for the need on the platform side, never add a line")
		case kinds != found[p]:
			problems = append(problems, p+" is listed with kinds "+kinds+" and now has "+found[p]+": a new kind is new knowledge")
		}
	}
	for _, p := range order {
		if _, ok := found[p]; !ok {
			problems = append(problems, p+" is listed in "+osKnowledgeList+" and no longer knows a system exists, or is gone: take the line off")
		}
	}
	return problems
}

// TestNoNewOsKnowledge is the check the commit tier runs, from the member
// os-code with the two tests that plant it: every file that knows a system
// exists is on the exact list, and every line still matches. Deleting the list leaves every finding unlisted.
func TestNoNewOsKnowledge(t *testing.T) {
	t.Parallel()
	list, err := fs.ReadFile(os.DirFS(root), osKnowledgeList)
	if err != nil {
		t.Fatal(err)
	}
	base, why, missing := osListBase(t, root, osKnowledgeList)
	if why != "" && !missing {
		t.Fatal(why)
	}
	if missing {
		t.Log("the list is not on main yet, so its shrink rule is skipped: " + why)
	}
	for _, p := range knowledgeProblems(os.DirFS(root), string(list), base, why == "") {
		t.Error(p)
	}
}

// knowledgePlants is one file of each kind the check refuses, in each spelling
// it reads, and the controls near each: a comment or a string that only
// mentions one, a URL path, a constant a + joins and a local name that shadows
// a package.
func knowledgePlants() map[string]string {
	pkg := "package x\n\n"
	bs := string(rune(backslash))
	return map[string]string{
		"p/a1.go":           pkg + "import \"syscall\"\n\nvar A = syscall.Getpid\n",
		"p/a2.go":           pkg + "import _ \"os/signal\"\n",
		"p/a3.go":           pkg + "import u \"golang.org/x/sys/unix\"\n\nvar A = u.Getpid\n",
		"p/b3.go":           pkg + "import \"os\"\n\nfunc f(i os.FileInfo) any { return i.Mode().Perm() }\n",
		"p/b4.go":           pkg + "import \"os\"\n\nfunc f(i os.FileInfo) any { return i.Mode() &^ 0 }\n",
		"p/b7.go":           pkg + "const mode = 0o644\n",
		"p/b8.go":           pkg + "import \"os\"\n\nfunc f(i os.FileInfo) bool { return i.Mode() == 0755 }\n",
		"p/b9.go":           pkg + "import \"os\"\n\nvar B = os.ModePerm\n",
		"p/c9.go":           pkg + "var C = `dir" + bs + "file.txt`\n",
		"p/c10.go":          pkg + "var C = `" + bs + bs + "host" + bs + "share`\n",
		"p/c11.go":          pkg + "var C = `in C:" + bs + "Users`\n",
		"p/d4.go":           pkg + "var D = \"#!/" + "bin/sh\\n\"\n",
		"p/d5.go":           pkg + "var D = \"/" + "private/var/x\"\n",
		"p/d6.go":           pkg + "var D = \"/" + "Volumes/disk\"\n",
		"p/d7.go":           pkg + "var D = \"a,/" + "tmp/x\"\n",
		"p/e1.go":           pkg + "var E = \"HOME\"\n",
		"p/e2.go":           pkg + "var E = \"XDG_CONFIG_HOME\"\n",
		"p/e3.go":           pkg + "var E = \"USERPROFILE\"\n",
		"p/f1.go":           pkg + "var F = \"git.exe\"\n",
		"p/f2.go":           pkg + "var F = \".exe\"\n",
		"p/f3.go":           pkg + "var F = \"setup.BAT\"\n",
		"p/f4.go":           pkg + "var F = \"run.cmd\"\n",
		"p/f5.go":           pkg + "var F = \"GIT.EXE\"\n",
		"p/e4.go":           pkg + "var E = \"$" + "HOME/x\"\n",
		"p/e5.go":           pkg + "var E = \"${" + "HOME}\"\n",
		"p/e6.go":           pkg + "var E = \"%" + "APPDATA%/x\"\n",
		"p/e7.go":           pkg + "var E = \"HOME" + "=/x\"\n",
		"plain/env2.go":     pkg + "var E = \"$" + "HOMEPAGE\"\nvar F = \"%" + "HOME\"\nvar G = \"HOME" + "WORK=1\"\n",
		"plain/env.go":      pkg + "var E = \"HOME is where\"\nvar H = \"HOMEPAGE\"\nvar X = \"XDG\"\n",
		"plain/exe.go":      pkg + "var F = \"the exe file\"\nvar G = \"a.exec\"\nvar H = \"exe\"\n",
		"plain/re.go":       pkg + "var R = `^a" + bs + "d+$`\nvar S = \"a" + bs + "n\"\n",
		"plain/dec.go":      pkg + "var D = 0\nvar E = 1.5\nvar F = 0x1f\nvar G = 755\n",
		"p/c1.go":           pkg + "import \"path/filepath\"\n\nvar C = filepath.Separator\n",
		"p/c2.go":           pkg + "import \"os\"\n\nvar C = os.PathSeparator\n",
		"p/c6.go":           pkg + "var C = `C:" + bs + "Users`\n",
		"p/d1.go":           pkg + "var D = \"/" + "home/runner/work\"\n",
		"p/d2.go":           pkg + "var D = []string{\"--root\", \"/" + "tmp\"}\n",
		"p/d3.go":           pkg + "var D = \"PATH=/" + "usr/bin:" + "/" + "bin\"\n",
		"plain/comment.go":  pkg + "// syscall, os.FileMode, filepath.Separator and /" + "home/x in a comment name no system.\nvar Plain = 1\n",
		"plain/url.go":      pkg + "var U = \"https://example.org/home/x\"\n",
		"plain/join.go":     pkg + "const dir = \"/ho\"\n\nvar J = dir + \"me/x\"\n",
		"plain/word.go":     pkg + "var W = \"the /tmpl and /homes folders\"\n",
		"plain/local.go":    pkg + "var os = struct{ FileMode, PathSeparator int }{}\n\nvar L = os.FileMode + os.PathSeparator\n",
		"plain/mode.go":     pkg + "type M struct{}\n\nfunc (M) Mode() int { return 1 }\n\nvar P = M{}.Mode() + 644\n",
		"plain/zero.go":     pkg + "import \"os\"\n\nfunc f() error { return os.Truncate(\"d\", 0) }\n",
		"plain/fp.go":       pkg + "import \"path/filepath\"\n\nvar F = filepath.Join(\"a\", \"b\")\n",
		"plain/mode1.go":    pkg + "import \"os\"\n\nvar B os.FileMode\n",
		"plain/mode2.go":    pkg + "import \"io/fs\"\n\nvar B fs.FileMode\n",
		"plain/perm1.go":    pkg + "import \"os\"\n\nfunc f() error { return os.MkdirAll(\"d\", 0o755) }\n",
		"plain/perm2.go":    pkg + "import \"os\"\n\nfunc f() error { return os.WriteFile(\"d\", nil, 0644) }\n",
		"plain/perm3.go":    pkg + "import \"io/fs\"\n\nvar M = fs.FileMode(0o600)\n",
		"plain/slash.go":    pkg + "import fp \"path/filepath\"\n\nvar C = fp.ToSlash\nvar D = fp.FromSlash\n",
		"plain/list.go":     pkg + "import \"os\"\nimport \"path/filepath\"\n\nvar C = filepath.ListSeparator\nvar D = os.PathListSeparator\nvar E = filepath.SplitList\n",
		"plain/bs.go":       pkg + "var C = \"" + bs + bs + "\"\nvar D = '" + bs + bs + "'\n",
		"plain/pct.go":      pkg + "var P = \"%s://%s%s\"\n",
		"plain/devnull1.go": pkg + "var W = func(w string) bool { return w == \"/" + "dev/null\" || \"/" + "dev/null\" != w }\n",
		"plain/devnull2.go": pkg + "func f(w string) bool {\n\tswitch w {\n\tcase \"/" + "dev/null\":\n\t\treturn true\n\t}\n\treturn false\n}\n",
		"p/c12.go":          pkg + "var C = `x C:" + bs + "y`\n",
		"p/d8.go":           pkg + "var D = \"/" + "dev/null\"\n",
		"p/d9.go":           pkg + "func f(p string) bool { return p == \"/" + "etc/x\" }\n",
		"p/d10.go":          pkg + "func f(p string) bool {\n\tswitch p {\n\tcase \"/" + "tmp\":\n\t\treturn true\n\t}\n\treturn false\n}\n",
		"p/d11.go":          pkg + "func f(p string) bool { return p == \"/" + "dev/null/x\" }\n",
		"p/b10.go":          pkg + "func wantMode(t any, p string, m uint32) {}\n\nfunc f() { wantMode(nil, \"p\", 0o600) }\n",
		"p/b11.go":          pkg + "type h struct{}\n\nfunc (h) Want(m uint32) {}\n\nfunc f() { h{}.Want(0o600) }\n",
		"p/b12.go":          pkg + "import \"os\"\n\nfunc f() error { return os.Truncate(\"d\", 0o644) }\n",
		"p/b13.go":          pkg + "import \"os\"\n\nfunc f() error { return os.WriteFile(\"d\", nil, 0o644, 0o600) }\n",
		"p/c13.go":          pkg + "import \"fmt\"\n\nvar C = fmt.Sprintf(`%c:" + bs + "`, 'C')\n",
		"p/c14.go":          pkg + "import \"fmt\"\n\nvar C = fmt.Sprintf(\"%c:" + "/x\", 'C')\n",
		"plain/pct2.go":     pkg + "var P = \"%s://%s\"\nvar Q = \"%d:%d\"\nvar R = \"%v: %v\"\n",
		"plain/perm4.go":    pkg + "import \"os\"\n\nfunc f(r *os.Root, h interface{ SetMode(uint32) }) error {\n\th.SetMode(0o755)\n\treturn r.MkdirAll(\"d\", 0o700)\n}\n",
		"plain/perm5.go":    pkg + "import \"github.com/birdtable/go-files/write\"\nimport c \"github.com/birdtable/go-files/confine\"\n\nvar W = write.Mode(0o600)\nvar M = c.Mkdir\nvar D, _ = c.Mkdir(nil, \"d\", 0o700)\n",
		"plain/perm6.go":    pkg + "import \"github.com/birdtable/go-files/trust/trusttest\"\n\nfunc f() { trusttest.Perm(nil, \"p\", 0o700) }\n",
	}
}

// TestTheOsKnowledgeCheckSeesEachKindRed plants a file of each spelling of each
// kind and the controls near them: each plant is red by its own kind and no
// control is reported. Planted failure: a spelling the check does not read, or
// a near miss it reports.
func TestTheOsKnowledgeCheckSeesEachKindRed(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{}
	for p, body := range knowledgePlants() {
		repo[p] = &fstest.MapFile{Data: []byte(body)}
	}
	found, err := knowledgeFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	for p := range knowledgePlants() {
		got, red := found[p]
		switch {
		case strings.HasPrefix(p, "p/") && (!red || got != strings.ToUpper(p[2:3])):
			t.Errorf("the plant %s: kinds %q (red %v), want %q", p, got, red, strings.ToUpper(p[2:3]))
		case strings.HasPrefix(p, "plain/") && red:
			t.Errorf("the control %s is red with kinds %q", p, got)
		}
	}
}

// TestTheOsKnowledgeListIsExact plants each failure of the list: a file with no
// line, a dead line, a line whose kinds are not the file's, a path twice, a line
// out of order, a line that is not path, kinds and ticket, and the list gone
// (every finding unlisted). The tree with its exact list is green.
func TestTheOsKnowledgeListIsExact(t *testing.T) {
	t.Parallel()
	tree := func(files ...string) fstest.MapFS {
		repo := fstest.MapFS{}
		for _, f := range files {
			repo[f] = &fstest.MapFile{Data: []byte(knowledgePlants()[f])}
		}
		return repo
	}
	const a, b = "p/a1.go A the move\n", "p/b7.go B the move\n"
	// bases is the list at the merge base of the cases that hold one.
	bases := map[string]string{
		"a list as the base has it":                 a + b,
		"a list that shrank":                        a + b,
		"a kind that shrank":                        "p/b7.go BC the move\n",
		"a line the base lacks":                     a,
		"a kind the base lacks":                     "p/a1.go B the move\n",
		"a base whose lines end on an issue number": "p/a1.go A #1591\np/b7.go B #1591\n",
		"a kind the numbered base lacks":            "p/a1.go B #1591\n",
	}
	for _, tc := range []struct {
		name string
		repo fstest.MapFS
		list string
		want string // a problem says; none when green
	}{
		{"the tree with its list", tree("p/a1.go", "p/b7.go", "plain/fp.go"), a + b, ""},
		{"a list as the base has it", tree("p/a1.go", "p/b7.go"), a + b, ""},
		{"a list that shrank", tree("p/a1.go"), a, ""},
		{"a base whose lines end on an issue number", tree("p/a1.go", "p/b7.go"), a + b, ""},
		{"a kind the numbered base lacks", tree("p/a1.go"), a, "the merge base has B: the kinds only shrink"},
		{"a kind that shrank", tree("p/b7.go"), b, ""},
		{"a line the base lacks", tree("p/a1.go", "p/b7.go"), a + b, "p/b7.go is a line of " + osKnowledgeList + " that the merge base"},
		{"a kind the base lacks", tree("p/a1.go"), "p/a1.go A the move\n", "the merge base has B: the kinds only shrink"},
		{"a file with no line", tree("p/a1.go", "p/b7.go"), a, "p/b7.go knows that a system exists (kinds B) and is not in"},
		{"the list gone", tree("p/a1.go"), "", "p/a1.go knows that a system exists"},
		{"a dead line", tree("p/a1.go", "plain/fp.go"), a + "plain/fp.go A the move\n", "plain/fp.go is listed in " + osKnowledgeList + " and no longer knows"},
		{"a line for a gone file", tree("p/a1.go"), a + b, "p/b7.go is listed in " + osKnowledgeList + " and no longer knows"},
		{"a kind that moved", tree("p/a1.go"), "p/a1.go AB the move\n", "is listed with kinds AB and now has A"},
		{"a path twice", tree("p/a1.go"), a + a, "p/a1.go is listed twice"},
		{"out of order", tree("p/a1.go", "p/b7.go"), b + a, "is not in path order"},
		{"kinds out of order", tree("p/a1.go"), "p/a1.go BA the move\n", "the kinds BA are not in order"},
		{"a line with no ticket", tree("p/a1.go"), "p/a1.go A\n", "is not <path> <kinds A-F> <what empties it"},
		{"a kind that is not one", tree("p/a1.go"), "p/a1.go G the move\n", "is not <path> <kinds A-F> <what empties it"},
		{"an issue number for the words", tree("p/a1.go"), "p/a1.go A #1591\n", "is not <path> <kinds A-F> <what empties it"},
		{"words that name an issue", tree("p/a1.go"), "p/a1.go A the move, after #1591\n", "is not <path> <kinds A-F> <what empties it"},
	} {
		base, held := bases[tc.name]
		got := knowledgeProblems(tc.repo, tc.list, base, held)
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%s: red: %q", tc.name, got)
		case tc.want != "" && !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.want) }):
			t.Errorf("%s: want a problem saying %q, got %q", tc.name, tc.want, got)
		}
	}
}
