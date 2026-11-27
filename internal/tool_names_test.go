package internal

// Gragas names no tool in its Go code. A tool's knowledge is its plugin's, so a
// tool's name in a non-test Go file of the product is red unless the list of today's places holds it, and
// that list only falls. The dev tools, the packages devTools() names (internal/dev_closed_test.go), are not
// read: they build and run the gate's own tools. The names are data
// (gate/tool-names.txt), so no name is written here; the plants below name a tool that does not exist.
//
// A name counts where it stands as a whole word: in an identifier, as one of the identifier's words
// (ESLintVersion holds eslint; tsconfig does not hold tsc), and in a string or a comment, as a word between
// characters that are no letter or digit, or on an identifier's word edge. Case is not read. So a short
// name that is also part of an ordinary word (tsc in tsconfig, a two-letter name inside msg) is no place.
// A spelling that is an ordinary word on its own is not one of a tool's spellings: TypeScript, the language
// gragas generates types in, is not the compiler's name, and the names file says so.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// toolNamesFile is the tools gragas's Go names nowhere outside tests, each with the spellings that count.
const toolNamesFile = "gate/tool-names.txt"

// toolPlacesFile is the places that name one today, each with its reason and what removes it.
const toolPlacesFile = "gate/tool-name-places.txt"

// toolAdapters is the folder of the tools' adapters, while gragas embeds them: each tool there has a line
// in the names file, so a new adapter cannot come in with a name the check does not read. The folder goes
// with the four tools, and then nothing is read here.
const toolAdapters = "internal/host/assets/adapters"

// toolSpelling is one spelling of a tool, lowercased.
type toolSpelling struct{ tool, spelling string }

// parseToolNames is the spellings of the names file, longest first so a longer spelling is matched before a
// shorter one inside it, and its problems: a tool with no spelling, a spelling twice, a file with no tool.
func parseToolNames(text string) (spellings []toolSpelling, problems []string) {
	seen := map[string]string{}
	for line := range strings.Lines(text) {
		line, _, _ = strings.Cut(line, "#")
		f := strings.Fields(strings.ToLower(line))
		if len(f) == 0 || f[0] == "!" {
			continue
		}
		if len(f) == 1 {
			problems = append(problems, toolNamesFile+": "+f[0]+" has no spelling, so nothing of it is read")
		}
		for _, s := range f[1:] {
			if t, ok := seen[s]; ok {
				problems = append(problems, fmt.Sprintf("%s: %s is a spelling of %s and of %s", toolNamesFile, s, t, f[0]))
				continue
			}
			seen[s] = f[0]
			spellings = append(spellings, toolSpelling{f[0], s})
		}
	}
	if len(spellings) == 0 {
		problems = append(problems, toolNamesFile+" names no tool, so the check reads nothing")
	}
	slices.SortStableFunc(spellings, func(a, b toolSpelling) int { return len(b.spelling) - len(a.spelling) })
	return spellings, problems
}

// toolAlnum is a letter or a digit of ASCII, and toolDigit a digit.
func toolAlnum(c byte) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' || toolDigit(c) }

func toolDigit(c byte) bool { return '0' <= c && c <= '9' }

// toolUpper is a capital of ASCII.
func toolUpper(c byte) bool { return 'A' <= c && c <= 'Z' }

// toolEdge says whether s has a word's edge before s[i]: an end of s, a character that is no letter or digit
// on either side, or an edge between an identifier's words: a capital after a small letter or a digit
// (eslintFile), the last capital of a run that a small letter follows (TSCVersion), a digit beside a letter.
func toolEdge(s string, i int) bool {
	if i == 0 || i == len(s) || !toolAlnum(s[i-1]) || !toolAlnum(s[i]) {
		return true
	}
	a, b := s[i-1], s[i]
	return toolUpper(b) && !toolUpper(a) ||
		toolUpper(a) && toolUpper(b) && i+1 < len(s) && toolAlnum(s[i+1]) && !toolUpper(s[i+1]) && !toolDigit(s[i+1]) ||
		toolDigit(a) != toolDigit(b)
}

// toolWords is each place in s where a spelling stands as a whole word, as written in s, with its tool. A
// spelling is matched without regard to case, starts and ends on a word's edge, and takes no letter a
// longer spelling took.
func toolWords(s string, spellings []toolSpelling) (words, tools []string) {
	low := strings.ToLower(s)
	taken := make([]bool, len(s))
	for _, sp := range spellings {
		for from := 0; ; {
			k := strings.Index(low[from:], sp.spelling)
			if k < 0 {
				break
			}
			i, j := from+k, from+k+len(sp.spelling)
			from = i + 1
			if !toolEdge(s, i) || !toolEdge(s, j) || slices.Contains(taken[i:j], true) {
				continue
			}
			for x := i; x < j; x++ {
				taken[x] = true
			}
			words, tools = append(words, s[i:j]), append(tools, sp.tool)
		}
	}
	return words, tools
}

// toolKind is where a name stands as a finding says it, from the word an entry of the list spells it with; "" for a
// word that is no kind.
func toolKind(kind string) string {
	switch kind {
	case "identifier":
		return "in an identifier"
	case "string", "comment":
		return "in a " + kind
	}
	return ""
}

// toolPlacesOf is every place a spelling stands in one Go file, each as "<kind> <text>\x00<tool>": the kind
// is identifier, string (a struct tag and an import path are strings) or comment, and the text is the
// identifier whole, or the word as the string or comment writes it.
func toolPlacesOf(body []byte, spellings []toolSpelling) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", body, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return []string{"parse\x00" + err.Error()}
	}
	var out []string
	add := func(kind, s string, whole bool) {
		words, tools := toolWords(s, spellings)
		for i, w := range words {
			if whole {
				w = s
			}
			out = append(out, kind+" "+w+"\x00"+tools[i])
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			add("identifier", n.Name, true)
		case *ast.BasicLit:
			if s, err := strconv.Unquote(n.Value); n.Kind == token.STRING && err == nil {
				add("string", s, false)
			}
		}
		return true
	})
	for _, g := range f.Comments {
		for _, c := range g.List {
			add("comment", c.Text, false)
		}
	}
	return out
}

// toolReason is a listed place's reason, after its #: plain words that say why
// the name stands there and what removes it, naming no issue by its number.
var toolReason = regexp.MustCompile(`^[a-z][^#\x00-\x1f\x7f]*$`)

// toolNameProblems holds repo's Go to the names and the list of places: every place a name stands in a
// non-test Go file, under the walk the OS checks take (osFilesBy), is a line of the list; every line is a
// place, with its reason and what removes it; and, when the list at the merge base is read
// (held), no line is one the base lacks, counted as often as it stands (difference, as the suppressions
// list is held). A file of a dev package, a folder dev names (not its subfolders, which are packages of
// their own), is not read. Each problem says what to do.
func toolNameProblems(repo fs.FS, names, list, base string, held bool, dev []string) []string {
	spellings, problems := parseToolNames(names)
	if len(spellings) == 0 {
		return problems
	}
	byFile, err := osFilesBy(repo, func(name string, body []byte) []string {
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		return toolPlacesOf(body, spellings)
	})
	if err != nil {
		return append(problems, "the Go files could not be read: "+err.Error())
	}
	var found []string
	tool := map[string]string{}
	for _, p := range slices.Sorted(maps.Keys(byFile)) {
		if slices.Contains(dev, path.Dir(p)) {
			continue
		}
		for _, x := range byFile[p] {
			place, t, _ := strings.Cut(x, "\x00")
			if place == "parse" {
				problems = append(problems, p+" does not parse, so the names in it cannot be read: "+t)
				continue
			}
			found = append(found, p+" "+place)
			tool[p+" "+place] = t
		}
	}
	listed, _ := parseListed(list)
	for line := range strings.Lines(list) {
		entry, reason, _ := strings.Cut(line, "#")
		if entry = strings.Join(strings.Fields(entry), " "); entry == "" {
			continue
		}
		if f := strings.Fields(entry); len(f) != 3 || toolKind(f[1]) == "" {
			problems = append(problems, toolPlacesFile+": "+entry+": a line is <path> <identifier|string|comment> <what stands there> # <reason and what removes it>")
		}
		if !toolReason.MatchString(strings.TrimSpace(reason)) {
			problems = append(problems, toolPlacesFile+": "+entry+": a listed place gives its reason and what removes it, in plain words with no issue number")
		}
	}
	for _, x := range difference(found, listed) {
		f := strings.SplitN(x, " ", 3)
		problems = append(problems, fmt.Sprintf("%s names the tool %s %s (%s), and %s does not list it: gragas's Go names no tool outside tests, so the tool's knowledge goes in its plugin, never in Go", f[0], tool[x], toolKind(f[1]), f[2], toolPlacesFile))
	}
	for _, x := range difference(listed, found) {
		problems = append(problems, x+": is listed in "+toolPlacesFile+", and the file no longer names the tool there as often; take the line off")
	}
	if held {
		was, _ := parseListed(base)
		for _, x := range difference(listed, was) {
			problems = append(problems, x+": is in "+toolPlacesFile+" and not in the list at the merge base; the list only falls, so take the name out of the Go instead")
		}
	}
	return problems
}

// toolAdapterProblems is each tool of the adapters folder of repo that the names file does not name.
func toolAdapterProblems(repo fs.FS, names string) []string {
	spellings, _ := parseToolNames(names)
	entries, err := fs.ReadDir(repo, toolAdapters)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !slices.ContainsFunc(spellings, func(s toolSpelling) bool { return s.tool == strings.ToLower(e.Name()) }) {
			out = append(out, path.Join(toolAdapters, e.Name())+" is a tool's adapter, and "+toolNamesFile+" does not name it: give it its line, so the check reads its name")
		}
	}
	return out
}

// toolListProblems holds the names file to the source test's list of tools (source.tools of layers.yaml)
// until one of them is left: the two differ by exactly the names of the file's ! line, each of
// which is in the source list and not a tool of the file, and by nothing else.
func toolListProblems(names string, source []string) []string {
	spellings, _ := parseToolNames(names)
	var tools, left []string
	for _, s := range spellings {
		tools = append(tools, s.tool)
	}
	for line := range strings.Lines(names) {
		line, _, _ = strings.Cut(line, "#")
		if f := strings.Fields(strings.ToLower(line)); len(f) > 0 && f[0] == "!" {
			left = append(left, f[1:]...)
		}
	}
	slices.Sort(tools)
	tools = slices.Compact(tools)
	var out []string
	for _, s := range source {
		if !slices.Contains(tools, s) && !slices.Contains(left, s) {
			out = append(out, s+" is in source.tools of layers.yaml, and "+toolNamesFile+" neither names it nor leaves it out on its ! line: give it its line, or put it on the ! line")
		}
	}
	for _, l := range left {
		switch {
		case slices.Contains(tools, l):
			out = append(out, l+" is left out on the ! line of "+toolNamesFile+" and is a tool of the file as well: it is one or the other")
		case !slices.Contains(source, l):
			out = append(out, l+" is left out on the ! line of "+toolNamesFile+", and source.tools of layers.yaml does not hold it: the lists differ by that line alone")
		}
	}
	for _, t := range tools {
		if !slices.Contains(source, t) {
			out = append(out, t+" is a tool of "+toolNamesFile+", and source.tools of layers.yaml does not hold it: the lists differ by the ! line alone, so give it its line there too")
		}
	}
	return out
}

// TestNoToolIsNamedInGragasGo is the check a push runs, from the member gate-lists:
// the tree's Go names no tool but where the list of places holds it, the list holds no place that is gone,
// and every tool of the adapters folder has its names. The list is held to the list at the newest of its
// merge bases with origin/main and origin/release-1; while neither base holds the list, no base is read, and the test says so.
func TestNoToolIsNamedInGragasGo(t *testing.T) {
	t.Parallel()
	repo := os.DirFS(root)
	names, err := fs.ReadFile(repo, toolNamesFile)
	if err != nil {
		t.Fatal(err)
	}
	list, err := fs.ReadFile(repo, toolPlacesFile)
	if err != nil {
		t.Fatal(err)
	}
	base, why, missing := osListBase(t, root, toolPlacesFile, "refs/remotes/origin/release-1")
	switch {
	case missing:
		t.Log("the list is at no merge base yet, so it is held to none: " + why)
	case why != "":
		t.Fatal(why)
	}
	for _, p := range toolNameProblems(repo, string(names), string(list), base, why == "", devTools()) {
		t.Error(p)
	}
	for _, p := range toolAdapterProblems(repo, string(names)) {
		t.Error(p)
	}
	if listed, _ := parseListed(string(list)); len(listed) == 0 {
		t.Log(toolPlacesFile + " holds no place: once the tools have left, the file goes and the test stays")
	}
}

// TestAToolNameIsRedInEachPlace plants each way a name comes back and each way the list grows, with a tool
// that does not exist, and the controls the check lets by: a test file that names the tool, a word the
// short name is only part of, and a line gone from the list with its place.
func TestAToolNameIsRedInEachPlace(t *testing.T) {
	t.Parallel()
	const names = "# the tools\nfakelint fakelint fakelintrc\nzq zq\n"
	const reason = " # it goes with the plugin\n"
	const clean = "package p\n\n// Package p lints nothing; it reads zqconfig and msgzq, which hold no tool's name.\nvar zqconfig, msgzq = 1, 2\n"
	held := "p/a.go comment fakelint" + reason
	tree := func(files ...string) fstest.MapFS {
		m := fstest.MapFS{"p/clean.go": {Data: []byte(clean)}, "p/x_test.go": {Data: []byte("package p\n\n// fakelint and zq, named by a test.\nvar FakelintVersion = \"zq\"\n")}}
		for i := 0; i+1 < len(files); i += 2 {
			m[files[i]] = &fstest.MapFile{Data: []byte(files[i+1])}
		}
		return m
	}
	listed := tree("p/a.go", "package p\n\n// fakelint is run here.\nvar A = 1\n")
	for _, tc := range []struct {
		name       string
		repo       fstest.MapFS
		list, base string
		want       []string
	}{
		{"a listed place, as the base holds it", listed, held, held, nil},
		{"no Go names a tool, and the list is empty", tree(), "", "", nil},
		{"a place gone, with its line", tree(), "", held, nil},
		{"a new name in a string", tree("p/b.go", "package p\n\nvar B = \"run fakelint now\"\n"), "", "",
			[]string{`p/b.go names the tool fakelint in a string (fakelint), and gate/tool-name-places.txt does not list it`}},
		{"a new name in an identifier", tree("p/b.go", "package p\n\nvar FakelintVersion = 1\n"), "", "",
			[]string{`p/b.go names the tool fakelint in an identifier (FakelintVersion), and gate/tool-name-places.txt does not list it`}},
		{"a short name as an identifier's first word, in capitals", tree("p/b.go", "package p\n\nvar ZQVersion, msgZQ2 = 1, 2\n"), "", "",
			[]string{`p/b.go names the tool zq in an identifier (ZQVersion)`, `p/b.go names the tool zq in an identifier (msgZQ2)`}},
		{"a new name in a comment", tree("p/b.go", "package p\n\n// B is the ZQ of it.\nvar B = 1\n"), "", "",
			[]string{`p/b.go names the tool zq in a comment (ZQ), and gate/tool-name-places.txt does not list it`}},
		{"a config file's spelling, longest first", tree("p/b.go", "package p\n\nvar B = \".fakelintrc\"\n"), "", "",
			[]string{`p/b.go names the tool fakelint in a string (fakelintrc)`}},
		{"a listed place used once more", tree("p/a.go", "package p\n\n// fakelint is run here, and fakelint again.\nvar A = 1\n"), held, held,
			[]string{`p/a.go names the tool fakelint in a comment (fakelint), and gate/tool-name-places.txt does not list it`}},
		{"a listed place whose name is gone but whose line stays", tree("p/a.go", "package p\n\n// a linter is run here.\nvar A = 1\n"), held, held,
			[]string{"p/a.go comment fakelint: is listed in gate/tool-name-places.txt, and the file no longer names the tool there as often; take the line off"}},
		{"a line added to the list that the base lacks", listed, held, "",
			[]string{"p/a.go comment fakelint: is in gate/tool-name-places.txt and not in the list at the merge base; the list only falls"}},
		{"a line moved to another file", tree("p/b.go", "package p\n\n// fakelint is run here.\nvar B = 1\n"), "p/b.go comment fakelint" + reason, held,
			[]string{"p/b.go comment fakelint: is in gate/tool-name-places.txt and not in the list at the merge base"}},
		{"a line with no reason", listed, "p/a.go comment fakelint #\n", held,
			[]string{"gate/tool-name-places.txt: p/a.go comment fakelint: a listed place gives its reason and what removes it"}},
		{"a line with no #", listed, "p/a.go comment fakelint\n", held,
			[]string{"gate/tool-name-places.txt: p/a.go comment fakelint: a listed place gives its reason and what removes it"}},
		{"a reason that names an issue by its number", listed, "p/a.go comment fakelint # it goes with the plugin (gragas#1)\n", held,
			[]string{"gate/tool-name-places.txt: p/a.go comment fakelint: a listed place gives its reason and what removes it, in plain words with no issue number"}},
		{"a line that is no place", listed, held + "p/a.go fakelint" + reason, held + "p/a.go fakelint" + reason,
			[]string{"gate/tool-name-places.txt: p/a.go fakelint: a line is <path> <identifier|string|comment>", "p/a.go fakelint: is listed in"}},
		{"a file that does not parse", tree("p/b.go", "package p\n\nvar = \n"), "", "",
			[]string{"p/b.go does not parse"}},
	} {
		got := toolNameProblems(tc.repo, names, tc.list, tc.base, true, nil)
		if len(got) != len(tc.want) {
			t.Errorf("%s: %q, want %d problems", tc.name, got, len(tc.want))
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i], w) {
				t.Errorf("%s: %q, want it to say %q", tc.name, got[i], w)
			}
		}
	}
	if got := toolNameProblems(listed, names, held, "", false, nil); len(got) != 0 {
		t.Errorf("a line the base lacks, with no base read: %q, want none", got)
	}
	// The dev packages, both ways: a dev package's file may name a tool; the same file outside one, and a
	// subfolder of a dev package, which is a package of its own, may not.
	const named = "package d\n\n// fakelint is run here.\nvar D = 1\n"
	for _, tc := range []struct {
		name, file string
		dev        []string
		red        bool
	}{
		{"a dev package", "cmd/d/main.go", []string{"cmd/d"}, false},
		{"a product package", "cmd/d/main.go", []string{"cmd/e"}, true},
		{"a subfolder of a dev package", "cmd/d/sub/x.go", []string{"cmd/d"}, true},
	} {
		got := toolNameProblems(tree(tc.file, named), names, "", "", true, tc.dev)
		if red := slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.file+" names the tool fakelint") }); red != tc.red || len(got) > 1 {
			t.Errorf("%s naming a tool: %q, want red %v", tc.name, got, tc.red)
		}
	}
	for _, tc := range []struct{ name, names, want string }{
		{"no tool", "# nothing\n", "names no tool, so the check reads nothing"},
		{"a tool with no spelling", "fakelint\nzq zq\n", "fakelint has no spelling"},
		{"a spelling twice", "fakelint fakelint zq\nzq zq\n", "zq is a spelling of fakelint and of zq"},
	} {
		if got := toolNameProblems(listed, tc.names, held, held, true, nil); !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, tc.want) }) {
			t.Errorf("the names file with %s: %q, want it to say %q", tc.name, got, tc.want)
		}
	}
	adapters := fstest.MapFS{toolAdapters + "/fakelint/1.0.0.yaml": {}, toolAdapters + "/other/1.0.0.yaml": {}, toolAdapters + "/.placeholder": {}}
	if got := toolAdapterProblems(adapters, names); len(got) != 1 || !strings.Contains(got[0], toolAdapters+"/other is a tool's adapter, and "+toolNamesFile+" does not name it") {
		t.Errorf("an adapter the names file does not name: %q, want one problem for other", got)
	}
	if got := toolAdapterProblems(fstest.MapFS{}, names); len(got) != 0 {
		t.Errorf("no adapters folder: %q, want none", got)
	}
}

// TestTheToolListsDifferByTheLeftOut holds the two lists of tools to each other until one of them is
// left: the tree's names file and source.tools of layers.yaml
// differ by the names file's ! line alone, and each way they could drift apart is red, with a tool that
// does not exist.
func TestTheToolListsDifferByTheLeftOut(t *testing.T) {
	t.Parallel()
	names, err := os.ReadFile(path.Join(root, toolNamesFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range toolListProblems(string(names), table(t).Source.Tools) {
		t.Error(p)
	}
	const file = "fakelint fakelint fakelintrc\n! zq # the gate's own\n"
	for _, tc := range []struct {
		name, names string
		source      []string
		want        string
	}{
		{"the lists as they should be", file, []string{"fakelint", "zq"}, ""},
		{"a source tool neither named nor left out", file, []string{"fakelint", "zq", "qx"}, "qx is in source.tools of layers.yaml, and gate/tool-names.txt neither names it nor leaves it out"},
		{"a left-out name the source lacks", file, []string{"fakelint"}, "zq is left out on the ! line of gate/tool-names.txt, and source.tools of layers.yaml does not hold it"},
		{"a left-out name that is a tool as well", file + "zq zq\n", []string{"fakelint", "zq"}, "zq is left out on the ! line of gate/tool-names.txt and is a tool of the file as well"},
		{"a tool the source lacks", file, []string{"zq"}, "fakelint is a tool of gate/tool-names.txt, and source.tools of layers.yaml does not hold it"},
	} {
		got := toolListProblems(tc.names, tc.source)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: %q, want none", tc.name, got)
			}
			continue
		}
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			t.Errorf("%s: %q, want one problem saying %q", tc.name, got, tc.want)
		}
	}
	// The ! line is no tool: its names are no spelling, so a Go file that names one is no place.
	if spellings, _ := parseToolNames(file); slices.ContainsFunc(spellings, func(s toolSpelling) bool { return s.spelling == "zq" || s.tool == "!" }) {
		t.Errorf("the ! line read as a tool: %v", spellings)
	}
}
