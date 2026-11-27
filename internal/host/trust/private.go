package trust

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	gotrust "github.com/birdtable/go-files/trust"
)

// Private answers dir, absolute and clean, as a root, once every name of it
// that is there is judged and what is missing is made, by go-files's judged
// private make: the nearest part of dir that is there is walked
// for who before anything is made, so nothing is made, nor opened, through a
// name the walk refuses, a link in a folder another account may write among
// them; each folder that is missing is then made private to this account,
// below the folder the walk reached, with no link followed; and dir is judged
// again once it is there. Who.Apart is go-files's, held by both walks, so no
// folder of dir is made through a link into a folder of Apart.
//
// A dir that another run makes, or writes in, while this call makes it is not
// refused: go-files sends the call back to the judge, which finds the folder
// there and judges it as one that was there, and refuses it with ErrReplaced
// only after 8 such tries.
//
// A dir that is there is walked as s's folder with Walk and opened, nothing
// made, and it keeps its mode, for its caller to judge its owner: go-files's
// make would refuse a folder that is there and not private, which a state
// folder or a cache a person made at 0755 is and has been accepted as.
//
// A refusal of a name the walk does not trust is an *Unsafe in the words of
// s, as Walk gives it; any other error is go-files's, for its caller to word.
// It is the one place host/fs's state folder and host/git's releases cache
// are made, and the sandbox's make shares it (gate/shared.yaml, private-dir).
func Private(dir string, s Subject, who Who) (*os.Root, error) {
	_, err := os.Lstat(dir)
	switch {
	case err == nil:
		if _, err := Walk(s, dir, who); err != nil {
			return nil, err
		}
		return os.OpenRoot(dir)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	admin, err := group()
	if err != nil {
		return nil, &Failed{What: "the admin group could not be read, so the path of " + s.Name + " is not trusted", Err: err}
	}
	made, err := mkdirPrivate(dir, gotrust.Who{RootOnly: who.RootOnly, NoLinks: who.NoLinks, Group: admin, Apart: who.Apart, Mounts: who.Mounts})
	if u, ok := errors.AsType[*gotrust.Unsafe](err); ok {
		return nil, s.unsafe(dir, u, err)
	}
	return made, err
}

// mkdirPrivate is go-files's judged private make of dir for who, asked as
// walk asks: in a test binary first with Who.Overflow, and again without it
// where go-files refuses that with ErrOverflow.
func mkdirPrivate(dir string, who gotrust.Who) (*os.Root, error) {
	if testing.Testing() {
		in := who
		in.Overflow = true
		if made, err := gotrust.MkdirPrivate(dir, in); !errors.Is(err, gotrust.ErrOverflow) {
			return made, err
		}
	}
	return gotrust.MkdirPrivate(dir, who)
}

// Nearest is the longest part of dir that is there, dir itself included:
// each name read by Lstat, so a link at the last name found is that name,
// for the judge to follow and judge.
func Nearest(dir string) (string, error) {
	top := dir
	for {
		_, err := os.Lstat(top)
		if err == nil {
			return top, nil
		}
		up := filepath.Dir(top)
		if !errors.Is(err, fs.ErrNotExist) || up == top {
			return "", err
		}
		top = up
	}
}
