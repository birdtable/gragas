package git

import (
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// NotReady is a run that cannot start until something is set: git's path,
// or the identity git commits as. It ends "not ready" with the line to type.
type NotReady struct {
	model.Refusal
}

func (e *NotReady) Error() string { return e.Observed() }

// Refused is git at a path the adapter will not run: another account owns
// it, a folder on the way to it or a link on the way, its group or others
// can write the file or such a folder, or it is not a regular file it may
// execute (cli.process.2).
type Refused struct {
	model.Refusal
}

func (e *Refused) Error() string { return e.Observed() }

// Failed is git itself failing: it ran past the deadline, printed past the
// cap, or ended in a way the verb does not expect. It names the run id, says
// what was being done, and unwraps to its cause.
type Failed struct {
	model.Caused
}

// noIdentity is the not ready of a repository that commits as nobody, naming
// what is missing.
func noIdentity(missing string) error {
	r, err := model.NewRefusal(model.NotReady,
		"git config sets no "+missing+", so there is no one to name as who", nil,
		`git config --global user.name "Your Name" && git config --global user.email you@example.com`)
	if err != nil {
		return err
	}
	return &NotReady{r}
}

// failed is a *Failed of this run. New refuses an empty run id, so the
// failure is always built; were it not, the cause is still answered.
func (g *Git) failed(what string, err error) error {
	c, cerr := model.NewCaused(FailureKind, g.cfg.Run, what, err)
	if cerr != nil {
		return cerr
	}
	return &Failed{c}
}

// notReady is a *NotReady of what is observed, with the line that fixes it.
func notReady(observed, fix string) error {
	r, err := model.NewRefusal(model.NotReady, observed, nil, fix)
	if err != nil {
		return err
	}
	return &NotReady{r}
}

// tooOld is the not ready of a git before 2.38, which lacks the machine
// formats the History methods read (history-version-bands).
func tooOld(found string) error {
	return notReady("git "+found+" is older than 2.38, which the history reads need",
		"install git 2.38 or later and set "+Setting+" in the settings file to its path")
}

// partialClone is the not ready of a git before 2.44 in a repository that
// names a promisor remote: such a git fetches a missing object from it, and
// only 2.44 can be told not to (history-version-bands).
func partialClone(found string) error {
	return notReady("the repository names a promisor remote, and git "+found+" would fetch a missing object from it; git 2.44 or newer reads a partial clone without fetching",
		"install git 2.44 or later and set "+Setting+" in the settings file to its path")
}

// belowTop is the not ready of a folder that git found below the top of
// its repository: the .git the folder holds is not a repository, or git
// ran in a folder of one.
func belowTop() error {
	return notReady("the folder is not the top of its git repository, so the listing's paths are not its own",
		"run in the top folder of the repository, or run git init in this folder")
}

// shallowClone is the not ready of a read a shallow clone cannot answer.
func shallowClone(what string) error {
	return notReady("the repository is a shallow clone, so "+what+" may not be in it", "git fetch --unshallow")
}

// mergeInProgress is the not ready of an index that holds a path at more
// than one stage.
func mergeInProgress() error {
	return notReady("the index holds a merge in progress, so it has no one file at each path",
		"git merge --continue, or git merge --abort")
}

// Missing is a revision the repository does not have. It does not match
// fs.ErrNotExist, which Blob keeps for a path the revision lacks.
type Missing struct {
	// Rev is the revision as it was asked for, checked to be a revision.
	Rev string
}

func (e *Missing) Error() string { return fmt.Sprintf("the revision %q does not exist", e.Rev) }

// Is matches port.ErrNoRev, which the History port names for a revision the
// repository does not have.
func (e *Missing) Is(target error) bool { return target == port.ErrNoRev }

// NoBase is two revisions with no one merge base: none in common, or more
// than one, as after a criss-cross merge, where none is picked.
type NoBase struct {
	// A and B are the revisions as they were asked for.
	A, B string
	// Bases is how many merge bases there are, 0 or more than 1.
	Bases int
}

func (e *NoBase) Error() string {
	if e.Bases == 0 {
		return fmt.Sprintf("%q and %q have no commit in common", e.A, e.B)
	}
	return fmt.Sprintf("%q and %q have %d merge bases, and none is picked", e.A, e.B, e.Bases)
}

// Is matches port.ErrNoBase, which the History port names for two
// revisions with no one merge base.
func (e *NoBase) Is(target error) bool { return target == port.ErrNoBase }

// errBadRev is a revision refused before anything runs: one that could be
// read as an option or as more than a name.
var errBadRev = errors.New("not a revision this adapter runs")

// errBadPath is a path refused before anything runs, or printed by git: one
// that climbs, is absolute, has an empty, "." or ".." part, names .git, or
// holds a control character, a backslash or bytes that are not UTF-8.
var errBadPath = errors.New("not a path of a tree")
