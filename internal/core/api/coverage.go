package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// CoverageSchema is the schema number of CoverageDoc, the document the
// coverage command answers with under --json.
const CoverageSchema = 1

// upstreamSuffix ends the name of a flattened set's file.
const upstreamSuffix = ".flat.json"

// ListCoverage is the coverage of the rulebook at root, both ways: the rules
// an effective binding holds, and the entries the rules explain and the
// upstream sets leave unexplained (link.Coverage). It reads the flattened set
// of each tool from the rulebook's upstream/ folder as data; a tool with no
// file has no upstream set, which is no error and counts zero unmapped, and a
// file in a newer version is *contract.UpstreamNotReady. It writes nothing. The
// proof grid is not read yet, so no binding is
// proven here and standing comes from the rung-6 pairs alone; the grid is the
// one argument to wire in. A folder with no rulebook is ErrNoBook, one with
// findings is FindingsError, and an open run and a newer format are as for
// the other listings.
func ListCoverage(ctx context.Context, p port.Ports, root model.Root) (model.Coverage, error) {
	if err := ctx.Err(); err != nil {
		return model.Coverage{}, err
	}
	in, err := readBook(ctx, p, root)
	if err != nil {
		return model.Coverage{}, err
	}
	if !in.hasBook {
		return model.Coverage{}, ErrNoBook
	}
	if n := len(in.rep.Findings); n > 0 {
		return model.Coverage{}, &FindingsError{Count: n}
	}
	held, err := link.Derive(in.rep.Book, model.Grid{}, model.Chain{})
	if err != nil {
		return model.Coverage{}, err
	}
	up, err := readUpstream(p.Tree, root, in.rep.Dir)
	if err != nil {
		return model.Coverage{}, err
	}
	c := link.Coverage(in.rep.Book, held, up)
	c.Dir = in.rep.Dir
	return c, nil
}

// readUpstream reads each flattened set below the rulebook's folder, by tool
// name. No upstream folder is no sets.
func readUpstream(tree port.Tree, root model.Root, dir string) (map[string]model.Upstream, error) {
	entries, err := tree.List(root, path.Join(dir, "upstream"), model.Every)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]model.Upstream{}
	folder := path.Join(dir, "upstream")
	for _, e := range entries {
		name := path.Base(e.Path)
		if e.Dir || path.Dir(e.Path) != folder || !strings.HasSuffix(name, upstreamSuffix) {
			continue // only the files at upstream/<tool>.flat.json, never one in a folder below
		}
		if e.Size > contract.MaxUpstream {
			return nil, &UpstreamRefused{File: e.Path, Err: fmt.Errorf("the file is %d bytes, over the %d a flattened set may hold", e.Size, contract.MaxUpstream)}
		}
		data, err := tree.Read(root, e.Path)
		if err != nil {
			return nil, err
		}
		tool := strings.TrimSuffix(name, upstreamSuffix)
		u, err := contract.ReadUpstream(tool, data)
		if err != nil {
			return nil, &UpstreamRefused{File: e.Path, Err: err}
		}
		out[tool] = u
	}
	return out, nil
}

// UpstreamRefused is a flattened set file the reader refused, whole.
type UpstreamRefused struct {
	File string
	Err  error
}

func (e *UpstreamRefused) Error() string { return e.File + ": " + e.Err.Error() }
func (e *UpstreamRefused) Unwrap() error { return e.Err }

// CoverageDoc is the coverage as the document under --json. Its coverage
// member holds the two lists the contract's bundle names.
type CoverageDoc struct {
	Schema   int    `json:"schema"`
	Contract string `json:"contract"`
	Command  string `json:"command"`
	// Word is the closing word: always done: an unmapped entry is a line of the listing.
	Word string `json:"word"`
	// Dir is the folder of the rulebook below the root, "" for the root.
	Dir string `json:"dir"`
	// RulesHeld is how many rules an effective binding holds.
	RulesHeld int `json:"rules_held"`
	// EntriesExplained is how many units the rules explain, an adopted
	// upstream set counted as one.
	EntriesExplained int                   `json:"entries_explained"`
	Coverage         CoverageLists         `json:"coverage"`
	Rules            []model.CoverRule     `json:"rules"`
	Entries          []model.CoverEntry    `json:"entries"`
	Unmapped         []model.CoverUnmapped `json:"unmapped"`
}

// CoverageLists are the contract's coverage object: the rules a binding holds
// and the entries a rule explains.
type CoverageLists struct {
	RulesToBindings []string `json:"rules_to_bindings"`
	BindingsToRules []string `json:"bindings_to_rules"`
}

// CoverageDocument is c as the document.
func CoverageDocument(c model.Coverage) CoverageDoc {
	lists := CoverageLists{RulesToBindings: []string{}, BindingsToRules: []string{}}
	for _, r := range c.Rules {
		if r.Held {
			lists.RulesToBindings = append(lists.RulesToBindings, string(r.Rule))
		}
	}
	for _, e := range c.Entries {
		lists.BindingsToRules = append(lists.BindingsToRules, e.Tool+" "+e.Entry)
	}
	slices.Sort(lists.RulesToBindings)
	return CoverageDoc{Schema: CoverageSchema, Contract: model.ContractVersion, Command: "coverage", Word: string(model.Done), Dir: c.Dir,
		RulesHeld: c.RulesHeld(), EntriesExplained: len(c.Entries), Coverage: lists,
		Rules: append([]model.CoverRule{}, c.Rules...), Entries: append([]model.CoverEntry{}, c.Entries...), Unmapped: append([]model.CoverUnmapped{}, c.Unmapped...)}
}

// coverageCheckText is the sentence SARIF shows beside model.CoverageCheck.
const coverageCheckText = "The coverage check: an entry the upstream config holds that no rule explains (not a rulebook rule)"

// CoverageSARIF is c's unmapped entries as one SARIF document of the contract
// (cli.output.2), one result each, in the file of the flattened set that
// holds it. tag is the engine's release tag, as for ValidationSARIF.
func CoverageSARIF(c model.Coverage, tag string) ([]byte, error) {
	version := tag
	if len(tag) > 1 && tag[0] == 'v' {
		version = tag[1:]
	}
	tool := contract.Tool{Name: "gragas", Version: version, Checks: []contract.Check{{ID: model.CoverageCheck, Text: coverageCheckText}}}
	results := make([]contract.Result, 0, len(c.Unmapped))
	for _, u := range c.Unmapped {
		results = append(results, contract.Result{Check: model.CoverageCheck, File: path.Join(c.Dir, contract.UpstreamFile(u.Tool)),
			Msg: u.Tool + ": the entry " + u.Entry + " is held by the upstream config and no rule explains it", Fix: "bind a rule to it with gragas bind"})
	}
	return contract.EncodeFindings(tool, results)
}

// UpstreamNotReady is a flattened set in a version newer than the engine reads,
// as the command package names it.
type UpstreamNotReady = contract.UpstreamNotReady
