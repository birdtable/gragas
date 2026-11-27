package tool

// The cache folders of a grant's cache_vars. A cache that steps share is a
// path from one step into the next: what a step writes there, a later step
// of the same grant reads. So the folder is keyed by everything the person
// approved and the project it ran for (the key is the host's, and the
// contract promises nothing of the folder), the names are the ones the
// approval showed, and the opener has refused any name the run's own
// environment sets (Reserved).

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/birdtable/go-files/confine"
	gotrust "github.com/birdtable/go-files/trust"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// cacheKeyLen is how many hex digits of the key's sum name its folder: 128
// bits, which no two keys of one machine share, and a short path on a system
// that bounds one.
const cacheKeyLen = 32

// cacheKey is the name of the folder of one plugin, its version, one of its
// grants and one project: the sum of the four as a canonical array, so no
// spelling of one runs into the next and no name of them reaches the path.
func cacheKey(name, version, grant, root string) (string, error) {
	b, err := canon.Encode(canon.Array{canon.String(name), canon.String(version), canon.String(grant), canon.String(root)}, canon.Canonical)
	if err != nil {
		return "", err
	}
	return digest.Hex(b)[:cacheKeyLen], nil
}

// homeCacheFolder is the name of the folder below a key folder that a step's
// Home cache is kept in (proc.HomeEnvCache). No cache_vars name can be it, on
// a file system that folds case or one that does not: a name holds only
// capitals, digits and underscores, and this one holds a hyphen.
const homeCacheFolder = "home-cache"

// stepCache is what a step of a grant that gives cache_vars is given to keep
// its caches in between steps: NAME=folder for each name, in the grant's
// order, and home, the folder go-proc points its Home's cache at
// (proc.HomeEnvCache). The zero stepCache keeps nothing.
type stepCache struct {
	vars []string
	home string
}

// cacheEnv is the stepCache of the grant id, for a step of this read: each
// name a folder of its own below the key folder (cacheKey) in the config's
// Cache, and the Home's cache one more there (homeCacheFolder), so the
// approval that showed the names is the yes to a kept cache, and a grant that
// gives none keeps nothing from one step into the next. The folder above the
// Cache is walked by host/trust's Private, with the project and the scratch
// folder held apart, and opened if it is there or made private if not
// (gate/shared.yaml, private-dir), so nothing is made below the project or
// where another account may write. From the Cache down to each variable's
// folder, every level is then made or opened by handle through go-files'
// confine, which follows no link: every step's program runs
// as this account, so a link one step left at any of those levels, wherever
// it points, could be another step's and is refused before the next step
// starts. Each level, once made or opened, is held to this account by its
// handle (cacheHeld), since a make by handle keeps the mode of a folder that
// was there. What a variable's folder holds is the tool's, and is not walked. A
// grant with no cache variables, or a config with no Cache, answers the zero
// stepCache and makes nothing: the program then keeps every cache in the
// step's Home.
func (b *bound) cacheEnv(id string) (stepCache, error) {
	g, err := grantOf(b.scope.Manifest, id)
	if err != nil || len(g.cache) == 0 || b.t.cfg.Cache == "" {
		return stepCache{}, err
	}
	if strings.ContainsFunc(b.t.cfg.Cache, unicode.IsControl) {
		return stepCache{}, refused("the tools cache holds a control character", "point the cache folder at a folder whose name holds none")
	}
	if fault := pathFault(b.t.cfg.Cache); fault != "" {
		return stepCache{}, refused("the tools cache "+hosttrust.Quote(b.t.cfg.Cache)+" "+fault, "point the cache folder at an absolute, clean folder")
	}
	key, err := cacheKey(b.scope.Manifest.Name, b.scope.Manifest.Version, id, string(b.scope.Root))
	if err != nil {
		return stepCache{}, fmt.Errorf("the key of the grant %s's cache: %w", hosttrust.Quote(id), err)
	}
	apart := []string{filepath.FromSlash(string(b.scope.Root)), filepath.FromSlash(b.scope.Scratch.Dir)}
	cache := filepath.FromSlash(b.t.cfg.Cache)
	what := "the tools cache " + hosttrust.Quote(b.t.cfg.Cache) + ": "
	tools, err := cacheTools(cache, hosttrust.Who{Mounts: b.t.cfg.Mounts, Apart: apart})
	if err != nil {
		return stepCache{}, cacheRefusal(what, err)
	}
	var sc stepCache
	if err = cacheHeld(tools, what, b.t.cfg.Mounts); err == nil {
		sc, err = cacheFolders(tools, cache, key, id, g.cache, b.t.cfg.Mounts)
	}
	if err = errors.Join(err, tools.Close()); err != nil {
		return stepCache{}, err
	}
	return sc, nil
}

// cacheSubject is the tools cache in the words of every walk of it.
func cacheSubject() hosttrust.Subject {
	return hosttrust.Subject{Name: "the tools cache", File: "the tools cache", Last: hosttrust.Folder, Bare: true,
		Point: "point this account's cache folder there"}
}

// cacheTools is the tools cache as a root: the folder above it walked for
// who and opened, or made private, by Private, then the Cache's own name made
// or opened below it by handle, so a link at that name is refused
// (ErrOutside), and walked as a name once there, so the project itself is
// refused too.
func cacheTools(cache string, who hosttrust.Who) (*os.Root, error) {
	above, err := hosttrust.Private(filepath.Dir(cache), cacheSubject(), who)
	if err != nil {
		return nil, err
	}
	tools, err := confine.MkdirPrivate(above, filepath.Base(cache))
	if err = errors.Join(err, above.Close()); err != nil {
		if tools != nil {
			err = errors.Join(err, tools.Close())
		}
		return nil, err
	}
	if _, err := hosttrust.Walk(cacheSubject(), cache, who); err != nil {
		return nil, errors.Join(err, tools.Close())
	}
	return tools, nil
}

// cacheFolders makes or opens the key folder below tools, the root of the
// folder cache, and below that each NAME of the grant id and the Home's
// cache folder (homeCacheFolder), by handle, holds each to this account by
// its handle once made (cacheHeld), and answers the stepCache: NAME=folder
// for each name in order, and the Home cache's folder.
func cacheFolders(tools *os.Root, cache, key, id string, names, mounts []string) (sc stepCache, err error) {
	what := "the key folder of the grant " + hosttrust.Quote(id) + ": "
	keyed, err := confine.MkdirPrivate(tools, key)
	if err != nil {
		return stepCache{}, cacheRefusal(what, err)
	}
	defer func() {
		if err = errors.Join(err, keyed.Close()); err != nil {
			sc = stepCache{}
		}
	}()
	if err := cacheHeld(keyed, what, mounts); err != nil {
		return stepCache{}, err
	}
	made := func(name, what string) (string, error) {
		f, err := confine.MkdirPrivate(keyed, name)
		if err != nil {
			return "", cacheRefusal(what, err)
		}
		if err := errors.Join(cacheHeld(f, what, mounts), f.Close()); err != nil {
			return "", err
		}
		return filepath.Join(cache, key, name), nil
	}
	sc.vars = make([]string, 0, len(names))
	for _, name := range names {
		dir, err := made(name, "the folder of "+name+" for the grant "+hosttrust.Quote(id)+": ")
		if err != nil {
			return stepCache{}, err
		}
		sc.vars = append(sc.vars, name+"="+dir)
	}
	if sc.home, err = made(homeCacheFolder, "the Home's cache folder for the grant "+hosttrust.Quote(id)+": "); err != nil {
		return stepCache{}, err
	}
	return sc, nil
}

// cacheHeld judges the folder root stands for by its handle, as the
// sandbox judges its scratch parent: owned by this account, and written by no
// other, as go-files' Held judges it through host/trust's Held, with mounts.
// A make by handle keeps the mode and owner of a folder that was there
// (confine.Mkdir), so a level of the cache an earlier step left open to
// others, or a folder of another account's there, is refused before the next
// step starts. what names the folder.
func cacheHeld(root *os.Root, what string, mounts []string) (err error) {
	const fix = "remove that folder, and gragas makes it again, private to this account, at the next step"
	f, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("%sit could not be opened: %w", what, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	owner, err := gotrust.OwnerOf(f)
	if err != nil {
		return refused(what+"its owner cannot be read: "+err.Error(), fix)
	}
	me, err := gotrust.Account()
	if err != nil || me == "" {
		return refused(what+"this account cannot be read, so no folder can be held to it", "run gragas as an account the system can name")
	}
	if owner != me {
		return refused(what+"it is owned by account "+owner+", not by this one ("+me+")", fix)
	}
	err = hosttrust.Held(f, hosttrust.Folder, mounts)
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return refused(what+u.Observed, u.Fix)
	}
	if errors.Is(err, gotrust.ErrMount) {
		return refused(what+"it is on a disk whose owners are not judged", hosttrust.MountFix)
	}
	if u, ok := errors.AsType[*gotrust.Unsafe](err); ok {
		return refused(what+u.Observed, fix)
	}
	if err != nil {
		return refused(what+"it can be written by another account, or who may write it cannot be judged where it is: "+err.Error(), fix)
	}
	return nil
}

// cacheRefusal is what a make of a cache folder answering err comes to, what
// naming the folder: a name the walk does not trust, one it could not read,
// and a link or a file where a folder is made by handle (ErrOutside) are
// refusals, and nothing starts; any other fault is the make's own.
func cacheRefusal(what string, err error) error {
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return refused(what+u.Observed, cmp.Or(u.Fix, "point the cache folder at a folder of this account's, outside the project, that only it can write"))
	}
	if f, ok := errors.AsType[*hosttrust.Failed](err); ok {
		return refused(what+f.Error(), "point the cache folder at a folder this account can read")
	}
	if errors.Is(err, confine.ErrOutside) {
		return refused(what+"a link, or not a folder, stands on the way: "+err.Error(), "remove it from the cache folder; gragas follows no link there, as a step could have left it")
	}
	return fmt.Errorf("%s%w", what, err)
}

// SweepCaches removes every key folder of the tools cache, and all each
// holds, and answers how many it removed (port.Steps).
func (t *Tool) SweepCaches() (int, error) { return t.keyFolders(true) }

// CachesLeft answers how many key folders SweepCaches would remove now, and
// writes nothing (port.Steps).
func (t *Tool) CachesLeft() (int, error) { return t.keyFolders(false) }

// keyFolders counts the entries of the config's Cache at a name a key folder
// takes (keyName), and with remove removes each, and all it holds, through
// go-files' confine.RemoveAllOwned: a folder a program left read-only is
// given its owner's write bit to be emptied, and a link anywhere below is
// removed, never followed. The folder above the Cache is walked as a step
// judges it (host/trust's Walk), then opened with nothing made; the Cache's
// own name is walked in it by handle
// (confine.NoLinks), so a link there is refused before anything is read
// below it; and the Cache is opened, held to be the folder walked, and held
// to this account as a step holds it (cacheHeld). Each entry is removed by
// its name in that one handle, so nothing outside the Cache is reached. An
// entry at another name is left: gragas makes none there. No Cache, set or
// there, answers 0; the first entry that cannot be removed ends the sweep,
// and the ones before it stay removed.
func (t *Tool) keyFolders(remove bool) (n int, err error) {
	if t.cfg.Cache == "" {
		return 0, nil
	}
	if strings.ContainsFunc(t.cfg.Cache, unicode.IsControl) {
		return 0, refused("the tools cache holds a control character", "point the cache folder at a folder whose name holds none")
	}
	if fault := pathFault(t.cfg.Cache); fault != "" {
		return 0, refused("the tools cache "+hosttrust.Quote(t.cfg.Cache)+" "+fault, "point the cache folder at an absolute, clean folder")
	}
	cache := filepath.FromSlash(t.cfg.Cache)
	what := "the tools cache " + hosttrust.Quote(t.cfg.Cache) + ": "
	parent := filepath.Dir(cache)
	if _, err := os.Lstat(parent); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	judge := func(p string) error {
		_, err := hosttrust.Walk(cacheSubject(), p, hosttrust.Who{Mounts: t.cfg.Mounts})
		return err
	}
	if err := judge(parent); err != nil {
		return 0, cacheRefusal(what, err)
	}
	above, err := os.OpenRoot(parent)
	if err != nil {
		return 0, cacheRefusal(what, err)
	}
	tools, ok, err := heldTools(above, filepath.Base(cache), what)
	if err = errors.Join(err, above.Close()); err != nil || !ok {
		if tools != nil {
			err = errors.Join(err, tools.Close())
		}
		return 0, err
	}
	defer func() { err = errors.Join(err, tools.Close()) }()
	if err := judge(cache); err != nil {
		return 0, cacheRefusal(what, err)
	}
	if err := cacheHeld(tools, what, t.cfg.Mounts); err != nil {
		return 0, err
	}
	names, err := keyNames(tools)
	if err != nil {
		return 0, fmt.Errorf("%sits folders could not be listed: %w", what, err)
	}
	if !remove {
		return len(names), nil
	}
	for _, name := range names {
		if err := confine.RemoveAllOwned(tools, name); err != nil {
			return n, cacheRefusal(what+"the key folder "+hosttrust.Quote(name)+" could not be removed: ", err)
		}
		n++
	}
	return n, nil
}

// heldTools is the Cache, named name in above, as a root: walked by handle,
// so a link at name, or a file there, is refused, and held, once opened, to
// be the folder walked. ok is false, with no error, when name is not there.
func heldTools(above *os.Root, name, what string) (tools *os.Root, ok bool, err error) {
	walked, err := confine.NoLinks(above, name)
	if err != nil {
		return nil, false, cacheRefusal(what, err)
	}
	if walked == nil {
		return nil, false, nil
	}
	if !walked.IsDir() {
		return nil, false, refused(what+"it is not a folder", "point the cache folder at a folder, or remove what stands at its name")
	}
	tools, err = above.OpenRoot(name)
	if err != nil {
		return nil, false, fmt.Errorf("%sit could not be opened: %w", what, err)
	}
	opened, err := tools.Stat(".")
	if err == nil && !os.SameFile(walked, opened) {
		err = refused(what+"it changed between its walk and its open", "run the clear again once nothing else writes the cache folder")
	}
	if err != nil {
		return nil, false, errors.Join(err, tools.Close())
	}
	return tools, true, nil
}

// keyNames is the names in tools that a key folder takes (keyName), in the
// order the folder lists them.
func keyNames(tools *os.Root) (names []string, err error) {
	f, err := tools.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if keyName(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// keyName is whether name is one cacheKey gives: cacheKeyLen lowercase hex
// digits.
func keyName(name string) bool {
	if len(name) != cacheKeyLen {
		return false
	}
	for _, r := range name {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
