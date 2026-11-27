package root

import (
	"cmp"
	"github.com/birdtable/gragas/internal/cli/kit"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// helpCommands is the block of the root's help that lists the commands of
// the tree: every row built or legacy, with its two examples. It is made from
// the registry, so help says only what the code does (cli.commands.9).
func helpCommands(r kit.Registry) string {
	var lines [][2]string
	width := 0
	for _, c := range r.InTree() {
		if c.Name == "help" {
			continue
		}
		for _, e := range c.Examples {
			if c.NotYetExample(e) {
				continue
			}
			lines = append(lines, [2]string{e.Line, e.Does})
			width = max(width, len([]rune(e.Line)))
		}
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("  " + kit.Column(l[0], width+2) + l[1] + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// menuHint is what answers the menu without a terminal: the command it would
// have run, typed (cli.environment.14).
const menuHint = "gragas <command>"

// menuTitle is the question the menu asks.
const menuTitle = "What would you like to do?"

// group is a run of commands the menu leads with, in order.
type group struct {
	name  string
	verbs []string
}

// lead is what the menu of each place leads with (S19, spec 15.1). Both lists
// the rulebook group first and the project group second, and upgrade and
// propose in each: the two groups ask for their subject.
func lead(p Place) []group {
	rulebook := group{"rulebook", []string{"validate", "rule", "bind", "prove", "release", "attention"}}
	project := group{"project", []string{"status", "explain", "check", "waive", "upgrade", "switch", "report", "agents"}}
	switch p {
	case PlaceRulebook:
		return []group{rulebook}
	case PlaceProject:
		return []group{project}
	case PlaceBoth:
		rulebook.verbs = append(rulebook.verbs, "upgrade", "propose")
		project.verbs = append(project.verbs, "propose")
		return []group{rulebook, project}
	case PlaceNeither:
	}
	return []group{{"start", []string{"init", "handbook"}}}
}

// fits is whether a command that runs in c may be offered in p.
func fits(c kit.Context, p Place) bool {
	switch c {
	case kit.InRulebook:
		return p == PlaceRulebook || p == PlaceBoth
	case kit.InProject:
		return p == PlaceProject || p == PlaceBoth
	case kit.InBoth:
		return p != PlaceNeither
	case kit.Anywhere:
		return true
	}
	return true
}

// ledBy are the commands whose own row says the group leads with them (its Menu
// names it), by rank and then name, after those the group's list names. They
// are in the tree, offered, and not led yet.
func ledBy(r kit.Registry, group string, led map[string]bool) []kit.Command {
	var out []kit.Command
	for _, c := range r.InTree() {
		if c.Menu.Offered && c.Menu.Lead == group && !led[c.Name] {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b kit.Command) int {
		return cmp.Or(cmp.Compare(a.Menu.Rank, b.Menu.Rank), strings.Compare(a.Name, b.Name))
	})
	return out
}

// groupChoices are what the group offers: the commands its list names, then
// those whose own row says it leads with them. Each is in the tree and offered.
// A command that two groups lead with says whose it is when several groups lead.
func groupChoices(r kit.Registry, g group, several bool, led map[string]bool) []model.Choice {
	var out []model.Choice
	for _, v := range g.verbs {
		c, ok := r.Find(v)
		if !ok || c.State == kit.NotBuilt || !c.Menu.Offered {
			continue
		}
		led[v] = true
		blurb := c.Summary
		if several && (v == "upgrade" || v == "propose") {
			blurb += " (" + g.name + ")"
		}
		out = append(out, model.Choice{Label: c.Name, Blurb: blurb, Value: c.Name})
	}
	for _, c := range ledBy(r, g.name, led) {
		led[c.Name] = true
		out = append(out, model.Choice{Label: c.Name, Blurb: c.Summary, Value: c.Name})
	}
	return out
}

// menuChoices are what the bare menu offers in place p: the commands of the
// groups that lead it, then the other commands that run here, each only if it
// is in the tree and marked for the menu (cli.commands.9). A blank line ends
// each group. A command that two groups lead with is offered in both, its
// blurb saying whose.
func menuChoices(r kit.Registry, p Place) []model.Choice {
	var out []model.Choice
	led := map[string]bool{}
	groups := lead(p)
	for _, g := range groups {
		start := len(out)
		out = append(out, groupChoices(r, g, len(groups) > 1, led)...)
		if len(out) > start {
			out[len(out)-1].Gap = true
		}
	}
	for _, c := range r.InTree() {
		if c.Menu.Offered && !led[c.Name] && fits(c.Context, p) {
			out = append(out, model.Choice{Label: c.Name, Blurb: c.Summary, Value: c.Name})
		}
	}
	if n := len(out); n > 0 {
		out[n-1].Gap = false
	}
	return out
}
