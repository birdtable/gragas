package canon

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
)

// ErrKey is a public key that is not one line of the authorized-keys text
// form: a key type, the key's base64 and an optional comment.
var ErrKey = errors.New("not a public key in the authorized-keys text form")

// Fingerprint derives the fingerprint of publicKey, one public key in the
// authorized-keys text form, in the form OpenSSH prints: "SHA256:" and the
// unpadded base64 of the SHA-256 of the key's decoded bytes. It is the one
// function that makes a fingerprint (a typed one is never a trust anchor, A12);
// the comment after the key is not part of it, so renaming a key changes
// nothing. The key must be in full: a key type, then its base64, and the base64
// must carry the same key type.
func Fingerprint(publicKey string) (string, error) {
	fields := strings.Fields(publicKey)
	if len(fields) < 2 || strings.ContainsAny(strings.TrimSpace(publicKey), "\r\n") {
		return "", fmt.Errorf("%w: want a key type and its base64 on one line", ErrKey)
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(blob) < 4 {
		return "", fmt.Errorf("%w: the second field is not base64", ErrKey)
	}
	n := int64(binary.BigEndian.Uint32(blob)) // as wide as any system's int, so the bound is the same on each
	if n < 1 || n > int64(len(blob)-4) || string(blob[4:4+n]) != fields[0] {
		return "", fmt.Errorf("%w: the key type %q is not the one the key carries", ErrKey, fields[0])
	}
	sum := digest.Sum(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}
