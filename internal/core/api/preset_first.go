package coreapi

import (
	"fmt"
	"strconv"
	"strings"
)

// firstDifference names the first line at which have is not want: its number,
// what the tree holds there and what the book makes, each cut to a short
// quote. A file that ends early or runs on names the missing or extra line.
func firstDifference(have, want []byte) string {
	h, w := strings.SplitAfter(string(have), "\n"), strings.SplitAfter(string(want), "\n")
	for i := 0; i < max(len(h), len(w)); i++ {
		var a, b string
		if i < len(h) {
			a = h[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a == b {
			continue
		}
		switch {
		case i >= len(h) || a == "":
			return fmt.Sprintf("line %d is missing: the book makes %s", i+1, quoteLine(b))
		case i >= len(w) || b == "":
			return fmt.Sprintf("line %d is not one the book makes: it holds %s", i+1, quoteLine(a))
		}
		return fmt.Sprintf("line %d holds %s, the book makes %s", i+1, quoteLine(a), quoteLine(b))
	}
	return "the bytes differ"
}

// quoteLine is line quoted, its newline kept visible, cut to 60 bytes.
func quoteLine(line string) string {
	const most = 60
	if len(line) > most {
		return strconv.Quote(line[:most]) + "…"
	}
	return strconv.Quote(line)
}
