package link

import (
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/model"
)

// Borrowed is a borrowed rule as the borrower holds it (S34, S70): the
// lender's bindings, each one the borrower keeps a record of at its entry
// replaced by the record, and the records at entries the lender binds none
// at added, in the order of the records. Records of another rule are left
// out. A binding taken from a record carries no Tightens: the record's
// subject is the rule itself.
func Borrowed(rule model.Rule, records []model.Tightening) model.Rule {
	out := rule
	out.Bindings = slices.Clone(rule.Bindings)
	for _, t := range records {
		if t.Subject != rule.ID {
			continue
		}
		b := t.Binding
		b.Tightens = ""
		if i := slices.IndexFunc(out.Bindings, func(o model.Binding) bool { return sameKey(o, b) }); i >= 0 {
			out.Bindings[i] = b
			continue
		}
		out.Bindings = append(out.Bindings, b)
	}
	return out
}

// records sets the entries of the layer's records of borrowed rules: each
// replaces its subject's entry of the same tool, target and key, or adds
// one, and reads "changed" by the layer. A rule is borrowed when an earlier
// layer holds it or the layer imported it. A record that cannot be read, or
// whose subject the layer did not borrow, is an error naming the layer, and
// sets no entry.
func records(out *model.Resolved, l model.ChainLayer, held map[model.RuleID]bool) []error {
	read, bad := ReadRecords(l.Book.BindingRecords)
	var errs []error
	for _, r := range bad {
		errs = append(errs, fmt.Errorf("layer %s: %s", l.Name, r))
	}
	for _, t := range read {
		if !held[t.Subject] && !importedRule(l.Book, t.Subject) {
			errs = append(errs, fmt.Errorf("layer %s: its record %s names rule %s, which it did not borrow from an earlier layer or an import", l.Name, t.Path, t.Subject))
			continue
		}
		b := t.Binding
		b.Tightens = t.Subject
		if change(out, b, l.Name, t.Subject) {
			continue
		}
		out.Entries = append(out.Entries, model.ResolvedEntry{Rule: t.Subject, Binding: b, Layer: l.Name, How: model.ResolvedChanged})
	}
	return errs
}

// importedRule says whether the book holds id as an imported rule.
func importedRule(b model.Book, id model.RuleID) bool {
	return slices.ContainsFunc(b.Rules, func(r model.Rule) bool { return r.ID == id && r.Origin == model.Imported })
}
