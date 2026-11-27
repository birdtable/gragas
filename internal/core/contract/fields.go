package contract

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// Fields is the field reference of one entity or document of the contract.
type Fields struct {
	// Schema is the name the reference gives the schema: entity-rule, document-bundle.
	Schema string
	// Contract is the contract version the reference describes.
	Contract Version
	// Rows are the fields, as FieldReference lists them.
	Rows []FieldRow
}

// UnknownEntity is a name that is neither an entity nor a document of the
// contract. It ends usage, and names every one that is.
type UnknownEntity struct {
	Name  string
	Known []string
}

func (e *UnknownEntity) Error() string {
	return fmt.Sprintf("%q is not an entity or a document of the contract", clip(e.Name))
}

// Word is the closing word an unknown name ends on.
func (e *UnknownEntity) Word() model.Word { return model.Usage }

// Entities are the names FieldsOf takes, sorted: each entity and document
// schema of the current major, without its prefix.
func Entities(files port.Assets) ([]string, error) {
	names, err := files.List(path.Join(SchemaDir, majorDir(Current())))
	if err != nil {
		return nil, fmt.Errorf("list the contract schemas: %w", err)
	}
	var out []string
	for _, n := range names {
		b := path.Base(n)
		if b == CommonFile || !strings.HasSuffix(b, ".schema.json") {
			continue
		}
		b = strings.TrimSuffix(b, ".schema.json")
		b = strings.TrimPrefix(strings.TrimPrefix(b, "entity-"), "document-")
		out = append(out, b)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func majorDir(v Version) string { return fmt.Sprintf("v%d", v.Major) }

// FieldsOf reads the field reference of entity (rule, term, bundle) from the
// schemas of the current major in files. An entity wins over a document of
// the same name. A name that is neither is *UnknownEntity.
func FieldsOf(entity string, files port.Assets) (Fields, error) {
	dir := path.Join(SchemaDir, majorDir(Current()))
	prefixes := []string{"entity-", "document-"}
	if entity == "" || strings.ContainsAny(entity, "/\\.") {
		prefixes = nil // a name that is no slug reaches no file
	}
	for _, prefix := range prefixes {
		name := prefix + entity
		data, err := files.Read(path.Join(dir, name+".schema.json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Fields{}, fmt.Errorf("read the %s schema: %w", name, err)
		}
		v, err := ordered.ParseYAML(data)
		if err != nil {
			return Fields{}, fmt.Errorf("the %s schema is not JSON: %w", name, err)
		}
		m, ok := v.(*ordered.Map)
		if !ok {
			return Fields{}, fmt.Errorf("the %s schema is not a JSON object", name)
		}
		return Fields{Schema: name, Contract: Current(), Rows: Rows(m)}, nil
	}
	known, err := Entities(files)
	if err != nil {
		return Fields{}, err
	}
	return Fields{}, &UnknownEntity{Name: entity, Known: known}
}
