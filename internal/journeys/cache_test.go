package journeys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// folders are fresh state and cache folders, so a golden never holds what
// another journey left in the shared ones.
func folders(t *testing.T) (env map[string]string, state string) {
	t.Helper()
	root := t.TempDir()
	env = map[string]string{stateKey: filepath.Join(root, "state"), cacheKey: filepath.Join(root, "cache")}
	return env, appFolders(t, env).State
}

// TestTheCacheListGoldens: cache list on an empty machine and its help, as a
// pipe at 80, 60 and 40 columns, each against its golden, written only with
// -update by a person who reads the diff (cli.look.8). Risk: a listing that
// changes shape unseen.
func TestTheCacheListGoldens(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	for _, cols := range []string{"80", "60", "40"} {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{"cache-list", []string{"cache", "list"}},
			{"cache-help", []string{"cache", "--help"}},
		} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				env, _ := folders(t)
				env["COLUMNS"] = cols
				r := run(t, repo, env, tc.args...)
				got := "$ gragas " + strings.Join(tc.args, " ") + "\n--- stdout\n" + r.stdout + "--- stderr\n" + r.stderr
				uitest.Golden(t, tc.name+"-"+cols, got)
			})
		}
	}
}

// TestCacheListJSONIsOneDocumentAndNullsSayWhy: --json is one document with a
// schema number; an empty machine holds nothing (size 0), a figure with no
// answer is null, and the reason is beside it (cli.output.9).
func TestCacheListJSONIsOneDocumentAndNullsSayWhy(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	r := run(t, fixture(t), env, "cache", "list", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var doc struct {
		Schema float64 `json:"schema"`
		Keep   string  `json:"keep"`
		Kinds  []struct {
			Kind       string   `json:"kind"`
			Size       *float64 `json:"size"`
			Oldest     *float64 `json:"oldest"`
			OldestNote string   `json:"oldest_note"`
		} `json:"kinds"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	// Six kinds: the tools kind of a grant's cache_vars is listed.
	if doc.Schema != 1 || doc.Keep != "30d" || len(doc.Kinds) != 6 {
		t.Fatalf("schema %v keep %q kinds %d", doc.Schema, doc.Keep, len(doc.Kinds))
	}
	for _, k := range doc.Kinds {
		if k.Size == nil || *k.Size != 0 || k.Oldest != nil || k.OldestNote == "" {
			t.Errorf("%s: %+v, want size 0 and a null oldest with its reason", k.Kind, k)
		}
	}
	uitest.Golden(t, "json-cache-list", r.stdout)
}

// snapshot is every path under dirs with its kind, and a file's size and
// modification time, so a file made, changed, touched or removed, and a folder
// made, show. A folder's own size and time are not read: what a system keeps
// there is its own (a size of blocks or entries, a time it moves when a name is
// added), and a name added or removed shows as a path.
// Each path's own record is read with Lstat, not the walk's entry: a file
// system may answer the entry from the folder's copy, brought up to date
// later, as when a reader opens the file, so a run that only read would show
// as a change.
func snapshot(t *testing.T, dirs ...string) []string {
	t.Helper()
	var out []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) && p == dir {
				out = append(out, p+" absent")
				return filepath.SkipDir
			}
			if err != nil {
				return err
			}
			info, err := os.Lstat(p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				out = append(out, p+" dir")
				return nil
			}
			out = append(out, fmt.Sprintf("%s file size=%d mtime=%d", p, info.Size(), info.ModTime().UnixNano()))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestCacheListReadsAndWritesNothing: against a state folder that holds an
// open run and a cache folder that holds a file, list counts them and leaves
// both folders exactly as it found them: no file made, changed, touched or
// removed, and no folder made. It reads through the Tree and never writes (the
// persisted-files test of the issue follows with clear, the only cache
// command that writes). Risk: a listing that creates the folder it lists.
func TestCacheListReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	env, state := folders(t)
	cache := appFolders(t, env).Cache
	for dir, files := range map[string]map[string]string{
		state: {
			"aa.journal":        runlogLine(t, "r1", json.RawMessage(`{"v":1,"kind":"intent","at":1000}`)) + "\n",
			"aa.copies/r1/bb":   "copy",
			"unrelated.keep-me": "x",
		},
		cache: {"explain/index": "index"},
	} {
		for name, body := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A folder that is not there stays not there.
	absent := filepath.Join(filepath.Dir(cache), "never-made")
	before := snapshot(t, state, cache, absent)
	r := run(t, fixture(t), env, "cache", "list", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var doc struct {
		Kinds []struct {
			Kind  string `json:"kind"`
			Files int    `json:"files"`
			Open  *int   `json:"open"`
		} `json:"kinds"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if j := doc.Kinds[0]; j.Kind != "journal" || j.Files != 2 || j.Open == nil || *j.Open != 1 {
		t.Errorf("journal row %+v, want two files and one open run", j)
	}
	if e := doc.Kinds[1]; e.Kind != "explain" || e.Files != 1 {
		t.Errorf("explain row %+v, want the one file of the cache folder", e)
	}
	if after := snapshot(t, state, cache, absent); !slices.Equal(before, after) {
		t.Errorf("a list changed what it read:\nbefore %v\nafter  %v", before, after)
	}
}

// put writes files under dir, making the folders between.
func put(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestThePopulatedCacheListGoldens: cache list over a state folder that holds
// closed runs, one still open and their copies, and a cache folder that holds
// an entry of a kind that dates nothing; and over a journal with a line of a
// newer version, whose age is null with its note. At 80, 60 and 40 columns,
// each against its golden, written only with -update by a person who reads
// the diff. The oldest is a date so that the clock does not move a golden.
// Risk: a listing whose populated shape, its null and its note change unseen.
func TestThePopulatedCacheListGoldens(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	cases := map[string]struct{ state, cache map[string]string }{
		"cache-list-populated": {
			state: map[string]string{
				"aa.journal": strings.Join([]string{
					runlogLine(t, "r1", json.RawMessage(`{"v":1,"kind":"intent","at":1770000000}`), json.RawMessage(`{"v":1,"kind":"close","at":1770000100}`)),
					runlogLine(t, "r2", json.RawMessage(`{"v":1,"kind":"intent","at":1775000000}`)),
					runlogLine(t, "r3", json.RawMessage(`{"v":1,"kind":"intent","at":1778000000}`), json.RawMessage(`{"v":1,"kind":"close","at":1778000200}`)),
				}, "\n") + "\n",
				"aa.copies/r1/dd": "a copy of a file a run replaced",
				"bb.journal":      runlogLine(t, "r9", json.RawMessage(`{"v":1,"kind":"intent","at":1779000000}`)) + "\n",
			},
			cache: map[string]string{"explain/index": "an index of some length, kept for explain"},
		},
		"cache-list-newer": {
			state: map[string]string{"aa.journal": runlogLine(t, "r1", json.RawMessage(`{"v":9,"kind":"close","at":1770000100}`)) + "\n"},
		},
	}
	for name, tc := range cases {
		for _, cols := range []string{"80", "60", "40"} {
			t.Run(name+"-"+cols, func(t *testing.T) {
				env, state := folders(t)
				put(t, state, tc.state)
				put(t, appFolders(t, env).Cache, tc.cache)
				env["COLUMNS"] = cols
				r := run(t, repo, env, "cache", "list")
				if r.code != 0 {
					t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
				}
				uitest.Golden(t, name+"-"+cols, "$ gragas cache list\n--- stdout\n"+r.stdout+"--- stderr\n"+r.stderr)
			})
		}
	}
}
