package format

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
)

// Names is a list of names that rule text must not hold, such as the names of
// organisations, products, tools or people. The list is private: it is read
// from a file outside the repository and never committed. A rulebook's own
// terms are the allowance for a name its rules must use.
//
// This is the format context's own implementation and not an import of
// internal/core/rulebook's: contexts do not import one another (arch.ports.8),
// and the old file goes with the old commands (gate/baselines.yaml, legacy-names,
// zero by M6). It adds the typed BadNames, which names the line of a bad pattern.
type Names struct {
	patterns []*regexp.Regexp
	// hashes are the names written as the SHA-256 of their words (names_hash.go).
	hashes map[[digest.Size]byte]struct{}
}

// BadNames is a names file with a pattern that does not compile. It ends a
// run not ready: the check cannot say what it would have found. Its line says
// the line's number and nothing of Err, since regexp's error quotes the
// pattern, and the pattern is the private list itself.
type BadNames struct {
	Line int
	Err  error
}

// Error says the line and no more, never the compiler's message, which quotes
// the pattern and so a piece of the private list.
func (e *BadNames) Error() string {
	return fmt.Sprintf("line %d of the names file is not a pattern that compiles", e.Line)
}
func (e *BadNames) Unwrap() error { return e.Err }

// ParseNames reads a names file: a line of 64 hexadecimal digits is the hash
// of a name (names_hash.go); any other line is a regular expression (RE2
// syntax), matched without regard to case. A blank line and a line that starts
// with # are skipped.
func ParseNames(data []byte) (*Names, error) {
	n := &Names{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, len(data)+1)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if isHashLine(text) {
			n.addHash(text)
			continue
		}
		re, err := regexp.Compile("(?i)" + text)
		if err != nil {
			return nil, &BadNames{Line: line, Err: err}
		}
		n.patterns = append(n.patterns, re)
	}
	return n, nil
}

// Find returns the first name of the list that text holds, or "".
func (n *Names) Find(text string) string {
	if n == nil {
		return ""
	}
	for _, re := range n.patterns {
		if m := re.FindString(text); m != "" {
			return m
		}
	}
	if n.findHashed(text) {
		return MaskedName
	}
	return ""
}

// Len is how many names the list holds: its patterns and its hashes. A list of
// none can find nothing, which a gate that must not pass silently refuses.
func (n *Names) Len() int {
	if n == nil {
		return 0
	}
	return len(n.patterns) + len(n.hashes)
}

// ReadNames is ParseNames over a reader, for a caller that holds the file
// open and not its bytes.
func ReadNames(r io.Reader) (*Names, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseNames(data)
}
