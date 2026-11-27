package main

import (
	"fmt"
	"os"

	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// sharedOf is the shared registry in root, held to its schema from the
// repository's gate/schema and to the table (arch.shared.8). Every read goes
// through root, so neither file is read through a link out of it.
func sharedOf(root *os.Root, t *layers.Table) (*layers.Shared, error) {
	data, err := root.ReadFile(layers.SharedFile)
	if err != nil {
		return nil, err
	}
	set, err := jsonschema.Compile(root.FS(), gate.SchemaDir, layers.SharedSchema+".schema.json")
	if err != nil {
		return nil, fmt.Errorf("the schema of %s: %w", layers.SharedFile, err)
	}
	s, err := layers.ParseShared(data, func(doc any) []string {
		var out []string
		for _, p := range set.Validate(layers.SharedSchema+".schema.json", doc) {
			out = append(out, fmt.Sprintf("line %d: %s", p.Line, p.Msg))
		}
		return out
	})
	if err != nil {
		return nil, err
	}
	return s, t.Hold(s)
}
