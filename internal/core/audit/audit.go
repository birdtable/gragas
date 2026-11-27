// Package audit is the project-facing read of gragas check: it reads the
// tool configs a repository owns and reports each applicable rule whose
// binding the config does not implement. It never runs a tool and never
// writes a file; what needs a tool's own reading of its config, the entries
// it holds, arrives through the Entries seam as the tool's plugin answers it,
// and what needs a TOML parser arrives as a TOML
// function, both from the layer above.
//
// A tool joins the check by a row of the readers data (Source: the tool, the
// files its config may be in, and the kind of read) and, only when its config
// is of a new kind, one file here for that kind. No tool is named in this
// package: the tools and their files are data the layer above hands in.
package audit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model/plugin"
)

// Binding is one rule's binding to a tool key, as rulebook/bindings.json says
// it: the rule, the tool and the key the tool's config must carry.
type Binding struct {
	Rule, Tool, Key string
}

// Finding is one rule the repository's config does not implement.
type Finding struct {
	Rule    string `json:"rule"`
	Tool    string `json:"tool"`
	Key     string `json:"key"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Missing string `json:"missing"`
}

// String is the finding as a person reads it, in the shape of the rulebook's
// findings: "file: rule (tool key): what is missing".
func (f Finding) String() string {
	return f.where() + ": " + f.Rule + " (" + f.Tool + " " + f.Key + "): " + f.Missing
}

// where is the file, and the line after a colon when the finding has one.
func (f Finding) where() string {
	if f.Line > 0 {
		return f.File + ":" + strconv.Itoa(f.Line)
	}
	return f.File
}

// Exempt is a finding that a row of gate/exemptions.yaml waives until Until.
type Exempt struct {
	Finding
	Until string `json:"until"`
}

// String is the waived finding with its expiry.
func (e Exempt) String() string { return e.Finding.String() + " (exempt until " + e.Until + ")" }

// NotChecked says a tool's rules were not judged, and why. It is not a
// finding and not a pass.
type NotChecked struct {
	Tool string `json:"tool"`
	Why  string `json:"why"`
	// Partial says the tool was judged and one part of it was not.
	Partial bool `json:"partial,omitempty"`
}

// String is the tool and the reason.
func (n NotChecked) String() string {
	if n.Partial {
		return n.Tool + ": " + n.Why
	}
	return n.Tool + " rules were not checked: " + n.Why
}

// Result is what the check found, in the order of the bindings. Halt is
// each reason the check cannot vouch for a tool it was to ask, which ends
// the check red and is never "not checked": today a run held to a memory
// limit this system neither holds nor reads (port.ErrNotHeld).
// It is nil when none was met.
type Result struct {
	Findings   []Finding    `json:"findings"`
	Exempt     []Exempt     `json:"exempt"`
	NotChecked []NotChecked `json:"not_checked"`
	Halt       error        `json:"-"`
}

// EntriesCall asks a tool's plugin for the entries of a config: the plan
// rounds and the one entries call, in the shape of link's
// EntriesCall, which this context does not import (arch.ports.8). An error
// means the answer could not be had, and the tool's rules are then not
// checked, unless it matches port.ErrNotHeld: that is the check's Halt.
type EntriesCall func(ctx context.Context, in plugin.PlanRequest) (plugin.EntriesResponse, error)

// Entries is the seam to a tool's plugin: the entries call of the tool, the
// tool version its plan is asked for, and the reason there is none, "" when
// there is one. The operations wire it (internal/core/api); the tests give
// the stand-in.
type Entries func(tool string) (call EntriesCall, toolVersion, why string)

// TOML reads a TOML document into maps, lists and scalars. The core links no
// TOML library: the one that reads .gitleaks.toml sits in an adapter.
type TOML func(data []byte) (map[string]any, error)

// The kinds of read a Source names.
const (
	// KindEntries reads a key as an entry of the tool's config that must be
	// on, the entries being the tool's plugin's answer.
	KindEntries = "entries"
	// KindTOMLKeys reads a key of a TOML document.
	KindTOMLKeys = "toml-keys"
	// KindTaskNames reads a key as a task the file must define.
	KindTaskNames = "task-names"
	// KindYAMLKeys reads a key of a release tool's YAML config; with no file
	// there is nothing to hold, and no key is a finding.
	KindYAMLKeys = "yaml-keys"
	// KindOSFiles reads the repository's Go files for those that name an
	// operating system outside the host layer, the folders the config's row
	// at the Source's Host key spares (arch.ports.24); its one key is its own
	// name, and its exemption rows are read by path, never by rule.
	KindOSFiles = "os-files"
)

// Source says where one tool's config is and how it is read: Files are the
// names it may have at the repository root, the first that exists is read,
// and Kind is one of the kinds above. Host is, for the os-files kind, the key
// of the config whose list of files spares the host layer.
type Source struct {
	Tool  string
	Files []string
	Kind  string
	Host  string
}

// Input is what one run reads. Sources are the tools the check reads, in the
// order they are reported. Bindings are those of the applicable rules only;
// Exempt says whether a rule has a live exemption row, and until when;
// ExemptAt whether a live row of the rule names the file at a slash path, by
// the file or a folder of it. A nil Entries or TOML means that kind's rules
// are not checked.
type Input struct {
	Repo     fs.FS
	Sources  []Source
	Bindings []Binding
	Exempt   func(rule string) (until string, ok bool)
	ExemptAt func(rule, file string) (until string, ok bool)
	Entries  Entries
	TOML     TOML
}

// verdict is what a reader says of one binding: "" holds, else what is
// missing.
type verdict func(b Binding) string

// config is one tool's config as found: the Source, the file's name (the first
// of Files when none is there), its content and whether it is there.
type config struct {
	Source
	File    string
	Data    []byte
	Present bool
	// Unread is the reason a file that is there could not be read (a FIFO, a
	// folder, a device): the tool's rules are then not checked, not reported
	// as missing.
	Unread string
}

// reader reads a config once and returns a verdict per binding, the reason
// the tool's rules cannot be judged, or the halt that ends the check red
// (Result.Halt).
type reader func(ctx context.Context, in Input, c config) (verdict, string, error)

// readerOf is the reader of a kind of Source.
func readerOf(kind string) (reader, bool) {
	switch kind {
	case KindEntries:
		return readEntries, true
	case KindTOMLKeys:
		return neverHalts(readTOMLKeys), true
	case KindTaskNames:
		return neverHalts(readTaskNames), true
	case KindYAMLKeys:
		return neverHalts(readYAMLKeys), true
	case KindOSFiles:
		return neverHalts(readOSFiles), true
	}
	return nil, false
}

// neverHalts is a reader that runs no tool, so nothing it meets ends the
// check red.
func neverHalts(read func(ctx context.Context, in Input, c config) (verdict, string)) reader {
	return func(ctx context.Context, in Input, c config) (verdict, string, error) {
		judge, why := read(ctx, in, c)
		return judge, why, nil
	}
}

// Check judges each binding against the config its tool reads.
func Check(ctx context.Context, in Input) Result {
	res := Result{Findings: []Finding{}, Exempt: []Exempt{}, NotChecked: []NotChecked{}}
	for _, src := range in.Sources {
		var bs []Binding
		for _, b := range in.Bindings {
			if b.Tool == src.Tool {
				bs = append(bs, b)
			}
		}
		if len(bs) > 0 {
			judgeSource(ctx, in, src, bs, &res)
		}
	}
	return res
}

// judgeSource reads one tool's config and adds what it says of the tool's
// bindings to res.
func judgeSource(ctx context.Context, in Input, src Source, bs []Binding, res *Result) {
	read, ok := readerOf(src.Kind)
	if !ok {
		res.NotChecked = append(res.NotChecked, NotChecked{Tool: src.Tool, Why: "no reader is known for the kind " + src.Kind})
		return
	}
	c := find(in.Repo, src)
	if c.Unread != "" {
		res.NotChecked = append(res.NotChecked, NotChecked{Tool: src.Tool, Why: c.Unread})
		return
	}
	judge, why, halt := read(ctx, in, c)
	if halt != nil {
		res.Halt = errors.Join(res.Halt, halt)
		return
	}
	if why != "" {
		res.NotChecked = append(res.NotChecked, NotChecked{Tool: src.Tool, Why: why})
		return
	}
	slices.SortStableFunc(bs, func(x, y Binding) int { return strings.Compare(x.Rule, y.Rule) })
	for _, b := range bs {
		missing := judge(b)
		if missing == "" {
			continue
		}
		f := Finding{Rule: b.Rule, Tool: b.Tool, Key: b.Key, File: c.File, Missing: missing}
		// An os-files finding is files no row names: a row of its rule for
		// another file waives none of them.
		if until, ok := lookup(in, b.Rule); ok && src.Kind != KindOSFiles {
			res.Exempt = append(res.Exempt, Exempt{f, until})
			continue
		}
		res.Findings = append(res.Findings, f)
	}
}

func lookup(in Input, rule string) (string, bool) {
	if in.Exempt == nil {
		return "", false
	}
	return in.Exempt(rule)
}

// find reads the tool's config from the repository root. The file is the first
// of its names that is there, or the first name when none is.
func find(repo fs.FS, src Source) config {
	listed := names(repo)
	for _, n := range src.Files {
		if listed != nil && !listed[n] {
			continue
		}
		if b, err := fs.ReadFile(repo, n); err == nil {
			return config{Source: src, File: n, Data: b, Present: true}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return config{Source: src, File: n, Unread: fmt.Sprintf("%s is there and cannot be read inside the repository (%v), so what it allows is not checked", n, err)}
		}
	}
	name := ""
	if len(src.Files) > 0 {
		name = src.Files[0]
	}
	return config{Source: src, File: name}
}

// names is the entries of the repository's root by their exact names, nil when
// the root cannot be listed. A file is opened only when its name is there as
// written, so a host that folds case reads the same file as one that keeps it.
func names(repo fs.FS) map[string]bool {
	es, err := fs.ReadDir(repo, ".")
	if err != nil {
		return nil
	}
	m := make(map[string]bool, len(es))
	for _, e := range es {
		m[e.Name()] = true
	}
	return m
}
