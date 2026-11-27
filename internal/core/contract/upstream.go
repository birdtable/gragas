package contract

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	mc "github.com/birdtable/gragas/internal/model/contract"
)

// UpstreamVersion is the version of the flattened upstream set this engine
// reads. M9's upgrade writes the same shape (the architecture page's file
// list), so the two read one document.
const UpstreamVersion = 1

// MaxUpstream caps a flattened set file, read unparsed past it never.
const MaxUpstream = 1 << 20

// UpstreamFile is the path of a tool's flattened set below the rulebook's
// folder. A missing file means no upstream set for the tool, never an error.
func UpstreamFile(tool string) string { return "upstream/" + tool + ".flat.json" }

// UpstreamNotReady is a file in a newer version than this engine reads.
type UpstreamNotReady struct{ Found, Reads int }

func (e *UpstreamNotReady) Error() string {
	return fmt.Sprintf("the flattened upstream set is version %d and this engine reads version %d", e.Found, e.Reads)
}

// ReadUpstream reads data, the flattened set of tool from a tree the engine
// did not write (it can come from a fetched parent's bundle), failing closed,
// in this order, and the first step that refuses ends it:
//
//  1. the size cap, MaxUpstream;
//  2. canon's decode;
//  3. the version, read off the object before any other member, since a newer
//     file is described by a schema this engine does not have: a version
//     newer than this engine reads is *UpstreamNotReady, whatever else it holds;
//  4. the shape the schema states, held by hand: every member present and not
//     null, the tool a slug, no empty preset or entry;
//  5. the generated FromCanon, which refuses a member the type has no field for;
//  6. the tool's name, and no preset, no entry of a set and no observed entry twice.
func ReadUpstream(tool string, data []byte) (model.Upstream, error) {
	if len(data) > MaxUpstream {
		return model.Upstream{}, fmt.Errorf("the flattened upstream set is over %d bytes", MaxUpstream)
	}
	v, err := canon.Decode(data)
	if err != nil {
		return model.Upstream{}, fmt.Errorf("the flattened upstream set is not read: %w", err)
	}
	obj, ok := v.(canon.Object)
	if !ok {
		return model.Upstream{}, errors.New("the flattened upstream set is not a JSON object")
	}
	ver, ok := obj["version"].(canon.Int)
	switch {
	case !ok || ver < 1:
		return model.Upstream{}, errors.New("the flattened upstream set names no version of 1 or more")
	case int(ver) > UpstreamVersion:
		return model.Upstream{}, &UpstreamNotReady{Found: int(ver), Reads: UpstreamVersion}
	}
	if err := upstreamShape(obj); err != nil {
		return model.Upstream{}, fmt.Errorf("the flattened upstream set fails its schema: %w", err)
	}
	var doc mc.UpstreamSet
	if err := doc.FromCanon(v); err != nil {
		return model.Upstream{}, fmt.Errorf("the flattened upstream set is not read: %w", err)
	}
	if string(doc.Tool) != tool {
		return model.Upstream{}, fmt.Errorf("the flattened upstream set is for %q, not %q", doc.Tool, tool)
	}
	out := model.Upstream{Tool: tool}
	seen := map[string]bool{}
	for _, s := range doc.Sets {
		if seen[string(s.Preset)] {
			return model.Upstream{}, fmt.Errorf("the flattened upstream set names the preset %q twice", s.Preset)
		}
		seen[string(s.Preset)] = true
		set := model.UpstreamSet{Preset: string(s.Preset)}
		for _, e := range s.Entries {
			set.Entries = append(set.Entries, string(e))
		}
		if repeated(set.Entries) {
			return model.Upstream{}, fmt.Errorf("the flattened upstream set lists an entry of the preset %q twice", s.Preset)
		}
		out.Sets = append(out.Sets, set)
	}
	for _, e := range doc.Observed {
		out.Observed = append(out.Observed, string(e))
	}
	if repeated(out.Observed) {
		return model.Upstream{}, errors.New("the flattened upstream set names an observed entry twice")
	}
	return out, nil
}

// repeated is whether a key is in keys twice.
func repeated(keys []string) bool {
	return len(slices.Compact(slices.Sorted(slices.Values(keys)))) != len(keys)
}

// upstreamShape is the shape of entity-upstream-set.schema.json that the
// generated codec leaves at a zero value: each member present and not null,
// the tool a slug, and no empty text.
func upstreamShape(obj canon.Object) error {
	tool, ok := obj["tool"].(canon.String)
	if !ok || !slug(string(tool)) {
		return errors.New("tool is not a slug")
	}
	sets, ok := obj["sets"].(canon.Array)
	if !ok {
		return errors.New("sets is missing or not an array")
	}
	for _, s := range sets {
		set, ok := s.(canon.Object)
		if !ok {
			return errors.New("a set is not an object")
		}
		if p, ok := set["preset"].(canon.String); !ok || p == "" {
			return errors.New("a set has no preset name")
		}
		if err := texts(set["entries"], "a set's entries"); err != nil {
			return err
		}
	}
	return texts(obj["observed"], "observed")
}

// texts is whether v is an array of non-empty strings.
func texts(v canon.Value, what string) error {
	arr, ok := v.(canon.Array)
	if !ok {
		return fmt.Errorf("%s is missing or not an array", what)
	}
	for _, e := range arr {
		if s, ok := e.(canon.String); !ok || s == "" {
			return fmt.Errorf("%s holds an empty or non-text entry", what)
		}
	}
	return nil
}

// slugPattern is common.schema.json's slug: words of lower-case letters and
// digits joined by single hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func slug(s string) bool { return slugPattern.MatchString(s) }
