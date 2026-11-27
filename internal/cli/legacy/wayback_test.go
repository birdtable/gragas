package legacy

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// noOther is a host whose line maker gives no line for a program other than
// gragas, as the root's does in PowerShell and cmd.
type noOther struct{ recorder }

func (noOther) Line([]string) (string, error) { return "", model.ErrOtherProgram }

// TestGensWayBackIsALineInPOSIXAndWordsElsewhere: gen's way
// back is the git restore line where the root makes one, and where it gives
// no line for git (PowerShell and cmd) the same step in words, each path and
// the folder as %q, with no line at all; it is never an error, since gen
// wrote. Risk: a git line pasted where gragas cannot know git reads its words
// as written, or a run that wrote and ends as if it did not.
func TestGensWayBackIsALineInPOSIXAndWordsElsewhere(t *testing.T) {
	t.Parallel()
	paths := []string{"presets/a b.yml", "presets/c;$(d).yml"}
	u, err := genWayBack(&recorder{}, "sub dir", paths)
	if err != nil || len(u.Lines) != 1 || u.Lines[0] != "git -C sub dir restore --source=HEAD -- presets/a b.yml presets/c;$(d).yml" {
		t.Errorf("a host that makes the line: %+v, %v", u, err)
	}
	u, err = genWayBack(&noOther{}, "sub dir", paths)
	want := `the generated files are tracked: git puts back the ones before: restore "presets/a b.yml", "presets/c;$(d).yml" from HEAD with git in the repository at "sub dir"`
	if err != nil || len(u.Lines) != 0 || u.Why != want {
		t.Errorf("a host that gives no line for git: %+v, %v\nwant why %s", u, err, want)
	}
	if u, err = genWayBack(&noOther{}, ".", paths[:1]); err != nil || u.Why != `the generated files are tracked: git puts back the ones before: restore "presets/a b.yml" from HEAD with git` {
		t.Errorf("the folder itself: %+v, %v", u, err)
	}
}
