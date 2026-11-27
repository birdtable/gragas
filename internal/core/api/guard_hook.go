package coreapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// MaxHookInput is the most of the hook's input guard edit reads, in bytes. A
// longer input is refused, never cut and read in part.
const MaxHookInput = 16 << 20

// HookDrain is the most guard edit waits, after it refused the hook's input
// for its length, while it reads the rest of that input away, so the host's
// write ends rather than fail on a closed pipe. Past it the guard ends
// anyway: a stuck pipe does not hold the refusal.
const HookDrain = 10 * time.Second

// HookEdit is one proposed edit as the agent hook gives it: the tool, the
// file, and what the tool would write. It is data from outside (agents.guards.8):
// only Tool and Path decide, and only by the forms below.
type HookEdit struct {
	// Tool is the tool the agent calls: Write, Edit or MultiEdit.
	Tool string
	// Path is the file, an absolute path as the host spells it.
	Path string
	// Content is the whole new file, for Write.
	Content *string
	// Edits are the replacements, for Edit (one) and MultiEdit (each).
	Edits []HookReplace
}

// HookReplace is one replacement of Edit or MultiEdit.
type HookReplace struct {
	Old, New string
	All      bool
}

// hookInput is the hook protocol's document: the tool's name and input. Every
// other field the hook sends is read past.
type hookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type hookReplace struct {
	Old *string `json:"old_string"`
	New *string `json:"new_string"`
	All bool    `json:"replace_all"`
}

type toolInput struct {
	FilePath string        `json:"file_path"`
	Content  *string       `json:"content"`
	Old      *string       `json:"old_string"`
	New      *string       `json:"new_string"`
	All      bool          `json:"replace_all"`
	Edits    []hookReplace `json:"edits"`
}

// ParseHookEdit reads one proposed edit from the hook's input. Anything but
// one JSON document naming Write, Edit or MultiEdit with a file and its
// content or replacements is an error: the guard refuses what it cannot read.
// A key that is repeated in an object is an error too (oneKeyEach).
func ParseHookEdit(input []byte) (HookEdit, error) {
	if len(input) > MaxHookInput {
		return HookEdit{}, fmt.Errorf("the input is %d bytes, more than the %d read", len(input), MaxHookInput)
	}
	d := json.NewDecoder(bytes.NewReader(input))
	var in hookInput
	if err := d.Decode(&in); err != nil {
		return HookEdit{}, fmt.Errorf("the input is not one JSON document: %w", err)
	}
	if rest := bytes.Trim(input[d.InputOffset():], " \t\r\n"); len(rest) != 0 {
		return HookEdit{}, errors.New("the input holds more than one JSON document")
	}
	if err := oneKeyEach(input); err != nil {
		return HookEdit{}, err
	}
	var ti toolInput
	if err := json.Unmarshal(in.ToolInput, &ti); err != nil || len(in.ToolInput) == 0 {
		return HookEdit{}, fmt.Errorf("the tool's input is not a JSON object: %w", errors.Join(err, errors.New("tool_input")))
	}
	if ti.FilePath == "" {
		return HookEdit{}, errors.New("the tool's input names no file_path")
	}
	return editOf(in.ToolName, ti)
}

// editOf is the edit tool makes with its input ti: a Write's content, an
// Edit's replacement or a MultiEdit's each, every one of them whole.
func editOf(tool string, ti toolInput) (HookEdit, error) {
	e := HookEdit{Tool: tool, Path: ti.FilePath}
	switch tool {
	case "Write":
		if ti.Content == nil {
			return HookEdit{}, errors.New("a Write with no content")
		}
		e.Content = ti.Content
	case "Edit":
		if ti.Old == nil || ti.New == nil {
			return HookEdit{}, errors.New("an Edit with no old_string or new_string")
		}
		e.Edits = []HookReplace{{Old: *ti.Old, New: *ti.New, All: ti.All}}
	case "MultiEdit":
		if len(ti.Edits) == 0 {
			return HookEdit{}, errors.New("a MultiEdit with no edits")
		}
		for i, r := range ti.Edits {
			if r.Old == nil || r.New == nil {
				return HookEdit{}, fmt.Errorf("edit %d of the MultiEdit has no old_string or new_string", i+1)
			}
			e.Edits = append(e.Edits, HookReplace{Old: *r.Old, New: *r.New, All: r.All})
		}
	default:
		return HookEdit{}, fmt.Errorf("the tool %q is not one guard edit reads (Write, Edit, MultiEdit)", tool)
	}
	return e, nil
}

// Applied is the file's content once e is made on old, as the tool would make
// it: a Write's content whole, or each replacement in turn, where an old
// string that is missing, empty or, without replace_all, found more than once
// is an error, since the tool would refuse it too. So is an old string the
// file holds before a line's end, replaced by nothing, which the agent's host
// takes out with that end: the guard would judge another edit.
func (e HookEdit) Applied(old []byte, exists bool) ([]byte, error) {
	if e.Content != nil {
		return []byte(*e.Content), nil
	}
	if !exists {
		return nil, errors.New("an edit of a file that does not exist")
	}
	s := bytes.Clone(old)
	for i, r := range e.Edits {
		o := []byte(r.Old)
		n := bytes.Count(s, o)
		switch {
		case r.Old == "":
			return nil, fmt.Errorf("replacement %d has an empty old_string", i+1)
		case n == 0:
			return nil, fmt.Errorf("replacement %d's old_string is not in the file", i+1)
		case n > 1 && !r.All:
			return nil, fmt.Errorf("replacement %d's old_string is in the file %d times, and replace_all is not set", i+1, n)
		case r.New == "" && !bytes.HasSuffix(o, []byte("\n")) && (bytes.Contains(s, slices.Concat(o, []byte("\n"))) || bytes.Contains(s, slices.Concat(o, []byte("\r\n")))):
			// the agent's host takes the line's end out with such an old
			// string, and joins the line below to the one above: the guard
			// would judge another edit
			return nil, fmt.Errorf("replacement %d takes out an old_string the file holds before a line's end, with nothing in its place, which the tool may take out with that end; name the line's end in old_string", i+1)
		case r.All:
			s = bytes.ReplaceAll(s, o, []byte(r.New))
		default:
			s = bytes.Replace(s, o, []byte(r.New), 1)
		}
	}
	return s, nil
}

// oneKeyEach is an error when an object of the document holds a key twice,
// or a key the guard reads spelled in another case. A reader that takes the
// first value and one that takes the last disagree about the same bytes, so
// the guard judges neither. It reads only a document
// that has decoded whole, so its tokens end with its last closing delimiter.
func oneKeyEach(input []byte) error {
	d := json.NewDecoder(bytes.NewReader(input))
	var open []*keyed // nil for an array
	for {
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("the input is not one JSON document: %w", err)
		}
		if n := len(open); n > 0 {
			if err := open[n-1].take(tok); err != nil {
				return err
			}
		}
		if open = nest(open, tok); len(open) == 0 {
			return nil
		}
	}
}

// keyed is one open object of the document: the keys it has held, and
// whether its next token is a key. A nil keyed is an open array.
type keyed struct {
	keys    map[string]bool
	wantKey bool
}

// take reads tok inside o: a key where a key is due, held to oneKeyEach's
// terms, and otherwise a value, after which a key is due.
func (o *keyed) take(tok json.Token) error {
	if o == nil {
		return nil
	}
	k, ok := tok.(string)
	if !ok || !o.wantKey {
		o.wantKey = true
		return nil
	}
	if o.keys[k] {
		return fmt.Errorf("the input holds the key %q twice", k)
	}
	// encoding/json matches a key to its field in any case, so a key that
	// folds to one the guard reads and is spelled otherwise is read as it.
	for _, h := range [...]string{"tool_name", "tool_input", "file_path", "content", "old_string", "new_string", "replace_all", "edits"} {
		if k != h && bytes.EqualFold([]byte(k), []byte(h)) {
			return fmt.Errorf("the input spells the key %q as %q", h, k)
		}
	}
	o.keys[k], o.wantKey = true, false
	return nil
}

// nest is open once tok is read: an object or an array opened or closed.
func nest(open []*keyed, tok json.Token) []*keyed {
	switch tok {
	case json.Delim('{'):
		return append(open, &keyed{keys: map[string]bool{}, wantKey: true})
	case json.Delim('['):
		return append(open, nil)
	case json.Delim('}'), json.Delim(']'):
		return open[:len(open)-1]
	}
	return open
}
