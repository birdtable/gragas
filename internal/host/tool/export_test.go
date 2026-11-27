package tool

import (
	"context"
	"os"
	"time"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-proc/proc"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Bound sets the deadline and the cap of every later run, lower than the
// adapter's own, so a test of an ended run takes no minutes or megabytes.
func (t *Tool) Bound(wait time.Duration, maxOutput int) { t.wait, t.max = wait, maxOutput }

// Outcome is what a run of lint-a, held to the deadline Bound set and to a
// memory limit of mib (none below 1), comes to when go-proc's Run answers
// res and err under ctx: start's mapping past proc.Run, with go-proc's
// answer planted, so the mapping is held on every system with no process
// started.
func Outcome(t *Tool, ctx context.Context, mib int, res proc.Result, err error) (model.Output, error) {
	return t.outcome(ctx, launch{name: "lint-a", shape: shapeStep, path: "/tools/lint-a", wait: t.wait, mib: mib}, false, res, err)
}

// OverMemory is the end of a run named name, whose peak was peak MiB of
// kind when read, held to mib, judged after the run alone when judgedAfter.
func OverMemory(name string, peak int, kind string, read, judgedAfter bool, mib int) *Ended {
	return overMemory(name, peak, kind, read, judgedAfter, mib)
}

// LeftUnread is the end of a run named name, held to mib, that left a
// process of its tree running.
func LeftUnread(name string, mib int) *Ended { return leftUnread(name, mib) }

// TreeMemory is the bound, in bytes, a run held to mib has its tree held to
// where go-proc holds one.
func TreeMemory(mib int) uint64 { return treeMemory(mib) }

// Drain is how long every run reads a pipe held past the kill.
const Drain = drain

// VersionOf is the version the first line of out reads as, "" for none, and
// "" for a version that holds more after the patch.
func VersionOf(out string) string {
	if v, more, found := readVersion(out); found && more == "" {
		return v
	}
	return ""
}

// Trusted is the trust walk of the tool at p, with its refusals the
// adapter's own.
func Trusted(p string) (string, error) { return trust(subject(), p, nil) }

// TrustedOn is Trusted with mounts as the walk's Who.Mounts.
func TrustedOn(p string, mounts []string) (string, error) { return trust(subject(), p, mounts) }

// WalkRefusal is what the trust walk's answer err comes to in the adapter.
func WalkRefusal(err error) error { return walkRefusal(err) }

// WayRefused is the refusal of a folder the walk could not compare with the
// root.
func WayRefused(what string, root model.Root, u *hosttrust.Unsafe) error {
	return wayRefused(what, root, u)
}

// HomeOK is the check of the Home h against the project root and the
// folder of the call.
func HomeOK(h, root, dir string) error { return home(h, []string{root, dir}, nil) }

// FirstLine is firstLine: the first line of the file at p, read inside p's
// folder.
func FirstLine(p string) (string, error) { return firstLine(p) }

// MaxHops is how many scripts deep an interpreter chain is followed.
const MaxHops = maxHops

// PathFault is pathFault: why p is not an absolute, clean, slash-separated
// path, or "" when it is one.
func PathFault(p string) string { return pathFault(p) }

// PathFolder is pathFolder: a PATH folder in its clean slash form, and
// whether a pinned find may keep it.
func PathFolder(dir string) (string, bool) { return pathFolder(dir) }

// Interpreter is the interpreter check of the tool at target, env reading
// path and held to root, "" for none, as a find's runs are.
func Interpreter(t *Tool, target, path, root string) error {
	return t.interpreter(target, within{path: path, root: model.Root(root)})
}

// Env is the whole environment a run of t is given with the PATH path.
func Env(t *Tool, path string) []string {
	env, err := t.env(path, t.cfg.Home, nil, "")
	if err != nil {
		panic(err) // with no Home cache, env refuses nothing
	}
	return env
}

// EnvCached is Env with the step's cache variables cache, NAME=folder.
func EnvCached(t *Tool, path string, cache []string) []string {
	env, err := t.env(path, t.cfg.Home, cache, "")
	if err != nil {
		panic(err) // with no Home cache, env refuses nothing
	}
	return env
}

// EnvHomeCached is Env with the Home's cache kept in homeCache
// (proc.HomeEnvCache), or env's refusal of it.
func EnvHomeCached(t *Tool, path, homeCache string) ([]string, error) {
	return t.env(path, t.cfg.Home, nil, homeCache)
}

// HomeCacheFolder is the name of the folder below a grant's key folder that
// its steps' Home cache is kept in.
const HomeCacheFolder = homeCacheFolder

// CacheKey is the name of the cache folder of the plugin name at version, its
// grant and the project root.
func CacheKey(name, version, grant, root string) (string, error) {
	return cacheKey(name, version, grant, root)
}

// ManifestBytes is manifestBytes, for the boundary test of maxPackageJSON.
func ManifestBytes(dir, name string) ([]byte, error) { return manifestBytes(dir, name) }

const MaxPackageJSON = maxPackageJSON

// StartRefused is startRefused for a tool named name at path, which go-proc
// refused with err, so a test plants go-proc's own refusals on any system.
func StartRefused(name, path string, err error) error {
	return startRefused(launch{name: name, path: path}, err)
}

// StepWords is a step's words as the program is given them, in the scratch
// folder scratch.
func StepWords(argv []string, scratch string) ([]string, error) { return stepWords(argv, scratch) }

// StepPath is a path a step places, collects or reads, in the system's form
// below the scratch folder.
func StepPath(p string) (string, error) { return stepPath(p) }

// ReadGlob is the files of the scratch folder root that glob matches, held to
// the caps at the read.
func ReadGlob(root *os.Root, glob string) ([]plugin.File, error) { return readGlob(root, glob) }

// Collect is the files of the scratch folder root a run named to be read
// back, held to the caps of a read.
func Collect(root *os.Root, paths []string) ([]plugin.File, error) { return collect(root, paths) }

// LocateGrant is the find of the program m grants by id at root, pinned or
// not: the grant read as a step's run reads it, then found.
func LocateGrant(t *Tool, ctx context.Context, root model.Root, m plugin.Manifest, id string, pinned bool) (model.Found, error) {
	g, err := grantOf(m, id)
	if err != nil {
		return model.Found{}, err
	}
	return t.locateGrant(ctx, root, g, pinned, func(context.Context, model.Found) error { return nil })
}

// RunFix is the fix of a version run of the program at path with argv that
// exited non-zero, for the dialect d.
func RunFix(d shell.Dialect, path string, argv []string) string { return runFix(d, path, argv) }
