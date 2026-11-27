// Package rulebook is the rulebook's domain (schema v2, docs/schema-v2.md): its
// parts, product kinds, sources, concern files, rules and ids, the checks
// across files, each rule's derived standing, and the baselines that may
// only fall.
//
// It reads through legacy.Source and checks shapes through legacy.Checker, so it
// touches no disk of its own and a test hands it a tree in memory.
package rulebook

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// Dir is the rulebook's folder, relative to the repository root.
const Dir = "rulebook"

// defaultOrder places a concern file with no order after every ordered one.
const defaultOrder = 999

// SchemaDir is the folder of the JSON Schemas, relative to the repository root.
const SchemaDir = Dir + "/schema"

// Schema file names, under SchemaDir.
const (
	schemaCommon   = "common.schema.json"
	schemaRules    = "rules.schema.json"
	schemaTools    = "tools.schema.json"
	schemaProducts = "products.schema.json"
	schemaParts    = "parts.schema.json"
	schemaSources  = "sources.schema.json"
	// SchemaRulesJSON is the shape of the generated rules.json.
	SchemaRulesJSON = "rules-json.schema.json"
)

// The rulebook's files besides the parts' own, under Dir.
const (
	partsFile    = "parts.yaml"
	productsFile = "products.yaml"
	sourcesFile  = "sources.yaml"
	// LockFile lists every rule id ever issued; gragas gen writes it.
	LockFile = "ids.lock"
	// BaselineFile holds the baselines, which may only fall.
	BaselineFile = "baseline.json"
)

// bindingStrengths are the strengths that bind as written; a measured one
// binds only once it is promoted.
func bindingStrengths() []string { return []string{"Hard", "Answered", "Session"} }

// measuredStrengths are the strengths that are measured, and bind once promoted.
func measuredStrengths() []string { return []string{"Lens", "Record"} }

// Finding is one problem in the rulebook, at a file and, where known, a line.
type Finding struct {
	Path string
	Line int
	Msg  string
}

// String is "path:line: message", or "path: message" with no line.
func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Msg)
	}
	return f.Path + ": " + f.Msg
}

type finding struct {
	line int
	msg  string
}

// File is one concern file, rulebook/<layer>/<part>/rules/<concern>.yaml.
type File struct {
	Part    string
	Concern string
	Path    string
	Doc     *ordered.Map
}

// Order is the concern's position in its part's README.
func (f *File) Order() int {
	if v, ok := f.Doc.Get("order"); ok {
		if n, ok := v.(int64); ok {
			return int(n)
		}
	}
	return defaultOrder
}

// Rules are the entries in the file, in file order.
func (f *File) Rules() []*ordered.Map { return ordered.Maps(f.Doc.List("rules")) }

// Rule is one rule entry and where it lives.
type Rule struct {
	Part    string
	Concern string
	Path    string
	Data    *ordered.Map
}

// Book is the loaded rulebook.
type Book struct {
	Parts *Parts
	// Files are the concern files, by part in parts.yaml order, then by name.
	Files []*File
	// Rules are the entries by id. When an id is used twice the later one is kept.
	Rules map[string]*Rule
	// Tools are each part's stack, from its tools.yaml.
	Tools map[string][]*ordered.Map
	// Guides marks the parts that have a guide.md.
	Guides map[string]bool
	// Kinds are the product kinds of rulebook/products.yaml, in file order.
	Kinds []*ordered.Map
	// Sources is the registry of rulebook/sources.yaml, by key.
	Sources *ordered.Map
	// Lock is every id ever issued, from rulebook/ids.lock, sorted.
	Lock []string
	// Counts are the baseline invariants as counted now, by name.
	Counts map[string]int
	// Findings are the problems found, in the order they were found.
	Findings []Finding

	schemas  legacy.Checker
	names    NameFinder
	src      legacy.Source
	tools    map[string]*tool
	standing map[string]string
	kinds    map[string]bool
}

// NameFinder is a list of names that public text must not hold: Find returns
// the first it finds in text, or "". It is an interface so that the list may
// be written as patterns or as hashes; the finding prints what Find returns.
type NameFinder interface{ Find(text string) string }

// Options are the inputs a load takes besides the tree.
type Options struct {
	// Names, when set, turns on the forbidden-names check.
	Names NameFinder
}

// findName is the first forbidden name in text, or "" when there is none or no
// list was given.
func (b *Book) findName(text string) string {
	if b.names == nil {
		return ""
	}
	return b.names.Find(text)
}

// ErrNoParts means the tree has no rulebook/parts.yaml, so it is not a rulebook.
var ErrNoParts = errors.New("no " + Dir + "/parts.yaml")

// SchemaFiles are the schema files a load checks documents against, in
// SchemaDir; an adapter compiles them before Load.
func SchemaFiles() []string {
	return []string{schemaCommon, schemaRules, schemaTools, schemaProducts, schemaParts, schemaSources, SchemaRulesJSON}
}

// Probe reports ErrNoParts when src holds no rulebook, before any schema is read.
func Probe(src legacy.Source) error {
	if _, err := fs.Stat(src, path.Join(Dir, "parts.yaml")); err != nil {
		return fmt.Errorf("%w: %w", ErrNoParts, err)
	}
	return nil
}

// Load reads and checks the rulebook in src. Problems in the rulebook are
// returned as Findings; an error means the rulebook could not be read at all.
func Load(ctx context.Context, src legacy.Source, schemas legacy.Checker, opts Options) (*Book, error) {
	partsPath := path.Join(Dir, "parts.yaml")
	if err := Probe(src); err != nil {
		return nil, err
	}
	b := &Book{
		Rules:    map[string]*Rule{},
		Tools:    map[string][]*ordered.Map{},
		Guides:   map[string]bool{},
		Sources:  ordered.NewMap(0),
		Counts:   map[string]int{},
		schemas:  schemas,
		names:    opts.Names,
		src:      src,
		tools:    map[string]*tool{},
		standing: map[string]string{},
		kinds:    map[string]bool{},
	}
	doc := b.doc(src, partsPath, schemaParts)
	if doc == nil {
		return b, nil
	}
	ps, bad := partsFromDoc(doc)
	b.Parts = ps
	b.add(partsPath, bad...)
	b.loadProducts(src)
	b.loadSources(src)
	for _, p := range ps.List {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("load part %s: %w", p.Short, err)
		}
		if err := b.loadPart(ctx, src, p); err != nil {
			return nil, err
		}
	}
	b.loadPresets(src)
	b.checkReplacedBy()
	b.checkLinks()
	if err := b.checkFolders(src); err != nil {
		return nil, err
	}
	if err := b.loadLock(src); err != nil {
		return nil, err
	}
	b.derive()
	return b, nil
}

func (b *Book) add(p string, found ...finding) {
	for _, f := range found {
		b.Findings = append(b.Findings, Finding{Path: p, Line: f.line, Msg: f.msg})
	}
}

// doc reads one YAML file and checks it against a schema. It returns nil, and
// records why, when the file cannot be read, parsed or fails the schema.
func (b *Book) doc(src legacy.Source, p, schema string) *ordered.Map {
	data, err := fs.ReadFile(src, p)
	if err != nil {
		b.add(p, finding{msg: "read: " + err.Error()})
		return nil
	}
	v, err := ordered.ParseYAML(data)
	if err != nil {
		msg := err.Error()
		if se, ok := errors.AsType[*ordered.SyntaxError](err); ok {
			msg = se.Detail
		}
		b.add(p, finding{msg: "YAML: " + msg})
		return nil
	}
	if bad := b.schemas.Validate(schema, v); len(bad) > 0 {
		for _, x := range bad {
			b.add(p, finding{line: x.Line, msg: x.Msg})
		}
		return nil
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		b.add(p, finding{msg: "schema at top: not a mapping"})
		return nil
	}
	return m
}

func (b *Book) loadPart(ctx context.Context, src legacy.Source, p Part) error {
	rdir := path.Join(p.Dir(), "rules")
	entries, err := fs.ReadDir(src, rdir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("list %s: %w", rdir, err)
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("load %s: %w", rdir, err)
		}
		fp := path.Join(rdir, e.Name())
		if !strings.HasSuffix(e.Name(), ".yaml") {
			b.add(fp, finding{msg: "only .yaml files belong in rules/"})
			continue
		}
		b.loadConcern(src, p.Short, fp, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	if _, err := fs.Stat(src, path.Join(p.Dir(), "guide.md")); err == nil {
		b.Guides[p.Short] = true
	}
	tpath := path.Join(p.Dir(), "tools.yaml")
	if _, err := fs.Stat(src, tpath); err == nil {
		b.loadTools(src, p.Short, tpath)
	}
	return nil
}

func (b *Book) loadConcern(src legacy.Source, short, fp, concern string) {
	doc := b.doc(src, fp, schemaRules)
	if doc == nil {
		return
	}
	if got := doc.Str("part"); got != short {
		b.add(fp, finding{line: doc.KeyLine("part"), msg: fmt.Sprintf("part is %s, folder says %s", ordered.Repr(got), ordered.Repr(short))})
	}
	if got := doc.Str("concern"); got != concern {
		b.add(fp, finding{line: doc.KeyLine("concern"), msg: fmt.Sprintf("concern is %s, file says %s", ordered.Repr(got), ordered.Repr(concern))})
	}
	f := &File{Part: short, Concern: concern, Path: fp, Doc: doc}
	b.Files = append(b.Files, f)
	for _, r := range f.Rules() {
		id := r.Str("id")
		if _, dup := b.Rules[id]; dup {
			b.add(fp, finding{line: r.Line, msg: fmt.Sprintf("id %s used twice", id)})
		}
		b.Rules[id] = &Rule{Part: short, Concern: concern, Path: fp, Data: r}
		b.add(fp, b.checkRule(short, r)...)
	}
}

func (b *Book) loadTools(src legacy.Source, short, tpath string) {
	doc := b.doc(src, tpath, schemaTools)
	if doc == nil {
		return
	}
	if got := doc.Str("part"); got != short {
		b.add(tpath, finding{line: doc.KeyLine("part"), msg: fmt.Sprintf("part is %s, folder says %s", ordered.Repr(got), ordered.Repr(short))})
	}
	tools := ordered.Maps(doc.List("tools"))
	for _, t := range tools {
		key := t.Str("key")
		if _, dup := b.tools[key]; dup {
			b.add(tpath, finding{line: t.Line, msg: fmt.Sprintf("tool key %s listed twice", ordered.Repr(key))})
			continue
		}
		b.tools[key] = &tool{part: short, data: t}
	}
	b.Tools[short] = tools
	if m := b.findName(ordered.JSON(doc, -1)); m != "" {
		b.add(tpath, finding{msg: fmt.Sprintf("names %s: public text must stay generic", ordered.Repr(m))})
	}
}

// loadProducts reads the product kinds a when or an unless may name.
func (b *Book) loadProducts(src legacy.Source) {
	ppath := path.Join(Dir, productsFile)
	doc := b.doc(src, ppath, schemaProducts)
	if doc == nil {
		return
	}
	b.Kinds = ordered.Maps(doc.List("kinds"))
	for _, k := range b.Kinds {
		key := k.Str("key")
		if b.kinds[key] {
			b.add(ppath, finding{line: k.Line, msg: fmt.Sprintf("kind %s listed twice", key)})
		}
		b.kinds[key] = true
	}
}

// loadSources reads the registry every citation names.
func (b *Book) loadSources(src legacy.Source) {
	doc := b.doc(src, path.Join(Dir, sourcesFile), schemaSources)
	if doc == nil {
		return
	}
	if s := doc.Sub("sources"); s != nil {
		b.Sources = s
	}
}

// checkReplacedBy: a replaced_by names a rule that exists, and not itself.
func (b *Book) checkReplacedBy() {
	for _, f := range b.Files {
		for _, r := range f.Rules() {
			to := r.Str("replaced_by")
			if to == "" {
				continue
			}
			id := r.Str("id")
			if _, ok := b.Rules[to]; !ok || to == id {
				b.add(f.Path, finding{line: r.Line, msg: fmt.Sprintf("%s: replaced_by %s is not another rule", id, to)})
			}
		}
	}
}

// checkFolders: every folder under a layer is a part of that layer, and every
// folder beside schema/ is a layer, so a misplaced part is not skipped unseen.
func (b *Book) checkFolders(src legacy.Source) error {
	top, err := fs.ReadDir(src, Dir)
	if err != nil {
		return fmt.Errorf("list %s: %w", Dir, err)
	}
	layers := map[string]bool{}
	for _, l := range b.Parts.Layers {
		layers[l.Key] = true
	}
	for _, e := range top {
		if !e.IsDir() || e.Name() == "schema" || e.Name() == "pairs" { // pairs/<rule id>/<n>/ is read by nothing here (A56); this allowance ends with the old check at M6
			continue
		}
		if !layers[e.Name()] {
			b.add(path.Join(Dir, e.Name()), finding{msg: "folder is not a layer in parts.yaml"})
			continue
		}
		subs, err := fs.ReadDir(src, path.Join(Dir, e.Name()))
		if err != nil {
			return fmt.Errorf("list %s: %w", path.Join(Dir, e.Name()), err)
		}
		for _, s := range subs {
			if !s.IsDir() {
				continue
			}
			if p, ok := b.Parts.Part(s.Name()); !ok || p.Layer != e.Name() {
				b.add(path.Join(Dir, e.Name(), s.Name()), finding{msg: "folder is not a part of layer " + e.Name() + " in parts.yaml"})
			}
		}
	}
	return nil
}

// IDs returns every rule id, sorted.
func (b *Book) IDs() []string {
	ids := make([]string, 0, len(b.Rules))
	for id := range b.Rules {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// HasGuide reports whether the part has a guide.md beside its rules.
func (b *Book) HasGuide(p Part) bool { return b.Guides[p.Short] }
