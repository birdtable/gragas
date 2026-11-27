package ordered

import (
	"go.yaml.in/yaml/v3"
)

// ParseStrict reads one YAML document from a source the engine does not
// control, such as an adapter document of a rulebook or a project. It is
// ParseYAML with every form that makes one place stand for another refused
// before anything is expanded: an anchor (&name), an alias (*name), a merge
// key (<<) and an explicit tag (!name or !!type). A key written twice in one
// mapping is refused too, with both lines, where ParseYAML keeps the last
// value. Each refusal is a SyntaxError that names the line. An empty document
// is nil.
//
// ParseYAML expands an alias where it stands, so a short document of aliases
// of aliases grows by a factor at every level; here none is ever followed.
func ParseStrict(data []byte) (any, error) {
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, err
	}
	if err := plain(n, 0); err != nil {
		return nil, err
	}
	return (&converter{}).convert(n, 0)
}

// plain refuses, below n, each node that names or reuses another and each
// key written twice in one mapping.
func plain(n *yaml.Node, depth int) error {
	if depth > maxDepth {
		return syntaxAt(n.Line, "nesting deeper than %d", maxDepth)
	}
	switch {
	case n.Kind == yaml.AliasNode:
		return syntaxAt(n.Line, "the alias *%s stands for a value written elsewhere; write the value out here", n.Value)
	case n.Anchor != "":
		return syntaxAt(n.Line, "the anchor &%s lets another place stand for this value; remove it", n.Anchor)
	case n.Tag == "!!merge":
		return syntaxAt(n.Line, "merge keys (<<) are not supported; write the keys out")
	case n.Style&yaml.TaggedStyle != 0:
		return syntaxAt(n.Line, "the tag %s is not read here; write the value without it", n.Tag)
	}
	seen := map[string]int{}
	for i, c := range n.Content {
		if err := plain(c, depth+1); err != nil {
			return err
		}
		if n.Kind != yaml.MappingNode || i%2 != 0 || c.Kind != yaml.ScalarNode {
			continue
		}
		if first, again := seen[c.Value]; again {
			return syntaxAt(c.Line, "the key %s is written again, first at line %d", c.Value, first)
		}
		seen[c.Value] = c.Line
	}
	return nil
}
