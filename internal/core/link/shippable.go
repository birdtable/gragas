package link

import (
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// PresetFile is one generated file of one tool.
type PresetFile struct {
	// File is the path, relative to the config set, and the bytes.
	File model.File
	// Tool is the tool the file configures.
	Tool string
	// CopyOnce is set where the tool has no extends: a project may take the
	// file over by removing its mark, and the engine then never writes it
	// again (S42).
	CopyOnce bool
	// Mark is the mark the plugin wrote in the file, "" where its manifest
	// asks for none.
	Mark string
}

// Shipped is one binding the presets ship, with where it went. It is how every
// entry of a preset traces to the one binding that set it.
type Shipped struct {
	// Rule is the rule that holds the binding.
	Rule model.RuleID
	// Binding is the binding as the rule holds it.
	Binding model.Binding
	// File is the preset file the entry is in.
	File string
}

// Generated is the presets of one resolved rulebook (S42), as the plugins
// write them.
type Generated struct {
	// Files are the preset files, in the order of their tool and path.
	Files []PresetFile
	// Notes name, in the engine's words, each binding not shipped. They hold no
	// date.
	Notes []string
}

// markGenerated is the text of the mark a generated preset carries: the
// model's, so the gate's preset-plugins member and the engine hold one text.
const markGenerated = model.PresetMark

// candidate is a binding on its way to a preset.
type candidate struct {
	rule  model.RuleID
	trial bool
	b     model.Binding
}

// An Entry is one entry a plugin is to write: the binding that stands for a key
// at a target, the rules that hold it (every rule that states it as the binding
// does, in canon order), and whether the rule whose binding stands is a trial.
type Entry struct {
	Rules   []model.RuleID
	Trial   bool
	Binding model.Binding
}

// Shippable is, by tool, the entries of the book that ship to the tools wanted
// names, which a plugin serves: the bindings of the active and trial rules,
// below the last rung, not blocked by an open entry conflict of the chain, and
// at a target the tool serves. The bindings of a tool are taken by key, rule
// and target in canon order; the first of a key stands, a later one that
// states the key otherwise is dropped with a note ("is not written"), and one
// that states it alike adds its rule to the entry of its target. So a plugin is given each key at a target once, the
// entries in canon order of key, and never chooses between two statements of
// one key. targets answers the targets a tool serves, and whether any plugin
// holds it. The notes are those of the bindings that do not ship, and of those
// dropped.
func Shippable(book model.Book, chain model.Chain, wanted func(tool string) bool, targets func(tool string) ([]model.TargetType, bool)) (map[string][]Entry, []string) {
	by, notes := gather(book, chain, wanted, "plugin", targets)
	out := make(map[string][]Entry, len(by))
	for _, tool := range canon.SortedKeys(by) {
		cs := by[tool]
		sortCandidates(cs)
		first := map[string]candidate{}
		at := map[[2]string]int{}
		for _, c := range cs {
			if f, dup := first[c.b.Key]; dup {
				if note := conflict(f, c); note != "" {
					notes = append(notes, note)
					continue
				}
			} else {
				first[c.b.Key] = c
			}
			k := [2]string{c.b.Key, string(c.b.Target)}
			if i, ok := at[k]; ok {
				out[tool][i].Rules = append(out[tool][i].Rules, c.rule)
				continue
			}
			at[k] = len(out[tool])
			out[tool] = append(out[tool], Entry{Rules: []model.RuleID{c.rule}, Trial: c.trial, Binding: c.b})
		}
		for i := range out[tool] {
			slices.SortFunc(out[tool][i].Rules, func(a, b model.RuleID) int { return canon.Compare(string(a), string(b)) })
		}
	}
	return out, notes
}

// Unserved is, in canon order, the tools that an active or trial rule of the
// book binds below the last rung and that served says nothing holds: the tools
// whose entries Shippable would not hand to any plugin.
func Unserved(book model.Book, served func(tool string) bool) []string {
	seen := map[string]bool{}
	for _, r := range book.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		for _, b := range r.Bindings {
			if b.Rung < model.MaxRung && !served(b.Tool) {
				seen[b.Tool] = true
			}
		}
	}
	return canon.SortedKeys(seen)
}

// Mark is the text of the mark a plugin's preset opens with, in no comment
// syntax. It is the one mark, whatever the manifest's extends says: while a
// copy carries it, preset --check holds the copy byte for byte, and a project
// takes a copy-once file as its own by removing it.
func Mark() string {
	return markGenerated
}

// Canon is a value a binding holds, as canon's value.
func Canon(v any) (canon.Value, error) { return toCanon(v) }

// toCanon is a value a binding holds as canon's value.
func toCanon(v any) (canon.Value, error) {
	switch x := v.(type) {
	case nil:
		return canon.Null{}, nil
	case bool:
		return canon.Bool(x), nil
	case int:
		return canon.Int(x), nil
	case int64:
		return canon.Int(x), nil
	case float64:
		return canon.Float(x), nil
	case string:
		return canon.String(x), nil
	case *ordered.Map:
		return toCanon(ordered.Plain(x))
	case []any:
		out := make(canon.Array, len(x))
		for i, m := range x {
			c, err := toCanon(m)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := make(canon.Object, len(x))
		for k, m := range x {
			c, err := toCanon(m)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	}
	return nil, fmt.Errorf("a value of the type %T has no JSON form", v)
}

// gather is the one reading of which bindings of the book ship, for a tool
// that is served by an adapter or by a plugin alike: the bindings of the
// active and trial rules at a rung below MaxRung, of the tools wanted names.
// targets answers the targets a tool serves and whether anything holds the
// tool; holder names what holds it in the note of a binding that does not
// ship. A binding that does not ship adds its note.
func gather(book model.Book, chain model.Chain, wanted func(tool string) bool, holder string, targets func(tool string) ([]model.TargetType, bool)) (map[string][]candidate, []string) {
	byTool := map[string][]candidate{}
	var notes []string
	for _, r := range book.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		for _, b := range r.Bindings {
			if b.Rung >= model.MaxRung || !wanted(b.Tool) {
				continue
			}
			where := fmt.Sprintf("%s: %s %s %s", r.ID, b.Tool, b.Target, b.Key)
			ts, ok := targets(b.Tool)
			switch {
			case chain.Blocks(b.Ref(r.ID)):
				notes = append(notes, where+" is not shipped: it is in an open entry conflict")
			case !ok:
				notes = append(notes, where+" is not shipped: no "+holder+" holds the tool")
			case !slices.Contains(ts, b.Target):
				notes = append(notes, where+" is not shipped: the tool does not cover that target")
			default:
				byTool[b.Tool] = append(byTool[b.Tool], candidate{r.ID, r.Status == model.Trial, b})
			}
		}
	}
	return byTool, notes
}

// cmpAll is the first of cs that is not 0, or 0.
func cmpAll(cs ...int) int {
	for _, c := range cs {
		if c != 0 {
			return c
		}
	}
	return 0
}

// sortCandidates is the order a tool's candidates are settled in: by key,
// then rule, then target, in canon order. The first of a key is the one
// written, so this order is what decides which of two bindings that state a key
// differently stands.
func sortCandidates(cs []candidate) {
	slices.SortFunc(cs, func(a, b candidate) int {
		return cmpAll(canon.Compare(a.b.Key, b.b.Key), canon.Compare(string(a.rule), string(b.rule)),
			canon.Compare(string(a.b.Target), string(b.b.Target)))
	})
}

// sameEntry says whether two bindings of an entry state it alike.
func sameEntry(a, b model.Binding) bool {
	return fmt.Sprintf("%#v %#v %q", a.Value, a.Options, a.Severity) == fmt.Sprintf("%#v %#v %q", b.Value, b.Options, b.Severity)
}

// conflict is the note of a candidate that is not written because the first of
// its key, which is written, states the entry otherwise; "" when c states it
// as first does.
func conflict(first, c candidate) string {
	if sameEntry(first.b, c.b) {
		return ""
	}
	return fmt.Sprintf("%s: %s %s %s is not written: %s already sets the entry otherwise", c.rule, c.b.Tool, c.b.Target, c.b.Key, first.rule)
}
