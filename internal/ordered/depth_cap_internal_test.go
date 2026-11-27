package ordered

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// nested is a scalar under n sequences, so the scalar lies at depth n.
func nested(n int) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "x", Line: 9}
	for range n {
		node = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{node}, Line: 9}
	}
	return node
}

// TestNestingIsHeldAtMaxDepthByEachWalkAndByTheParser (boundary: maxDepth): a
// scalar maxDepth deep is read by the plain walk, by the converter and by
// ParseYAML, and one level deeper is refused by each, with the cap's own
// words, so the refusal is the cap's and not the text's size.
func TestNestingIsHeldAtMaxDepthByEachWalkAndByTheParser(t *testing.T) {
	t.Parallel()
	if err := plain(nested(maxDepth), 0); err != nil {
		t.Errorf("plain at maxDepth = %v", err)
	}
	if err := plain(nested(maxDepth+1), 0); err == nil || !strings.Contains(err.Error(), "nesting deeper than") {
		t.Errorf("plain at maxDepth+1 = %v, want the depth refused", err)
	}
	if _, err := (&converter{}).convert(nested(maxDepth), 0); err != nil {
		t.Errorf("convert at maxDepth = %v", err)
	}
	if _, err := (&converter{}).convert(nested(maxDepth+1), 0); err == nil || !strings.Contains(err.Error(), "deeper than") {
		t.Errorf("convert at maxDepth+1 = %v, want the depth refused", err)
	}
	text := func(n int) []byte { return []byte(strings.Repeat("[", n) + "x" + strings.Repeat("]", n)) }
	if _, err := ParseYAML(text(maxDepth)); err != nil {
		t.Errorf("ParseYAML at maxDepth = %v", err)
	}
	if _, err := ParseYAML(text(maxDepth + 1)); err == nil || !strings.Contains(err.Error(), "deeper than") {
		t.Errorf("ParseYAML at maxDepth+1 = %v, want the depth refused", err)
	}
}
