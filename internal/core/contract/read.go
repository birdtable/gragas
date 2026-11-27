package contract

import (
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Schemas is what a document from outside is checked against.
type Schemas struct {
	// Check is the Schema port compiled over SchemaDir, which names each
	// schema by its path below it.
	Check port.Schema
	// Files is the Assets port that holds SchemaDir, read for the
	// deprecation marks, which a schema check does not answer.
	Files port.Assets
	// Cap is the most bytes a document may hold, and it can only narrow: zero,
	// or one over MaxDocument, is MaxDocument. A test sets it low to hold the
	// cap at a size it can write.
	Cap int
}

// limit is the most bytes a document may hold under s: MaxDocument, or the
// Cap of s when that is smaller, so a Cap never widens the reader.
func limit(s Schemas) int {
	if s.Cap > 0 && s.Cap < MaxDocument {
		return s.Cap
	}
	return MaxDocument
}

// read is the reading every document from outside takes, in this order: the
// size cap, canon's decode, the contract version it names, the schema of that
// version, and the deprecation windows. It answers the value and the version
// with every finding; a value with a finding is not to be used. A document in
// a newer major is *NotReady, and a port that fails is an error.
func read(k Kind, data []byte, s Schemas) (canon.Object, Version, []model.Finding, error) {
	if !k.known() {
		return nil, Version{}, nil, fmt.Errorf("%q is not a document of the contract", clip(string(k)))
	}
	limit := limit(s)
	if len(data) > limit {
		return nil, Version{}, []model.Finding{{
			Entity: string(k),
			Msg:    fmt.Sprintf("the %s is %d bytes, more than the %d a document may hold, so it is not read", k, len(data), limit),
		}}, nil
	}
	v, err := canon.Decode(data)
	if err != nil {
		var ce *canon.Error
		if !errors.As(err, &ce) {
			return nil, Version{}, nil, fmt.Errorf("decode the %s: %w", k, err)
		}
		return nil, Version{}, []model.Finding{{
			Entity: string(k),
			Msg:    fmt.Sprintf("the %s is not one JSON value every reader reads alike (%s at byte %d)", k, ce.Kind, ce.Offset),
			Fix:    "write it as JSON with no repeated key, valid UTF-8 and numbers no larger than 2^53-1",
		}}, nil
	}
	obj, isObject := v.(canon.Object)
	if !isObject {
		return nil, Version{}, []model.Finding{{Entity: string(k), Msg: fmt.Sprintf("the %s is not a JSON object", k)}}, nil
	}
	ver, bad := named(k, obj)
	if bad != nil {
		return nil, Version{}, []model.Finding{*bad}, nil
	}
	cur := Current()
	if ver.Major > cur.Major {
		return nil, ver, nil, &NotReady{Kind: k, Found: ver, Reads: cur}
	}
	if oldest := max(1, cur.Major-1); ver.Major < oldest {
		return nil, ver, []model.Finding{{
			Entity: string(k),
			Msg:    fmt.Sprintf("the %s names contract %s, older than %d.0, the oldest major this engine reads", k, ver, oldest),
			Fix:    "write it again with an engine that reads it, in the current contract",
		}}, nil
	}
	body, err := canon.Encode(obj, canon.Document)
	if err != nil {
		return nil, ver, nil, fmt.Errorf("encode the %s for its schema: %w", k, err)
	}
	problems, err := s.Check.Check(k.schema(ver.Major), body)
	if err != nil {
		return nil, ver, nil, fmt.Errorf("check the %s against %s: %w", k, k.schema(ver.Major), err)
	}
	var out []model.Finding
	for _, p := range problems {
		out = append(out, model.Finding{Entity: string(k), Msg: p.Msg})
	}
	old, err := deprecated(k, obj, ver, s.Files)
	if err != nil {
		return nil, ver, nil, err
	}
	return obj, ver, append(out, old...), nil
}

// named is the contract version obj names in its contract field, or the
// finding that says why it names none.
func named(k Kind, obj canon.Object) (Version, *model.Finding) {
	f := &model.Finding{Entity: string(k), Fix: fmt.Sprintf("name the contract the %s is written in, such as \"contract\": %q", k, Current().String())}
	raw, present := obj["contract"]
	if !present {
		f.Msg = fmt.Sprintf("the %s names no contract version: it has no contract field", k)
		return Version{}, f
	}
	text, isString := raw.(canon.String)
	if !isString {
		f.Msg = fmt.Sprintf("the %s's contract field is not a string", k)
		return Version{}, f
	}
	ver, err := ParseVersion(string(text))
	if err != nil {
		f.Msg = fmt.Sprintf("the %s's contract field: %v", k, err)
		return Version{}, f
	}
	return ver, nil
}
