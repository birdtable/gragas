package proof

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// PairsDir is the folder of a rulebook's root that holds the example pairs,
// pairs/<rule id>/<n>/good.* and bad.* (A56, S44).
const PairsDir = "pairs"

// MostExample is the most bytes one example file may hold. A pair is foreign
// data from a repository: one past the limit is a finding, never read whole.
const MostExample = 1 << 20

// Example is one file of a pair: its name in the pair's folder, and its whole
// content. The name is "good" or "bad" and an extension, which is the pair's
// language to the tool that reads it.
type Example struct {
	Name string
	Data []byte
}

// Pair is a rule's example pair as the rulebook holds it: a good file and a
// bad one, the target the rule says they are, and a digest of both that a run
// keys its result by.
type Pair struct {
	Rule   model.RuleID
	Number int
	Target model.TargetType
	Good   Example
	Bad    Example
	// Digest is the digest of both examples, names and content, so a cell
	// proven against one pair is not read for another. It is not the cell's
	// key, which a contract document fixes (binding and tool version).
	Digest model.Digest
}

// Finding is something wrong in the pairs of a rulebook that does not stop the
// others from loading. A pair whose good example trips the bad check is not
// one: it is a proof result (M4-07), not a load error.
type Finding struct {
	Rule model.RuleID
	// Pair is the pair's number, or 0 when the finding is about a rule or a
	// binding as a whole.
	Pair int
	Msg  string
}

// String is the finding as a line a person reads.
func (f Finding) String() string {
	if f.Pair == 0 {
		return fmt.Sprintf("%s: %s", clip(string(f.Rule)), f.Msg)
	}
	return fmt.Sprintf("%s pair %d: %s", clip(string(f.Rule)), f.Pair, f.Msg)
}

// Loaded is what LoadPairs found: the pairs that are whole, in the order of
// the rules and then the numbers, and every finding, in the same order.
type Loaded struct {
	Pairs    []Pair
	Findings []Finding
}

// side is the half of a pair a file is.
type side struct {
	name string
	data []byte
}

// folder is one pairs/<id>/<n>/ as the listing shows it, before it is read.
type folder struct {
	good, bad []string // the paths of the files named good.* and bad.*
	big       bool     // a file is over MostExample: the pair is refused
}

// LoadPairs reads pairs/<rule id>/<n>/ below root for the rules of the book.
// It never reads outside pairs/, and it reads a name only as a rule id from
// rules and a canonical positive number: a folder or file that is neither is
// a finding and is not opened. An absent pairs folder is no pairs, not an
// error. A pair with a missing or doubled side, an over-large file, a number
// its rule does not list, or a folder for no rule is a finding; a binding that
// names no pair, or one that did not load, is a finding. An error is the
// Tree's own failure to list or read.
func LoadPairs(tree port.Tree, root model.Root, rules []model.Rule) (Loaded, error) {
	entries, err := tree.List(root, PairsDir, model.Every)
	if errors.Is(err, fs.ErrNotExist) {
		entries, err = nil, nil
	}
	if err != nil {
		return Loaded{}, fmt.Errorf("list %s: %w", PairsDir, err)
	}
	known := map[model.RuleID]bool{}
	for _, r := range rules {
		known[r.ID] = true
	}
	found, out := gather(entries, known)
	for _, r := range rules {
		pairs, findings, err := loadRule(tree, root, r, found[r.ID])
		if err != nil {
			return Loaded{}, err
		}
		out.Pairs = append(out.Pairs, pairs...)
		out.Findings = append(out.Findings, findings...)
	}
	return out, nil
}

// gather sorts the listed files into folders by rule and number, and returns
// the findings of the names that are not a pair file, or are of no rule.
func gather(entries []model.Entry, known map[model.RuleID]bool) (map[model.RuleID]map[int]*folder, Loaded) {
	var out Loaded
	found := map[model.RuleID]map[int]*folder{}
	stray := map[string]bool{}
	note := func(key string, f Finding) {
		if !stray[key] {
			stray[key] = true
			out.Findings = append(out.Findings, f)
		}
	}
	for _, e := range entries {
		if e.Dir {
			continue
		}
		id, n, file, ok := split(e.Path)
		if !ok {
			note(e.Path, Finding{Msg: fmt.Sprintf("%s is not pairs/<rule id>/<n>/good.* or bad.*", clip(e.Path))})
			continue
		}
		if !known[id] {
			note(string(id), Finding{Rule: id, Msg: "a pairs folder for a rule the book does not hold"})
			continue
		}
		if found[id] == nil {
			found[id] = map[int]*folder{}
		}
		if found[id][n] == nil {
			found[id][n] = &folder{}
		}
		f := found[id][n]
		switch {
		case e.Size > MostExample:
			out.Findings = append(out.Findings, Finding{Rule: id, Pair: n, Msg: fmt.Sprintf("%s is over %d bytes", clip(file), MostExample)})
			f.big = true
		case strings.HasPrefix(file, "good."):
			f.good = append(f.good, e.Path)
		default:
			f.bad = append(f.bad, e.Path)
		}
	}
	return found, out
}

// loadRule reads the folders found for one rule, and reports its bindings.
func loadRule(tree port.Tree, root model.Root, r model.Rule, folders map[int]*folder) ([]Pair, []Finding, error) {
	listed := map[int]model.TargetType{}
	for _, p := range r.Pairs {
		listed[p.Number] = p.Target
	}
	nums := make([]int, 0, len(folders))
	for n := range folders {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	var pairs []Pair
	var findings []Finding
	loaded := map[int]bool{}
	for _, n := range nums {
		p, why, err := pairOf(tree, root, r.ID, n, folders[n], listed)
		if err != nil {
			return nil, nil, err
		}
		if why != "" {
			findings = append(findings, Finding{Rule: r.ID, Pair: n, Msg: why})
			continue
		}
		loaded[n] = true
		pairs = append(pairs, p)
	}
	for _, b := range r.Bindings {
		switch {
		case b.Pair == 0:
			findings = append(findings, Finding{Rule: r.ID, Msg: fmt.Sprintf("a binding of %s names no pair", clip(b.Key))})
		case !loaded[b.Pair]:
			findings = append(findings, Finding{Rule: r.ID, Pair: b.Pair, Msg: fmt.Sprintf("a binding of %s names this pair, which is not there whole", clip(b.Key))})
		}
	}
	return pairs, findings, nil
}

// split reads a listed path as pairs/<id>/<n>/<file>, where the file is good.
// or bad. and an extension. ok is false for any other shape.
func split(p string) (id model.RuleID, n int, file string, ok bool) {
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[0] != PairsDir || parts[1] == "" {
		return "", 0, "", false
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n < 1 || strconv.Itoa(n) != parts[2] {
		return "", 0, "", false
	}
	stem, ext, dotted := strings.Cut(parts[3], ".")
	if !dotted || ext == "" || (stem != "good" && stem != "bad") {
		return "", 0, "", false
	}
	return model.RuleID(parts[1]), n, parts[3], true
}

// pairOf reads the two files of one folder. why is "" for a whole pair, else
// the sentence of its finding; err is the Tree's failure.
func pairOf(tree port.Tree, root model.Root, id model.RuleID, n int, f *folder, listed map[int]model.TargetType) (Pair, string, error) {
	target, ok := listed[n]
	switch {
	case f.big:
		return Pair{}, "a file is too large to read", nil
	case !ok:
		return Pair{}, "the rule does not list this pair", nil
	case len(f.good) == 0 || len(f.bad) == 0:
		return Pair{}, "a pair holds one good.* and one bad.*", nil
	case len(f.good) > 1 || len(f.bad) > 1:
		return Pair{}, "a pair holds one good.* and one bad.*, and this holds more", nil
	}
	var sides [2]side
	for i, p := range [2]string{f.good[0], f.bad[0]} {
		data, err := tree.Read(root, p)
		if err != nil {
			return Pair{}, "", fmt.Errorf("read %s: %w", clip(p), err)
		}
		sides[i] = side{name: p[strings.LastIndex(p, "/")+1:], data: data}
	}
	var sum bytes.Buffer
	for _, s := range sides {
		sum.WriteString(s.name + "\x00" + strconv.Itoa(len(s.data)) + "\x00")
		sum.Write(s.data)
	}
	return Pair{
		Rule: id, Number: n, Target: target,
		Good:   Example{Name: sides[0].name, Data: sides[0].data},
		Bad:    Example{Name: sides[1].name, Data: sides[1].data},
		Digest: model.Digest(digest.Prefixed(sum.Bytes())),
	}, "", nil
}
