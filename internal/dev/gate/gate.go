// Package gate is the domain of the engine's own gate registry: the files
// under gate/ that hold every shrink-only list and exception (A45,
// architecture 13.2). It reads them, holds each to its schema and to what it
// says about the tree, and returns the findings. It reaches nothing itself:
// the files, the schema check and the linked modules arrive in Inputs, so a
// test plants any state it likes.
package gate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// The registry's files, from the repository root, and the schema each is held
// to (under SchemaDir, by name).
const (
	Dir       = "gate"
	SchemaDir = "gate/schema"

	Exemptions  = "gate/exemptions.yaml"
	Standins    = "gate/standins.txt"
	ClosureList = "gate/closure.txt"
	// Crash is JSON, which the YAML reader reads as it reads YAML.
	Crash = "gate/crash.json"
	// Subsets is JSON, which the YAML reader reads as it reads YAML.
	Subsets = "gate/subsets.json"
	// Shared is the shared capabilities' rows (arch.shared.8).
	Shared = "gate/shared.yaml"
)

// ErrMissing is a registry file that is not there; ErrEmpty one that holds
// nothing. A gate that cannot read its input fails (gates.plants.5).
var (
	ErrMissing = errors.New("is missing")
	ErrEmpty   = errors.New("holds nothing")
)

// Inputs is everything the checks read. A nil function is a reader that finds
// nothing.
type Inputs struct {
	// Read returns a file of the repository, by slash path from its root;
	// an error that matches fs.ErrNotExist says it is not there.
	Read func(path string) ([]byte, error)
	// Validate holds doc to the schema file named schema and returns what
	// is wrong with it, each as one line.
	Validate func(schema string, doc any) []string
	// Closure are the external modules the shipped binary links.
	Closure []string
	// Base returns a file as the base revision (the remote main) has it, by
	// slash path, and nil with no error when the base holds no file there.
	// Nil is no base: the lists that only shrink against it are not held.
	Base func(path string) ([]byte, error)
	// Names returns the names of the regular files of a folder of the
	// repository (a folder or a link named like one is none),
	// by slash path from its root, each as git writes it: a name is held by
	// its exact spelling, never by a file system that folds case.
	Names func(dir string) ([]string, error)
}

// Report is what the check found. Findings fail the tier. Lengths is a line
// for each list that only shrinks, its length beside what the registry
// records for it, so a list at its baseline is seen on every run
// (gates.ratchets.1); it fails nothing. The exemptions' line is gragas
// check's, which gragas-dev prints beside these.
type Report struct {
	Findings []string
	Lengths  []string
}

// Registry is the files, read and checked against their schemas.
type Registry struct {
	in   Inputs
	bad  []string
	docs map[string]*ordered.Map
	text map[string][]entry
	// names are the files of the decision records, once listed.
	names []string
	// lengths are the lines of Report.Lengths.
	lengths []string
}

// entry is one line of a text registry file: the value and the reason after
// the #.
type entry struct {
	Value, Reason string
	Line          int
}

// Check loads every registry file, holds it to its schema, and runs every
// check over what the tree says. It returns the findings, none when the gate
// is satisfied.
func Check(in Inputs) Report {
	r := &Registry{in: in, docs: map[string]*ordered.Map{}, text: map[string][]entry{}}
	r.load()
	r.closure()
	r.subsets()
	r.sharedShrink()
	return Report{Findings: r.bad, Lengths: r.lengths}
}

func (r *Registry) add(file string, format string, args ...any) {
	r.bad = append(r.bad, file+": "+fmt.Sprintf(format, args...))
}

// yamlFiles are the registry files that are YAML, with the schema and the
// list each must hold at least one row of.
func yamlFiles() []struct{ file, schema, list string } {
	return []struct{ file, schema, list string }{
		{Exemptions, "exemptions", "exemptions"},
		{Crash, "crash", "crash"},
		{Subsets, "subsets", "subsets"},
	}
}

// textFiles are the registry files that are one entry a line.
func textFiles() []string {
	return []string{Standins, ClosureList}
}

// read is a registry file's bytes, or the finding that it is missing or
// empty.
func (r *Registry) read(file string) ([]byte, bool) {
	if r.in.Read == nil {
		r.add(file, "%v", ErrMissing)
		return nil, false
	}
	data, err := r.in.Read(file)
	switch {
	case err != nil:
		r.add(file, "%v: %v", ErrMissing, err)
		return nil, false
	case strings.TrimSpace(string(data)) == "":
		r.add(file, "%v", ErrEmpty)
		return nil, false
	}
	return data, true
}

func (r *Registry) load() {
	for _, f := range yamlFiles() {
		r.loadYAML(f.file, f.schema, f.list)
	}
	for _, f := range textFiles() {
		data, ok := r.read(f)
		if !ok {
			continue
		}
		es := parseText(string(data))
		if len(es) == 0 {
			r.add(f, "%v", ErrEmpty)
			continue
		}
		r.text[f] = es
		r.textSchema(f, es)
	}
}

// loadYAML reads one YAML registry file, holds it to its schema, and keeps
// it for the checks; its list key must hold at least one row.
func (r *Registry) loadYAML(file, schema, list string) {
	data, ok := r.read(file)
	if !ok {
		return
	}
	v, err := ordered.ParseStrict(data)
	doc, isMap := v.(*ordered.Map)
	switch {
	case err != nil:
		r.add(file, "%v", err)
		return
	case !isMap:
		r.add(file, "is not a mapping")
		return
	}
	r.docs[file] = doc
	if r.in.Validate == nil {
		r.add(file, "no schema check was given, so nothing was checked")
	} else {
		for _, p := range r.in.Validate(schema, v) {
			r.add(file, "%s", p)
		}
	}
	if len(doc.List(list)) == 0 {
		r.add(file, "%s: %v", list, ErrEmpty)
	}
}

// parseText reads a text registry file: one value a line, then a # and the
// reason; a line that starts with # is a comment.
func parseText(s string) []entry {
	var out []entry
	for i, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		v, why, _ := strings.Cut(t, "#")
		out = append(out, entry{Value: strings.TrimSpace(v), Reason: strings.TrimSpace(why), Line: i + 1})
	}
	return out
}

// textSchema holds the entries of a text file to the one schema they share.
func (r *Registry) textSchema(file string, es []entry) {
	if r.in.Validate == nil {
		return
	}
	// The closure list is a list of modules and has no reasons.
	list := make([]any, 0, len(es))
	for _, e := range es {
		m := ordered.NewMap(e.Line)
		m.Set("value", e.Value, e.Line)
		if file != ClosureList {
			m.Set("reason", e.Reason, e.Line)
		}
		list = append(list, m)
	}
	doc := ordered.NewMap(1)
	doc.Set("entries", list, 1)
	schema := "entries"
	if file == ClosureList {
		schema = "modules"
	}
	for _, p := range r.in.Validate(schema, doc) {
		r.add(file, "%s", p)
	}
}
