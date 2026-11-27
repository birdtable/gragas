package format

import (
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// The words of a names finding. None says the name, so a finding never
// prints what the list holds: it says where and nothing more. A caller tells
// a names finding by its Names mark and never by these words, which a list
// that holds one of them would have masked.
const (
	// NamesMsg is a line of a file the load read that holds a name.
	NamesMsg = "this line holds a name the names list refuses"
	// NamesValueMsg is a key of a file the load decodes (a YAML file, a
	// fenced block of guide.md, a line of history.jsonl) whose key or decoded
	// value holds a name that no one line of it holds: a value folded over two
	// lines, or written with an escape.
	NamesValueMsg = "the value of the key on this line holds a name the names list refuses"
	// NamesPathMsg is a file the load read whose path holds a name.
	NamesPathMsg = "this file's path holds a name the names list refuses"
	namesFix     = "reword it, or define a term whose literal is the name and write {term}"
)

// seen is the Tree the load reads through while a names list is held: it
// keeps each file it answers, once, so the names check reads every line of
// every file the load read and each one's path, and not the fields a reader
// picked out of them. A field the format renames, or a file a
// later part reads, is then read for names without a line of its own here.
// The load reads in one goroutine, so nothing here is locked.
//
// Two limits, both true today. A file is kept by its path alone, because one
// load reads one root; when a fetch reads a parent or an import through the
// Tree, the key becomes (root, path). And a file the load lists and never
// reads, as proposals/ skips what is not .yaml, is outside the scan, path
// included: nothing decodes it, and a name in its path reaches no line.
type seen struct {
	port.Tree
	files []model.File
	at    map[string]bool
}

func seeing(tree port.Tree) *seen {
	return &seen{Tree: tree, at: map[string]bool{}}
}

func (s *seen) Read(root model.Root, p string) ([]byte, error) {
	data, err := s.Tree.Read(root, p)
	if err == nil && !s.at[p] {
		s.at[p] = true
		s.files = append(s.files, model.File{Path: p, Data: data})
	}
	return data, err
}

// read is the files the load read, in the order it first read each.
func (s *seen) read() []model.File { return s.files }

// Literals is what the names check blanks before it reads a line, for each
// term that is not retired: its literal, the one bare name the rulebook may
// use, and its reference, {slug}. Only a defined term's reference is blanked:
// braces alone prove nothing, since a YAML flow mapping, {slug: x, title: y},
// is in braces too, and an undefined {name} is a name written down.
func Literals(terms []model.Term) []string {
	var out []string
	for _, t := range terms {
		if t.Status == model.Retired {
			continue
		}
		if t.Literal != "" {
			out = append(out, t.Literal)
		}
		out = append(out, "{"+t.Slug+"}")
	}
	return out
}

// NamesIn is the names check over one file, read with the term references and
// the literals blanked: a finding for its path when the path holds a name, one
// for each line that holds one, and, for a YAML file, one for each key whose
// key or decoded value holds a name that no line of the value holds. A line
// is not a value: a name a folded value breaks over two lines, or a quoted
// value writes with an escape, is in the value the load hands on and in no
// line. So both are read, and a place both find
// is one finding, the line's. A nil list finds nothing.
func NamesIn(n *Names, literals []string, file string, data []byte) []Finding {
	return namesInBook(n, literals, "", file, data)
}

// namesInBook is NamesIn for a file of a book whose bindings/ folder is bindings
// (a path in the tree, "" when the file is of no book's): a file below it is
// a record whatever its name.
func namesInBook(n *Names, literals []string, bindings, file string, data []byte) []Finding {
	if n == nil {
		return nil
	}
	b := blankerOf(literals)
	var out []Finding
	if n.Find(b.blank(file)) != "" {
		out = append(out, Finding{File: file, Msg: NamesPathMsg, Fix: "rename the file, or the folder that holds it", Names: true})
	}
	lines := strings.Split(string(data), "\n")
	byLine := map[int]bool{}
	for i, line := range lines {
		if n.Find(b.blank(line)) != "" {
			byLine[i+1] = true
			out = append(out, Finding{File: file, Line: i + 1, Msg: NamesMsg, Fix: namesFix, Names: true})
		}
	}
	v := valuesIn(n, b, bindings, file, data)
	for _, at := range v.held {
		if !slices.ContainsFunc(v.span(at, len(lines)), func(l int) bool { return byLine[l] }) {
			byLine[at] = true
			out = append(out, Finding{File: file, Line: at, Msg: NamesValueMsg, Fix: namesFix, Names: true})
		}
	}
	slices.SortStableFunc(out, func(a, b Finding) int { return a.Line - b.Line })
	return out
}

// values is what the names check found in the values of a YAML file: the
// line of each key whose key or value holds a name, and the line of every key,
// sorted, by which a value's lines are told.
type values struct {
	held, keys []int
}

// span is the lines of the value of the key on line at: from it up to the
// next key's line, or the file's last line.
func (v values) span(at, last int) []int {
	end := last + 1
	if i, _ := slices.BinarySearch(v.keys, at+1); i < len(v.keys) {
		end = v.keys[i]
	}
	out := make([]int, 0, max(end-at, 1))
	for l := at; l < max(end, at+1); l++ {
		out = append(out, l)
	}
	return out
}

// valuesIn reads a file as the load decodes it and holds every key and every
// string value to the list, each at its key's line: a YAML file whole, a
// guide.md one fenced gragas block at a time, history.jsonl one line at a
// time, and a file of the book's own bindings/ whole under any name. The kind
// is what the loader does with the file, and where it reads it, not its
// extension. Below bindings/ the kinds are a union, since a layer may be named
// bindings and its guide.md is then both a guide and a record: both reads are
// made and the findings merge. The load calls Tree.Stat for existence only
// and Tree.List for names, so only Read gives content. A file the load does
// not decode, or that does not parse, has no values here: the load says what
// is wrong with it, and its lines are read all the same.
func valuesIn(n *Names, b blanker, bindings, file string, data []byte) values {
	w := valueWalk{n: n, b: b}
	record := bindings != "" && strings.HasPrefix(file, bindings+"/")
	if record || path.Ext(file) == ".yaml" || path.Ext(file) == ".yml" {
		w.decode(data, 0) // a record whatever its name; a YAML file whole
	}
	switch path.Base(file) {
	case GuideFile: // also a guide below bindings/: a layer may be named so
		for _, blk := range guideBlocks(data) {
			w.decode([]byte(blk.Body), blk.Line)
			w.v.keys = append(w.v.keys, blk.EndLine) // the block's last value ends at its fence
		}
	case HistoryFile:
		for i, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				w.decode([]byte(line), i)
			}
		}
	}
	slices.Sort(w.v.keys)
	w.v.keys = slices.Compact(w.v.keys)
	slices.Sort(w.v.held)
	w.v.held = slices.Compact(w.v.held)
	return w.v
}

// guideBlocks is the closed gragas blocks of a guide, as the load reads them.
// A fence never closed is the load's finding and leaves the blocks before it;
// a guide Parse cannot read at all has none.
func guideBlocks(data []byte) []markdown.Block {
	doc, err := markdown.Parse(data)
	if _, open := errors.AsType[*markdown.Error](err); err != nil && !open {
		return nil
	}
	return gragasBlocks(doc)
}

// decode holds one YAML text to the list, which starts below line off of its
// file: a text that does not parse has no values.
func (w *valueWalk) decode(text []byte, off int) {
	doc, err := ordered.ParseYAML(text)
	if err != nil {
		return
	}
	w.off = off
	w.walk(doc, 1)
}

// valueWalk is valuesIn's walk of a decoded document: what it found so far,
// and the list and blanker it holds each key and string value to.
type valueWalk struct {
	n   *Names
	b   blanker
	v   values
	off int // the lines above the text being walked, in the file
}

func (w *valueWalk) held(s string) bool { return w.n.Find(w.b.blank(s)) != "" }

// walk holds x to the list, x being a value whose key is on line: a map's
// keys each at their own line, or line when the decoder gave none.
func (w *valueWalk) walk(x any, line int) {
	switch x := x.(type) {
	case *ordered.Map:
		for _, k := range x.Keys() {
			at := x.KeyLine(k)
			if at == 0 {
				at = line
			}
			w.v.keys = append(w.v.keys, at+w.off)
			if w.held(k) {
				w.v.held = append(w.v.held, at+w.off)
			}
			w.walk(x.Val(k), at)
		}
	case []any:
		for _, e := range x {
			w.walk(e, line)
		}
	case string:
		if w.held(x) {
			w.v.held = append(w.v.held, line+w.off)
		}
	}
}

// blanker takes the term references and the literals out of a text: each
// literal's pattern is compiled once for a file, not once for each line.
type blanker []*regexp.Regexp

func blankerOf(literals []string) blanker {
	b := make(blanker, 0, len(literals))
	for _, l := range literals {
		b = append(b, literalAt(l))
	}
	return b
}

func (b blanker) blank(text string) string {
	for _, re := range b {
		text = re.ReplaceAllString(text, " ")
	}
	return text
}

// Mask is s with each name of the list in it hidden, so no line a load
// answers says a name while a list is held: the path of a file can hold one,
// and a finding's words can quote one. A name found by its hash has no text
// to cut, so the whole of s goes. With no list it is s.
func (n *Names) Mask(s string) string {
	for range 64 {
		if n == nil || s == "" {
			return s
		}
		m := n.Find(s)
		if m == "" {
			return s
		}
		if !strings.Contains(s, m) {
			return MaskedName
		}
		s = strings.ReplaceAll(s, m, MaskedName)
	}
	return MaskedName
}
