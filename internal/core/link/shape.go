package link

import (
	"regexp"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// The forms a text value must have, and how a person is told each.
const (
	slugSays   = "lowercase letters and digits in words joined by -"
	semverSays = "major.minor.patch"
	wordSays   = "one word of letters, digits and . _ / -"
)

var (
	slugForm = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	wordForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	stepForm = regexp.MustCompile(`^\.{0,2}[A-Za-z0-9_-][A-Za-z0-9._-]*$`)
)

// relative reads a slash-separated path below the config set: no leading
// slash, no drive, no backslash, and no step that is . or ..
func (r *reader) relative(m *ordered.Map, where, k string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if expressive(s) {
		r.refuse(m.KeyLine(k), CheckExpression, "%s is %q, which holds an expression's characters", at(where, k), s)
		return ""
	}
	for step := range strings.SplitSeq(s, "/") {
		if !stepForm.MatchString(step) {
			r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be a path below the config set: slash-separated, no step empty, . or .., no drive", at(where, k), s)
			return ""
		}
	}
	return s
}
