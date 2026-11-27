package newcommand

import (
	"fmt"
	"strconv"
	"strings"
)

// Seed is what the row of a new command starts from: the planned file of the
// command when it has one, so building it keeps what was decided, and a stub
// to fill in when it has none. Its words are the registry's: a context letter,
// an ending, closing words.
type Seed struct {
	// Order is the row's place in the table of spec 15.1, 0 for the next free one.
	Order int
	// Context is R, P, B or A; Ends is plan, report or listing.
	Context, Ends string
	// OptIn is whether the plain form ends otherwise than the row says.
	OptIn bool
	// Summary is the sentence of help.
	Summary string
	// Examples are the two lines to type and what each does.
	Examples [2][2]string
	// Writes are the files a run writes, Exits the closing words it ends on.
	Writes, Exits []string
	// JSON is the kind of the --json document.
	JSON string
	// Then are the verbs of a group, or the modes of a flat command.
	Then []string
}

// stub is the seed of a command with no planned file.
func stub(n Name) Seed {
	line := "gragas " + n.String()
	return Seed{Context: "A", Ends: "listing", Summary: "Fill in: what " + n.Family() + " does",
		Examples: [2][2]string{{line, "fill in"}, {line + " --json", "as one JSON document"}},
		Exits:    []string{"done"}, JSON: n.Family()}
}

// The Go names of the registry's words.
func contextNames() map[string]string {
	return map[string]string{"R": "kit.InRulebook", "P": "kit.InProject", "B": "kit.InBoth", "A": "kit.Anywhere"}
}

func endingNames() map[string]string {
	return map[string]string{"plan": "kit.EndsPlan", "report": "kit.EndsReport", "listing": "kit.EndsListing"}
}

func closingNames() map[string]string {
	return map[string]string{"done": "model.Done", "failed": "model.Failed", "findings": "model.Findings",
		"usage": "model.Usage", "busy": "model.Busy", "not ready": "model.NotReady", "refused": "model.Refused",
		"declined": "model.Declined"}
}

// quoted is the words as Go strings, joined by commas.
func quoted(in []string) string {
	q := make([]string, len(in))
	for i, s := range in {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}

// named is each word as its constant in names, joined by commas.
func named(names map[string]string, kind string, in []string) (string, error) {
	out := make([]string, len(in))
	for i, w := range in {
		c, ok := names[w]
		if !ok {
			return "", fmt.Errorf("%q is not a %s", w, kind)
		}
		out[i] = c
	}
	return strings.Join(out, ", "), nil
}
