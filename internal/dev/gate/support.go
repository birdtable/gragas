package gate

import (
	"strconv"
	"strings"
)

// ShippedPlatforms are the systems the shipped binary is built for, as
// GOOS/GOARCH: the four systems gragas runs on, each on both architectures.
// The gate reads the binary's modules for each and joins them, so
// gate/closure.txt is the same on every host (A67), and a module one system
// alone links, such as mousetrap on Windows (gragas#1371) or the flock library
// go-files' lock takes on Windows (gragas#909), is on the list for every host.
// internal/closure_test.go's closurePlatforms holds the same list.
func ShippedPlatforms() []string {
	return []string{"darwin/amd64", "darwin/arm64", "freebsd/amd64", "freebsd/arm64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"}
}

// Printable is s as it is when it is printable ASCII, and otherwise quoted to
// ASCII as Go would, without the quotes: a control character, a byte that is
// not UTF-8 and a rune past ASCII such as a bidi override (U+202E) are each
// escaped. gragas-dev escapes a line it reads from another program with it.
func Printable(s string) string {
	if !strings.ContainsFunc(s, control) {
		return s
	}
	q := strconv.QuoteToASCII(s)
	return q[1 : len(q)-1]
}

func control(r rune) bool { return r < 0x20 || r >= 0x7f }
