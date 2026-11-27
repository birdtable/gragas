package project

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// PinFormat is the newest version of the pin file this gragas reads.
const PinFormat = 1

// pinKeys are the pin's keys, in the order a missing one is named.
func pinKeys() []string { return []string{"rulebook", "source", "parts", "format"} }

// PinError is a pin refused: a Finding, its file, the line (0 for the whole
// file) and what is wrong.
type PinError struct{ Finding }

func (e *PinError) Error() string { return e.String() }

// PinNotReady is a pin written in a format newer than this gragas reads. It
// names both, and the line of the format key.
type PinNotReady struct {
	Path  string
	Line  int
	Found int64
	Reads int
}

func (e *PinNotReady) Error() string {
	return fmt.Sprintf("%s:%d: the pin is format %d, and this gragas reads pin format %d: install a gragas that reads pin format %d", e.Path, e.Line, e.Found, e.Reads, e.Found)
}

// ParsePin reads a project's pin: file is its path, for the refusals, and
// data its bytes. It is read strictly (ordered.ParseStrict: no anchor, alias,
// merge key or tag, and no key written twice), and every fault is refused as
// "path:line: message" in a *PinError. No fault answers a pin with a nil error.
//
// A format key is judged first, by ordered.Version, since a newer file's
// other keys are not this reader's to judge: a format newer than PinFormat is
// a *PinNotReady naming both, and one below 1 or not a whole number is
// refused. Then each key the pin holds beside the four is refused by name,
// the spec's version, kinds and content hash among them: release 1's reader
// does not read them. Then a missing key, format among them, which is never
// read as format 1. Then each key's value: rulebook a name, parts a list of
// at least one name, and source a folder of the repository (sourceFault).
func ParsePin(file string, data []byte) (model.Pin, error) {
	r := pinReader{file: file}
	v, err := ordered.ParseStrict(data)
	if se, ok := errors.AsType[*ordered.SyntaxError](err); ok {
		return model.Pin{}, r.refuse(se.Line, "%s", strings.TrimPrefix(se.Detail, fmt.Sprintf("line %d: ", se.Line)))
	} else if err != nil {
		return model.Pin{}, r.refuse(0, "%v", err)
	}
	if v == nil {
		return model.Pin{}, r.refuse(0, "is empty: a pin holds %s", keyList())
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return model.Pin{}, r.refuse(1, "is not a mapping: a pin holds %s", keyList())
	}
	r.doc = doc
	if err := r.keys(); err != nil {
		return model.Pin{}, err
	}
	pin, err := r.values()
	if err != nil {
		return model.Pin{}, err
	}
	return pin, nil
}

// pinReader is one pin being read: its path, for the refusals, and its
// mapping once parsed.
type pinReader struct {
	file string
	doc  *ordered.Map
}

func (r pinReader) refuse(line int, format string, args ...any) error {
	return &PinError{Finding{Path: r.file, Line: line, Msg: fmt.Sprintf(format, args...)}}
}

// line is the line of key k, or of the mapping when k is not there.
func (r pinReader) line(k string) int {
	if l := r.doc.KeyLine(k); l > 0 {
		return l
	}
	return max(r.doc.Line, 1)
}

// keys judges a present format, then the keys beside the four, then a missing
// one, in that order.
func (r pinReader) keys() error {
	if format, ok := r.doc.Get("format"); ok {
		n, is := ordered.Version(format, PinFormat)
		switch is {
		case ordered.VersionNotWhole:
			return r.refuse(r.line("format"), "format must be a whole number, %d, and it is %s", PinFormat, ordered.Repr(format))
		case ordered.VersionBelowOne:
			return r.refuse(r.line("format"), "format %d is not a pin format: write format: %d", n, PinFormat)
		case ordered.VersionNewer:
			return &PinNotReady{Path: r.file, Line: r.line("format"), Found: n, Reads: PinFormat}
		case ordered.VersionRead:
		}
	}
	for _, k := range r.doc.Keys() {
		if !slices.Contains(pinKeys(), k) {
			return r.refuse(r.line(k), "%s is not a key of the pin: pin format %d holds %s", k, PinFormat, keyList())
		}
	}
	for _, k := range pinKeys() {
		if _, ok := r.doc.Get(k); !ok {
			return r.refuse(r.line(""), "has no %s key: a pin holds %s", k, keyList())
		}
	}
	return nil
}

// values judges each key's value, once keys has passed: format is then a
// version this gragas reads.
func (r pinReader) values() (model.Pin, error) {
	rulebook, ok := r.doc.Val("rulebook").(string)
	switch {
	case !ok:
		return model.Pin{}, r.refuse(r.line("rulebook"), "rulebook must be the rulebook's name, a string, and it is %s", ordered.Repr(r.doc.Val("rulebook")))
	case strings.TrimSpace(rulebook) == "":
		return model.Pin{}, r.refuse(r.line("rulebook"), "rulebook is empty: write the rulebook's name")
	}
	source, ok := r.doc.Val("source").(string)
	if !ok {
		return model.Pin{}, r.refuse(r.line("source"), "source must be a folder of the repository, a string, and it is %s", ordered.Repr(r.doc.Val("source")))
	}
	if why := sourceFault(source); why != "" {
		return model.Pin{}, r.refuse(r.line("source"), "source %q %s", source, why)
	}
	parts, err := r.parts()
	if err != nil {
		return model.Pin{}, err
	}
	format, _ := ordered.Version(r.doc.Val("format"), PinFormat)
	return model.Pin{Rulebook: rulebook, Source: source, Parts: parts, Format: int(format)}, nil
}

// parts is the pin's list of part names: at least one, none blank.
func (r pinReader) parts() ([]string, error) {
	list, ok := r.doc.Val("parts").([]any)
	if !ok {
		return nil, r.refuse(r.line("parts"), "parts must be a list of part names, and it is %s", ordered.Repr(r.doc.Val("parts")))
	}
	if len(list) == 0 {
		return nil, r.refuse(r.line("parts"), "parts is empty: name at least one part")
	}
	parts := make([]string, 0, len(list))
	for i, p := range list {
		name, ok := p.(string)
		switch {
		case !ok:
			return nil, r.refuse(r.line("parts"), "part %d must be a part's name, a string, and it is %s", i+1, ordered.Repr(p))
		case strings.TrimSpace(name) == "":
			return nil, r.refuse(r.line("parts"), "part %d is empty: write the part's name", i+1)
		}
		parts = append(parts, name)
	}
	return parts, nil
}

// sourceFault is why source is no folder of the repository, "" when it is
// one: the one rule of the pin's source, the standard library's alone.
//
// It must pass fs.ValidPath, slash-separated with no leading or trailing
// slash and no empty, "." or ".." step, and hold no colon, which is how a git
// host and a mirror are told from a path. "." alone is the project's own
// folder. A spelling only one system gives a meaning, a backslash or a drive,
// is not judged here: the folder is opened through the Tree port, whose
// adapter refuses what leaves the root. Release 1 reads a folder of the
// repository and nothing else.
func sourceFault(source string) string {
	switch {
	case source == "":
		return "is empty: write the folder of the repository that holds the rulebook, or . for this one"
	case strings.Contains(source, ":"):
		return "is a git host or a mirror, not a path: release 1 reads a folder of the repository"
	case strings.HasPrefix(source, "/"):
		return "is absolute: write the folder relative to the project, inside the repository"
	case slices.Contains(strings.Split(source, "/"), ".."):
		return "leaves the repository: write a folder inside it, with no .. step"
	case !fs.ValidPath(source):
		return "is not a clean path: write it slash-separated, with no empty or . step and no slash at the end"
	}
	return ""
}

// keyList is the pin's four keys, for a refusal.
func keyList() string {
	keys := pinKeys()
	return strings.Join(keys[:len(keys)-1], ", ") + " and " + keys[len(keys)-1]
}
