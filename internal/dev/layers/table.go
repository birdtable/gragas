// Package layers is the domain of a Go program's layer table: the one file
// that names its packages, the layer each sits in and what each may import
// (arch.layers.1, .8, .9). It reads the table, answers what may import what,
// and renders from it the layer document and the depguard rows of the lint
// file, so neither is written by hand.
package layers

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// File is the table's file, from the repository root.
const File = "layers.yaml"

// The kinds of layer.
const (
	// KindPure is a layer whose non-test files import no effect package and
	// no library but a pure one (arch.ports.1).
	KindPure = "pure"
	// KindEdge is the entry and the command tree, which reach the process
	// and the terminal themselves.
	KindEdge = "edge"
	// KindHost is the adapters, one package per tool.
	KindHost = "host"
	// KindStandIn is a test-only package with files of its own, imported
	// from _test.go files only.
	KindStandIn = "stand-in"
	// KindTests is a package of _test.go files only.
	KindTests = "tests"
)

// ErrTable is a layer table that cannot be read as one.
var ErrTable = errors.New("the layer table")

// milestone is a milestone of the architecture's phasing.
var milestone = regexp.MustCompile("^M(0[ab]|[1-9]|10)$")

// taskID is a task of a milestone: M0a-19.
var taskID = regexp.MustCompile("^M(0[ab]|[1-9]|10)-[0-9]+$")

// programName is the shape of an outside program's name. The name is
// written into the lint file, so nothing else is taken: no newline, quote,
// colon or space could start a rule of its own there.
var programName = regexp.MustCompile("^[a-z][a-z0-9-]*$")

// Table is a program's layer table.
type Table struct {
	// Module is the module path every package path is relative to.
	Module string
	// tree is the packages of the repository, set by WithTree: with it, a
	// pattern is written as one rule for each package it holds.
	tree []string
	// DocFile, LintFile and EslintFile are the files the table renders, from
	// the repository root.
	DocFile, LintFile, EslintFile string
	// LintBase and LintProject are the two files the lint file is joined from
	// with the table: the shared base, and the project's overrides.
	LintBase, LintProject string
	// About is the document's opening paragraph.
	About string
	// Rows are the layers, in the order written.
	Rows []Row
	// Effects are the packages only an edge or an adapter imports, and the
	// reason a breach says.
	Effects    []string
	EffectsWhy string
	// EffectCalls are calls, as package path dot name, that reach the world
	// from a package a pure layer may import.
	EffectCalls []string
	// External is every library and its one owner.
	External []External
	// Shared is the shared registry, gate/shared.yaml, whose bans the lint
	// file holds; nil renders none (arch.shared.8).
	Shared *Shared
	// Ports are the ports, with their adapters and fake.
	Ports Ports
	// Programs are the outside programs, each with the one adapter package
	// that starts it (arch.ports.6, cli.process.1).
	Programs []Program
	// Platform is the words no core file holds: system roots, OS tools and
	// OS names (arch.ports.4, arch.ports.5).
	Platform Platform
	// Source is what the source test holds core and cli files to.
	Source Source
	// Planned are the packages the architecture names that do not exist yet,
	// each with the milestone that creates it. A package here is in no row.
	Planned []Planned
	// Contexts are the bounded contexts, whose packages refuse one another
	// (arch.ports.8).
	Contexts Contexts
	// Explorer is the TypeScript workspace's modules and their allowed imports.
	Explorer Explorer
}

// Planned are packages, relative to the module, that a milestone creates,
// each in the layer it will join. A planned package is in no row yet: it moves
// into its row in the pull request that creates it.
type Planned struct {
	// Created is the milestone, M0a to M10.
	Created string
	// Layer is the row the packages join when they exist.
	Layer    string
	Packages []string
}

// Contexts are the bounded contexts: each is the package internal/core/<name>,
// and no one of them imports another.
type Contexts struct {
	Why   string
	Names []string
}

// Explorer is the explorer workspace's modules, each with the modules it may
// import.
type Explorer struct {
	Why     string
	Modules []Module
}

// Module is one module of the explorer. A name ending in /* stands for each
// folder below it, which may not import one another.
type Module struct {
	Name, Path string
	May        []string
}

// Row is one layer.
type Row struct {
	Layer, Kind string
	// Packages are the layer's packages, relative to the module.
	Packages []string
	// Patterns are folders whose every package below joins the layer, written
	// folder/...: a new package there is no new line (the layer holds the
	// folder). A pattern is also a name a row may list in May.
	Patterns []string
	// Carves are the packages another row lists that this row's patterns hold
	// and the row gives up: each is held by its own row, and the table says so
	// here, so a package taken out of a pattern shows in the diff.
	Carves []string
	// Holds says what the packages hold; Template where the template puts
	// the layer, or why it sits where it does.
	Holds, Template string
	// May are the packages of the module the layer's packages may import,
	// beside their own and the stand-ins.
	May []string
	// Std are the standard packages a pure layer's non-test files may
	// import, each exactly: an allow list (arch.ports.1).
	Std []string
	// Why is the reason a forbidden import says.
	Why string
	// Until is the milestone by which a temporary row is gone, or empty.
	Until string
	// Product is why the row's packages stay in this program, and
	// Home is the platform repository they belong to; a row names at most one.
	Product, Home string
}

// Also is one other owner of a library: Until is the task that ends it, Why the
// reason it stays when no task does.
type Also struct {
	Owner, Until, Why string
	// Files narrows the owner to some of its files: "tests" is its test files, any
	// other item one file of its folder by name. None means the whole folder.
	Files []string
}

// TestsOnly is the item of Files that stands for an owner's test files.
const TestsOnly = "tests"

// Globs are the depguard file globs of the owner, to be spared the ban: its
// whole folder when Files names nothing, else the files it names.
func (a Also) Globs() []string {
	if len(a.Files) == 0 {
		return []string{"!**/" + a.Owner + "/*.go"}
	}
	out := make([]string, 0, len(a.Files))
	for _, f := range a.Files {
		if f == TestsOnly {
			f = "*_test.go"
		}
		out = append(out, "!**/"+a.Owner+"/"+f)
	}
	return out
}

// External is a library, by import path prefix, and the one package that
// imports it; Owner is "" for a library imported nowhere.
type External struct {
	Path, Owner, Why string
	// Within are patterns (folder/...) whose every package may import the
	// library beside the owner.
	Within []string
	// Also are the other owners: packages that import the library beside the
	// owner. One that is going names the task that moves it off (Until, as
	// M0a-19); one that stays says why. An also that no longer imports the
	// library fails the layer test, so a row cannot outlive its move.
	Also []Also
	// Pure is a library a pure package may import.
	Pure bool
	// Replaces are the import paths this library took the place of: each is
	// banned by name in every file, and the failure says what to import
	// instead (supply.intake.1).
	Replaces []string
}

// Ports are the port package's ports.
type Ports struct {
	// Fake is the stand-in that fills every port.
	Fake string
	// Carries are the port package's other interfaces.
	Carries []string
	List    []Port
}

// Platform is the words of one system or another: path roots a core file
// never starts a string with, OS tools and OS names it never names.
type Platform struct {
	Roots, Tools, Names []string
	// Homes are system folders below a home: Library/LaunchAgents.
	Homes []string
}

// Source is the source test's table: the folders whose Go strings and names
// stay free of any rulebook or tool (arch.ports.2), the shape of a rule id,
// the tools and their config formats, the package names the template
// refuses (arch.structure.7), the line limit of one layer's files, and the
// init functions outside main, which only fall.
type Source struct {
	Scope, Tools, Formats, Refused []string
	RuleID                         string
	LimitLayer                     string
	LimitLines, Inits              int
}

// Program is an outside program and the one adapter package that starts it.
// The adapter is a host package that exists: a program joins the table in the
// pull request that builds its adapter.
type Program struct {
	Name, Adapter string
	// File is the one file of the adapter that starts the program, when the
	// package is not the only place that may import os/exec: "" is the whole
	// package.
	File string
}

// Port is one port, the subject it is, and the adapter that fills it.
type Port struct {
	Name, Subject, Adapter, Why string
}

// Parse reads the table from data. Every problem it finds is in the error,
// which matches ErrTable.
func Parse(data []byte) (*Table, error) {
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTable, err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%w: not a mapping", ErrTable)
	}
	p := &parser{}
	t := p.table(doc)
	p.check(t)
	if len(p.bad) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrTable, strings.Join(p.bad, "; "))
	}
	return t, nil
}

// parser gathers the problems of one table.
type parser struct {
	bad []string
}

func (p *parser) add(line int, format string, args ...any) {
	p.bad = append(p.bad, fmt.Sprintf("line %d: ", line)+fmt.Sprintf(format, args...))
}

// keys refuses a key of m not in known, and a missing one of need.
func (p *parser) keys(m *ordered.Map, known, need []string) {
	for _, k := range m.Keys() {
		if !slices.Contains(known, k) {
			p.add(m.KeyLine(k), "%s is not a key of the layer table here", k)
		}
	}
	for _, k := range need {
		if !m.Truthy(k) {
			p.add(m.Line, "%s is missing", k)
		}
	}
}

// productOrHome refuses a row that names both a product and a home: a package
// stays in this program or belongs elsewhere, not both.
func (p *parser) productOrHome(m *ordered.Map) {
	if m.Truthy("product") && m.Truthy("home") {
		p.add(m.KeyLine("product"), "layer %s: a row names a product or a home, not both", m.Str("layer"))
	}
}

func (p *parser) table(doc *ordered.Map) *Table {
	p.keys(doc, []string{"module", "renders", "about", "layers", "effects", "external", "ports", "programs", "platform", "source", "planned", "contexts", "explorer"},
		[]string{"module", "renders", "about", "layers", "effects", "ports", "platform", "source", "contexts", "explorer"})
	t := &Table{Module: doc.Str("module"), About: doc.Str("about")}
	if r := doc.Sub("renders"); r != nil {
		p.keys(r, []string{"doc", "lint", "lint-base", "lint-project", "eslint"}, []string{"doc", "lint", "lint-base", "lint-project", "eslint"})
		t.DocFile, t.LintFile, t.EslintFile = r.Str("doc"), r.Str("lint"), r.Str("eslint")
		t.LintBase, t.LintProject = r.Str("lint-base"), r.Str("lint-project")
	}
	for _, m := range ordered.Maps(doc.List("layers")) {
		p.keys(m, []string{"layer", "kind", "packages", "patterns", "carves", "holds", "template", "may", "std", "why", "until", "product", "home"},
			[]string{"layer", "kind", "holds", "template", "why"})
		p.productOrHome(m)
		t.Rows = append(t.Rows, Row{Layer: m.Str("layer"), Kind: m.Str("kind"), Packages: ordered.Strings(m.List("packages")), Patterns: ordered.Strings(m.List("patterns")), Carves: ordered.Strings(m.List("carves")),
			Holds: m.Str("holds"), Template: m.Str("template"), May: ordered.Strings(m.List("may")), Std: ordered.Strings(m.List("std")), Why: m.Str("why"), Until: m.Str("until"),
			Product: m.Str("product"), Home: m.Str("home")})
		if u := m.Str("until"); u != "" && !milestone.MatchString(u) {
			p.add(m.KeyLine("until"), "layer %s: until %q is no milestone (M0a, M0b, M1 to M10)", m.Str("layer"), u)
		}
		if (m.Str("kind") == KindPure) != m.Truthy("std") {
			p.add(m.Line, "layer %s: a pure layer, and only a pure layer, lists its standard packages in std", m.Str("layer"))
		}
		if !slices.Contains([]string{KindPure, KindEdge, KindHost, KindStandIn, KindTests}, m.Str("kind")) {
			p.add(m.KeyLine("kind"), "layer %s: kind %q is not one of pure, edge, host, stand-in, tests", m.Str("layer"), m.Str("kind"))
		}
	}
	if e := doc.Sub("effects"); e != nil {
		p.keys(e, []string{"why", "packages", "calls"}, []string{"why", "packages"})
		t.Effects, t.EffectsWhy, t.EffectCalls = ordered.Strings(e.List("packages")), e.Str("why"), ordered.Strings(e.List("calls"))
	}
	p.externals(doc, t)
	if pl := doc.Sub("platform"); pl != nil {
		p.keys(pl, []string{"roots", "tools", "names", "homes"}, []string{"roots", "tools", "names"})
		t.Platform = Platform{Roots: ordered.Strings(pl.List("roots")), Tools: ordered.Strings(pl.List("tools")),
			Names: ordered.Strings(pl.List("names")), Homes: ordered.Strings(pl.List("homes"))}
	}
	t.Source = p.sourceTable(doc)
	t.Planned = p.plannedTable(doc)
	t.Contexts, t.Explorer = p.contextsTable(doc), p.explorerTable(doc)
	if ps := doc.Sub("ports"); ps != nil {
		p.keys(ps, []string{"fake", "carries", "list"}, []string{"fake"})
		t.Ports = Ports{Fake: ps.Str("fake"), Carries: ordered.Strings(ps.List("carries"))}
		for _, m := range ordered.Maps(ps.List("list")) {
			p.keys(m, []string{"port", "subject", "adapter", "why"}, []string{"port", "subject", "adapter"})
			t.Ports.List = append(t.Ports.List, Port{Name: m.Str("port"), Subject: m.Str("subject"), Adapter: m.Str("adapter"), Why: m.Str("why")})
		}
	}
	for _, m := range ordered.Maps(doc.List("programs")) {
		p.keys(m, []string{"program", "adapter", "file"}, []string{"program", "adapter"})
		pr := Program{Name: m.Str("program"), Adapter: m.Str("adapter"), File: m.Str("file")}
		if f := pr.File; f != "" && !plainGoFile(f) {
			p.bad = append(p.bad, fmt.Sprintf("program %s: file %q is no plain non-test .go file name", pr.Name, f))
		}
		t.Programs = append(t.Programs, pr)
	}
	return t
}

// plainGoFile is a bare non-test Go file name of lower-case letters, digits
// and underscores: no path, no glob character, no brace or comma that a depguard
// glob would read, no leading dot.
func plainGoFile(f string) bool {
	stem, ok := strings.CutSuffix(f, ".go")
	if !ok || stem == "" || strings.HasSuffix(stem, "_test") {
		return false
	}
	return !strings.ContainsFunc(stem, func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// check holds the table to itself: each package in one row, each name it
// uses a package of a row or a planned one, each owner and adapter where it
// belongs, the contexts apart from one another and the explorer's modules
// without a cycle.
func (p *parser) check(t *Table) {
	seen := p.packages(t)
	p.carves(t)
	planned := p.planned(t, seen)
	known := func(pkg string) bool {
		_, a := seen[pkg]
		_, b := planned[pkg]
		return a || b || t.heldByPattern(pkg)
	}
	for _, r := range t.Rows {
		if len(r.Packages) == 0 && len(r.Patterns) == 0 && len(t.PlannedIn(r.Layer)) == 0 {
			p.bad = append(p.bad, fmt.Sprintf("layer %s holds no package and none is planned for it", r.Layer))
		}
		for _, m := range r.May {
			if !known(m) {
				p.bad = append(p.bad, fmt.Sprintf("layer %s may import %s, which is in no layer and planned for none", r.Layer, m))
			}
		}
	}
	p.checkExternal(t, known)
	p.contexts(t, known)
	p.explorer(t)
	if r := t.RowOf(t.Ports.Fake); r == nil || r.Kind != KindStandIn {
		p.bad = append(p.bad, fmt.Sprintf("the fake %s is not a stand-in", t.Ports.Fake))
	}
	for _, port := range t.Ports.List {
		if !t.isHost(port.Adapter) {
			p.bad = append(p.bad, fmt.Sprintf("port %s: its adapter %s is not in a host layer", port.Name, port.Adapter))
		}
	}
	p.programs(t)
}

// programs holds the outside programs to the rows: each is named once, and
// its adapter is a package of a host row, not a planned one, since the lint
// exclusion written for it spares a package that is there.
func (p *parser) programs(t *Table) {
	named := map[string]bool{}
	for _, pr := range t.Programs {
		if named[pr.Name] {
			p.bad = append(p.bad, fmt.Sprintf("program %s is named twice: one adapter starts it", pr.Name))
		}
		named[pr.Name] = true
		if !programName.MatchString(pr.Name) {
			p.bad = append(p.bad, fmt.Sprintf("program %q is not a name of the shape %s", pr.Name, programName))
		}
		if pr.File != "" && (strings.ContainsAny(pr.File, "/*?[\"") || !strings.HasSuffix(pr.File, ".go")) {
			p.bad = append(p.bad, fmt.Sprintf("program %s: file %q is not the name of a Go file of its adapter", pr.Name, pr.File))
		}
		if r := t.RowOf(pr.Adapter); r == nil || r.Kind != KindHost {
			p.bad = append(p.bad, fmt.Sprintf("program %s: its adapter %s is not a package of a host layer", pr.Name, pr.Adapter))
		}
	}
}

// ProgramAdapters are the packages that start an outside program, each once,
// in the order the table names them.
func (t *Table) ProgramAdapters() []string {
	var out []string
	for _, pr := range t.Programs {
		if !slices.Contains(out, pr.Adapter) {
			out = append(out, pr.Adapter)
		}
	}
	return out
}

// planned holds the planned packages to the rows: a package a milestone
// creates is in no row yet, is planned once, and joins a layer that is a row.
// It returns each planned package by its layer.
func (p *parser) planned(t *Table, seen map[string]string) map[string]string {
	out := map[string]string{}
	for _, pl := range t.Planned {
		if !slices.ContainsFunc(t.Rows, func(r Row) bool { return r.Layer == pl.Layer }) {
			p.bad = append(p.bad, fmt.Sprintf("planned for %s: layer %s is no row", pl.Created, pl.Layer))
		}
		for _, pk := range pl.Packages {
			if l := seen[pk]; l != "" {
				p.bad = append(p.bad, fmt.Sprintf("%s is planned for %s and is in layer %s already", pk, pl.Created, l))
			}
			if prev, twice := out[pk]; twice {
				p.bad = append(p.bad, fmt.Sprintf("%s is planned twice, for layers %s and %s", pk, prev, pl.Layer))
			}
			out[pk] = pl.Layer
		}
	}
	return out
}

// contexts holds the contexts table: each context is the package
// internal/core/<name>, in a layer or planned, and no layer lets a context
// import another (arch.ports.8).
func (p *parser) contexts(t *Table, known func(string) bool) {
	seen := map[string]bool{}
	for _, name := range t.Contexts.Names {
		pkg := ContextPackage(name)
		switch {
		case !contextName.MatchString(name) || name == "api":
			p.bad = append(p.bad, fmt.Sprintf("contexts: %q is no context name", name))
		case seen[name]:
			p.bad = append(p.bad, fmt.Sprintf("contexts: %s is listed twice", name))
		case !known(pkg):
			p.bad = append(p.bad, fmt.Sprintf("contexts: %s is in no layer and planned for none", pkg))
		}
		seen[name] = true
	}
	ctx := t.ContextPackages()
	for _, r := range t.Rows {
		if !slices.ContainsFunc(t.AllPackages(r), func(pk string) bool { return slices.Contains(ctx, pk) }) {
			continue
		}
		for _, m := range r.May {
			if slices.Contains(ctx, m) {
				p.bad = append(p.bad, fmt.Sprintf("layer %s holds a context and may import the context %s: contexts never import one another", r.Layer, m))
			}
		}
	}
}

// explorer holds the explorer table: each module once, each import a module
// of the table and not itself, and no cycle among them (arch.layers.1).
func (p *parser) explorer(t *Table) {
	names := map[string]bool{}
	for _, m := range t.Explorer.Modules {
		if names[m.Name] {
			p.bad = append(p.bad, fmt.Sprintf("explorer: module %s is listed twice", m.Name))
		}
		names[m.Name] = true
	}
	for _, m := range t.Explorer.Modules {
		for _, imp := range m.May {
			switch {
			case imp == m.Name:
				p.bad = append(p.bad, fmt.Sprintf("explorer: module %s may import itself", m.Name))
			case !names[imp]:
				p.bad = append(p.bad, fmt.Sprintf("explorer: module %s may import %s, which is no module", m.Name, imp))
			}
		}
	}
	if cycle := t.Explorer.cycle(); cycle != "" {
		p.bad = append(p.bad, "explorer: imports run in a circle: "+cycle)
	}
}

// cycle is a path of module names that leads back to its start, or empty.
func (e Explorer) cycle() string {
	may := map[string][]string{}
	for _, m := range e.Modules {
		may[m.Name] = m.May
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var walk func(name string, path []string) string
	walk = func(name string, path []string) string {
		switch state[name] {
		case visiting:
			return strings.Join(append(path, name), " -> ")
		case done:
			return ""
		}
		state[name] = visiting
		for _, next := range may[name] {
			if c := walk(next, append(path, name)); c != "" {
				return c
			}
		}
		state[name] = done
		return ""
	}
	for _, m := range e.Modules {
		if c := walk(m.Name, nil); c != "" {
			return c
		}
	}
	return ""
}

// contextName is what a context is called: one lower-case word.
var contextName = regexp.MustCompile("^[a-z]+$")

// ContextPackage is the package of the context name, relative to the module.
func ContextPackage(name string) string { return "internal/core/" + name }

// ContextPackages are the packages of the contexts, in the table's order.
func (t *Table) ContextPackages() []string {
	out := make([]string, len(t.Contexts.Names))
	for i, n := range t.Contexts.Names {
		out[i] = ContextPackage(n)
	}
	return out
}

// PlannedIn are the packages planned for layer, in the table's order.
func (t *Table) PlannedIn(layer string) []string {
	var out []string
	for _, pl := range t.Planned {
		if pl.Layer == layer {
			out = append(out, pl.Packages...)
		}
	}
	return out
}

// AllPackages are the packages of r that exist, then the ones planned for it.
func (t *Table) AllPackages(r Row) []string {
	return append(slices.Clone(r.Packages), t.PlannedIn(r.Layer)...)
}

// packages is every package of t by its layer; a package in two layers is
// a problem.
func (p *parser) packages(t *Table) map[string]string {
	seen := map[string]string{}
	for _, r := range t.Rows {
		for _, pat := range r.Patterns {
			p.pattern(t, r, pat, seen)
		}
		for _, pkg := range r.Packages {
			if prev, twice := seen[pkg]; twice {
				p.bad = append(p.bad, fmt.Sprintf("%s is in layer %s and in layer %s", pkg, prev, r.Layer))
			}
			seen[pkg] = r.Layer
		}
	}
	return seen
}

// heldByPattern is whether a pattern of some row holds pkg: it is a package
// below the pattern's folder, whether or not the package exists yet.
func (t *Table) heldByPattern(pkg string) bool {
	return slices.ContainsFunc(t.Rows, func(r Row) bool {
		return slices.ContainsFunc(r.Patterns, func(pat string) bool { return Under(pat, pkg) })
	})
}

// RowOf is the row holding pkg, a path relative to the module, or nil when
// no row holds it. A row that lists pkg holds it even where another row's
// pattern reaches it: the listed row carves its package out of the pattern.
func (t *Table) RowOf(pkg string) *Row {
	for i := range t.Rows {
		if slices.Contains(t.Rows[i].Packages, pkg) {
			return &t.Rows[i]
		}
	}
	for i := range t.Rows {
		if slices.ContainsFunc(t.Rows[i].Patterns, func(pat string) bool { return Under(pat, pkg) }) {
			return &t.Rows[i]
		}
	}
	return nil
}

// StandIns are the packages of the stand-in rows.
func (t *Table) StandIns() []string {
	var out []string
	for _, r := range t.Rows {
		if r.Kind == KindStandIn {
			out = append(out, r.Packages...)
		}
	}
	return out
}

// May is whether from may import to, both relative to the module: its own
// package, a package its row may import, and, from a test, a stand-in.
func (t *Table) May(from, to string, test bool) bool {
	r := t.RowOf(from)
	switch {
	case r == nil:
		return false
	case from == to, slices.Contains(r.May, to), slices.ContainsFunc(r.May, func(m string) bool { return Under(m, to) }):
		return true
	}
	return test && slices.Contains(t.StandIns(), to)
}

// Owner is the library entry whose path is the longest prefix of the import
// path imp, as a path: the entry itself or a folder above it.
func (t *Table) Owner(imp string) (External, bool) {
	var best External
	found := false
	for _, e := range t.External {
		if (imp == e.Path || strings.HasPrefix(imp, e.Path+"/")) && len(e.Path) > len(best.Path) {
			best, found = e, true
		}
	}
	return best, found
}

// Effect is whether imp is a package only an edge or an adapter imports.
func (t *Table) Effect(imp string) bool {
	for _, e := range t.Effects {
		if exact, ok := strings.CutSuffix(e, "$"); ok {
			if imp == exact {
				return true
			}
			continue
		}
		if imp == e || strings.HasPrefix(imp, e+"/") {
			return true
		}
	}
	return false
}

// PureMay is whether a non-test file of pkg, in a pure row, may import imp,
// a path outside the module: one of its row's standard packages, exactly,
// or a pure library pkg owns. Any import of a package outside a pure row is
// true here; the other rows hold it.
func (t *Table) PureMay(pkg, imp string) bool {
	r := t.RowOf(pkg)
	if r == nil || r.Kind != KindPure {
		return true
	}
	if slices.Contains(r.Std, imp) {
		return true
	}
	e, ok := t.Owner(imp)
	return ok && e.Pure && e.Owner == pkg
}

// plannedTable reads the planned: list.
func (p *parser) plannedTable(doc *ordered.Map) []Planned {
	var out []Planned
	for _, m := range ordered.Maps(doc.List("planned")) {
		p.keys(m, []string{"created", "layer", "packages"}, []string{"created", "layer", "packages"})
		out = append(out, Planned{Created: m.Str("created"), Layer: m.Str("layer"), Packages: ordered.Strings(m.List("packages"))})
		if !milestone.MatchString(m.Str("created")) {
			p.add(m.KeyLine("created"), "planned: %q is no milestone (M0a, M0b, M1 to M10)", m.Str("created"))
		}
	}
	return out
}

// contextsTable reads the contexts: table.
func (p *parser) contextsTable(doc *ordered.Map) Contexts {
	c := doc.Sub("contexts")
	if c == nil {
		return Contexts{}
	}
	p.keys(c, []string{"why", "names"}, []string{"why", "names"})
	return Contexts{Why: c.Str("why"), Names: ordered.Strings(c.List("names"))}
}

// explorerTable reads the explorer: table.
func (p *parser) explorerTable(doc *ordered.Map) Explorer {
	e := doc.Sub("explorer")
	if e == nil {
		return Explorer{}
	}
	p.keys(e, []string{"why", "modules"}, []string{"why", "modules"})
	out := Explorer{Why: e.Str("why")}
	for _, m := range ordered.Maps(e.List("modules")) {
		p.keys(m, []string{"name", "path", "may"}, []string{"name", "path"})
		out.Modules = append(out.Modules, Module{Name: m.Str("name"), Path: m.Str("path"), May: ordered.Strings(m.List("may"))})
	}
	return out
}

// isHost is whether pkg is in a host layer: a package a row holds, or one a
// milestone plans for a host row, so a port may name the adapter its
// milestone creates (arch.ports.10: the ports come before their adapters).
func (t *Table) isHost(pkg string) bool {
	if r := t.RowOf(pkg); r != nil && r.Kind == KindHost {
		return true
	}
	for _, r := range t.Rows {
		if r.Kind == KindHost && slices.Contains(t.PlannedIn(r.Layer), pkg) {
			return true
		}
	}
	return false
}

// sourceTable reads the source: table.
func (p *parser) sourceTable(doc *ordered.Map) Source {
	s := doc.Sub("source")
	if s == nil {
		return Source{}
	}
	keys := []string{"scope", "rule_id", "tools", "formats", "refused", "limit", "inits"}
	p.keys(s, keys, []string{"scope", "rule_id", "tools", "formats", "refused", "limit"})
	out := Source{Scope: ordered.Strings(s.List("scope")), RuleID: s.Str("rule_id"), Tools: ordered.Strings(s.List("tools")),
		Formats: ordered.Strings(s.List("formats")), Refused: ordered.Strings(s.List("refused"))}
	if _, err := regexp.Compile(out.RuleID); err != nil {
		p.add(s.KeyLine("rule_id"), "source: rule_id is no regular expression: %v", err)
	}
	if l := s.Sub("limit"); l != nil {
		p.keys(l, []string{"layer", "lines"}, []string{"layer", "lines"})
		out.LimitLayer = l.Str("layer")
		n, ok := l.Val("lines").(int64)
		if !ok || n < 1 {
			p.add(l.KeyLine("lines"), "source: limit lines is not a whole number above 0")
		}
		out.LimitLines = int(n)
	}
	n, ok := s.Val("inits").(int64)
	if !ok || n < 0 {
		p.add(s.KeyLine("inits"), "source: inits is not a whole number, 0 or more")
	}
	out.Inits = int(n)
	return out
}

// externals reads the external rows into t. A library has one row: a path
// written twice is refused, since Owner reads only the first and the lint
// would render a rule for each (supply.intake.1, gragas#918); a second owner
// is an also of the one row, with its why.
func (p *parser) externals(doc *ordered.Map, t *Table) {
	seen := map[string]bool{}
	for _, m := range ordered.Maps(doc.List("external")) {
		p.keys(m, []string{"path", "owner", "also", "within", "why", "pure", "replaces"}, []string{"path"})
		if path := m.Str("path"); seen[path] {
			p.add(m.Line, "external: %s is listed twice: a library has one owner row, and a second owner is an also of it with its why", path)
		} else {
			seen[path] = true
		}
		ext := External{Path: m.Str("path"), Owner: m.Str("owner"), Why: m.Str("why"), Pure: m.Val("pure") == true, Within: ordered.Strings(m.List("within")), Replaces: ordered.Strings(m.List("replaces"))}
		p.also(m, &ext)
		t.External = append(t.External, ext)
		if m.Str("owner") == "" && m.Str("why") == "" {
			p.add(m.Line, "%s: a library with no owner says why", m.Str("path"))
		}
	}
}

// also reads the other owners of a library: a list of maps, each an owner with
// an until or a why, neither blank. Any other shape is refused, not dropped.
func (p *parser) also(m *ordered.Map, ext *External) {
	v, present := m.Get("also")
	if !present {
		return
	}
	list, ok := v.([]any)
	if !ok {
		p.add(m.KeyLine("also"), "%s: also is a list of maps, each with an owner and an until or a why", m.Str("path"))
		return
	}
	for _, item := range list {
		a, ok := item.(*ordered.Map)
		if !ok {
			p.add(m.KeyLine("also"), "%s: an also entry is a map with an owner and an until or a why", m.Str("path"))
			continue
		}
		p.keys(a, []string{"owner", "until", "why", "files"}, []string{"owner"})
		al := Also{Owner: a.Str("owner"), Until: a.Str("until"), Why: a.Str("why"), Files: p.alsoFiles(a, m.Str("path"))}
		switch {
		case blankKey(a, "until") || blankKey(a, "why"):
			p.add(a.Line, "%s: also %s has a blank until or why", m.Str("path"), al.Owner)
		case (al.Until == "") == (al.Why == ""):
			p.add(a.Line, "%s: also %s names an until or a why, one of the two", m.Str("path"), al.Owner)
		case al.Until != "" && !taskID.MatchString(al.Until):
			p.add(a.KeyLine("until"), "%s: until %q is no task (M0a-19, M0b-02)", m.Str("path"), al.Until)
		}
		ext.Also = append(ext.Also, al)
	}
}

// alsoFiles reads the optional files of an also: a list of file names in the owner's folder, or
// "tests" for its test files. Any other shape is refused, not dropped.
func (p *parser) alsoFiles(a *ordered.Map, path string) []string {
	v, present := a.Get("files")
	if !present {
		return nil
	}
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		p.add(a.KeyLine("files"), "%s: the files of also %s are a list of file names or tests", path, a.Str("owner"))
		return nil
	}
	var out []string
	for _, item := range list {
		f, isText := item.(string)
		if !isText || (f != TestsOnly && (!strings.HasSuffix(f, ".go") || strings.ContainsAny(f, "/*?[]{}\\") || f == ".go")) || slices.Contains(out, f) {
			p.add(a.KeyLine("files"), "%s: the files of also %s hold %v, which is not tests, a file name ending in .go, or is named twice", path, a.Str("owner"), item)
			continue
		}
		out = append(out, f)
	}
	return out
}

// WideAlsos is a line for each also that covers its owner's whole folder, so
// that a width no row narrowed is seen when the table is rendered.
func (t *Table) WideAlsos() []string {
	var out []string
	for _, e := range t.External {
		for _, a := range e.Also {
			if len(a.Files) == 0 {
				out = append(out, "wide: "+a.Owner+" may import "+e.Path+" in every file of its folder; its also names no files")
			}
		}
	}
	return out
}

// blankKey is whether the map has the key and its text is empty or only spaces.
func blankKey(m *ordered.Map, key string) bool {
	v, ok := m.Get(key)
	if !ok {
		return false
	}
	s, isText := v.(string)
	return !isText || strings.TrimSpace(s) == ""
}

// checkExternal holds each library's owners to the packages that exist.
func (p *parser) checkExternal(t *Table, known func(string) bool) {
	for _, e := range t.External {
		for _, a := range e.Also {
			if !known(a.Owner) {
				p.bad = append(p.bad, fmt.Sprintf("%s has the second owner %s, which is in no layer", e.Path, a.Owner))
			}
		}
		if e.Owner != "" && !known(e.Owner) {
			p.bad = append(p.bad, fmt.Sprintf("%s is owned by %s, which is in no layer", e.Path, e.Owner))
		}
		for _, w := range e.Within {
			if !IsPattern(w) || !known(w) {
				p.bad = append(p.bad, fmt.Sprintf("%s may be imported within %s, which is no pattern of a layer", e.Path, w))
			}
		}
	}
}

// Wildcard ends a pattern: folder/... is every package below the folder.
const Wildcard = "/..."

// IsPattern is whether name is a pattern, folder/..., and not a package.
func IsPattern(name string) bool { return strings.HasSuffix(name, Wildcard) }

// Folder is the folder a pattern names: internal/cli/cmd for internal/cli/cmd/...
func Folder(pattern string) string { return strings.TrimSuffix(pattern, Wildcard) }

// Under is whether pkg is a package below the folder of pattern, at any depth.
// The folder itself is not below it, and a name that is no pattern holds nothing.
func Under(pattern, pkg string) bool {
	return IsPattern(pattern) && strings.HasPrefix(pkg, Folder(pattern)+"/")
}

// pattern holds one pattern of a row to the table: it names a folder, no other
// pattern holds it or a folder inside it. A package a row lists inside the
// folder is held by that row only when the pattern's row carves it out by name.
// The pattern joins seen so a row's May may name it.
func (p *parser) pattern(t *Table, r Row, pat string, seen map[string]string) {
	if !IsPattern(pat) || Folder(pat) == "" || strings.Contains(Folder(pat), "...") {
		p.bad = append(p.bad, fmt.Sprintf("layer %s: pattern %q is not of the shape folder/...", r.Layer, pat))
		return
	}
	if prev, twice := seen[pat]; twice {
		p.bad = append(p.bad, fmt.Sprintf("pattern %s is in layer %s and in layer %s", pat, prev, r.Layer))
	}
	seen[pat] = r.Layer
	for _, o := range t.Rows {
		for _, other := range o.Patterns {
			if other != pat && (Under(pat, Folder(other)) || Under(other, Folder(pat))) {
				p.bad = append(p.bad, fmt.Sprintf("pattern %s (layer %s) and pattern %s (layer %s) hold the same packages", pat, r.Layer, other, o.Layer))
			}
		}
		for _, pkg := range o.Packages {
			if Under(pat, pkg) && !slices.Contains(r.Carves, pkg) {
				p.bad = append(p.bad, fmt.Sprintf("%s is in layer %s, and pattern %s of layer %s holds it", pkg, o.Layer, pat, r.Layer))
			}
		}
	}
}

// AlsoOwner is whether pkg is one of the other owners of the library.
func (e External) AlsoOwner(pkg string) bool {
	return slices.ContainsFunc(e.Also, func(a Also) bool { return a.Owner == pkg })
}

// WithinPattern is whether pkg is a package below one of the patterns the
// library may be imported within.
func (e External) WithinPattern(pkg string) bool {
	return slices.ContainsFunc(e.Within, func(pat string) bool { return Under(pat, pkg) })
}

// carves holds each package a row carves out to a package some row lists that
// one of the row's own patterns holds.
func (p *parser) carves(t *Table) {
	for _, r := range t.Rows {
		for _, c := range r.Carves {
			switch {
			case !slices.ContainsFunc(t.Rows, func(o Row) bool { return slices.Contains(o.Packages, c) }):
				p.bad = append(p.bad, fmt.Sprintf("layer %s carves %s, which no layer lists", r.Layer, c))
			case !slices.ContainsFunc(r.Patterns, func(pat string) bool { return Under(pat, c) }):
				p.bad = append(p.bad, fmt.Sprintf("layer %s carves %s, which none of its patterns holds", r.Layer, c))
			}
		}
	}
}
