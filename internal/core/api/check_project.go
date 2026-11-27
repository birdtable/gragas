package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ProjectAsk is what the layer above gives check in a project: the tables and
// the program's answers the read needs, the clock, and the names list's path.
// Nothing in it is read from the repository.
type ProjectAsk struct {
	Readers Readers
	Tiers   TierTable
	// Plugins is the ports the tools' installed plugins are read through; its
	// zero value reads no tool (ErrNoPlugin).
	Plugins AuditPlugins
	TOML    TOML
	// Today is the date, YYYY-MM-DD; nil gives no date.
	Today func() string
	// NamesFile is the private names list's path; "" when none is named.
	NamesFile string
	// OpenHistory opens the repository's history for the names, tracked-files
	// and counts checks, nil in a build with no git; BaseRef is the ref whose
	// merge base with HEAD the counts are taken against, "" for DefaultBase.
	OpenHistory func() (port.History, error)
	BaseRef     string
}

// CheckProjectAt finds the project the working folder start is in, with the
// pin at pinFile, and checks it (project.Load, then CheckProject). A project
// that is not there to load (no repository, a source not there), a pin that is
// refused and a pin newer than this gragas reads are each ErrNotReady, in the
// project's own words: the caller ends the run on not ready.
func CheckProjectAt(ctx context.Context, p port.Ports, start, pinFile string, ask ProjectAsk) (model.ProjectCheck, error) {
	l, err := project.Load(p.Tree, start, pinFile)
	if err != nil {
		return model.ProjectCheck{}, projectNotReady(err)
	}
	return checkProject(ctx, p, l, ask)
}

// projectNotReady is err as ErrNotReady, with the words of err, when err is
// one project.Load answers for a project it cannot go on from: a
// *project.NotReady, a *project.PinError or a *project.PinNotReady. Any other
// error is returned as it is.
func projectNotReady(err error) error {
	var nr *project.NotReady
	var pe *project.PinError
	var pn *project.PinNotReady
	if errors.As(err, &nr) || errors.As(err, &pe) || errors.As(err, &pn) {
		return &loadNotReady{err}
	}
	return err
}

// loadNotReady says what the project says and is ErrNotReady.
type loadNotReady struct{ err error }

func (e *loadNotReady) Error() string   { return e.err.Error() }
func (e *loadNotReady) Unwrap() []error { return []error{ErrNotReady, e.err} }

// checkProject holds the project l was loaded from (project.Load) to the
// bindings of the rulebook its pin names. The parts are the
// pin's, and a part the book lacks, or a book with no rule, is ErrNotReady:
// nothing can be judged against it. The bindings are the rung 0 (tool, key)
// ones of the active and trial rules of those parts; the project's tool
// configs are judged against them, then the tier contract and the exemptions,
// then the names, the tracked files and the counts (repoChecks).
// It writes no file, and runs no tool but the git behind the ask's History. A
// tool the check was to ask and could not vouch for ends the run in the error
// it is (the run's Halt).
//
// It reads nothing of the old engine: not the old Loaded, not .gragas/parts.
func checkProject(ctx context.Context, p port.Ports, l project.Loaded, ask ProjectAsk) (model.ProjectCheck, error) {
	rep, err := LoadBookNamed(ctx, p, l.Rulebook, ask.NamesFile)
	if err != nil {
		return model.ProjectCheck{}, err
	}
	if len(rep.Findings) > 0 {
		return model.ProjectCheck{}, fmt.Errorf("%w: the rulebook the pin names has %d finding(s), the first: %s: %s; run gragas check in it", ErrNotReady, len(rep.Findings), rep.Findings[0].Entity, rep.Findings[0].Msg)
	}
	if len(rep.Book.Rules) == 0 {
		return model.ProjectCheck{}, fmt.Errorf("%w: the rulebook the pin names holds no rule", ErrNotReady)
	}
	if lacking := partsLacking(rep.Book, l.Pin.Parts); len(lacking) > 0 {
		return model.ProjectCheck{}, fmt.Errorf("%w: %s names %s, which the rulebook does not have", ErrNotReady, l.PinPath, strings.Join(lacking, ", "))
	}
	repo, err := p.Tree.FS(l.Root)
	if err != nil {
		return model.ProjectCheck{}, fmt.Errorf("open the project to read: %w", err)
	}
	date := ""
	if ask.Today != nil {
		date = ask.Today()
	}
	bs := projectBindings(rep.Book, l.Pin.Parts)
	out := model.ProjectCheck{Findings: []model.GateFinding{}, Exempt: []string{}, NotChecked: []model.NotChecked{}}
	var halt error

	tool := audit.Result{}
	sources := sourcesOf(ask.Readers.Rows)
	switch {
	case ask.Readers.Refused != "":
		out.NotChecked = append(out.NotChecked, unjudgedTools(bs, nil, "the tool configs were not checked: "+ask.Readers.Refused)...)
	default:
		tool = audit.Check(ctx, audit.Input{
			Repo: repo, Sources: sources, Bindings: bs, Exempt: liveRows(repo, date),
			ExemptAt: liveRowsAt(repo, date), Entries: ask.Plugins.entries(rep.Book.Identity.Tested), TOML: audit.TOML(ask.TOML),
		})
		halt = tool.Halt
		out.NotChecked = append(out.NotChecked, unjudgedTools(bs, sources, "no reader is held for this tool, so its rules were not checked")...)
	}
	if len(bs) == 0 {
		out.NotChecked = append(out.NotChecked, model.NotChecked{Msg: "no rule of the pinned parts binds a tool, so no tool config was judged"})
	}
	for _, f := range tool.Findings {
		cf := ConfigFinding(f)
		out.Findings = append(out.Findings, gateFinding(model.GateToolConfig, cf.where(), cf.File, cf.Line, f.Rule+" ("+f.Tool+" "+f.Key+"): "+f.Missing, "set "+f.Key+" in "+f.File+" as the rule says"))
	}
	for _, e := range tool.Exempt {
		out.Exempt = append(out.Exempt, ConfigExempt{ConfigFinding(e.Finding), e.Until}.String())
	}
	for _, n := range tool.NotChecked {
		out.NotChecked = append(out.NotChecked, model.NotChecked{Tool: n.Tool, Bindings: countOf(bs, n.Tool), Msg: ConfigNotChecked(n).String()})
	}

	tiers := audit.Result{}
	switch {
	case ask.Tiers.Refused != "":
		out.NotChecked = append(out.NotChecked, model.NotChecked{Msg: "the tier contract was not checked: " + ask.Tiers.Refused})
	case ask.Tiers.Count() == 0:
		out.NotChecked = append(out.NotChecked, model.NotChecked{Msg: "the tier contract was not checked: no tier table is held"})
	default:
		tiers = audit.CheckTiers(audit.TierInput{
			Repo: repo, Sources: sources, Tiers: ask.Tiers.table, SystemsFile: SystemsFile,
			Systems: declaredSystems(repo), Exempt: liveRows(repo, date),
		})
		halt = errors.Join(halt, tiers.Halt)
	}
	for _, f := range tiers.Findings {
		cf := ConfigFinding(f)
		out.Findings = append(out.Findings, gateFinding(model.GateTierContract, cf.where(), cf.File, cf.Line, f.Rule+" ("+f.Tool+" "+f.Key+"): "+f.Missing, ""))
	}
	for _, e := range tiers.Exempt {
		out.Exempt = append(out.Exempt, ConfigExempt{ConfigFinding(e.Finding), e.Until}.String())
	}
	for _, n := range tiers.NotChecked {
		// a tool left unjudged ends the run not ready; a row that says one part of a
		// tool judged otherwise was not read (Partial) names no tool, since the rest
		// of it was, and a tier row of no tool at all is held by its message
		row := model.NotChecked{Msg: ConfigNotChecked(n).String()}
		if !n.Partial {
			row.Tool, row.Bindings = n.Tool, countOf(bs, n.Tool)
		}
		out.NotChecked = append(out.NotChecked, row)
	}

	out.Findings = append(out.Findings, exemptionFindings(repo, rep.Book, date)...)
	out.NotChecked = append(out.NotChecked, namesRows(ask.NamesFile, rep.Notices)...)
	return repoChecks(ctx, p.FileNames, l, rep, repo, ask).AddTo(out), halt
}

// partsLacking is the parts, in the pin's order, that the book's structure
// does not hold.
func partsLacking(b model.Book, parts []string) []string {
	have := map[string]bool{}
	for _, layer := range b.Structure.Layers {
		for _, part := range layer.Parts {
			have[part.Slug] = true
		}
	}
	var out []string
	for _, p := range parts {
		if !have[p] {
			out = append(out, p)
		}
	}
	return out
}

// projectBindings are the rung 0 (tool, key) bindings of the active and trial
// rules of the parts, in the book's order. A rule's part is the first word of
// its id. The program's computed keys are not judged against a config: the
// program computes them.
func projectBindings(b model.Book, parts []string) []audit.Binding {
	var out []audit.Binding
	for _, r := range b.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		id := string(r.ID)
		if part, _, _ := strings.Cut(id, "."); !slices.Contains(parts, part) {
			continue
		}
		for _, bd := range r.Bindings {
			if bd.Rung != 0 || bd.Tool == "" || bd.Key == "" || bd.Tool == audit.Program && audit.Computed(bd.Key) {
				continue
			}
			out = append(out, audit.Binding{Rule: id, Tool: bd.Tool, Key: bd.Key})
		}
	}
	return out
}

// unjudgedTools is a row for each tool that has bindings and no source among
// sources, so that no binding is dropped without a word.
func unjudgedTools(bs []audit.Binding, sources []audit.Source, why string) []model.NotChecked {
	var out []model.NotChecked
	seen := map[string]bool{}
	for _, b := range bs {
		if seen[b.Tool] || slices.ContainsFunc(sources, func(s audit.Source) bool { return s.Tool == b.Tool }) {
			continue
		}
		seen[b.Tool] = true
		out = append(out, model.NotChecked{Tool: b.Tool, Bindings: countOf(bs, b.Tool), Msg: b.Tool + ": " + why})
	}
	return out
}

// countOf is how many of the bindings are the tool's.
func countOf(bs []audit.Binding, tool string) int {
	n := 0
	for _, b := range bs {
		if b.Tool == tool {
			n++
		}
	}
	return n
}

// gateFinding is a finding of the project check.
func gateFinding(kind model.GateKind, entity, file string, line int, msg, fix string) model.GateFinding {
	return model.GateFinding{Kind: kind, Entity: entity, Msg: msg, Fix: fix, File: file, Line: line}
}

// statFS is an fs.FS that can be stat'ed, by the FS's own Stat or an open.
type statFS struct{ fs.FS }

func (s statFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(s.FS, name) }

// exemptionFindings holds the exemptions file to the book's active rules and
// to today. No file is none; a file that cannot be read is a finding.
func exemptionFindings(repo fs.FS, b model.Book, today string) []model.GateFinding {
	data, err := fs.ReadFile(repo, project.ExemptionsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return []model.GateFinding{gateFinding(model.GateExemption, project.ExemptionsFile, project.ExemptionsFile, 0, project.ExemptionsFile+": cannot be read: "+err.Error(), "")}
	}
	active := map[string]bool{}
	for _, r := range b.Rules {
		if r.Status == model.Active {
			active[string(r.ID)] = true
		}
	}
	var out []model.GateFinding
	for _, f := range project.CheckExemptions(data, active, today, statFS{repo}).Findings {
		out = append(out, gateFinding(model.GateExemption, f.String(), f.Path, f.Line, f.Msg, ""))
	}
	return out
}

// ProjectDoc is the document of check --json below a pin: one document on
// stdout, as the rulebook's check is, with the findings as model.GateFinding
// (a kind, and entity, msg and fix), the sentences of what an exemption
// excuses, and the rows that say something was not checked. Each is a list,
// never null.
type ProjectDoc struct {
	Schema     int                 `json:"schema"`
	Contract   string              `json:"contract"`
	Command    string              `json:"command"`
	Word       string              `json:"word"`
	Findings   []model.GateFinding `json:"findings"`
	Exempt     []string            `json:"exempt"`
	NotChecked []model.NotChecked  `json:"not_checked"`
}

// ProjectDocOf is p as the document of a run that ended on word.
func ProjectDocOf(p model.ProjectCheck, word model.Word) ProjectDoc {
	return ProjectDoc{Schema: CheckSchema, Contract: model.ContractVersion, Command: "check", Word: string(word),
		Findings: append([]model.GateFinding{}, p.Findings...), Exempt: append([]string{}, p.Exempt...),
		NotChecked: append([]model.NotChecked{}, p.NotChecked...)}
}
