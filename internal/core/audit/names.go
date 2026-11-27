package audit

import (
	"strings"
)

// Matcher is the private list of names, as the layer above has read it. Find
// says whether text holds a name of the list; what it holds is never asked,
// so no finding can print the name or the pattern that matched it.
type Matcher interface {
	Find(text string) bool
}

// Message is one commit's whole message and the commit it belongs to.
type Message struct {
	Commit string
	Text   string
}

// NamesInput is what the names check reads: the tracked files with their
// text, the tracked paths, the commit messages of a range, and any texts
// handed in by path (the lander's card passes a pull request's title and
// body). A nil List is a check that did not run, and CheckNames says so.
type NamesInput struct {
	Rules    RepoRules
	List     Matcher
	Files    []Doc
	Paths    []string
	Messages []Message
	Texts    []Doc
}

// said is the whole of what a names finding says of the name.
const said = "holds a name from the private names list"

// CheckNames is the names check (supply.secrets.4): a name of the list in a
// tracked file, in a tracked file's name, in a commit message of the range or
// in a text handed in is a finding that says where, never what. With no list
// it finds nothing and answers that it did not run; the caller must not take
// that for a pass.
func CheckNames(in NamesInput) ([]Finding, *NotChecked) {
	if in.List == nil {
		n := Unjudged(KeyNames, "no names list was given: set the names-file key or GRAGAS_NAMES_FILE")
		return nil, &n
	}
	var out []Finding
	for _, p := range in.Paths {
		if in.List.Find(p) {
			out = append(out, found(in.Rules, KeyNames, p, 0, "has a file name that "+said))
		}
	}
	for _, f := range in.Files {
		out = append(out, textHits(in.Rules, in.List, f.Path, f.Text, "")...)
	}
	for _, m := range in.Messages {
		out = append(out, textHits(in.Rules, in.List, "commit "+m.Commit, m.Text, "message ")...)
	}
	for _, t := range in.Texts {
		out = append(out, textHits(in.Rules, in.List, t.Path, t.Text, "")...)
	}
	return sorted(out), nil
}

// textHits is the findings of one text: none when it is binary (a NUL) or
// holds no name; else one for the first line that holds one, or, when only the
// whole text does (a name across a line break), one with no line.
func textHits(rules RepoRules, list Matcher, where, text, what string) []Finding {
	if strings.IndexByte(text, 0) >= 0 || !list.Find(text) {
		return nil
	}
	line := 0
	for i, l := range strings.Split(text, "\n") {
		if list.Find(l) {
			line = i + 1
			break
		}
	}
	return []Finding{found(rules, KeyNames, where, line, what+said)}
}
