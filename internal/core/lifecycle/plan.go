package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// The plan functions of this context share one shape: they take the book as
// the format context loaded it, a request in the words of the command and a
// Moment, and answer a Plan or a Refusal. They read no file and take no
// clock; coreapi hands in the book, the files and the moment, and carries the
// Delta to format.Render and the Record to format.HistoryRecord
// (architecture 5, A35).

// Moment is who makes a change and on which day, taken once by the caller so
// that the plan shown and the plan applied agree.
type Moment struct {
	// Who is the name and address the repository commits as.
	Who string
	// Date is the day, as YYYY-MM-DD, UTC.
	Date string
	// When is the instant, as YYYY-MM-DDThh:mm:ssZ, UTC, for a record that
	// keeps the time: an exclusion and its lifting.
	When string
}

// Record is what the history line of a plan says, besides who, when and the
// plan digest. It mirrors format.Entry: this context cannot import format.
type Record struct {
	// Kind is the slug of what the command did, such as new or edit.
	Kind string
	// Entity is the entity kind and its key, such as rule:go.errors.1.
	Entity string
	// Before is the lifecycle fields before; nil for a new entity.
	Before *model.Lifecycle
	// After is the lifecycle fields after; the status is always set.
	After model.Lifecycle
	// Decision is the slug of the decision the change records, "" for none.
	Decision string
	// Reverts is the digest of the history entry an undo takes back, "" for none.
	Reverts model.Digest
}

// Plan is what a plan function decides: the delta, the history entry and the
// sections of spec 15.3 that the context can say. An empty Delta is a plan
// whose goal already holds. Also are the entries after Record that the same
// plan writes: a source edit notes itself on each rule that cites the source.
// They share the plan's yes and its digest.
type Plan struct {
	Delta    model.Delta
	Record   Record
	Also     []Record
	Sections []model.Section
}

// Refusal is a request the context will not plan, with the line that tells a
// person what to type instead (cli.errors.2).
type Refusal struct {
	// Msg says what is wrong, in words a person reads.
	Msg string
	// Hint says what to do about it.
	Hint string
	// Word is the closing word, "" for refused: usage for a request that
	// asks for what the state does not allow, not ready for one a later
	// release of a parent could allow.
	Word model.Word
}

func (r *Refusal) Error() string { return r.Msg }

func refuse(hint, format string, args ...any) error {
	return &Refusal{Msg: fmt.Sprintf(format, args...), Hint: hint}
}

// refuseAs is refuse ending on the word w.
func refuseAs(w model.Word, hint, format string, args ...any) error {
	return &Refusal{Msg: fmt.Sprintf(format, args...), Hint: hint, Word: w}
}

// The titles of the sections of spec 15.3 a context fills, in order. The
// question is the terminal's.
const (
	secChanges   = "What changes"
	secConsumers = "Consumer actions"
	secWritten   = "What is written where"
)

// sections is the plan screen of a change to rules alone. A section with
// nothing to say is left out, never printed empty (the plan-screen board): the
// consumer actions are there only when a line names one, which a line that
// starts with "none" does not, and what is written where only when a file is.
// Re-keyed or orphaned, renderings rewritten and checker-code changes are the
// preset diff's and the bindings', so a change to rules alone has none.
func sections(changes, written []string, consumers string) []model.Section {
	out := []model.Section{{Title: secChanges, Lines: changes}}
	if consumers != "" && !strings.HasPrefix(consumers, "none") {
		out = append(out, model.Section{Title: secConsumers, Lines: []string{consumers}})
	}
	if len(written) > 0 {
		out = append(out, model.Section{Title: secWritten, Lines: written})
	}
	return out
}

// scalar writes s as a YAML value: bare when it is plain words, else
// double-quoted. A bare word that YAML would read as another type, such as
// a date, a number or yes, is quoted.
func scalar(s string) string {
	if plain(s) {
		return s
	}
	return fmt.Sprintf("%q", s)
}

func plain(s string) bool {
	if s == "" || strings.TrimSpace(s) != s || !isLetter(rune(s[0])) {
		return false
	}
	for _, r := range s {
		if !isLetter(r) && (r < '0' || r > '9') && !strings.ContainsRune(" _.-", r) {
			return false
		}
	}
	return !slices.Contains([]string{"true", "false", "null", "yes", "no", "on", "off", "y", "n"}, strings.ToLower(s))
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// clean says s holds no control character, which a one-line value cannot.
func clean(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// validSlug is the slug pattern of the schemas: words of a-z and 0-9 joined by
// single hyphens.
func validSlug(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// validDate is YYYY-MM-DD with a month of 1 to 12 and a day of 1 to 31.
func validDate(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s {
		if i != 4 && i != 7 && (r < '0' || r > '9') {
			return false
		}
	}
	m, d := s[5:7], s[8:10]
	return m >= "01" && m <= "12" && d >= "01" && d <= "31"
}

func number(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// lifecycleOf is the guarded fields of a rule as the book holds them now.
func lifecycleOf(r model.Rule) model.Lifecycle {
	return model.Lifecycle{Status: r.Status, Owner: r.Owner, ReviewBy: r.ReviewBy, ZeroBy: r.ZeroBy, ReplacedBy: r.ReplacedBy}
}

func hasPart(b model.Book, part string) bool {
	for _, l := range b.Structure.Layers {
		for _, p := range l.Parts {
			if p.Slug == part {
				return true
			}
		}
	}
	return false
}

func hasConcern(b model.Book, part, concern string) bool {
	for _, l := range b.Structure.Layers {
		for _, p := range l.Parts {
			if p.Slug != part {
				continue
			}
			for _, c := range p.Concerns {
				if c.Slug == concern {
					return true
				}
			}
		}
	}
	return false
}

func hasKind(b model.Book, kind string) bool {
	return slices.ContainsFunc(b.Structure.Kinds, func(k model.ProgramKind) bool { return k.Slug == kind })
}

func hasSource(b model.Book, slug string) bool {
	return slices.ContainsFunc(b.Sources, func(s model.Source) bool { return s.Slug == slug })
}

func hasDecision(b model.Book, slug string) bool {
	return slices.ContainsFunc(b.Decisions, func(d model.Decision) bool { return d.Slug == slug })
}

// bySlug is the entry of xs whose slug is slug: a term, a source.
func bySlug[T any](xs []T, slug string, slugOf func(T) string) (T, bool) {
	i := slices.IndexFunc(xs, func(x T) bool { return slugOf(x) == slug })
	if i < 0 {
		var none T
		return none, false
	}
	return xs[i], true
}

// nothingPlan is the plan of a goal that holds: no delta, and a record that is
// valid, since the write path asks for it once even for an empty plan.
func nothingPlan(kind, entity string, now model.Lifecycle, why string) Plan {
	return Plan{Record: Record{Kind: kind, Entity: entity, Before: &now, After: now},
		Sections: sections([]string{"nothing: " + why}, nil, "none")}
}

// secTools is the section of the preset diff, after what changes.
const secTools = "Tool entries"

// WithPresets is p with the preset change its Delta causes: the section of
// tool entries after what changes, and the preset files the change writes
// named in what is written where (S43). An empty entries is "no preset
// change", so a plan that moves no entry says so and writes no preset file.
// The rulebook's own change is p's; this adds only what the generated presets
// make of it.
func WithPresets(p Plan, entries, written []string) Plan {
	if len(entries) == 0 {
		entries = []string{"no preset change"}
	}
	out := make([]model.Section, 0, len(p.Sections)+1)
	for _, s := range p.Sections {
		if s.Title == secWritten && len(written) > 0 {
			s.Lines = append(slices.Clone(s.Lines), written...)
		}
		out = append(out, s)
		if s.Title == secChanges {
			out = append(out, model.Section{Title: secTools, Lines: entries})
		}
	}
	p.Sections = out
	return p
}

// secStanding and secProof are the sections of the standing before and after
// and of the proof to be run, which follow the tool entries (S34, S43).
const (
	secStanding = "Standing before and after"
	secProof    = "Proof to run"
)

// WithStanding is p with the standing the change moves and the proof it adds:
// a section for each that has a line, none that has not.
func WithStanding(p Plan, standing, proof []string) Plan {
	out := make([]model.Section, 0, len(p.Sections)+2)
	for _, s := range p.Sections {
		out = append(out, s)
		if s.Title != secTools {
			continue
		}
		if len(standing) > 0 {
			out = append(out, model.Section{Title: secStanding, Lines: standing})
		}
		if len(proof) > 0 {
			out = append(out, model.Section{Title: secProof, Lines: proof})
		}
	}
	p.Sections = out
	return p
}
