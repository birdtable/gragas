package trust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-files/appdirs"
	"github.com/birdtable/go-files/confine"
	gotrust "github.com/birdtable/go-files/trust"
)

// StableTemp makes the folder the cached members of the Taskfile hand Go's
// test cache as TMPDIR, at mode 0700, and returns it once its way from / down
// is trusted as the running account. go-files's
// appdirs.Temp makes and judges the account's own temp folder for gragas,
// <temp>/gragas-<uid> on Unix and <temp>\gragas on Windows, with no link in
// it, and the folder tmp is made in it, so the path is fixed for each user and
// the test cache sees one on every run. A folder a group can write, or below a
// parent one can, is refused as an *Unsafe, whose message the caller prints, so
// the refusal is the trust walk's and not red tests later. A temp folder that
// is, lies inside or holds one of apart (the work tree) is refused with an
// *Unsafe too. Each of apart is made absolute and must be there:
// one that is not is an *Unsafe, and an empty one is left out.
func StableTemp(apart []string) (string, error) {
	kept, err := workTrees(apart)
	if err != nil {
		return "", err
	}
	dir, err := accountTemp(homeApp, gotrust.Who{Apart: kept})
	if err != nil {
		return "", refusal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	tmp, err := confine.Mkdir(root, "tmp", 0o700)
	if err != nil {
		return "", errors.Join(err, root.Close())
	}
	if err := errors.Join(tmp.Close(), root.Close()); err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmp"), nil
}

// accountTemp is go-files's appdirs.Temp of app for who, asked as walk asks
// the trust walk: in a test binary first with appdirs.TempOverflow, which
// go-files grants only on Linux in a user namespace that maps this account and
// not root, where root's / shows as the overflow account's, and
// again with Temp where go-files refuses that with ErrOverflow.
// A program asks Temp once.
func accountTemp(app string, who gotrust.Who) (string, error) {
	if testing.Testing() {
		if dir, err := appdirs.TempOverflow(app, who); !errors.Is(err, gotrust.ErrOverflow) {
			return dir, err
		}
	}
	return appdirs.Temp(app, who)
}

// workTrees is apart as appdirs.Temp takes it: no empty entry, each absolute
// and clean, and each checked to be there before anything is made.
func workTrees(apart []string) ([]string, error) {
	var out []string
	for _, a := range apart {
		if a == "" {
			continue
		}
		abs, err := filepath.Abs(a)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, &Unsafe{Observed: "the work tree " + Quote(abs) + " cannot be checked: " + err.Error(), Fix: "run in a folder that exists"}
		}
		out = append(out, abs)
	}
	return out, nil
}

// temp names the temp folder for a person, the folder of a refusal.
func temp() Subject {
	return Subject{Name: "the temp folder", File: "the temp folder", Last: Folder, Bare: true}
}

// refusal is err of appdirs.Temp in the words of the temp folder: a temp
// folder that is, lies inside or holds the work tree as an *Unsafe that
// says so, whether go-files's walk refused it by Who.Apart (an *Unsafe
// matching ErrInside, which carries no fix of its own) or its last check
// did; go-files's other refusals of the walk as an *Unsafe
// with its fix line; and any other error as it is.
func refusal(err error) error {
	if errors.Is(err, gotrust.ErrInside) {
		return &Unsafe{Observed: "the temp folder is, or is inside, or holds, the work tree, where a head can commit a file the tools read: " + err.Error(),
			Fix: "give the temp folder a folder outside the work tree"}
	}
	if u, ok := errors.AsType[*gotrust.Unsafe](err); ok {
		return temp().unsafe("", u, err)
	}
	return err
}

// RunHome makes a Home of its own for one run in the account's own temp folder
// for gragas (appdirs.Temp, as StableTemp), a fresh folder at mode 0700
// (confine.MkdirFresh) beside the runs of any other, kept apart from the folders
// of apart. It answers the folder and the call that removes it and all it
// holds, which the caller makes when the run ends.
func RunHome(apart []string) (dir string, remove func() error, err error) {
	return fresh(homeApp, apart, homePrefix)
}

// homeApp is the app whose account temp folder holds the stable tmp and every
// Home, the folder SweepHomes reads; workApp is the app whose account temp
// folder holds every FreshFolder, and nothing else; treeApp is the app whose
// account temp folder holds every TreeFolder, and nothing else. No one of the
// three is inside another, so a work tree made in a TreeFolder is never inside
// the folder StableTemp, RunHome, SweepHomes or FreshFolder keeps apart from it.
const (
	homeApp = "gragas"
	workApp = "gragas-work"
	treeApp = "gragas-tree"
)

// homePrefix starts the name of every Home RunHome makes, and of no folder
// FreshFolder makes, so SweepHomes takes the one and never the other.
const homePrefix = "run-"

// FreshFolder makes a folder of its own for one use in the account's own temp
// folder for gragas-work (appdirs.Temp, <temp>/gragas-work-<uid> on Unix),
// fresh, at mode 0700 (confine.MkdirFresh), named prefix and a random tail,
// kept apart from the folders of apart. It is not in the folder of the Homes
// and the stable tmp, so a work tree made in it, as the pre-push hook makes
// one, is not inside the folder StableTemp, RunHome and SweepHomes keep apart
// from that tree. It is never swept: SweepHomes reads only the folder of the
// Homes, and takes only a Home there, so a FreshFolder lives as long as its
// use, a pre-push worktree's hour as much as a scan's second, and the caller
// removes it with the call it answers when the use ends (gragas#2264). A
// killed use's leftover is found under gragas-work alone (gragas#2465). A
// prefix that starts as a Home's is refused, a second guard.
func FreshFolder(apart []string, prefix string) (dir string, remove func() error, err error) {
	if strings.HasPrefix(prefix, homePrefix) {
		return "", nil, fmt.Errorf("a fresh folder named %q would be swept as a run's Home: its prefix starts %q", prefix, homePrefix)
	}
	return fresh(workApp, apart, prefix)
}

// TreeFolder is FreshFolder for a folder that will hold a work tree, as the
// pre-push hook makes one: made in the account's own temp folder for
// gragas-tree, not gragas-work, so a FreshFolder that a tool run in that tree
// keeps apart from it, as boards and the names scan do, is not refused for a
// temp folder that holds the tree.
func TreeFolder(apart []string, prefix string) (dir string, remove func() error, err error) {
	if strings.HasPrefix(prefix, homePrefix) {
		return "", nil, fmt.Errorf("a fresh folder named %q would be swept as a run's Home: its prefix starts %q", prefix, homePrefix)
	}
	return fresh(treeApp, apart, prefix)
}

// fresh is RunHome and FreshFolder: a folder named prefix and a random tail,
// made fresh in the account's temp folder for app, and its removal.
func fresh(app string, apart []string, prefix string) (dir string, remove func() error, err error) {
	kept, err := workTrees(apart)
	if err != nil {
		return "", nil, err
	}
	parent, err := accountTemp(app, gotrust.Who{Apart: kept})
	if err != nil {
		return "", nil, refusal(err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return "", nil, err
	}
	made, name, err := confine.MkdirFresh(root, prefix)
	if err != nil {
		return "", nil, errors.Join(err, root.Close())
	}
	if err := made.Close(); err != nil {
		return "", nil, errors.Join(err, root.Close())
	}
	// The Home is this account's own, and a program in it may have left a
	// folder read-only (0555), as the go command leaves a module cache:
	// go-files' confine.RemoveAllOwned gives each such folder its owner's
	// write bit before it empties it, and follows no link.
	return filepath.Join(parent, name), func() error {
		return errors.Join(confine.RemoveAllOwned(root, name), root.Close())
	}, nil
}

// SweepHomes removes every Home of RunHome that no live run can hold, and
// answers how many: each run-* folder in the account's temp
// folder not written in for older, the longest a step lives. A younger one is
// held, and left; so is any name RunHome could not have made. Every removal is
// made through the folder as a root, so no link is followed.
func SweepHomes(apart []string, older time.Duration) (n int, err error) {
	kept, err := workTrees(apart)
	if err != nil {
		return 0, err
	}
	parent, err := accountTemp(homeApp, gotrust.Who{Apart: kept})
	if err != nil {
		return 0, refusal(err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	entries, err := dir.ReadDir(-1)
	if cerr := dir.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, homePrefix) || !e.IsDir() {
			continue
		}
		if info, ierr := e.Info(); ierr != nil || time.Since(info.ModTime()) < older {
			continue
		}
		swept, rerr := sweepOne(root, name)
		if rerr != nil {
			err = errors.Join(err, rerr)
			continue
		}
		if swept {
			n++
		}
	}
	return n, err
}

// sweepTries is how often sweepOne goes at a Home another sweep is also
// removing: each pass finds less of it, so a few end it.
const sweepTries = 5

// sweepOne removes the Home name below root and says whether this sweep did.
// Another sweep, in a second run of gragas, may remove the same old Home at the
// same time, and the walk of the slower says so as "not exist" or as a
// refusal ("a component changed") while the Home is half there. So an error
// is met with another pass; a Home that is gone by then was swept by the other,
// which is no error and not counted. A Home still there after the passes keeps
// the last error.
func sweepOne(root *os.Root, name string) (swept bool, err error) {
	for range sweepTries {
		if err = confine.RemoveAllOwned(root, name); err == nil {
			return true, nil
		}
		if _, serr := root.Lstat(name); errors.Is(serr, fs.ErrNotExist) {
			return false, nil
		}
	}
	return false, err
}
