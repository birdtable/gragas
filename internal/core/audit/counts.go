package audit

import (
	"cmp"
	"fmt"
	"go/scanner"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// The four things whose count may not rise against the merge base: a
// directive, as DirectiveKind names it, and these three. A loosening is held
// strictly: any of its rows head has and base lacks is new, whatever the
// count.
const (
	// KindExclusion is a row of the linter config that excludes code from a
	// linter: a rule, a path, a pattern.
	KindExclusion = "linter exclusion"
	// KindSkip is a skipped test: a call of Skip, Skipf or SkipNow in a
	// _test.go file.
	KindSkip = "test skip"
	// KindLoosening is a value of the linter config that drops findings past
	// the tool's default (a Loosening): its row is the key path and the
	// value's text, so a value changed is a row the base lacks.
	KindLoosening = "loosening"
)

// Doc is one file's path and whole text, as a repository holds it at one
// revision or in its work tree.
type Doc struct {
	Path string
	Text string
}

// Counted is one counted line: what it is, where it stands and what it says.
type Counted struct {
	Kind string
	File string
	Line int
	Text string
}

// Counting is what the count reads of one tool, as the tool's plugin gives it
// (its manifest's counted; the readers row until the plugin is read): Configs
// are the names its config may have, Exclusions the key paths from a config's
// root to each list whose items exclude code from the tool, and Directive the
// word of a Go comment that silences the tool at its line, "" for none. No
// tool's paths or word is written in this package.
type Counting struct {
	Configs    []string
	Exclusions [][]string
	Loosening  []Loosening
	Directive  string
	// FoldKeys says the tool's reader takes a config's keys without their
	// case: each step of a path then matches a key in any case,
	// and two keys of one mapping that differ only in case, which the tool
	// reads as one, are a loosening row of their own.
	FoldKeys bool
}

// Loosening is a value of a tool's config that excludes more than the tool's
// default: At is its key path from the config's root.
// With Unless, the value is one row when it is there, not null, and anything
// but Unless, the tool's default: a value with no text (a mapping, a float) is
// one too, written as ordered.Repr writes it, so a spelling the tool's reader
// casts (0, "f", 0.0 for false) is never passed. With Values, the value is one
// row when its text is one of them, and a value with no text is none: right
// only for a tool that reads the value as its exact text. A value's text is a
// string as it is, a boolean as true or false, an integer in decimal, and a
// list as its items' texts in [ ] joined by ", ", each string item quoted as
// Go quotes it, so one string that holds a comma is never two items.
// A null or an absent key is no row and no error.
type Loosening struct {
	At     []string
	Values []string
	Unless *string
}

// loosens is the text of v when it is a loosening value of l, and whether it is.
func (l Loosening) loosens(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	text, ok := textOf(v)
	switch {
	case l.Unless != nil && !ok:
		return ordered.Repr(v), true
	case !ok:
		return "", false
	case l.Unless != nil:
		return text, text != *l.Unless
	}
	return text, slices.Contains(l.Values, text)
}

// textOf is the text of a value as Loosening matches it, and whether it has one.
func textOf(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case int:
		return strconv.Itoa(x), true
	case []any:
		items := make([]string, len(x))
		for i, it := range x {
			t, ok := textOf(it)
			if !ok {
				return "", false
			}
			if _, isString := it.(string); isString {
				t = strconv.Quote(t)
			}
			items[i] = t
		}
		return "[" + strings.Join(items, ", ") + "]", true
	}
	return "", false
}

// DirectiveKind is the kind a directive of word is counted as.
func DirectiveKind(word string) string { return word + " directive" }

// directiveSuffix ends each directive kind, so a kind is known for one.
const directiveSuffix = " directive"

// skipName says whether s is one of the methods of testing.T and B that skip a test.
func skipName(s string) bool { return s == "Skip" || s == "Skipf" || s == "SkipNow" }

// CountOf is the counted lines of one file: the directives of each tool when
// it is a Go file, its test skips when it is a test file, and its exclusion
// rows when it is one tool's config.
func CountOf(f Doc, tools []Counting) []Counted { return NewCounter(tools)(f) }

// NewCounter is CountOf over tools with each tool's directive pattern compiled
// once, for a caller that counts many files.
func NewCounter(tools []Counting) func(f Doc) []Counted {
	var directives []*regexp.Regexp
	var kinds []string
	for _, c := range tools {
		if c.Directive != "" {
			directives = append(directives, regexp.MustCompile(`^//\s*`+regexp.QuoteMeta(c.Directive)+`\b`))
			kinds = append(kinds, DirectiveKind(c.Directive))
		}
	}
	return func(f Doc) []Counted { return countedBy(f, tools, directives, kinds) }
}

// countedBy is the counted lines of f, with the directives of the tools
// already compiled.
func countedBy(f Doc, tools []Counting, directives []*regexp.Regexp, kinds []string) []Counted {
	base := path.Base(f.Path)
	if !strings.HasSuffix(base, ".go") {
		for _, c := range tools {
			if slices.Contains(c.Configs, base) {
				return exclusionRows(f, c)
			}
		}
		return nil
	}
	return goCounted(f, strings.HasSuffix(base, "_test.go"), directives, kinds)
}

// goCounted reads a Go file as tokens, so that a directive is a comment that
// starts with // and one of the directives' words, counted as its kind, and a
// skip is a call of Skip, Skipf or SkipNow: the words in a string or in the
// middle of a comment count for nothing. Source that does not parse is read
// as far as the scanner goes.
func goCounted(f Doc, test bool, directives []*regexp.Regexp, kinds []string) []Counted {
	lines := strings.Split(f.Text, "\n")
	fset := token.NewFileSet()
	src := []byte(f.Text)
	file := fset.AddFile(f.Path, fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, func(token.Position, string) {}, scanner.ScanComments)
	var out []Counted
	add := func(kind string, pos token.Pos) {
		line := fset.Position(pos).Line
		// A copy: the row outlives the file text, and a substring of it would hold
		// the whole text for as long as the row is kept.
		out = append(out, Counted{kind, f.Path, line, strings.Clone(strings.TrimSpace(lines[line-1]))})
	}
	var prev, prev2 token.Token
	var prevLit string
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return out
		}
		if tok == token.COMMENT {
			if i := slices.IndexFunc(directives, func(d *regexp.Regexp) bool { return d.MatchString(lit) }); i >= 0 {
				add(kinds[i], pos)
			}
		}
		switch {
		case test && tok == token.LPAREN && prev == token.IDENT && prev2 == token.PERIOD && skipName(prevLit):
			add(KindSkip, pos)
		}
		if tok != token.COMMENT {
			prev2, prev, prevLit = prev, tok, lit
		}
	}
}

// exclusionRows reads a YAML linter config as YAML: the items of the lists at
// the tool's Exclusions, in block or flow style alike, and each of its
// Loosening values that loosens, as the row "<key path>: <value's text>" at
// the key's line. A row's text is the item
// as ordered.Repr writes a mapping and as itself for a string, so the two styles
// of one row are the same row. A mapping item stands at its own line; a scalar
// item has none of its own, so it stands at the line of the next scalar of the
// document, from its list's key on, that is its text (ordered.Scalars, each match
// used once, so two equal items have a line each), and at the key's line when the
// document has no such scalar. A config in another syntax, or one that does not
// parse, has no rows.
func exclusionRows(f Doc, c Counting) []Counted {
	if !strings.HasSuffix(f.Path, ".yml") && !strings.HasSuffix(f.Path, ".yaml") {
		return nil
	}
	root, err := ordered.ParseYAML([]byte(f.Text))
	if err != nil || root == nil {
		return nil
	}
	scalars, err := ordered.Scalars([]byte(f.Text))
	if err != nil {
		scalars = nil // no line of a scalar: its row stands at its list's key
	}
	var out []Counted
	for _, l := range c.Loosening {
		for _, at := range walk(root, l.At, c.FoldKeys) {
			if text, ok := l.loosens(at.v); ok {
				out = append(out, Counted{KindLoosening, f.Path, at.line, strings.Join(l.At, ".") + ": " + strings.Clone(text)})
			}
		}
	}
	if c.FoldKeys {
		out = append(out, caseTwins(f.Path, root, nil)...)
	}
	for _, p := range c.Exclusions {
		for _, at := range walk(root, p, c.FoldKeys) {
			if items, ok := at.v.([]any); ok {
				out = append(out, itemRows(f.Path, items, at.line, scalars)...)
			}
		}
	}
	slices.SortFunc(out, func(a, b Counted) int { return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Text, b.Text)) })
	return out
}

// itemRows is an exclusion row for each item of a list whose key stands at
// line key: a mapping at its own line, a scalar at the line of the next
// scalar that is its text, from the key on.
func itemRows(file string, items []any, key int, scalars []ordered.Scalar) []Counted {
	var out []Counted
	next := 0
	for _, it := range items {
		text, line := ordered.Repr(it), key
		if m, ok := it.(*ordered.Map); ok && m.Line > 0 {
			line = m.Line
		} else if s, ok := it.(string); ok {
			text = s
			line, next = scalarLine(scalars, next, key, s)
		}
		// A copy, as goCounted's rows are: the yaml parse may hand back a piece of the text.
		out = append(out, Counted{KindExclusion, file, line, strings.Clone(text)})
	}
	return out
}

// reached is a value a path reaches and the line of its last key.
type reached struct {
	v    any
	line int
}

// walk is each value the keys p reach below root, with its key's line, none
// when the path reaches nothing. Read as written, a step matches its key alone,
// so there is one value at most. With fold, a step matches every key of its
// mapping in any case, and every match is followed: viper 1.12.0 keeps one of
// two case twins by a map's range, so either may be the one the tool reads,
// and the count reads both.
func walk(root any, p []string, fold bool) []reached {
	here := []reached{{root, 0}}
	for _, step := range p {
		var next []reached
		for _, h := range here {
			m, ok := h.v.(*ordered.Map)
			if !ok {
				continue
			}
			for _, k := range m.Keys() {
				if k == step || fold && strings.ToLower(k) == step {
					v, _ := m.Get(k)
					next = append(next, reached{v, m.KeyLine(k)})
				}
			}
		}
		here = next
	}
	return here
}

// caseTwins is a loosening row for each two keys of one mapping, anywhere
// below root, that differ only in case: a reader that lower-cases keys takes
// them as one key, so what either holds may stand in for the other, and the
// count would read only the one its path names. Each row names the folded
// path and the keys as written, at the second key's line.
func caseTwins(file string, v any, at []string) []Counted {
	var out []Counted
	switch x := v.(type) {
	case *ordered.Map:
		first := map[string]string{}
		for _, k := range x.Keys() {
			low := strings.ToLower(k)
			here := append(slices.Clone(at), low)
			if was, twice := first[low]; twice {
				out = append(out, Counted{KindLoosening, file, x.KeyLine(k), strings.Join(here, ".") + ": keys of one name to the tool, " + strings.Clone(was) + " and " + strings.Clone(k)})
			} else {
				first[low] = k
			}
			sub, _ := x.Get(k)
			out = append(out, caseTwins(file, sub, here)...)
		}
	case []any:
		for _, it := range x {
			out = append(out, caseTwins(file, it, at)...)
		}
	}
	return out
}

// scalarLine is the line of the first scalar from index next on that is s, is
// not a key and stands at or below line key, and the index after it; a scalar
// with none is at key, and next is unchanged.
func scalarLine(scalars []ordered.Scalar, next, key int, s string) (line, after int) {
	for i := next; i < len(scalars); i++ {
		if c := scalars[i]; !c.Key && c.Line >= key && c.Text == s {
			return c.Line, i + 1
		}
	}
	return key, next
}

// CheckCounts is the count check against the merge base: a count of any of
// the four kinds that is higher in head than in base is a finding for each
// line of head that base does not match, with the line, and a loosening line
// of head that base does not match is one whatever the count, since a
// changed value is a looser one as often as not (a keep-list shortened, a
// revision moved). Lines are matched by what they say and not where they
// stand, so a moved line is not a new one and a removed one lowers its
// count. Nothing is stored: base is read from the merge base every time.
func CheckCounts(rules RepoRules, base, head []Counted) []Finding {
	var out []Finding
	have := map[string]int{}
	total := map[string]int{}
	for _, c := range base {
		have[c.Kind+"\x00"+c.Text]++
		total[c.Kind]++
	}
	now := map[string]int{}
	for _, c := range head {
		now[c.Kind]++
	}
	for _, c := range head {
		if now[c.Kind] <= total[c.Kind] && c.Kind != KindLoosening {
			continue
		}
		k := c.Kind + "\x00" + c.Text
		if have[k] > 0 {
			have[k]--
			continue
		}
		out = append(out, found(rules, keyOfKind(c.Kind), c.File, c.Line,
			fmt.Sprintf("is a new %s (%d against %d at the merge base): %s", c.Kind, now[c.Kind], total[c.Kind], c.Text)))
	}
	return sorted(out)
}

// keyOfKind is the key a kind of counted line is reported under.
func keyOfKind(kind string) string {
	if strings.HasSuffix(kind, directiveSuffix) {
		return KeyDirective
	}
	return KeyDebt
}
