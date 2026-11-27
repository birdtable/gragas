// Written by gragas-generate from contract/plugin/v1/manifest.schema.json; edits are lost, so edit the source and run task gen.

package plugin

import "github.com/birdtable/gragas/internal/canon"

// PresetReadback is what the tool's config cannot say of a preset entry, so the read-back of plugin test's C23 compares only what a preset can say. It only narrows the read-back side: a preset entry that is dropped or changed on read-back is red whatever this holds, and a key read back that is neither in the preset nor in defaults is red. Absent where the config holds all of an entry (C23 compares value, options and severity of each).
type PresetReadback struct {
	// Severity is false: the tool's config holds no severity, so C23 compares none, and a plugin that declares it and reads a severity back from its own preset fails C23. true is what an absent member means, and is the same as leaving it out.
	Severity *bool `json:"severity"`
	// Defaults is the keys the tool turns on by itself, which C23 leaves out of the entries it reads back and the preset does not hold, and only those. In byte order, each once; an empty list is refused, leave the key out.
	Defaults []string `json:"defaults"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PresetReadback) ToCanon() canon.Value {
	o := canon.Object{}
	if x.Severity != nil {
		o["severity"] = canon.Bool((*x.Severity))
	}
	if x.Defaults != nil {
		o["defaults"] = listOf(x.Defaults, func(x0 string) canon.Value { return canon.String(x0) })
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PresetReadback) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PresetReadback) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "severity", "defaults"); err != nil {
		return err
	}
	if len(o) < 1 {
		return fault(p, "has fewer than 1 members")
	}
	{
		m, ok := o["severity"]
		if ok {
			var s bool
			t1, err := truth(m, pointer(p, "severity"))
			if err != nil {
				return err
			}
			s = t1
			x.Severity = &s
		}
	}
	{
		m, ok := o["defaults"]
		if ok {
			a1, err := array(m, pointer(p, "defaults"), 1, 64)
			if err != nil {
				return err
			}
			x.Defaults = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "defaults"), i1), 1, 128, pattern1)
				if err != nil {
					return err
				}
				x.Defaults[i1] = s2
			}
			if err := distinct(x.Defaults, pointer(p, "defaults")); err != nil {
				return err
			}
		}
	}
	return nil
}

// ManifestPreset is a message or part of one of plugin contract 1.
type ManifestPreset struct {
	// Path is the preset file, where the engine writes what preset returns. Held to the adapter format's relative: slash-separated, no leading slash, no drive, no backslash, no step that is . or .., and only letters, digits, ., _, - and /.
	Path string `json:"path"`
	// Inputs is the files handed to preset as inputs, each held to the adapter format's relative.
	Inputs []string `json:"inputs"`
	// Mark is true: the mark stands in the first line of every preset file.
	Mark bool `json:"mark"`
	// Readback is what the tool's config cannot say of a preset entry, so the read-back of plugin test's C23 compares only what a preset can say. It only narrows the read-back side: a preset entry that is dropped or changed on read-back is red whatever this holds, and a key read back that is neither in the preset nor in defaults is red. Absent where the config holds all of an entry (C23 compares value, options and severity of each).
	Readback *PresetReadback `json:"readback"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x ManifestPreset) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = canon.String(x.Path)
	o["inputs"] = listOf(x.Inputs, func(x0 string) canon.Value { return canon.String(x0) })
	o["mark"] = canon.Bool(x.Mark)
	if x.Readback != nil {
		o["readback"] = (*x.Readback).ToCanon()
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *ManifestPreset) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *ManifestPreset) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "path", "inputs", "mark", "readback"); err != nil {
		return err
	}
	{
		m, ok := o["path"]
		if !ok {
			return fault(p, "has no member path")
		}
		s1, err := text(m, pointer(p, "path"), 0, 4096, pattern1)
		if err != nil {
			return err
		}
		if !pattern2.MatchString(s1) {
			return fault(pointer(p, "path"), "does not match %s", pattern2)
		}
		x.Path = s1
	}
	{
		m, ok := o["inputs"]
		if !ok {
			return fault(p, "has no member inputs")
		}
		a1, err := array(m, pointer(p, "inputs"), 0, 16)
		if err != nil {
			return err
		}
		x.Inputs = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "inputs"), i1), 0, 4096, pattern1)
			if err != nil {
				return err
			}
			if !pattern2.MatchString(s2) {
				return fault(index(pointer(p, "inputs"), i1), "does not match %s", pattern2)
			}
			x.Inputs[i1] = s2
		}
		if err := distinct(x.Inputs, pointer(p, "inputs")); err != nil {
			return err
		}
	}
	{
		m, ok := o["mark"]
		if !ok {
			return fault(p, "has no member mark")
		}
		t1, err := truth(m, pointer(p, "mark"))
		if err != nil {
			return err
		}
		x.Mark = t1
	}
	{
		m, ok := o["readback"]
		if ok {
			var s PresetReadback
			if err := s.read(m, pointer(p, "readback")); err != nil {
				return err
			}
			x.Readback = &s
		}
	}
	return nil
}

// WiringForm is one of the strings string, list lists.
type WiringForm string

const (
	// WiringFormString is "string".
	WiringFormString WiringForm = "string"
	// WiringFormList is "list".
	WiringFormList WiringForm = "list"
)

// Valid says whether WiringForm is one of the values.
func (e WiringForm) Valid() bool {
	switch e {
	case WiringFormString, WiringFormList:
		return true
	}
	return false
}

// WiringImport is a message or part of one of plugin contract 1.
type WiringImport struct {
	// At is where the project's config names the config it extends: a JSON pointer, a plain lookup with no expression's character and no control character.
	At   string     `json:"at"`
	Form WiringForm `json:"form"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x WiringImport) ToCanon() canon.Value {
	o := canon.Object{}
	o["at"] = canon.String(x.At)
	o["form"] = canon.String(x.Form)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *WiringImport) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *WiringImport) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "at", "form"); err != nil {
		return err
	}
	{
		m, ok := o["at"]
		if !ok {
			return fault(p, "has no member at")
		}
		s1, err := text(m, pointer(p, "at"), 0, 1024, pattern3)
		if err != nil {
			return err
		}
		if !pattern4.MatchString(s1) {
			return fault(pointer(p, "at"), "does not match %s", pattern4)
		}
		x.At = s1
	}
	{
		m, ok := o["form"]
		if !ok {
			return fault(p, "has no member form")
		}
		s1, err := text(m, pointer(p, "form"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !WiringForm(s1).Valid() {
			return fault(pointer(p, "form"), "is not one of string, list")
		}
		x.Form = WiringForm(s1)
	}
	return nil
}

// Wiring is how a project wires a preset in; absent where the tool needs neither an import nor an install.
type Wiring struct {
	Import *WiringImport `json:"import"`
	// Install is the tool's own documented install, literal words; the engine prints it and never runs it.
	Install []string `json:"install"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Wiring) ToCanon() canon.Value {
	o := canon.Object{}
	if x.Import != nil {
		o["import"] = (*x.Import).ToCanon()
	}
	if x.Install != nil {
		o["install"] = listOf(x.Install, func(x0 string) canon.Value { return canon.String(x0) })
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Wiring) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Wiring) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "import", "install"); err != nil {
		return err
	}
	if len(o) < 1 {
		return fault(p, "has fewer than 1 members")
	}
	{
		m, ok := o["import"]
		if ok {
			var s WiringImport
			if err := s.read(m, pointer(p, "import")); err != nil {
				return err
			}
			x.Import = &s
		}
	}
	{
		m, ok := o["install"]
		if ok {
			a1, err := array(m, pointer(p, "install"), 1, 64)
			if err != nil {
				return err
			}
			x.Install = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "install"), i1), 1, 256, pattern5)
				if err != nil {
					return err
				}
				x.Install[i1] = s2
			}
		}
	}
	return nil
}

// LooseningValues is a value that loosens when its text is one of values: for a tool that reads a value as its exact text.
type LooseningValues struct {
	At []string `json:"at"`
	// Values is the texts of the values at the key that loosen the tool past its default: a string as it is, true or false, an integer in decimal.
	Values []string `json:"values"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x LooseningValues) ToCanon() canon.Value {
	o := canon.Object{}
	o["at"] = listOf(x.At, func(x0 string) canon.Value { return canon.String(x0) })
	o["values"] = listOf(x.Values, func(x0 string) canon.Value { return canon.String(x0) })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *LooseningValues) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *LooseningValues) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "at", "values"); err != nil {
		return err
	}
	{
		m, ok := o["at"]
		if !ok {
			return fault(p, "has no member at")
		}
		a1, err := array(m, pointer(p, "at"), 1, 16)
		if err != nil {
			return err
		}
		x.At = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "at"), i1), 0, 128, pattern6)
			if err != nil {
				return err
			}
			x.At[i1] = s2
		}
	}
	{
		m, ok := o["values"]
		if !ok {
			return fault(p, "has no member values")
		}
		a1, err := array(m, pointer(p, "values"), 1, 16)
		if err != nil {
			return err
		}
		x.Values = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "values"), i1), 0, 32, pattern7)
			if err != nil {
				return err
			}
			x.Values[i1] = s2
		}
		if err := distinct(x.Values, pointer(p, "values")); err != nil {
			return err
		}
	}
	return nil
}

// LooseningUnless is a value that loosens whenever it is anything but the default, a value with no text too: for a tool whose reader casts a value's spelling.
type LooseningUnless struct {
	At []string `json:"at"`
	// Unless is the tool's default, as a value's text: any value there that is not null and either has no text (a mapping, a float) or whose text is anything else loosens the tool.
	Unless string `json:"unless"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x LooseningUnless) ToCanon() canon.Value {
	o := canon.Object{}
	o["at"] = listOf(x.At, func(x0 string) canon.Value { return canon.String(x0) })
	o["unless"] = canon.String(x.Unless)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *LooseningUnless) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *LooseningUnless) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "at", "unless"); err != nil {
		return err
	}
	{
		m, ok := o["at"]
		if !ok {
			return fault(p, "has no member at")
		}
		a1, err := array(m, pointer(p, "at"), 1, 16)
		if err != nil {
			return err
		}
		x.At = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "at"), i1), 0, 128, pattern6)
			if err != nil {
				return err
			}
			x.At[i1] = s2
		}
	}
	{
		m, ok := o["unless"]
		if !ok {
			return fault(p, "has no member unless")
		}
		s1, err := text(m, pointer(p, "unless"), 0, 32, pattern8)
		if err != nil {
			return err
		}
		x.Unless = s1
	}
	return nil
}

// Loosening is one of LooseningValues, LooseningUnless, by the alternative it holds; exactly one of its members is set.
type Loosening struct {
	// Values is set when the member is a LooseningValues.
	Values *LooseningValues
	// Unless is set when the member is a LooseningUnless.
	Unless *LooseningUnless
}

// ToCanon is the alternative that is set; with none set it is null, which no
// schema accepts.
func (x Loosening) ToCanon() canon.Value {
	switch {
	case x.Values != nil:
		return x.Values.ToCanon()
	case x.Unless != nil:
		return x.Unless.ToCanon()
	}
	return canon.Null{}
}

// FromCanon reads v into the alternative it is.
func (x *Loosening) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Loosening) read(v canon.Value, p string) error {
	var n int
	if a := new(LooseningValues); a.read(v, p) == nil {
		x.Values = a
		n++
	}
	if a := new(LooseningUnless); a.read(v, p) == nil {
		x.Unless = a
		n++
	}
	if n != 1 {
		*x = Loosening{}
		return fault(p, "is not exactly one of LooseningValues, LooseningUnless")
	}
	return nil
}

// Counted is what the engine's repository count reads of the tool: the lists of its config whose items exclude code from it, the values that loosen it past its default (by their texts, or unless the default), the word of a Go comment that silences it at its line, and whether its reader takes keys without their case. Absent where the tool has none of them. The engine counts the items at each path in each of the tool's config files, and in Go files each comment that is // and the word; the // and the Go file are the engine's.
type Counted struct {
	// Exclusions is key paths, from the config's root, to the lists whose every item excludes code from the tool, each path once.
	Exclusions [][]string `json:"exclusions"`
	// Loosening is values of the config that exclude more than the tool's default: each at a key path, from the config's root, each path once. With unless, a value loosens when it is there, not null, and anything but unless, a value with no text (a mapping, a float) too; with values, when its text is one of them, which is right only for a tool that reads a value as its exact text. A value's text is a string as it is, true or false, an integer in decimal, or a list as its items' texts in [ ] joined by ", ". A null or an absent key counts nothing and is no error.
	Loosening []Loosening `json:"loosening"`
	// Directive is the word that, after // at the start of a Go comment, silences the tool at its line.
	Directive *string `json:"directive"`
	// FoldKeys is whether the tool reads a config's keys without their case: each step of a path then matches a key in any case, and two keys of one mapping that differ only in case are a loosening row. Absent is false.
	FoldKeys *bool `json:"fold_keys"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Counted) ToCanon() canon.Value {
	o := canon.Object{}
	if x.Exclusions != nil {
		o["exclusions"] = listOf(x.Exclusions, func(x0 []string) canon.Value {
			return listOf(x0, func(x1 string) canon.Value { return canon.String(x1) })
		})
	}
	if x.Loosening != nil {
		o["loosening"] = listOf(x.Loosening, func(x0 Loosening) canon.Value { return x0.ToCanon() })
	}
	if x.Directive != nil {
		o["directive"] = canon.String((*x.Directive))
	}
	if x.FoldKeys != nil {
		o["fold_keys"] = canon.Bool((*x.FoldKeys))
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Counted) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Counted) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "exclusions", "loosening", "directive", "fold_keys"); err != nil {
		return err
	}
	if len(o) < 1 {
		return fault(p, "has fewer than 1 members")
	}
	{
		m, ok := o["exclusions"]
		if ok {
			a1, err := array(m, pointer(p, "exclusions"), 1, 64)
			if err != nil {
				return err
			}
			x.Exclusions = make([][]string, len(a1))
			for i1, e1 := range a1 {
				a2, err := array(e1, index(pointer(p, "exclusions"), i1), 1, 16)
				if err != nil {
					return err
				}
				x.Exclusions[i1] = make([]string, len(a2))
				for i2, e2 := range a2 {
					s3, err := text(e2, index(index(pointer(p, "exclusions"), i1), i2), 0, 128, pattern6)
					if err != nil {
						return err
					}
					x.Exclusions[i1][i2] = s3
				}
			}
		}
	}
	{
		m, ok := o["loosening"]
		if ok {
			a1, err := array(m, pointer(p, "loosening"), 1, 16)
			if err != nil {
				return err
			}
			x.Loosening = make([]Loosening, len(a1))
			for i1, e1 := range a1 {
				if err := x.Loosening[i1].read(e1, index(pointer(p, "loosening"), i1)); err != nil {
					return err
				}
			}
		}
	}
	{
		m, ok := o["directive"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "directive"), 0, 32, pattern9)
			if err != nil {
				return err
			}
			s = s1
			x.Directive = &s
		}
	}
	{
		m, ok := o["fold_keys"]
		if ok {
			var s bool
			t1, err := truth(m, pointer(p, "fold_keys"))
			if err != nil {
				return err
			}
			s = t1
			x.FoldKeys = &s
		}
	}
	return nil
}

// ProgramFound is a program found in the settings and the machine tools file alone: the pinned places, the safest form.
type ProgramFound struct {
	ID             string   `json:"id"`
	Versions       string   `json:"versions"`
	VersionArgv    []string `json:"version_argv"`
	VersionPattern *string  `json:"version_pattern"`
	CacheVars      []string `json:"cache_vars"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x ProgramFound) ToCanon() canon.Value {
	o := canon.Object{}
	o["id"] = canon.String(x.ID)
	o["versions"] = canon.String(x.Versions)
	o["version_argv"] = listOf(x.VersionArgv, func(x0 string) canon.Value { return canon.String(x0) })
	if x.VersionPattern != nil {
		o["version_pattern"] = canon.String((*x.VersionPattern))
	}
	if x.CacheVars != nil {
		o["cache_vars"] = listOf(x.CacheVars, func(x0 string) canon.Value { return canon.String(x0) })
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *ProgramFound) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *ProgramFound) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "id", "versions", "version_argv", "version_pattern", "cache_vars"); err != nil {
		return err
	}
	{
		m, ok := o["id"]
		if !ok {
			return fault(p, "has no member id")
		}
		s1, err := text(m, pointer(p, "id"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.ID = s1
	}
	{
		m, ok := o["versions"]
		if !ok {
			return fault(p, "has no member versions")
		}
		s1, err := text(m, pointer(p, "versions"), 0, 256, pattern11)
		if err != nil {
			return err
		}
		x.Versions = s1
	}
	{
		m, ok := o["version_argv"]
		if !ok {
			return fault(p, "has no member version_argv")
		}
		a1, err := array(m, pointer(p, "version_argv"), 1, 16)
		if err != nil {
			return err
		}
		x.VersionArgv = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "version_argv"), i1), 1, 256, pattern5)
			if err != nil {
				return err
			}
			x.VersionArgv[i1] = s2
		}
	}
	{
		m, ok := o["version_pattern"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "version_pattern"), 1, 256, pattern3)
			if err != nil {
				return err
			}
			s = s1
			x.VersionPattern = &s
		}
	}
	{
		m, ok := o["cache_vars"]
		if ok {
			a1, err := array(m, pointer(p, "cache_vars"), 1, 8)
			if err != nil {
				return err
			}
			x.CacheVars = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "cache_vars"), i1), 5, 64, pattern12)
				if err != nil {
					return err
				}
				x.CacheVars[i1] = s2
			}
			if err := distinct(x.CacheVars, pointer(p, "cache_vars")); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProgramAt is a program also looked for at one place below the project.
type ProgramAt struct {
	ID             string   `json:"id"`
	Versions       string   `json:"versions"`
	VersionArgv    []string `json:"version_argv"`
	VersionPattern *string  `json:"version_pattern"`
	CacheVars      []string `json:"cache_vars"`
	// Path is one place the program is looked for, below the project, with no system's suffix (host adds it); a pinned find never takes it. Held to the adapter format's relative: slash-separated, no leading slash, no drive, no backslash, no step that is . or .., and only letters, digits, ., _, - and /.
	Path string `json:"path"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x ProgramAt) ToCanon() canon.Value {
	o := canon.Object{}
	o["id"] = canon.String(x.ID)
	o["versions"] = canon.String(x.Versions)
	o["version_argv"] = listOf(x.VersionArgv, func(x0 string) canon.Value { return canon.String(x0) })
	if x.VersionPattern != nil {
		o["version_pattern"] = canon.String((*x.VersionPattern))
	}
	if x.CacheVars != nil {
		o["cache_vars"] = listOf(x.CacheVars, func(x0 string) canon.Value { return canon.String(x0) })
	}
	o["path"] = canon.String(x.Path)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *ProgramAt) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *ProgramAt) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "id", "versions", "version_argv", "version_pattern", "cache_vars", "path"); err != nil {
		return err
	}
	{
		m, ok := o["id"]
		if !ok {
			return fault(p, "has no member id")
		}
		s1, err := text(m, pointer(p, "id"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.ID = s1
	}
	{
		m, ok := o["versions"]
		if !ok {
			return fault(p, "has no member versions")
		}
		s1, err := text(m, pointer(p, "versions"), 0, 256, pattern11)
		if err != nil {
			return err
		}
		x.Versions = s1
	}
	{
		m, ok := o["version_argv"]
		if !ok {
			return fault(p, "has no member version_argv")
		}
		a1, err := array(m, pointer(p, "version_argv"), 1, 16)
		if err != nil {
			return err
		}
		x.VersionArgv = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "version_argv"), i1), 1, 256, pattern5)
			if err != nil {
				return err
			}
			x.VersionArgv[i1] = s2
		}
	}
	{
		m, ok := o["version_pattern"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "version_pattern"), 1, 256, pattern3)
			if err != nil {
				return err
			}
			s = s1
			x.VersionPattern = &s
		}
	}
	{
		m, ok := o["cache_vars"]
		if ok {
			a1, err := array(m, pointer(p, "cache_vars"), 1, 8)
			if err != nil {
				return err
			}
			x.CacheVars = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "cache_vars"), i1), 5, 64, pattern12)
				if err != nil {
					return err
				}
				x.CacheVars[i1] = s2
			}
			if err := distinct(x.CacheVars, pointer(p, "cache_vars")); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["path"]
		if !ok {
			return fault(p, "has no member path")
		}
		s1, err := text(m, pointer(p, "path"), 0, 4096, pattern1)
		if err != nil {
			return err
		}
		if !pattern2.MatchString(s1) {
			return fault(pointer(p, "path"), "does not match %s", pattern2)
		}
		x.Path = s1
	}
	return nil
}

// ProgramScript is an interpreted script a package manifest names, run by its interpreter with the script first; all three fields together.
type ProgramScript struct {
	ID             string   `json:"id"`
	Versions       string   `json:"versions"`
	VersionArgv    []string `json:"version_argv"`
	VersionPattern *string  `json:"version_pattern"`
	CacheVars      []string `json:"cache_vars"`
	// Manifest is an interpreted script: the package manifest that names it, relative to the project and never under a .bin folder (the opener holds the folder). Held to the adapter format's relative: slash-separated, no leading slash, no drive, no backslash, no step that is . or .., and only letters, digits, ., _, - and /.
	Manifest string `json:"manifest"`
	// ScriptAt is an interpreted script: the JSON pointer of its path in that manifest. The value there is a string, relative to the manifest's folder, and stays inside it by its spelling and through links; the schema cannot see that value, so the opener holds it. A JSON pointer, a plain lookup with no expression's character and no control character.
	ScriptAt string `json:"script_at"`
	// Interpreter is an interpreted script: the program that runs it; the engine puts the script first.
	Interpreter string `json:"interpreter"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x ProgramScript) ToCanon() canon.Value {
	o := canon.Object{}
	o["id"] = canon.String(x.ID)
	o["versions"] = canon.String(x.Versions)
	o["version_argv"] = listOf(x.VersionArgv, func(x0 string) canon.Value { return canon.String(x0) })
	if x.VersionPattern != nil {
		o["version_pattern"] = canon.String((*x.VersionPattern))
	}
	if x.CacheVars != nil {
		o["cache_vars"] = listOf(x.CacheVars, func(x0 string) canon.Value { return canon.String(x0) })
	}
	o["manifest"] = canon.String(x.Manifest)
	o["script_at"] = canon.String(x.ScriptAt)
	o["interpreter"] = canon.String(x.Interpreter)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *ProgramScript) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *ProgramScript) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "id", "versions", "version_argv", "version_pattern", "cache_vars", "manifest", "script_at", "interpreter"); err != nil {
		return err
	}
	{
		m, ok := o["id"]
		if !ok {
			return fault(p, "has no member id")
		}
		s1, err := text(m, pointer(p, "id"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.ID = s1
	}
	{
		m, ok := o["versions"]
		if !ok {
			return fault(p, "has no member versions")
		}
		s1, err := text(m, pointer(p, "versions"), 0, 256, pattern11)
		if err != nil {
			return err
		}
		x.Versions = s1
	}
	{
		m, ok := o["version_argv"]
		if !ok {
			return fault(p, "has no member version_argv")
		}
		a1, err := array(m, pointer(p, "version_argv"), 1, 16)
		if err != nil {
			return err
		}
		x.VersionArgv = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "version_argv"), i1), 1, 256, pattern5)
			if err != nil {
				return err
			}
			x.VersionArgv[i1] = s2
		}
	}
	{
		m, ok := o["version_pattern"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "version_pattern"), 1, 256, pattern3)
			if err != nil {
				return err
			}
			s = s1
			x.VersionPattern = &s
		}
	}
	{
		m, ok := o["cache_vars"]
		if ok {
			a1, err := array(m, pointer(p, "cache_vars"), 1, 8)
			if err != nil {
				return err
			}
			x.CacheVars = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "cache_vars"), i1), 5, 64, pattern12)
				if err != nil {
					return err
				}
				x.CacheVars[i1] = s2
			}
			if err := distinct(x.CacheVars, pointer(p, "cache_vars")); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["manifest"]
		if !ok {
			return fault(p, "has no member manifest")
		}
		s1, err := text(m, pointer(p, "manifest"), 0, 4096, pattern1)
		if err != nil {
			return err
		}
		if !pattern2.MatchString(s1) {
			return fault(pointer(p, "manifest"), "does not match %s", pattern2)
		}
		x.Manifest = s1
	}
	{
		m, ok := o["script_at"]
		if !ok {
			return fault(p, "has no member script_at")
		}
		s1, err := text(m, pointer(p, "script_at"), 0, 1024, pattern3)
		if err != nil {
			return err
		}
		if !pattern13.MatchString(s1) {
			return fault(pointer(p, "script_at"), "does not match %s", pattern13)
		}
		x.ScriptAt = s1
	}
	{
		m, ok := o["interpreter"]
		if !ok {
			return fault(p, "has no member interpreter")
		}
		s1, err := text(m, pointer(p, "interpreter"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.Interpreter = s1
	}
	return nil
}

// Program is one of ProgramFound, ProgramAt, ProgramScript, by the alternative it holds; exactly one of its members is set.
type Program struct {
	// Found is set when the member is a ProgramFound.
	Found *ProgramFound
	// At is set when the member is a ProgramAt.
	At *ProgramAt
	// Script is set when the member is a ProgramScript.
	Script *ProgramScript
}

// ToCanon is the alternative that is set; with none set it is null, which no
// schema accepts.
func (x Program) ToCanon() canon.Value {
	switch {
	case x.Found != nil:
		return x.Found.ToCanon()
	case x.At != nil:
		return x.At.ToCanon()
	case x.Script != nil:
		return x.Script.ToCanon()
	}
	return canon.Null{}
}

// FromCanon reads v into the alternative it is.
func (x *Program) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Program) read(v canon.Value, p string) error {
	var n int
	if a := new(ProgramFound); a.read(v, p) == nil {
		x.Found = a
		n++
	}
	if a := new(ProgramAt); a.read(v, p) == nil {
		x.At = a
		n++
	}
	if a := new(ProgramScript); a.read(v, p) == nil {
		x.Script = a
		n++
	}
	if n != 1 {
		*x = Program{}
		return fault(p, "is not exactly one of ProgramFound, ProgramAt, ProgramScript")
	}
	return nil
}

// Manifest is a message or part of one of plugin contract 1.
type Manifest struct {
	// Needs is features beyond the base; contract 1 starts with none. An engine without one refuses the plugin at open (ErrFeature). The features: cache-vars, a grant's cache_vars; a manifest that holds cache_vars needs it, and one that needs it holds cache_vars in a grant, or the opener refuses it.
	Needs []string `json:"needs"`
	// Name is the bundle's folder.
	Name string `json:"name"`
	// Version is the plugin's own version, its folder below the name: three whole numbers with no leading zero, so semver orders it.
	Version string `json:"version"`
	// Tool is the name a binding uses.
	Tool string `json:"tool"`
	// ToolVersions is the tool versions served: one comparator, or a lower and an upper bound. It judges the grant whose id is tool, and no other; the opener refuses a manifest with no such grant.
	ToolVersions string   `json:"tool_versions"`
	Targets      []Target `json:"targets"`
	// Provides is at least one; an op outside it is answered unsupported (C15).
	Provides []Op `json:"provides"`
	// Extends is false: the preset is copied once.
	Extends bool `json:"extends"`
	// Warning is false: the tool has no warning level.
	Warning bool `json:"warning"`
	// ConfigLoadsCode is true: the tool's config can load code, a plugins key of a data config among them, so the engine reads every config of the tool at the sandboxed level, whatever its extension. It only tightens. Absent is false: a .json, .yml or .yaml config is read isolated, any other sandboxed.
	ConfigLoadsCode *bool           `json:"config_loads_code"`
	Preset          *ManifestPreset `json:"preset"`
	// Wiring is how a project wires a preset in; absent where the tool needs neither an import nor an install.
	Wiring *Wiring `json:"wiring"`
	// ResolveFrom is the folder, relative to the config set, that a run links into the scratch folder so the tool resolves what the config imports. A folder of the project the tool then loads code from: a trust input of its own, shown at approval Held to the adapter format's relative: slash-separated, no leading slash, no drive, no backslash, no step that is . or .., and only letters, digits, ., _, - and /.
	ResolveFrom *string `json:"resolve_from"`
	// Counted is what the engine's repository count reads of the tool: the lists of its config whose items exclude code from it, the values that loosen it past its default (by their texts, or unless the default), the word of a Go comment that silences it at its line, and whether its reader takes keys without their case. Absent where the tool has none of them. The engine counts the items at each path in each of the tool's config files, and in Go files each comment that is // and the word; the // and the Go file are the engine's.
	Counted *Counted `json:"counted"`
	// Programs is the grants: every program the plugin may ever ask the engine to run. No two alike; an id given twice is the opener's to refuse, since a screen that shows the first and a run that takes the last is the hole.
	Programs []Program `json:"programs"`
	// Wasm is the digest of plugin.wasm.
	Wasm string `json:"wasm"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Manifest) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = canon.Int(1)
	o["needs"] = listOf(x.Needs, func(x0 string) canon.Value { return canon.String(x0) })
	o["name"] = canon.String(x.Name)
	o["version"] = canon.String(x.Version)
	o["tool"] = canon.String(x.Tool)
	o["tool_versions"] = canon.String(x.ToolVersions)
	o["targets"] = listOf(x.Targets, func(x0 Target) canon.Value { return canon.String(x0) })
	o["provides"] = listOf(x.Provides, func(x0 Op) canon.Value { return canon.String(x0) })
	o["extends"] = canon.Bool(x.Extends)
	o["warning"] = canon.Bool(x.Warning)
	if x.ConfigLoadsCode != nil {
		o["config_loads_code"] = canon.Bool((*x.ConfigLoadsCode))
	}
	if x.Preset != nil {
		o["preset"] = (*x.Preset).ToCanon()
	}
	if x.Wiring != nil {
		o["wiring"] = (*x.Wiring).ToCanon()
	}
	if x.ResolveFrom != nil {
		o["resolve_from"] = canon.String((*x.ResolveFrom))
	}
	if x.Counted != nil {
		o["counted"] = (*x.Counted).ToCanon()
	}
	o["programs"] = listOf(x.Programs, func(x0 Program) canon.Value { return x0.ToCanon() })
	o["wasm"] = canon.String(x.Wasm)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Manifest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Manifest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "contract", "needs", "name", "version", "tool", "tool_versions", "targets", "provides", "extends", "warning", "config_loads_code", "preset", "wiring", "resolve_from", "counted", "programs", "wasm"); err != nil {
		return err
	}
	{
		m, ok := o["contract"]
		if !ok {
			return fault(p, "has no member contract")
		}
		if m != canon.Int(1) {
			return fault(pointer(p, "contract"), "is not 1")
		}
	}
	{
		m, ok := o["needs"]
		if !ok {
			return fault(p, "has no member needs")
		}
		a1, err := array(m, pointer(p, "needs"), 0, 64)
		if err != nil {
			return err
		}
		x.Needs = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "needs"), i1), 0, 64, pattern14)
			if err != nil {
				return err
			}
			x.Needs[i1] = s2
		}
		if err := distinct(x.Needs, pointer(p, "needs")); err != nil {
			return err
		}
	}
	{
		m, ok := o["name"]
		if !ok {
			return fault(p, "has no member name")
		}
		s1, err := text(m, pointer(p, "name"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.Name = s1
	}
	{
		m, ok := o["version"]
		if !ok {
			return fault(p, "has no member version")
		}
		s1, err := text(m, pointer(p, "version"), 0, 256, pattern15)
		if err != nil {
			return err
		}
		x.Version = s1
	}
	{
		m, ok := o["tool"]
		if !ok {
			return fault(p, "has no member tool")
		}
		s1, err := text(m, pointer(p, "tool"), 0, 64, pattern10)
		if err != nil {
			return err
		}
		x.Tool = s1
	}
	{
		m, ok := o["tool_versions"]
		if !ok {
			return fault(p, "has no member tool_versions")
		}
		s1, err := text(m, pointer(p, "tool_versions"), 0, 256, pattern11)
		if err != nil {
			return err
		}
		x.ToolVersions = s1
	}
	{
		m, ok := o["targets"]
		if !ok {
			return fault(p, "has no member targets")
		}
		a1, err := array(m, pointer(p, "targets"), 1, -1)
		if err != nil {
			return err
		}
		x.Targets = make([]Target, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "targets"), i1), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Target(s2).Valid() {
				return fault(index(pointer(p, "targets"), i1), "is not one of source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry-registry, dependency, package")
			}
			x.Targets[i1] = Target(s2)
		}
		if err := distinct(x.Targets, pointer(p, "targets")); err != nil {
			return err
		}
	}
	{
		m, ok := o["provides"]
		if !ok {
			return fault(p, "has no member provides")
		}
		a1, err := array(m, pointer(p, "provides"), 1, -1)
		if err != nil {
			return err
		}
		x.Provides = make([]Op, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "provides"), i1), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Op(s2).Valid() {
				return fault(index(pointer(p, "provides"), i1), "is not one of plan, entries, findings, rank, preset")
			}
			x.Provides[i1] = Op(s2)
		}
		if err := distinct(x.Provides, pointer(p, "provides")); err != nil {
			return err
		}
	}
	{
		m, ok := o["extends"]
		if !ok {
			return fault(p, "has no member extends")
		}
		t1, err := truth(m, pointer(p, "extends"))
		if err != nil {
			return err
		}
		x.Extends = t1
	}
	{
		m, ok := o["warning"]
		if !ok {
			return fault(p, "has no member warning")
		}
		t1, err := truth(m, pointer(p, "warning"))
		if err != nil {
			return err
		}
		x.Warning = t1
	}
	{
		m, ok := o["config_loads_code"]
		if ok {
			var s bool
			t1, err := truth(m, pointer(p, "config_loads_code"))
			if err != nil {
				return err
			}
			s = t1
			x.ConfigLoadsCode = &s
		}
	}
	{
		m, ok := o["preset"]
		if ok {
			var s ManifestPreset
			if err := s.read(m, pointer(p, "preset")); err != nil {
				return err
			}
			x.Preset = &s
		}
	}
	{
		m, ok := o["wiring"]
		if ok {
			var s Wiring
			if err := s.read(m, pointer(p, "wiring")); err != nil {
				return err
			}
			x.Wiring = &s
		}
	}
	{
		m, ok := o["resolve_from"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "resolve_from"), 0, 4096, pattern1)
			if err != nil {
				return err
			}
			if !pattern2.MatchString(s1) {
				return fault(pointer(p, "resolve_from"), "does not match %s", pattern2)
			}
			s = s1
			x.ResolveFrom = &s
		}
	}
	{
		m, ok := o["counted"]
		if ok {
			var s Counted
			if err := s.read(m, pointer(p, "counted")); err != nil {
				return err
			}
			x.Counted = &s
		}
	}
	{
		m, ok := o["programs"]
		if !ok {
			return fault(p, "has no member programs")
		}
		a1, err := array(m, pointer(p, "programs"), 1, 16)
		if err != nil {
			return err
		}
		x.Programs = make([]Program, len(a1))
		for i1, e1 := range a1 {
			if err := x.Programs[i1].read(e1, index(pointer(p, "programs"), i1)); err != nil {
				return err
			}
		}
		if err := distinctValues(x.Programs, pointer(p, "programs"), func(e Program) canon.Value { return e.ToCanon() }); err != nil {
			return err
		}
	}
	{
		m, ok := o["wasm"]
		if !ok {
			return fault(p, "has no member wasm")
		}
		s1, err := text(m, pointer(p, "wasm"), 0, 71, pattern16)
		if err != nil {
			return err
		}
		x.Wasm = s1
	}
	return nil
}
