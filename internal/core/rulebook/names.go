package rulebook

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Names is a list of names that public rule text must not hold, such as the
// names of organisations, products or people. The list itself is private: it
// is read from a file outside the repository and never committed.
type Names struct {
	patterns []*regexp.Regexp
}

// ParseNames reads a names file: one regular expression (RE2 syntax) per line,
// matched without regard to case. Blank lines and lines starting with # are
// skipped. Patterns are tried in file order and the first that matches is
// reported.
func ParseNames(r io.Reader) (*Names, error) {
	n := &Names{}
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		re, err := regexp.Compile("(?i)" + text)
		if err != nil {
			return nil, fmt.Errorf("names file line %d: %w", line, err)
		}
		n.patterns = append(n.patterns, re)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read names file: %w", err)
	}
	return n, nil
}

// Find returns the first forbidden name in text, or "" when there is none.
func (n *Names) Find(text string) string {
	if n == nil {
		return ""
	}
	for _, re := range n.patterns {
		if m := re.FindString(text); m != "" {
			return m
		}
	}
	return ""
}
