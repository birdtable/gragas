package rulebook

import (
	"fmt"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// Layer is one layer of parts, such as base or lang.
type Layer struct {
	Key  string
	Name string
}

// Part is one row of rulebook/parts.yaml.
type Part struct {
	Short       string
	Split       string
	Layer       string
	Name        string
	Description string
	BuildsOn    []string
	// Bindings are the bindings its checks may use.
	Bindings []string
	// Facets are the facets the part names.
	Facets []string
	// Owner answers for the part's rules and grants its waivers.
	Owner string
	// When says which products take the part; nil takes every product.
	When *ordered.Map
	// Data is the row as written.
	Data *ordered.Map
}

// Dir is the part's folder, relative to the repository root.
func (p Part) Dir() string { return path.Join(Dir, p.Layer, p.Short) }

// Parts is the table of parts and layers, in file order.
type Parts struct {
	Layers []Layer
	List   []Part
	// Waivers is the default waiver of each strength.
	Waivers *ordered.Map
	short   map[string]int
	layer   map[string]int
}

// Part returns the part with the given short name.
func (ps *Parts) Part(short string) (Part, bool) {
	i, ok := ps.short[short]
	if !ok {
		return Part{}, false
	}
	return ps.List[i], true
}

// LayerName returns the display name of a layer key.
func (ps *Parts) LayerName(key string) string {
	if i, ok := ps.layer[key]; ok {
		return ps.Layers[i].Name
	}
	return key
}

// partsFromDoc builds the table from a schema-valid parts.yaml and reports
// rows that point at nothing, and a part named model.EnginePart, the word the
// engine's own check ids start with.
func partsFromDoc(doc *ordered.Map) (*Parts, []finding) {
	ps := &Parts{short: map[string]int{}, layer: map[string]int{}, Waivers: doc.Sub("waivers")}
	var bad []finding
	for _, m := range ordered.Maps(doc.List("layers")) {
		key := m.Str("key")
		if _, dup := ps.layer[key]; dup {
			bad = append(bad, finding{line: m.Line, msg: fmt.Sprintf("layer %s listed twice", key)})
			continue
		}
		ps.layer[key] = len(ps.Layers)
		ps.Layers = append(ps.Layers, Layer{Key: key, Name: m.Str("name")})
	}
	rows := ordered.Maps(doc.List("parts"))
	for _, m := range rows {
		p := Part{
			Short:       m.Str("short"),
			Split:       m.Str("split"),
			Layer:       m.Str("layer"),
			Name:        m.Str("name"),
			Description: m.Str("description"),
			BuildsOn:    ordered.Strings(m.List("builds_on")),
			Bindings:    ordered.Strings(m.List("bindings")),
			Facets:      ordered.Strings(m.List("facets")),
			Owner:       m.Str("owner"),
			When:        m.Sub("when"),
			Data:        m,
		}
		if _, dup := ps.short[p.Short]; dup {
			bad = append(bad, finding{line: m.Line, msg: fmt.Sprintf("part %s listed twice", p.Short)})
			continue
		}
		if p.Short == model.EnginePart {
			bad = append(bad, finding{line: m.Line, msg: fmt.Sprintf("part %s is reserved: it is the first word of the engine's own check ids, such as %s, so a rule of that part could not be told from the engine's check", p.Short, model.ValidateCheck)})
		}
		if _, ok := ps.layer[p.Layer]; !ok {
			bad = append(bad, finding{line: m.Line, msg: fmt.Sprintf("part %s: layer %s is not in layers", p.Short, p.Layer)})
		}
		ps.short[p.Short] = len(ps.List)
		ps.List = append(ps.List, p)
	}
	for i, p := range ps.List {
		for _, b := range p.BuildsOn {
			if _, ok := ps.short[b]; !ok || b == p.Short {
				bad = append(bad, finding{line: rows[i].Line, msg: fmt.Sprintf("part %s: builds_on %s is not another part", p.Short, b)})
			}
		}
	}
	return ps, append(bad, ps.checkFacets()...)
}

// FacetOwner returns the part that names a facet.
func (ps *Parts) FacetOwner(facet string) (string, bool) {
	for _, p := range ps.List {
		if slices.Contains(p.Facets, facet) {
			return p.Short, true
		}
	}
	return "", false
}

// checkFacets: a facet is named by one part only.
func (ps *Parts) checkFacets() []finding {
	var bad []finding
	seen := map[string]string{}
	for _, p := range ps.List {
		for _, f := range p.Facets {
			if other, ok := seen[f]; ok {
				bad = append(bad, finding{msg: fmt.Sprintf("facet %s is named by %s and %s", f, other, p.Short)})
			}
			seen[f] = p.Short
		}
	}
	return bad
}
