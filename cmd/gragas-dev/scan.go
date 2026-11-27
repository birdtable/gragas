package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-proc/gitenv"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/scan"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// runScan is the scan command: the names scan over the tree in -C, which
// `task scan-names` runs by hand. No tier runs it (#432 replaces it). env is
// the environment as main reads it, from which the scan takes the path of
// its list; environ is the same environment whole, which its programs run
// with.
func runScan(ctx context.Context, env func(string) (string, bool), environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("scan", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the repository to scan")
	// The flags may come before the scan's name or after it.
	err := fl.Parse(args)
	name := fl.Arg(0)
	if err == nil && name != "" {
		err = fl.Parse(fl.Args()[1:])
	}
	if err != nil || name != "names" || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		talk.Text("gragas-dev scan: " + err.Error() + "\n")
		return exitFailed
	}
	h := &scanHost{root: root, env: env, environ: environ, mounts: func() ([]string, error) { return settingsMounts(env) }}
	lines, err := scan.Names(ctx, h.machine())
	err = errors.Join(err, h.remove())
	out := &say.Speaker{W: stdout}
	for _, l := range lines {
		out.Text(l + "\n")
	}
	if err != nil {
		talk.Text("gragas-dev scan " + name + ": " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// scanHost is the host as the scan sees it, over the tree at root, with the
// temp folders it made.
type scanHost struct {
	root  string
	temps []string
	// removes removes each temp folder made, as FreshFolder answered it.
	removes []func() error
	// env reads the environment, as main hands it in; nil reads nothing.
	env func(string) (string, bool)
	// environ is the environment main read, which every program the scan
	// starts runs with under gitenv.Env.
	environ []string
	// mounts answers the settings' mounts, on whose disks the hashed list is
	// judged by its owners, read only once a list is named; nil is none.
	mounts func() ([]string, error)
}

// settingsMounts is the mounts of the settings env names, as config.Load
// reads them for the product: GRAGAS_MOUNTS, then the settings file's mounts
// key, each file read by host/trust's owned read (gragas#1897).
func settingsMounts(env func(string) (string, bool)) ([]string, error) {
	getenv := func(k string) string {
		v, _ := env(k)
		return v
	}
	cfg, err := config.Load(getenv, "", hosttrust.ReadOwned)
	return cfg.Mounts, err
}

func (h *scanHost) machine() scan.Machine {
	return scan.Machine{Temp: h.temp, Write: h.write, Run: h.run, Git: h.git, TrackedFiles: h.trackedFiles, NamesList: h.namesList, GitFile: h.gitFile, Root: h.root, GitEnv: scanGitAdd(), Within: within}
}

// within is whether p is root or lies below it, by this host's rules for a path.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && filepath.IsLocal(rel)
}

// scanGitAdd is what a scan's git runs with over gitenv.Env, which has already
// dropped every GIT_ of this process's and set no system config: no global
// config either, and no replace ref, so it shows the commits a push sends.
func scanGitAdd() []string {
	return append(gitenv.NoGlobalConfig(), "GIT_NO_REPLACE_OBJECTS=1")
}

// temp makes a folder the scan removes when it ends: host/trust's
// FreshFolder (gragas#2264), fresh at 0700 in the account's own temp folder,
// kept apart from the tree it scans.
func (h *scanHost) temp(context.Context) (string, error) {
	d, remove, err := hosttrust.FreshFolder([]string{h.root}, "gragas-scan-")
	if err != nil {
		return "", err
	}
	h.temps = append(h.temps, d)
	h.removes = append(h.removes, func() error {
		if err := remove(); err != nil {
			return fmt.Errorf("the temp folder %s is left: %w", d, err)
		}
		return nil
	})
	// The folder as go-files' confine.Resolve follows it, one hop at a time
	// and a junction among them on Windows, which filepath.EvalSymlinks has
	// not followed since Go 1.23 (gragas#2265). FreshFolder's parent has no
	// link in it today; the scan's write check does not lean on that.
	resolved, err := confine.Resolve(d)
	if err != nil {
		return "", err
	}
	h.temps = append(h.temps, resolved)
	return resolved, nil
}

// remove removes every temp folder, and says which it could not.
func (h *scanHost) remove() error {
	var errs []error
	for _, r := range h.removes {
		errs = append(errs, r())
	}
	return errors.Join(errs...)
}

// write writes under a temp folder only.
func (h *scanHost) write(p string, data []byte) error {
	if !underAny(p, h.temps) {
		return fmt.Errorf("a scan writes only under its temp folders, not %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// run runs prog with the network blocked, as every tool the gate starts,
// and returns its streams and exit status; an error is a program that
// could not start. The block nests inside the one make check runs its
// members in, and holds a scan run by hand as well.
func (h *scanHost) run(ctx context.Context, dir string, env []string, prog string, args ...string) (scan.Result, error) {
	if dir == "" {
		dir = h.root
	}
	// Through proc.Run, its tree killed whole at outputTimeout or when ctx
	// ends (gragas#2263); the program's own exit code is the scan's to read.
	res, err := progRun{name: prog, dir: dir, args: args, env: gitenv.Env(h.environ, env...), timeout: outputTimeout}.do(ctx)
	if err != nil {
		return scan.Result{}, fmt.Errorf("%s did not run to its end: %w", filepath.Base(prog), err)
	}
	return scan.Result{Code: res.Code, Stdout: res.Stdout, Stderr: res.Stderr}, nil
}

// git is git as every gate step starts it.
func (h *scanHost) git() (string, error) { return exec.LookPath("git") }

// trackedFiles reads every file git tracks in dir, the tree for "" or a
// temp folder of this scan, as it is on disk: a link as its target, which
// is what git stores and a push publishes. A tracked file deleted from disk
// and a name that is neither, such as a submodule, are left out. git reads
// no system or global config here (scanGit).
func (h *scanHost) trackedFiles(ctx context.Context, dir string) (map[string][]byte, error) {
	dir, err := h.listed(dir)
	if err != nil {
		return nil, err
	}
	out, err := scanGit(ctx, h.environ, dir, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, name := range strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }) {
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return nil, fmt.Errorf("git listed %q, which is not inside %s", name, dir)
		}
		b, ok, err := trackedBytes(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		if ok {
			files[name] = b
		}
	}
	return files, nil
}

// namesList is the path scan.NamesHashesVar names and the file's bytes, or
// "" when it is unset. A path that is not absolute and clean is refused
// before anything is read. The path is native, a value the environment hands
// us and never the model's slash form, so it is clean as this system cleans
// it, filepath.Clean(p) == p (gragas#1668's rule).
func (h *scanHost) namesList() (string, []byte, error) {
	if h.env == nil {
		return "", nil, nil
	}
	p, _ := h.env(scan.NamesHashesVar)
	switch {
	case p == "":
		return "", nil, nil
	case !filepath.IsAbs(p) || filepath.Clean(p) != p:
		return "", nil, fmt.Errorf("%s must be an absolute, clean path", scan.NamesHashesVar)
	}
	// The list is read as the names file is, by the owned read (gragas#1801),
	// on the settings' mounts, as the product reads the names file
	// (gragas#1897): an account that could write it could take hashes out,
	// and the scan would pass on names it no longer looks for. The refusal's
	// words name the path, so none of them is said.
	var mounts []string
	if h.mounts != nil {
		var err error
		if mounts, err = h.mounts(); err != nil {
			return "", nil, errSettings
		}
	}
	b, err := (&hostfs.Tree{Mounts: mounts}).ReadOwned(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil, fmt.Errorf("no file is there: %w", fs.ErrNotExist)
	case err != nil:
		return "", nil, errHashesRefused
	}
	return p, b, nil
}

// errHashesRefused is why the hashed list was not read when the owned read
// refused it, and errSettings why it was not read when the settings that name
// the mounts could not be: words that hold no part of the list's path or the
// settings', either of which may hold a login.
var (
	errHashesRefused = errors.New("it is read only as this account's own file: a regular file at the path, not a link, owned by this account or root, that no other account can write, that this account can read, of 1 MiB or less, on a disk whose owners are judged or one mounted on a folder " + hosttrust.MountsVar + " or the " + hosttrust.MountsKey + " setting names; where root mounted the list's disk for this machine, name the folder it is mounted on in " + hosttrust.MountsVar + " or the " + hosttrust.MountsKey + " setting")
	errSettings      = errors.New("the settings could not be read, so the mounts " + hosttrust.MountsVar + " and the " + hosttrust.MountsKey + " setting name are not known, and nothing is read: gragas says why")
)

// pathless is err without the path a file error holds, so a message about a
// names list never says where it lies: the path may hold a login, and a
// login may be on the list.
func pathless(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return fmt.Errorf("%s: %w", pe.Op, pe.Err)
	}
	return err
}

// gitFile is whether the repository in dir, the tree when "" or a temp
// folder of this scan, holds rel in its git folder, of any kind, as git
// rev-parse --git-path names it. git reads no config of the host's here.
func (h *scanHost) gitFile(ctx context.Context, dir, rel string) (bool, error) {
	dir, err := h.listed(dir)
	if err != nil {
		return false, err
	}
	if !filepath.IsLocal(filepath.FromSlash(rel)) {
		return false, fmt.Errorf("a scan asks for a path inside the git folder, not %s", rel)
	}
	out, err := scanGit(ctx, h.environ, dir, "rev-parse", "--path-format=absolute", "--git-path", rel)
	if err != nil {
		return false, err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		return false, fmt.Errorf("git rev-parse printed %q for %s, not an absolute path", p, rel)
	}
	_, err = os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, pathless(err)
	}
	return true, nil
}

// scanGit runs git with args, as a scan starts it on the tree or a temp
// folder of its own, dir, and returns what it wrote to stdout, as output
// does: gitenv.ReadArgs's arguments, so it reads dir alone
// and runs no hook and no file system monitor dir's config names, in
// gitenv.Env's environment with no global config. A git hook exports GIT_DIR,
// GIT_INDEX_FILE and others to the tier it runs, and a git that read them
// would work on the repository that runs the gate, not in dir: a plant's
// commit would land on the branch being pushed. gitenv.Env drops every GIT_
// of this process's, whatever its case (as run does for the scan's own git).
// Its -C dir names the folder, so it starts with no Dir (outputIn's ""): a
// relative dir would be read twice. environ is the environment main read.
func scanGit(ctx context.Context, environ []string, dir string, args ...string) ([]byte, error) {
	return outputIn(ctx, "", gitenv.Env(environ, gitenv.NoGlobalConfig()...), "git", gitenv.ReadArgs(dir, args...)...)
}

// listed is the folder a scan lists the files of: the tree for "", else a
// temp folder of this scan, and no other.
func (h *scanHost) listed(dir string) (string, error) {
	if dir == "" {
		return h.root, nil
	}
	if !underAny(dir, h.temps) {
		return "", fmt.Errorf("a scan lists only the tree or its temp folders, not %s", dir)
	}
	return dir, nil
}

// trackedFiles reads every file git tracks in dir, the tree for "" or a

// regular file's bytes, and nothing, with ok false, for a name gone from
// disk or one that is neither.
func trackedBytes(p string) ([]byte, bool, error) {
	fi, err := os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case fi.Mode().Type() == fs.ModeSymlink:
		target, err := os.Readlink(p)
		// git stores a link's target with forward slashes, which is what a push
		// publishes and the digest is taken over; a Windows Readlink gives them
		// as backslashes, so ToSlash makes the target what git stores on every
		// system (gragas#1371).
		return []byte(filepath.ToSlash(target)), err == nil, err
	case fi.Mode().IsRegular():
		b, err := os.ReadFile(p)
		return b, err == nil, err
	}
	return nil, false, nil
}

// underAny is whether p is inside one of the folders dirs.
func underAny(p string, dirs []string) bool {
	for _, d := range dirs {
		if rel, err := filepath.Rel(d, p); err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}
