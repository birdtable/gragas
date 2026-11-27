package link

import (
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// The fields Tighten names, one per way a borrower's binding can fail to be
// the borrowed one tightened (S34, S70).
const (
	// FieldEntry is a move to another tool, target or key.
	FieldEntry = "entry"
	// FieldRung and FieldRunPoint are a change of where the binding runs: a
	// borrower adds a binding instead.
	FieldRung     = "rung"
	FieldRunPoint = "run_point"
	// FieldValue, FieldOptions and FieldSeverity are the three aspects a
	// borrower may tighten.
	FieldValue    = "value"
	FieldOptions  = "options"
	FieldSeverity = "severity"
	// FieldRemedy, FieldPair and FieldChecker are fields no plugin ranks, so a
	// change of one cannot be compared.
	FieldRemedy  = "remedy"
	FieldPair    = "pair"
	FieldChecker = "checker"
)

// Against is why a borrower's binding is not the borrowed one tightened: the
// field, and a sentence saying how, which names the field and both values.
type Against struct {
	// Field is one of the Field constants.
	Field string
	// Msg is the sentence, without a closing word.
	Msg string
}

// Tighten compares next, the binding an overlay or an importer asks for at
// an entry of a rule it borrowed, with was, the binding it borrows there
// (S34, S70). It answers same when next is was, and else nil when next is was
// tightened: the same entry, rung and run point, each of value, options and
// severity the same or tighter, and at least one tighter. Anything else is
// an Against naming the first field that keeps it from that, in the order
// entry, rung, run point, value, options, severity, then the fields no
// plugin ranks. A value the plugin's rank op cannot rank, or one of a tool no
// plugin serves, a severity outside the closed set and a change of a field no
// plugin ranks cannot be compared, and what cannot be compared is refused,
// never let through.
//
// rank is the rank op of the plugin that serves next's tool, which ranks the
// value at next's key; nil ranks none.
func Tighten(rank Rank, was, next model.Binding) (same bool, against *Against) {
	if was.Tool != next.Tool || was.Target != next.Target || was.Key != next.Key {
		return false, &Against{FieldEntry, fmt.Sprintf("the entry would move from %s to %s: a borrowed rule's binding stays at its entry, and the borrower adds a binding at another", entryName(was), entryName(next))}
	}
	if was.Rung != next.Rung {
		return false, &Against{FieldRung, fmt.Sprintf("the rung would go from %d to %d: a borrowed rule's binding keeps its rung, and the borrower adds a binding at another", was.Rung, next.Rung)}
	}
	if was.RunPoint != next.RunPoint {
		return false, &Against{FieldRunPoint, fmt.Sprintf("the run point would go from %s to %s: a borrowed rule's binding keeps its run point, and the borrower adds a binding at another", was.RunPoint, next.RunPoint)}
	}
	tighter := false
	for _, step := range []func(Rank, model.Binding, model.Binding) (bool, *Against){valueStep, optionsStep, severityStep} {
		up, a := step(rank, was, next)
		if a != nil {
			return false, a
		}
		tighter = tighter || up
	}
	for _, f := range []struct {
		field     string
		was, next string
	}{
		{FieldRemedy, was.Remedy, next.Remedy},
		{FieldPair, fmt.Sprintf("%d %s", was.Pair, was.PairPath), fmt.Sprintf("%d %s", next.Pair, next.PairPath)},
		{FieldChecker, was.Checker, next.Checker},
	} {
		if f.was != f.next {
			return false, &Against{f.field, fmt.Sprintf("the %s would change, and no plugin ranks a %s, so the change cannot be compared with the borrowed binding", f.field, f.field)}
		}
	}
	return !tighter, nil
}

// valueStep is whether next's value is tighter than was's, or why it is not
// the same or tighter: the plugin reads it as weaker, or cannot rank the two.
func valueStep(rank Rank, was, next model.Binding) (bool, *Against) {
	switch rankOf(rank, next.Key, was.Value, next.Value) {
	case plugin.WordSame:
		return false, nil
	case plugin.WordTighter:
		return true, nil
	case plugin.WordWeaker:
		return false, &Against{FieldValue, fmt.Sprintf("the value would go from %s to %s, which the plugin that serves %s reads as weaker at %s", stable(was.Value), stable(next.Value), next.Tool, next.Key)}
	case plugin.WordUnknown:
		// The two cannot be compared, as for a word no plugin should give.
	}
	return false, &Against{FieldValue, fmt.Sprintf("the value would go from %s to %s, and no installed plugin of %s ranks the two at %s, so the change cannot be compared", stable(was.Value), stable(next.Value), next.Tool, next.Key)}
}

// optionsStep is whether next's options are was's, or why they are not: an
// options change carries no word, since the rank op ranks a value at a key and
// never an options map, and dropping an option can as well turn a check off as
// loosen one (gragas#1554). So the same options are the same, and any other
// options cannot be compared.
func optionsStep(_ Rank, was, next model.Binding) (bool, *Against) {
	if stable(was.Options) == stable(next.Options) {
		return false, nil
	}
	return false, &Against{FieldOptions, fmt.Sprintf("the options would go from %s to %s, and no plugin ranks options, so the change cannot be compared with the borrowed binding", stable(was.Options), stable(next.Options))}
}

// severityStep is whether next's severity is higher than was's, or why it is
// not the same or higher.
func severityStep(_ Rank, was, next model.Binding) (bool, *Against) {
	if was.Severity == next.Severity {
		return false, nil
	}
	wr, nr := severityRank(was.Severity), severityRank(next.Severity)
	switch {
	case wr < 0 || nr < 0:
		return false, &Against{FieldSeverity, fmt.Sprintf("the severity would go from %s to %s, and only error, warning and info rank, so the change cannot be compared", was.Severity, next.Severity)}
	case nr < wr:
		return false, &Against{FieldSeverity, fmt.Sprintf("the severity would go from %s to %s, which is weaker", was.Severity, next.Severity)}
	}
	return true, nil
}

func entryName(b model.Binding) string { return fmt.Sprintf("%s %s %s", b.Tool, b.Target, b.Key) }
