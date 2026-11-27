// Package planned holds the commands gragas decided on and has not built: one
// YAML file each, in this folder. A planned command has no code, so its
// declaration is data; building it deletes its file and adds its package below
// internal/cli/cmd in the same change, and the registry refuses a name in both.
// A file is a row of the table of spec 15.1 that is absent from the command
// tree (cli.commands.9).
package planned

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// Files are the planned commands, one YAML file each.
//
//go:embed *.yaml
var Files embed.FS

// keys are the keys of a file, and which of them it must have.
func keys() (all, required []string) {
	return []string{"name", "order", "context", "ends", "optin", "summary", "then", "examples", "writes", "exits", "json"},
		[]string{"name", "context", "ends", "summary", "examples", "exits", "json"}
}

// Load reads the planned commands from fsys, sorted by name. A file that is
// not a row is an error, and Load reads the others: the error names each one.
func Load(fsys fs.FS) ([]kit.Command, error) {
	names, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	var out []kit.Command
	var errs []error
	for _, file := range names {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c, err := parse(file, data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		out = append(out, c)
	}
	return out, errors.Join(errs...)
}

func parse(file string, data []byte) (kit.Command, error) {
	doc, err := ordered.ParseYAML(data)
	if err != nil {
		return kit.Command{}, err
	}
	m, ok := doc.(*ordered.Map)
	if !ok {
		return kit.Command{}, errors.New("not a map of keys")
	}
	all, required := keys()
	var bad []string
	for _, k := range m.Keys() {
		if !slices.Contains(all, k) {
			bad = append(bad, k+" is not a key")
		}
	}
	for _, k := range required {
		if _, ok := m.Get(k); !ok {
			bad = append(bad, k+" is missing")
		}
	}
	if name := m.Str("name"); name+".yaml" != file {
		bad = append(bad, fmt.Sprintf("name %q is not the file's", name))
	}
	if len(bad) > 0 {
		return kit.Command{}, errors.New(strings.Join(bad, "; "))
	}
	c := kit.Command{Name: m.Str("name"), Spec: true, Context: kit.Context(m.Str("context")), Ends: kit.Ends(m.Str("ends")),
		OptIn: m.Truthy("optin"), Summary: m.Str("summary"), Then: ordered.Strings(m.List("then")),
		Writes: ordered.Strings(m.List("writes")), JSONKind: m.Str("json"), State: kit.NotBuilt}
	switch n := m.Val("order").(type) {
	case nil:
	case int:
		c.Order = n
	case int64:
		c.Order = int(n)
	default:
		return kit.Command{}, errors.New("order is not a whole number")
	}
	for _, w := range ordered.Strings(m.List("exits")) {
		c.Exits = append(c.Exits, model.Word(w))
	}
	ex := ordered.Maps(m.List("examples"))
	if len(ex) != 2 {
		return kit.Command{}, fmt.Errorf("examples are %d, want two", len(ex))
	}
	for i, e := range ex {
		c.Examples[i] = kit.Example{Line: e.Str("line"), Does: e.Str("does")}
	}
	return c, nil
}
