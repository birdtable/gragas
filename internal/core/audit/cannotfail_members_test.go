package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/audit"
)

// TestCannotFailHoldsToTheMembersGoTaskRuns: each line of testdata/members.txt is
// a member and what go-task 3.54.0 does with it, which internal's
// TestCannotFailAgreesWithTheGoTaskThatRunsTheMember holds to the pinned go-task
// itself. A member go-task ends green is one CannotFail names a form of, and one
// it ends red is one it finds nothing in: a reading that took errexit as off, or
// pipefail as on, turns a row red.
func TestCannotFailHoldsToTheMembersGoTaskRuns(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/members.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.ToSlash(t.TempDir())
	rows := 0
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.SplitN(line, " ", 3)
		if len(f) != 3 || (f[0] != "green" && f[0] != "red" && f[0] != "unjudged") || (f[1] != "p" && f[1] != "-") {
			t.Fatalf("bad row %q", line)
		}
		rows++
		member := strings.ReplaceAll(strings.ReplaceAll(f[2], "{root}", root), "; ", "\n")
		v := audit.CannotFail(member, f[1] == "p")
		switch {
		case f[0] == "unjudged":
			if v.Why != "" || v.Unjudged == "" {
				t.Errorf("%q: %+v, want it unjudged with no form named", f[2], v)
			}
		case v.Unjudged != "":
			t.Errorf("%q is unjudged (%s), and a green or red row is a form the check reads", f[2], v.Unjudged)
		case f[0] == "green" && v.Why == "":
			t.Errorf("%q: go-task ends it green and the check finds nothing", f[2])
		case f[0] == "red" && v.Why != "":
			t.Errorf("%q: go-task ends it red and the check says it cannot fail (%s)", f[2], v.Why)
		}
	}
	if rows < 20 {
		t.Errorf("%d rows: the table was emptied", rows)
	}
}
