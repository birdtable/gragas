package pluginbundle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// GrantsDigest is the digest of the grants a manifest asks for: its programs,
// in the order written, as canon's canonical bytes. It is what an approval
// keeps beside the bundle's digest (model.Approval.Grants), so a later approval
// can say the grants changed. It is of the manifest alone.
func GrantsDigest(m plugin.Manifest) (model.Digest, error) {
	programs := make(canon.Array, 0, len(m.Programs))
	for _, p := range m.Programs {
		programs = append(programs, p.ToCanon())
	}
	b, err := canon.Encode(programs, canon.Canonical)
	if err != nil {
		return "", fmt.Errorf("the grants of %s cannot be summed: %w", plugin.Shown(m.Name), err)
	}
	return model.Digest(digest.Prefixed(b)), nil
}

// GrantsHeld is the grants a manifest asks for one by one: each program's id
// and the digest of that program alone, as canon's canonical bytes, sorted by
// id. It is what an approval keeps beside GrantsDigest (model.Approval.Held),
// so a later approval can say which grants were added, which went, and which
// kept an id and changed. A manifest with no program is an empty
// list, not nil. Two programs with one id are an error: the list would not
// say which was meant.
func GrantsHeld(m plugin.Manifest) ([]model.GrantHeld, error) {
	out := make([]model.GrantHeld, 0, len(m.Programs))
	for _, p := range m.Programs {
		var id string
		switch {
		case p.Found != nil:
			id = p.Found.ID
		case p.At != nil:
			id = p.At.ID
		case p.Script != nil:
			id = p.Script.ID
		}
		b, err := canon.Encode(p.ToCanon(), canon.Canonical)
		if err != nil {
			return nil, fmt.Errorf("the grant %s of %s cannot be summed: %w", plugin.Shown(id), plugin.Shown(m.Name), err)
		}
		out = append(out, model.GrantHeld{ID: id, Digest: model.Digest(digest.Prefixed(b))})
	}
	slices.SortFunc(out, func(a, b model.GrantHeld) int { return strings.Compare(a.ID, b.ID) })
	for i := 1; i < len(out); i++ {
		if out[i-1].ID == out[i].ID {
			return nil, fmt.Errorf("%s asks for the grant %s twice", plugin.Shown(m.Name), plugin.Shown(out[i].ID))
		}
	}
	return out, nil
}
