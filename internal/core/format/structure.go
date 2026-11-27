package format

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// structureOf reads a document the structure schema accepted, in the order it
// was written: the order of each list is the order of the things it lists.
func structureOf(m *ordered.Map) model.Structure {
	s := model.Structure{Format: model.FormatVersion}
	for _, l := range ordered.Maps(m.List("layers")) {
		layer := model.Layer{Slug: l.Str("slug"), Name: l.Str("name"), Aliases: ordered.Strings(l.List("aliases"))}
		for _, p := range ordered.Maps(l.List("parts")) {
			part := model.Part{Slug: p.Str("slug"), Name: p.Str("name"), Aliases: ordered.Strings(p.List("aliases")), Owners: ordered.Strings(p.List("owners"))}
			for _, c := range ordered.Maps(p.List("concerns")) {
				part.Concerns = append(part.Concerns, model.Concern{Slug: c.Str("slug"), Name: c.Str("name"), Aliases: ordered.Strings(c.List("aliases"))})
			}
			layer.Parts = append(layer.Parts, part)
		}
		s.Layers = append(s.Layers, layer)
	}
	for _, k := range ordered.Maps(m.List("kinds")) {
		s.Kinds = append(s.Kinds, model.ProgramKind{Slug: k.Str("slug"), Name: k.Str("name"), Description: k.Str("description")})
	}
	return s
}

// reservedPart is the finding on a part slug or alias spelled
// model.EnginePart.
const reservedPart = `part "` + model.EnginePart + `" is reserved: it is the first word of the engine's own check ids, such as ` + model.ValidateCheck + `, so a rule of that part could not be told from the engine's check`

// reservedID is the finding, formatted with the id, on a rule id or alias
// whose first word is model.EnginePart. A rule's id is not held to its part,
// and an alias may carry a renamed part's old slug, so the part refusal alone
// would not keep an engine check's id out of a rulebook.
const reservedID = `the id %s starts with "` + model.EnginePart + `", the word the engine's own check ids start with, such as ` + model.ValidateCheck + `; a rule's id or alias never does`

// reservedIDs are the id and the aliases of rule whose first word is
// model.EnginePart, in the order the rule writes them.
func reservedIDs(rule model.Rule) []model.RuleID {
	var out []model.RuleID
	for _, id := range append([]model.RuleID{rule.ID}, rule.Aliases...) {
		if first, _, _ := strings.Cut(string(id), "."); first == model.EnginePart {
			out = append(out, id)
		}
	}
	return out
}

// slugFindings holds the structure to what a schema cannot say: a part slug is
// the first word of a rule id and a concern slug the second, so each is
// written once, an alias never takes a slug that is in use, and no part slug
// or alias is the word the engine's own checks start with.
func slugFindings(file string, m *ordered.Map) []Finding {
	var bad []Finding
	seen := map[string]int{}
	use := func(kind, slug string, line int) {
		key := kind + " " + slug
		if first, dup := seen[key]; dup {
			bad = append(bad, Finding{File: file, Line: line, Msg: fmt.Sprintf("%s %q is already used on line %d; a slug names one thing", kind, slug, first)})
			return
		}
		seen[key] = line
	}
	part := func(slug string, line int) {
		if slug == model.EnginePart {
			bad = append(bad, Finding{File: file, Line: line, Msg: reservedPart})
		}
		use("part", slug, line)
	}
	for _, l := range ordered.Maps(m.List("layers")) {
		use("layer", l.Str("slug"), lineOfKey(l, "slug"))
		for _, p := range ordered.Maps(l.List("parts")) {
			part(p.Str("slug"), lineOfKey(p, "slug"))
			for _, a := range ordered.Strings(p.List("aliases")) {
				part(a, lineOfKey(p, "aliases"))
			}
			seenConcern := map[string]int{}
			for _, c := range ordered.Maps(p.List("concerns")) {
				slug := c.Str("slug")
				if first, dup := seenConcern[slug]; dup {
					bad = append(bad, Finding{File: file, Line: lineOfKey(c, "slug"), Msg: fmt.Sprintf("concern %q of part %q is already used on line %d", slug, p.Str("slug"), first)})
					continue
				}
				seenConcern[slug] = lineOfKey(c, "slug")
			}
		}
	}
	for _, k := range ordered.Maps(m.List("kinds")) {
		use("kind", k.Str("slug"), lineOfKey(k, "slug"))
	}
	return bad
}
