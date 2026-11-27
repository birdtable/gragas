// Package census is the census of gragas#1719 D (gragas#1750): every
// identifier of the standard library that differs by system, read from the
// files the go tool builds for each first-class port, and the decision
// gate/os-api.txt records for each.
//
// The ports are data: the ones go tool dist list -json marks first class.
// For each, go list -e std names the files it builds, with cgo off, as the
// shipped binary is built. No system is named here and nothing reads
// go/build: a port is a GOOS and a GOARCH the go tool gave, and a file is
// per-system when some port does not build it. An identifier differs when
// one of the three sources the census used says so:
//
//   - the API: it is not declared for every port, its declaration's text
//     differs between them, or it is declared in a per-system file;
//   - reach: its body or its value, in a file every port builds, names a
//     func, var, const or method that differs, of its own package or of
//     another std package, internal and vendored ones included, to a fixed
//     point. A method is followed by
//     its name, as .<name>, across the package's types; a selector off a
//     value of another package's type is not followed, and neither are
//     runtime's unexported names and methods (the ruling of 11:01Z);
//   - syscall reach: its declaration names the file's import of syscall;
//   - the doc text: a sentence of its doc comment names a GOOS any port of
//     the list has, or says the behaviour depends on the system.
//
// Hold settles by allows: an allow line on a name the reach passes through
// stops the reach there, so an identifier that differs only through allowed
// names needs no line of its own. A ban or a later stops nothing: it says
// the difference is wrong, so each name that reaches it takes its own line,
// and a root, which no lint row can refuse, takes only allow.
//
// It reaches no disk and starts no program: the command and the test hand in
// the go tool and the GOROOT's files.
package census

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Port is one port of the go tool: a GOOS and a GOARCH, as go tool dist
// list writes them.
type Port struct {
	GOOS   string
	GOARCH string
}

// String is the port as the go tool writes it.
func (p Port) String() string { return p.GOOS + "/" + p.GOARCH }

// Go runs the go tool with args, its environment the caller's with env
// added, and returns what it printed on stdout.
type Go func(ctx context.Context, env []string, args ...string) ([]byte, error)

// Census is what Take read.
type Census struct {
	// Ports are the first-class ports, in the order the go tool listed
	// them.
	Ports []Port
	// Packages is how many packages of std were read.
	Packages int
	// Read is how many exported identifiers were read, every port's
	// counted once.
	Read int
	// Differ are the identifiers that differ by system, in name order.
	Differ []Ident
	// Terms are the words a doc sentence is held to: each GOOS of the
	// dist list, and the phrases of docPhrases.
	Terms []string

	// w, order and found are what was read and what differs, which settled
	// reaches again with the decisions; a census built by hand holds none.
	w     world
	order []string
	found marks
}

// Ident is one identifier that differs by system: its name, as
// <import path>.<name> or <import path>.<type>.<method or field>, why, each
// reason once, in order, the doc sentence that matched when WhyDoc is one,
// and the first name it reaches that differs when WhyReach is one.
type Ident struct {
	Name string
	Why  []string
	Doc  string
	Via  string
}

// The reasons an identifier differs.
const (
	WhyPartial = "not declared for every port"
	WhyDefined = "declared differently between ports"
	WhyFile    = "declared in a per-system file"
	WhyReach   = "reaches what differs"
	WhySyscall = "reaches syscall"
	WhyDoc     = "its doc names a system"
)

// ErrNoPort is a dist list that marks no port first class, and ErrNoIdent a
// census that read no identifier: neither is a pass (gates.plants.5).
var (
	ErrNoPort  = errors.New("the go tool marks no port first class")
	ErrNoIdent = errors.New("the census read no identifier")
)

// DistArgs is the go tool's command line that lists the ports.
func DistArgs() []string { return []string{"tool", "dist", "list", "-json"} }

// listFormat is go list's line for a package: its import path, then each
// file it builds, split by spaces (no std file name holds one).
const listFormat = "{{.ImportPath}}{{range .GoFiles}} {{.}}{{end}}"

// ListArgs is the go tool's command line that lists std's packages and the
// files each builds. -e lists a package whose files the port builds none
// of, with no file, rather than stop.
func ListArgs() []string { return []string{"list", "-e", "-f", listFormat, "std"} }

// ListEnv is the environment ListArgs runs in for a port: cgo off, as the
// shipped binary is built, and no GOFLAGS or workspace of the caller's.
func ListEnv(p Port) []string {
	return []string{"GOOS=" + p.GOOS, "GOARCH=" + p.GOARCH, "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off"}
}

// FirstClass reads go tool dist list -json: the ports the census compares,
// in its order, and every GOOS it lists, sorted. The census asks what
// differs by system, not by architecture, so it compares the first-class
// GOOS values on each GOARCH that every one of them has a first-class port
// for: a port of an architecture some first-class GOOS lacks would set
// apart what only the architecture changes.
func FirstClass(data []byte) (ports []Port, systems []string, err error) {
	var rows []struct {
		GOOS, GOARCH string
		FirstClass   bool
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, nil, fmt.Errorf("the dist list does not read: %w", err)
	}
	var first []Port
	for _, r := range rows {
		if r.GOOS == "" || r.GOARCH == "" {
			return nil, nil, fmt.Errorf("the dist list has a port with no GOOS or GOARCH: %+v", r)
		}
		systems = append(systems, r.GOOS)
		if r.FirstClass {
			first = append(first, Port{r.GOOS, r.GOARCH})
		}
	}
	if ports = shared(first); len(ports) == 0 {
		return nil, nil, ErrNoPort
	}
	slices.Sort(systems)
	return ports, slices.Compact(systems), nil
}

// shared are the ports of first whose GOARCH every GOOS of first has a port
// for, in first's order.
func shared(first []Port) []Port {
	arches := map[string]map[string]bool{} // GOARCH -> the GOOS values it has
	goos := map[string]bool{}
	for _, p := range first {
		if arches[p.GOARCH] == nil {
			arches[p.GOARCH] = map[string]bool{}
		}
		arches[p.GOARCH][p.GOOS] = true
		goos[p.GOOS] = true
	}
	return slices.DeleteFunc(slices.Clone(first), func(p Port) bool { return len(arches[p.GOARCH]) != len(goos) })
}

// Packages reads go list's output in listFormat: each package's files, by
// import path, internal and vendored ones too, which the reach goes through
// though no program imports them; builtin, which only documents, is left
// out. A package listed with no file is there, holding none.
func Packages(out []byte) (map[string][]string, error) {
	pkgs := map[string][]string{}
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if _, dup := pkgs[f[0]]; dup {
			return nil, fmt.Errorf("go list names %s twice", f[0])
		}
		if f[0] == "builtin" {
			continue
		}
		pkgs[f[0]] = f[1:]
	}
	if len(pkgs) == 0 {
		return nil, errors.New("go list named no package of std")
	}
	return pkgs, nil
}

// Importable is whether a program outside std may import the package at
// path: none under internal or vendor, and not builtin, which only
// documents.
func Importable(path string) bool {
	if path == "builtin" || strings.HasPrefix(path, "vendor/") {
		return false
	}
	return !slices.Contains(strings.Split(path, "/"), "internal")
}

// Take runs the census: the first-class ports from the dist list, the files
// each builds from go list, and each file from src, the GOROOT, at
// src/<import path>/<file>.
func Take(ctx context.Context, goTool Go, src fs.FS) (*Census, error) {
	dist, err := goTool(ctx, nil, DistArgs()...)
	if err != nil {
		return nil, fmt.Errorf("go tool dist list: %w", err)
	}
	ports, systems, err := FirstClass(dist)
	if err != nil {
		return nil, err
	}
	built := make([]map[string][]string, len(ports))
	for i, p := range ports {
		out, err := goTool(ctx, ListEnv(p), ListArgs()...)
		if err != nil {
			return nil, fmt.Errorf("go list std for %s: %w", p, err)
		}
		if built[i], err = Packages(out); err != nil {
			return nil, fmt.Errorf("go list std for %s: %w", p, err)
		}
	}
	return Of(ports, systems, built, src)
}

// Of is the census of the files each port builds: built[i] is ports[i]'s,
// each package's files by import path; systems are the GOOS values a doc
// sentence is held to.
func Of(ports []Port, systems []string, built []map[string][]string, src fs.FS) (*Census, error) {
	if len(ports) == 0 {
		return nil, ErrNoPort
	}
	if len(built) != len(ports) {
		return nil, fmt.Errorf("%d ports and %d listings", len(ports), len(built))
	}
	c := &Census{Ports: ports, Terms: terms(systems)}
	r := newReader(src, docMatcher(c.Terms))
	order := packagePaths(built)
	w := world{}
	for _, path := range order {
		files := make([][]string, len(ports))
		for i := range ports {
			files[i] = built[i][path]
		}
		d, err := r.pkg(path, files)
		if err != nil {
			return nil, err
		}
		w[path] = d
	}
	c.w, c.order = w, order
	c.found = w.reach(order, runtimeHelper)
	for _, path := range order {
		if !Importable(path) {
			continue
		}
		idents, read := w.report(w[path], c.found)
		c.Packages++
		c.Read += read
		c.Differ = append(c.Differ, idents...)
	}
	if c.Read == 0 {
		return nil, ErrNoIdent
	}
	slices.SortFunc(c.Differ, func(a, b Ident) int { return strings.Compare(a.Name, b.Name) })
	return c, nil
}

// packagePaths are the import paths any port lists, sorted.
func packagePaths(built []map[string][]string) []string {
	var out []string
	for _, b := range built {
		for p := range b {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TSV is the census as the command prints it: a line for each identifier,
// its name, its reasons, the doc sentence that matched and the name it
// reaches, split by tabs.
func (c *Census) TSV() string {
	var b strings.Builder
	for _, id := range c.Differ {
		b.WriteString(id.Name + "\t" + strings.Join(id.Why, "; ") + "\t" + id.Doc + "\t" + id.Via + "\n")
	}
	return b.String()
}
