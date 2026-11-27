package main

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/scan"
)

// runNames is the names command: names hash reads names on stdin, one a
// line, and prints the list lines the names scan reads, a SHA-256 each,
// sorted and each once. It takes no name as an argument, so none lands in
// a shell's history, and it never prints a name: a line it refuses is named
// by its number.
func runNames(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) != 1 || args[0] != "hash" {
		talk.Text(usage)
		return exitUsage
	}
	hashes, names, err := hashNames(stdin)
	if err != nil {
		talk.Text("gragas-dev names hash: " + err.Error() + "\n")
		return exitFailed
	}
	out := &say.Speaker{W: stdout}
	for _, h := range hashes {
		out.Text(h + "\n")
	}
	talk.Text(fmt.Sprintf("names hash: %d %s, %d %s: keep the list outside the tree and set %s to its path\n",
		names, pluralWord(names, "name", "names"), len(hashes), pluralWord(len(hashes), "line", "lines"), scan.NamesHashesVar))
	return exitDone
}

// hashNames reads one name a line, passing over a blank line and one that
// starts with #, and answers the list lines and how many names it read.
func hashNames(r io.Reader) ([]string, int, error) {
	var hashes []string
	names := 0
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hs, err := scan.NameHashes(line)
		if err != nil {
			return nil, 0, fmt.Errorf("line %d: %w", n, err)
		}
		hashes = append(hashes, hs...)
		names++
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if names == 0 {
		return nil, 0, fmt.Errorf("no name was read on stdin: write one name a line, of 1 to %d words", scan.MaxRun)
	}
	slices.Sort(hashes)
	return slices.Compact(hashes), names, nil
}
