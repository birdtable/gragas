package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/census"
	"github.com/birdtable/gragas/internal/ordered"
)

// osAPIList is the decision for each standard-library identifier that
// differs by system (gragas#1750, row L14 of gragas#1719 D).
const osAPIList = "gate/os-api.txt"

// takenCensus is the census of std at the toolchain go.mod names, under t's
// context. A go tool of another release fails the test: std differs between
// releases, and the list is held to the one the module builds with.
func takenCensus(t *testing.T) *census.Census {
	t.Helper()
	c, err := takeCensus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func takeCensus(ctx context.Context) (*census.Census, error) {
	goTool := func(_ context.Context, env []string, args ...string) ([]byte, error) {
		cmd := inRoot(env, "go", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
		}
		return out, nil
	}
	want, err := goModToolchain()
	if err != nil {
		return nil, err
	}
	got, err := goTool(ctx, nil, "env", "GOVERSION")
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(string(got)); v != want {
		return nil, fmt.Errorf("the go tool is %s and go.mod's toolchain is %s: the census is of std at the toolchain the module builds with", v, want)
	}
	goroot, err := goTool(ctx, nil, "env", "GOROOT")
	if err != nil {
		return nil, err
	}
	return census.Take(ctx, goTool, os.DirFS(strings.TrimSpace(string(goroot))))
}

// goModToolchain is the release go.mod's toolchain line names.
func goModToolchain() (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "toolchain "); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", errors.New("go.mod has no toolchain line, so the census has no release to be taken at")
}

// lintRows is what of a lint config a ban is held to: every forbidigo row,
// and every deny of every depguard rule with its rule's files. A config
// with neither holds no ban, which fails: a ban line is held to a row, and
// with no rows none could be.
func lintRows(doc *ordered.Map) (census.Lint, error) {
	var lint census.Lint
	if v, ok := lookup(doc, "linters.settings.forbidigo.forbid"); ok {
		l, _ := v.([]any)
		for _, f := range ordered.Maps(l) {
			lint.Forbid = append(lint.Forbid, census.Forbid{Pattern: f.Str("pattern"), Pkg: f.Str("pkg"), Msg: f.Str("msg")})
		}
	}
	if v, ok := lookup(doc, "linters.settings.depguard.rules"); ok {
		rules, _ := v.(*ordered.Map)
		for _, name := range keysOf(rules) {
			r := rules.Sub(name)
			if r == nil {
				continue
			}
			files := ordered.Strings(r.List("files"))
			for _, d := range ordered.Maps(r.List("deny")) {
				lint.Deny = append(lint.Deny, census.Deny{Pkg: d.Str("pkg"), Desc: d.Str("desc"), Files: files})
			}
		}
	}
	switch {
	case len(lint.Forbid) == 0:
		return lint, errors.New("the lint config holds no forbidigo row, so no ban could be held to one")
	case len(lint.Deny) == 0:
		return lint, errors.New("the lint config holds no depguard deny, so no package ban could be held to one")
	}
	return lint, nil
}

// keysOf is m's keys, and none for no map.
func keysOf(m *ordered.Map) []string {
	if m == nil {
		return nil
	}
	return m.Keys()
}

// osAPIDecisions is the list as the tree has it, read.
func osAPIDecisions(t *testing.T) []census.Decision {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, osAPIList))
	if err != nil {
		t.Fatal(err)
	}
	ds, bad := census.ParseDecisions(string(data))
	for _, p := range bad {
		t.Error(osAPIList + ": " + p)
	}
	if len(ds) == 0 {
		t.Fatal(osAPIList + " holds no decision")
	}
	return ds
}

// TestEveryIdentifierThatDiffersBySystemIsDecided is done-when 4 of
// gragas#1750: the census of the module's toolchain, each identifier with
// one line of gate/os-api.txt, each line with an identifier, and each ban
// with the lint row that refuses it. It reads the list at the merge base
// with git too, so it runs with -count=1 (docs/checks.md): a cached pass
// would outlive a fetch.
//
// The census starts the go tool once for the dist list and once a port, so
// it is taken once here, and the plants run on the same one as a subtest:
// no package variable holds it between tests.
func TestEveryIdentifierThatDiffersBySystemIsDecided(t *testing.T) {
	t.Parallel()
	c := takenCensus(t)
	t.Run("the list", func(t *testing.T) {
		t.Parallel()
		theListIsDecided(t, c)
	})
	t.Run("its plants", func(t *testing.T) {
		t.Parallel()
		theDecisionsFailOnTheirPlants(t, c)
	})
	t.Run("its floor", func(t *testing.T) {
		t.Parallel()
		theCensusHoldsItsFloor(t, c)
	})
}

// theCensusHoldsItsFloor holds the census c to its floor, and plants the
// settling on it: with internal/filepathlite.volumeNameLen's root line gone,
// path/filepath.Base, which differs only through it, wants a line of its own.
//
// The floor is what the rows of gragas#1750 name, and the review of
// #1854 found the census missed. Left out, each with its reason:
// runtime.GOARCH, since the compared ports share one GOARCH, so it differs by
// none of them; os.ModePerm, os.ModeSetuid and fs.FileMode.Perm, one
// declaration on every port, whose use as a mode row L6's forbidigo refuses;
// and os.File.Chown, which reaches the system only through a method of
// another package's type, which the census does not follow.
func theCensusHoldsItsFloor(t *testing.T, c *census.Census) {
	t.Helper()
	floor := strings.Fields(`
os.Chdir os.Chmod os.Chown os.CreateTemp os.File.Chmod os.FindProcess
os.Getegid os.Geteuid os.Getgid os.Getgroups os.Getuid os.Getwd os.Hostname
os.IsPathSeparator os.Lchown os.Link os.MkdirTemp os.PathListSeparator
os.PathSeparator os.ProcAttr os.Process.Kill os.Process.Signal os.Readlink
os.Remove os.RemoveAll os.Rename os.Root.Chmod os.Root.Chown os.Root.Lchown
os.Root.Link os.Root.Readlink os.Root.Remove os.Root.RemoveAll os.Root.Rename
os.Root.Symlink os.StartProcess os.Stderr os.Stdout os.Symlink os.TempDir
path/filepath.Base path/filepath.Dir path/filepath.EvalSymlinks
path/filepath.Ext path/filepath.Glob path/filepath.HasPrefix
path/filepath.IsAbs path/filepath.IsLocal path/filepath.ListSeparator
path/filepath.Match path/filepath.Separator path/filepath.Split
path/filepath.ToSlash path/filepath.VolumeName runtime.GOOS
`)
	found := map[string]bool{}
	for _, id := range c.Differ {
		found[id.Name] = true
	}
	for _, name := range floor {
		if !found[name] {
			t.Errorf("the census does not find %s to differ by system, and a row of gragas#1750 names it", name)
		}
	}
	lint, err := lintRows(lintDoc(t, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ds := osAPIDecisions(t)
	gone := slices.DeleteFunc(slices.Clone(ds), func(d census.Decision) bool { return d.Name == "internal/filepathlite.volumeNameLen" })
	if len(gone) == len(ds) {
		t.Fatal("gate/os-api.txt holds no line on internal/filepathlite.volumeNameLen, so the settling is planted on nothing")
	}
	if got := census.Hold(c, gone, lint); !slices.ContainsFunc(got, func(p string) bool { return strings.HasPrefix(p, "path/filepath.Base differs by system") }) {
		t.Errorf("with the root path/filepath.Base reaches undecided, it wants a line; the problems are %v", got)
	}
}

// theListIsDecided holds gate/os-api.txt to the census c and the lint, and
// its later lines to the merge base's.
func theListIsDecided(t *testing.T, c *census.Census) {
	t.Helper()
	ds := osAPIDecisions(t)
	lint, err := lintRows(lintDoc(t, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range census.Hold(c, ds, lint) {
		t.Error(osAPIList + ": " + p)
	}
	base, why, missing := osListBase(t, root, osAPIList)
	switch {
	case missing:
		t.Log("the list is not on main yet, so its later lines are held to no base: " + why)
	case why != "":
		t.Fatal(why)
	default:
		was, bad := census.ParseDecisions(base)
		if len(bad) != 0 {
			t.Fatalf("%s at the merge base does not read, so the later lines cannot be held to it: %v", osAPIList, bad)
		}
		for _, p := range census.LaterGrown(ds, was) {
			t.Error(osAPIList + ": " + p)
		}
	}
}

// theDecisionsFailOnTheirPlants plants on the real census c, list and lint:
// an identifier with its line gone, a ban with its lint row gone, a census
// that read nothing, and a lint config with no rows.
func theDecisionsFailOnTheirPlants(t *testing.T, c *census.Census) {
	t.Helper()
	ds := osAPIDecisions(t)
	doc := lintDoc(t, ".golangci.yml")
	lint, err := lintRows(doc)
	if err != nil {
		t.Fatal(err)
	}
	says := func(name string, got []string, want string) {
		t.Helper()
		if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, want) }) {
			t.Errorf("%s: want a problem that says %q, got %v", name, want, got)
		}
	}
	gone := slices.DeleteFunc(slices.Clone(ds), func(d census.Decision) bool { return d.Name == "os.Chmod" })
	says("an identifier with no line", census.Hold(c, gone, lint), "os.Chmod differs by system")

	noRow := lint
	noRow.Forbid = slices.DeleteFunc(slices.Clone(lint.Forbid), func(f census.Forbid) bool { return strings.Contains(f.Pattern, "Exit") })
	says("a ban with its forbidigo row gone", census.Hold(c, ds, noRow), "os.Exit is banned under go.errors.4 and no forbidigo row")

	noDeny := lint
	noDeny.Deny = slices.DeleteFunc(slices.Clone(lint.Deny), func(d census.Deny) bool { return d.Pkg == "syscall" })
	says("a package ban with its depguard deny gone", census.Hold(c, ds, noDeny), "syscall.* is banned under arch.ports.24 and no depguard rule")

	says("a census that read nothing", census.Hold(&census.Census{}, ds, lint), census.ErrNoIdent.Error())

	if _, err := lintRows(ordered.NewMap(1)); err == nil {
		t.Error("a lint config with no forbidigo row and no depguard deny was read as one a ban can be held to")
	}
	if len(c.Differ) < 1000 || len(c.Ports) < 2 {
		t.Errorf("the census found %d identifiers for %d ports: too few to mean anything", len(c.Differ), len(c.Ports))
	}
}
