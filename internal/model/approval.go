package model

import (
	"fmt"
	"io/fs"
)

// Approval is what a person said yes to when they approved a plugin to run
// on this machine: the digest of its bundle, the
// reference it was shown by, and the digest of the grants it asks for, as they
// were shown at the yes. The Trust port records it and answers for its Digest
// alone.
type Approval struct {
	// Digest is the bundle's digest, as the lock names it: over the plugin's
	// folder, its plugin.yaml, and so its grants, and its module. It is canon's "sha256:" and sixty-four
	// lower-case hexadecimal digits, the one thing an approval is of: a
	// bundle whose module or grant changed by one byte has another digest,
	// and is not approved. Trust is handed it and never computes it.
	Digest Digest
	// Ref is the reference the plugin was shown by at the yes, as the person
	// read it: text, never empty, and never a path that is read. It is
	// display text that can come from a lock or a registry the person does
	// not write, so every print of it goes through the escape shown text
	// takes, never raw.
	Ref string
	// Grants is the digest of the grants the plugin asks for, as they were
	// shown at the yes, in the same form as Digest: kept so a later approval
	// can say what changed. Approved never reads it.
	Grants Digest
	// Held is the grants one by one (GrantHeld), as they were shown at the
	// yes, sorted by id (GrantsHeldFormed). Nil is an approval that recorded
	// none by id, as an older one did; Approved never reads it.
	Held []GrantHeld
}

// Formed is whether d is in the digest's text form: "sha256:" and exactly
// sixty-four lower-case hexadecimal digits, nothing before or after. An empty
// d is not formed. The Trust port refuses a digest that is not.
func (d Digest) Formed() bool {
	const prefix = "sha256:"
	s := string(d)
	if len(s) != len(prefix)+64 || s[:len(prefix)] != prefix {
		return false
	}
	for _, c := range s[len(prefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// GrantHeld is one grant of an approval, as it was shown at the yes: the grant's
// id, and the digest of that grant alone, so a later approval can say which
// grants were added, which went, and which kept an id and changed.
type GrantHeld struct {
	// ID is the grant's id, as the manifest names it: text, never empty.
	ID string
	// Digest is the digest of that one grant, in the digest's form.
	Digest Digest
}

// GrantsHeldFormed is whether h is a list the Trust port records: every id not
// empty, every digest formed, ids sorted and none twice. A nil h is formed:
// it is an approval that recorded no grants by id (a record of version 1), and
// is not the same as an empty one, a plugin that asks for no grant.
func GrantsHeldFormed(h []GrantHeld) error {
	for i, g := range h {
		switch {
		case g.ID == "":
			return fmt.Errorf("grant %d has no id: %w", i, fs.ErrInvalid)
		case !g.Digest.Formed():
			return fmt.Errorf("the digest of grant %q is not of the form sha256:<64 hex>: %w", g.ID, fs.ErrInvalid)
		case i > 0 && h[i-1].ID >= g.ID:
			return fmt.Errorf("grant %q is not after grant %q: the grants are sorted by id, each once: %w", g.ID, h[i-1].ID, fs.ErrInvalid)
		}
	}
	return nil
}
