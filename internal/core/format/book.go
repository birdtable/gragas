package format

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// The files of a book below its folder, and the schemas that hold them.
const (
	TermsFile      = "terms.yaml"
	SourcesFile    = "sources.yaml"
	DecisionsFile  = "decisions.yaml"
	ExclusionsFile = "exclusions.yaml"
	HistoryFile    = "history.jsonl"
	ProposalsDir   = "proposals"
	BindingsDir    = "bindings"

	rulesSchema      = "v3/rules.schema.json"
	termsSchema      = "v3/terms.schema.json"
	sourcesSchema    = "v3/sources.schema.json"
	decisionsSchema  = "v3/decisions.schema.json"
	exclusionsSchema = "v3/exclusions.schema.json"
	historySchema    = "v3/history.schema.json"
	proposalsSchema  = "v3/proposals.schema.json"
)

// reader reads the files of one book. Each file is read and parsed once.
type reader struct {
	tree   port.Tree
	schema port.Schema
	root   model.Root
	dir    string
	out    *Loaded
	// ids is where each rule id and alias was first used, file:line.
	ids map[model.RuleID]string
	// refs and slugs are the places that name a rule and a decision, checked
	// once everything is read.
	refs  *[]ref
	slugs *[]slugRef
	prose *[]proseItem
	trans *[]transEntry
	opts  Options
}

// loadRest reads everything the structure names, and the entity files beside
// it, into out. A file with a problem adds findings and the rest is still
// read. Only a failure of the port is an error.
func (r reader) loadRest(st model.Structure) error {
	for _, l := range st.Layers {
		for _, p := range l.Parts {
			if err := r.rules(l.Slug, p); err != nil {
				return err
			}
		}
	}
	var err error
	if r.out.Book.Terms, err = readList(r, TermsFile, termsSchema, "terms", termOf); err != nil {
		return err
	}
	if r.out.Book.Sources, err = readList(r, SourcesFile, sourcesSchema, "sources", sourceOf); err != nil {
		return err
	}
	if r.out.Book.Decisions, err = readList(r, DecisionsFile, decisionsSchema, "decisions", r.decisionRefs); err != nil {
		return err
	}
	if r.out.Book.Exclusions, err = readList(r, ExclusionsFile, exclusionsSchema, "exclusions", r.exclusionKeys()); err != nil {
		return err
	}
	if err := r.history(); err != nil {
		return err
	}
	if err := r.proposals(); err != nil {
		return err
	}
	if err := r.bindingRecords(); err != nil {
		return err
	}
	if err := r.translations(); err != nil {
		return err
	}
	r.checkTranslations()
	r.checkRefs()
	r.lint()
	return nil
}

// exclusionKeys reads the exclusions and lists a key written twice: two
// branches that each excluded the rule took the same next number, and a
// merge made past guard staged holds both records (S4). No command
// renumbers or removes a record, and a hand renumber is a change with no
// line, so the finding names the way guard staged's refusal of that merge
// names: one side's commit dropped, and its exclusion made again through
// its command on the merge (A38).
func (r reader) exclusionKeys() func(*ordered.Map) model.Exclusion {
	first := map[string]int{}
	return func(m *ordered.Map) model.Exclusion {
		e := exclusionOf(m)
		key := fmt.Sprintf("%s/%s/%d", e.Overlay, e.Rule, e.Number)
		if line, ok := first[key]; ok {
			r.out.Findings = append(r.out.Findings, Finding{File: join(r.dir, ExclusionsFile), Line: m.Line, Msg: fmt.Sprintf(
				"the exclusion %s is written twice (first on line %d): two branches each took the number, and guard staged refuses the merge that keeps both", key, line),
				Fix: "make the merge again: drop one side's commit that takes the number, merge again, then exclude the rule again with gragas rule exclude on the merge, which takes the next number"})
		} else {
			first[key] = m.Line
		}
		return e
	}
}

// decisionRefs reads a decision and keeps the rules it cites to be checked.
func (r reader) decisionRefs(m *ordered.Map) model.Decision {
	d := decisionOf(m)
	if d.Cites != nil {
		line := m.Sub("cites").KeyLine("rules")
		for _, id := range d.Cites.Rules {
			*r.refs = append(*r.refs, ref{id: id, file: join(r.dir, DecisionsFile), line: line, what: "cites"})
		}
	}
	return d
}

// rules reads the concern files of one part: <layer>/<part>/rules/<concern>.yaml.
func (r reader) rules(layer string, p model.Part) error {
	base := join(r.dir, layer+"/"+p.Slug+"/rules")
	known := map[string]bool{}
	for _, c := range p.Concerns {
		file := base + "/" + c.Slug + ".yaml"
		known[file] = true
		doc, bad, err := readFile(r.tree, r.schema, r.root, file, rulesSchema, migrations())
		if err != nil {
			return err
		}
		r.out.Findings = append(r.out.Findings, bad...)
		if doc == nil {
			continue
		}
		if got := doc.Str("part"); got != p.Slug {
			r.add(file, lineOfKey(doc, "part"), "part is "+got+" and the file is in the folder of part "+p.Slug)
		}
		if got := doc.Str("concern"); got != c.Slug {
			r.add(file, lineOfKey(doc, "concern"), "concern is "+got+" and the file is named for concern "+c.Slug)
		}
		for _, m := range ordered.Maps(doc.List("rules")) {
			r.addRule(ruleOf(m), m, file, 0)
		}
	}
	if err := r.guide(layer, p); err != nil {
		return err
	}
	entries, err := r.tree.List(r.root, base, model.Every)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Dir && !known[e.Path] {
			r.add(e.Path, 0, "not a concern of part "+p.Slug+"; name it in structure.yaml or remove it")
		}
	}
	return nil
}

// readList reads a file that holds one list of entities under key. The file is
// optional: a rulebook that has no exclusions has no exclusions.yaml.
func readList[T any](r reader, name, schemaName, key string, conv func(*ordered.Map) T) ([]T, error) {
	file := join(r.dir, name)
	if !r.exists(file) {
		return nil, nil
	}
	doc, bad, err := readFile(r.tree, r.schema, r.root, file, schemaName, migrations())
	if err != nil {
		return nil, err
	}
	r.out.Findings = append(r.out.Findings, bad...)
	if doc == nil {
		return nil, nil
	}
	var out []T
	for _, m := range ordered.Maps(doc.List(key)) {
		out = append(out, conv(m))
	}
	return out, nil
}

// history reads history.jsonl, one entry a line. A blank line is skipped; a
// line with a problem is a finding at its line in the file.
func (r reader) history() error {
	file := join(r.dir, HistoryFile)
	if !r.exists(file) {
		return nil
	}
	data, err := r.tree.Read(r.root, file)
	if err != nil {
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		doc, bad, err := parse(r.schema, file, []byte(line), historySchema, migrations())
		if nr, ok := errors.AsType[*NotReady](err); ok {
			return nr
		}
		if err != nil {
			return err
		}
		for _, f := range bad {
			f.Line += i
			if f.Line == i {
				f.Line = i + 1
			}
			r.out.Findings = append(r.out.Findings, f)
		}
		if doc != nil {
			h := historyOf(doc)
			h.Digest = lineDigest([]byte(line))
			r.out.Book.History = append(r.out.Book.History, h)
		}
	}
	return nil
}

// proposals reads each proposals/<slug>.yaml.
func (r reader) proposals() error {
	base := join(r.dir, ProposalsDir)
	entries, err := r.tree.List(r.root, base, model.Every)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Dir || path.Ext(e.Path) != ".yaml" {
			continue
		}
		doc, bad, err := readFile(r.tree, r.schema, r.root, e.Path, proposalsSchema, migrations())
		if err != nil {
			return err
		}
		r.out.Findings = append(r.out.Findings, bad...)
		if doc == nil {
			continue
		}
		if want := strings.TrimSuffix(path.Base(e.Path), ".yaml"); doc.Str("slug") != want {
			r.add(e.Path, lineOfKey(doc, "slug"), "slug is "+doc.Str("slug")+" and the file is named "+want)
		}
		r.agrees(e.Path, doc)
		r.out.Book.Proposals = append(r.out.Book.Proposals, proposalOf(doc))
	}
	return nil
}

// agrees adds the findings of disagreements.
func (r reader) agrees(file string, doc *ordered.Map) {
	r.out.Findings = append(r.out.Findings, disagreements(file, doc)...)
}

// disagreements is a finding for a document that holds no proposal, and one
// for each field the proposal file and its document hold differently: the
// file's slug, target, state, link and opened are what the engine reads, and
// the document is what a target's owner reads, so the two never say
// different things.
func disagreements(file string, doc *ordered.Map) []Finding {
	if doc == nil {
		return nil // the schema has refused the file
	}
	d := doc.Sub("document")
	if d == nil {
		return nil // the schema has refused a file with no document
	}
	p := d.Sub("proposal")
	if p == nil {
		return []Finding{{File: file, Line: lineOfKey(doc, "document"), Msg: "document holds no proposal: it is the proposal document propose printed, with its contract and its proposal"}}
	}
	var out []Finding
	for _, k := range []string{"slug", "target", "state", "link", "opened"} {
		if p.Str(k) != doc.Str(k) {
			line := p.KeyLine(k)
			if line == 0 {
				line = lineOfKey(d, "proposal") // the key is missing from the document
			}
			out = append(out, Finding{File: file, Line: line, Msg: fmt.Sprintf("document.proposal.%s is %q and the file's %s is %q: write the same in both", k, p.Str(k), k, doc.Str(k))})
		}
	}
	return out
}

// bindingRecords keeps each file of bindings/ as path, digest and bytes: no
// schema, nothing interpreted (link owns what a record means, the overlay
// contract reads its subject).
func (r reader) bindingRecords() error {
	base := join(r.dir, BindingsDir)
	entries, err := r.tree.List(r.root, base, model.Every)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		data, err := r.tree.Read(r.root, e.Path)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(e.Path, join(r.dir, ""))
		r.out.Book.BindingRecords = append(r.out.Book.BindingRecords, model.Opaque{
			Path: rel, Digest: digestOf(data), Data: data,
		})
	}
	return nil
}

func (r reader) exists(file string) bool {
	_, err := r.tree.Stat(r.root, file)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}

func (r reader) add(file string, line int, msg string) {
	r.out.Findings = append(r.out.Findings, Finding{File: file, Line: line, Msg: msg})
}
