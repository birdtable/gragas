package root

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// initLike is a command with a repeatable --import and a plain --name, set
// to the values given, as gragas init's flags are.
func initLike(t *testing.T, name string, imports ...string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "init", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("name", "", "")
	c.Flags().StringArray("import", nil, "")
	if err := c.Flags().Set("name", name); err != nil {
		t.Fatal(err)
	}
	for _, v := range imports {
		if err := c.Flags().Set("import", v); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// TestTheCommandLineQuotesItsWholeArgvOnce (A5, A6): commandLine
// makes one shell.Quote call over the whole argv, so a value with '=' after
// --import is a plain word, as the first-word rule reads only the program's
// name, and the line splits back into the same words. Quoting each value on
// its own read every value as a first word and quoted 'a=b' (the two_imports
// journey of TestInitTheApplyLineItPrintsWritesThePlanItWasMadeFor).
// Risk: a line that pastes as other words, or quotes a word the shell reads
// as it is.
func TestTheCommandLineQuotesItsWholeArgvOnce(t *testing.T) {
	t.Parallel()
	c := initLike(t, "my rules", "other@1.0.0=go=sha256:ab", "third@2.1.0=ts,go=sha256:cd")
	line, err := commandLine(shell.POSIX, c, initRulebook, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := initRulebook + " --import other@1.0.0=go=sha256:ab --import third@2.1.0=ts,go=sha256:cd --name 'my rules'"
	if line != want {
		t.Fatalf("line is %s\nwant %s", line, want)
	}
	got, err := splitLine(line)
	wantWords := append(strings.Fields(initRulebook), "--import", "other@1.0.0=go=sha256:ab", "--import", "third@2.1.0=ts,go=sha256:cd", "--name", "my rules")
	if err != nil || !slices.Equal(got, wantWords) {
		t.Fatalf("split %q: got %q, %v, want %q", line, got, err, wantWords)
	}
}

// TestAValueNoLineCanCarryGivesNoLineAndNamesIt (A5, A6): a
// value with a newline, a carriage return or a NUL is no word of any line a
// person pastes, so commandLine and applies give no line, and the reason
// names the value, escaped. So does a tab, ESC or DEL, which the screen does
// not show as written. Risk: a line a newline
// splits into two commands, a line quoted some other way that runs what the
// person did not type, or a line that shows ab.txt and runs on a<TAB>b.txt.
func TestAValueNoLineCanCarryGivesNoLineAndNamesIt(t *testing.T) {
	t.Parallel()
	for name, v := range map[string]string{
		"a newline":         "a\nrm -rf x",
		"a carriage return": "a\rb",
		"a NUL":             "a\x00b",
		"a tab":             "a\tb",
		"an ESC":            "a\x1b[2Kb",
		"a DEL":             "a\x7fb",
	} {
		c := initLike(t, v)
		if line, err := commandLine(shell.POSIX, c, initRulebook, nil, nil); err == nil || line != "" || !strings.Contains(err.Error(), strconv.Quote(v)) || strings.Contains(err.Error(), v) {
			t.Errorf("%s: commandLine = %q, %v; want no line and the value named escaped", name, line, err)
		}
		if line, err := applies(shell.POSIX, c, []string{"go.errors.2"}); err == nil || line != "" || !strings.Contains(err.Error(), strconv.Quote(v)) {
			t.Errorf("%s: applies = %q, %v; want no line and the value named", name, line, err)
		}
		// the Liner journal restore is handed: a path of a cloned repository
		if line, err := pasteLine(shell.POSIX, []string{"rm", "--", v}); err == nil || line != "" || !strings.Contains(err.Error(), strconv.Quote(v)) {
			t.Errorf("%s: the way-back line = %q, %v; want no line and the path named", name, line, err)
		}
	}
}

// TestEachDialectQuotesOrRefusesAsShellQuoteDoes:
// the root's one line maker writes each line for the dialect it is handed,
// as go-console's shell.Quote writes it, over the whole argv at once. In Cmd,
// a % or a ! is expanded inside any quoting, so such a word gives no line,
// and the reason names it escaped and wraps shell.ErrCmdChar. A word the
// screen would not show as written gives no line in any dialect. A session
// writes its lines for the dialect main handed it, and for POSIX when main
// handed none. A line whose first word is a program other than gragas is
// given for POSIX only: PowerShell and cmd give none, ErrOtherProgram, and a
// dialect this build does not know gives none for any argv.
// The parent process is never asked: each dialect is handed in.
// PowerShell's lines are checked by these rows only until a live pwsh 7 run.
// Risk: a line that a Windows shell expands into another command, or a line
// quoted for one shell and pasted into another.
func TestEachDialectQuotesOrRefusesAsShellQuoteDoes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		d    shell.Dialect
		argv []string
		want string
	}{
		{"POSIX, a spaced value", shell.POSIX, []string{"gragas", "rule", "new", "--name", "my rules", "--apply"}, "gragas rule new --name 'my rules' --apply"},
		{"POSIX, a quote", shell.POSIX, []string{"cp", "--", "/state/a b/c", "it's.txt"}, `cp -- '/state/a b/c' 'it'\''s.txt'`},
		{"POSIX, a %", shell.POSIX, []string{"gragas", "gen", "-C", "50%off"}, "gragas gen -C 50%off"},
		{"Cmd, a spaced value", shell.Cmd, []string{"gragas", "rule", "new", "--name", "my rules", "--apply"}, `gragas rule new --name ^"my rules^" --apply`},
		{"Cmd, a quote", shell.Cmd, []string{"gragas", "--", "/state/a b/c", "it's.txt"}, `gragas -- ^"/state/a b/c^" it's.txt`},
		{"Cmd, an import with =", shell.Cmd, []string{"gragas", "init", "--import", "other@1.0.0=go=sha256:ab"}, "gragas init --import other@1.0.0=go=sha256:ab"},
		{"PowerShell, a spaced value", shell.PowerShell, []string{"gragas", "rule", "new", "--name", "my rules", "--apply"}, "gragas rule new --name 'my rules' --apply"},
		{"PowerShell, a quote", shell.PowerShell, []string{"gragas", "--", "/state/a b/c", "it's.txt"}, `gragas -- '/state/a b/c' 'it''s.txt'`},
		{"PowerShell, a $ and a backtick", shell.PowerShell, []string{"gragas", "gen", "-C", word(t, "ps-dollar-backtick")}, word(t, "ps-dollar-line")},
	} {
		quoted, err := shell.Quote(c.d, c.argv)
		if err != nil || quoted != c.want {
			t.Fatalf("%s: shell.Quote = %q, %v; the row wants %q", c.name, quoted, err, c.want)
		}
		if line, err := pasteLine(c.d, c.argv); err != nil || line != c.want {
			t.Errorf("%s: pasteLine = %q, %v; want %q", c.name, line, err, c.want)
		}
	}
	for _, v := range []string{"50%off", "wow!"} {
		line, err := pasteLine(shell.Cmd, []string{"gragas", "gen", "-C", v})
		if line != "" || !errors.Is(err, shell.ErrCmdChar) || !strings.Contains(err.Error(), strconv.Quote(v)) {
			t.Errorf("Cmd, %q: pasteLine = %q, %v; want no line, the value named and ErrCmdChar", v, line, err)
		}
	}
	for _, d := range []shell.Dialect{shell.POSIX, shell.Cmd, shell.PowerShell} {
		for _, v := range []string{"a\nb", "a\tb", "a\x1bb"} {
			if line, err := pasteLine(d, []string{"gragas", "gen", "-C", v}); line != "" || err == nil || !strings.Contains(err.Error(), strconv.Quote(v)) {
				t.Errorf("dialect %d, %q: pasteLine = %q, %v; want no line and the value named", d, v, line, err)
			}
		}
	}
	argv := []string{"gragas", "rule", "new", "--name", "my rules"}
	// a dialect this build does not know, as go-console may add, gives no
	// line even for gragas: never POSIX's quoting by default
	for _, a := range [][]string{{"gragas", "gen"}, {"cp", "--", "a"}} {
		if line, err := pasteLine(shell.Dialect(99), a); line != "" || !errors.Is(err, model.ErrInWords) || errors.Is(err, model.ErrOtherProgram) {
			t.Errorf("an unknown dialect, %q: pasteLine = %q, %v; want no line and ErrInWords, not ErrOtherProgram", a, line, err)
		}
	}
	for d, want := range map[shell.Dialect]string{
		shell.POSIX:      `cp -- '/state/a b/c' 'it'\''s.txt'`,
		shell.Cmd:        "",
		shell.PowerShell: "",
	} {
		line, err := pasteLine(d, []string{"cp", "--", "/state/a b/c", "it's.txt"})
		if want == "" && (line != "" || !errors.Is(err, model.ErrOtherProgram)) || want != "" && (err != nil || line != want) {
			t.Errorf("%s, a program other than gragas: pasteLine = %q, %v; want %q, or no line and ErrOtherProgram", d, line, err, want)
		}
	}
	for d, want := range map[shell.Dialect]string{0: "gragas rule new --name 'my rules'", shell.Cmd: `gragas rule new --name ^"my rules^"`} {
		s := &session{p: Process{Dialect: d}}
		if line, err := s.line(argv); err != nil || line != want {
			t.Errorf("a session handed dialect %d: line = %q, %v; want %q", d, line, err, want)
		}
	}
}

// saidLines is a terminal that keeps each line said on it.
type saidLines struct {
	port.Terminal
	said []string
}

func (t *saidLines) Say(line string) error { t.said = append(t.said, line); return nil }

// TestAStepGivenNoLineIsSaidInWordsAndWhyOnce: a
// shell gragas knows no safe quoting for gives no line at all, and a dialect
// this build does not know gives none. Each is model.ErrInWords, not a
// refusal: a gragas command is said in words in the line's place, the reason
// once on standard error, and any other program's step is the caller's to
// say. The reason shows the kit's text, which names the shell, only as %q,
// and not at all when it holds ESC or a bidi control. Risk: a pasted line
// that a shell taken for POSIX runs as other words, a run that fails where it
// only lacks a line, or a shell's name that drives the person's terminal.
func TestAStepGivenNoLineIsSaidInWordsAndWhyOnce(t *testing.T) {
	t.Parallel()
	bidi := string(rune(0x202e))
	words := `run gragas with the words "gen", "-C", "a\\b"`
	for _, c := range []struct {
		name, why string
		p         Process
	}{
		{"a shell with no safe quoting", `your shell has no safe quoting gragas can use ("the shell gragas was started from: here: fish.exe has no dialect"); set GRAGAS_SHELL`, Process{NoQuoting: errors.New("the shell gragas was started from: here: fish.exe has no dialect")}},
		{"a shell whose name holds ESC [", "(its name holds a character the screen does not show as written); set GRAGAS_SHELL", Process{NoQuoting: errors.New("here: \x1b[2Jfish.exe has no dialect")}},
		{"a shell whose name holds a bidi control", "(its name holds a character the screen does not show as written); set GRAGAS_SHELL", Process{NoQuoting: errors.New("here: " + bidi + "exe.hsif has no dialect")}},
		{"a dialect this build does not know", "knows no quoting for shell 99", Process{Dialect: shell.Dialect(99)}},
	} {
		term := &saidLines{}
		c.p.Diagnostics = term
		s := &session{p: c.p}
		for range 2 {
			if line, err := s.line([]string{"gragas", "gen", "-C", word(t, "a-b")}); err != nil || line != words {
				t.Errorf("%s: line = %q, %v; want %q", c.name, line, err, words)
			}
		}
		if len(term.said) != 1 || !strings.Contains(term.said[0], c.why) {
			t.Errorf("%s: said %q; want the reason once, holding %q", c.name, term.said, c.why)
		}
		if strings.ContainsAny(strings.Join(term.said, ""), "\x1b"+bidi) {
			t.Errorf("%s: said %q; want no ESC and no bidi control on the terminal", c.name, term.said)
		}
		if line, err := s.line([]string{"cp", "--", word(t, "a-b"), "c"}); line != "" || !errors.Is(err, model.ErrInWords) {
			t.Errorf("%s, cp: line = %q, %v; want no line and ErrInWords, for the caller to say", c.name, line, err)
		}
	}

	for d, applies := range map[shell.Dialect]string{
		shell.POSIX:      `gragas migrate --owner 'O'\''Neil A' --apply`,
		shell.PowerShell: `gragas migrate --owner 'O''Neil A' --apply`,
		shell.Cmd:        `gragas migrate --owner ^"O'Neil A^" --apply`,
	} {
		s := &session{p: Process{Dialect: d}}
		record, line, err := s.planLines([]string{"gragas", "migrate", "--owner", "O'Neil A"})
		if err != nil || record != `gragas migrate --owner 'O'\''Neil A'` || line != applies {
			t.Errorf("%s: planLines = %q, %q, %v; want the POSIX record and %q", d, record, line, err, applies)
		}
	}
}

// TestAWordPowerShellPassesWrongIsRefusedByItsIndex: in
// PowerShell, go-console refuses a word Windows PowerShell 5.1 hands a
// program as another argv whatever the quoting, and the line maker gives no
// line, naming the word's index in argv and the word as %q, so the caller
// prints the help in its place. The refused words follow go-console v0.12.0's
// own rows: a double quote, straight or curly; an empty word; --%; and a word
// holding a space that ends in a backslash. A word only like a flag, a word
// starting with an en dash and a trailing backslash with no space are quoted,
// not refused. A tab or U+180E is refused before PowerShell is asked, in every
// shell (TestAControlOrInvisibleCharacterIsRefusedByItsIndex). Risk: a pasted
// line whose program reads an added flag or a dropped word, or a refusal that
// names the wrong word.
func TestAWordPowerShellPassesWrongIsRefusedByItsIndex(t *testing.T) {
	t.Parallel()
	for _, w := range []string{`a"b`, "\u201cx\u201d", "\u201ex\u201f", "", "--%", "a b\\"} {
		line, err := pasteLine(shell.PowerShell, []string{"gragas", "rule", "new", "--name", w})
		want := "the argument at index 4 can't be passed safely to PowerShell: " + strconv.Quote(w)
		if line != "" || !errors.Is(err, shell.ErrPowerShellArg) || errors.Is(err, model.ErrInWords) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: pasteLine = %q, %v; want no line and the refusal %q", w, line, err, want)
		}
		if _, err := pasteLine(shell.POSIX, []string{"gragas", "rule", "new", "--name", w}); errors.Is(err, shell.ErrPowerShellArg) {
			t.Errorf("%q: POSIX refused it as PowerShell's: %v", w, err)
		}
	}
	for w, want := range map[string]string{
		"-o:-x:--apply":      `gragas rule new --name '-o:-x:--apply'`,
		"\u2013x":            "gragas rule new --name '\u2013x'",
		word(t, "drive-dir"): word(t, "drive-dir-line"),
	} {
		if line, err := pasteLine(shell.PowerShell, []string{"gragas", "rule", "new", "--name", w}); err != nil || line != want {
			t.Errorf("%q: pasteLine = %q, %v; want %q", w, line, err, want)
		}
	}
}

// TestAControlOrInvisibleCharacterIsRefusedByItsIndex: a word holding a tab, or U+180E that the screen draws as
// nothing, gets no line to paste in any shell, and the refusal names the
// word's index in argv, never the index of the kit's probe, with the word as
// %q so the character shows escaped. It is the control refusal, not
// PowerShell's, even where the word also ends in a backslash. Risk: a pasted
// line that runs other words than it shows, or a refusal that names the wrong
// argument.
func TestAControlOrInvisibleCharacterIsRefusedByItsIndex(t *testing.T) {
	t.Parallel()
	words := []string{"a\tb", "a\tb\\", "a\u180eb", "a\u180eb\\", word(t, "dollar-tab")}
	dialects := []shell.Dialect{shell.POSIX, shell.PowerShell, shell.Cmd}
	seen := 0
	for _, d := range dialects {
		for _, w := range words {
			seen++
			line, err := pasteLine(d, []string{"gragas", "rule", "new", "--name", w})
			want := "the argument at index 4 holds a character no line to paste can carry: " + strconv.Quote(w)
			if line != "" || !errors.Is(err, shell.ErrControl) || errors.Is(err, shell.ErrPowerShellArg) || errors.Is(err, model.ErrInWords) || !strings.HasPrefix(err.Error(), want) {
				t.Errorf("dialect %d, %q: pasteLine = %q, %v; want no line and the refusal %q", d, w, line, err, want)
			}
			if err != nil && strings.Contains(err.Error(), "argument 1") {
				t.Errorf("dialect %d, %q: %v names the probe's index, not argv's", d, w, err)
			}
		}
	}
	if seen != len(dialects)*len(words) {
		t.Fatalf("ran %d cases, want %d", seen, len(dialects)*len(words))
	}
}
