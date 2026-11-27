package audit

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/digest"
)

// chain is a Taskfile that includes a file that includes another, n files deep.
func chain(n int) fstest.MapFS {
	repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  a: {taskfile: f1.yml}\ntasks:\n  pre-push:\n    cmds: [go vet ./...]\n")}}
	for i := 1; i <= n; i++ {
		body := "version: '3'\ntasks: {}\n"
		if i < n {
			body = "version: '3'\nincludes:\n  a: {taskfile: f" + strconv.Itoa(i+1) + ".yml}\ntasks: {}\n"
		}
		repo["f"+strconv.Itoa(i)+".yml"] = &fstest.MapFile{Data: []byte(body)}
	}
	return repo
}

func nested(t *testing.T, n int) bool {
	t.Helper()
	in := TierInput{
		Repo:    chain(n),
		Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
		Tiers:   Tiers{Tiers: []Tier{{Name: "pre-push"}}, Rules: TierRules{Tier: "t.1"}},
	}
	for _, f := range CheckTiers(in).Findings {
		if strings.Contains(f.Missing, "nested more than") {
			return true
		}
	}
	return false
}

// boundary: maxIncludeDepth
func TestAnIncludeNestedPastTheCapIsAFindingAndOneAtItIsNot(t *testing.T) {
	t.Parallel()
	if nested(t, maxIncludeDepth) {
		t.Errorf("a chain of %d includes is a finding", maxIncludeDepth)
	}
	if !nested(t, maxIncludeDepth+1) {
		t.Errorf("a chain of %d includes is not a finding", maxIncludeDepth+1)
	}
}

// TestARemoteTextThatIsNotTheOneItsChecksumNamesIsAFinding: the text the caller
// gives for a remote include is held to the include's checksum, so a cache that holds another file is
// named; the matching text, with or without go-task's sha256: prefix, is read as the include.
func TestARemoteTextThatIsNotTheOneItsChecksumNamesIsAFinding(t *testing.T) {
	t.Parallel()
	text := "version: '3'\ntasks:\n  vet:\n    cmds: [go vet ./...]\n"
	sum := digest.Hex([]byte(text))
	run := func(body, checksum string) []string {
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  r: {taskfile: 'https://example.org/T.yml', checksum: " + checksum + "}\ntasks:\n  pre-push:\n    cmds: [go vet ./...]\n")}},
			Remote:  map[string][]byte{"https://example.org/T.yml": []byte(body)},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers:   Tiers{Tiers: []Tier{{Name: "pre-push"}}, Rules: TierRules{Tier: "t.1"}},
		}
		var out []string
		for _, f := range CheckTiers(in).Findings {
			out = append(out, f.Key+": "+f.Missing)
		}
		return out
	}
	for _, ok := range []string{sum, "sha256:" + sum, strings.ToUpper(sum)} {
		if got := run(text, ok); len(got) != 0 {
			t.Errorf("checksum %q over its own text: findings %v", ok, got)
		}
	}
	got := run(text+"# changed\n", sum)
	if len(got) != 1 || !strings.Contains(got[0], "include r checksum mismatch") {
		t.Errorf("another text under the checksum: findings %v, want the one mismatch", got)
	}
}
