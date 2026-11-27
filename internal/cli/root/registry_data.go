package root

import (
	"strings"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/cli/planned"
	"github.com/birdtable/gragas/internal/model"
)

// commands is the registry of gragas: the commands of spec 15.1, each
// family once, in the order of that table, and gen, the old command the spec
// does not name, which leaves with the old check at M6.
//
// the rules of NewRegistry, so a row that breaks one fails the tests.
//
// Summaries and examples are printed text, so they follow the docs rules:
// British English, no semicolon, no code identifier.
func commands() kit.Registry {
	var table, packages []kit.Command
	for _, d := range rows() {
		table = append(table, d.Command)
	}
	for _, b := range built() {
		packages = append(packages, b.Row)
	}
	// A file that does not load is held by TestThePlannedFilesLoad; the rows
	// that did are used, so the program still runs and the test says what is wrong.
	if not, err := planned.Load(planned.Files); err == nil {
		table = append(table, not...)
	}
	return kit.Assemble(table, packages)
}

// draft is a row while it is written: the helpers below build it up.
type draft struct{ kit.Command }

// words are closing words written the way the spec's table writes them.
func words(s string) []model.Word {
	var out []model.Word
	for w := range strings.SplitSeq(s, ";") {
		out = append(out, model.Word(strings.TrimSpace(w)))
	}
	return out
}

func ex(line, does string) kit.Example { return kit.Example{Line: line, Does: does} }

// row builds a not-built row, the default: a command enters the tree in the
// milestone that builds it and its state changes in the same change.
func row(name string, ctx kit.Context, ends kit.Ends, summary string) draft {
	return draft{kit.Command{Name: name, Spec: true, Context: ctx, Ends: ends, Summary: summary, State: kit.NotBuilt}}
}

// with adds the two examples of a row.
func (c draft) with(first, second kit.Example) draft {
	c.Examples = [2]kit.Example{first, second}
	return c
}

// says adds the closing words of a row, the kind of its --json document and the files it writes.
func (c draft) says(exits, json string, writes ...string) draft {
	c.Exits, c.JSONKind, c.Writes = words(exits), json, writes
	return c
}

// family is a row with the words that follow its name.
func family(c draft, then ...string) draft {
	c.Then = then
	return c
}

func state(c draft, s kit.State, menu bool) draft {
	c.State, c.Menu = s, kit.Menu{Offered: menu}
	return c
}

func rows() []draft {
	return []draft{
		state(row("init", kit.Anywhere, kit.EndsPlan, "Set up a project or a rulebook").
			with(ex("gragas init --kind rulebook", "show the plan for a rulebook"), ex("gragas init --kind rulebook --apply", "write it")).
			says("done; declined; refused; not ready", "plan", "the pin and the presets of a project", "rulebook.yaml, structure.yaml and the first line of history.jsonl for a rulebook"), kit.Built, false).at(390),
		state(family(row("rule", kit.InRulebook, kit.EndsPlan, "Add or change a rule, with its history").
			with(ex("gragas rule new", "add a draft rule"), ex("gragas rule retire <id>", "retire a rule and keep its id")).
			says("done; declined; refused", "plan", "the concern's rules file", "history.jsonl"),
			"new", "edit", "replace", "retire", "restore", "renumber", "promote", "confirm", "exclude", "include"),
			kit.Built, false).at(30),
		state(family(row("term", kit.InRulebook, kit.EndsPlan, "Define, change or retire a term").
			with(ex("gragas term new", "define a term"), ex("gragas term retire <slug>", "retire a term nothing uses")).
			says("done; declined; refused", "plan", "terms.yaml", "history.jsonl"),
			"new", "edit", "retire"), kit.Built, false).at(40),
		bare("help", "Show the help of a command", "", true).at(510),
		legacyGen().at(550),
	}
}

// bare is a command built by the skeleton.
func bare(name, summary, does string, menu bool) draft {
	c := row(name, kit.Anywhere, kit.EndsListing, summary).
		with(ex("gragas "+name, does), ex("gragas "+name+" --json", "as one JSON document")).
		says("done", name)
	if name == "help" {
		c.Examples = [2]kit.Example{ex("gragas help", "list the commands"), ex("gragas help <command>", "show one command's help")}
	}
	return state(c, kit.Built, menu)
}

// legacyGen is gen, the old command that is not in the spec: it leaves at M6
// with the old check, when the commands of the contexts replace them.
func legacyGen() draft {
	c := row("gen", kit.Anywhere, kit.EndsPlan, "Say which generated files are out of date, and write them").
		with(ex("gragas gen", "list the files out of date"), ex("gragas gen --apply", "write them")).
		says("done; findings; declined; refused", "plan", "rules.json and ids.lock")
	c.Spec = false
	return state(c, kit.Legacy, true)
}

// at gives a row its place in the table of spec 15.1. The rows still in this
// table and those the planned files give are in one order, by these numbers.
func (c draft) at(order int) draft {
	c.Order = order
	return c
}
