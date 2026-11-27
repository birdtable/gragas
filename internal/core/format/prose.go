package format

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// A statement names a term by writing its slug in braces, {slug}. The term
// must be defined in terms.yaml; the literal of the term is what a reader
// sees, so the statement itself stays neutral. A term's literal written bare
// is allowed through the names check too, since it is the rulebook's own.
var termRef = regexp.MustCompile(`\{([^{}]*)\}`)

var (
	passive   = regexp.MustCompile(`(?i)\b(is|are|was|were|be|been)\s+(\w+ed|written|made|done|given|taken|kept|run|used|built|known|shown|held|sent|seen|found)\b`)
	weWord    = regexp.MustCompile(`(?i)\b(we|our|ours)\b`)
	sentences = regexp.MustCompile(`[.!?](\s+|$)`)
)

// notImperative are words a statement in the imperative does not start with.
func notImperative(w string) bool {
	return map[string]bool{
		"the": true, "a": true, "an": true, "this": true, "that": true, "these": true, "those": true, "it": true,
		"they": true, "we": true, "there": true, "each": true, "every": true, "all": true, "no": true, "any": true,
		"some": true, "one": true, "your": true, "our": true, "its": true, "their": true,
	}[w]
}

// issue is one thing wrong with a piece of prose and the fix to offer.
type issue struct{ msg, fix string }

// lintStatement holds a statement to the prose lint: one sentence, in the
// imperative and the active voice, within the word budget (0 for none), and
// no "we". The fix of each issue is the reword to offer.
func lintStatement(text string, budget int) []issue {
	flat := strings.Join(strings.Fields(termRef.ReplaceAllString(text, "term")), " ")
	var out []issue
	if n := countSentences(flat); n > 1 {
		out = append(out, issue{fmt.Sprintf("the statement is %d sentences; a statement is one", n), "reword it to one sentence, and move the rest to the reasoning"})
	}
	if w := firstWord(flat); w != "" && (notImperative(strings.ToLower(w)) || notVerb(w)) {
		out = append(out, issue{fmt.Sprintf("the statement starts with %q, which is not the imperative", w), "reword it to start with a verb, as in \"Wrap the error\""})
	}
	if m := passive.FindString(flat); m != "" {
		out = append(out, issue{fmt.Sprintf("the statement is passive (%q)", m), "reword it with the actor first, in the imperative"})
	}
	if weWord.MatchString(flat) {
		out = append(out, issue{"the statement says we or our", "reword it to address the reader in the imperative"})
	}
	if words := len(strings.Fields(flat)); budget > 0 && words > budget {
		out = append(out, issue{fmt.Sprintf("the statement is %d words and the budget is %d", words, budget), "shorten it, or move detail to the reasoning"})
	}
	return out
}

func countSentences(s string) int {
	n := len(sentences.FindAllStringIndex(s, -1))
	if s != "" && !strings.ContainsAny(s[len(s)-1:], ".!?") {
		n++
	}
	return n
}

func firstWord(s string) string {
	s = strings.TrimLeft(s, "`\"'*_ ")
	end := strings.IndexFunc(s, func(r rune) bool { return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') })
	if end < 0 {
		return s
	}
	return s[:end]
}

// notVerb is the shape of a first word that is a gerund or a third-person
// verb: a statement that starts with one is a description, not an order.
func notVerb(w string) bool {
	w = strings.ToLower(w)
	if len(w) > 4 && strings.HasSuffix(w, "ing") {
		return true
	}
	return len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is")
}

// undefinedTerms are the slugs written in braces in text that are not in defined.
func undefinedTerms(text string, defined map[string]bool) []string {
	var out []string
	for _, m := range termRef.FindAllStringSubmatch(text, -1) {
		if !defined[m[1]] {
			out = append(out, m[1])
		}
	}
	return out
}

// withoutTerms is text with every term reference and every literal of the
// rulebook's own terms taken out, so the names check sees only what is bare.
func withoutTerms(text string, literals []string) string {
	return blankerOf(literals).blank(text)
}

// proseItem is one piece of a rule's text, kept with its place to be held to
// the prose lint and the names check once every file is read, and with the
// rule's id and origin, which the hold of a migrated statement reads.
type proseItem struct {
	file      string
	line      int
	text      string
	statement bool
	rule      model.RuleID
	origin    model.Origin
}

// proseItems lists the text of the rule at rm: its statement, which the lint
// decides on (arch.ports.19), and the reasoning, which only the names check
// reads. off is the line above the first line of rm's text in file.
func proseItems(rule model.Rule, rm *ordered.Map, file string, off int) []proseItem {
	out := []proseItem{{file: file, line: off + rm.KeyLine("statement"), text: rule.Statement, statement: true, rule: rule.ID, origin: rule.Origin}}
	if x := rule.Reasoning; x != nil {
		rs := rm.Sub("reasoning")
		out = append(out, proseItem{file: file, line: off + rs.KeyLine("definition"), text: x.Definition})
		out = append(out, proseItem{file: file, line: off + rs.KeyLine("pros"), text: strings.Join(x.Pros, "\n")})
		out = append(out, proseItem{file: file, line: off + rs.KeyLine("cons"), text: strings.Join(x.Cons, "\n")})
	}
	return out
}

// lint holds the statements to the prose lint and every piece of rule text to
// the terms: a term in braces must be defined. The names check reads every
// line of every file the load read, not the rule text alone (NamesIn).
func (r reader) lint() {
	defined := map[string]bool{}
	for _, t := range r.out.Book.Terms {
		if t.Status != model.Retired { // a retired term defines nothing
			defined[t.Slug] = true
		}
	}
	budget := 0
	if b := r.out.Book.Identity.Budgets; b != nil {
		budget = b.StatementWords
	}
	recorded := r.recorded()
	for _, p := range *r.prose {
		switch {
		case p.statement && held(p, recorded):
			r.out.Held++
		case p.statement:
			for _, i := range lintStatement(p.text, budget) {
				r.out.Findings = append(r.out.Findings, Finding{File: p.file, Line: p.line, Msg: i.msg, Fix: i.fix, Prose: true})
			}
		}
		for _, slug := range undefinedTerms(p.text, defined) {
			r.out.Findings = append(r.out.Findings, Finding{File: p.file, Line: p.line,
				Msg: fmt.Sprintf("the term {%s} is not defined in terms.yaml", slug), Fix: "define the term, or write the words out"})
		}
	}
}

// literalAt matches the literal as whole words: a short literal never blanks
// the middle of a longer word that the names check should see.
func literalAt(l string) *regexp.Regexp {
	q := regexp.QuoteMeta(l)
	if r := []rune(l); len(r) > 0 {
		if isWordRune(r[0]) {
			q = `\b` + q
		}
		if isWordRune(r[len(r)-1]) {
			q += `\b`
		}
	}
	return regexp.MustCompile(q)
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
