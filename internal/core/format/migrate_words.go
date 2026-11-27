package format

import (
	"fmt"
	"strings"
)

// refuse records a word of the old layout that has no counterpart; MigrateV2
// returns them all as its error, since a guess would write a wrong rule.
func (g *migrator) refuse(item, field, word string) {
	g.bad = append(g.bad, fmt.Sprintf("%s: %s %q has no counterpart at format 3", item, field, word))
}

// mapped is the counterpart of an old word, or refused.
func (g *migrator) mapped(item, field, word string, table map[string]string) string {
	v, ok := table[word]
	if !ok {
		g.refuse(item, field, word)
	}
	return v
}

// problem is the refusals as one error, "" when there are none.
func (g *migrator) problem() string {
	return strings.Join(g.bad, "; ")
}
