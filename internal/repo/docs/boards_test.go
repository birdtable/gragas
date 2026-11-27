package docs

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// The terminal boards of docs/boards held to the look: typed front matter, at
// most 80 columns, a closing word from the closed set with the exit code the
// exit table gives it, a line saying what answers each question, and printed
// text with no semicolon and none of the names a person never says
// (cli.look.5, docs.glossary.4, docs.voice.1). Like the other document checks
// it is a function of a docWorld, so it is also run with a breach planted.

const boardWidth = 80

var (
	boardFence = regexp.MustCompile("(?s)```text\n(.*?)```\n+([^\n]*)\n([^\n]*)")
	boardEnds  = regexp.MustCompile(`^Ends: (nothing yet|[a-z ]+?)(?:, exit (\d))?[.,]`)
	boardAsk   = regexp.MustCompile(`^Answered by: \S`)
	// names a person never says: a package-qualified name, a camel-case
	// identifier, a path into the code or the design notes, a register id.
	boardJargon = regexp.MustCompile(`\b[a-z]+\.[A-Z]\w+|\b[a-z]+[A-Z]\w+\b|\binternal/|\bdocs/|\b[AS]\d{1,3}\b|\bM\d[ab]?-\d+\b`)
)

func isBoard(name string) bool {
	return strings.HasPrefix(name, "docs/boards/") && name != "docs/boards/README.md"
}

// boardProblems lists what is wrong with one board.
func boardProblems(name, text string) []string {
	var out []string
	meta, body, ok := front(text)
	if !ok || meta["type"] != "board" {
		return []string{name + ": front matter does not say type: board"}
	}
	screens := boardFence.FindAllStringSubmatch(body, -1)
	if len(screens) == 0 {
		return []string{name + ": no text screen"}
	}
	for i, m := range screens {
		at := fmt.Sprintf("%s: screen %d", name, i+1)
		block := strings.TrimRight(m[1], "\n")
		lines := strings.Split(block, "\n")
		for _, l := range lines {
			if n := utf8.RuneCountInString(l); n > boardWidth {
				out = append(out, fmt.Sprintf("%s: a line of %d columns, over %d", at, n, boardWidth))
			}
			if strings.Contains(l, ";") {
				out = append(out, at+": a semicolon in printed text")
			}
			if j := boardJargon.FindString(l); j != "" {
				out = append(out, at+": "+j+" is a name a person does not say")
			}
		}
		end := boardEnds.FindStringSubmatch(m[2])
		if end == nil {
			out = append(out, at+": no Ends line (the word and the exit code)")
			continue
		}
		if !boardAsk.MatchString(m[3]) {
			out = append(out, at+": no Answered by line")
		}
		if end[1] == "nothing yet" {
			continue
		}
		w := model.Word(end[1])
		if !model.Known(w) {
			out = append(out, at+": ends on "+end[1]+", which is not one of the eight words")
			continue
		}
		if code, _ := exit.Code(w); fmt.Sprint(code) != end[2] {
			out = append(out, fmt.Sprintf("%s: %s is exit %d, the board says %s", at, w, code, end[2]))
		}
		last := ""
		for _, l := range lines {
			if l != "" && !strings.HasPrefix(l, "    ") {
				last = l
			}
		}
		if strings.HasSuffix(last, "]") || strings.HasSuffix(last, ":") {
			continue // the screen stops at a question and the answer ends it
		}
		if !regexp.MustCompile("^  " + regexp.QuoteMeta(string(w)) + "  +\\S").MatchString(last) {
			out = append(out, at+": the closing line is not the word "+string(w))
		}
	}
	return out
}

func boards(w docWorld) []string {
	var out []string
	reg := registryVerbs(w.docs["docs/commands.md"])
	if len(reg) == 0 {
		out = append(out, "docs/commands.md: holds no command to check the boards against")
	}
	var found []string
	for _, name := range w.names() {
		if isBoard(name) {
			found = append(found, name)
			out = append(out, boardProblems(name, w.docs[name])...)
			out = append(out, boardCommands(name, w.docs[name], reg)...)
		}
	}
	if len(found) == 0 {
		return []string{"docs/boards holds no board"}
	}
	if words, ok := w.docs["docs/boards/closing-words.md"]; ok {
		for _, cw := range model.Words() {
			if !strings.Contains(words, "Ends: "+string(cw)+", exit") {
				out = append(out, "docs/boards/closing-words.md: no screen ends on "+string(cw))
			}
		}
	} else {
		out = append(out, "docs/boards/closing-words.md: missing")
	}
	return out
}

func TestTheBoardsHoldToTheLook(t *testing.T) {
	t.Parallel()
	for _, p := range boards(readDocWorld(t)) {
		t.Error(p)
	}
}

// Each check fires on a board with one breach planted.
func TestTheBoardChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	w := readDocWorld(t)
	good := w.docs["docs/boards/busy.md"]
	if good == "" {
		t.Fatal("no busy board to plant a breach in")
	}
	tests := []struct{ name, text, says string }{
		{"a wrong type", strings.Replace(good, "type: board", "type: reference", 1), "does not say type: board"},
		{"a line over 80 columns", strings.Replace(good, "reading the lock", strings.Repeat("x", 80), 1), "over 80"},
		{"a semicolon", strings.Replace(good, "reading the lock", "reading; the lock", 1), "a semicolon"},
		{"a code name", strings.Replace(good, "reading the lock", "reading model.Word", 1), "a name a person does not say"},
		{"a path into the code", strings.Replace(good, "reading the lock", "reading internal/exit", 1), "a name a person does not say"},
		{"a ninth word", strings.Replace(good, "  busy   another", "  stuck   another", 1), "the closing line is not the word busy"},
		{"a wrong exit code", strings.Replace(good, "Ends: busy, exit 3", "Ends: busy, exit 4", 1), "busy is exit 3, the board says 4"},
		{"a word outside the eight", strings.Replace(good, "Ends: busy, exit 3", "Ends: stuck, exit 3", 1), "not one of the eight words"},
		{"no Ends line", strings.Replace(good, "Ends: busy, exit 3.", "", 1), "no Ends line"},
		{"no Answered by line", strings.Replace(good, "Answered by:", "Reply:", 1), "no Answered by line"},
		{"a command that is not one", strings.Replace(good, "reading the lock", "gragas release withdraw 1.1.0", 1), "withdraw is a command of its own"},
		{"a verb a group lacks", strings.Replace(good, "reading the lock", "gragas rule frobnicate", 1), "gragas rule takes one of"},
		{"a word that is no command", strings.Replace(good, "reading the lock", "gragas frobnicate", 1), "frobnicate is not a command"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := boards(w.planted("docs/boards/busy.md", tc.text))
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("said %q, want a problem saying %q", got, tc.says)
			}
		})
	}
	t.Run("a closing word with no screen", func(t *testing.T) {
		t.Parallel()
		cw := w.docs["docs/boards/closing-words.md"]
		got := boards(w.planted("docs/boards/closing-words.md", strings.Replace(cw, "Ends: refused, exit 5", "Ends: done, exit 0", 1)))
		if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "no screen ends on refused") }) {
			t.Errorf("said %q", got)
		}
	})
}

// A command a board prints is one the program has decided on: every
// "gragas ..." in a board, and the command a screen opens with, resolves
// against the registry's documents. A verb the command does not have, such as
// release withdraw, which is withdraw on its own, is the red case.
var (
	commandRow  = regexp.MustCompile(`(?m)^\| ([a-z][a-z-]*)(?: \(([^)]*)\))? \|`)
	boardTyped  = regexp.MustCompile("(?m)(?:^|`)\\s*gragas ([a-z][a-z -]*)")
	boardOpens  = regexp.MustCompile("```text\n([a-z][^\n]*)\n")
	bareWordsRe = regexp.MustCompile(`^[a-z][a-z-]*`)
)

// registryVerbs maps each command of docs/commands.md to its verbs.
func registryVerbs(commands string) map[string][]string {
	out := map[string][]string{}
	for _, m := range commandRow.FindAllStringSubmatch(commands, -1) {
		var verbs []string
		for v := range strings.SplitSeq(m[2], ",") {
			if v = strings.TrimSpace(v); v != "" {
				verbs = append(verbs, v)
			}
		}
		out[m[1]] = verbs
	}
	return out
}

// commandProblem says what is wrong with a printed command, "" when it holds.
func commandProblem(reg map[string][]string, line string) string {
	var words []string
	for w := range strings.FieldsSeq(line) {
		if !bareWordsRe.MatchString(w) || w != bareWordsRe.FindString(w) {
			break
		}
		words = append(words, w)
		if len(words) == 2 {
			break
		}
	}
	if len(words) == 0 {
		return ""
	}
	verbs, ok := reg[words[0]]
	if !ok {
		return words[0] + " is not a command"
	}
	if len(verbs) > 0 {
		if len(words) == 1 || !slices.Contains(verbs, words[1]) {
			return "gragas " + words[0] + " takes one of " + strings.Join(verbs, ", ")
		}
		return ""
	}
	if len(words) == 2 {
		if _, isCommand := reg[words[1]]; isCommand {
			return "gragas " + words[0] + " " + words[1] + " is not a command: " + words[1] + " is a command of its own"
		}
	}
	return ""
}

// boardCommands lists the printed commands of a board the registry does not hold.
func boardCommands(name, text string, reg map[string][]string) []string {
	var out []string
	for _, m := range boardTyped.FindAllStringSubmatch(text, -1) {
		if p := commandProblem(reg, m[1]); p != "" {
			out = append(out, name+": gragas "+m[1]+": "+p)
		}
	}
	for _, m := range boardOpens.FindAllStringSubmatch(text, -1) {
		if p := commandProblem(reg, m[1]); p != "" {
			out = append(out, name+": a screen opens with "+m[1]+": "+p)
		}
	}
	return out
}
