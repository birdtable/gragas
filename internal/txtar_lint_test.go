package internal

// gragas#1604: gragas imports testscript directly for its
// command-line tests, and testscript runs its own [windows], [unix], [linux]
// and [darwin] conditions before any a program adds, so no wrapper can take
// them away. What is left is a scan of the script text. A script of a .txtar
// file (and of a .txt file in a folder named scripts) is the text before its
// first "-- name --" line, and it is refused when a line of it:
//
//   - holds a condition that is not on the allow-list: short, net, gc, go1.N,
//     and exec:<tool> for a tool the list names (scriptTools), any of them
//     negated. Every other condition names a system, an architecture or a
//     tool of one system, as [freebsd], [!plan9], [386], [!symlink] and
//     [!exec:sh] do, and the shells (sh, bash, cmd, powershell) are never on
//     the list;
//   - runs chmod or symlink, which the systems do not do alike.
//
// A script that must differ by system is a test of the program's own
// commands, which carry the difference; a script tests one behaviour, the
// same on every system. The check reads text and carries no system's code
// itself. The files after the first marker are data, a comment line (#) says
// nothing, and the check lists a refused line with its file and number.

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

var (
	// scriptAllowed is a condition that holds on every system alike.
	scriptAllowed = regexp.MustCompile(`^(short|net|gc|go1\.\d+)$`)
	// scriptMarker starts a file of the archive, where the script ends.
	scriptMarker = regexp.MustCompile(`^-- .+ --$`)
)

// scriptTools is the tools a script may ask for by [exec:<tool>]: ones the
// tests' own environment has on every system. No shell is on it.
func scriptTools() []string { return []string{"git", "go", "gofmt"} }

// conditionAllowed says whether the condition name (without its !) holds on
// every system alike.
func conditionAllowed(name string) bool {
	if scriptAllowed.MatchString(name) {
		return true
	}
	tool, ok := strings.CutPrefix(name, "exec:")
	return ok && slices.Contains(scriptTools(), tool)
}

// scriptWord is one word of a script line and whether an unquoted $ is in it,
// which testscript expands before it reads the word as a condition or command.
type scriptWord struct {
	text   string
	dollar bool
}

// scriptWords splits a line into the words testscript reads: separated by
// spaces, tabs and carriage returns, a single quote opens and closes a quoted
// run inside a word (two quotes in one are a quote), so '[darwin]' and
// ch'mod' are the words [darwin] and chmod; an unquoted # ends the line
// wherever it falls (testscript.go:1310, 1317).
func scriptWords(line string) []scriptWord {
	var words []scriptWord
	var cur strings.Builder
	dollar, inWord, quoted := false, false, false
	end := func() {
		if inWord {
			words = append(words, scriptWord{cur.String(), dollar})
			cur.Reset()
			dollar, inWord = false, false
		}
	}
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quoted && r == '\'' && i+1 < len(rs) && rs[i+1] == '\'':
			cur.WriteRune('\'')
			i++
		case quoted && r == '\'':
			quoted = false
		case quoted:
			cur.WriteRune(r)
		case r == '\'':
			quoted, inWord = true, true
		case r == ' ' || r == '\t' || r == '\r':
			end()
		case r == '#':
			end()
			return words
		default:
			if r == '$' {
				dollar = true
			}
			cur.WriteRune(r)
			inWord = true
		}
	}
	end()
	return words
}

// txtarProblems is each refused line of the script text of one file, named
// by path and line number.
func txtarProblems(name, body string) []string {
	var found []string
	for i, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if scriptMarker.MatchString(line) {
			break
		}
		at := name + ":" + strconv.Itoa(i+1) + ": "
		words := scriptWords(line)
		literal := at + "a script names its conditions and its command literally: no $ there"
		// the leading words that are conditions, as testscript reads them
		for len(words) > 0 {
			w := words[0].text
			if words[0].dollar {
				found = append(found, literal)
				words = nil
				break
			}
			if len(w) < 2 || !strings.HasPrefix(w, "[") || !strings.HasSuffix(w, "]") {
				break
			}
			neg, cond := "", w[1:len(w)-1]
			if c, ok := strings.CutPrefix(cond, "!"); ok {
				neg, cond = "!", c
			}
			if !conditionAllowed(cond) {
				found = append(found, at+"the condition ["+neg+cond+"] is not on the allow-list (short, net, gc, go1.N, exec:<"+strings.Join(scriptTools(), "|")+">): a script runs the same on every system")
			}
			words = words[1:]
		}
		for len(words) > 0 && !words[0].dollar && (words[0].text == "!" || words[0].text == "?") {
			words = words[1:]
		}
		switch {
		case len(words) == 0:
		case words[0].dollar:
			found = append(found, literal)
		case words[0].text == "chmod" || words[0].text == "symlink":
			found = append(found, at+words[0].text+" differs between systems: a script does not run it")
		}
	}
	return found
}

// isScript says whether a path is a script of testscript: a .txtar file, or
// a .txt file in a folder named scripts.
func isScript(p string) bool {
	switch path.Ext(p) {
	case ".txtar":
		return true
	case ".txt":
		return path.Base(path.Dir(p)) == "scripts"
	}
	return false
}

// txtarTreeProblems is the problems of every script under repo, in a walk's
// order. It skips the folders no author's file is in.
func txtarTreeProblems(repo fs.FS) ([]string, error) {
	var found []string
	err := fs.WalkDir(repo, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); p != "." && (strings.HasPrefix(n, ".") || n == "node_modules" || n == "vendor") {
				return fs.SkipDir
			}
			return nil
		}
		if !isScript(p) {
			return nil
		}
		body, err := fs.ReadFile(repo, p)
		if err != nil {
			return err
		}
		found = append(found, txtarProblems(p, string(body))...)
		return nil
	})
	return found, err
}

// TestNoScriptNamesASystem: no script of the tree holds a condition on a
// system or a refused command. Risk: a script that tests one system and is
// read as a test of all of them.
func TestNoScriptNamesASystem(t *testing.T) {
	t.Parallel()
	problems, err := txtarTreeProblems(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestTheScriptCheckSeesEachKindRed: each refused line is found, with its
// number, and the controls pass: the same words in a comment, in a file after
// the first marker, and a condition that names no system. Risk: a check that
// reads nothing and is green for ever.
func TestTheScriptCheckSeesEachKindRed(t *testing.T) {
	t.Parallel()
	red := map[string]string{
		"windows":         "exec gragas\n[windows] skip\n",
		"not unix":        "[!unix] skip 'no'\nexec gragas\n",
		"linux":           "[linux] exec gragas\n",
		"darwin":          "# a comment\n[darwin] exec gragas\n",
		"amd64":           "[amd64] exec gragas\n",
		"arm64 negated":   "[!arm64] exec gragas\n",
		"chmod":           "chmod 0755 f\n",
		"guarded chmod":   "[exec:git] chmod 0755 f\n",
		"symlink":         "symlink a -> b\n",
		"carriage lines":  "exec gragas\r\n[windows] skip\r\n",
		"other system":    "[freebsd] skip\n",
		"not plan9":       "[!plan9] exec gragas\n",
		"other arch":      "[386] skip\n",
		"riscv64":         "[riscv64] skip\n",
		"no sh":           "[!exec:sh] skip\n",
		"exec sh":         "[exec:sh] exec sh -c true\n",
		"exec powershell": "[exec:powershell] skip\n",
		"no symlink":      "[!symlink] skip\n",
		"second guard":    "[short] [windows] skip\n",
		// testscript unquotes a word before it takes the conditions off, so
		// a quoted word is read as the word it holds.
		"quoted condition":    "'[darwin]' stop\n",
		"quoted second guard": "[short] '[linux]' skip\n",
		"quoted chmod":        "'chmod' 0755 f\n",
		"quoted not exec sh":  "'[!exec:sh]' skip\n",
		"quoted inside chmod": "ch'mod' 0755 f\n",
		"quoted symlink":      "'symlink' a -> b\n",
		"negated chmod":       "! chmod 0755 f\n",
		// testscript expands $VAR before it reads conditions and the command,
		// and a lone carriage return separates words.
		"expanded condition": "env C=[darwin]\n$C stop\n",
		"expanded command":   "env X=chmod\n$X 0755 f\n",
		"braced second":      "[short] ${C} skip\n",
		"carriage return":    "[darwin]\rstop\n",
		"hash inside a word": "[windows]# a note\n",
		"tabbed condition":   "[short]\t[windows]\tskip\n",
	}
	for name, body := range red {
		if got := txtarProblems(name+".txtar", body); len(got) != 1 {
			t.Errorf("%s: want one problem, got %v", name, got)
		}
	}
	if got := txtarProblems("n.txtar", "exec gragas\n[windows] skip\n"); len(got) != 1 || !strings.HasPrefix(got[0], "n.txtar:2: ") {
		t.Errorf("the line number is not named: %v", got)
	}
	green := map[string]string{
		"plain":           "exec gragas version\nstdout v1\n",
		"comment":         "# [windows] chmod symlink\nexec gragas\n",
		"after marker":    "exec gragas\n-- f --\n[windows] chmod symlink\n",
		"other condition": "[exec:git] exec git version\n[!exec:git] skip\n",
		"allowed ones":    "[short] skip\n[!net] skip\n[gc] [go1.21] exec gragas\n",
		"a word":          "exec gragas chmodel\ncmp stdout symlinked\n",
		"quoted argument": "exec gragas '[windows]' 'chmod'\nstdout 'symlink a'\n",
		"quoted comment":  "exec gragas '#' x\n",
		"a variable":      "exec gragas $WORK/x\ncmp $WORK/a b\n",
	}
	for name, body := range green {
		if got := txtarProblems(name+".txtar", body); len(got) > 0 {
			t.Errorf("%s: want none, got %v", name, got)
		}
	}
	repo := fstest.MapFS{
		"a/testdata/scripts/x.txt": {Data: []byte("[windows] skip\n")},
		"a/testdata/y.txtar":       {Data: []byte("chmod 0755 f\n")},
		"a/testdata/z.txt":         {Data: []byte("[windows] skip\n")},
		".hidden/w.txtar":          {Data: []byte("chmod 0755 f\n")},
	}
	got, err := txtarTreeProblems(repo)
	if err != nil || len(got) != 2 {
		t.Errorf("the walk reads a .txtar and a .txt in scripts only, and no hidden folder: %v, %v", got, err)
	}
}
