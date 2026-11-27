package git

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/model"
)

// parseIdentity reads what config-identity prints: entries of "key\nvalue"
// each ended by NUL, the keys user.name and user.email. Where a key is set
// more than once the last wins, as git's own reading does. It answers the
// identity, each half one line with its control sequences stripped, and
// which half is missing ("" when neither). Output git does not print, a key
// the verb did not ask for, or text that is not UTF-8 is an error.
func parseIdentity(out []byte) (id model.Identity, missing string, err error) {
	var name, email string
	var haveName, haveEmail bool
	for len(out) > 0 {
		end := bytes.IndexByte(out, 0)
		if end < 0 {
			return model.Identity{}, "", errors.New("an entry is not ended by NUL")
		}
		entry := out[:end]
		out = out[end+1:]
		key, value, ok := bytes.Cut(entry, []byte{'\n'})
		if !ok {
			// a key with no value at all ("[user] name" alone) sets nothing
			value = nil
		}
		if !utf8.Valid(value) {
			return model.Identity{}, "", fmt.Errorf("the value of %q is not UTF-8", key)
		}
		switch string(key) {
		case "user.name":
			name, haveName = clean(string(value)), ok
		case "user.email":
			email, haveEmail = clean(string(value)), ok
		default:
			return model.Identity{}, "", fmt.Errorf("a key that was not asked for: %q", clip(key))
		}
	}
	haveName = haveName && name != ""
	haveEmail = haveEmail && email != ""
	switch {
	case !haveName && !haveEmail:
		return model.Identity{}, "user.name and user.email", nil
	case !haveName:
		return model.Identity{}, "user.name", nil
	case !haveEmail:
		return model.Identity{}, "user.email", nil
	}
	return model.Identity{Name: name, Email: email}, "", nil
}

// sequence is an escape sequence: CSI (ESC [ ... final), OSC (ESC ] ... BEL
// or ESC \), or ESC and one more character. It is the terminal adapter's
// pattern; this adapter cannot import that one (arch.ports.6).
var sequence = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\x1b[@-_]")

// clean is a value with its escape sequences and control characters
// removed, newlines and tabs among them, so it is one line that draws as
// itself (cli.errors.4), trimmed of the spaces at either end.
func clean(v string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, sequence.ReplaceAllString(v, "")))
}

// clip is at most 40 bytes of b, quoted, for an error.
func clip(b []byte) string {
	if len(b) > 40 {
		b = b[:40]
	}
	return string(b)
}
