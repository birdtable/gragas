package layers

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// The lint file is written whole from three inputs: the shared base, the
// project's overrides and this table. The base names the places a project may
// change with a pair of comment lines,
//
//	# slot <name> begin
//	# slot <name> end
//
// and what lies between them is the base's default. The project file is a
// mapping from a slot's name to the text that takes the default's place; a slot
// the project does not name keeps the default. The slot lines are not written to
// the lint file. Three slots are filled from the table whether or not the
// project names them: the layer rows, and the Clock and program exclusions.
const (
	slotBegin = "# slot "
	slotEnd   = " end"
	slotStart = " begin"
)

// The slots the table fills. A project's text for layer-rows is the comment
// before the generated block; exclusions-generated takes no text.
const (
	SlotLayerRows  = "layer-rows"
	SlotExclusions = "exclusions-generated"
)

// ProgramsToken in a project's text is the exclusions of the programs'
// adapters, as a list of quoted globs for a depguard files: list, in the order
// the table names the programs.
const ProgramsToken = "{{programs}}"

// ErrSlots is a base and a project that do not fit: a slot that is never
// closed or named twice, or text for a slot the base does not have.
var ErrSlots = errors.New("the base and the project do not fit")

// Join is the lint file the base and the project make, with the table's blocks
// in place and filled: what gragas-dev writes to the file the table names.
func (t *Table) Join(base, project []byte) ([]byte, error) {
	text, err := projectText(project)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.LintProject, err)
	}
	lines := strings.SplitAfter(string(base), "\n")
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(lines); i++ {
		name, indent, ok := slotLine(lines[i], slotStart)
		if !ok {
			if _, _, stray := slotLine(lines[i], slotEnd); stray {
				return nil, fmt.Errorf("%s: %w: line %d ends a slot that was not begun", t.LintBase, ErrSlots, i+1)
			}
			out = append(out, lines[i])
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("%s: %w: slot %s is there twice", t.LintBase, ErrSlots, name)
		}
		seen[name] = true
		end := slices.IndexFunc(lines[i+1:], func(l string) bool { n, _, ok := slotLine(l, slotEnd); return ok && n == name })
		if end < 0 {
			return nil, fmt.Errorf("%s: %w: slot %s is never ended", t.LintBase, ErrSlots, name)
		}
		filled, err := t.fill(name, indent, lines[i+1:i+1+end], text)
		if err != nil {
			return nil, err
		}
		out = append(out, filled...)
		i += end + 1
	}
	if err := t.fits(seen, text); err != nil {
		return nil, err
	}
	return t.Lint([]byte(strings.Join(out, "")))
}

// fill is the lines a slot becomes: the project's text for it, or the base's
// default in body, with the table's blocks where the table fills the slot.
func (t *Table) fill(name, indent string, body []string, text map[string]string) ([]string, error) {
	given, named := text[name]
	switch {
	case name == SlotExclusions:
		if named {
			return nil, fmt.Errorf("%s: %w: slot %s is filled from %s and takes no text", t.LintProject, ErrSlots, name, File)
		}
		return t.exclusionMarks(indent), nil
	case name == SlotSharedCalls || name == SlotSharedImports:
		if named {
			return nil, fmt.Errorf("%s: %w: slot %s is filled from %s and takes no text", t.LintProject, ErrSlots, name, SharedFile)
		}
		if name == SlotSharedCalls {
			return []string{indent + CallsBeginMark + "\n", indent + CallsEndMark + "\n"}, nil
		}
		return []string{indent + ImportsBeginMark + "\n", indent + ImportsEndMark + "\n"}, nil
	case name == SlotLayerRows:
		lead := body
		if named {
			lead = indented(t.expand(given), indent)
		}
		return append(slices.Clone(lead), indent+BeginMark+"\n", indent+EndMark+"\n"), nil
	case named:
		return indented(t.expand(given), indent), nil
	}
	return body, nil
}

// fits holds the project's slots to the base's: each one named is in the base,
// and the four the table and the shared registry fill are there.
func (t *Table) fits(seen map[string]bool, text map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(text)) {
		if !seen[name] {
			return fmt.Errorf("%s: %w: %s names slot %s, which %s does not have", t.LintProject, ErrSlots, name, name, t.LintBase)
		}
	}
	for _, name := range []string{SlotLayerRows, SlotExclusions, SlotSharedCalls, SlotSharedImports} {
		if !seen[name] {
			return fmt.Errorf("%s: %w: it has no slot %s, which the table fills", t.LintBase, ErrSlots, name)
		}
	}
	return nil
}

// slotLine reads a slot line of the given kind: its name and its indent.
func slotLine(l, kind string) (name, indent string, ok bool) {
	trimmed := strings.TrimSpace(l)
	if !strings.HasPrefix(trimmed, slotBegin) || !strings.HasSuffix(trimmed, kind) {
		return "", "", false
	}
	name = strings.TrimSuffix(strings.TrimPrefix(trimmed, slotBegin), kind)
	if name == "" || strings.ContainsAny(name, " \t") {
		return "", "", false
	}
	return name, l[:len(l)-len(strings.TrimLeft(l, " "))], true
}

// exclusionMarks are the marker pairs of the exclusions the table fills, for
// the blocks it has: the Clock adapter's and the programs' adapters', and
// the shared owners', which is there with no row too.
func (t *Table) exclusionMarks(indent string) []string {
	var out []string
	if t.ClockAdapter() != "" {
		out = append(out, indent+ClockBeginMark+"\n", indent+ClockEndMark+"\n")
	}
	if len(t.ProgramAdapters()) > 0 {
		out = append(out, indent+ProgramsBeginMark+"\n", indent+ProgramsEndMark+"\n")
	}
	out = append(out, indent+OwnersBeginMark+"\n", indent+OwnersEndMark+"\n")
	return out
}

// expand is text with the programs' token written out.
func (t *Table) expand(text string) string {
	return strings.ReplaceAll(text, ProgramsToken, t.ProgramGlobs())
}

// ProgramGlobs are the files of the programs' adapters as depguard globs, each
// the whole package or, where the table names a file, that file alone: the
// places that may import os/exec.
func (t *Table) ProgramGlobs() string {
	var globs []string
	for _, pr := range t.Programs {
		g := "!**/" + pr.Adapter + "/**"
		if pr.File != "" {
			g = "!**/" + pr.Adapter + "/" + pr.File
		}
		if !slices.Contains(globs, g) {
			globs = append(globs, g)
		}
	}
	for i, g := range globs {
		globs[i] = "\"" + g + "\""
	}
	return strings.Join(globs, ", ")
}

// indented is text's lines, each behind indent unless it is empty.
func indented(text, indent string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = indent + l
		}
	}
	lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "\n") + "\n"
	return lines
}

// projectText reads the project file: a mapping from slot name to text.
func projectText(project []byte) (map[string]string, error) {
	v, err := ordered.ParseYAML(project)
	if err != nil {
		return nil, err
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%w: it is not a mapping of slot names to text", ErrSlots)
	}
	out := map[string]string{}
	for _, k := range m.Keys() {
		s, isText := m.Val(k).(string)
		if !isText {
			return nil, fmt.Errorf("%w: slot %s is not text", ErrSlots, k)
		}
		out[k] = s
	}
	return out, nil
}
