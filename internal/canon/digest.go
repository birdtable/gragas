package canon

import (
	"encoding/hex"
	"hash"
	"strconv"

	"github.com/birdtable/gragas/internal/digest"
)

// DigestPrefix starts a digest's text form. The form is DigestPrefix and the
// 64 lower-case hex digits of a SHA-256 sum, written by String and read by
// ParseDigest and nowhere else. Plans, the history and releases store it, so
// it is a compatibility surface and never changes.
const DigestPrefix = digest.Prefix

// digestLen is the length of a digest's text form.
const digestLen = len(DigestPrefix) + 2*digest.Size

// Digest is the SHA-256 sum of a value's canonical bytes, its RFC 8785
// encoding. In canon a digest has that one meaning: it is never of a file's
// own bytes, which for a Document are not canonical. A digest of a file as
// written, such as a verdict's, is the gate's fingerprint, not canon's. Two
// digests are equal under == when they are of the same canonical bytes. The
// zero Digest is no digest: IsZero reports it, and its String is "".
type Digest struct {
	sum [digest.Size]byte
	set bool
}

// DigestOf is the digest of v, the SHA-256 of its Canonical encoding, so any
// implementation of RFC 8785 and SHA-256 gives the same digest from the same
// JSON. DigestOf refuses what Encode refuses.
func DigestOf(v Value) (Digest, error) {
	b, err := Encode(v, Canonical)
	if err != nil {
		return Digest{}, err
	}
	h := digest.New()
	h.Write(b)
	return fromHash(h), nil
}

// fromHash is the digest a SHA-256 hash has summed so far.
func fromHash(h hash.Hash) Digest {
	d := Digest{set: true}
	h.Sum(d.sum[:0])
	return d
}

// ParseDigest reads a digest's text form, DigestPrefix and exactly 64
// lower-case hex digits with nothing before or after, and refuses anything
// else with BadDigest, its Offset at the first byte that is wrong.
func ParseDigest(s string) (Digest, error) {
	bad := func(at int, why string) (Digest, error) {
		return Digest{}, &Error{Kind: BadDigest, Offset: at, Detail: why}
	}
	if len(s) < len(DigestPrefix) || s[:len(DigestPrefix)] != DigestPrefix {
		return bad(0, "a digest starts "+strconv.Quote(DigestPrefix))
	}
	for i := len(DigestPrefix); i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return bad(i, "a digest is lower-case hex after its prefix")
		}
	}
	if len(s) != digestLen {
		return bad(min(len(s), digestLen), "a digest has 64 hex digits, this one "+strconv.Itoa(len(s)-len(DigestPrefix)))
	}
	d := Digest{set: true}
	if _, err := hex.Decode(d.sum[:], []byte(s[len(DigestPrefix):])); err != nil {
		return bad(len(DigestPrefix), err.Error())
	}
	return d, nil
}

// String is the digest's text form, DigestPrefix and 64 lower-case hex
// digits, or "" for the zero Digest.
func (d Digest) String() string {
	if !d.set {
		return ""
	}
	return DigestPrefix + hex.EncodeToString(d.sum[:])
}

// IsZero is whether d is the zero Digest, which is no digest.
func (d Digest) IsZero() bool { return !d.set }
