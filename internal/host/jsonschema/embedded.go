package jsonschema

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// FromAssets compiles every .json file below each root of the Assets port,
// each named by its path under its root, such as "v3/rules.schema.json" for
// root "format/schema". The bytes come from the port, never from the file
// system. Two files of one name, in one root or two, are an error, never a
// silent overwrite.
func FromAssets(a port.Assets, roots ...string) (*Schema, error) {
	var files []model.File
	seen := map[string]string{}
	for _, root := range roots {
		paths, err := a.List(root)
		if err != nil {
			return nil, fmt.Errorf("list the schemas under %s: %w", root, err)
		}
		for _, p := range paths {
			if !strings.HasSuffix(p, ".json") {
				continue
			}
			data, err := a.Read(p)
			if err != nil {
				return nil, fmt.Errorf("read schema %s: %w", p, err)
			}
			name := strings.TrimPrefix(p, root+"/")
			if first, twice := seen[name]; twice {
				return nil, fmt.Errorf("the schema %s is in both %s and %s", name, first, p)
			}
			seen[name] = p
			files = append(files, model.File{Path: name, Data: data})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no schema under %s", strings.Join(roots, ", "))
	}
	return New(files)
}

// Embedded is the Schema port over the schemas below the roots of a. When they
// do not compile it is still a Schema: every Check answers the error, so a
// build defect is told when a document is checked and never hidden.
func Embedded(a port.Assets, roots ...string) port.Schema {
	s, err := FromAssets(a, roots...)
	if err != nil {
		return broken{err}
	}
	return s
}

// broken is a Schema whose schemas did not compile.
type broken struct{ err error }

func (b broken) Check(name string, _ []byte) ([]model.Problem, error) {
	return nil, fmt.Errorf("check against %s: %w", name, b.err)
}
