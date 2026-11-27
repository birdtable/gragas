package audit

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/port"
)

// refusing is a repository whose reader refuses the names in it as the host's
// reader refuses a link: the file is there, and opening it is
// port.ErrOutside with the reason said, never fs.ErrNotExist. It has only Open,
// so every read goes through it.
type refusing struct {
	repo  fstest.MapFS // named, so its own ReadFile does not pass Open by
	links map[string]bool
}

const linkReason = "it is a link, or a folder on the way to it is, and no link in the repository is followed"

func (r refusing) Open(name string) (fs.File, error) {
	if r.links[name] {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("%s: %w", linkReason, port.ErrOutside)}
	}
	return r.repo.Open(name)
}

// TestAFileTheReaderRefusesIsNotCheckedAndSaysWhy: a config, a pre-push hook or an include that is a link in the
// repository is there, and the reader refuses it, so audit says it is not
// checked and gives the reader's reason; it is never read as missing, nor
// passed over in silence. The control: the same repository without the link
// reads each file and has none of these findings.
func TestAFileTheReaderRefusesIsNotCheckedAndSaysWhy(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{
		"Taskfile.yml":   {Data: []byte("version: '3'\nincludes:\n  more: {taskfile: more.yml}\ntasks:\n  pre-push:\n    cmds: [go vet ./...]\n")},
		"more.yml":       {Data: []byte("version: '3'\ntasks: {}\n")},
		"hooks/pre-push": {Data: []byte("#!sh\ntask pre-push\n")},
		".golangci.yml":  {Data: []byte("version: '2'\n")},
	}
	tiers := func(r fs.FS) []Finding {
		return CheckTiers(TierInput{
			Repo:    r,
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{
				Tiers: []Tier{{Name: "pre-push"}}, Hook: []string{"hooks/pre-push"}, HookCalls: []string{"task pre-push"},
				Rules: TierRules{Tier: "t.1", Hook: "t.hook", HookCalls: "t.calls"},
			},
		}).Findings
	}
	src := Source{Tool: "golangci-lint", Files: []string{".golangci.yml"}}

	for _, f := range tiers(repo) {
		t.Errorf("the control: finding %s", f)
	}
	if c := find(repo, src); !c.Present || c.Unread != "" {
		t.Errorf("the control: the config is %+v, want it read", c)
	}

	linked := refusing{repo, map[string]bool{"more.yml": true, "hooks/pre-push": true, ".golangci.yml": true}}
	want := map[string]string{
		"include more unread": "the include more names more.yml, which is there and cannot be read inside the repository",
		"pre-push hook":       "the hook hooks/pre-push is there and cannot be read inside the repository",
	}
	got := map[string]string{}
	for _, f := range tiers(linked) {
		got[f.Key] = f.Missing
	}
	for key, says := range want {
		m, ok := got[key]
		if !ok || !strings.Contains(m, says) || !strings.Contains(m, linkReason) || strings.Contains(m, "not in the repository") {
			t.Errorf("finding %q = %q (there: %v), want it to say %q and the reader's reason", key, m, ok, says)
		}
	}
	if len(got) != len(want) {
		t.Errorf("findings %v, want only %v", got, want)
	}
	c := find(linked, src)
	if c.Present || !strings.Contains(c.Unread, "is there and cannot be read inside the repository") || !strings.Contains(c.Unread, linkReason) {
		t.Errorf("the linked config is %+v, want it not checked, with the reader's reason", c)
	}
}
