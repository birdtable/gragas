package link

import (
	"github.com/birdtable/gragas/internal/model"
)

// The grammar of a tool version range and the order of versions are model's
// (model.ParseRange), so an adapter's tool range and a plugin grant's
// versions, which host's step runner holds a program to, are read by one
// reading. These are its names here, for the link context.

// ErrRange is a tool version range that does not parse or holds no version.
var ErrRange = model.ErrRange

// ParseRange parses a tool version range. An error matches ErrRange and says
// why.
func ParseRange(s string) (model.VersionRange, error) {
	return model.ParseRange(s)
}

// InRange is whether version, major.minor.patch, is in vr. A version of
// another form is an error matching ErrRange.
func InRange(vr model.VersionRange, version string) (bool, error) {
	return model.InRange(vr, version)
}

// Compare orders two versions of the form major.minor.patch: -1, 0 or 1. The
// order is semver's, golang.org/x/mod/semver's.
func Compare(a, b string) int {
	return model.CompareVersions(a, b)
}
