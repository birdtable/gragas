// Package digest is gragas's one owner of SHA-256 (arch.shared.8):
// the sum of bytes, in its raw, bare-hex and prefixed forms, and the hash a
// caller streams into. Every other package sums through it; the lint refuses
// crypto/sha256's New and Sum256 anywhere else (gate/shared.yaml). It holds
// no meaning of its own: canon gives a sum of canonical bytes its meaning,
// and each caller says what it sums.
package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
)

// Size is the length of a sum in bytes.
const Size = sha256.Size

// Prefix starts the prefixed form: Prefix and the 64 lower-case hex digits of
// the sum. Plans, the history, stamps and releases store that form, so it
// never changes.
const Prefix = "sha256:"

// Sum is the SHA-256 of b.
func Sum(b []byte) [Size]byte { return sha256.Sum256(b) }

// Hex is the SHA-256 of b in 64 lower-case hex digits.
func Hex(b []byte) string {
	s := Sum(b)
	return hex.EncodeToString(s[:])
}

// Prefixed is the SHA-256 of b in the prefixed form, Prefix and its hex.
func Prefixed(b []byte) string { return Prefix + Hex(b) }

// New is a SHA-256 hash to stream bytes into, and the hash an HMAC is made
// over (hmac.New(digest.New, key)).
func New() hash.Hash { return sha256.New() }

// HexOf is what h has summed so far, in lower-case hex. h is left as it is,
// so more can be written to it.
func HexOf(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

// PrefixedOf is what h has summed so far, in the prefixed form.
func PrefixedOf(h hash.Hash) string { return Prefix + HexOf(h) }

// Reader is the SHA-256 of everything r gives until its end, in lower-case
// hex, and how many bytes that was. A read that fails is the error and no
// sum; the count is what was summed before it.
func Reader(r io.Reader) (string, int64, error) {
	h := New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", n, err
	}
	return HexOf(h), n, nil
}
