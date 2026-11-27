package coreapi

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ToolsSchema is the schema number of ToolsDoc, the document the tools
// command answers with under --json. In 2 a tool's row names
// the plugin that serves it, by name and version, where 1 named the version
// of the adapter that read it, and the word is not ready while a tool a rule
// binds to has none.
const ToolsSchema = 2

// FindingsError is a rulebook with findings: what its bindings say cannot be
// told, because a file with a finding is not read. validate lists them.
type FindingsError struct{ Count int }

func (e *FindingsError) Error() string {
	return fmt.Sprintf("the rulebook has %d findings, so its bindings cannot be read", e.Count)
}

// UnapprovedPlugin is a listing of the tools ended by an installed plugin that
// is not approved: Open refuses it (port.ErrNotApproved), and a refusal gives
// no manifest, so which tool it serves cannot be said. It
// matches Err, and the ending names the approval that opens it.
type UnapprovedPlugin struct {
	Ref model.PluginRef
	Err error
}

func (e *UnapprovedPlugin) Error() string {
	return fmt.Sprintf("the plugin %s %s could not be opened: %v", e.Ref.Name, e.Ref.Version, e.Err)
}

// Unwrap is the host's refusal.
func (e *UnapprovedPlugin) Unwrap() error { return e.Err }

// ListTools lists the tools the rulebook at root binds rules to: for each, the
// version its tested set names, the installed plugin that serves it, and the
// entries its rules hold, each with the rules that hold it. A tool the tested
// set names that no rule binds to is listed with no entry, and a tool a rule
// binds to that the tested set does not name has no tested version. This is
// the rulebook form; the project form comes later. A folder with no rulebook
// is ErrNoBook, a rulebook with findings is FindingsError, and an open run and
// a newer format are as for the other listings. It reads no adapter: the
// plugin of a tool is the first installed one, in the order List gives, whose
// manifest names it, and a plugin that is refused or fails to open ends the
// listing, as it ends every read of the plugins (fail closed).
func ListTools(ctx context.Context, p port.Ports, root model.Root) (model.ToolsListing, error) {
	if err := ctx.Err(); err != nil {
		return model.ToolsListing{}, err
	}
	in, err := readBook(ctx, p, root)
	if err != nil {
		return model.ToolsListing{}, err
	}
	if !in.hasBook {
		return model.ToolsListing{}, ErrNoBook
	}
	if n := len(in.rep.Findings); n > 0 {
		return model.ToolsListing{}, &FindingsError{Count: n}
	}
	held, bound := heldEntries(in.rep.Book.Rules)
	tested := map[string]string{}
	if in.tested != nil {
		for _, v := range in.tested.Tools {
			tested[v.Name] = v.Version
		}
	}
	served, err := servingPlugins(ctx, p.Plugins, root, in.tested)
	if err != nil {
		return model.ToolsListing{}, err
	}
	return model.ToolsListing{Dir: in.rep.Dir, Rules: bound, Rows: toolRows(held, tested, served),
		Checkers: checkerRows(in.rep.Book.Identity.Name, in.rep.Checkers)}, nil
}

// servingPlugins is, by tool, the installed plugin of root that serves it, by
// the name and version its manifest says: the one chosenPlugins chooses, the
// same the rank, the preset and the import choose. Each plugin
// is opened to read its manifest, since a ref carries no tool, and the kept
// ones closed again. One that is refused or fails to open is an error, and no
// listing is given in its place: a refusal gives no manifest, so a refused
// plugin cannot be told from a tampered plugin of a tool the rules bind to.
func servingPlugins(ctx context.Context, plugins port.Plugins, root model.Root, tested *model.Tested) (map[string]model.ToolPlugin, error) {
	chosen, err := chosenPlugins(ctx, plugins, root, tested)
	if err != nil {
		return nil, err
	}
	served := map[string]model.ToolPlugin{}
	for _, tool := range canon.SortedKeys(chosen) {
		m := chosen[tool].Manifest()
		served[tool] = model.ToolPlugin{Name: m.Name, Version: m.Version}
	}
	if err := closePresetPlugins(chosen); err != nil {
		return nil, err
	}
	return served, nil
}

// Unserved is the tools a rule of l binds to that no installed plugin
// serves, by name: while there is one, the listing is not ready, for no check
// of the rulebook reads that tool's config. A tool only the
// tested set names is not one.
func Unserved(l model.ToolsListing) []string {
	var out []string
	for _, r := range l.Rows {
		if r.Plugin == nil && len(r.Entries) > 0 {
			out = append(out, r.Tool)
		}
	}
	return out
}

// checkerRows is each checker-code declaration as the listing shows it, by
// name: the rulebook's own under its own name, and an import's by reference
// under the import's name and the publisher it has there. Nothing in
// the code is read.
func checkerRows(book string, decls []model.Checker) []model.CheckerRow {
	out := make([]model.CheckerRow, 0, len(decls))
	for _, c := range decls {
		row := model.CheckerRow{Name: c.Name, Tool: c.Tool, Publisher: c.Publisher, Rulebook: book}
		if c.From != "" {
			row.Rulebook, row.ByReference = c.From, true
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b model.CheckerRow) int { return strings.Compare(a.Name, b.Name) })
	if len(out) == 0 {
		return nil
	}
	return out
}

// entryKey is a tool entry: the target and the key.
type entryKey struct {
	target model.TargetType
	key    string
}

// heldEntries is, by tool, each entry the rules' bindings hold with the rules
// that hold it in the order the rules are, and how many rules hold any binding.
func heldEntries(rules []model.Rule) (map[string]map[entryKey][]model.RuleID, int) {
	held := map[string]map[entryKey][]model.RuleID{}
	bound := 0
	for _, r := range rules {
		for _, b := range r.Bindings {
			if held[b.Tool] == nil {
				held[b.Tool] = map[entryKey][]model.RuleID{}
			}
			k := entryKey{b.Target, b.Key}
			if !slices.Contains(held[b.Tool][k], r.ID) {
				held[b.Tool][k] = append(held[b.Tool][k], r.ID)
			}
		}
		if len(r.Bindings) > 0 {
			bound++
		}
	}
	return held, bound
}

// toolRows is a row for each tool a rule binds to or the tested set names, by
// name, with its tested version, the plugin that serves it and its entries.
func toolRows(held map[string]map[entryKey][]model.RuleID, tested map[string]string, served map[string]model.ToolPlugin) []model.ToolRow {
	names := make([]string, 0, len(held)+len(tested))
	for n := range held {
		names = append(names, n)
	}
	for n := range tested {
		if _, ok := held[n]; !ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	var rows []model.ToolRow
	for _, n := range names {
		row := model.ToolRow{Tool: n, Tested: tested[n], Entries: entryRows(held[n])}
		if pl, ok := served[n]; ok {
			row.Plugin = &pl
		}
		rows = append(rows, row)
	}
	return rows
}

// entryRows is the entries of one tool by target and key, each with its rules
// by id.
func entryRows(entries map[entryKey][]model.RuleID) []model.ToolEntryRow {
	rows := []model.ToolEntryRow{}
	for k, rules := range entries {
		slices.Sort(rules)
		rows = append(rows, model.ToolEntryRow{Target: k.target, Key: k.key, Rules: rules})
	}
	slices.SortFunc(rows, func(a, b model.ToolEntryRow) int {
		switch {
		case a.Target != b.Target && a.Target < b.Target, a.Target == b.Target && a.Key < b.Key:
			return -1
		case a.Target != b.Target, a.Key != b.Key:
			return 1
		}
		return 0
	})
	return rows
}

// ToolsDoc is the tools listing as the document under --json.
type ToolsDoc struct {
	Schema   int    `json:"schema"`
	Contract string `json:"contract"`
	Command  string `json:"command"`
	// Word is the closing word: done, or not ready while Unserved names a
	// tool.
	Word string `json:"word"`
	// Unserved is the tools a rule binds to that no installed plugin serves,
	// by name; empty when every one has a plugin.
	Unserved []string `json:"unserved"`
	// Dir is the folder of the rulebook below the root, "" for the root.
	Dir string `json:"dir"`
	// Rules is how many rules hold at least one binding.
	Rules int `json:"rules"`
	// Entries is how many tool entries the rules hold, over every tool.
	Entries int `json:"entries"`
	// Unexplained is the entries of the tools' configs that no rule explains,
	// which the listing does not read yet (coverage reads them, S47): always
	// empty here, and said so.
	Unexplained []string        `json:"unexplained"`
	Tools       []model.ToolRow `json:"tools"`
	// Checkers are the rulebook's checker-code declarations, its own and an
	// import's by reference; absent where it declares none.
	Checkers []model.CheckerRow `json:"checkers,omitempty"`
}

// ToolsDocument is l as the document.
func ToolsDocument(l model.ToolsListing) ToolsDoc {
	n := 0
	for _, r := range l.Rows {
		n += len(r.Entries)
	}
	word, unserved := model.Done, Unserved(l)
	if len(unserved) > 0 {
		word = model.NotReady
	}
	return ToolsDoc{Schema: ToolsSchema, Contract: model.ContractVersion, Command: "tools", Word: string(word), Unserved: append([]string{}, unserved...),
		Dir: l.Dir, Rules: l.Rules, Entries: n, Unexplained: []string{}, Tools: append([]model.ToolRow{}, l.Rows...), Checkers: l.Checkers}
}
