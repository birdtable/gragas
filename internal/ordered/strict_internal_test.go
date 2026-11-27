package ordered

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestPlainRefusesAnAliasItMeets: every alias follows its anchor, so the
// anchor is refused first in any parsed text; this holds the alias's own
// refusal, which stands in case the walk or the library ever changes order.
func TestPlainRefusesAnAliasItMeets(t *testing.T) {
	t.Parallel()
	target := &yaml.Node{Kind: yaml.ScalarNode, Value: "1", Line: 1}
	alias := &yaml.Node{Kind: yaml.AliasNode, Value: "x", Alias: target, Line: 2}
	root := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{target, alias}, Line: 1}
	err := plain(root, 0)
	if err == nil || !strings.Contains(err.Error(), "line 2: the alias *x stands for a value written elsewhere") {
		t.Errorf("plain = %v, want the alias at line 2 refused", err)
	}
	if err := plain(&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{target}}, 0); err != nil {
		t.Errorf("plain of the scalar alone = %v, want nil", err)
	}
}

// TestPlainRefusesNestingDeeperThanTheCap: the walk stops at the depth convert
// stops at, so a deep document is refused and never overflows the stack.
func TestPlainRefusesNestingDeeperThanTheCap(t *testing.T) {
	t.Parallel()
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: "x", Line: 9}
	for range maxDepth + 1 {
		n = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{n}, Line: 9}
	}
	if err := plain(n, 0); err == nil || !strings.Contains(err.Error(), "nesting deeper than") {
		t.Errorf("plain = %v, want the depth refused", err)
	}
}
