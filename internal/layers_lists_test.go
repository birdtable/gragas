package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// layers.yaml is edited by many pull requests at once. A list on one line makes
// every one of them touch that line; a list with one entry per line, in order,
// lets two insertions at different points merge. These checks hold the source
// to that shape.

var (
	// tableFlow is a key whose value is a list on the line, with an entry.
	tableFlow = regexp.MustCompile(`^\s*(?:- )?([a-z_]+): \[\s*[^\s\]].*$`)
	// tableKey is a key that opens a block, and tableItem a plain entry of one.
	tableKey  = regexp.MustCompile(`^(\s*)(?:- )?([a-z_]+):$`)
	tableItem = regexp.MustCompile(`^(\s*)- ([^:{\[#"'][^:{\[#]*)$`)
)

// keepsOrder are the keys whose order the rendered files show, so they are
// block lists in the order they are meant to be read: the packages of a row
// (the rules and the table rows follow it), the contexts, the explorer's
// modules and the words of the platform and source sections, which are printed
// as prose.
func keepsOrder(section, key string) bool {
	if section == "explorer" {
		return true
	}
	switch key {
	case "packages", "names", "roots", "tools", "homes", "scope", "formats", "refused":
		return true
	}
	return false
}

// listFindings are the lists of the layer table's text that are on one line or
// out of order.
func listFindings(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	section := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line != "" && line[0] >= 'a' && line[0] <= 'z' {
			section, _, _ = strings.Cut(line, ":")
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if m := tableFlow.FindStringSubmatch(line); m != nil {
			out = append(out, fmt.Sprintf("line %d: %s is a list on one line; write one entry per line", i+1, m[1]))
			continue
		}
		k := tableKey.FindStringSubmatch(line)
		if k == nil {
			continue
		}
		var items []string
		for i+1 < len(lines) {
			it := tableItem.FindStringSubmatch(lines[i+1])
			if it == nil || len(it[1]) < len(k[1]) {
				break
			}
			items = append(items, strings.TrimSpace(it[2]))
			i++
		}
		if len(items) > 1 && !keepsOrder(section, k[2]) && !slices.IsSorted(items) {
			out = append(out, fmt.Sprintf("line %d: the list %s is not in order", i+1-len(items)+1, k[2]))
		}
	}
	return out
}

// TestTheLayerTableListsAreOneEntryPerLineInOrder: layers.yaml holds no list
// on one line and no unordered list that the output does not follow.
func TestTheLayerTableListsAreOneEntryPerLineInOrder(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(root, "layers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := listFindings(string(data)); len(got) != 0 {
		t.Errorf("layers.yaml: %s", strings.Join(got, "\n"))
	}
}

// TestAListOnOneLineOrOutOfOrderIsFound: each shape the check refuses, and the
// ones it lets be.
func TestAListOnOneLineOrOutOfOrderIsFound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, text, want string
	}{
		{"a list with entries on one line", "layers:\n  - layer: a\n    may: [x, y]\n", "may is a list on one line"},
		{"a list of one entry on one line", "layers:\n  - layer: a\n    std: [x]\n", "std is a list on one line"},
		{"a block list out of order", "layers:\n  - layer: a\n    may:\n      - y\n      - x\n", "the list may is not in order"},
		{"an empty list", "layers:\n  - layer: a\n    may: []\n", ""},
		{"a block list in order", "layers:\n  - layer: a\n    may:\n      - x\n      - y\n", ""},
		{"packages kept in the order of the output", "layers:\n  - layer: a\n    packages:\n      - y\n      - x\n", ""},
		{"a module list of the explorer kept in the order of the output", "explorer:\n  modules:\n    - name: a\n      may:\n        - y\n        - x\n", ""},
		{"a list inside a comment", "# may: [y, x]\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(listFindings(tc.text), "\n")
			switch {
			case tc.want == "" && got != "":
				t.Errorf("found %q", got)
			case tc.want != "" && !strings.Contains(got, tc.want):
				t.Errorf("found %q, want %q", got, tc.want)
			}
		})
	}
}
