package gate_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/dev/gate"
)

const goodRules = `format: 3
part: go
concern: style
rules:
  - id: go.style.1
    statement: Names are short.
    strength: must
    status: active
  - id: go.style.2
    statement: Comments say why.
    strength: should
    status: proposed
`

func book(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, s := range files {
		m[p] = &fstest.MapFile{Data: []byte(s)}
	}
	return m
}

// TestRulesReadsEveryRuleOfAFormatThreeFolder: the reader returns each rule's id, part, status and
// strength from the concern files of the folder, whatever layer they are in.
func TestRulesReadsEveryRuleOfAFormatThreeFolder(t *testing.T) {
	t.Parallel()
	got, err := gate.ReadRules(book(map[string]string{
		"base/go/rules/style.yaml":   goodRules,
		"cloud/sst/rules/state.yaml": "format: 3\npart: sst\nconcern: state\nrules:\n  - {id: sst.state.1, statement: x, strength: may, status: retired}\n",
		"base/go/tools.yaml":         "not: a rule file\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, r := range got {
		seen = append(seen, strings.Join([]string{r.ID, r.Part, r.Status, r.Strength}, " "))
	}
	want := "go.style.1 go active must|go.style.2 go proposed should|sst.state.1 sst retired may"
	if strings.Join(seen, "|") != want {
		t.Errorf("rules read:\n got  %s\n want %s", strings.Join(seen, "|"), want)
	}
	active := gate.ActiveRules(got)
	if len(active) != 1 || !active["go.style.1"] {
		t.Errorf("active rules = %v, want go.style.1 alone", active)
	}
}

// TestRulesRefusesWhatItCannotReadAndSaysWhat: each plant is refused with a line naming the file, and
// the subject missing (no folder, no rule file, an empty file, an empty list) is a failure, never "no
// active rule".
func TestRulesRefusesWhatItCannotReadAndSaysWhat(t *testing.T) {
	t.Parallel()
	one := func(body string) fstest.MapFS { return book(map[string]string{"base/go/rules/style.yaml": body}) }
	head := "format: 3\npart: go\nconcern: style\nrules:\n"
	rows := []struct {
		name string
		fsys fs.FS
		want string
	}{
		{"a duplicate id", book(map[string]string{
			"base/go/rules/style.yaml": goodRules,
			"base/go/rules/other.yaml": "format: 3\npart: go\nconcern: other\nrules:\n  - {id: go.style.1, statement: x, strength: must, status: active}\n",
		}), "the id go.style.1 is also at base/go/rules/other.yaml:"},
		{"a rule with no status", one(head + "  - {id: go.style.1, statement: x, strength: must}\n"), "go.style.1 has no status"},
		{"a status no reader knows", one(head + "  - {id: go.style.1, statement: x, strength: must, status: later}\n"), "has the status later"},
		{"a strength no reader knows", one(head + "  - {id: go.style.1, statement: x, strength: Hard, status: active}\n"), "has the strength \"Hard\""},
		{"a rule with no id", one(head + "  - {statement: x, strength: must, status: active}\n"), "a rule with no id"},
		{"a file that is not a mapping", one("- a\n- b\n"), "base/go/rules/style.yaml: the file is not a mapping"},
		{"a rule that is not a mapping", one(head + "  - just words\n"), "rule 1 is not a mapping"},
		{"a duplicate key", one(head + "  - {id: go.style.1, id: go.style.2, statement: x, strength: must, status: active}\n"), "base/go/rules/style.yaml:"},
		{"an empty file", one(""), "the file is empty"},
		{"a folder with no rule file", book(map[string]string{"base/go/tools.yaml": "a: b\n"}), "no rule file read"},
		{"a file with no rules key", one("format: 3\npart: go\nconcern: style\n"), "the file has no rules list"},
		{"a file whose rules is not a list", one("format: 3\npart: go\nconcern: style\nrules: none\n"), "the file has no rules list"},
		{"a file with an empty list", one(head + "  []\n"), "no rule read"},
		{"a folder that is not there", os.DirFS(filepath.Join(t.TempDir(), "absent")), "the rulebook folder is not read"},
	}
	for _, r := range rows {
		got, err := gate.ReadRules(r.fsys)
		if err == nil || !strings.Contains(err.Error(), r.want) {
			t.Errorf("%s: err = %v, want a line holding %q", r.name, err, r.want)
		}
		if len(got) > 1 && r.name != "a duplicate id" && r.name != "a rule with no status" {
			t.Errorf("%s: read %d rules besides the refused one", r.name, len(got))
		}
	}
}
