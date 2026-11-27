package coreapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// borrowedFindings are S34's findings at load, which validate lists: each
// record of bindings/ that link cannot read, and each whose binding weakens
// the one its lender lends at the same entry, compared by link's Tighten with
// the rank op of the plugin installed under root that serves the tool. A
// record of an imported rule is compared with the rule the book carries; one
// of a parent's rule only when the parent is handed in, and the contract's
// notice already says when it is not. A record
// whose subject no lender holds is the contract's finding, not one here, and
// a record at an entry the lender does not bind adds a binding, which
// weakens nothing. A record whose value differs from its lender's, of a tool
// no installed plugin serves, is not compared, and a notice names the tool; a
// plugin refused, or one that fails a rank, compares nothing, and the notice
// says why: a check that did not run is not a pass (cli.output.9).
func borrowedFindings(ctx context.Context, p port.Ports, root model.Root, l format.Loaded, parent *format.Foreign) (found, notices []format.Finding) {
	if len(l.Book.BindingRecords) == 0 {
		return nil, nil
	}
	read, bad := link.ReadRecords(l.Book.BindingRecords)
	for _, r := range bad {
		found = append(found, format.Finding{File: below(l.Dir, r.Path), Line: r.Line,
			Msg: fmt.Sprintf("a binding record that cannot be read: %s: %s", r.Check, r.Msg), Fix: "write it as gragas bind writes one, or remove it"})
	}
	rk := newRankers(p.Plugins, root).reading(l.Book.Identity)
	rank := rk.of(ctx)
	// unranked is a weakening whose value the plugins gave no word on: the
	// rank was unknown, and it is a finding unless no plugin served its tool,
	// or one failed, which the notice says instead.
	type weak struct {
		unranked bool
		tool     string
		f        format.Finding
	}
	var weakened []weak
	for _, t := range read {
		rule, from, ok := lenderOf(l.Book, parent, t.Subject)
		if !ok {
			continue
		}
		i := slices.IndexFunc(rule.Bindings, func(b model.Binding) bool { return sameEntry(b, t.Binding) })
		if i < 0 {
			continue
		}
		was := rule.Bindings[i]
		unknown := false
		ranked := func(key string, preset, found canon.Value) plugin.Word {
			w := rank(was.Tool)(key, preset, found)
			unknown = unknown || w == plugin.WordUnknown
			return w
		}
		_, against := link.Tighten(ranked, was, t.Binding)
		if against == nil {
			continue
		}
		file := below(l.Dir, t.Path)
		weakened = append(weakened, weak{unknown && against.Field == link.FieldValue, was.Tool, format.Finding{File: file, Line: lineOf(t.Text, against.Field),
			Msg: fmt.Sprintf("the record of %s weakens the binding %s lends at %s %s %s, at its %s: %s (S34)",
				t.Subject, from, was.Tool, was.Target, was.Key, strings.ReplaceAll(against.Field, "_", " "), against.Msg),
			Fix: removeRecord(file)}})
	}
	err := rk.close()
	if err != nil {
		notices = append(notices, format.Finding{File: below(l.Dir, format.BindingsDir),
			Msg: "the binding records were not all compared with the bindings they borrow: " + err.Error()})
	}
	for _, w := range weakened {
		if !w.unranked || !notCompared(err, w.tool) {
			found = append(found, w.f)
		}
	}
	return found, notices
}

// notCompared says whether err, what ended the rankers, left a value of tool
// with no word: a tool no plugin serves, where that is all that ended them,
// and any tool where a plugin failed or was refused.
func notCompared(err error, tool string) bool {
	if err == nil {
		return false
	}
	if u, ok := errors.AsType[*UnservedTools](err); ok {
		return slices.Contains(u.Tools, tool)
	}
	return true
}

// removeRecord is the fix of a weakening record: the step in words, with the
// record's path as %q. It is never a line to paste: the path is a name of the
// repository's own tree, which any file of bindings/ may take, a space, ; or
// $( ) among them, and a command joined from it would run more than the
// removal in any shell (go-console#218).
func removeRecord(file string) string {
	return fmt.Sprintf("remove the record %q from the repository with git rm", file)
}

// lenderOf is the borrowed rule id as its lender holds it, and who lends it:
// a rule the book imported, or one of the parent handed in.
func lenderOf(b model.Book, parent *format.Foreign, id model.RuleID) (model.Rule, string, bool) {
	if i := slices.IndexFunc(b.Rules, func(r model.Rule) bool { return r.ID == id }); i >= 0 {
		if b.Rules[i].Origin != model.Imported {
			return model.Rule{}, "", false
		}
		return b.Rules[i], importOf(b, id), true
	}
	if parent == nil {
		return model.Rule{}, "", false
	}
	if i := slices.IndexFunc(parent.Rules, func(r model.Rule) bool { return r.ID == id }); i >= 0 {
		return parent.Rules[i], "the parent " + parent.Rulebook + " " + parent.Version, true
	}
	return model.Rule{}, "", false
}

func sameEntry(a, b model.Binding) bool {
	return a.Tool == b.Tool && a.Target == b.Target && a.Key == b.Key
}

// lineOf is the line of the record text whose key is field, the line a
// finding points at; 1, the subject's, for a field the record has no line of.
func lineOf(text, field string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, field+":") {
			return i + 1
		}
	}
	return 1
}

// below is the file name below the book's folder dir, slash-separated from
// the root.
func below(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}
