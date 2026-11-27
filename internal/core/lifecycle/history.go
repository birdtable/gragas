// Package lifecycle is the context that decides how a rule's life changes. It
// holds what history.jsonl means: the reader of the chain, which orders the
// entries, checks each one's link to the entry before it and says what it
// could not read. It takes lines as arguments and imports no other context
// (arch.ports.8); coreapi reads the bytes through the Tree or History port and
// hands them here.
package lifecycle

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
)

// Entry is one entry read from a line, with where it was and its own digest:
// the canonical digest of the line, which the entry after it names.
type Entry struct {
	model.HistoryEntry
	// Line is the line of the file the entry was read from, from 1.
	Line int
	// Digest is the canonical digest of the line.
	Digest model.Digest
}

// Skipped is a line that could not be read as an entry. It is not an entry
// and the chain goes on without it (store.state-files.5).
type Skipped struct {
	// Line is the line of the file, from 1.
	Line int
	// Why says what is wrong with it, in words a person reads.
	Why string
}

// Kind is what a finding of the chain is about.
type Kind string

const (
	// BrokenPrevious is an entry whose previous digest is the digest of no
	// entry with a lower number: a line was rewritten or removed.
	BrokenPrevious Kind = "broken-previous"
	// MissingPrevious is an entry after the first that names no previous.
	MissingPrevious Kind = "missing-previous"
	// Gap is a number, or a run of numbers, that no entry holds although a
	// higher one exists.
	Gap Kind = "gap"
)

// Finding is one thing wrong with the chain. A shared sequence number is
// normal after a merge (A38) and is never one.
type Finding struct {
	Kind Kind
	// Number is the entry's number; for a gap the first number missing.
	Number int
	// Line is the entry's line; for a gap the line of the entry after it.
	Line int
	// Msg says what is wrong, in words a person reads.
	Msg string
}

// Chain is the history as read: the entries in order, what was skipped, and
// what the check of the links found.
type Chain struct {
	// Entries are the entries read, by number, then when, then plan digest,
	// then the line's own digest, so that two files with the same lines in any
	// order give the same sequence.
	Entries []Entry
	// Skipped are the lines that were not entries, in file order.
	Skipped []Skipped
	// Findings are the broken links and gaps, in the order of Entries.
	Findings []Finding
}

// Next is the number the next append takes: the highest number plus one, one
// for an empty history. It is what the apply path puts in the delta.
func (c Chain) Next() int {
	if len(c.Entries) == 0 {
		return 1
	}
	return c.Entries[len(c.Entries)-1].Number + 1
}

// Lines splits file content into its lines, leaving out a blank one and the
// carriage return of a CRLF.
func Lines(data []byte) [][]byte {
	lines, _ := split(data)
	return lines
}

// split is Lines with the line of the file each came from.
func split(data []byte) (lines [][]byte, at []int) {
	n := 0
	for len(data) > 0 {
		n++
		var line []byte
		line, data, _ = bytes.Cut(data, []byte("\n"))
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines, at = append(lines, line), append(at, n)
	}
	return lines, at
}

// ReadFile reads the content of history.jsonl: ReadHistory over its lines, the
// lines of the file numbering any that are skipped.
func ReadFile(data []byte) Chain {
	lines, at := split(data)
	return read(lines, at)
}

// ReadHistory reads lines, one entry each, in one pass: each is decoded and
// digested once, the entries are put in order, and the digest each names as
// the one before it is looked up. A line that is not an entry is skipped and
// listed. The lines are numbered from 1 in the order given.
func ReadHistory(lines [][]byte) Chain {
	return read(lines, nil)
}

func read(lines [][]byte, at []int) Chain {
	var c Chain
	c.Entries = make([]Entry, 0, len(lines))
	for i, line := range lines {
		n := i + 1
		if at != nil {
			n = at[i]
		}
		e, err := entryOf(line)
		if err != nil {
			c.Skipped = append(c.Skipped, Skipped{Line: n, Why: err.Error()})
			continue
		}
		e.Line = n
		c.Entries = append(c.Entries, e)
	}
	slices.SortStableFunc(c.Entries, func(a, b Entry) int {
		return cmp.Or(
			cmp.Compare(a.Number, b.Number),
			strings.Compare(a.When, b.When),
			strings.Compare(string(a.Plan), string(b.Plan)),
			strings.Compare(string(a.Digest), string(b.Digest)),
		)
	})
	c.Findings = check(c.Entries)
	return c
}

// check lists the links that do not hold. An entry's previous must be the
// digest of an entry with a lower number; entries that share a number and
// name the same parent both hold. Numbers must run on from 1 without a hole.
func check(entries []Entry) []Finding {
	lowest := make(map[model.Digest]int, len(entries))
	for _, e := range entries {
		if _, seen := lowest[e.Digest]; !seen {
			lowest[e.Digest] = e.Number
		}
	}
	var out []Finding
	seen := 0
	for _, e := range entries {
		if e.Number > seen+1 {
			out = append(out, Finding{Kind: Gap, Number: seen + 1, Line: e.Line, Msg: gapText(seen+1, e.Number-1)})
		}
		seen = max(seen, e.Number)
		switch n, ok := lowest[e.Previous]; {
		case e.Previous == "" && e.Number == 1:
		case e.Previous == "":
			out = append(out, Finding{Kind: MissingPrevious, Number: e.Number, Line: e.Line,
				Msg: fmt.Sprintf("entry %d names no previous entry, so the chain does not reach back from it", e.Number)})
		case !ok || n >= e.Number:
			out = append(out, Finding{Kind: BrokenPrevious, Number: e.Number, Line: e.Line,
				Msg: fmt.Sprintf("entry %d names %s as the entry before it, and no entry with a lower number has that digest: a line was rewritten or removed", e.Number, e.Previous)})
		}
	}
	return out
}

func gapText(from, to int) string {
	if from == to {
		return fmt.Sprintf("no entry has number %d, and a higher one exists", from)
	}
	return fmt.Sprintf("no entry has a number from %d to %d, and a higher one exists", from, to)
}

// entryOf reads one line. Every field the schema requires must be there with
// its type; a field the reader does not know is left alone, since a newer
// writer may add one.
func entryOf(line []byte) (Entry, error) {
	v, err := canon.Decode(line)
	if err != nil {
		return Entry{}, fmt.Errorf("the line is not JSON: %w", err)
	}
	o, ok := v.(canon.Object)
	if !ok {
		return Entry{}, fmt.Errorf("the line is not an object")
	}
	var e Entry
	f, err := intField(o, "format")
	if err != nil {
		return Entry{}, err
	}
	n, err := intField(o, "number")
	if err != nil {
		return Entry{}, err
	}
	if n < 1 {
		return Entry{}, fmt.Errorf("the number %d is not 1 or more", n)
	}
	e.Format, e.Number = f, n
	for _, s := range []struct {
		key string
		to  *string
	}{{"kind", &e.Kind}, {"entity", &e.Entity}, {"who", &e.Who}, {"when", &e.When}} {
		if *s.to, err = textField(o, s.key, true); err != nil {
			return Entry{}, err
		}
	}
	plan, err := textField(o, "plan", true)
	if err != nil {
		return Entry{}, err
	}
	previous, err := textField(o, "previous", false)
	if err != nil {
		return Entry{}, err
	}
	if e.Decision, err = textField(o, "decision", false); err != nil {
		return Entry{}, err
	}
	reverts, err := textField(o, "reverts", false)
	if err != nil {
		return Entry{}, err
	}
	e.Reverts = model.Digest(reverts)
	if e.Command, err = textField(o, "command", false); err != nil {
		return Entry{}, err
	}
	e.Plan, e.Previous = model.Digest(plan), model.Digest(previous)
	if e.After, err = lifecycleField(o, "after", true); err != nil {
		return Entry{}, err
	}
	if _, has := o["before"]; has {
		b, err := lifecycleField(o, "before", true)
		if err != nil {
			return Entry{}, err
		}
		e.Before = &b
	}
	d, err := canon.DigestOf(v)
	if err != nil {
		return Entry{}, fmt.Errorf("the line cannot be digested: %w", err)
	}
	e.Digest = model.Digest(d.String())
	return e, nil
}

func intField(o canon.Object, key string) (int, error) {
	v, ok := o[key]
	if !ok {
		return 0, fmt.Errorf("the line has no %s", key)
	}
	n, ok := v.(canon.Int)
	if !ok {
		return 0, fmt.Errorf("%s is not a whole number", key)
	}
	return int(n), nil
}

func textField(o canon.Object, key string, required bool) (string, error) {
	v, ok := o[key]
	if !ok {
		if required {
			return "", fmt.Errorf("the line has no %s", key)
		}
		return "", nil
	}
	s, ok := v.(canon.String)
	if !ok {
		return "", fmt.Errorf("%s is not text", key)
	}
	if required && s == "" {
		return "", fmt.Errorf("%s is empty", key)
	}
	return string(s), nil
}

func lifecycleField(o canon.Object, key string, required bool) (model.Lifecycle, error) {
	v, ok := o[key]
	if !ok {
		if required {
			return model.Lifecycle{}, fmt.Errorf("the line has no %s", key)
		}
		return model.Lifecycle{}, nil
	}
	m, ok := v.(canon.Object)
	if !ok {
		return model.Lifecycle{}, fmt.Errorf("%s is not an object", key)
	}
	var l model.Lifecycle
	for _, s := range []struct {
		field string
		to    *string
	}{{"status", (*string)(&l.Status)}, {"owner", &l.Owner}, {"review_by", &l.ReviewBy}, {"zero_by", &l.ZeroBy}, {"replaced_by", (*string)(&l.ReplacedBy)}} {
		t, err := textField(m, s.field, false)
		if err != nil {
			return model.Lifecycle{}, fmt.Errorf("%s: %w", key, err)
		}
		*s.to = t
	}
	return l, nil
}
