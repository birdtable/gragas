package generate

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/dev/generate/pluginwire"
	"github.com/birdtable/gragas/internal/ordered"
)

// Where the inputs and the outputs sit, from the repository root.
const (
	// ConsumersDir holds the consumers' literal field lists.
	ConsumersDir = "contract/consumers/"
	// FieldReferenceFile is the field reference skeleton.
	FieldReferenceFile = "contract/field-reference.md"
	// FixtureDir holds the fixture's hand-authored documents.
	FixtureDir = "contract/fixture/"
	// FixtureMirrorDir is where `gragas contract fixture` reads them from: a
	// copy of FixtureDir inside the one go:embed site, which cannot reach the
	// folder above it. The copy is generated and never edited.
	FixtureMirrorDir = "internal/host/assets/contract/fixture/"
)

// Marker is what every generated file says in its first line. It is not the
// go tool's generated-file header: the lint reads no file as generated
// (cli.process.1), and the gate holds each file to the schemas by diff.
const Marker = "Written by gragas-generate"

// Input is what is generated from: the schema files by name, the
// consumers' field lists by file name, and the fixture's documents by path
// below FixtureDir.
type Input struct {
	Schemas   map[string][]byte
	Consumers map[string][]byte
	Fixture   map[string][]byte
	// PluginSchemas are the two schemas of plugin contract 1 by file name
	// (pluginwire.SchemaDir), from which pluginwire makes internal/model/plugin.
	// With none, none of its files is made.
	PluginSchemas map[string][]byte
}

// Generate makes every generated file from in, by repository-relative path.
// It fails when a schema is outside what the generators read, or a consumer
// reads a field the schemas lack.
func Generate(in Input) (map[string][]byte, error) {
	files, t, err := load(in)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	gofiles, err := goFiles(t)
	if err != nil {
		return nil, err
	}
	maps.Copy(out, gofiles)
	if len(in.PluginSchemas) > 0 {
		plugin, err := pluginwire.Generate(in.PluginSchemas)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, plugin)
	}
	out[FieldReferenceFile] = []byte(contract.FieldReference(files, SchemaDir))
	for p, b := range in.Fixture {
		out[FixtureMirrorDir+p] = b
	}
	names := make([]string, 0, len(in.Consumers))
	for n := range in.Consumers {
		names = append(names, n)
	}
	slices.Sort(names)
	var cons []consumer
	var errs []error
	for _, n := range names {
		c, err := parseConsumer(n, in.Consumers[n])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, r := range c.Reads {
			if miss := t.missing(r.Document, r.Fields); len(miss) > 0 {
				errs = append(errs, fmt.Errorf("%s: the %s reads fields the %s schema lacks: %s", n, c.Name, r.Document, strings.Join(miss, ", ")))
			}
		}
		cons = append(cons, c)
	}
	var cnames []string
	for _, c := range cons {
		cnames = append(cnames, c.Name)
		out[TSDir+c.Name+"-reads.gen.ts"] = readsFile(c)
	}
	maps.Copy(out, tsFiles(t, cnames))
	listed, err := exportsFile(exportNames(t, cons))
	out[ExportsFile] = listed
	return out, errors.Join(append(errs, err)...)
}

// ContractPackage is the folder of the TypeScript package that is generated.
const ContractPackage = "explorer/packages/contract"

// ExportsFile lists the names the contract package exports, for the surface
// listing (M0a-25). The code that writes the package is the one that knows
// them, so the listing cannot differ from the package; gragas-dev surface
// reads this file and links no generator.
const ExportsFile = ContractPackage + "/exports.gen.json"

// Name is one name the contract package exports.
type Name struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Summary   string `json:"summary"`
}

// exportNames lists what the package exports: the zod schema of each declaration,
// which is also its type, and each consumer's field list.
func exportNames(t *tree, cons []consumer) []Name {
	var out []Name
	for _, d := range t.Decls {
		out = append(out, Name{d.Name, "const", "zod schema; z.infer of it is the type of the same name", firstLine(d.Title)})
	}
	for _, c := range cons {
		out = append(out, Name{c.Name + "Reads", "const", "the fields of each document the " + c.Name + " reads", ""})
	}
	return out
}

func exportsFile(ns []Name) ([]byte, error) {
	b, err := json.MarshalIndent(ns, "", "  ")
	return append(b, '\n'), err
}

// load parses the schemas and builds the tree of them.
func load(in Input) (map[string]*ordered.Map, *tree, error) {
	files := map[string]*ordered.Map{}
	for name, data := range in.Schemas {
		v, err := ordered.ParseYAML(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s is not JSON: %w", name, err)
		}
		m, ok := v.(*ordered.Map)
		if !ok {
			return nil, nil, fmt.Errorf("%s is not a JSON object", name)
		}
		files[name] = m
	}
	t, err := build(files)
	return files, t, err
}
