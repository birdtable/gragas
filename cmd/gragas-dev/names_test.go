package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/dev/scan"
	"github.com/birdtable/gragas/internal/digest"
)

// TestTheNamesScanReadsEveryTrackedFileAsGitStoresIt: the names scan reads
// each file git tracks as it is on disk, a link as its target, which is
// what a push publishes, and leaves out a tracked file deleted from disk and
// a file nobody has added. Planted failures: a read that follows the link,
// which gives the target's bytes; git ls-files with --others, which reads
// new.txt.
func TestTheNamesScanReadsEveryTrackedFileAsGitStoresIt(t *testing.T) {
	t.Parallel()
	dir := scanRepo(t)
	testenv.Link(t, "bin/kept.txt", filepath.Join(dir, "link.txt"))
	gitIn(t, dir, "add", "link.txt")
	if err := os.Remove(filepath.Join(dir, "main.go")); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "new.txt", "not added\n")
	put(t, dir, "bin/kept.txt", "changed on disk\n")
	h := &scanHost{root: dir, environ: os.Environ()}
	got, err := h.trackedFiles(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	names := slices.Sorted(func(yield func(string) bool) {
		for n := range got {
			if !yield(n) {
				return
			}
		}
	})
	want := []string{".gitignore", "bin/kept.txt", "go.sum", "link.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("read %q\nwant %q", names, want)
	}
	if string(got["link.txt"]) != "bin/kept.txt" || string(got["bin/kept.txt"]) != "changed on disk\n" {
		t.Errorf("link.txt = %q, bin/kept.txt = %q: want the link's target and the file as it is on disk", got["link.txt"], got["bin/kept.txt"])
	}
	if _, err := h.trackedFiles(t.Context(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "only the tree or its temp folders") {
		t.Errorf("a folder outside the tree and the temps: %v", err)
	}
}

// TestTheNamesListIsReadOnlyFromAnAbsoluteCleanPath: the host hands the
// scan the list its variable names, and nothing when it is unset; a path
// that is relative or climbs is refused before it is read. Planted failure:
// a host that reads the relative path from the working folder.
func TestTheNamesListIsReadOnlyFromAnAbsoluteCleanPath(t *testing.T) {
	t.Parallel()
	list := filepath.Join(t.TempDir(), "hashes")
	put(t, filepath.Dir(list), "hashes", strings.Repeat("0", 64)+"\n")
	env := func(v string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			if k == scan.NamesHashesVar && v != "" {
				return v, true
			}
			return "", false
		}
	}
	for _, c := range []struct {
		name string
		env  func(string) (string, bool)
		path string
		says string
		is   error
	}{
		{"no environment handed in", nil, "", "", nil},
		{"the variable unset", env(""), "", "", nil},
		{"a relative path", env("hashes"), "", "must be an absolute, clean path", nil},
		{"a path that climbs", env(filepath.Dir(list) + "/x/../hashes"), "", "must be an absolute, clean path", nil},
		{"a file that is not there", env(list + ".gone"), "", "", fs.ErrNotExist},
		{"the list", env(list), list, "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, data, err := (&scanHost{root: t.TempDir(), env: c.env}).namesList()
			switch {
			case err != nil && strings.Contains(err.Error(), filepath.Dir(list)):
				t.Errorf("err %v says where the list lies", err)
			case c.says != "" && (err == nil || !strings.Contains(err.Error(), c.says)):
				t.Errorf("err %v, want one that says %q", err, c.says)
			case c.is != nil && !errors.Is(err, c.is):
				t.Errorf("err %v, want one that is %v", err, c.is)
			case c.says == "" && c.is == nil && err != nil:
				t.Errorf("refused: %v", err)
			case p != c.path || (p != "" && len(data) == 0):
				t.Errorf("path %q with %d bytes, want %q", p, len(data), c.path)
			}
		})
	}
}

// TestTheHashedListIsReadOwned (gragas#1801 item 5): the hashed list is read
// as the names file is, by the owned read, so a list its group can write, a
// link at its path or a folder there is refused, in words that hold no part
// of the path. The control is the same list before its group may write it,
// which is read. Risk: an account that could write the list takes hashes
// out, and the scan passes on names it no longer looks for. Planted: the
// plain os.ReadFile of the list back (the group row reads it, red here).
func TestTheHashedListIsReadOwned(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		plant func(t *testing.T, list string) string
	}{
		{"a list its group can write", func(t *testing.T, list string) string {
			t.Helper()
			trusttest.Loosen(t, list, trusttest.GroupWrites)
			return list
		}},
		{"a link to the list", func(t *testing.T, list string) string {
			t.Helper()
			link := filepath.Join(filepath.Dir(list), "link")
			testenv.Link(t, list, link)
			return link
		}},
		{"a folder at the path", func(t *testing.T, list string) string {
			t.Helper()
			d := filepath.Join(filepath.Dir(list), "folder")
			if err := os.Mkdir(d, 0o700); err != nil {
				t.Fatal(err)
			}
			return d
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			list := filepath.Join(t.TempDir(), "hashes")
			put(t, filepath.Dir(list), "hashes", strings.Repeat("0", 64)+"\n")
			host := func(p string) *scanHost {
				return &scanHost{root: t.TempDir(), env: func(k string) (string, bool) { return p, k == scan.NamesHashesVar }}
			}
			if p, data, err := host(list).namesList(); err != nil || p != list || len(data) == 0 {
				t.Fatalf("the control: %q with %d bytes, %v; want the list read", p, len(data), err)
			}
			at := c.plant(t, list)
			p, data, err := host(at).namesList()
			if err == nil || data != nil || p != "" {
				t.Fatalf("%s: %q with %d bytes, %v; want it refused", c.name, p, len(data), err)
			}
			if !strings.Contains(err.Error(), "this account's own file") || strings.Contains(err.Error(), filepath.Dir(list)) {
				t.Errorf("%s: %v; want the owned read's words and no part of the path", c.name, err)
			}
		})
	}
}

// TestNamesHashPrintsEachNamesLinesAndNeverAName: names hash prints, sorted
// and each once, the SHA-256 of each name's words folded and joined by a
// space, and of its words run together when it has more than one; a blank
// line and a comment are passed over. A refusal names the line by its
// number, and nothing it prints holds a name.
func TestNamesHashPrintsEachNamesLinesAndNeverAName(t *testing.T) {
	t.Parallel()
	sum := func(s string) string {
		return digest.Hex([]byte(s))
	}
	var stdout, stderr bytes.Buffer
	code := runNames(strings.NewReader("# the stand-ins\nAlex\n\n  alex   BOX \nALEX\n"), []string{"hash"}, &stdout, &stderr)
	want := []string{sum("alex"), sum("alex box"), sum("alexbox")}
	slices.Sort(want)
	if code != exitDone || stdout.String() != strings.Join(want, "\n")+"\n" {
		t.Errorf("exit %d, stdout %q\nwant %q", code, stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "3 names, 3 lines") {
		t.Errorf("stderr %q, want the count of names and lines", stderr.String())
	}
	for _, c := range []struct {
		name  string
		input string
		args  []string
		code  int
		says  string
	}{
		{"a name of five words", "alex\nalex box alex box alex\n", []string{"hash"}, exitFailed, "line 2: a name of 5 words"},
		{"a line of no word", "alex\n--\n", []string{"hash"}, exitFailed, "line 2: a name holds no word"},
		{"no name at all", "# only a comment\n\n", []string{"hash"}, exitFailed, "no name was read on stdin"},
		{"a name as an argument", "", []string{"hash", "alex"}, exitUsage, "gragas-dev names hash"},
		{"no subcommand", "alex\n", nil, exitUsage, "gragas-dev names hash"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := runNames(strings.NewReader(c.input), c.args, &stdout, &stderr)
			if got != c.code || !strings.Contains(stderr.String(), c.says) || stdout.Len() != 0 {
				t.Errorf("exit %d, stdout %q, stderr %q: want exit %d saying %q, and nothing on stdout", got, stdout.String(), stderr.String(), c.code, c.says)
			}
			if c.code == exitFailed && strings.Contains(strings.ToLower(stderr.String()), "alex") {
				t.Errorf("the refusal prints a name: %q", stderr.String())
			}
		})
	}
}

// TestTheNamesScanRunsWholeOnARealRepository: the scan proves itself on its
// plant repositories with the git on the PATH,
// then passes a clean repository and turns it red once a tracked file holds
// a name of the list. The list is made from a stand-in at run time.
func TestTheNamesScanRunsWholeOnARealRepository(t *testing.T) {
	t.Parallel()
	dir := scanRepo(t)
	gitIn(t, dir, "update-ref", "refs/remotes/"+scan.NamesBase, "HEAD")
	gitIn(t, dir, "checkout", "-q", "-b", "work")
	lines, err := scan.NameHashes("box")
	if err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(t.TempDir(), "hashes")
	put(t, filepath.Dir(list), "hashes", strings.Join(lines, "\n")+"\n")
	env := func(k string) (string, bool) {
		if k == scan.NamesHashesVar {
			return list, true
		}
		return "", false
	}
	runIn := func() ([]string, error) {
		h := &scanHost{root: dir, env: env, environ: os.Environ()}
		out, err := scan.Names(t.Context(), h.machine())
		return out, errors.Join(err, h.remove())
	}
	out, err := runIn()
	if err != nil {
		t.Fatalf("a clean repository: %v\n%s", err, strings.Join(out, "\n"))
	}
	if got := strings.Join(out, "\n"); !strings.Contains(got, "names: 4 tracked files, the branch, and 0 commits past origin/main") {
		t.Errorf("the input count is missing or wrong:\n%s", got)
	}
	put(t, dir, "notes.txt", "a Box was here\n")
	gitIn(t, dir, "add", "notes.txt")
	out, err = runIn()
	var r *scan.Refusal
	if !errors.As(err, &r) || !slices.Equal(r.Findings, []string{"notes.txt:1: a name of 1 word on the list"}) {
		t.Errorf("a tracked file that holds the name: %v\n%s", err, strings.Join(out, "\n"))
	}
	gitIn(t, dir, "commit", "-q", "--no-verify", "-m", "add a note")
	added := strings.TrimSpace(gitIn(t, dir, "rev-parse", "--short=12", "HEAD"))
	put(t, dir, "notes.txt", "a note\n")
	gitIn(t, dir, "commit", "-q", "--no-verify", "-a", "-m", "tidy the note")
	out, err = runIn()
	// The stand-in's commits carry its address, a finding of their own.
	if !errors.As(err, &r) || !slices.Contains(r.Findings, "commit "+added+": notes.txt:1, a line it adds, holds a name of 1 word on the list") || slices.Contains(r.Findings, "notes.txt:1: a name of 1 word on the list") {
		t.Errorf("a name a pushed commit adds and a later one removes: %v\n%s", err, strings.Join(out, "\n"))
	}
	if got := strings.Join(out, "\n") + err.Error(); strings.Contains(got, list) || strings.Contains(got, filepath.Dir(list)) {
		t.Errorf("the scan printed where the list lies:\n%s", got)
	}
}

// TestTheHashedListOnAnUnjudgedDiskIsReadOnlyWithItsMountNamed (gragas#1897
// part 1), a Bind plant: it needs a disk mounted nosuid, which only a user
// namespace that keeps its capabilities gives without root, so it runs where
// trusttest.Bind can mount one and Bind skips it elsewhere. A hashed list on
// such a disk, whose owners go-files does not judge, is refused in closed
// words that name the mount among their conditions and GRAGAS_MOUNTS in
// their fix, and no part of the path; with the folder the disk is mounted on
// among the settings' mounts, the same list is read. Planted: the settings'
// mounts not handed to the Tree (the second read refused, red here).
func TestTheHashedListOnAnUnjudgedDiskIsReadOnlyWithItsMountNamed(t *testing.T) {
	t.Parallel()
	entry, dir := trusttest.Bind(t, trusttest.Nosuid)
	put(t, dir, "hashes", strings.Repeat("0", 64)+"\n")
	list := filepath.Join(dir, "hashes")
	host := func(mounts []string) *scanHost {
		return &scanHost{
			root:   t.TempDir(),
			env:    func(k string) (string, bool) { return list, k == scan.NamesHashesVar },
			mounts: func() ([]string, error) { return mounts, nil },
		}
	}
	_, data, err := host(nil).namesList()
	if err == nil || data != nil {
		t.Fatalf("a list on a nosuid disk, no mounts named: %d bytes, %v; want it refused", len(data), err)
	}
	if !strings.Contains(err.Error(), "on a disk whose owners are judged or one mounted on a folder GRAGAS_MOUNTS") || !strings.Contains(err.Error(), "name the folder it is mounted on in GRAGAS_MOUNTS") || strings.Contains(err.Error(), dir) {
		t.Errorf("a list on a nosuid disk: %v; want the mount named among the conditions and in the fix, and no part of the path", err)
	}
	if p, data, err := host([]string{entry}).namesList(); err != nil || p != list || len(data) == 0 {
		t.Errorf("the same list with %s among the mounts: %q with %d bytes, %v; want it read", entry, p, len(data), err)
	}
}

// TestTheHashedListIsNotReadWhenTheSettingsCannotBe (gragas#1897 part 1): a
// named list is read on the settings' mounts, so settings that cannot be read
// stop the scan before the list is opened, in words that hold no path. The
// control is the same list with the settings read. Planted: the settings'
// error dropped, so the list is read with no mounts (red here).
func TestTheHashedListIsNotReadWhenTheSettingsCannotBe(t *testing.T) {
	t.Parallel()
	list := filepath.Join(t.TempDir(), "hashes")
	put(t, filepath.Dir(list), "hashes", strings.Repeat("0", 64)+"\n")
	host := func(err error) *scanHost {
		return &scanHost{
			root:   t.TempDir(),
			env:    func(k string) (string, bool) { return list, k == scan.NamesHashesVar },
			mounts: func() ([]string, error) { return nil, err },
		}
	}
	if p, data, err := host(nil).namesList(); err != nil || p != list || len(data) == 0 {
		t.Fatalf("the control: %q with %d bytes, %v; want the list read", p, len(data), err)
	}
	p, data, err := host(errors.New("the settings file " + list + " is refused")).namesList()
	if err == nil || p != "" || data != nil || !strings.Contains(err.Error(), "the settings could not be read") || strings.Contains(err.Error(), filepath.Dir(list)) {
		t.Errorf("settings that cannot be read: %q with %d bytes, %v; want nothing read, the closed words and no path", p, len(data), err)
	}
}
