package format

import (
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// digestOf is the digest of bytes as a file holds them, in the form a plan
// and a bindings record store.
func digestOf(data []byte) model.Digest {
	return model.Digest(digest.Prefixed(data))
}

// lineDigest is the canonical digest of one line of history.jsonl as it is on
// disk, the digest the next line names as its previous and an undo as the entry
// it takes back; "" when the line cannot be digested, which the schema check
// has already reported.
func lineDigest(line []byte) model.Digest {
	v, err := canon.Decode(line)
	if err != nil {
		return ""
	}
	d, err := canon.DigestOf(v)
	if err != nil {
		return ""
	}
	return model.Digest(d.String())
}
