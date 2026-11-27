package git_test

import (
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// TestNothingIsFetchedThroughAReleasesCacheNameTheWalkRefuses: Fetch judges
// every name of the releases cache
// that is there before anything is made, so a refused Fetch leaves nothing
// wherever a name another account controls leads. Risk: a link in a folder
// others may write sends the making of the releases cache, and the fetched
// tree in it, into a folder of that account's choosing. The probe is the
// cache open/c/releases, open written by others and c a link to the victim,
// the cache itself a link, open/c2 to the victim, and open/new, not there
// yet, whose nearest folder that is there, open, others may write. The
// control is a cache that is not there yet in a private folder, made, and v1
// fetched into it, as before. Planted: privateRoot making the folders before
// the walk (each victim then holds the cache, red here).
func TestNothingIsFetchedThroughAReleasesCacheNameTheWalkRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		// rels is the releases cache, and empty the folder that must hold
		// nothing after the refusal.
		rels func(t *testing.T, open, victim string) (rels, empty string)
	}{
		{"below a link in a folder others may write", func(t *testing.T, open, victim string) (string, string) {
			t.Helper()
			testenv.Link(t, victim, filepath.Join(open, "c"))
			return filepath.Join(open, "c", "releases"), victim
		}},
		{"that is a link in a folder others may write", func(t *testing.T, open, victim string) (string, string) {
			t.Helper()
			testenv.Link(t, victim, filepath.Join(open, "c2"))
			return filepath.Join(open, "c2"), victim
		}},
		{"not there yet, whose nearest folder others may write", func(_ *testing.T, open, _ string) (string, string) {
			return filepath.Join(open, "new"), open
		}},
	} {
		t.Run("a releases cache "+tc.what, func(t *testing.T) {
			t.Parallel()
			w := newRemote(t, map[string]string{"a.txt": "a\n"})
			open, victim := filepath.Join(w.base, "open"), resolved(t, filestest.Private(t))
			mkdir(t, open)
			var empty string
			w.rels, empty = tc.rels(t, open, victim)
			trusttest.Loosen(t, open, trusttest.OthersWrite)
			got, err := w.adapter(t).Fetch(t.Context(), w.src, "v1")
			if !isRefused(err) || got != "" || !strings.Contains(err.Error(), w.rels) || !strings.Contains(err.Error(), open) {
				t.Errorf("Fetch into %s = %q, %v; want refused by the cache's name and %s's", w.rels, got, err, open)
			}
			if left := entries(t, empty); len(left) != 0 {
				t.Errorf("%s holds %v after a refused Fetch, want nothing", empty, left)
			}
		})
	}
	t.Run("the control, a releases cache that is not there yet", func(t *testing.T) {
		t.Parallel()
		w := newRemote(t, map[string]string{"a.txt": "a\n"})
		w.rels = filepath.Join(w.base, "new", "releases")
		root, err := w.adapter(t).Fetch(t.Context(), w.src, "v1")
		rel, rerr := filepath.Rel(w.rels, string(root))
		if err != nil || rerr != nil || rel == "." || !filepath.IsLocal(rel) {
			t.Fatalf("Fetch into a cache not there yet = %q, %v; want v1 fetched into %s", root, err, w.rels)
		}
	})
}

// TestTheReleasesCacheWalkTrustsTheConfiguredMounts: the walk of the releases
// cache Fetch opens trusts the configured
// mounts, as every other walk of the product does, so a cache on a nosuid
// disk GRAGAS_MOUNTS names passes there. Risk: on such a disk, Fetch refused
// with a fix line that names the setting the person already set. As in
// host/fs, go-files' judgment of each entry before it walks
// shows the mounts reach the walk on any system: a configured mount that
// names nothing is refused by its name, with host/trust's MountEntryFix, in
// the cache's words, where go-files judges entries, and the cache is made
// where it judges none. The cache is opened as Fetch opens it, through
// OpenCache: Fetch walks the source first with the same mounts, so its
// refusal would be the source's whichever way the cache is walked. Planted:
// privateRoot walking with hosttrust.Who{} (the cache made, red where entries
// are judged, as on the Mac).
func TestTheReleasesCacheWalkTrustsTheConfiguredMounts(t *testing.T) {
	t.Parallel()
	base := resolved(t, filestest.Private(t))
	judged := mountsJudged(t, base)
	m, rels := filepath.Join(base, "gone"), filepath.Join(base, "releases")
	g, err := git.New(git.Config{Git: configtest.Git(t), Releases: rels, Home: filestest.Private(t), Run: "run-1", Mounts: []string{m}})
	if err != nil {
		t.Fatal(err)
	}
	err = git.OpenCache(g, rels)
	r, ok := errors.AsType[*git.Refused](err)
	refused := ok && strings.HasPrefix(r.Observed(), "the releases cache "+rels+" is not trusted: the mount "+m) && r.Hint() == hosttrust.MountEntryFix
	if judged && !refused || !judged && err != nil {
		t.Errorf("the releases cache %s with the configured mount %s, which names nothing: %v; want the mount refused by its name, with MountEntryFix, in the cache's words, where go-files judges entries (%v), else the cache made", rels, m, err, judged)
	}
}

// mountsJudged is whether go-files judges a configured mount on this system,
// asked of go-files itself: a walk of dir passes with no
// mount, and is refused with an entry that is not absolute wherever go-files
// judges one. Where it judges none, the test holds that the cache is made and
// says so in its log, since a skip needs an allow-list entry (tests.admission.5).
func mountsJudged(t *testing.T, dir string) bool {
	t.Helper()
	probe := hosttrust.Subject{Name: "the probe", Last: hosttrust.Folder}
	if _, err := hosttrust.Walk(probe, dir, hosttrust.Who{}); err != nil {
		t.Fatalf("the probe's walk of %s with no mount: %v", dir, err)
	}
	if _, err := hosttrust.Walk(probe, dir, hosttrust.Who{Mounts: []string{"relative"}}); err == nil {
		t.Log("go-files judges no configured mount on this system: the mounts are held to pass unjudged")
		return false
	}
	return true
}

// entries is the names in dir, links not followed.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}
