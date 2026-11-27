package scan_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/scan"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// fake is a host for a scan: the files it was handed and wrote, and each
// tool as a function of the files it is run on. It starts no program.
type fake struct {
	tree map[string]string
	// ignored are files on disk that git ignores.
	ignored map[string]string
	// untracked are files on disk that git neither tracks nor ignores.
	untracked map[string]string
	written   map[string]string
	// indexed are the files a plant repository's git added, by their path.
	indexed map[string]bool
	// depNotices are the NOTICE files beside a library's licence, by its path.
	depNotices map[string]string
	// copies breaks the host's copy of the unignored files: "tracked" or
	// "folder".
	copies  string
	tools   map[string]func(args []string, read func(string) string) scan.Result
	missing string
	temps   int
	// dir is the folder the last program ran in, "" for the tree.
	dir string
	// env is the extra env lines the last program ran with.
	env  []string
	runs []string
	// now is the clock the scan reads.
	now time.Time
	// repos are the names scan's git repositories by folder, "" for the
	// tree, whose files are tree.
	repos map[string]*gitRepo
	// namesPath, namesData and namesErr are what the host hands the names
	// scan as its list.
	namesPath, namesData string
	namesErr             error
	// gitBreak breaks the names scan's git or host: "log" prints no commit,
	// "files" reads no file of a plant repository.
	gitBreak string
	// index and indexErr are what the host hands the commit's scan as the
	// index GIT_INDEX_FILE names; fold makes the host's disk one that folds
	// the case of a name, as the files it lists spell them.
	index    string
	indexErr error
	fold     bool
}

func newFake() *fake {
	return &fake{tree: map[string]string{}, ignored: map[string]string{}, untracked: map[string]string{}, written: map[string]string{}, indexed: map[string]bool{}, depNotices: map[string]string{}, repos: map[string]*gitRepo{}, tools: map[string]func([]string, func(string) string) scan.Result{}}
}

func (f *fake) machine() scan.Machine {
	return scan.Machine{
		Temp: func(context.Context) (string, error) {
			f.temps++
			if f.temps == 1 {
				return "/work/scratch/scan", nil
			}
			return fmt.Sprintf("/work/scratch/scan-%d", f.temps), nil
		},
		Write: func(p string, data []byte) error {
			f.written[p] = string(data)
			return nil
		},
		Run: func(_ context.Context, dir string, env []string, prog string, args ...string) (scan.Result, error) {
			f.dir, f.env = dir, env
			name := path.Base(prog)
			f.runs = append(f.runs, name)
			tool, ok := f.tools[name]
			if !ok {
				return scan.Result{}, errors.New(name + " could not run")
			}
			return tool(args, f.read), nil
		},
		Tool: func(name string) (string, error) {
			if name == f.missing {
				return "", errors.New("the pinned " + name + " is not in .tools/bin: run make tools")
			}
			return "/tools/" + name, nil
		},
		Git: func() (string, error) {
			if f.missing == "git" {
				return "", errors.New("git: executable file not found")
			}
			return "/tools/git", nil
		},
		Go: func() (string, error) {
			if f.missing == "go" {
				return "", errors.New("go: executable file not found")
			}
			return "/tools/go", nil
		},
		DepNotice: func(_ context.Context, licence string) (string, error) { return f.depNotices[licence], nil },
		Tracked: func(context.Context) ([]string, error) {
			var out []string
			for p := range f.tree {
				if !strings.HasPrefix(p, "#") {
					out = append(out, p)
				}
			}
			return out, nil
		},
		CopyUnignored: func(ctx context.Context, dir string) (string, int, error) {
			return f.copyUnignored(ctx, dir)
		},
		TrackedFiles: func(_ context.Context, dir string) (map[string][]byte, error) { return f.trackedFiles(dir), nil },
		NamesList: func() (string, []byte, error) {
			if f.namesErr != nil || f.namesPath == "" {
				return "", nil, f.namesErr
			}
			return f.namesPath, []byte(f.namesData), nil
		},
		Exists: func(dir, name string) (bool, error) { return f.exists(dir, name), nil },
		GitFile: func(_ context.Context, dir, rel string) (bool, error) {
			if dir == "" {
				_, ok := f.tree["#git/"+rel]
				return ok, nil
			}
			_, ok := f.written[dir+"/.git/"+rel]
			return ok, nil
		},
		Read: func(p string) ([]byte, error) {
			s, ok := f.tree[p]
			if !ok {
				s, ok = f.ignored[p]
			}
			if !ok {
				s, ok = f.untracked[p]
			}
			if !ok {
				return nil, os.ErrNotExist
			}
			return []byte(s), nil
		},
		Index:  func() (string, error) { return f.index, f.indexErr },
		Files:  func(dir string) ([]string, error) { return f.files(dir), nil },
		Root:   "/repo",
		Now:    func() time.Time { return f.now },
		GitEnv: []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_NO_REPLACE_OBJECTS=1"},
		Within: func(root, p string) bool { return p == root || strings.HasPrefix(p, root+"/") },
	}
}

// files is every file written under dir, by its path relative to it, each
// once, in lower case when the disk folds case.
func (f *fake) files(dir string) []string {
	var out []string
	for p := range f.written {
		rel, ok := strings.CutPrefix(p, dir+"/")
		if f.fold {
			rel = strings.ToLower(rel)
		}
		if ok && !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// exists is whether dir, the tree for "", holds name on disk: a file of the
// tree, tracked, ignored or not, or one written under dir.
func (f *fake) exists(dir, name string) bool {
	if dir != "" {
		_, ok := f.written[dir+"/"+name]
		return ok
	}
	_, tracked := f.tree[name]
	_, ignored := f.ignored[name]
	_, untracked := f.untracked[name]
	return tracked || ignored || untracked
}

// read is a file the scan wrote, else one of the tree.
func (f *fake) read(p string) string {
	if s, ok := f.written[p]; ok {
		return s
	}
	return f.tree[p]
}

func TestProveStopsAtTheFirstCaseThatMissesItsVerdict(t *testing.T) {
	t.Parallel()
	red := func(v bool) func(context.Context) (bool, error) {
		return func(context.Context) (bool, error) { return v, nil }
	}
	cases := []struct {
		name  string
		cases []scan.Case
		want  string
		lines int
	}{
		{"a plant that stays green", []scan.Case{{Name: "the plant", Red: true, Judge: red(false)}, {Name: "the control", Judge: red(false)}}, "the plant: " + scan.ErrProof.Error() + ": it was green, and it must be red", 0},
		{"a control that turns red", []scan.Case{{Name: "the plant", Red: true, Judge: red(true)}, {Name: "the control", Judge: red(true)}}, "the control: " + scan.ErrProof.Error() + ": it was red, and it must be green", 1},
		{"a case that cannot run", []scan.Case{{Name: "the plant", Red: true, Judge: func(context.Context) (bool, error) { return false, errors.New("no tool") }}}, "the plant: " + scan.ErrProof.Error() + ": no tool", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lines, err := scan.Prove(context.Background(), "s", c.cases...)
			if err == nil || !errors.Is(err, scan.ErrProof) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err %v, want one saying %q", err, c.want)
			}
			if len(lines) != c.lines {
				t.Errorf("%d lines before the failure, want %d: %q", len(lines), c.lines, lines)
			}
		})
	}
	t.Run("a plant red and a control green pass, a line each", func(t *testing.T) {
		lines, err := scan.Prove(context.Background(), "s", scan.Case{Name: "p", Red: true, Judge: red(true)}, scan.Case{Name: "c", Judge: red(false)})
		if err != nil || len(lines) != 2 || lines[0] != "  ok  red: p" || lines[1] != "  ok  green: c" {
			t.Fatalf("lines %q, err %v", lines, err)
		}
	})
}

func TestARefusalNamesEachFindingItsHomeAndTheFix(t *testing.T) {
	t.Parallel()
	r := &scan.Refusal{Scan: "s", Findings: []string{"a:1: x", "b:2: y"}, Home: "the home", Fix: "the fix"}
	want := "s: 2 findings\n  a:1: x\n  b:2: y\n  home: the home\n  fix: the fix"
	if r.Error() != want {
		t.Fatalf("%q\nwant %q", r.Error(), want)
	}
}

// copyUnignored copies, as the host does, the files git does not ignore: in
// the tree, its files and the untracked ones; in a plant repository, every
// file written under it but those under a prefix its .gitignore names. A
// broken host copies only the tracked files when copies is "tracked", and
// every file, ignored or not, when it is "folder".
func (f *fake) copyUnignored(ctx context.Context, dir string) (string, int, error) {
	files := map[string]string{}
	if dir == "" {
		for p, body := range f.tree {
			if !strings.HasPrefix(p, "#") {
				files[p] = body
			}
		}
		if f.copies != "tracked" {
			maps.Copy(files, f.untracked)
		}
		if f.copies == "folder" {
			maps.Copy(files, f.ignored)
		}
	} else {
		ignore := strings.Fields(f.written[dir+"/.gitignore"])
		for p, body := range f.written {
			rel, ok := strings.CutPrefix(p, dir+"/")
			switch {
			case !ok:
			case f.copies == "tracked" && !f.indexed[p]:
			case f.copies != "folder" && slices.ContainsFunc(ignore, func(prefix string) bool { return strings.HasPrefix(rel, prefix) }):
			default:
				files[rel] = body
			}
		}
	}
	to, err := f.machine().Temp(ctx)
	if err != nil {
		return "", 0, err
	}
	for p, body := range files {
		f.written[to+"/"+p] = body
	}
	return to, len(files), nil
}

// shallow is what the fake git's rev-parse --is-shallow-repository prints
// in dir: for the tree, #shallow, false when unset; for a plant
// repository, whether its git folder holds a shallow file.
func shallow(f *fake, dir string) string {
	if dir == "" {
		return cmp.Or(f.tree["#shallow"], "false")
	}
	_, ok := f.written[dir+"/.git/shallow"]
	return strconv.FormatBool(ok)
}
